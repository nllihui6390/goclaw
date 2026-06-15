package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/nllihui6390/go-agent/tool"
)

// =============================================
// AgentState 持久化（ 的 AgentState）
//
// AgentScope 的 AgentState 是一个 Pydantic 模型，保存了恢复智能体所需的全部信息：
// - 对话上下文
// - 压缩摘要
// - 权限规则
// - 工具状态
// - 当前 reply 位置
//
// AgentScope 提供两个主要方法：
// - get_session(user_id, agent_id, session_id) → SessionRecord
// - update_session_state(user_id, agent_id, session_id, state) → 更新状态
// =============================================

// AgentState Agent 状态（可序列化为 JSON）
type AgentState struct {
	SessionID   string                 `json:"session_id"`   // 会话 ID
	AgentID     string                 `json:"agent_id"`     // Agent ID
	UserID      string                 `json:"user_id"`      // 用户 ID
	Context     []Msg                  `json:"context"`      // 对话上下文（未压缩消息）
	Summary     *Summary               `json:"summary"`      // 压缩摘要
	Permission  []tool.PermissionRule  `json:"permission"`   // 权限规则
	ToolState   map[string]interface{} `json:"tool_state"`   // 工具状态
	ReplyID     string                 `json:"reply_id"`     // 当前 reply ID（用于恢复）
	ReplyPaused bool                   `json:"reply_paused"` // reply 是否暂停（等待外部事件）
	CreatedAt   string                 `json:"created_at"`   // 创建时间
	UpdatedAt   string                 `json:"updated_at"`   // 更新时间
}

// NewAgentState 创建新的 AgentState
func NewAgentState(sessionID, agentID, userID string) *AgentState {
	return &AgentState{
		SessionID:   sessionID,
		AgentID:     agentID,
		UserID:      userID,
		Context:     make([]Msg, 0),
		Summary:     nil,
		Permission:  make([]tool.PermissionRule, 0),
		ToolState:   make(map[string]interface{}),
		ReplyID:     "",
		ReplyPaused: false,
		CreatedAt:   nowISO(),
		UpdatedAt:   nowISO(),
	}
}

