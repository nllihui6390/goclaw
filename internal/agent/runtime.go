package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-claw/internal/tool"
	glog "go-claw/pkg/log"
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

// StreamCallback 流式回调
type StreamCallback func(chunk string)

// ToolEventHandler 工具事件回调（输出工具调用过程）
type ToolEventHandler func(event ToolEvent)

// ToolEvent 工具执行事件
type ToolEvent struct {
	Type     string // "calling", "result", "error", "thinking"
	ToolName string
	Args     string
	Result   string
	Error    string
	Thinking string
}

// Execute 执行Agent循环（阻塞版）
func (r *Runtime) Execute(ctx context.Context, session *Session, tools []tool.Tool, maxIterations int, handler ToolEventHandler) (string, error) {
	logger := glog.Logger()
	messages := r.buildMessages(session)
	logger.Info("[Runtime] 开始执行循环",
		"model", r.config.Model,
		"provider", r.config.ProviderType,
		"messages_count", len(messages),
		"tools_count", len(tools),
		"max_iterations", maxIterations)

	for i := 0; i < maxIterations; i++ {
		logger.Info("[Runtime] 迭代开始", "iteration", i+1, "messages_count", len(messages))

		// 每次迭代开始时通知用户正在思考
		if handler != nil {
			handler(ToolEvent{
				Type:     "thinking",
				Thinking: "正在思考...",
			})
		}

		resp, err := r.callLLM(ctx, messages, tools)
		if err != nil {
			logger.Error("[Runtime] LLM调用失败", "iteration", i+1, "err", err)
			return "", err
		}

		logger.Info("[Runtime] LLM响应收到",
			"iteration", i+1,
			"content_len", len(resp.Content),
			"tool_calls_count", len(resp.ToolCalls))

		// 输出思考内容
		if handler != nil && resp.Content != "" && len(resp.ToolCalls) > 0 {
			handler(ToolEvent{
				Type:     "thinking",
				Thinking: resp.Content,
			})
		}

		if len(resp.ToolCalls) == 0 {
			logger.Info("[Runtime] 无工具调用，返回最终响应", "iteration", i+1)
			return resp.Content, nil
		}

		logger.Info("[Runtime] 检测到工具调用", "count", len(resp.ToolCalls))

		assistantMsg := ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		for idx, tc := range resp.ToolCalls {
			logger.Info("[Runtime] 执行工具",
				"iteration", i+1,
				"tool_idx", idx+1,
				"tool_name", tc.Function.Name,
				"tool_call_id", tc.ID)

			// 输出工具调用开始
			if handler != nil {
				handler(ToolEvent{
					Type:     "calling",
					ToolName: tc.Function.Name,
					Args:     tc.Function.Arguments,
				})
			}

			result, err := r.executeTool(ctx, tc, tools)
			if err != nil {
				result = fmt.Sprintf("工具执行错误: %v", err)
				logger.Error("[Runtime] 工具执行出错",
					"tool_name", tc.Function.Name,
					"err", err)
				if handler != nil {
					handler(ToolEvent{
						Type:     "error",
						ToolName: tc.Function.Name,
						Error:    err.Error(),
					})
				}
			} else {
				logger.Info("[Runtime] 工具执行成功",
					"tool_name", tc.Function.Name,
					"result_len", len(result))
				// 输出工具执行结果
				if handler != nil {
					handler(ToolEvent{
						Type:     "result",
						ToolName: tc.Function.Name,
						Result:   result,
					})
				}
			}

			toolMsg := ChatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			}
			messages = append(messages, toolMsg)
		}
	}

	logger.Warn("[Runtime] 达到最大迭代次数", "max_iterations", maxIterations)
	return "达到最大迭代次数", nil
}

