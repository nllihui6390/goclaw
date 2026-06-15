package agent

import (
	"sync"
)

// =============================================
// Session — 会话管理
// =============================================

// Session 管理对话历史记录。
//
// 会话存储当前对话中所有的 Msg（用户消息、assistant 回复、工具调用/结果）。
// 通过 SessionStore 接口支持持久化到不同后端（内存、文件、数据库等）。
//
// 会话是并发安全的（内部使用互斥锁保护）。
type Session struct {
	id      string       // 会话唯一标识符
	store   SessionStore // 会话持久化存储
	history []Msg        // 对话历史（按时间排序）
	mu      sync.Mutex   // 互斥锁，保证并发安全
}

// SessionStore 会话存储接口。
//
// 定义会话消息的持久化方式。可以通过实现此接口
// 将会话存储到内存、文件、数据库、Redis 等不同后端。
type SessionStore interface {
	// Save 保存会话的全部消息到存储。
	//
	// 参数：
	//   - sessionID: 会话唯一标识符
	//   - messages: 要保存的消息列表
	//
	// 返回：
	//   - error: 保存错误
	Save(sessionID string, messages []Msg) error

	// Load 从存储中加载会话的全部消息。
	//
	// 参数：
	//   - sessionID: 会话唯一标识符
	//
	// 返回：
	//   - []Msg: 消息列表（会话不存在时返回 nil, nil）
	//   - error: 加载错误
	Load(sessionID string) ([]Msg, error)
}

// =============================================
// Session 方法
// =============================================

// NewSession 创建新会话。
//
// 自动生成唯一 session ID，初始化空历史记录。
//
// 参数：
//   - store: 会话存储实现（nil 表示不持久化）
//
// 返回：
//   - *Session: 初始化完成的会话
func NewSession(store SessionStore) *Session {
	return &Session{
		id:      generateSessionID(),
		store:   store,
		history: make([]Msg, 0),
	}
}

// AddMessage 添加消息到会话历史。
//
// 自动设置消息的 CreatedAt 时间戳（如果未设置），
// 然后持久化到 SessionStore。
//
// 参数：
//   - msg: 要添加的消息（用户消息、assistant 回复等）
func (s *Session) AddMessage(msg Msg) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg.CreatedAt == "" {
		msg.CreatedAt = nowISO()
	}
	s.history = append(s.history, msg)

	if s.store != nil {
		_ = s.store.Save(s.id, s.history)
	}
}

// GetHistory 获取会话的完整历史记录（副本）。
//
// 返回切片副本，修改返回的内容不会影响会话内部状态。
//
// 返回：
//   - []Msg: 历史消息列表副本
func (s *Session) GetHistory() []Msg {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]Msg, len(s.history))
	copy(result, s.history)
	return result
}

// GetHistoryExcludeMarks 获取排除指定标记的历史记录。
//
// /AgentScope 的 get_memory(mark) 方法：
// 构建模型输入时，排除已压缩（MsgMarkCompressed）的消息，
// 只保留摘要 + 未压缩的近期消息。
//
// 参数：
//   - excludeMarks: 要排除的标记列表
//
// 返回：
//   - []Msg: 过滤后的历史消息列表
func (s *Session) GetHistoryExcludeMarks(excludeMarks ...MsgMark) []Msg {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]Msg, 0, len(s.history))
	for _, msg := range s.history {
		excluded := false
		for _, mark := range excludeMarks {
			if msg.HasMark(mark) {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, msg)
		}
	}
	return result
}

// MarkMessagesCompressed 批量标记消息为已压缩。
//
// /AgentScope 的 mark_messages_compressed：
// 压缩完成后，标记旧消息为 Compressed，
// 构建模型输入时会自动排除这些消息。
//
// 参数：
//   - messageIDs: 要标记的消息 ID 列表
//
// 返回：
//   - int: 成功标记的消息数量
func (s *Session) MarkMessagesCompressed(messageIDs []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for i, msg := range s.history {
		for _, id := range messageIDs {
			if msg.ID == id {
				s.history[i].SetMark(MsgMarkCompressed)
				count++
				break
			}
		}
	}

	if s.store != nil && count > 0 {
		_ = s.store.Save(s.id, s.history)
	}

	return count
}

// AddHintMessage 添加提示消息（标记为 MsgMarkHint）。
func (s *Session) AddHintMessage(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg := Msg{
		ID:        generateID("hint"),
		Role:      RoleUser,
		Content:   []ContentBlock{NewTextBlock(content)},
		CreatedAt: nowISO(),
	}
	msg.SetMark(MsgMarkHint)
	s.history = append(s.history, msg)
}

// ClearHintMessages 清理所有 HINT 标记的消息。
func (s *Session) ClearHintMessages() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	filtered := make([]Msg, 0, len(s.history))
	for _, msg := range s.history {
		if !msg.HasMark(MsgMarkHint) {
			filtered = append(filtered, msg)
		} else {
			count++
		}
	}
	s.history = filtered
	return count
}

// Clear 清除会话全部历史。
//
// 清空内存中的历史记录，并将空列表持久化到存储。
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = make([]Msg, 0)
	if s.store != nil {
		_ = s.store.Save(s.id, s.history)
	}
}

// SetID 设置会话 ID。
//
// 参数：
//   - id: 新的会话 ID
func (s *Session) SetID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = id
}

// GetID 获取会话 ID。
//
// 返回：
//   - string: 当前会话的唯一标识符
func (s *Session) GetID() string {
	return s.id
}

// generateSessionID 生成新的会话 ID。
//
// 格式：sess_<YYYYMMDD_HHMMSS>
//
// 返回：
//   - string: 新的会话 ID
func generateSessionID() string {
	return generateID("sess")
}

// =============================================
// InMemorySessionStore — 内存会话存储
// =============================================

// InMemorySessionStore 会话的内存存储实现。
//
// 使用 map 存储会话，程序重启后数据会丢失。
// 适用于测试或无持久化需求的场景。
type InMemorySessionStore struct {
	sessions map[string][]Msg // key: sessionID, value: 消息列表
	mu       sync.Mutex       // 互斥锁，保证并发安全
}

// NewInMemorySessionStore 创建内存会话存储。
//
// 返回：
//   - *InMemorySessionStore: 初始化完成的内存存储
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string][]Msg),
	}
}

// Save 保存会话消息（实现 SessionStore 接口）。
//
// 参数：
//   - sessionID: 会话 ID
//   - messages: 消息列表
//
// 返回：
//   - error: 始终为 nil（内存操作不会失败）
func (s *InMemorySessionStore) Save(sessionID string, messages []Msg) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[sessionID] = messages
	return nil
}

// Load 加载会话消息（实现 SessionStore 接口）。
//
// 参数：
//   - sessionID: 会话 ID
//
// 返回：
//   - []Msg: 消息列表副本（会话不存在时返回 nil）
//   - error: 始终为 nil（内存操作不会失败）
func (s *InMemorySessionStore) Load(sessionID string) ([]Msg, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages, ok := s.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	result := make([]Msg, len(messages))
	copy(result, messages)
	return result, nil
}

// Delete 删除会话。
//
// 参数：
//   - sessionID: 要删除的会话 ID
//
// 返回：
//   - error: 始终为 nil
func (s *InMemorySessionStore) Delete(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}
