package tool

import (
	"context"

	"go-claw/internal/channel"
)

// Tool 工具接口（返回纯文本）
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

// StructuredTool 结构化工具接口（返回 ContentBlocks）
// 工具可以同时实现 Tool 和 StructuredTool 接口
// Runtime 会优先使用 StructuredTool 接口
type StructuredTool interface {
	Tool
	ExecuteStructured(ctx context.Context, params map[string]interface{}) (channel.ContentBlocks, error)
}

// AsStructuredTool 尝试将 Tool 转换为 StructuredTool
func AsStructuredTool(t Tool) StructuredTool {
	if st, ok := t.(StructuredTool); ok {
		return st
	}
	return nil
}

// WrapTextResult 将纯文本结果包装为 ContentBlocks
func WrapTextResult(text string) channel.ContentBlocks {
	return channel.ContentBlocksFromText(text)
}
