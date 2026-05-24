package memory

import (
	"context"
	"time"
)

// MemoryEntry 记忆条目
type MemoryEntry struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	Type        string                 `json:"type"` // "short_term", "long_term", "episodic"
	SessionID   string                 `json:"session_id"`
	UserID      string                 `json:"user_id"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	AccessCount int                    `json:"access_count"`
	Importance  float64                `json:"importance"` // 重要性评分 0-1
}

// SearchResult 搜索结果
type SearchResult struct {
	Entry MemoryEntry
	Score float64
}

// Memory 记忆接口
type Memory interface {
	// Store 存储记忆
	Store(ctx context.Context, entry MemoryEntry) error

	// Retrieve 检索相关记忆
	Retrieve(ctx context.Context, query string, sessionID string, limit int) ([]SearchResult, error)

	// GetRecent 获取最近的记忆
	GetRecent(ctx context.Context, sessionID string, limit int) ([]MemoryEntry, error)

	// GetByID 根据ID获取记忆
	GetByID(ctx context.Context, id string) (*MemoryEntry, error)

	// Update 更新记忆
	Update(ctx context.Context, entry MemoryEntry) error

	// Delete 删除记忆
	Delete(ctx context.Context, id string) error

	// ClearSession 清除会话记忆
	ClearSession(ctx context.Context, sessionID string) error

	// Consolidate 记忆巩固（将短期转为长期）
	Consolidate(ctx context.Context) error

	// Forget 遗忘不重要的记忆
	Forget(ctx context.Context, threshold float64) error
}
