package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go-claw/internal/channel"
	"go-claw/internal/security"
	"go-claw/internal/skill"
	"go-claw/internal/tool"
	glog "go-claw/pkg/log"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
		config: cfg,
		client: &http.Client{Timeout: 180 * time.Second},
	}
}

// SetWorkspaceDir 设置工作空间目录（用于缓存文件）
func (r *Runtime) SetWorkspaceDir(dir string) {
	r.workspaceDir = dir
}

// SetSkillRegistry 设置技能注册中心（用于热重载）
func (r *Runtime) SetSkillRegistry(reg *skill.Registry) {
	r.config.SkillRegistry = reg
}

// runtimeConfig 运行时配置（model、apiKey、baseURL、providerType）
type runtimeConfig struct {
	Model        string
	APIKey       string
	BaseURL      string
	ProviderType string
}

// getRuntimeConfig 获取当前运行时配置（优先从 ConfigProvider 动态获取，降级使用静态值）
func (r *Runtime) getRuntimeConfig() *runtimeConfig {
	if r.config.ConfigProvider != nil {
		model, apiKey, baseURL, providerType := r.config.ConfigProvider()
		return &runtimeConfig{
			Model:        model,
			APIKey:       apiKey,
			BaseURL:      baseURL,
			ProviderType: providerType,
		}
	}
	return &runtimeConfig{
		Model:        r.config.Model,
		APIKey:       r.config.APIKey,
		BaseURL:      r.config.BaseURL,
		ProviderType: r.config.ProviderType,
	}
}

// ChatMessage 对话消息
type ChatMessage struct {
	Role             string                `json:"role"`
	Content          string                `json:"content"`
	Blocks           channel.ContentBlocks `json:"-"` // 结构化内容块（多模态：图片/文件等）
	ToolCalls        []ToolCall            `json:"tool_calls,omitempty"`
	ToolCallID       string                `json:"tool_call_id,omitempty"`
	Name             string                `json:"name,omitempty"` // tool 角色消息的工具名称
	FinishReason     string                `json:"-"`              // LLM 返回的 finish_reason
	ReasoningContent string                `json:"-"`              // DeepSeek 等模型的 reasoning_content（推理/思考过程）
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
	result := content
	for {
		start := strings.Index(result, "<think>")
		if start == -1 {
			break
		}
		sub := result[start:]
		end := strings.Index(sub, "</think>")
		if end == -1 {
			break
		}
		result = result[:start] + sub[end+len("</think>"):]
	}
	return strings.TrimSpace(result)
}

// extractThinkContent 提取 <think> 标签内的思考内容（保留用于流式显示）
func extractThinkContent(content string) string {
	start := strings.Index(content, "<think>")
	if start == -1 {
		return ""
	}
	start += 7 // len("<think>")
	end := strings.Index(content[start:], "</think>")
	if end == -1 {
		return content[start:]
	}
	return content[start : start+end]
}

