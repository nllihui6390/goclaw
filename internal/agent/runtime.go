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
	"os"
	"strings"
	"time"
)

// Runtime Agent运行时（处理LLM调用和工具执行）
type Runtime struct {
	config       *Config
	client       *http.Client
	workspaceDir string // 用于缓存大工具结果
}

// NewRuntime 创建运行时
func NewRuntime(cfg *Config) *Runtime {
	return &Runtime{
		config:       cfg,
		client:       &http.Client{Timeout: 60 * time.Second},
	}
}

// SetWorkspaceDir 设置工作空间目录（用于缓存文件）
func (r *Runtime) SetWorkspaceDir(dir string) {
	r.workspaceDir = dir
}

// ChatMessage 对话消息
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"` // tool 角色消息的工具名称
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

// stripThinkTags 剥离 DeepSeek 等模型的内部推理标签
// DeepSeek 使用 <think>...</think> 或 ellites 标签做内部推理
func stripThinkTags(content string) string {
	// 剥离 <think>...</think>
	result := content
	for {
		start := strings.Index(result, "<think>")
		if start == -1 {
			break
		}
		end := strings.Index(result, "</think>")
		if end == -1 || end <= start {
			break
		}
		result = result[:start] + result[end+8:]
	}

	return strings.TrimSpace(result)
}

// suggestsToolUse 检测响应内容是否暗示要使用工具但没实际调用
// 注意：先剥离内部推理标签，只检测用户可见的实际响应内容
func suggestsToolUse(content string) bool {
	// 先剥离 DeepSeek 的内部推理标签
	visibleContent := stripThinkTags(content)
	if visibleContent == "" {
		// 剥离后内容为空，说明模型只做了内部推理，没有实际响应
		// 这种情况不应触发 auto-continue，而是让模型自然返回
		return false
	}
	hints := []string{"我来", "我将", "让我", "我可以使用", "我会调用", "我将使用", "我来查询", "我来执行"}
	for _, h := range hints {
		if strings.Contains(visibleContent, h) {
			return true
		}
	}
	return false
}

// isRetryableError 检查错误是否可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection reset")
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
	messages := r.buildMessages(session, tools)
	logger.Info("[Runtime] 开始执行循环",
		"model", r.config.Model,
		"provider", r.config.ProviderType,
		"messages_count", len(messages),
		"tools_count", len(tools),
		"max_iterations", maxIterations)

	// 跟踪失败情况，用于智能退出
	consecutiveFailures := 0
	maxConsecutiveFailures := 3
	lastFailedTool := ""
	totalFailures := 0
	totalSuccess := 0
	// auto-continue 跟踪
	autoContinueCount := 0
	maxAutoContinue := 3
	// summarizing 标记
	summarizing := false

	for i := 0; ; i++ {
		// 安全上限：防止无限循环（但远高于正常需求）
		if i >= 100 {
			logger.Warn("[Runtime] 达到安全上限100次迭代，强制退出")
			return "已达到最大处理次数，请简化您的请求或稍后重试。", nil
		}

		// 用户配置的软上限：触发 summarizing 模式
		if maxIterations > 0 && i >= maxIterations && !summarizing {
			logger.Warn("[Runtime] 达到配置的迭代上限，请求模型总结", "max_iterations", maxIterations)
			summarizing = true
			// 强制模型返回纯文本总结（不传 tools = tool_choice none）
			summaryResp, err := r.callLLMWithRetry(ctx, messages, nil)
			if err != nil {
				logger.Error("[Runtime] 总结调用失败", "err", err)
				return "已完成处理，但总结时出错。", nil
			}
			return summaryResp.Content, nil
		}

		logger.Info("[Runtime] 迭代开始", "iteration", i+1, "messages_count", len(messages))

		// 每次迭代开始时通知用户正在思考
		if handler != nil {
			handler(ToolEvent{
				Type:     "thinking",
				Thinking: "正在思考...",
			})
		}

		resp, err := r.callLLMWithRetry(ctx, messages, tools)
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
			// Auto-continue: 检测是否暗示要使用工具但没实际调用
			if suggestsToolUse(resp.Content) && autoContinueCount < maxAutoContinue && len(tools) > 0 {
				autoContinueCount++
				logger.Info("[Runtime] 检测到暗示工具使用，注入提示继续",
					"auto_continue_count", autoContinueCount,
					"content_preview", truncate(resp.Content, 100))
				messages = append(messages, ChatMessage{
					Role:    "assistant",
					Content: resp.Content,
				})
				messages = append(messages, ChatMessage{
					Role:    "user",
					Content: "请直接调用工具来完成任务，不要只是描述你打算做什么。",
				})
				continue
			}

			// 剥离内部推理标签后：如果可见内容为空，注入提示让模型输出实际回答
			visibleContent := stripThinkTags(resp.Content)
			if visibleContent == "" && autoContinueCount < maxAutoContinue {
				autoContinueCount++
				logger.Info("[Runtime] 模型只返回内部推理（无可见内容），注入提示",
					"auto_continue_count", autoContinueCount)
				messages = append(messages, ChatMessage{
					Role:    "assistant",
					Content: resp.Content,
				})
				messages = append(messages, ChatMessage{
					Role:    "user",
					Content: "请直接给出你的回答。",
				})
				continue
			}

			// 返回前剥离内部推理标签，用户不需要思考内容
			finalContent := stripThinkTags(resp.Content)
			if finalContent == "" {
				finalContent = resp.Content
			}
			logger.Info("[Runtime] 无工具调用，返回最终响应", "iteration", i+1, "visible_len", len(finalContent))
			return finalContent, nil
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

				// 检查是否是重复工具失败
				totalFailures++
				if tc.Function.Name == lastFailedTool {
					consecutiveFailures++
					if consecutiveFailures >= maxConsecutiveFailures {
						logger.Warn("[Runtime] 同一工具连续失败，智能退出",
							"tool", tc.Function.Name,
							"consecutive_failures", consecutiveFailures)
						return fmt.Sprintf("抱歉，工具 %s 连续失败%d次：%s", tc.Function.Name, consecutiveFailures, err.Error()), nil
					}
				} else {
					consecutiveFailures = 1
					lastFailedTool = tc.Function.Name
				}

				// 失败次数远超成功次数时退出
				if totalFailures >= 8 && totalFailures > totalSuccess*3 {
					logger.Warn("[Runtime] 失败次数远超成功次数，智能退出",
						"total_failures", totalFailures,
						"total_success", totalSuccess)
					return "抱歉，多次尝试后仍无法完成您的请求。", nil
				}
			} else {
				consecutiveFailures = 0
				lastFailedTool = ""
				totalSuccess++
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

	// unreachable: loop has internal returns
}

