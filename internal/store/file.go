package store

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"go-claw/pkg/log"
)

// FileStore 基于JSON文件的简单持久化存储
type FileStore struct {
	mu       sync.RWMutex
	sessions map[string]SessionData
	memories map[string]MemoryEntry
	path     string
}

func NewFileStore(path string) (*FileStore, error) {
	s := &FileStore{
		sessions: make(map[string]SessionData),
		memories: make(map[string]MemoryEntry),
		path:     path,
	}
	if err := s.load(); err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	return s, nil
}

func (s *FileStore) persist() error {
	data := map[string]interface{}{
		"sessions": s.sessions,
		"memories": s.memories,
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, out, 0644)
}

func (s *FileStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var store map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &store); err != nil {
		return err
	}
	if sessions, ok := store["sessions"]; ok {
		for id, raw := range sessions {
			var sess SessionData
			if err := json.Unmarshal(raw, &sess); err == nil {
				s.sessions[id] = sess
			}
		}
	}
	if memories, ok := store["memories"]; ok {
		for id, raw := range memories {
			var entry MemoryEntry
			if err := json.Unmarshal(raw, &entry); err == nil {
				s.memories[id] = entry
			}
		}
	}
	return nil
}

func (s *FileStore) SaveSession(_ context.Context, session SessionData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session.UpdatedAt = time.Now().Format(time.RFC3339)
	s.sessions[session.ID] = session
	if err := s.persist(); err != nil {
		log.Logger().Error("持久化会话失败", "err", err)
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
	if err := s.persist(); err != nil {
		log.Logger().Error("删除会话失败", "err", err)
	}
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
			changed = true
		}
	}
	if changed {
		return s.persist()
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
	if err := s.persist(); err != nil {
		log.Logger().Error("持久化记忆失败", "err", err)
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
	if err := s.persist(); err != nil {
		log.Logger().Error("删除记忆失败", "err", err)
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
		return s.persist()
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
