package memory

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// SimpleMemory 简单内存记忆实现
type SimpleMemory struct {
	mu         sync.RWMutex
	entries    map[string]MemoryEntry
	sessionIdx map[string][]string // sessionID -> entryIDs
	idCounter  int64
}

// NewSimpleMemory 创建简单记忆实例
func NewSimpleMemory() *SimpleMemory {
	m := &SimpleMemory{
		entries:    make(map[string]MemoryEntry),
		sessionIdx: make(map[string][]string),
	}

	// 启动定期记忆巩固和遗忘协程
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
		m.Forget(ctx, 0.1) // 遗忘重要性低于0.1的记忆
	}
}

// generateID 生成唯一ID
func (m *SimpleMemory) generateID() string {
	m.idCounter++
	hash := md5.Sum([]byte(fmt.Sprintf("%d-%d", time.Now().UnixNano(), m.idCounter)))
	return hex.EncodeToString(hash[:])[:16]
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

	// 更新索引
	if entry.SessionID != "" {
		if _, exists := m.sessionIdx[entry.SessionID]; !exists {
			m.sessionIdx[entry.SessionID] = []string{}
		}
		m.sessionIdx[entry.SessionID] = append(m.sessionIdx[entry.SessionID], entry.ID)
	}

	return nil
}

// Retrieve 检索相关记忆（使用简单的关键词匹配）
func (m *SimpleMemory) Retrieve(ctx context.Context, query string, sessionID string, limit int) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []SearchResult
	query = strings.ToLower(query)

	// 获取会话的所有记忆
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

	// 计算相关性评分
	for _, entry := range sessionEntries {
		score := m.calculateRelevance(query, entry)
		if score > 0 {
			results = append(results, SearchResult{
				Entry: entry,
				Score: score,
			})
		}
	}

	// 按评分排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// calculateRelevance 计算相关性（简化版：关键词匹配 + 时间衰减 + 重要性）
func (m *SimpleMemory) calculateRelevance(query string, entry MemoryEntry) float64 {
	var score float64 = 0

	// 1. 内容匹配度
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

	// 2. 重要性评分
	score += entry.Importance * 0.3

	// 3. 时间衰减（最近7天内有效）
	hoursSince := time.Since(entry.CreatedAt).Hours()
	timeDecay := 1.0
	if hoursSince > 168 { // 超过7天
		timeDecay = 0.5
	} else if hoursSince > 24 {
		timeDecay = 0.8
	}
	score *= timeDecay

	// 4. 访问频率加成
	if entry.AccessCount > 0 {
		score += float64(min(entry.AccessCount, 10)) / 100.0
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

	// 按时间倒序排序
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

	// 增加访问计数
	entry.AccessCount++
	entry.UpdatedAt = time.Now()
	m.mu.RUnlock()
	m.mu.Lock()
	m.entries[id] = entry
	m.mu.Unlock()
	m.mu.RLock()

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

	// 从会话索引中移除
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

	return nil
}

// Consolidate 记忆巩固（将短期记忆转为长期）
func (m *SimpleMemory) Consolidate(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, entry := range m.entries {
		// 如果记忆存在超过1小时，且类型为短期，且重要性>0.3，则转为长期
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
		// 遗忘长期且重要性低、超过30天未访问的记忆
		if entry.Type == "long_term" &&
			entry.Importance < threshold &&
			time.Since(entry.UpdatedAt).Hours() > 720 { // 30天
			toDelete = append(toDelete, id)
		}
		// 遗忘短期且超过24小时的记忆
		if entry.Type == "short_term" && time.Since(entry.CreatedAt).Hours() > 24 {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		m.Delete(ctx, id)
	}

	return nil
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
