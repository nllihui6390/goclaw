package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go-claw/internal/store"
	"go-claw/pkg/log"
)

// SimpleMemory 简单内存记忆实现
type SimpleMemory struct {
	mu         sync.RWMutex
	entries    map[string]MemoryEntry
	sessionIdx map[string][]string
	idCounter  int64
	st         store.Store
}

// NewSimpleMemory 创建简单记忆实例
func NewSimpleMemory(st store.Store) *SimpleMemory {
	m := &SimpleMemory{
		entries:    make(map[string]MemoryEntry),
		sessionIdx: make(map[string][]string),
		st:         st,
	}

	go m.maintenanceLoop()

	return m
}

// maintenanceLoop 定期维护循环
func (m *SimpleMemory) maintenanceLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		m.Consolidate(ctx)
		m.Forget(ctx, 0.1)
	}
}

// generateID 生成唯一ID
func (m *SimpleMemory) generateID() string {
	m.idCounter++
	return fmt.Sprintf("mem-%d-%d", time.Now().UnixNano(), m.idCounter)
}

func toStoreEntry(e MemoryEntry) store.MemoryEntry {
	return store.MemoryEntry{
		ID:          e.ID,
		Content:     e.Content,
		Type:        e.Type,
		SessionID:   e.SessionID,
		UserID:      e.UserID,
		Metadata:    e.Metadata,
		CreatedAt:   e.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   e.UpdatedAt.Format(time.RFC3339),
		AccessCount: e.AccessCount,
		Importance:  e.Importance,
	}
}

func toMemoryEntry(e store.MemoryEntry) (MemoryEntry, error) {
	var createdAt, updatedAt time.Time
	var err error
	if e.CreatedAt != "" {
		createdAt, err = time.Parse(time.RFC3339, e.CreatedAt)
		if err != nil {
			return MemoryEntry{}, err
		}
	}
	if e.UpdatedAt != "" {
		updatedAt, err = time.Parse(time.RFC3339, e.UpdatedAt)
		if err != nil {
			return MemoryEntry{}, err
		}
	}
	return MemoryEntry{
		ID:          e.ID,
		Content:     e.Content,
		Type:        e.Type,
		SessionID:   e.SessionID,
		UserID:      e.UserID,
		Metadata:    e.Metadata,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		AccessCount: e.AccessCount,
		Importance:  e.Importance,
	}, nil
}

// Store 存储记忆
func (m *SimpleMemory) Store(ctx context.Context, entry MemoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.ID == "" {
		entry.ID = m.generateID()
	}

	now := time.Now()
	entry.CreatedAt = now
	entry.UpdatedAt = now

	m.entries[entry.ID] = entry

	if entry.SessionID != "" {
		if _, exists := m.sessionIdx[entry.SessionID]; !exists {
			m.sessionIdx[entry.SessionID] = []string{}
		}
		m.sessionIdx[entry.SessionID] = append(m.sessionIdx[entry.SessionID], entry.ID)
	}

	if m.st != nil {
		if err := m.st.SaveMemory(ctx, toStoreEntry(entry)); err != nil {
			log.Logger().Error("保存记忆失败", "err", err)
		}
	}

	return nil
}

