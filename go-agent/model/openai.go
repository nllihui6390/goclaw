package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIModel OpenAI 兼容模型实现
type OpenAIModel struct {
	config      ModelConfig
	provider    ProviderType
	client      *http.Client
	tools       []ToolDefinition // 可用工具定义（由 Agent 注入）
	formatter   Formatter        // 消息格式化器（默认 OpenAIFormatter）
	rateLimiter *RateLimiter     // 速率限制器（ RetryChatModel）
}

// NewOpenAIModel 创建 OpenAI 模型
func NewOpenAIModel(config ModelConfig) *OpenAIModel {
	timeout := time.Duration(config.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	// 根据配置创建格式化器
	formatter := NewFormatter(config.FormatterType)
	if formatter == nil {
		formatter = NewOpenAIFormatter()
	}

	// 创建速率限制器（每个 model key 独立）
	rlConfig := config.RateLimitConfig
	if rlConfig.MaxConcurrent == 0 {
		rlConfig = DefaultRateLimitConfig()
	}
	limiterKey := config.Model + ":" + string(ProviderOpenAI)
	rateLimiter := GetRateLimiter(limiterKey, rlConfig)

	return &OpenAIModel{
		config:      config,
		provider:    ProviderOpenAI,
		client:      &http.Client{Timeout: timeout},
		formatter:   formatter,
		rateLimiter: rateLimiter,
	}
}

// NewDeepSeekModel 创建 DeepSeek 模型
func NewDeepSeekModel(config ModelConfig) *OpenAIModel {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.deepseek.com/v1"
	}
	m := NewOpenAIModel(config)
	m.provider = ProviderDeepSeek
	return m
}

// NewOllamaModel 创建 Ollama 模型
func NewOllamaModel(config ModelConfig) *OpenAIModel {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434/v1"
	}
	m := NewOpenAIModel(config)
	m.provider = ProviderOllama
	return m
}

// Call 同步调用
func (m *OpenAIModel) Call(ctx context.Context, messages []Msg) (*Response, error) {
	reqBody := m.buildRequest(messages, false)

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.config.BaseURL+"/chat/completions", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if m.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.config.APIKey)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	return m.parseResponse(resp.Body)
}

// Stream 流式调用
func (m *OpenAIModel) Stream(ctx context.Context, messages []Msg) (<-chan StreamChunk, error) {
	reqBody := m.buildRequest(messages, true)

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.config.BaseURL+"/chat/completions", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if m.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.config.APIKey)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	output := make(chan StreamChunk, 100)
	go m.parseStreamResponse(resp.Body, output)

	return output, nil
}

// GetName 获取模型名称
func (m *OpenAIModel) GetName() string {
	return m.config.Model
}

// GetProvider 获取提供商名称
func (m *OpenAIModel) GetProvider() string {
	return string(m.provider)
}

// SetTools 设置可用工具定义（实现 ToolSetter 接口）。
//
// Agent 在调用前注入工具定义，使模型能够进行工具调用。
//
// 参数：
//   - tools: 工具定义列表（OpenAI API 格式）
func (m *OpenAIModel) SetTools(tools []ToolDefinition) {
	m.tools = tools
}

// buildRequest 构建请求（使用 Formatter 格式化消息）
func (m *OpenAIModel) buildRequest(messages []Msg, stream bool) map[string]interface{} {
	// 使用 Formatter 格式化消息
	formattedMessages := m.formatter.FormatMessages(messages, m.tools)

	req := map[string]interface{}{
		"model":    m.config.Model,
		"messages": formattedMessages,
		"stream":   stream,
	}

	// 流式请求时启用 usage 返回（OpenAI API 需要 stream_options.include_usage）
	if stream {
		req["stream_options"] = map[string]interface{}{
			"include_usage": true,
		}
	}

	if m.config.MaxTokens > 0 {
		req["max_tokens"] = m.config.MaxTokens
	}

	// 添加工具定义（如果有，使用 Formatter 格式化）
	if len(m.tools) > 0 {
		// OpenAI 格式化器有 FormatTools 方法
		if openaiFormatter, ok := m.formatter.(*OpenAIFormatter); ok {
			req["tools"] = openaiFormatter.FormatTools(m.tools)
		} else {
			// 其他格式化器的工具定义需要单独处理
			req["tools"] = openaiFormatter.FormatTools(m.tools)
		}
	}

	return req
}

