package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-claw/internal/tool"
	"io"
	"net/http"
	"strings"
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
		client: &http.Client{Timeout: 60 * time.Second},
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

// Execute 执行Agent循环（阻塞版）
func (r *Runtime) Execute(ctx context.Context, session *Session, tools []tool.Tool, maxIterations int) (string, error) {
	messages := r.buildMessages(session)

	for i := 0; i < maxIterations; i++ {
		resp, err := r.callLLM(ctx, messages, tools)
		if err != nil {
			return "", err
		}

		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		assistantMsg := ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		for _, tc := range resp.ToolCalls {
			result, err := r.executeTool(ctx, tc, tools)
			if err != nil {
				result = fmt.Sprintf("工具执行错误: %v", err)
			}

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

// StreamCallback 流式回调
type StreamCallback func(chunk string)

// ExecuteStream 执行Agent循环（流式版）
func (r *Runtime) ExecuteStream(ctx context.Context, session *Session, tools []tool.Tool, maxIterations int, cb StreamCallback) (string, error) {
	messages := r.buildMessages(session)

	for i := 0; i < maxIterations; i++ {
		var fullContent strings.Builder
		var toolCalls []ToolCall
		var err error

		// 如果有工具，先用非流式调用判断是否需要工具调用
		if len(tools) > 0 {
			resp, err := r.callLLM(ctx, messages, tools)
			if err != nil {
				return "", err
			}
			if len(resp.ToolCalls) > 0 {
				// 需要工具调用，走阻塞路径
				assistantMsg := ChatMessage{
					Role:      "assistant",
					Content:   resp.Content,
					ToolCalls: resp.ToolCalls,
				}
				messages = append(messages, assistantMsg)

				for _, tc := range resp.ToolCalls {
					result, err := r.executeTool(ctx, tc, tools)
					if err != nil {
						result = fmt.Sprintf("工具执行错误: %v", err)
					}
					toolMsg := ChatMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    result,
					}
					messages = append(messages, toolMsg)
				}
				continue
			}
			// 不需要工具，走流式
			toolCalls = resp.ToolCalls
			fullContent.WriteString(resp.Content)
			for _, ch := range resp.Content {
				cb(string(ch))
			}
		} else {
			fullContent, toolCalls, err = r.callLLMStream(ctx, messages, cb)
			if err != nil {
				return "", err
			}
		}

		if len(toolCalls) == 0 {
			return fullContent.String(), nil
		}
	}

	return "达到最大迭代次数", nil
}

// callLLM 调用大模型API（阻塞版）
func (r *Runtime) callLLM(ctx context.Context, messages []ChatMessage, tools []tool.Tool) (*ChatMessage, error) {
	reqBody := r.buildRequest(messages, tools)
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

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
		return nil, fmt.Errorf("no response from LLM: %s", string(body))
	}

	msg := &ChatMessage{
		Role:      llmResp.Choices[0].Message.Role,
		Content:   llmResp.Choices[0].Message.Content,
		ToolCalls: llmResp.Choices[0].Message.ToolCalls,
	}

	return msg, nil
}

// callLLMStream 调用大模型API（流式版）
func (r *Runtime) callLLMStream(ctx context.Context, messages []ChatMessage, cb StreamCallback) (strings.Builder, []ToolCall, error) {
	reqBody := r.buildRequest(messages, nil)
	reqBody["stream"] = true
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return strings.Builder{}, nil, err
	}

	url := r.config.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return strings.Builder{}, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.config.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := r.client.Do(req)
	if err != nil {
		return strings.Builder{}, nil, err
	}
	defer resp.Body.Close()

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			if content != "" {
				fullContent.WriteString(content)
				if cb != nil {
					cb(content)
				}
			}
			if chunk.Choices[0].FinishReason != "" {
				break
			}
		}
	}

	return fullContent, nil, nil
}

func (r *Runtime) buildRequest(messages []ChatMessage, tools []tool.Tool) map[string]interface{} {
	reqBody := map[string]interface{}{
		"model":       r.config.Model,
		"messages":    messages,
		"max_tokens":  2000,
		"temperature": 0.7,
	}

	if len(tools) > 0 {
		reqBody["tools"] = r.convertTools(tools)
		reqBody["tool_choice"] = "auto"
	}

	return reqBody
}

// executeTool 执行工具
func (r *Runtime) executeTool(ctx context.Context, tc ToolCall, tools []tool.Tool) (string, error) {
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

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
		return "", err
	}

	return targetTool.Execute(ctx, params)
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

// buildMessages 构建消息列表
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
