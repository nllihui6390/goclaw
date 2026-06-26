package model

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// MemoryTokenRecorder 内存 Token 计数器
//
// 在进程内存中累计每次模型调用的 token 用量，
// 提供查询接口获取总计和各模型明细。
//
// 适用场景：短期对话、内存友好的 token 统计。
type MemoryTokenRecorder struct {
	mu          sync.RWMutex
	totalInput  int
	totalOutput int
	totalCalls  int
	byModel     map[string]*modelStats
}

type modelStats struct {
	modelName   string
	inputTokens int
	outputTokens int
	calls       int
}

// NewMemoryTokenRecorder 创建内存 Token 计数器。
//
// 返回：
//   - *MemoryTokenRecorder: 计数器实例
func NewMemoryTokenRecorder() *MemoryTokenRecorder {
	return &MemoryTokenRecorder{
		byModel: make(map[string]*modelStats),
	}
}

// Record 记录一次模型调用的 token 使用量。
//
// 参数：
//   - providerID: 提供商标识（如 "openai", "ollama"）
//   - modelName: 模型名称（如 "qwen3.7-plus"）
//   - inputTokens: 输入 token 数
//   - outputTokens: 输出 token 数
func (r *MemoryTokenRecorder) Record(providerID, modelName string, inputTokens, outputTokens int) {
	if inputTokens == 0 && outputTokens == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.totalInput += inputTokens
	r.totalOutput += outputTokens
	r.totalCalls++

	key := providerID + "/" + modelName
	stats, ok := r.byModel[key]
	if !ok {
		stats = &modelStats{modelName: modelName}
		r.byModel[key] = stats
	}
	stats.inputTokens += inputTokens
	stats.outputTokens += outputTokens
	stats.calls++
}

// GetTotals 获取总的 token 使用量。
//
// 返回：
//   - totalInput: 总输入 token
//   - totalOutput: 总输出 token
//   - totalCalls: 总调用次数
func (r *MemoryTokenRecorder) GetTotals() (totalInput, totalOutput, totalCalls int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.totalInput, r.totalOutput, r.totalCalls
}

// GetByModel 获取按模型分类的使用量。
//
// 返回：
//   - map[string]*modelStats: 模型名 -> 统计
func (r *MemoryTokenRecorder) GetByModel() map[string]*modelStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*modelStats, len(r.byModel))
	for k, v := range r.byModel {
		cp := *v
		result[k] = &cp
	}
	return result
}

// String 返回人类可读的 token 使用报告。
//
// 返回：
//   - string: 格式化的报告文本
func (r *MemoryTokenRecorder) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb string
	sb += "=== Token Usage Summary ===\n"
	sb += fmt.Sprintf("Total calls: %d\n", r.totalCalls)
	sb += fmt.Sprintf("Total input tokens:  %d\n", r.totalInput)
	sb += fmt.Sprintf("Total output tokens: %d\n", r.totalOutput)
	sb += fmt.Sprintf("Total tokens: %d\n", r.totalInput+r.totalOutput)

	if len(r.byModel) > 0 {
		sb += "\nBy model:\n"
		for key, stats := range r.byModel {
			sb += fmt.Sprintf("  %s: %d calls, %d input, %d output (%d total)\n",
				key, stats.calls, stats.inputTokens, stats.outputTokens,
				stats.inputTokens+stats.outputTokens)
		}
	}
	return sb
}

// FileTokenRecorder 文件持久化 Token 计数器
//
// 每次调用后追加到 JSON 文件，支持跨进程/重启恢复。
// 文件结构：
//
//	{
//	  "provider": "openai",
//	  "model": "qwen3.7-plus",
//	  "calls": [
//	    {"timestamp": "2026-01-01T00:00:00Z", "input_tokens": 100, "output_tokens": 50},
//	    ...
//	  ],
//	  "totals": {"input": 1000, "output": 500, "calls": 10}
//	}
type FileTokenRecorder struct {
	mu       sync.Mutex
	path     string
	data     *fileTokenData
	interval time.Duration
	lastSave time.Time
}