func shouldForceContinue(resp *ChatMessage) bool {
	if len(resp.ToolCalls) > 0 {
		return false
	}

	visible := strings.TrimSpace(stripThinkTags(resp.Content))

	// 如果 content 为空但 reasoning_content 已有充分推理内容，说明模型已完成推理，
	// 不应强制继续（DeepSeek 等模型会把推理放在 reasoning_content 中）
	if visible == "" && len([]rune(resp.ReasoningContent)) >= 100 {
		return false
	}

	if resp.FinishReason == "length" {
		return true
	}

	if visible == "" {
		return true
	}

	runes := len([]rune(visible))
	if runes <= 3 {
		return true
	}

	// 可见回复已较完整，视为最终答案
	if runes >= 150 {
		return false
	}

	// 仅对短回复中明确的「将要行动」措辞轻推，避免技能说明等正文误触发
	intentPhrases := []string{
		"让我来", "让我先", "我需要先", "接下来我会", "接下来我将",
		"我现在来", "我现在先", "我先来", "我将要", "我会先",
	}
	for _, phrase := range intentPhrases {
		if strings.Contains(visible, phrase) {
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
	Type     string // "calling", "result", "error", "thinking", "content"
	ToolName string
	Args     string
	Result   string
	Error    string
	Thinking string
	Content  channel.ContentBlocks // 结构化内容块（用于 StructuredTool 返回结果）
}

// Execute 执行Agent循环（阻塞版）
func (r *Runtime) Execute(ctx context.Context, session *Session, tools []tool.Tool, maxIterations int, handler ToolEventHandler) (string, error) {
	return r.ExecuteWithEnhancedMessage(ctx, session, "", tools, maxIterations, handler)
}

// ExecuteWithEnhancedMessage 执行Agent循环，支持传入增强后的最后一条用户消息
// enhancedMessage 如果不为空，将替换会话中最后一条 user 消息的 content，仅用于本次 LLM 调用
func (r *Runtime) ExecuteWithEnhancedMessage(ctx context.Context, session *Session, enhancedMessage string, tools []tool.Tool, maxIterations int, handler ToolEventHandler) (string, error) {
	logger := glog.Logger()

	// 如果提供了增强消息，临时替换会话中最后一条 user 消息的 content
	var originalContent channel.ContentBlocks
	var enhanced bool
	if enhancedMessage != "" && len(session.Messages) > 0 {
		lastIdx := len(session.Messages) - 1
		if session.Messages[lastIdx].Role == "user" {
			originalContent = session.Messages[lastIdx].Content
			session.Messages[lastIdx].Content = channel.ContentBlocksFromText(enhancedMessage)
			enhanced = true
			logger.Debug("[Runtime] 已替换最后一条 user 消息为增强版本", "original_len", len(channel.TextOnlyContent(originalContent)), "enhanced_len", len(enhancedMessage))
		}
	}
	// 构建消息列表（包含工具调用结果）

	messages := r.buildMessages(session, tools)

	// 恢复原始内容（确保后续持久化时保存的是原始消息）
	if enhanced && len(session.Messages) > 0 {
		lastIdx := len(session.Messages) - 1
		if session.Messages[lastIdx].Role == "user" {
			session.Messages[lastIdx].Content = originalContent
		}
	}

	logger.Info("[Runtime] 开始执行循环",
		"model", r.getRuntimeConfig().Model,
		"provider", r.getRuntimeConfig().ProviderType,
		"messages_count", len(messages),
		"tools_count", len(tools),
		"max_iterations", maxIterations)

	// 跟踪失败情况，用于智能退出
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
			return stripThinkTags(summaryResp.Content), nil
		}

		logger.Info("[Runtime] 迭代开始", "iteration", i+1, "messages_count", len(messages))

		// 每次迭代开始时通知用户正在思考
		if handler != nil {
			handler(ToolEvent{
				Type:     "thinking",
				Thinking: "正在思考...",
			})
		}
		// 调用 LLM 并处理工具调用结果<-主进程逻辑开始处
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
		if handler != nil && len(resp.ToolCalls) > 0 {
			thinkingContent := resp.ReasoningContent
			if thinkingContent == "" {
				thinkingContent = resp.Content
			}
			if thinkingContent != "" {
				handler(ToolEvent{
					Type:     "thinking",
					Thinking: thinkingContent,
				})
			}
		}

		if len(resp.ToolCalls) == 0 {
			visible := stripThinkTags(resp.Content)

			if shouldForceContinue(resp) && autoContinueCount < maxAutoContinue && len(tools) > 0 {
				// 强制继续，但不计入迭代次数
				autoContinueCount++
				logger.Info("[Runtime] force continue", "count", autoContinueCount)
				messages = append(messages, ChatMessage{
					Role:    "assistant",
					Content: resp.Content,
				})
				messages = append(messages, ChatMessage{
					Role:    "user",
					Content: "你刚才描述了后续步骤但未调用工具。若完成用户的上一条请求仍需工具，请直接调用；若已充分回答，请给出简洁的最终回复。",
				})
				continue
			}

			if visible == "" {
				visible = resp.Content
			}
			// DeepSeek 等模型：content 为空时，降级使用 reasoning_content 作为回复
			if visible == "" && resp.ReasoningContent != "" {
				visible = stripThinkTags(resp.ReasoningContent)
			}
			logger.Info("[Runtime] return final", "len", len(visible))
			return visible, nil
		}

		logger.Info("[Runtime] 检测到工具调用", "count", len(resp.ToolCalls))
		autoContinueCount = 0

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
			// 执行工具并捕获结果（带结果裁剪）
			result, contentBlocks, err := r.executeTool(ctx, tc, tools)
			if err != nil {
				result = fmt.Sprintf("工具执行错误: %v", err)
				logger.Error("[Runtime] 工具执行出错", "tool_name", tc.Function.Name, "err", err)
				if handler != nil {
					handler(ToolEvent{
						Type:     "error",
						ToolName: tc.Function.Name,
						Error:    err.Error(),
					})
				}

				// 记录失败次数
				totalFailures++

				// 失败次数远超成功次数时退出
				if totalFailures >= 8 && totalFailures > totalSuccess*3 {
					logger.Warn("[Runtime] 失败次数远超成功次数，智能退出", "total_failures", totalFailures, "total_success", totalSuccess)
					return "抱歉，多次尝试后仍无法完成您的请求。", nil
				}
			} else {
				totalSuccess++
				logger.Info("[Runtime] 工具执行成功", "tool_name", tc.Function.Name, "result_len", len(result))
				// 输出工具执行结果
				if handler != nil {
					handler(ToolEvent{
						Type:     "result",
						ToolName: tc.Function.Name,
						Result:   result,
					})
					// send_file 工具额外发送 file 事件（供前端实时渲染）
					if tc.Function.Name == "send_file" {
						var toolParams map[string]interface{}
						if json.Unmarshal([]byte(tc.Function.Arguments), &toolParams) == nil {
							path, _ := toolParams["path"].(string)
							filename, _ := toolParams["filename"].(string)
							if filename == "" && path != "" {
								filename = filepath.Base(path)
							}
							fileType := "file"
							if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
								fileType = "url"
							}
							handler(ToolEvent{
								Type:     "file",
								ToolName: filename,
								Args:     fileType,
								Result:   path,
							})
						}
					}
					// StructuredTool 返回的结构化内容块（用于 session 持久化）
					if len(contentBlocks) > 0 {
						handler(ToolEvent{
							Type:    "content",
							Content: contentBlocks,
						})
					}
				}
			}

			toolMsg := ChatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result + "\n\n如果任务尚未完成，请继续执行。",
			}
			messages = append(messages, toolMsg)
		}
	}

}