// Retrieve 检索相关记忆
func (m *SimpleMemory) Retrieve(ctx context.Context, query string, sessionID string, limit int) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []SearchResult
	query = strings.ToLower(query)

	var sessionEntries []MemoryEntry
	if sessionID != "" {
		if ids, exists := m.sessionIdx[sessionID]; exists {
			for _, id := range ids {
				if entry, ok := m.entries[id]; ok {
					sessionEntries = append(sessionEntries, entry)
				}
			}
		}
	} else {
		for _, entry := range m.entries {
			sessionEntries = append(sessionEntries, entry)
		}
	}

	for _, entry := range sessionEntries {
		score := m.calculateRelevance(query, entry)
		if score > 0 {
			results = append(results, SearchResult{
				Entry: entry,
				Score: score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (m *SimpleMemory) calculateRelevance(query string, entry MemoryEntry) float64 {
	var score float64

	content := strings.ToLower(entry.Content)
	words := strings.Fields(query)
	matchedWords := 0
	for _, word := range words {
		if strings.Contains(content, word) {
			matchedWords++
		}
	}
	if len(words) > 0 {
		score += float64(matchedWords) / float64(len(words)) * 0.6
	}

	score += entry.Importance * 0.3

	hoursSince := time.Since(entry.CreatedAt).Hours()
	timeDecay := 1.0
	if hoursSince > 168 {
		timeDecay = 0.5
	} else if hoursSince > 24 {
		timeDecay = 0.8
	}
	score *= timeDecay

	if entry.AccessCount > 0 {
		score += float64(minInt(entry.AccessCount, 10)) / 100.0
	}

	return score
}

// GetRecent 获取最近的记忆
func (m *SimpleMemory) GetRecent(ctx context.Context, sessionID string, limit int) ([]MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var entries []MemoryEntry

	if sessionID != "" {
		if ids, exists := m.sessionIdx[sessionID]; exists {
			for _, id := range ids {
				if entry, ok := m.entries[id]; ok {
					entries = append(entries, entry)
				}
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	return entries, nil
}

// GetByID 根据ID获取记忆
func (m *SimpleMemory) GetByID(ctx context.Context, id string) (*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.entries[id]
	if !exists {
		return nil, fmt.Errorf("记忆不存在: %s", id)
	}

	entry.AccessCount++
	entry.UpdatedAt = time.Now()
	m.entries[id] = entry

	return &entry, nil
}

// Update 更新记忆
func (m *SimpleMemory) Update(ctx context.Context, entry MemoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[entry.ID]; !exists {
		return fmt.Errorf("记忆不存在: %s", entry.ID)
	}

	entry.UpdatedAt = time.Now()
	m.entries[entry.ID] = entry

	return nil
}

// Delete 删除记忆
func (m *SimpleMemory) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.entries[id]
	if !exists {
		return nil
	}

	if entry.SessionID != "" {
		ids := m.sessionIdx[entry.SessionID]
		for i, eid := range ids {
			if eid == id {
				m.sessionIdx[entry.SessionID] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
	}

	delete(m.entries, id)

	if m.st != nil {
		if err := m.st.DeleteMemory(ctx, id); err != nil {
			log.Logger().Error("删除持久化记忆失败", "err", err)
		}
	}

	return nil
}

// ClearSession 清除会话记忆
func (m *SimpleMemory) ClearSession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ids, exists := m.sessionIdx[sessionID]; exists {
		for _, id := range ids {
			delete(m.entries, id)
		}
		delete(m.sessionIdx, sessionID)
	}

	if m.st != nil {
		if err := m.st.ClearSessionMemories(ctx, sessionID); err != nil {
			log.Logger().Error("清除持久化记忆失败", "err", err)
		}
	}

	return nil
}

// Consolidate 记忆巩固
func (m *SimpleMemory) Consolidate(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, entry := range m.entries {
		if entry.Type == "short_term" &&
			now.Sub(entry.CreatedAt).Hours() > 1 &&
			entry.Importance > 0.3 {
			entry.Type = "long_term"
			entry.UpdatedAt = now
			m.entries[id] = entry
		}
	}

	return nil
}

// Forget 遗忘不重要的记忆
func (m *SimpleMemory) Forget(ctx context.Context, threshold float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	toDelete := []string{}
	for id, entry := range m.entries {
		if entry.Type == "long_term" &&
			entry.Importance < threshold &&
			time.Since(entry.UpdatedAt).Hours() > 720 {
			toDelete = append(toDelete, id)
		}
		if entry.Type == "short_term" && time.Since(entry.CreatedAt).Hours() > 24 {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		delete(m.entries, id)
		if entry, ok := m.entries[id]; ok && entry.SessionID != "" {
			ids := m.sessionIdx[entry.SessionID]
			for i, eid := range ids {
				if eid == id {
					m.sessionIdx[entry.SessionID] = append(ids[:i], ids[i+1:]...)
					break
				}
			}
		}
	}

	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
