package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go-claw/utils"
)

// SessionIndexEntry 会话索引条目（前端会话管理直接读取）
type SessionIndexEntry struct {
	ID         string `json:"id"`          // UUID
	SessionID  string `json:"session_id"`  // 原始会话标识（channel:user_id）
	Name       string `json:"name"`        // 会话标题（用户第一句话）
	Channel    string `json:"channel"`     // 渠道名称（wecom/wechat/lark/webhook/console）
	UserID     string `json:"user_id"`     // 用户标识
	Agent      string `json:"agent"`       // 使用的 Agent 名称
	CreatedAt  string `json:"created_at"`  // RFC3339
	UpdatedAt  string `json:"updated_at"`  // RFC3339
}

// SessionIndex 会话索引（clawdata/sessions_index.json）
// 提供 channel:user → UUID 映射，前端会话管理直接读取此文件
type SessionIndex struct {
	mu       sync.RWMutex
	filePath string
	entries  map[string]*SessionIndexEntry // key = UUID
	chanMap  map[string]string             // channel:user → UUID 快速查找
}

// NewSessionIndex 打开或创建会话索引文件
func NewSessionIndex(dataDir string) (*SessionIndex, error) {
	os.MkdirAll(dataDir, 0755)
	filePath := filepath.Join(dataDir, "sessions_index.json")

	idx := &SessionIndex{
		filePath: filePath,
		entries:  make(map[string]*SessionIndexEntry),
		chanMap:  make(map[string]string),
	}

	data, err := os.ReadFile(filePath)
	if err == nil {
		var wrapper struct {
			Sessions []*SessionIndexEntry `json:"sessions"`
		}
		if err := json.Unmarshal(data, &wrapper); err == nil {
			for _, e := range wrapper.Sessions {
				idx.entries[e.ID] = e
				if e.SessionID != "" {
					idx.chanMap[e.SessionID] = e.ID
				}
			}
		}
	} else {
		// 文件不存在时创建空文件
		idx.persistLocked()
	}

	return idx, nil
}

// LookupOrCreate 根据 channel:user 查找或创建新会话
// 返回 UUID 和是否为新创建
func (idx *SessionIndex) LookupOrCreate(channel, user, agent string) (string, bool) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	chanKey := channel + ":" + user

	// 已有映射直接返回
	if uuid, ok := idx.chanMap[chanKey]; ok {
		return uuid, false
	}

	// 新建 UUID
	uuid := utils.UUID()
	now := time.Now().Format(time.RFC3339)

	entry := &SessionIndexEntry{
		ID:         uuid,
		SessionID:  chanKey,
		Channel:    channel,
		UserID:     user,
		Agent:      agent,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	idx.entries[uuid] = entry
	idx.chanMap[chanKey] = uuid
	idx.persistLocked()

	return uuid, true
}

// UpdateName 更新会话标题（第一条用户消息）
func (idx *SessionIndex) UpdateName(uuid, name, agent string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if e, ok := idx.entries[uuid]; ok {
		// 仅在首次设置标题
		if e.Name == "" {
			if len(name) > 50 {
				name = name[:50] + "..."
			}
			e.Name = name
		}
		e.Agent = agent
		e.UpdatedAt = time.Now().Format(time.RFC3339)
		idx.persistLocked()
	}
}

// EnsureEntry 确保 UUID 在索引中有记录（用于 webhook/console 等已生成 UUID 的渠道）
func (idx *SessionIndex) EnsureEntry(uuid, channel, user, agent string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if _, ok := idx.entries[uuid]; ok {
		return
	}
	now := time.Now().Format(time.RFC3339)
	idx.entries[uuid] = &SessionIndexEntry{
		ID: uuid, SessionID: channel + ":" + user,
		Channel: channel, UserID: user,
		Agent: agent, CreatedAt: now, UpdatedAt: now,
	}
	idx.persistLocked()
}

// Touch 更新时间戳
func (idx *SessionIndex) Touch(uuid string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if e, ok := idx.entries[uuid]; ok {
		e.UpdatedAt = time.Now().Format(time.RFC3339)
		idx.persistLocked()
	}
}

// List 返回所有会话条目（供前端会话管理 API）
func (idx *SessionIndex) List() []SessionIndexEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result := make([]SessionIndexEntry, 0, len(idx.entries))
	for _, e := range idx.entries {
		result = append(result, *e)
	}
	return result
}

// ListJSON 返回所有会话条目的 JSON 字符串
func (idx *SessionIndex) ListJSON() string {
	data, _ := json.Marshal(idx.List())
	return string(data)
}

// Delete 删除会话索引条目
func (idx *SessionIndex) Delete(uuid string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if e, ok := idx.entries[uuid]; ok {
		delete(idx.chanMap, e.SessionID)
		delete(idx.entries, uuid)
		idx.persistLocked()
	}
}

// persistLocked 写入磁盘（需持有写锁，直接构建列表避免 List() 的死锁）
func (idx *SessionIndex) persistLocked() {
	list := make([]SessionIndexEntry, 0, len(idx.entries))
	for _, e := range idx.entries {
		list = append(list, *e)
	}
	wrapper := struct {
		Sessions []SessionIndexEntry `json:"sessions"`
	}{Sessions: list}
	data, _ := json.MarshalIndent(wrapper, "", "  ")
	os.WriteFile(idx.filePath, data, 0644)
}

