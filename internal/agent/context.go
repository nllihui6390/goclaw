package agent

import (
	"context"
	"strings"
	"sync"
	"time"

	"go-claw/internal/store"
	"go-claw/pkg/log"
)

// Message 历史消息
type Message struct {
	Role       string    // "user", "assistant", "system", "tool"
	Content    string
	ToolCallID string    // 工具调用ID（tool角色消息专用）
	Name       string    // 工具名称（tool角色消息专用）
	Timestamp  time.Time
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
	store     store.Store
}

// AddMessage 添加消息
func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = append(s.Messages, Message{
		Role:      role,
		Content:  content,
		Timestamp: time.Now(),
	})
	s.UpdatedAt = time.Now()

	// 保留最近50条
	if len(s.Messages) > 50 {
		s.Messages = s.Messages[len(s.Messages)-50:]
	}

	// 持久化
	s.persistLocked()
}

func (s *Session) persistLocked() {
	if s.store == nil {
		return
	}
	msgs := make([]store.SessionMessage, 0, len(s.Messages))
	for _, m := range s.Messages {
		msgs = append(msgs, store.SessionMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
			Timestamp:  m.Timestamp.Format(time.RFC3339),
		})
	}
	data := store.SessionData{
		ID:        s.ID,
		Channel:   s.Channel,
		User:      s.User,
		Messages:  msgs,
		CreatedAt: s.CreatedAt.Format(time.RFC3339),
		UpdatedAt: s.UpdatedAt.Format(time.RFC3339),
	}
	if err := s.store.SaveSession(context.Background(), data); err != nil {
		log.Logger().Error("保存会话失败", "err", err)
	}
}

// SessionManager 会话管理器
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	store    store.Store
}

// NewSessionManager 创建会话管理器
func NewSessionManager(store store.Store) *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
		store:    store,
	}
	return sm
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
		store:     sm.store,
	}

	// 解析 sessionID (格式: "channel:user")
	parts := strings.SplitN(sessionID, ":", 2)
	if len(parts) == 2 {
		session.Channel = parts[0]
		session.User = parts[1]
	}

	sm.sessions[sessionID] = session

	// 持久化
	if sm.store != nil {
		sm.store.SaveSession(context.Background(), store.SessionData{
			ID:        session.ID,
			Channel:   session.Channel,
			User:      session.User,
			CreatedAt: session.CreatedAt.Format(time.RFC3339),
			UpdatedAt: session.UpdatedAt.Format(time.RFC3339),
		})
	}

	return session
}

// ListSessions 列出所有会话（不含消息内容）
func (sm *SessionManager) ListSessions() []Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var list []Session
	for _, s := range sm.sessions {
		s.mu.RLock()
		list = append(list, Session{
			ID:        s.ID,
			Channel:   s.Channel,
			User:      s.User,
			Messages:  nil, // 列表不返回消息
			CreatedAt: s.CreatedAt,
			UpdatedAt: s.UpdatedAt,
		})
		s.mu.RUnlock()
	}
	return list
}

// GetSession 获取单个会话
func (sm *SessionManager) GetSession(id string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, exists := sm.sessions[id]
	return s, exists
}

// DeleteSession 删除会话
func (sm *SessionManager) DeleteSession(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)
	if sm.store != nil {
		return sm.store.DeleteSession(context.Background(), id)
	}
	return nil
}

// CleanupExpired 清理过期会话
func (sm *SessionManager) CleanupExpired(ttlMinutes int) {
	if ttlMinutes <= 0 {
		return
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	cutoff := time.Now().Add(-time.Duration(ttlMinutes) * time.Minute)
	changed := false
	for id, s := range sm.sessions {
		s.mu.RLock()
		if s.UpdatedAt.Before(cutoff) {
			delete(sm.sessions, id)
			changed = true
		}
		s.mu.RUnlock()
	}
	if changed && sm.store != nil {
		sm.store.CleanupExpiredSessions(context.Background(), ttlMinutes)
	}
}