// ExecuteStream 执行Agent循环（流式版）
func (r *Runtime) ExecuteStream(ctx context.Context, session *Session, tools []tool.Tool, maxIterations int, cb StreamCallback, handler ToolEventHandler) (string, error) {
	messages := r.buildMessages(session, tools)

	for i := 0; i < maxIterations; i++ {
		var fullContent strings.Builder
		var toolCalls []ToolCall
		var err error

		// 如果有工具，先用非流式调用判断是否需要工具调用
		if len(tools) > 0 {
			resp, err := r.callLLMWithRetry(ctx, messages, tools)
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

	return "", nil
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

// callLLMWithRetry 带重试的LLM调用（429/5xx自动重试 + 指数退避）
func (r *Runtime) callLLMWithRetry(ctx context.Context, messages []ChatMessage, tools []tool.Tool) (*ChatMessage, error) {
	logger := glog.Logger()
	maxRetries := 3
	baseDelay := 1 * time.Second
	maxDelay := 30 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := r.callLLM(ctx, messages, tools)
		if err == nil {
			return resp, nil
		}

		if !isRetryableError(err) || attempt == maxRetries {
			return nil, err
		}

		delay := baseDelay * time.Duration(1<<uint(attempt))
		if delay > maxDelay {
			delay = maxDelay
		}
		logger.Warn("[Runtime] API调用失败，准备重试",
			"attempt", attempt+1, "max", maxRetries, "delay", delay, "err", err)
		time.Sleep(delay)
	}
	return nil, fmt.Errorf("重试次数耗尽")
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
	logger.Info("[Runtime] 发送OpenAI请求", "url", url, "model", r.config.Model, "request_len", len(jsonData))
	logger.Debug("[Runtime] 请求体", "body", string(jsonData))

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("[Runtime] 读取响应体失败", "err", err)
		return nil, err
	}

	logger.Info("[Runtime] HTTP响应收到", "status", resp.StatusCode, "elapsed_ms", elapsed.Milliseconds())
	logger.Debug("[Runtime] 响应体原始", "body", string(body))

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

	logger.Info("[Runtime] OpenAI响应解析",
		"content_len", len(msg.Content),
		"tool_calls_count", len(msg.ToolCalls),
		"content_preview", truncate(msg.Content, 200),
		"elapsed_ms", elapsed.Milliseconds())

	if len(msg.ToolCalls) > 0 {
		for i, tc := range msg.ToolCalls {
			logger.Info("[Runtime] tool_call详情",
				"idx", i+1,
				"id", tc.ID,
				"name", tc.Function.Name,
				"args_preview", truncate(tc.Function.Arguments, 200))
		}
	}

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

// executeTool 执行工具（带结果裁剪）
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

	// 工具结果裁剪：超长结果截断并保存到缓存文件
	maxBytes := r.config.ToolResultMaxBytes
	if maxBytes == 0 {
		maxBytes = 20000 // 默认 20KB
	}
	if len(result) > maxBytes {
		result = r.pruneToolResult(tc.Function.Name, result, maxBytes)
	}

	return result, nil
}

// pruneToolResult 裁剪工具结果：截断 + 缓存全文
func (r *Runtime) pruneToolResult(toolName, result string, maxBytes int) string {
	logger := glog.Logger()

	if r.workspaceDir == "" {
		logger.Warn("[Runtime] 无工作空间目录，仅截断结果不缓存", "tool", toolName)
		return result[:maxBytes] + "\n\n[结果已被截断，原长度: " + fmt.Sprintf("%d", len(result)) + " 字符]"
	}

	// 保存全文到缓存文件
	cacheDir := r.workspaceDir + "/cache"
	os.MkdirAll(cacheDir, 0755)

	// 生成唯一文件名
	cacheID := fmt.Sprintf("%s_%d", toolName, time.Now().UnixNano())
	cachePath := cacheDir + "/" + cacheID + ".txt"

	if err := os.WriteFile(cachePath, []byte(result), 0644); err != nil {
		logger.Warn("[Runtime] 缓存工具结果失败", "tool", toolName, "err", err)
		return result[:maxBytes] + "\n\n[结果已被截断，原长度: " + fmt.Sprintf("%d", len(result)) + " 字符]"
	}

	logger.Info("[Runtime] 工具结果已裁剪并缓存",
		"tool", toolName,
		"original_len", len(result),
		"truncated_len", maxBytes,
		"cache_file", cachePath)

	return result[:maxBytes] + "\n\n[结果已被截断，原长度: " + fmt.Sprintf("%d", len(result)) + " 字符。完整结果已保存至: " + cachePath + "]"
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

// buildMessages 构建消息列表（带上下文压缩）
func (r *Runtime) buildMessages(session *Session, tools []tool.Tool) []ChatMessage {
	logger := glog.Logger()
	systemContent := r.config.SystemPrompt

	// 加载工作空间人设文件（AGENTS.md + SOUL.md + PROFILE.md）
	if r.config.WorkspaceLoader != nil {
		personality := r.config.WorkspaceLoader.LoadSystemPrompt()
		if personality != "" {
			systemContent = personality + "\n\n" + systemContent
			logger.Debug("[Runtime] 人设文件已注入到 system prompt", "len", len(personality))
		}
	}

	// 多模态能力提示
	if !r.config.SupportsImage {
		systemContent += "\n\n注意：当前模型不支持图片输入，请勿尝试解析图片内容。"
	}
	if !r.config.SupportsVideo {
		systemContent += "\n\n注意：当前模型不支持视频输入，请勿尝试解析视频内容。"
	}

	// 首次引导：检测 BOOTSTRAP.md
	if r.config.WorkspaceLoader != nil && r.config.WorkspaceLoader.IsBootstrapNeeded() {
		if len(session.Messages) > 0 {
			lastMsg := session.Messages[len(session.Messages)-1]
			if lastMsg.Role == "user" {
				systemContent += "\n\n" + r.config.WorkspaceLoader.GetBootstrapGuidance()
				logger.Info("[Runtime] 首次引导模式，注入 BOOTSTRAP 指导")
				r.config.WorkspaceLoader.MarkBootstrapCompleted()
			}
		}
	}

	// 检查用户是否要创建技能，动态注入技能创建模板
	if len(session.Messages) > 0 {
		lastMsg := session.Messages[len(session.Messages)-1]
		if lastMsg.Role == "user" && wantsCreateSkill(lastMsg.Content) {
			systemContent += "\n\n" + getSkillCreationTemplate()
			logger.Info("[Runtime] 检测到技能创建意图，注入模板")
		}
	}

	// 如果有工具，则添加到系统提示中
	if len(tools) > 0 {
		systemContent += "\n\n## 可用工具\n你必须通过调用工具来完成用户的请求，不要直接猜测或仅描述打算使用什么工具。\n"
		for _, t := range tools {
			systemContent += fmt.Sprintf("- **%s**: %s\n", t.Name(), t.Description())
		}
		systemContent += "\n重要：当用户提出需要查询天气、执行命令、读写文件等具体请求时，你必须实际调用对应的工具（通过tool_calls），而不是仅在文本中说明你打算使用工具。"
	}

	// 基础 system message
	messages := []ChatMessage{
		{Role: "system", Content: systemContent},
	}

	// 如果有压缩摘要，注入到系统提示后面
	if session.CompressedSummary != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: "以下是之前对话的压缩摘要，请参考此上下文继续回答：\n\n" + session.CompressedSummary,
		})
		logger.Debug("[Runtime] 压缩摘要已注入", "len", len(session.CompressedSummary))
	}

	// 包含所有角色的消息（user, assistant, tool）
	for _, msg := range session.Messages {
		chatMsg := ChatMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		if msg.Role == "tool" && msg.Name != "" {
			chatMsg.Name = msg.Name
		}
		messages = append(messages, chatMsg)
	}

	// Token 预算管理
	maxContextTokens := 32000
	if r.config.MaxTokens > 0 {
		maxContextTokens = r.config.MaxTokens
	}
	maxMessages := maxContextTokens / 5000

	// 上下文压缩：接近阈值时压缩旧消息
	compactRatio := r.config.CompactThresholdRatio
	if compactRatio == 0 {
		compactRatio = 0.8
	}
	reserveRatio := r.config.ReserveThresholdRatio
	if reserveRatio == 0 {
		reserveRatio = 0.15
	}

	compactThreshold := int(float64(maxMessages) * compactRatio)
	reserveCount := int(float64(maxMessages) * reserveRatio)

	if len(messages) > compactThreshold && session.CompressedSummary == "" && reserveCount > 0 {
		logger.Info("[Runtime] 触发上下文压缩",
			"current_messages", len(messages),
			"compact_threshold", compactThreshold,
			"reserve_count", reserveCount)

		oldMsgs := messages[1:len(messages)-reserveCount]
		if len(oldMsgs) > 2 {
			summary := r.compressMessages(oldMsgs)
			if summary != "" {
				session.CompressedSummary = summary
				session.mu.Lock()
				session.persistLocked()
				session.mu.Unlock()

				recentMsgs := messages[len(messages)-reserveCount:]
				messages = []ChatMessage{
					{Role: "system", Content: systemContent},
					{Role: "system", Content: "以下是之前对话的压缩摘要：\n\n" + summary},
				}
				messages = append(messages, recentMsgs...)
				logger.Info("[Runtime] 上下文压缩完成",
					"original_count", len(session.Messages)+1,
					"compressed_count", len(messages),
					"summary_len", len(summary))
			}
		}
	} else if len(messages) > maxMessages {
		systemMsg := messages[0]
		recentMsgs := messages[len(messages)-maxMessages+1:]
		messages = append([]ChatMessage{systemMsg}, recentMsgs...)
		logger.Info("[Runtime] 上下文已截断", "original", len(session.Messages)+1, "kept", len(messages))
	}

	return messages
}

