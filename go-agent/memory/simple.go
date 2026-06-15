package memory

import (
	"context"
	"strings"
	"sync"
	"time"
)

// SimpleMemory 简单内存实现（关键词匹配 + 内存存储）
type SimpleMemory struct {
	items    []MemoryItem
	maxItems int
	mu       sync.Mutex
}

// NewSimpleMemory 创建简单内存
func NewSimpleMemory() *SimpleMemory {
	return &SimpleMemory{
		items:    make([]MemoryItem, 0),
		maxItems: 1000,
	}
}

// NewSimpleMemoryWithCapacity 创建带容量限制的简单内存
func NewSimpleMemoryWithCapacity(maxItems int) *SimpleMemory {
	return &SimpleMemory{
		items:    make([]MemoryItem, 0),
		maxItems: maxItems,
	}
}

// Store 存储记忆
func (m *SimpleMemory) Store(ctx context.Context, key string, content string, memType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item := MemoryItem{
		ID:         generateMemoryID(),
		Content:    content,
		Type:       memType,
		Importance: calculateImportance(content),
		CreatedAt:  time.Now().Unix(),
	}

	m.items = append(m.items, item)

	// 容量限制
	if len(m.items) > m.maxItems {
		m.items = m.items[len(m.items)-m.maxItems:]
	}

	return nil
}

// Retrieve 检索记忆（关键词匹配）
func (m *SimpleMemory) Retrieve(ctx context.Context, query string, limit int) ([]MemoryItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 关键词提取
	keywords := extractKeywords(query)

	// 搜索匹配
	results := make([]ScoredMemoryItem, 0)
	for _, item := range m.items {
		score := keywordMatchScore(item.Content, keywords)
		if score > 0 {
			results = append(results, ScoredMemoryItem{
				MemoryItem: item,
				Score:      score,
			})
		}
	}

	// 按分数排序
	sortByScore(results)

	// 限制数量
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	// 转换结果
	items := make([]MemoryItem, len(results))
	for i, r := range results {
		items[i] = r.MemoryItem
	}

	return items, nil
}

// GetRecent 获取最近记忆
func (m *SimpleMemory) GetRecent(ctx context.Context, limit int) ([]MemoryItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limit <= 0 || limit > len(m.items) {
		limit = len(m.items)
	}

	// 返回最近的记忆
	start := len(m.items) - limit
	if start < 0 {
		start = 0
	}

	results := make([]MemoryItem, limit)
	copy(results, m.items[start:])

	return results, nil
}

// Consolidate 合并记忆
func (m *SimpleMemory) Consolidate(ctx context.Context, threshold float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.items {
		if m.items[i].Type == "short_term" && m.items[i].Importance >= threshold {
			m.items[i].Type = "long_term"
		}
	}

	return nil
}

// Forget 遗忘记忆
func (m *SimpleMemory) Forget(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, item := range m.items {
		if item.ID == id {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return nil
		}
	}

	return nil
}

// Clear 清除所有记忆
func (m *SimpleMemory) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make([]MemoryItem, 0)
	return nil
}

// GetAll 获取所有记忆
func (m *SimpleMemory) GetAll() []MemoryItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MemoryItem, len(m.items))
	copy(result, m.items)
	return result
}

// Count 记忆数量
func (m *SimpleMemory) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

// 辅助函数

func generateMemoryID() string {
	return "mem_" + time.Now().Format("20060102150405") + "_" + randomString(4)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}

func calculateImportance(content string) float64 {
	// 简单的重要性计算：基于长度和关键词
	length := len(content)
	if length > 500 {
		return 0.8
	} else if length > 200 {
		return 0.6
	} else if length > 50 {
		return 0.4
	}
	return 0.2
}

func extractKeywords(text string) []string {
	// 简单的关键词提取：分词并过滤短词
	words := strings.Fields(text)
	keywords := make([]string, 0)
	for _, word := range words {
		word = strings.TrimSpace(strings.ToLower(word))
		if len(word) >= 3 {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

func keywordMatchScore(content string, keywords []string) float64 {
	contentLower := strings.ToLower(content)
	score := 0.0
	for _, kw := range keywords {
		if strings.Contains(contentLower, kw) {
			score += 1.0
		}
	}
	return score
}

func sortByScore(items []ScoredMemoryItem) {
	// 简单排序（冒泡）
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].Score > items[i].Score {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}