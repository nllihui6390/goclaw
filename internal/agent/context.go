package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"go-claw/internal/channel"
	"go-claw/internal/store"
	"go-claw/pkg/log"
)

// Message 历史消息
type Message struct {
	Role       string             // "user", "assistant", "system", "tool"
	Content    channel.ContentBlocks // 从 string 改为 ContentBlocks
	ToolCallID string             // 工具调用ID（tool角色消息专用）
	Name       string             // 工具名称（tool角色消息专用）
	Timestamp  time.Time
}

// Session 会话
type Session struct {
	ID                string // 主键（= SessionID，如 desktop:local）
	SessionID         string // 完整会话标识（channel:user_id 格式）
	Name              string // 会话标题（用户的第一句话）
	UserID            string // 用户标识（session_id 的右半部分）
	Channel           string
	Messages          []Message
	CompressedSummary string // 压缩后的历史摘要
	CreatedAt         time.Time
	UpdatedAt         time.Time
	mu                sync.RWMutex
	store             store.Store
}

// SetChannel 设置渠道名
func (s *Session) SetChannel(ch string) {
	s.mu.Lock()
	s.Channel = ch
	s.mu.Unlock()
}

// SetUser 设置用户 ID
func (s *Session) SetUser(user string) {
	s.mu.Lock()
	s.UserID = user
	s.mu.Unlock()
}

// SetSessionID 设置会话标识
func (s *Session) SetSessionID(id string) {
	s.mu.Lock()
	s.SessionID = id
	s.mu.Unlock()
}

// AddMessage 添加消息（ContentBlocks 版本）
func (s *Session) AddMessage(role string, content channel.ContentBlocks) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果是第一条用户消息，设置为会话名称
	if s.Name == "" && role == "user" && len(s.Messages) == 0 {
		// 从 ContentBlocks 提取文本作为会话名称
		text := channel.TextOnlyContent(content)
		s.Name = text
		// 截断过长的标题
		if len(s.Name) > 50 {
			s.Name = s.Name[:50] + "..."
		}
	}

	s.Messages = append(s.Messages, Message{
		Role:      role,
		Content:   content,
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

// AddTextMessage 添加纯文本消息（便捷方法）
func (s *Session) AddTextMessage(role, text string) {
	s.AddMessage(role, channel.ContentBlocksFromText(text))
}

func (s *Session) persistLocked() {
	if s.store == nil {
		return
	}
	msgs := make([]store.SessionMessage, 0, len(s.Messages))
	for _, m := range s.Messages {
		contentJSON, _ := json.Marshal(m.Content)
		msgs = append(msgs, store.SessionMessage{
			Role:       m.Role,
			Content:    json.RawMessage(contentJSON),
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
			Timestamp:  m.Timestamp.Format(time.RFC3339),
		})
	}
	data := store.SessionData{
		ID:        s.ID,
		SessionID: s.SessionID,
		Name:      s.Name,
		UserID:    s.UserID,
		Channel:   s.Channel,
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

	// 尝试从持久化存储加载已有会话
	var messages []Message
	var chName, userID, name string
	var sessionIDFromData string
	createdAt := time.Now()
	updatedAt := createdAt

	if sm.store != nil {
		if data, err := sm.store.GetSession(context.Background(), sessionID); err == nil && data != nil {
			for _, m := range data.Messages {
				// 解析 Content JSON 为 ContentBlocks
				var content channel.ContentBlocks
				if len(m.Content) > 0 {
					json.Unmarshal(m.Content, &content)
				}
				messages = append(messages, Message{
					Role:       m.Role,
					Content:    content,
					ToolCallID: m.ToolCallID,
					Name:       m.Name,
					Timestamp:  parseRFC3339(m.Timestamp),
				})
			}
			chName = data.Channel
			userID = data.UserID
			name = data.Name
			sessionIDFromData = data.SessionID
			if t, err := time.Parse(time.RFC3339, data.CreatedAt); err == nil {
				createdAt = t
			}
			if t, err := time.Parse(time.RFC3339, data.UpdatedAt); err == nil {
				updatedAt = t
			}
		}
	}

	// 解析 sessionID (格式: "channel:user")
	if chName == "" {
		parts := strings.SplitN(sessionID, ":", 2)
		if len(parts) == 2 {
			chName = parts[0]
			userID = parts[1]
		}
	}
	if userID == "" {
		userID = sessionID
	}
	// SessionID 格式: channel:user（如 web:uuid 或 wecom:userid）
	if sessionIDFromData == "" {
		if chName != "" && userID != "" {
			sessionIDFromData = chName + ":" + userID
		} else {
			sessionIDFromData = sessionID
		}
	}

	session := &Session{
		ID:        sessionID,
		SessionID: sessionIDFromData,
		Name:      name,
		UserID:    userID,
		Channel:   chName,
		Messages:  messages,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		store:     sm.store,
	}

	sm.sessions[sessionID] = session
	return session
}

// parseRFC3339 解析 RFC3339 时间字符串，失败返回零值
func parseRFC3339(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
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
			SessionID: s.SessionID,
			Name:      s.Name,
			UserID:    s.UserID,
			Channel:   s.Channel,
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