// unreachable: loop has internal returns

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
				// 输出思考内容（优先使用 reasoning_content，降级使用 content）
				if handler != nil {
					thinkingContent := resp.ReasoningContent
					if thinkingContent == "" {
						thinkingContent = resp.Content
					}
					if thinkingContent != "" {
						handler(ToolEvent{Type: "thinking", Thinking: thinkingContent})
					}
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

					result, contentBlocks, execErr := r.executeTool(ctx, tc, tools)
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
						// StructuredTool 返回的结构化内容块（用于 session 持久化）
						if len(contentBlocks) > 0 {
							handler(ToolEvent{
								Type:    "content",
								Content: contentBlocks,
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
	rtCfg := r.getRuntimeConfig()
	logger.Debug("[Runtime] callLLM", "provider", rtCfg.ProviderType, "model", rtCfg.Model, "messages", len(messages))
	if rtCfg.ProviderType == "ollama" {
		// Ollama 通过 OpenAI 兼容 API (/v1/chat/completions) 调用
		savedURL := rtCfg.BaseURL
		if !strings.HasSuffix(savedURL, "/v1") && !strings.HasSuffix(savedURL, "/v1/") {
			rtCfg.BaseURL = strings.TrimRight(savedURL, "/") + "/v1"
		}
	}
	return r.callOpenAIWithConfig(ctx, messages, tools, rtCfg)
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

// callOpenAIWithConfig OpenAI兼容API调用（使用动态获取的运行时配置）
func (r *Runtime) callOpenAIWithConfig(ctx context.Context, messages []ChatMessage, tools []tool.Tool, rtCfg *runtimeConfig) (*ChatMessage, error) {
	logger := glog.Logger()
	reqBody := r.buildOpenAIRequestWithConfig(messages, tools, rtCfg)
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("[Runtime] 构建请求JSON失败", "err", err)
		return nil, err
	}

	url := rtCfg.BaseURL + "/chat/completions"
	logger.Info("[Runtime] 发送OpenAI请求", "url", url, "model", rtCfg.Model, "request_len", len(jsonData))
	logger.Debug("[Runtime] 请求体", "body", string(jsonData))

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Error("[Runtime] 创建HTTP请求失败", "url", url, "err", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if rtCfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+rtCfg.APIKey)
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
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role             string     `json:"role"`
				Content          string     `json:"content"`
				ReasoningContent string     `json:"reasoning_content"` // DeepSeek 推理内容
				ToolCalls        []ToolCall `json:"tool_calls"`
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
		Role:             llmResp.Choices[0].Message.Role,
		Content:          llmResp.Choices[0].Message.Content,
		ReasoningContent: llmResp.Choices[0].Message.ReasoningContent,
		ToolCalls:        llmResp.Choices[0].Message.ToolCalls,
		FinishReason:     llmResp.Choices[0].FinishReason,
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

// buildOpenAIRequestWithConfig 构建 OpenAI 请求体（使用动态运行时配置）
func (r *Runtime) buildOpenAIRequestWithConfig(messages []ChatMessage, tools []tool.Tool, rtCfg *runtimeConfig) map[string]interface{} {
	// 转换消息格式：将带 Blocks 的消息转为 OpenAI vision 多模态格式
	apiMessages := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		apiMsg := map[string]interface{}{
			"role": msg.Role,
		}
		if msg.Name != "" {
			apiMsg["name"] = msg.Name
		}
		// 处理 content：有媒体块时转为多模态数组格式
		if len(msg.Blocks) > 0 {
			apiMsg["content"] = r.buildMessageContent(msg)
		} else {
			apiMsg["content"] = msg.Content
		}
		if len(msg.ToolCalls) > 0 {
			apiMsg["tool_calls"] = msg.ToolCalls
		}
		if msg.ToolCallID != "" {
			apiMsg["tool_call_id"] = msg.ToolCallID
		}
		apiMessages = append(apiMessages, apiMsg)
	}

	reqBody := map[string]interface{}{
		"model":       rtCfg.Model,
		"messages":    apiMessages,
		"temperature": 0.7,
	}

	if len(tools) > 0 {
		reqBody["tools"] = r.convertTools(tools)
		reqBody["tool_choice"] = "auto"
	}

	return reqBody
}

// buildMessageContent 将 ChatMessage 的 Blocks 转为 OpenAI vision 格式的 content 数组
func (r *Runtime) buildMessageContent(msg ChatMessage) interface{} {
	// 如果模型不支持图片，直接返回纯文本
	if !r.config.SupportsImage {
		return msg.Content
	}

	hasImage := false
	for _, block := range msg.Blocks {
		if block.Type() == channel.ContentTypeImage {
			hasImage = true
			break
		}
	}
	if !hasImage {
		return msg.Content
	}

	// 多模态图片只允许在 user 消息中，assistant/system/tool 消息忽略图片
	if msg.Role != "user" {
		logger := glog.Logger()
		logger.Debug("[Runtime] 非user角色的消息包含图片，跳过image block", "role", msg.Role)
		return msg.Content
	}

	var content []interface{}
	for _, block := range msg.Blocks {
		switch b := block.(type) {
		case *channel.ImageBlock:
			if b.Source.Type == "url" {
				imgURL := b.Source.URL
				// file:// URL → 读取本地文件并转为 base64 内联（云端 LLM 无法访问本地路径）
				localPath := channel.FileURLToLocalPath(imgURL)
				if localPath != imgURL {
					// 确实是 file:// 转换后的本地路径
					imgURL = r.fileToDataURL(localPath, b.Source.MediaType)
				}
				content = append(content, map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]interface{}{
						"url": imgURL,
					},
				})
			} else if b.Source.Type == "base64" {
				content = append(content, map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]interface{}{
						"url": "data:" + b.Source.MediaType + ";base64," + b.Source.Data,
					},
				})
			}
		case *channel.TextBlock:
			content = append(content, map[string]interface{}{
				"type": "text",
				"text": b.Text,
			})
		}
	}

	// 兜底：如果 blocks 里没有文本，但 msg.Content 有值，补上
	if len(content) == 0 || (msg.Content != "" && !hasTextBlock(content)) {
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": msg.Content,
		})
	}

	return content
}