// ToJSON 序列化为 JSON
func (s *AgentState) ToJSON() (string, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ParseAgentState 从 JSON 解析
func ParseAgentState(jsonStr string) (*AgentState, error) {
	var s AgentState
	if err := json.Unmarshal([]byte(jsonStr), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Clone 克隆状态
func (s *AgentState) Clone() *AgentState {
	context := make([]Msg, len(s.Context))
	copy(context, s.Context)

	permission := make([]tool.PermissionRule, len(s.Permission))
	copy(permission, s.Permission)

	toolState := make(map[string]interface{})
	for k, v := range s.ToolState {
		toolState[k] = v
	}

	return &AgentState{
		SessionID:   s.SessionID,
		AgentID:     s.AgentID,
		UserID:      s.UserID,
		Context:     context,
		Summary:     s.Summary,
		Permission:  permission,
		ToolState:   toolState,
		ReplyID:     s.ReplyID,
		ReplyPaused: s.ReplyPaused,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   nowISO(),
	}
}

// UpdateContext 更新上下文
func (s *AgentState) UpdateContext(messages []Msg) {
	s.Context = messages
	s.UpdatedAt = nowISO()
}

// SetSummary 设置摘要
func (s *AgentState) SetSummary(summary *Summary) {
	s.Summary = summary
	s.UpdatedAt = nowISO()
}

// AddPermissionRule 添加权限规则
func (s *AgentState) AddPermissionRule(rule tool.PermissionRule) {
	s.Permission = append(s.Permission, rule)
	s.UpdatedAt = nowISO()
}

// SetToolState 设置工具状态
func (s *AgentState) SetToolState(toolName string, state interface{}) {
	s.ToolState[toolName] = state
	s.UpdatedAt = nowISO()
}

// SetReplyPaused 设置回复暂停状态
func (s *AgentState) SetReplyPaused(replyID string, paused bool) {
	s.ReplyID = replyID
	s.ReplyPaused = paused
	s.UpdatedAt = nowISO()
}

// =============================================
// SessionRecord 会话记录
// =============================================

// SessionRecord 会话记录
type SessionRecord struct {
	SessionID string      `json:"session_id"`
	AgentID   string      `json:"agent_id"`
	UserID    string      `json:"user_id"`
	State     *AgentState `json:"state"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
}

// =============================================
// StateStorage 状态存储接口
// =============================================

// StateStorage 状态存储接口（ 的 RedisStorage）
type StateStorage interface {
	// GetSession 获取会话记录
	GetSession(ctx context.Context, userID, agentID, sessionID string) (*SessionRecord, error)

	// UpsertSession 创建或更新会话（首次创建）
	UpsertSession(ctx context.Context, record *SessionRecord) error

	// UpdateSessionState 更新会话状态
	UpdateSessionState(ctx context.Context, userID, agentID, sessionID string, state *AgentState) error

	// DeleteSession 删除会话
	DeleteSession(ctx context.Context, userID, agentID, sessionID string) error

	// ListSessions 列出用户的所有会话
	ListSessions(ctx context.Context, userID, agentID string) ([]*SessionRecord, error)
}

// =============================================
// InMemoryStateStorage 内存状态存储（用于测试）
// =============================================

// InMemoryStateStorage 内存状态存储
type InMemoryStateStorage struct {
	sessions map[string]*SessionRecord // key: userID:agentID:sessionID
	mu       sync.Mutex
}

// NewInMemoryStateStorage 创建内存状态存储
func NewInMemoryStateStorage() *InMemoryStateStorage {
	return &InMemoryStateStorage{
		sessions: make(map[string]*SessionRecord),
	}
}

// GetSession 获取会话记录
func (s *InMemoryStateStorage) GetSession(ctx context.Context, userID, agentID, sessionID string) (*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := makeKey(userID, agentID, sessionID)
	record := s.sessions[key]
	return record, nil
}

// UpsertSession 创建或更新会话
func (s *InMemoryStateStorage) UpsertSession(ctx context.Context, record *SessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := makeKey(record.UserID, record.AgentID, record.SessionID)
	s.sessions[key] = record
	return nil
}

// UpdateSessionState 更新会话状态
func (s *InMemoryStateStorage) UpdateSessionState(ctx context.Context, userID, agentID, sessionID string, state *AgentState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := makeKey(userID, agentID, sessionID)
	record := s.sessions[key]
	if record == nil {
		// 会话不存在，需要先创建
		return &SessionNotFoundError{UserID: userID, AgentID: agentID, SessionID: sessionID}
	}

	record.State = state
	record.UpdatedAt = nowISO()
	return nil
}

// DeleteSession 删除会话
func (s *InMemoryStateStorage) DeleteSession(ctx context.Context, userID, agentID, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := makeKey(userID, agentID, sessionID)
	delete(s.sessions, key)
	return nil
}

// ListSessions 列出用户的所有会话
func (s *InMemoryStateStorage) ListSessions(ctx context.Context, userID, agentID string) ([]*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*SessionRecord, 0)
	for key, record := range s.sessions {
		if strings.HasPrefix(key, userID+":"+agentID+":") {
			result = append(result, record)
		}
	}
	return result, nil
}

// makeKey 生成存储 key
func makeKey(userID, agentID, sessionID string) string {
	return userID + ":" + agentID + ":" + sessionID
}

// =============================================
// 错误类型
// =============================================

// SessionNotFoundError 会话未找到错误
type SessionNotFoundError struct {
	UserID    string
	AgentID   string
	SessionID string
}

func (e *SessionNotFoundError) Error() string {
	return "session not found: " + makeKey(e.UserID, e.AgentID, e.SessionID)
}
