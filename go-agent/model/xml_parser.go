// Package model 提供 XML 格式工具调用解析器。
//
// 某些国产模型（MiniMax、Kimi 等）不使用标准 OpenAI tool_calls 格式，
// 而是将工具调用以 XML 标签形式写入 content 字段。此模块提供
// 三种 XML 格式的工具调用提取：
//   - <invoke name="xxx"><parameter ...> 格式（MiniMax 等）
//   - <tool_use name="xxx"><input>...</input> 格式（Anthropic 风格）
//   - <tool_call>xxx 格式（自定义格式）
package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// xmlToolCallPatterns XML 工具调用格式正则（预编译，避免每次调用重新编译）
var xmlToolCallPatterns = struct {
	invokeBlock  *regexp.Regexp // <invoke name="xxx">...</invoke>
	parameter    *regexp.Regexp // <parameter name="key">value</parameter>
	toolUseBlock *regexp.Regexp // <tool_use name="xxx">...</tool_use>
	inputJSON    *regexp.Regexp // <input>{"key":"value"}</input>
	antToolCall  *regexp.Regexp // <arg_key>xxx 格式
	allXMLTags   *regexp.Regexp // 清理所有 XML 标签的通用正则
	minimaxClose *regexp.Regexp // </minimax:tool_call> 结尾标签
}{
	invokeBlock:  regexp.MustCompile(`(?s)<invoke\s+name="([^"]+)">\s*(.*?)\s*</invoke>`),
	parameter:    regexp.MustCompile(`<parameter\s+name="([^"]+)">(.*?)</parameter>`),
	toolUseBlock: regexp.MustCompile(`(?s)<tool_use\s+name="([^"]+)">\s*(.*?)\s*</tool_use>`),
	inputJSON:    regexp.MustCompile(`(?s)<input>\s*(.*?)\s*</input>`),
	antToolCall:  regexp.MustCompile(`(?s)<tool_call>\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\n(.*?)\n`),
	allXMLTags:   regexp.MustCompile(`(?s)<(?:/?[\w:]+)[^>]*>`),
	minimaxClose: regexp.MustCompile(`</minimax:tool_call>`),
}

// xmlCallCounter 用于生成唯一的 XML 工具调用 ID
var xmlCallCounter atomic.Int64

// generateXMLCallID 生成唯一的 XML 工具调用 ID。
//
// 格式：xml_call_<timestamp_ms>_<counter>
//
// 返回：
//   - string: 唯一 ID
func generateXMLCallID() string {
	return fmt.Sprintf("xml_call_%d_%d", time.Now().UnixMilli(), xmlCallCounter.Add(1))
}

// ExtractXMLToolCalls 从 content 中提取 XML 格式的工具调用。
//
// 支持三种格式：
//   - <invoke name="xxx"><parameter name="key">value</parameter></invoke>
//   - <tool_use name="xxx"><input>{"key":"value"}</input></tool_use>
//   - <arg_key>xxx\n<parameter ...>...\n
//
// 参数：
//   - content: LLM 返回的原始文本内容
//
// 返回：
//   - []ToolCall: 提取到的工具调用列表
//   - string: 清理 XML 标签后的纯文本
func ExtractXMLToolCalls(content string) ([]ToolCall, string) {
	var toolCalls []ToolCall

	// 1. 提取 <invoke name="xxx"><parameter ...> 格式（MiniMax/某些国产模型）
	invokeMatches := xmlToolCallPatterns.invokeBlock.FindAllStringSubmatch(content, -1)
	for _, match := range invokeMatches {
		toolName := match[1]
		body := match[2]

		// 从 body 中提取所有 parameter
		params := map[string]interface{}{}
		paramMatches := xmlToolCallPatterns.parameter.FindAllStringSubmatch(body, -1)
		for _, pm := range paramMatches {
			params[pm[1]] = pm[2]
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:     generateXMLCallID(),
			Name:   toolName,
			Params: params,
		})
	}

	// 2. 提取 <tool_use name="xxx"><input>...</input> 格式（Anthropic Claude 风格）
	toolUseMatches := xmlToolCallPatterns.toolUseBlock.FindAllStringSubmatch(content, -1)
	for _, match := range toolUseMatches {
		toolName := match[1]
		body := match[2]

		// 尝试从 <input> 中提取 JSON
		params := map[string]interface{}{}
		inputMatch := xmlToolCallPatterns.inputJSON.FindStringSubmatch(body)
		if len(inputMatch) >= 2 {
			// 验证是否为合法 JSON
			if json.Unmarshal([]byte(strings.TrimSpace(inputMatch[1])), &params) == nil {
				// 成功解析 JSON
			} else {
				params = nil
			}
		}

		if params == nil {
			// 降级：从 parameter 格式提取
			params = map[string]interface{}{}
			paramMatches := xmlToolCallPatterns.parameter.FindAllStringSubmatch(body, -1)
			for _, pm := range paramMatches {
				params[pm[1]] = pm[2]
			}
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:     generateXMLCallID(),
			Name:   toolName,
			Params: params,
		})
	}

	// 3. 提取 <arg_key>xxx 格式（某些模型的自定义格式）
	antMatches := xmlToolCallPatterns.antToolCall.FindAllStringSubmatch(content, -1)
	for _, match := range antMatches {
		toolName := match[1]
		body := match[2]

		// 从 body 中提取 parameter
		params := map[string]interface{}{}
		paramMatches := xmlToolCallPatterns.parameter.FindAllStringSubmatch(body, -1)
		for _, pm := range paramMatches {
			params[pm[1]] = pm[2]
		}

		// 如果没有 parameter，尝试提取 JSON
		if len(params) == 0 {
			var test map[string]interface{}
			if json.Unmarshal([]byte(strings.TrimSpace(body)), &test) == nil {
				params = test
			}
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:     generateXMLCallID(),
			Name:   toolName,
			Params: params,
		})
	}

	// 4. 清理 content：移除所有 XML 标签和 minimax:tool_call 标签
	cleaned := xmlToolCallPatterns.minimaxClose.ReplaceAllString(content, "")
	cleaned = xmlToolCallPatterns.allXMLTags.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	return toolCalls, cleaned
}