type fileTokenData struct {
	Provider string         `json:"provider"`
	Model    string         `json:"model"`
	Calls    []tokenCallLog `json:"calls"`
	Totals   tokenTotals    `json:"totals"`
}

type tokenCallLog struct {
	Timestamp    string `json:"timestamp"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

type tokenTotals struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Calls  int `json:"calls"`
}

// NewFileTokenRecorder 创建文件持久化 Token 计数器。
//
// 参数：
//   - path: JSON 文件路径（如 "clawdata/token_usage.json"）
//   - flushInterval: 写入间隔（默认 10 秒，0 表示每次调用立即写入）
//
// 返回：
//   - *FileTokenRecorder: 计数器实例
//   - error: 初始化错误
func NewFileTokenRecorder(path string, flushInterval time.Duration) (*FileTokenRecorder, error) {
	if flushInterval == 0 {
		flushInterval = 10 * time.Second
	}

	ft := &FileTokenRecorder{
		path:     path,
		interval: flushInterval,
		data:     &fileTokenData{Calls: make([]tokenCallLog, 0)},
	}

	// 尝试加载已有的 token 数据
	if err := ft.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load token usage from %s: %w", path, err)
	}

	return ft, nil
}

// load 从文件加载 token 数据。
func (r *FileTokenRecorder) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}
	var fd fileTokenData
	if err := json.Unmarshal(data, &fd); err != nil {
		return err
	}
	r.data = &fd
	return nil
}

// save 将 token 数据写入文件。
func (r *FileTokenRecorder) save() error {
	dir := r.data.Totals.Input // 使用 Totals 字段来推断目录
	_ = dir // 占位，避免未使用变量

	data, err := json.MarshalIndent(r.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0644)
}

// Record 记录一次模型调用的 token 使用量。
//
// 参数：
//   - providerID: 提供商标识（如 "openai", "ollama"）
//   - modelName: 模型名称（如 "qwen3.7-plus"）
//   - inputTokens: 输入 token 数
//   - outputTokens: 输出 token 数
func (r *FileTokenRecorder) Record(providerID, modelName string, inputTokens, outputTokens int) {
	if inputTokens == 0 && outputTokens == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.data.Provider = providerID
	r.data.Model = modelName

	call := tokenCallLog{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
	r.data.Calls = append(r.data.Calls, call)

	r.data.Totals.Input += inputTokens
	r.data.Totals.Output += outputTokens
	r.data.Totals.Calls++

	// 根据 flushInterval 决定是否写入文件
	if r.interval == 0 || time.Since(r.lastSave) >= r.interval {
		if err := r.save(); err != nil {
			fmt.Printf("[FileTokenRecorder] save failed: %v\n", err)
		}
		r.lastSave = time.Now()
	}
}

// GetTotals 获取总的 token 使用量。
//
// 返回：
//   - totalInput: 总输入 token
//   - totalOutput: 总输出 token
//   - totalCalls: 总调用次数
func (r *FileTokenRecorder) GetTotals() (totalInput, totalOutput, totalCalls int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.data.Totals.Input, r.data.Totals.Output, r.data.Totals.Calls
}

// Flush 强制将内存中的 token 数据写入文件。
func (r *FileTokenRecorder) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.save()
}

// GetByModel 获取按模型分类的使用量。
//
// 返回：
//   - map[string]*modelStats: 模型名 -> 统计
func (r *FileTokenRecorder) GetByModel() map[string]*modelStats {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make(map[string]*modelStats)
	for _, call := range r.data.Calls {
		key := r.data.Provider + "/" + r.data.Model
		stats, ok := result[key]
		if !ok {
			stats = &modelStats{modelName: r.data.Model}
			result[key] = stats
		}
		stats.calls++
		stats.inputTokens += call.InputTokens
		stats.outputTokens += call.OutputTokens
	}
	return result
}

// String 返回人类可读的 token 使用报告。
func (r *FileTokenRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return fmt.Sprintf("FileTokenRecorder{path:%s, calls:%d, input:%d, output:%d}",
		r.path, r.data.Totals.Calls, r.data.Totals.Input, r.data.Totals.Output)
}
