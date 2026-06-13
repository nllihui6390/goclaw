package store

import (
	"context"
	"encoding/json"
)

// SessionData 持久化会话数据
type SessionData struct {
	ID        string           `json:"id"`          // 主键（= SessionID，如 desktop:local）
	SessionID string           `json:"session_id"`  // 完整会话标识（channel:user_id 格式）
	Name      string           `json:"name"`        // 会话标题（用户的第一句话）
	UserID    string           `json:"user_id"`     // 用户标识（session_id 的右半部分）
	Channel   string           `json:"channel"`     // 渠道名称
	Messages  []SessionMessage `json:"messages"`
	CreatedAt string           `json:"created_at"`
	UpdatedAt string           `json:"updated_at"`
}

// SessionMessage 持久化消息
// Content 使用 json.RawMessage 避免导入 channel 包导致循环依赖
// 实际的 ContentBlocks 序列化/反序列化在 agent 包中处理
type SessionMessage struct {
	Role       string                 `json:"role"`
	Content    json.RawMessage        `json:"content"`
	ToolCallID string                 `json:"tool_call_id,omitempty"` // 工具调用ID（tool角色消息专用）
	Name       string                 `json:"name,omitempty"`         // 工具名称（tool角色消息专用）
	Metadata   map[string]interface{} `json:"metadata,omitempty"`     // 扩展字段（thinking、tool_calls、result等）
	Timestamp  string                 `json:"timestamp"`
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

	// SessionFilePath 返回会话文件的磁盘路径（用于摘要等附加文件存取）
	SessionFilePath(id string) string
}
