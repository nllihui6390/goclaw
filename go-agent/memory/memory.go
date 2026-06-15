// Package memory 提供 Agent 记忆存储抽象。
//
// 记忆（Memory）是 Agent 的长期信息存储机制，
// 用于在多次对话之间保留重要信息。
// 支持短期记忆和长期记忆两级体系。
//
// 使用示例：
//
//	mem := memory.NewSimpleMemory()
//	mem.Store(ctx, "key", "content", "short_term")
//	items, _ := mem.Retrieve(ctx, "query", 5)
package memory

import "context"

// =============================================
// MemoryItem — 记忆项
// =============================================

// MemoryItem 记忆项。
//
// 字段：
//   - ID: 唯一标识符
//   - Content: 记忆内容
//   - Type: 记忆类型（"short_term" 或 "long_term"）
//   - Importance: 重要性分数（0.0-1.0），用于 Consolidate 判断
//   - Metadata: 任意元数据（可选）
//   - CreatedAt: 创建时间（Unix 时间戳）
type MemoryItem struct {
	ID         string                 `json:"id"`
	Content    string                 `json:"content"`
	Type       string                 `json:"type"` // "short_term" / "long_term"
	Importance float64                `json:"importance"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  int64                  `json:"created_at"`
}

// =============================================
// Memory 接口
// =============================================

// Memory 记忆接口。
//
// 定义 Agent 长期记忆的存储、检索、合并和遗忘操作。
// 支持两级记忆体系：
//   - short_term: 最近对话内容（自动存储）
//   - long_term: 重要的长期事实（通过 Consolidate 转换）
type Memory interface {
	// Store 存储记忆项。
	//
	// 参数：
	//   - ctx: 上下文
	//   - key: 记忆键（用于检索和去重）
	//   - content: 记忆内容
	//   - memType: 记忆类型（"short_term" 或 "long_term"）
	//
	// 返回：
	//   - error: 存储错误
	Store(ctx context.Context, key string, content string, memType string) error

	// Retrieve 根据查询检索相关记忆。
	//
	// 参数：
	//   - ctx: 上下文
	//   - query: 查询文本
	//   - limit: 最大返回数量（0 表示返回全部）
	//
	// 返回：
	//   - []MemoryItem: 相关记忆列表（按相关性排序）
	//   - error: 检索错误
	Retrieve(ctx context.Context, query string, limit int) ([]MemoryItem, error)

	// GetRecent 获取最近存储的记忆。
	//
	// 参数：
	//   - ctx: 上下文
	//   - limit: 最大返回数量
	//
	// 返回：
	//   - []MemoryItem: 最近记忆列表
	//   - error: 检索错误
	GetRecent(ctx context.Context, limit int) ([]MemoryItem, error)

	// Consolidate 合并记忆：将重要性超过阈值的短期记忆转为长期记忆。
	//
	// 参数：
	//   - ctx: 上下文
	//   - threshold: 重要性阈值（0.0-1.0）
	//
	// 返回：
	//   - error: 合并错误
	Consolidate(ctx context.Context, threshold float64) error

	// Forget 遗忘指定记忆。
	//
	// 参数：
	//   - ctx: 上下文
	//   - id: 记忆 ID
	//
	// 返回：
	//   - error: 遗忘错误（记忆不存在时不报错）
	Forget(ctx context.Context, id string) error

	// Clear 清除所有记忆。
	//
	// 参数：
	//   - ctx: 上下文
	//
	// 返回：
	//   - error: 清除错误
	Clear(ctx context.Context) error
}

// =============================================
// 辅助类型
// =============================================

// SearchOptions 记忆搜索选项。
type SearchOptions struct {
	Type       string  // 按类型过滤（"" 表示不过滤）
	MinScore   float64 // 最小相关性分数
	MaxResults int     // 最大结果数
	SortBy     string  // 排序字段："importance"、"time"、"relevance"
}

// ScoredMemoryItem 带相关性分数的记忆项。
//
// 嵌入 MemoryItem 并附加相关性分数。
type ScoredMemoryItem struct {
	MemoryItem
	Score float64 // 相关性分数（0.0-1.0）
}