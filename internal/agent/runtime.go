package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-claw/internal/tool"
	"io"
	"net/http"
	"time"
)

// Runtime Agent运行时（处理LLM调用和工具执行）
type Runtime struct {
	config *Config
	client *http.Client
}

// NewRuntime 创建运行时
func NewRuntime(cfg *Config) *Runtime {
	return &Runtime{
		config: cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// ChatMessage 对话消息
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Execute 执行Agent循环
func (r *Runtime) Execute(ctx context.Context, session *Session, tools []tool.Tool, maxIterations int) (string, error) {
	// 构建消息列表
	messages := r.buildMessages(session)

	for i := 0; i < maxIterations; i++ {
		// 调用LLM
		resp, err := r.callLLM(ctx, messages, tools)
		if err != nil {
			return "", err
		}

		// 如果没有工具调用，返回最终响应
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// 执行工具调用
		assistantMsg := ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		// 执行每个工具
		for _, tc := range resp.ToolCalls {
			result, err := r.executeTool(ctx, tc, tools)
			if err != nil {
				result = fmt.Sprintf("工具执行错误: %v", err)
			}

			// 添加工具结果
			toolMsg := ChatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			}
			messages = append(messages, toolMsg)
		}
	}

	return "达到最大迭代次数", nil
}

// callLLM 调用大模型API
func (r *Runtime) callLLM(ctx context.Context, messages []ChatMessage, tools []tool.Tool) (*ChatMessage, error) {
	// 构建请求体
	reqBody := map[string]interface{}{
		"model":       r.config.Model,
		"messages":    messages,
		"max_tokens":  2000,
		"temperature": 0.7,
	}

	// 如果有工具，添加到请求
	if len(tools) > 0 {
		reqBody["tools"] = r.convertTools(tools)
		reqBody["tool_choice"] = "auto"
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	// 发送请求
	url := r.config.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.config.APIKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 解析响应
	var llmResp struct {
		Choices []struct {
			Message struct {
				Role      string     `json:"role"`
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &llmResp); err != nil {
		return nil, err
	}

	if len(llmResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	msg := &ChatMessage{
		Role:      llmResp.Choices[0].Message.Role,
		Content:   llmResp.Choices[0].Message.Content,
		ToolCalls: llmResp.Choices[0].Message.ToolCalls,
	}

	return msg, nil
}

// executeTool 执行工具
func (r *Runtime) executeTool(ctx context.Context, tc ToolCall, tools []tool.Tool) (string, error) {
	// 查找工具
	var targetTool tool.Tool
	for _, t := range tools {
		if t.Name() == tc.Function.Name {
			targetTool = t
			break
		}
	}

	if targetTool == nil {
		return "", fmt.Errorf("tool not found: %s", tc.Function.Name)
	}

	// 解析参数
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
		return "", err
	}

	// 执行工具
	result, err := targetTool.Execute(ctx, params)
	if err != nil {
		return "", err
	}

	return result, nil
}

// convertTools 转换工具格式
func (r *Runtime) convertTools(tools []tool.Tool) []map[string]interface{} {
	var openAITools []map[string]interface{}

	for _, t := range tools {
		openAITools = append(openAITools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.Parameters(),
			},
		})
	}

	return openAITools
}

// buildMessages 构建消息列表（包含系统提示词）
func (r *Runtime) buildMessages(session *Session) []ChatMessage {
	messages := []ChatMessage{
		{Role: "system", Content: r.config.SystemPrompt},
	}

	for _, msg := range session.Messages {
		messages = append(messages, ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return messages
}
