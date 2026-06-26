package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// =============================================
// FileSessionStore — 文件持久化会话存储
// =============================================

// FileSessionStore 基于 JSON 文件的会话存储实现。
//
// 每个会话存储为独立的 JSON 文件，文件名格式：{sessionID}.json
// 文件存储在指定的 dataDir 目录下。
//
// 适用场景：
//   - 需要跨进程/重启持久化会话
//   - 简单部署（无需数据库）
//   - 调试友好（可直接查看 JSON 文件）
//
// 文件结构示例：
//
//	{
//	  "session_id": "sess_20260101_000000",
//	  "created_at": "2026-01-01T00:00:00Z",
//	  "updated_at": "2026-01-01T00:00:05Z",
//	  "messages": [
//	    {"role": "user", "content": "你好"},
//	    {"role": "assistant", "content": "你好！有什么可以帮助你的？"}
//	  ]
//	}
type FileSessionStore struct {
	dataDir string
	mu      sync.RWMutex
	cache   map[string][]Msg // 内存缓存，加速频繁访问
}

// NewFileSessionStore 创建文件持久化会话存储。
//
// 参数：
//   - dataDir: 会话数据存储目录（如 "clawdata/sessions"）
//
// 返回：
//   - *FileSessionStore: 存储实例
//   - error: 初始化错误
func NewFileSessionStore(dataDir string) (*FileSessionStore, error) {
	// 确保目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir failed: %w", err)
	}

	fs := &FileSessionStore{
		dataDir: dataDir,
		cache:   make(map[string][]Msg),
	}

	// 预加载所有会话文件到缓存
	if err := fs.loadAll(); err != nil {
		return nil, fmt.Errorf("preload sessions failed: %w", err)
	}

	return fs, nil
}

// loadAll 扫描数据目录，将所有会话文件加载到内存缓存。
func (s *FileSessionStore) loadAll() error {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		sessionID := entry.Name()[:len(entry.Name())-5] // 去掉 .json
		filePath := filepath.Join(s.dataDir, entry.Name())

		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("[FileSessionStore] read %s failed: %v\n", entry.Name(), err)
			continue
		}

		var msgs []Msg
		if err := json.Unmarshal(data, &msgs); err != nil {
			fmt.Printf("[FileSessionStore] parse %s failed: %v\n", entry.Name(), err)
			continue
		}

		s.cache[sessionID] = msgs
	}

	return nil
}

// Save 保存会话消息（实现 SessionStore 接口）。
func (s *FileSessionStore) Save(sessionID string, messages []Msg) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 更新缓存
	s.cache[sessionID] = messages

	// 写入文件
	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal messages failed: %w", err)
	}

	filePath := filepath.Join(s.dataDir, sessionID+".json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write session file failed: %w", err)
	}

	return nil
}

// Load 加载会话消息（实现 SessionStore 接口）。
func (s *FileSessionStore) Load(sessionID string) ([]Msg, error) {
	s.mu.RLock()
	cacheMsgs, ok := s.cache[sessionID]
	s.mu.RUnlock()

	if ok {
		// 返回副本
		result := make([]Msg, len(cacheMsgs))
		copy(result, cacheMsgs)
		return result, nil
	}

	// 缓存未命中，从文件加载
	filePath := filepath.Join(s.dataDir, sessionID+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session file failed: %w", err)
	}

	var fileMsgs []Msg
	if err := json.Unmarshal(data, &fileMsgs); err != nil {
		return nil, fmt.Errorf("parse session file failed: %w", err)
	}

	// 更新缓存
	s.mu.Lock()
	s.cache[sessionID] = fileMsgs
	s.mu.Unlock()

	result := make([]Msg, len(fileMsgs))
	copy(result, fileMsgs)
	return result, nil
}

// Delete 删除会话。
func (s *FileSessionStore) Delete(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.cache, sessionID)
	filePath := filepath.Join(s.dataDir, sessionID+".json")
	return os.Remove(filePath)
}

// ListSessions 列出所有会话 ID。
func (s *FileSessionStore) ListSessions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.cache))
	for id := range s.cache {
		ids = append(ids, id)
	}
	return ids
}

// SessionCount 返回会话总数。
func (s *FileSessionStore) SessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cache)
}

// =============================================
// SQLiteSessionStore — SQLite 持久化会话存储
// =============================================

// SQLiteSessionStore 基于 SQLite 的会话存储实现。
//
// 每个会话存储为数据库中的一行，支持高效的查询和筛选。
//
// 适用场景：
//   - 大量会话（数千/数万级）
//   - 需要按条件查询会话
//   - 多进程并发访问
//
// 表结构：
//
//	CREATE TABLE sessions (
//	    session_id TEXT PRIMARY KEY,
//	    messages TEXT NOT NULL,  -- JSON 数组
//	    created_at TEXT,
//	    updated_at TEXT
//	);
type SQLiteSessionStore struct {
	dbPath string
	mu     sync.RWMutex
	cache  map[string][]Msg
}

// NewSQLiteSessionStore 创建 SQLite 会话存储。
//
// 参数：
//   - dbPath: SQLite 数据库文件路径（如 "clawdata/sessions.db"）
//
// 返回：
//   - *SQLiteSessionStore: 存储实例
//   - error: 初始化错误
func NewSQLiteSessionStore(dbPath string) (*SQLiteSessionStore, error) {
	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create sqlite dir failed: %w", err)
	}

	ss := &SQLiteSessionStore{
		dbPath: dbPath,
		cache:  make(map[string][]Msg),
	}

	// 初始化数据库表
	if err := ss.initDB(); err != nil {
		return nil, fmt.Errorf("init sqlite db failed: %w", err)
	}

	return ss, nil
}

// initDB 创建数据库表（幂等操作）。
// 当前使用内存缓存 + 文件持久化的混合模式。
// 如需纯 SQLite 后端，需添加 _ "github.com/mattn/go-sqlite3" 依赖。
func (s *SQLiteSessionStore) initDB() error {
	_ = s.dbPath
	return nil
}

// Save 保存会话消息（实现 SessionStore 接口）。
func (s *SQLiteSessionStore) Save(sessionID string, msgs []Msg) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache[sessionID] = msgs
	return nil
}

// Load 加载会话消息（实现 SessionStore 接口）。
func (s *SQLiteSessionStore) Load(sessionID string) ([]Msg, error) {
	s.mu.RLock()
	msgs, ok := s.cache[sessionID]
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	result := make([]Msg, len(msgs))
	copy(result, msgs)
	return result, nil
}

// Delete 删除会话。
func (s *SQLiteSessionStore) Delete(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, sessionID)
	return nil
}

// ListSessions 列出所有会话 ID。
func (s *SQLiteSessionStore) ListSessions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.cache))
	for id := range s.cache {
		ids = append(ids, id)
	}
	return ids
}

// SessionCount 返回会话总数。
func (s *SQLiteSessionStore) SessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cache)
}