// fileToDataURL 读取本地图片文件并返回 base64 data URI
// 如果读取失败或文件不存在，返回原始路径
func (r *Runtime) fileToDataURL(localPath, mediaType string) string {
	logger := glog.Logger()

	data, err := os.ReadFile(localPath)
	if err != nil {
		logger.Warn("[Runtime] 读取本地图片文件失败，使用原始URL", "path", localPath, "err", err)
		return "file://" + localPath
	}

	// 如果未提供 mediaType，尝试从文件扩展名推断
	if mediaType == "" {
		ext := strings.ToLower(filepath.Ext(localPath))
		switch ext {
		case ".png":
			mediaType = "image/png"
		case ".jpg", ".jpeg":
			mediaType = "image/jpeg"
		case ".gif":
			mediaType = "image/gif"
		case ".webp":
			mediaType = "image/webp"
		case ".bmp":
			mediaType = "image/bmp"
		default:
			mediaType = "application/octet-stream"
		}
	}

	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// hasTextBlock 检查 content 数组是否包含文本块
func hasTextBlock(content []interface{}) bool {
	for _, c := range content {
		if m, ok := c.(map[string]interface{}); ok && m["type"] == "text" {
			return true
		}
	}
	return false
}

// callLLMStream 调用大模型API（流式版）
func (r *Runtime) callLLMStream(ctx context.Context, messages []ChatMessage, cb StreamCallback) (strings.Builder, []ToolCall, error) {
	logger := glog.Logger()
	rtCfg := r.getRuntimeConfig()
	reqBody := r.buildOpenAIRequestWithConfig(messages, nil, rtCfg)
	reqBody["stream"] = true
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return strings.Builder{}, nil, err
	}

	url := rtCfg.BaseURL + "/chat/completions"
	logger.Debug("[Runtime] 发送SSE流式请求", "url", url, "model", rtCfg.Model)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Error("[Runtime] 创建流式HTTP请求失败", "err", err)
		return strings.Builder{}, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if rtCfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+rtCfg.APIKey)
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
func (r *Runtime) executeTool(ctx context.Context, tc ToolCall, tools []tool.Tool) (string, channel.ContentBlocks, error) {
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
		return "", nil, fmt.Errorf("tool not found: %s", tc.Function.Name)
	}

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
		logger.Error("[Runtime] 工具参数解析失败",
			"tool_name", tc.Function.Name,
			"args", tc.Function.Arguments,
			"err", err)
		return "", nil, err
	}

	logger.Debug("[Runtime] 工具参数解析成功",
		"tool_name", tc.Function.Name,
		"params_count", len(params))

	// 工具安全守卫检查
	if r.config.ToolGuard != nil {
		guardResult := r.config.ToolGuard.Check(ctx, tc.Function.Name, params)
		if guardResult.Decision == security.DecisionDeny {
			logger.Warn("[Runtime] 工具调用被安全守卫拒绝",
				"tool", tc.Function.Name, "reason", guardResult.Reason)
			return fmt.Sprintf("操作被拒绝: %s", guardResult.Message), nil, nil
		}
		if guardResult.Decision == security.DecisionGuard {
			// 需要用户确认，创建审批请求
			logger.Warn("[Runtime] 工具调用需要确认",
				"tool", tc.Function.Name, "reason", guardResult.Reason)

			approvalSvc := security.GetApprovalService()
			approvalID := fmt.Sprintf("approval_%s_%d", tc.Function.Name, time.Now().UnixNano())
			approval := approvalSvc.CreateApproval(
				approvalID,
				tc.Function.Name,
				params,
				guardResult.Reason,
				guardResult.Message,
			)

			// 构建通知消息（返回给 LLM，让 LLM 告知用户）
			notificationMsg := fmt.Sprintf(
				"⚠️ 安全守卫拦截了工具调用\n\n"+
					"工具: %s\n"+
					"原因: %s\n"+
					"审批ID: %s\n\n"+
					"请在安全设置页面审批，或使用 /approval approve %s 批准，或 /approval deny %s [原因] 拒绝",
				tc.Function.Name,
				guardResult.Message,
				approvalID,
				approvalID,
				approvalID,
			)

			// 等待用户审批（阻塞）
			result, err := approvalSvc.WaitForResult(ctx, approval)
			if err != nil {
				return fmt.Sprintf("%s\n\n审批等待失败: %v", notificationMsg, err), nil, nil
			}

			if !result.Approved {
				return fmt.Sprintf("%s\n\n操作被用户拒绝: %s", notificationMsg, result.DenyReason), nil, nil
			}

			// 用户批准了，继续执行工具
			logger.Info("[Runtime] 工具调用已获用户批准", "tool", tc.Function.Name, "approval_id", approvalID)
			_ = approval // 避免未使用警告
		}
	}

	startTime := time.Now()

	// 文件类工具（write_file, read_file, edit_file, append_file）自动拼接工作区目录
	// 只对简单相对路径生效，不拼接已包含完整路径的路径
	// if r.workspaceDir != "" {
	// 	switch tc.Function.Name {
	// 	case "write_file", "read_file", "edit_file", "append_file":
	// 		if path, ok := params["path"].(string); ok && path != "" {
	// 			// 只对简单相对路径拼接工作区目录
	// 			// 以下情况不拼接：绝对路径、已包含 clawdata/ 的路径
	// 			if !filepath.IsAbs(path) && !strings.Contains(path, "clawdata/") && !strings.Contains(path, "clawdata\\") {
	// 				params["path"] = filepath.Join(r.workspaceDir, path)
	// 			}
	// 		}
	// 	}
	// }

	// 优先使用 StructuredTool 接口
	var result string
	var contentBlocks channel.ContentBlocks
	var err error

	if st := tool.AsStructuredTool(targetTool); st != nil {
		contentBlocks, err = st.ExecuteStructured(ctx, params)
		if err == nil {
			result = channel.TextOnlyContent(contentBlocks)
		}
	} else {
		result, err = targetTool.Execute(ctx, params)
	}

	elapsed := time.Since(startTime)

	if err != nil {
		logger.Error("[Runtime] 工具执行失败",
			"tool_name", tc.Function.Name,
			"elapsed_ms", elapsed.Milliseconds(),
			"err", err)
		return "", nil, err
	}

	logger.Info("[Runtime] 工具执行完成",
		"tool_name", tc.Function.Name,
		"result_len", len(result),
		"content_blocks", len(contentBlocks),
		"elapsed_ms", elapsed.Milliseconds())

	// 工具结果裁剪：超长结果截断并保存到缓存文件
	if !r.isToolResultExempt(tc.Function.Name, result) {
		maxBytes := r.config.ToolResultMaxBytes
		if maxBytes == 0 {
			maxBytes = 20000 // 默认 20KB
		}
		if len(result) > maxBytes {
			result = r.pruneToolResult(tc.Function.Name, result, maxBytes)
		}
	}
	return result, contentBlocks, nil
}

