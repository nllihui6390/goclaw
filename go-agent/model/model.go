// Package model 提供 LLM 模型抽象接口和响应类型。
//
// 核心接口 ChatModel 定义了与语言模型交互的统一契约：
//   - Call: 同步调用
//   - Stream: 流式调用
//
// 通过实现 ChatModel 接口可适配任意 LLM 提供商（OpenAI、DeepSeek、Ollama 等）。
package model

import (
	"context"
	"encoding/json"
)

// =============================================
// ChatModel — LLM 模型抽象接口（ 的 ChatModelBase）
// =============================================

// ChatModel 抽象 LLM 模型接口。
//
// 定义与语言模型交互的统一契约。所有模型实现
// （OpenAI、DeepSeek、Ollama、Anthropic 等）都实现此接口。
type ChatModel interface {
	// Call 同步调用模型，传入消息列表，返回完整响应。
	//
	// 参数：
	//   - ctx: 上下文（用于取消和超时）
	//   - messages: 消息列表（按对话顺序）
	//
	// 返回：
	//   - *Response: 模型响应（含文本、工具调用、用量）
	//   - error: 调用错误
	Call(ctx context.Context, messages []Msg) (*Response, error)

	// Stream 流式调用模型，返回增量输出 channel。
	//
	// 参数：
	//   - ctx: 上下文（取消 ctx 会停止流）
	//   - messages: 消息列表
	//
	// 返回：
	//   - <-chan StreamChunk: 流式输出 channel（处理完成后关闭）
	//   - error: 启动错误
	Stream(ctx context.Context, messages []Msg) (<-chan StreamChunk, error)

	// GetName 获取模型名称（如 "gpt-4"、"deepseek-chat"）。
	//
	// 返回：
	//   - string: 模型名称
	GetName() string

	// GetProvider 获取提供商名称（如 "openai"、"deepseek"、"ollama"）。
	//
	// 返回：
	//   - string: 提供商名称
	GetProvider() string
}

// =============================================
// 消息和响应类型
// =============================================

// Msg 模型层的消息结构（API 层使用）。
//
// 与 agent 包中的 Msg 分离以避免循环导入。
// Agent 层在调用模型前将 agent.Msg 转换为此结构。
//
// 字段：
//   - Role: 角色（"system"、"user"、"assistant"、"tool"）
//   - Content: 文本内容
//   - Name: 发送方名称（可选）
//   - ToolCallID: 工具调用 ID（role=tool 时需要）
//   - ToolCalls: OpenAI 格式的工具调用列表（assistant 消息中有工具调用时）
type Msg struct {
	Role       string                 `json:"role"`
	Content    string                 `json:"content"`
	Name       string                 `json:"name,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	Blocks     []ContentBlock         `json:"blocks,omitempty"`
	ToolCalls  []ToolCall             `json:"tool_calls,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

// ContentBlock 模型层的多模态内容块。
type ContentBlock struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text,omitempty"`
	ImageURL string                 `json:"image_url,omitempty"`
	Data     []byte                 `json:"data,omitempty"`
	MimeType string                 `json:"mime_type,omitempty"`
	Extra    map[string]interface{} `json:"extra,omitempty"`
}

// Response 模型响应。
//
// 字段：
//   - Content: 文本回复
//   - ToolCalls: 工具调用列表（模型请求调用工具时）
//   - Usage: Token 用量
//   - StopReason: 停止原因（"stop"、"tool_calls"、"length"）
type Response struct {
	Content    string     // 文本回复
	ToolCalls  []ToolCall // 工具调用列表
	Usage      Usage      // Token 用量
	StopReason string     // 停止原因
}

// ToolCall 工具调用信息。
//
// 字段：
//   - ID: 工具调用唯一标识符
//   - Name: 工具名称
//   - Params: 工具参数（JSON 已解析为 map）
type ToolCall struct {
	ID     string                 // 工具调用 ID
	Name   string                 // 工具名称
	Params map[string]interface{} // 工具参数
}

// ParamsToJSON 将工具参数 map 序列化为 JSON 字符串。
//
// 参数：
//   - params: 工具参数 map
//
// 返回：
//   - string: JSON 字符串
//   - error: 序列化错误
func ParamsToJSON(params map[string]interface{}) (string, error) {
	if params == nil {
		return "{}", nil
	}
	bytes, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Usage Token 用量统计。
//
// 字段：
//   - InputTokens: 输入（prompt）token 数
//   - OutputTokens: 输出（completion）token 数
//   - TotalTokens: 总 token 数
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// StreamChunk 流式输出块。
//
// 字段：
//   - Type: 块类型（"content"、"thinking"、"tool_call"、"done"、"error"）
//   - Content: 增量文本（Type="content" 时）
//   - Thinking: 思考内容增量（Type="thinking" 时，DeepSeek 的 reasoning_content）
//   - ToolCall: 工具调用（Type="tool_call" 时）
//   - Error: 错误（Type="error" 时）
type StreamChunk struct {
	Type     string    // 块类型
	Content  string    // 增量文本
	Thinking string    // 思考内容增量（DeepSeek reasoning）
	ToolCall *ToolCall // 工具调用
	Error    error     // 错误
}

// ToolDefinition 工具定义（OpenAI API 格式）。
//
// 用于向模型传递可用工具信息，模型据此决定是否调用工具。
type ToolDefinition struct {
	Type        string                 `json:"type"`        // 工具类型，通常为 "function"
	Name        string                 `json:"name"`        // 工具名称
	Description string                 `json:"description"` // 工具描述
	Parameters  map[string]interface{} `json:"parameters"`  // 参数 JSON Schema
}

// ToolSetter 工具设置接口（可选）。
//
// 模型实现此接口后，Agent 可在调用前注入工具定义。
// 未实现此接口的模型将无法使用工具调用功能。
type ToolSetter interface {
	SetTools(tools []ToolDefinition)
}

// ModelConfig 模型配置。
//
// 字段：
//   - Model: 模型名称
//   - APIKey: API 密钥
//   - BaseURL: API 基础 URL
//   - Timeout: 超时时间（秒，默认 60）
//   - MaxTokens: 最大输出 token 数
//   - FormatterType: 格式化器类型（默认 "openai"， Formatter 系统）
//   - RateLimitConfig: 速率限制配置（ RetryChatModel）
type ModelConfig struct {
	Model           string          // 模型名称
	APIKey          string          // API 密钥
	BaseURL         string          // API 基础 URL
	Timeout         int             // 超时时间（秒）
	MaxTokens       int             // 最大输出 token
	FormatterType   FormatterType   // 格式化器类型（默认 OpenAI）
	RateLimitConfig RateLimitConfig // 速率限制配置
}

// =============================================
// ProviderType — 提供商类型标识
// =============================================

// ProviderType 提供商类型。
type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"    // OpenAI API（及兼容服务）
	ProviderAnthropic ProviderType = "anthropic" // Anthropic Claude API
	ProviderDeepSeek  ProviderType = "deepseek"  // DeepSeek API
	ProviderOllama    ProviderType = "ollama"    // Ollama 本地服务
	ProviderCustom    ProviderType = "custom"    // 自定义
)
