package tool

import (
	"context"

	"go-claw/internal/channel"

	// go-agent 的 Tool 接口直接作为 go-claw 的 Tool 类型
	goAgentTool "github.com/nllihui6390/go-agent/tool"
)

// Tool 工具接口（直接使用 go-agent 的类型）
// go-agent/tool.Tool 签名完全兼容，使用类型别名消除桥接层
type Tool = goAgentTool.Tool

// StructuredTool 结构化工具接口（返回 ContentBlocks）
// go-claw 特有：返回 channel.ContentBlocks 而非 go-agent 的 []ContentBlock，
// 因为 Session 持久化依赖 channel.ContentBlocks 的 JSON 序列化格式
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
