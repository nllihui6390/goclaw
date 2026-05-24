package agent

import (
	"sync"
	"time"
)

// Message 历史消息
type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// Session 会话
type Session struct {
	ID        string
	Channel   string
	User      string
	Messages  []Message
	CreatedAt time.Time
	UpdatedAt time.Time
	mu        sync.RWMutex
}

// AddMessage 添加消息
func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = append(s.Messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	s.UpdatedAt = time.Now()

	// 简单限制历史长度（保留最近50条）
	if len(s.Messages) > 50 {
		s.Messages = s.Messages[len(s.Messages)-50:]
	}
}

// SessionManager 会话管理器
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSessionManager 创建会话管理器
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// GetOrCreate 获取或创建会话
func (sm *SessionManager) GetOrCreate(sessionID string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		return session
	}

	session := &Session{
		ID:        sessionID,
		Messages:  []Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sm.sessions[sessionID] = session
	return session
}