// isToolResultExempt 检查工具结果是否豁免裁剪
func (r *Runtime) isToolResultExempt(toolName, result string) bool {
	for _, exempt := range r.config.ToolResultExemptTools {
		if exempt == toolName {
			return true
		}
	}
	for _, ext := range r.config.ToolResultExemptExts {
		if strings.Contains(result, ext) {
			return true
		}
	}
	return false
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

	// 注入技能提示词（Prompt-based 技能系统）
	if r.config.SkillRegistry != nil {
		if skillPrompt := r.config.SkillRegistry.GetSkillPrompt(); skillPrompt != "" {
			systemContent += "\n\n" + skillPrompt
			logger.Debug("[Runtime] 技能提示已注入到 system prompt")
		}
	}

	// 多模态能力提示
	if !r.config.SupportsImage {
		systemContent += "\n\n注意：当前模型不支持图片输入，请勿尝试解析图片内容。"
	}
	if !r.config.SupportsVideo {
		systemContent += "\n\n注意：当前模型不支持视频输入，请勿尝试解析视频内容。"
	}
	// 提示大模型工作区目录
	systemContent += "\n\n## 当前Agent工作区目录\n\n你的工作区目录是: " + r.workspaceDir + "，请使用这个目录进行文件读写操作。"
	systemContent += "\n ## 人设文件\n AGENTS.md、HEARTBEAT.md、MEMORY.md、PROFILE.md、SOUL.md都存放在当前Agent工作区目录下"

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
		if lastMsg.Role == "user" && wantsCreateSkill(channel.TextOnlyContent(lastMsg.Content)) {
			systemContent += "\n\n" + getSkillCreationTemplate()
			logger.Info("[Runtime] 检测到技能创建意图，注入模板")
		}
	}
	// 注入工具提示
	logger.Info("[Runtime] 开始构建工具加入到上下文", "len", len(tools))
	// 如果有工具，则添加到系统提示中
	if len(tools) > 0 {
		systemContent += "\n\n## 可用工具\n你必须通过调用工具来完成用户的请求，不要直接猜测或仅描述打算使用什么工具。\n"
		for _, t := range tools {
			logger.Debug("[Runtime] 可用工具已加载", "tool", t.Name(), "description", t.Description())
			systemContent += fmt.Sprintf("- **%s**: %s\n", t.Name(), t.Description())
		}
		systemContent += "\n重要：当用户提出需要查询天气、执行命令、读写文件等具体请求时，你必须实际调用对应的工具（通过tool_calls），而不是仅在文本中说明你打算使用工具。"
	}

	systemContent += `
	## Agent 执行约束（最高优先级）
	
	- 未完成任务时，不允许停留在计划阶段。
	- 不允许仅输出“我将…”“接下来…”“准备…”等描述而不执行。
	- 当前步骤存在可执行工具时，应直接执行，而不是继续分析。
	- 优先选择最小可行行动（Smallest Next Action），持续推进状态。
	- 只有以下情况允许停止执行：
	  1. 已输出最终结果
	  2. 缺少必要信息且无法推断
	  3. 所有可用工具均无法继续推进
	
	`

	// 基础 system message
	messages := []ChatMessage{
		{Role: "system", Content: systemContent},
	}

	// 注入当前会话信息（LLM 可据此自动设置定时任务的发送目标）
	sessionCtx := fmt.Sprintf("[会话信息]\n当前渠道: %s\n你的会话ID: %s\n（cron_status 的 session_id 参数使用此值可将结果发送到当前会话）", session.Channel, session.ID)
	messages = append(messages, ChatMessage{Role: "system", Content: sessionCtx})

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
			Content:    channel.TextOnlyContent(msg.Content),
			Blocks:     msg.Content, // 传递结构化内容块（多模态）
			ToolCallID: msg.ToolCallID,
		}
		if msg.Role == "tool" && msg.Name != "" {
			chatMsg.Name = msg.Name
		}
		messages = append(messages, chatMsg)
	}

	// Token 预算管理 100k token
	maxContextTokens := 100000
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

		oldMsgs := messages[1 : len(messages)-reserveCount]
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
		logger.Info("[Runtime] 上下文已截断", "original", len(session.Messages)+1, "kept", len(messages), "removed", len(session.Messages)+1-len(messages))
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
	return stripThinkTags(resp.Content)
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