// ExecuteStream 执行Agent循环（流式版）
func (r *Runtime) ExecuteStream(ctx context.Context, session *Session, tools []tool.Tool, maxIterations int, cb StreamCallback, handler ToolEventHandler) (string, error) {
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
				// 输出思考内容
				if handler != nil && resp.Content != "" {
					handler(ToolEvent{
						Type:     "thinking",
						Thinking: resp.Content,
					})
				}
				// 需要工具调用，走阻塞路径
				assistantMsg := ChatMessage{
					Role:      "assistant",
					Content:   resp.Content,
					ToolCalls: resp.ToolCalls,
				}
				messages = append(messages, assistantMsg)

				for _, tc := range resp.ToolCalls {
					if handler != nil {
						handler(ToolEvent{
							Type:     "calling",
							ToolName: tc.Function.Name,
							Args:     tc.Function.Arguments,
						})
					}

					result, execErr := r.executeTool(ctx, tc, tools)
					if execErr != nil {
						result = fmt.Sprintf("工具执行错误: %v", execErr)
						if handler != nil {
							handler(ToolEvent{
								Type:     "error",
								ToolName: tc.Function.Name,
								Error:    execErr.Error(),
							})
						}
					} else if handler != nil {
						handler(ToolEvent{
							Type:     "result",
							ToolName: tc.Function.Name,
							Result:   result,
						})
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
	logger := glog.Logger()
	logger.Debug("[Runtime] callLLM", "provider", r.config.ProviderType, "model", r.config.Model, "messages", len(messages))
	switch r.config.ProviderType {
	case "ollama":
		return r.callOllama(ctx, messages, tools)
	default:
		return r.callOpenAI(ctx, messages, tools)
	}
}

// callOpenAI OpenAI兼容API调用
func (r *Runtime) callOpenAI(ctx context.Context, messages []ChatMessage, tools []tool.Tool) (*ChatMessage, error) {
	logger := glog.Logger()
	reqBody := r.buildOpenAIRequest(messages, tools)
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("[Runtime] 构建请求JSON失败", "err", err)
		return nil, err
	}

	url := r.config.BaseURL + "/chat/completions"
	logger.Debug("[Runtime] 发送OpenAI请求", "url", url, "model", r.config.Model, "request_len", len(jsonData))

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Error("[Runtime] 创建HTTP请求失败", "url", url, "err", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if r.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.config.APIKey)
	}

	startTime := time.Now()
	resp, err := r.client.Do(req)
	if err != nil {
		logger.Error("[Runtime] HTTP请求失败", "url", url, "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	elapsed := time.Since(startTime)
	logger.Debug("[Runtime] HTTP响应收到", "status", resp.StatusCode, "elapsed_ms", elapsed.Milliseconds())

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("[Runtime] 读取响应体失败", "err", err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		logger.Error("[Runtime] API返回非200状态码",
			"status", resp.StatusCode,
			"body", truncate(string(body), 500))
		return nil, fmt.Errorf("API返回状态码 %d: %s", resp.StatusCode, truncate(string(body), 300))
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
		logger.Error("[Runtime] 解析响应JSON失败", "err", err, "body_len", len(body))
		return nil, err
	}

	if len(llmResp.Choices) == 0 {
		logger.Error("[Runtime] LLM返回空choices", "body", truncate(string(body), 500))
		return nil, fmt.Errorf("no response from LLM: %s", truncate(string(body), 200))
	}

	msg := &ChatMessage{
		Role:      llmResp.Choices[0].Message.Role,
		Content:   llmResp.Choices[0].Message.Content,
		ToolCalls: llmResp.Choices[0].Message.ToolCalls,
	}

	logger.Debug("[Runtime] OpenAI响应解析成功",
		"content_len", len(msg.Content),
		"tool_calls", len(msg.ToolCalls),
		"elapsed_ms", elapsed.Milliseconds())

	return msg, nil
}

// callOllama Ollama API调用
func (r *Runtime) callOllama(ctx context.Context, messages []ChatMessage, tools []tool.Tool) (*ChatMessage, error) {
	logger := glog.Logger()
	reqBody := r.buildOllamaRequest(messages, tools)
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("[Runtime] 构建Ollama请求JSON失败", "err", err)
		return nil, err
	}

	url := r.config.BaseURL + "/api/chat"
	logger.Debug("[Runtime] 发送Ollama请求", "url", url, "model", r.config.Model, "request_len", len(jsonData))

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Error("[Runtime] 创建Ollama HTTP请求失败", "url", url, "err", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	resp, err := r.client.Do(req)
	if err != nil {
		logger.Error("[Runtime] Ollama HTTP请求失败", "url", url, "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	elapsed := time.Since(startTime)
	logger.Debug("[Runtime] Ollama响应收到", "status", resp.StatusCode, "elapsed_ms", elapsed.Milliseconds())

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("[Runtime] Ollama返回非200状态码", "status", resp.StatusCode, "body", truncate(string(body), 500))
		return nil, fmt.Errorf("Ollama返回状态码 %d", resp.StatusCode)
	}

	// Ollama 返回的是流式JSON行，每行一个JSON对象
	var lastLine struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Done bool `json:"done"`
	}

	lineCount := 0
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		if err := json.Unmarshal([]byte(line), &lastLine); err != nil {
			logger.Debug("[Runtime] Ollama行解析跳过", "line", lineCount, "err", err)
			continue
		}
		if lastLine.Done {
			break
		}
	}

	logger.Debug("[Runtime] Ollama响应解析完成", "lines_read", lineCount, "elapsed_ms", elapsed.Milliseconds())

	if lastLine.Message.Content == "" {
		logger.Error("[Runtime] Ollama返回空内容")
		return nil, fmt.Errorf("no response from Ollama")
	}

	msg := &ChatMessage{
		Role:    lastLine.Message.Role,
		Content: lastLine.Message.Content,
	}

	logger.Info("[Runtime] Ollama响应成功",
		"content_len", len(msg.Content),
		"elapsed_ms", elapsed.Milliseconds())

	return msg, nil
}

func (r *Runtime) buildOllamaRequest(messages []ChatMessage, tools []tool.Tool) map[string]interface{} {
	// 转换消息格式为 Ollama 格式
	var ollamaMessages []map[string]interface{}
	for _, m := range messages {
		ollamaMessages = append(ollamaMessages, map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	reqBody := map[string]interface{}{
		"model":    r.config.Model,
		"messages": ollamaMessages,
		"stream":   false,
		"options": map[string]interface{}{
			"temperature": 0.7,
			"num_ctx":     4096,
		},
	}

	// Ollama 工具支持（如果模型支持）
	if len(tools) > 0 {
		reqBody["tools"] = r.convertOllamaTools(tools)
	}

	return reqBody
}

// convertOllamaTools 转换工具为 Ollama 格式
func (r *Runtime) convertOllamaTools(tools []tool.Tool) []map[string]interface{} {
	var ollamaTools []map[string]interface{}
	for _, t := range tools {
		ollamaTools = append(ollamaTools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.Parameters(),
			},
		})
	}
	return ollamaTools
}

// buildOpenAIRequest 构建 OpenAI 请求体
func (r *Runtime) buildOpenAIRequest(messages []ChatMessage, tools []tool.Tool) map[string]interface{} {
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

// callLLMStream 调用大模型API（流式版）
func (r *Runtime) callLLMStream(ctx context.Context, messages []ChatMessage, cb StreamCallback) (strings.Builder, []ToolCall, error) {
	logger := glog.Logger()
	reqBody := r.buildOpenAIRequest(messages, nil)
	reqBody["stream"] = true
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return strings.Builder{}, nil, err
	}

	url := r.config.BaseURL + "/chat/completions"
	logger.Debug("[Runtime] 发送SSE流式请求", "url", url, "model", r.config.Model)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Error("[Runtime] 创建流式HTTP请求失败", "err", err)
		return strings.Builder{}, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if r.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.config.APIKey)
	}
	req.Header.Set("Accept", "text/event-stream")

	startTime := time.Now()
	resp, err := r.client.Do(req)
	if err != nil {
		logger.Error("[Runtime] 流式HTTP请求失败", "err", err)
		return strings.Builder{}, nil, err
	}
	defer resp.Body.Close()

	logger.Debug("[Runtime] SSE连接建立", "status", resp.StatusCode)

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

	logger.Info("[Runtime] SSE流式响应完成",
		"content_len", fullContent.Len(),
		"elapsed_ms", time.Since(startTime).Milliseconds())

	return fullContent, nil, nil
}