// parseResponse 解析响应
func (m *OpenAIModel) parseResponse(body io.Reader) (*Response, error) {
	var resp openAIResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := resp.Choices[0]
	response := &Response{
		Content:    choice.Message.Content,
		StopReason: choice.FinishReason,
		Usage: Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
	}

	// 解析工具调用
	if len(choice.Message.ToolCalls) > 0 {
		response.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			// Arguments 是 JSON 字符串，需要解析
			var params map[string]interface{}
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
					params = make(map[string]interface{})
				}
			}
			response.ToolCalls[i] = ToolCall{
				ID:     tc.ID,
				Name:   tc.Function.Name,
				Params: params,
			}
		}
	}

	// Fallback: 从 content 中提取 XML 格式的工具调用
	// 某些模型（如 MiniMax、Kimi 等）不使用标准 OpenAI tool_calls 格式，
	// 而是将工具调用以 XML 标签形式写入 content 字段。
	if len(response.ToolCalls) == 0 {
		xmlCalls, cleaned := ExtractXMLToolCalls(response.Content)
		if len(xmlCalls) > 0 {
			response.ToolCalls = xmlCalls
			response.Content = cleaned
		}
	}

	return response, nil
}

// parseStreamResponse 解析流式响应
func (m *OpenAIModel) parseStreamResponse(body io.Reader, output chan<- StreamChunk) {
	defer close(output)

	reader := bufio.NewReader(body)

	// 累积工具调用（流式工具调用分多个 chunk 发送）
	toolCallAccum := make(map[int]*ToolCall) // key: tool_call index
	toolCallArgBuf := make(map[int]string)   // key: tool_call index, 累积参数字符串
	inThinkTag := false                      // 跟踪 <think> 标签状态（DeepSeek v4）

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				output <- StreamChunk{Type: "error", Error: err}
			}
			// 流结束时解析并发送已累积的工具调用
			for idx, tc := range toolCallAccum {
				if argBuf, ok := toolCallArgBuf[idx]; ok && argBuf != "" {
					var params map[string]interface{}
					if jsonErr := json.Unmarshal([]byte(argBuf), &params); jsonErr == nil {
						tc.Params = params
					} else {
						tc.Params = make(map[string]interface{})
					}
				}
				if tc.ID != "" {
					output <- StreamChunk{Type: "tool_call", ToolCall: tc}
				}
			}
			return
		}

		rawLine := strings.TrimSpace(line)
		if rawLine == "" {
			continue
		}

		if rawLine == "data: [DONE]" || rawLine == "data:[DONE]" {
			// 解析并发送已累积的工具调用
			for idx, tc := range toolCallAccum {
				if argBuf, ok := toolCallArgBuf[idx]; ok && argBuf != "" {
					var params map[string]interface{}
					if jsonErr := json.Unmarshal([]byte(argBuf), &params); jsonErr == nil {
						tc.Params = params
					} else {
						tc.Params = make(map[string]interface{})
					}
				}
				if tc.ID != "" {
					output <- StreamChunk{Type: "tool_call", ToolCall: tc}
				}
			}
			output <- StreamChunk{Type: "done"}
			continue
		}

		// 兼容两种 SSE 格式: "data: {...}" 和 "data:{...}"
		var data string
		if strings.HasPrefix(rawLine, "data: ") {
			data = strings.TrimPrefix(rawLine, "data: ")
		} else if strings.HasPrefix(rawLine, "data:") {
			data = strings.TrimPrefix(rawLine, "data:")
		} else {
			continue
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		// 解析 usage（OpenAI stream_options.include_usage=true 时最后一个 chunk 包含 usage）
		if chunk.Usage != nil {
			output <- StreamChunk{
				Type:         "usage",
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
			continue // usage chunk 通常没有 Choices，跳过后续处理
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		// 处理思考内容（DeepSeek reasoning_content 字段）
		if choice.Delta.ReasoningContent != "" {
			output <- StreamChunk{
				Type:     "thinking",
				Thinking: choice.Delta.ReasoningContent,
			}
		}

		// 处理文本内容（含 <think> 标签剥离，DeepSeek v4 有时将推理放在 content 字段中）
		if choice.Delta.Content != "" {
			text := choice.Delta.Content
			// 状态机跟踪 <think> 标签
			for {
				if inThinkTag {
					endIdx := strings.Index(text, "</think>")
					if endIdx >= 0 {
						// 结束标签前的部分 → thinking
						if endIdx > 0 {
							output <- StreamChunk{Type: "thinking", Thinking: text[:endIdx]}
						}
						text = text[endIdx+len("</think>"):]
						inThinkTag = false
					} else {
						// 全部是 thinking
						output <- StreamChunk{Type: "thinking", Thinking: text}
						text = ""
						break
					}
				} else {
					startIdx := strings.Index(text, "<think>")
					if startIdx >= 0 {
						// 开始标签前的部分 → content
						if startIdx > 0 {
							output <- StreamChunk{Type: "content", Content: text[:startIdx]}
						}
						text = text[startIdx+len("<think>"):]
						inThinkTag = true
					} else {
						// 没有标签 → content
						if text != "" {
							output <- StreamChunk{Type: "content", Content: text}
						}
						break
					}
				}
			}
		}

		// 处理工具调用（累积模式）
		for _, tc := range choice.Delta.ToolCalls {
			idx := tc.Index
			if toolCallAccum[idx] == nil {
				toolCallAccum[idx] = &ToolCall{
					ID:   tc.ID,
					Name: tc.Function.Name,
				}
				toolCallArgBuf[idx] = ""
			}
			// 更新 ID（首 chunk 可能没有 ID）
			if tc.ID != "" && toolCallAccum[idx].ID == "" {
				toolCallAccum[idx].ID = tc.ID
			}
			// 更新 Name（首 chunk 可能没有 Name）
			if tc.Function.Name != "" && toolCallAccum[idx].Name == "" {
				toolCallAccum[idx].Name = tc.Function.Name
			}
			// 累积参数字符串片段
			if tc.Function.Arguments != "" {
				toolCallArgBuf[idx] += tc.Function.Arguments
			}
		}

		// 结束原因
		if choice.FinishReason != "" {
			if choice.FinishReason == "tool_calls" {
				// 解析并发送所有已累积的工具调用
				for idx, tc := range toolCallAccum {
					if argBuf, ok := toolCallArgBuf[idx]; ok && argBuf != "" {
						var params map[string]interface{}
						if jsonErr := json.Unmarshal([]byte(argBuf), &params); jsonErr == nil {
							tc.Params = params
						} else {
							tc.Params = make(map[string]interface{})
						}
					}
					if tc.ID != "" {
						output <- StreamChunk{Type: "tool_call", ToolCall: tc}
					}
				}
				// 清空累积器
				toolCallAccum = make(map[int]*ToolCall)
				toolCallArgBuf = make(map[int]string)
			}
			output <- StreamChunk{Type: "done"}
		}
	}
}

// OpenAI 响应结构
type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"` // JSON 字符串
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string `json:"role,omitempty"`
			Content          string `json:"content,omitempty"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
			ToolCalls        []struct {
				ID       string `json:"id,omitempty"`
				Index    int    `json:"index"`
				Type     string `json:"type,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	// 流式响应的 usage 字段（stream_options.include_usage=true 时在最后一个 chunk 返回）
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}
