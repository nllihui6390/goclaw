package memory

import (
	"context"
	"time"

	goAgentMem "github.com/nllihui6390/go-agent/memory"
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

// ToAgentItem 转换为 go-agent MemoryItem
func (e *MemoryEntry) ToAgentItem() goAgentMem.MemoryItem {
	return goAgentMem.MemoryItem{
		ID:         e.ID,
		Content:    e.Content,
		Type:       e.Type,
		Importance: e.Importance,
		Metadata:   e.Metadata,
		CreatedAt:  e.CreatedAt.Unix(),
	}
}

// Memory 记忆接口（go-claw 扩展版，兼容 go-agent MemorySession）
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

// AsAgentMem 将 go-claw Memory 转为 go-agent 兼容接口。
// 实现了 goAgentMem.MemorySession 接口。
func AsAgentMem(m Memory) goAgentMem.Memory {
	return &agentMemWrapper{inner: m}
}

// agentMemWrapper 将 go-claw Memory 包装为 go-agent Memory/MemorySession。
type agentMemWrapper struct {
	inner Memory
}

func (w *agentMemWrapper) Store(ctx context.Context, key, content, memType string) error {
	return w.inner.Store(ctx, MemoryEntry{
		ID:         key,
		Content:    content,
		Type:       memType,
		Importance: 0.5,
		CreatedAt:  time.Now(),
	})
}

func (w *agentMemWrapper) Retrieve(ctx context.Context, query string, limit int) ([]goAgentMem.MemoryItem, error) {
	results, err := w.inner.Retrieve(ctx, query, "", limit)
	if err != nil {
		return nil, err
	}
	items := make([]goAgentMem.MemoryItem, len(results))
	for i, r := range results {
		items[i] = r.Entry.ToAgentItem()
	}
	return items, nil
}

func (w *agentMemWrapper) GetRecent(ctx context.Context, limit int) ([]goAgentMem.MemoryItem, error) {
	entries, err := w.inner.GetRecent(ctx, "", limit)
	if err != nil {
		return nil, err
	}
	items := make([]goAgentMem.MemoryItem, len(entries))
	for i, e := range entries {
		items[i] = e.ToAgentItem()
	}
	return items, nil
}

func (w *agentMemWrapper) Consolidate(ctx context.Context, threshold float64) error {
	return w.inner.Consolidate(ctx)
}

func (w *agentMemWrapper) Forget(ctx context.Context, id string) error {
	return w.inner.Delete(ctx, id)
}

func (w *agentMemWrapper) Clear(ctx context.Context) error { return nil }

// MemorySession 方法
func (w *agentMemWrapper) StoreEntry(ctx context.Context, sessionID, userID string, entry goAgentMem.MemoryItem) error {
	return w.inner.Store(ctx, MemoryEntry{
		ID:         entry.ID,
		Content:    entry.Content,
		Type:       entry.Type,
		SessionID:  sessionID,
		UserID:     userID,
		Metadata:   entry.Metadata,
		Importance: entry.Importance,
		CreatedAt:  time.Unix(entry.CreatedAt, 0),
	})
}

func (w *agentMemWrapper) RetrieveWithSession(ctx context.Context, query, sessionID string, limit int) ([]goAgentMem.ScoredMemoryItem, error) {
	results, err := w.inner.Retrieve(ctx, query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	items := make([]goAgentMem.ScoredMemoryItem, len(results))
	for i, r := range results {
		items[i] = goAgentMem.ScoredMemoryItem{
			MemoryItem: r.Entry.ToAgentItem(),
			Score:      r.Score,
		}
	}
	return items, nil
}

func (w *agentMemWrapper) GetRecentWithSession(ctx context.Context, sessionID string, limit int) ([]goAgentMem.MemoryItem, error) {
	entries, err := w.inner.GetRecent(ctx, sessionID, limit)
	if err != nil {
		return nil, err
	}
	items := make([]goAgentMem.MemoryItem, len(entries))
	for i, e := range entries {
		items[i] = e.ToAgentItem()
	}
	return items, nil
}

func (w *agentMemWrapper) GetByID(ctx context.Context, id string) (*goAgentMem.MemoryItem, error) {
	entry, err := w.inner.GetByID(ctx, id)
	if err != nil || entry == nil {
		return nil, err
	}
	item := entry.ToAgentItem()
	return &item, nil
}

func (w *agentMemWrapper) Update(ctx context.Context, entry goAgentMem.MemoryItem) error {
	return w.inner.Update(ctx, MemoryEntry{
		ID:         entry.ID,
		Content:    entry.Content,
		Type:       entry.Type,
		Importance: entry.Importance,
		Metadata:   entry.Metadata,
		CreatedAt:  time.Unix(entry.CreatedAt, 0),
	})
}

func (w *agentMemWrapper) ClearSession(ctx context.Context, sessionID string) error {
	return w.inner.ClearSession(ctx, sessionID)
}

// 确保编译时检查
var _ goAgentMem.Memory = (*agentMemWrapper)(nil)
var _ goAgentMem.MemorySession = (*agentMemWrapper)(nil)
