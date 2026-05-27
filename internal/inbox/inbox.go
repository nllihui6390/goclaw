package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Severity 事件严重程度
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Entry 收件箱条目
type Entry struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`      // "heartbeat", "cron", "proactive", "system"
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Severity  Severity  `json:"severity"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Store 收件箱存储
type Store struct {
	filePath string
	entries  []Entry
	mu       sync.RWMutex
}

// NewStore 创建收件箱存储
func NewStore(dataDir string) *Store {
	filePath := filepath.Join(dataDir, "inbox.json")
	s := &Store{
		filePath: filePath,
		entries:  make([]Entry, 0),
	}
	s.load()
	return s
}

// load 从文件加载
func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.entries)
}

// save 保存到文件
func (s *Store) save() {
	data, _ := json.MarshalIndent(s.entries, "", "  ")
	os.WriteFile(s.filePath, data, 0644)
}

// Add 添加条目
func (s *Store) Add(entryType, title, content string, severity Severity, metadata map[string]interface{}) Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := Entry{
		ID:        generateID(),
		Type:      entryType,
		Title:     title,
		Content:   content,
		Severity:  severity,
		Read:      false,
		CreatedAt: time.Now(),
		Metadata:  metadata,
	}

	s.entries = append([]Entry{entry}, s.entries...)

	// 保留最近 100 条
	if len(s.entries) > 100 {
		s.entries = s.entries[:100]
	}

	s.save()
	return entry
}

// List 列出条目
func (s *Store) List(limit int) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.entries) {
		limit = len(s.entries)
	}

	result := make([]Entry, limit)
	copy(result, s.entries[:limit])
	return result
}

// ListUnread 列出未读条目
func (s *Store) ListUnread() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Entry
	for _, e := range s.entries {
		if !e.Read {
			result = append(result, e)
		}
	}
	return result
}

// MarkRead 标记为已读
func (s *Store) MarkRead(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, e := range s.entries {
		if e.ID == id {
			s.entries[i].Read = true
			s.save()
			return true
		}
	}
	return false
}

// MarkAllRead 标记全部已读
func (s *Store) MarkAllRead() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.entries {
		s.entries[i].Read = true
	}
	s.save()
}

// Delete 删除条目
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, e := range s.entries {
		if e.ID == id {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			s.save()
			return true
		}
	}
	return false
}

// Clear 清空收件箱
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = make([]Entry, 0)
	s.save()
}

// Count 获取条目总数
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// CountUnread 获取未读数量
func (s *Store) CountUnread() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, e := range s.entries {
		if !e.Read {
			count++
		}
	}
	return count
}

// generateID 生成唯一 ID
func generateID() string {
	return time.Now().Format("20060102150405") + "-" + randomSuffix()
}

func randomSuffix() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[time.Now().Nanosecond()%len(chars)]
	}
	return string(b)
}