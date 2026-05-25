package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// VectorMemory 基于Embedding向量的语义记忆
type VectorMemory struct {
	mu      sync.RWMutex
	entries []vectorEntry
	baseURL string
	apiKey  string
	model   string
	backend *SimpleMemory // 持久化后端
}

type vectorEntry struct {
	MemoryEntry
	Embedding []float64
}

// NewVectorMemory 创建向量记忆
func NewVectorMemory(baseURL, apiKey, model string, backend *SimpleMemory) *VectorMemory {
	return &VectorMemory{
		entries:   make([]vectorEntry, 0),
		baseURL:   baseURL,
		apiKey:    apiKey,
		model:     model,
		backend:   backend,
	}
}

// Store 存储记忆（生成向量）
func (vm *VectorMemory) Store(ctx context.Context, entry MemoryEntry) error {
	// 先生成向量
	embedding, err := vm.embedding(ctx, entry.Content)
	if err != nil {
		// 降级：不生成向量，使用文本
		embedding = make([]float64, 384)
	}

	ve := vectorEntry{
		MemoryEntry: entry,
		Embedding:   embedding,
	}

	vm.mu.Lock()
	defer vm.mu.Unlock()

	if ve.ID == "" {
		ve.ID = fmt.Sprintf("vec-%d", time.Now().UnixNano())
	}
	now := time.Now()
	ve.CreatedAt = now
	ve.UpdatedAt = now

	vm.entries = append(vm.entries, ve)

	// 同步到持久化后端
	if vm.backend != nil {
		vm.backend.Store(ctx, entry)
	}
	return nil
}

// Retrieve 语义检索：使用余弦相似度
func (vm *VectorMemory) Retrieve(ctx context.Context, query string, sessionID string, limit int) ([]SearchResult, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	// 生成查询向量
	queryVec, err := vm.embedding(ctx, query)
	if err != nil {
		// 降级到关键词匹配
		return vm.retrieveByKeyword(query, sessionID, limit)
	}

	var results []SearchResult
	for _, ve := range vm.entries {
		if sessionID != "" && ve.SessionID != "" && ve.SessionID != sessionID {
			continue
		}
		score := cosineSimilarity(queryVec, ve.Embedding)
		if score > 0.1 {
			results = append(results, SearchResult{
				Entry: ve.MemoryEntry,
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

func (vm *VectorMemory) retrieveByKeyword(query string, sessionID string, limit int) ([]SearchResult, error) {
	var results []SearchResult
	query = strings.ToLower(query)
	for _, ve := range vm.entries {
		if sessionID != "" && ve.SessionID != "" && ve.SessionID != sessionID {
			continue
		}
		content := strings.ToLower(ve.Content)
		words := strings.Fields(query)
		matched := 0
		for _, w := range words {
			if strings.Contains(content, w) {
				matched++
			}
		}
		if len(words) > 0 && matched > 0 {
			score := float64(matched) / float64(len(words))
			results = append(results, SearchResult{
				Entry: ve.MemoryEntry,
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

// GetRecent 获取最近的记忆
func (vm *VectorMemory) GetRecent(_ context.Context, sessionID string, limit int) ([]MemoryEntry, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	var entries []MemoryEntry
	for _, ve := range vm.entries {
		if sessionID == "" || ve.SessionID == sessionID {
			entries = append(entries, ve.MemoryEntry)
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

func (vm *VectorMemory) GetByID(_ context.Context, id string) (*MemoryEntry, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	for _, ve := range vm.entries {
		if ve.ID == id {
			return &ve.MemoryEntry, nil
		}
	}
	return nil, fmt.Errorf("记忆不存在: %s", id)
}

func (vm *VectorMemory) Update(_ context.Context, entry MemoryEntry) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	for i, ve := range vm.entries {
		if ve.ID == entry.ID {
			entry.UpdatedAt = time.Now()
			vm.entries[i].MemoryEntry = entry
			return nil
		}
	}
	return fmt.Errorf("记忆不存在: %s", entry.ID)
}

func (vm *VectorMemory) Delete(_ context.Context, id string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	for i, ve := range vm.entries {
		if ve.ID == id {
			vm.entries = append(vm.entries[:i], vm.entries[i+1:]...)
			return nil
		}
	}
	return nil
}

func (vm *VectorMemory) ClearSession(_ context.Context, sessionID string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	var kept []vectorEntry
	for _, ve := range vm.entries {
		if ve.SessionID != sessionID {
			kept = append(kept, ve)
		}
	}
	vm.entries = kept
	return nil
}

func (vm *VectorMemory) Consolidate(_ context.Context) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	now := time.Now()
	for i, ve := range vm.entries {
		if ve.Type == "short_term" &&
			now.Sub(ve.CreatedAt).Hours() > 1 &&
			ve.Importance > 0.3 {
			vm.entries[i].Type = "long_term"
			vm.entries[i].UpdatedAt = now
		}
	}
	return nil
}

func (vm *VectorMemory) Forget(_ context.Context, threshold float64) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	var kept []vectorEntry
	for _, ve := range vm.entries {
		if ve.Type == "long_term" &&
			ve.Importance < threshold &&
			time.Since(ve.UpdatedAt).Hours() > 720 {
			continue
		}
		if ve.Type == "short_term" && time.Since(ve.CreatedAt).Hours() > 24 {
			continue
		}
		kept = append(kept, ve)
	}
	vm.entries = kept
	return nil
}

// embedding 调用Embedding API生成向量
func (vm *VectorMemory) embedding(ctx context.Context, text string) ([]float64, error) {
	if vm.baseURL == "" || vm.apiKey == "" {
		// 无API配置，返回简单哈希向量作为降级
		return textToVector(text), nil
	}

	// 调用API（OpenAI-compatible格式）
	// 这里简单实现，实际应该用HTTP请求
	return textToVector(text), nil
}

// textToVector 简单文本到向量的降级实现（基于字符哈希）
func textToVector(text string) []float64 {
	dim := 384
	vec := make([]float64, dim)
	runes := []rune(strings.ToLower(text))
	for i, r := range runes {
		idx := (int(r) * 31 + i) % dim
		vec[idx] += 1.0
	}
	// 归一化
	norm := 0.0
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	dot, normA, normB := 0.0, 0.0, 0.0
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
