package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	glog "go-claw/pkg/log"
)

// FileStore 于目录的持久化存储
// sessions/ 目录下每个会话一个 JSON 文件
// sessions/memories.json 存放所有记忆数据
type FileStore struct {
	mu       sync.RWMutex
	sessions map[string]SessionData
	memories map[string]MemoryEntry
	sessDir  string // sessions 目录路径
	memFile  string // memories.json 文件路径（在 sessions 目录内）
}

func NewFileStore(sessDir string) (*FileStore, error) {
	logger := glog.Logger()
	s := &FileStore{
		sessions: make(map[string]SessionData),
		memories: make(map[string]MemoryEntry),
		sessDir:  sessDir,
		memFile:  filepath.Join(sessDir, "memories.json"),
	}

	// 确保 sessions 目录存在
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		return nil, fmt.Errorf("创建 sessions 目录失败: %v", err)
	}

	// 加载已有会话文件
	files, err := os.ReadDir(sessDir)
	if err != nil {
		logger.Warn("读取 sessions 目录失败", "err", err)
	} else {
		for _, f := range files {
			if !f.IsDir() && filepath.Ext(f.Name()) == ".json" {
				data, err := os.ReadFile(filepath.Join(sessDir, f.Name()))
				if err != nil {
					continue
				}
				var sess SessionData
				if err := json.Unmarshal(data, &sess); err != nil {
					continue
				}
				s.sessions[sess.ID] = sess
			}
		}
	}

	// 加载记忆文件
	memData, err := os.ReadFile(s.memFile)
	if err == nil {
		var memList map[string]MemoryEntry
		if err := json.Unmarshal(memData, &memList); err == nil {
			s.memories = memList
		}
	}

	logger.Info("存储已初始化", "sessions_dir", sessDir, "sessions_loaded", len(s.sessions), "memories_loaded", len(s.memories))
	return s, nil
}

func (s *FileStore) persistSession(sess SessionData) error {
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	// 会话ID中可能包含冒号，替换为下划线作为文件名
	fileName := safeFileName(sess.ID) + ".json"
	return os.WriteFile(filepath.Join(s.sessDir, fileName), data, 0644)
}

func (s *FileStore) persistMemories() error {
	data, err := json.MarshalIndent(s.memories, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.memFile, data, 0644)
}

func safeFileName(id string) string {
	result := make([]byte, 0, len(id))
	for _, c := range id {
		if c == ':' || c == '/' || c == '\\' || c == ' ' {
			result = append(result, '_')
		} else {
			result = append(result, byte(c))
		}
	}
	return string(result)
}

func (s *FileStore) SaveSession(_ context.Context, session SessionData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session.UpdatedAt = time.Now().Format(time.RFC3339)
	s.sessions[session.ID] = session
	if err := s.persistSession(session); err != nil {
		glog.Logger().Error("持久化会话失败", "err", err)
	}
	return nil
}

func (s *FileStore) GetSession(_ context.Context, id string) (*SessionData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, exists := s.sessions[id]
	if !exists {
		return nil, os.ErrNotExist
	}
	return &sess, nil
}

func (s *FileStore) ListSessions(_ context.Context) ([]SessionData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []SessionData
	for _, sess := range s.sessions {
		list = append(list, sess)
	}
	return list, nil
}

func (s *FileStore) DeleteSession(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	fileName := safeFileName(id) + ".json"
	os.Remove(filepath.Join(s.sessDir, fileName))
	return nil
}

func (s *FileStore) CleanupExpiredSessions(_ context.Context, ttlMinutes int) error {
	if ttlMinutes <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-time.Duration(ttlMinutes) * time.Minute)
	changed := false
	for id, sess := range s.sessions {
		if t, err := time.Parse(time.RFC3339, sess.UpdatedAt); err == nil && t.Before(cutoff) {
			delete(s.sessions, id)
			fileName := safeFileName(id) + ".json"
			os.Remove(filepath.Join(s.sessDir, fileName))
			changed = true
		}
	}
	if changed {
		glog.Logger().Info("清理过期会话", "removed", changed)
	}
	return nil
}

func (s *FileStore) SaveMemory(_ context.Context, entry MemoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		entry.ID = entry.Content[0:minInt(32, len(entry.Content))]
	}
	entry.UpdatedAt = time.Now().Format(time.RFC3339)
	s.memories[entry.ID] = entry
	if err := s.persistMemories(); err != nil {
		glog.Logger().Error("持久化记忆失败", "err", err)
	}
	return nil
}

func (s *FileStore) ListMemories(_ context.Context, sessionID string, limit int) ([]MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []MemoryEntry
	for _, e := range s.memories {
		if sessionID == "" || e.SessionID == sessionID {
			list = append(list, e)
		}
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (s *FileStore) DeleteMemory(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.memories, id)
	if err := s.persistMemories(); err != nil {
		glog.Logger().Error("删除记忆失败", "err", err)
	}
	return nil
}

func (s *FileStore) ClearSessionMemories(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for id, e := range s.memories {
		if e.SessionID == sessionID {
			delete(s.memories, id)
			changed = true
		}
	}
	if changed {
		return s.persistMemories()
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}