package model

import (
	"encoding/json"
)

// =============================================
// Formatter 接口（ FormatterBase）
// =============================================

// Formatter 消息格式化器接口。
//
// 将统一的 Msg 格式转换为特定提供商的 API 请求格式。
//
//	FileBlockSupportFormatter 模式：
//
// 不同提供商使用不同的 Formatter 实现。
type Formatter interface {
	// FormatMessages 将消息列表格式化为请求体。
	//
	// 参数：
	//   - messages: 统一格式的消息列表
	//   - tools: 可用工具定义（可选）
	//
	// 返回：
	//   - []map[string]interface{}: 格式化后的消息列表（可直接序列化）
	FormatMessages(messages []Msg, tools []ToolDefinition) []map[string]interface{}
}

// FormatterType 格式化器类型标识。
type FormatterType string

const (
	FormatterOpenAI    FormatterType = "openai"    // OpenAI 兼容格式（默认）
	FormatterAnthropic FormatterType = "anthropic" // Anthropic Claude 格式
	FormatterGemini    FormatterType = "gemini"    // Google Gemini 格式
	FormatterDeepSeek  FormatterType = "deepseek"  // DeepSeek 格式（同 OpenAI）
)

// =============================================
// OpenAI Formatter
// =============================================

// OpenAIFormatter OpenAI 兼容格式化器。
type OpenAIFormatter struct{}

// NewOpenAIFormatter 创建 OpenAI 格式化器。
func NewOpenAIFormatter() *OpenAIFormatter {
	return &OpenAIFormatter{}
}

// FormatMessages 实现 Formatter 接口。
func (f *OpenAIFormatter) FormatMessages(messages []Msg, tools []ToolDefinition) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		result[i] = f.formatMessage(msg)
	}
	return result
}

// formatMessage 格式化单条消息。
func (f *OpenAIFormatter) formatMessage(msg Msg) map[string]interface{} {
	openaiMsg := map[string]interface{}{
		"role": msg.Role,
	}

	// 根据角色设置内容
	if msg.Role == "tool" {
		// 工具结果消息
		openaiMsg["tool_call_id"] = msg.ToolCallID
		openaiMsg["content"] = msg.Content
	} else if len(msg.ToolCalls) > 0 {
		// 带工具调用的助手消息
		if msg.Content != "" {
			openaiMsg["content"] = msg.Content
		} else {
			openaiMsg["content"] = nil
		}
		toolCallsJSON := make([]map[string]interface{}, len(msg.ToolCalls))
		for j, tc := range msg.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Params)
			toolCallsJSON[j] = map[string]interface{}{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      tc.Name,
					"arguments": string(argsJSON),
				},
			}
		}
		openaiMsg["tool_calls"] = toolCallsJSON
	} else {
		// 普通消息
		openaiMsg["content"] = msg.Content
		if msg.Name != "" {
			openaiMsg["name"] = msg.Name
		}
	}

	return openaiMsg
}

// FormatTools 格式化工具定义为 OpenAI API 格式。
func (f *OpenAIFormatter) FormatTools(tools []ToolDefinition) []map[string]interface{} {
	if len(tools) == 0 {
		return nil
	}
	toolDefs := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		toolDefs[i] = map[string]interface{}{
			"type": t.Type,
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		}
	}
	return toolDefs
}

// =============================================
// Anthropic Formatter
// =============================================

// AnthropicFormatter Anthropic Claude 格式化器。
type AnthropicFormatter struct{}

// NewAnthropicFormatter 创建 Anthropic 格式化器。
func NewAnthropicFormatter() *AnthropicFormatter {
	return &AnthropicFormatter{}
}

// FormatMessages 实现 Formatter 接口。
func (f *AnthropicFormatter) FormatMessages(messages []Msg, tools []ToolDefinition) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(messages))

	for _, msg := range messages {
		if msg.Role == "tool" {
			// Anthropic: tool_result 作为 user 消息
			result = append(result, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{{
					"type":        "tool_result",
					"tool_use_id": msg.ToolCallID,
					"content":     msg.Content,
				}},
			})
		} else if len(msg.ToolCalls) > 0 {
			// 带工具调用的助手消息
			content := make([]map[string]interface{}, 0)
			if msg.Content != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Params)
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": json.RawMessage(argsJSON),
				})
			}
			result = append(result, map[string]interface{}{
				"role":    "assistant",
				"content": content,
			})
		} else {
			// 普通消息
			anthropicMsg := map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			}
			result = append(result, anthropicMsg)
		}
	}

	return result
}

// FormatTools 格式化工具定义为 Anthropic API 格式。
func (f *AnthropicFormatter) FormatTools(tools []ToolDefinition) []map[string]interface{} {
	if len(tools) == 0 {
		return nil
	}
	toolDefs := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		toolDefs[i] = map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"input_schema": map[string]interface{}{
				"type":       "object",
				"properties": t.Parameters["properties"],
				"required":   t.Parameters["required"],
			},
		}
	}
	return toolDefs
}

// =============================================
// Formatter 工厂
// =============================================

// NewFormatter 根据类型创建格式化器。
func NewFormatter(formatterType FormatterType) Formatter {
	switch formatterType {
	case FormatterAnthropic:
		return NewAnthropicFormatter()
	case FormatterGemini:
		// Gemini 使用 OpenAI 兼容格式
		return NewOpenAIFormatter()
	case FormatterDeepSeek:
		// DeepSeek 使用 OpenAI 兼容格式
		return NewOpenAIFormatter()
	default:
		return NewOpenAIFormatter()
	}
}