// compressMessages 调用 LLM 压缩旧消息
func (r *Runtime) compressMessages(messages []ChatMessage) string {
	logger := glog.Logger()

	summaryPrompt := `请将以下对话历史压缩为简洁的摘要，保留关键信息：
- 用户的主要问题和请求
- AI 的关键回答和操作
- 重要的决策和结论
- 未完成或待办的事项

只输出摘要内容，不要添加其他说明。`

	summaryMessages := []ChatMessage{
		{Role: "system", Content: summaryPrompt},
	}
	summaryMessages = append(summaryMessages, messages...)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := r.callLLMWithRetry(ctx, summaryMessages, nil)
	if err != nil {
		logger.Warn("[Runtime] 压缩摘要失败", "err", err)
		return ""
	}

	logger.Info("[Runtime] 压缩摘要生成成功", "len", len(resp.Content))
	return resp.Content
}

// ExtractMemory 调用 LLM 提取对话中的关键信息
func (r *Runtime) ExtractMemory(ctx context.Context, userMsg, assistantMsg string) string {
	logger := glog.Logger()

	extractPrompt := `请从以下对话中提取值得长期记住的关键信息。
只提取以下类型的内容：
1. 用户表达的重要偏好、习惯、身份信息
2. 关键决策或结论
3. 重要的事实或数据
4. 未完成但重要的待办事项

如果对话中没有值得记住的信息，返回空字符串。
每条信息用简短的一句话描述，不要添加解释。格式：
- 信息1
- 信息2

对话内容：`

	messages := []ChatMessage{
		{Role: "system", Content: extractPrompt},
		{Role: "user", Content: userMsg},
		{Role: "assistant", Content: assistantMsg},
	}

	resp, err := r.callLLMWithRetry(ctx, messages, nil)
	if err != nil {
		logger.Warn("[Runtime] 记忆提取失败", "err", err)
		return ""
	}

	// 如果提取结果为空或无意义，不存储
	content := strings.TrimSpace(resp.Content)
	if content == "" || content == "无" || content == "没有值得记住的信息" {
		return ""
	}

	logger.Info("[Runtime] 记忆提取完成", "len", len(content))
	return content
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