// executeTool 执行工具
func (r *Runtime) executeTool(ctx context.Context, tc ToolCall, tools []tool.Tool) (string, error) {
	logger := glog.Logger()
	logger.Debug("[Runtime] 开始执行工具",
		"tool_name", tc.Function.Name,
		"tool_call_id", tc.ID)

	var targetTool tool.Tool
	for _, t := range tools {
		if t.Name() == tc.Function.Name {
			targetTool = t
			break
		}
	}

	if targetTool == nil {
		logger.Error("[Runtime] 工具未注册", "tool_name", tc.Function.Name)
		return "", fmt.Errorf("tool not found: %s", tc.Function.Name)
	}

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
		logger.Error("[Runtime] 工具参数解析失败",
			"tool_name", tc.Function.Name,
			"args", tc.Function.Arguments,
			"err", err)
		return "", err
	}

	logger.Debug("[Runtime] 工具参数解析成功",
		"tool_name", tc.Function.Name,
		"params_count", len(params))

	startTime := time.Now()
	result, err := targetTool.Execute(ctx, params)
	elapsed := time.Since(startTime)

	if err != nil {
		logger.Error("[Runtime] 工具执行失败",
			"tool_name", tc.Function.Name,
			"elapsed_ms", elapsed.Milliseconds(),
			"err", err)
		return "", err
	}

	logger.Info("[Runtime] 工具执行完成",
		"tool_name", tc.Function.Name,
		"result_len", len(result),
		"elapsed_ms", elapsed.Milliseconds())

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

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
