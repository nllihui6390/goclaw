package store

import "context"

// SessionData 持久化会话数据
type SessionData struct {
	ID        string           `json:"id"`
	Channel   string           `json:"channel"`
	User      string           `json:"user"`
	Messages  []SessionMessage `json:"messages"`
	CreatedAt string           `json:"created_at"`
	UpdatedAt string           `json:"updated_at"`
}

// SessionMessage 持久化消息
type SessionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// MemoryEntry 记忆条目 (copy to avoid import cycle)
type MemoryEntry struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	Type        string                 `json:"type"`
	SessionID   string                 `json:"session_id"`
	UserID      string                 `json:"user_id"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
	AccessCount int                    `json:"access_count"`
	Importance  float64                `json:"importance"`
}

// Store 持久化存储接口
type Store interface {
	SaveSession(ctx context.Context, session SessionData) error
	GetSession(ctx context.Context, id string) (*SessionData, error)
	ListSessions(ctx context.Context) ([]SessionData, error)
	DeleteSession(ctx context.Context, id string) error
	CleanupExpiredSessions(ctx context.Context, ttlMinutes int) error

	SaveMemory(ctx context.Context, entry MemoryEntry) error
	ListMemories(ctx context.Context, sessionID string, limit int) ([]MemoryEntry, error)
	DeleteMemory(ctx context.Context, id string) error
	ClearSessionMemories(ctx context.Context, sessionID string) error
}
