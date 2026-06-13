package token_usage

import (
	"path/filepath"
	"sync"
	"time"

	glog "go-claw/pkg/log"
)

// Manager Token 使用量管理器（单例模式）
type Manager struct {
	buffer   *Buffer
	storage  *Storage
	mu       sync.RWMutex

	once     sync.Once
	stopCh   chan struct{}
}

var (
	instance    *Manager
	instMu      sync.Mutex
	// globalRecorder 全局记录函数（由外部设置，避免 import cycle）
	globalRecorder func(providerID, modelName string, promptTokens, completionTokens int)
)

// Init 初始化全局 Manager（在 bootstrap 时调用）
func Init(dataDir string) {
	instMu.Lock()
	defer instMu.Unlock()

	if instance != nil {
		return
	}

	path := filepath.Join(dataDir, "token_usage.json")
	storage := NewStorage(path)
	buffer := NewBuffer(storage)

	instance = &Manager{
		buffer:  buffer,
		storage: storage,
		stopCh:  make(chan struct{}),
	}

	// 启动缓冲区后台刷新
	instance.buffer.Start()

	// 设置全局记录函数
	globalRecorder = instance.Record

	glog.Logger().Info("token_usage: 管理器已初始化", "path", path)
}

// Record 全局记录函数（供 runtime 直接调用，不需要导入 global）
func Record(providerID, modelName string, promptTokens, completionTokens int) {
	if globalRecorder != nil {
		globalRecorder(providerID, modelName, promptTokens, completionTokens)
	}
}

// GetManager 获取全局单例 Manager
func GetManager() *Manager {
	instMu.Lock()
	defer instMu.Unlock()
	return instance
}

// Record 记录一次 LLM 调用的 Token 使用量（fire-and-forget，不阻塞）
func (m *Manager) Record(providerID, modelName string, promptTokens, completionTokens int) {
	event := UsageEvent{
		ProviderID:      providerID,
		ModelName:       modelName,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		DateStr:         time.Now().Format("2006-01-02"),
		Timestamp:       time.Now().Format(time.RFC3339),
	}
	m.buffer.Enqueue(event)
	glog.Logger().Debug("token_usage: 记录事件",
		"provider", providerID,
		"model", modelName,
		"prompt", promptTokens,
		"completion", completionTokens,
		"date", event.DateStr)
}

// GetDetails 获取原始 Token 使用记录列表（前端聚合用）
func (m *Manager) GetDetails(startDate, endDate string, modelName, providerID string) []TokenUsageRecord {
	merged := m.buffer.GetMergedData()

	records := []TokenUsageRecord{}
	current, _ := time.Parse("2006-01-02", startDate)
	end, _ := time.Parse("2006-01-02", endDate)

	if current.IsZero() || end.IsZero() {
		return records
	}

	for d := current; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		dayData, ok := merged[dateStr]
		if !ok {
			continue
		}

		for key, entry := range dayData {
			recModel := entry.ModelName
			if recModel == "" {
				recModel = key
			}
			// 过滤
			if modelName != "" && recModel != modelName {
				continue
			}
			if providerID != "" && entry.ProviderID != providerID {
				continue
			}

			records = append(records, TokenUsageRecord{
				Date:            dateStr,
				ProviderID:      entry.ProviderID,
				Model:           recModel,
				PromptTokens:     entry.PromptTokens,
				CompletionTokens: entry.CompletionTokens,
				CallCount:        entry.CallCount,
			})
		}
	}

	return records
}

// GetSummary 获取聚合的 Token 使用摘要
func (m *Manager) GetSummary(startDate, endDate string, modelName, providerID string) *TokenUsageSummary {
	records := m.GetDetails(startDate, endDate, modelName, providerID)

	summary := &TokenUsageSummary{
		ByModel: make(map[string]TokenUsageByModel),
		ByDate:  make(map[string]TokenUsageStats),
	}

	for _, r := range records {
		summary.TotalPromptTokens += r.PromptTokens
		summary.TotalCompletionTokens += r.CompletionTokens
		summary.TotalCalls += r.CallCount

		// 按模型聚合
		modelKey := r.ProviderID + ":" + r.Model
		if r.ProviderID == "" {
			modelKey = r.Model
		}
		if existing, ok := summary.ByModel[modelKey]; ok {
			existing.PromptTokens += r.PromptTokens
			existing.CompletionTokens += r.CompletionTokens
			existing.CallCount += r.CallCount
			summary.ByModel[modelKey] = existing
		} else {
			summary.ByModel[modelKey] = TokenUsageByModel{
				ProviderID:      r.ProviderID,
				Model:           r.Model,
				PromptTokens:     r.PromptTokens,
				CompletionTokens: r.CompletionTokens,
				CallCount:        r.CallCount,
			}
		}

		// 按日期聚合
		if existing, ok := summary.ByDate[r.Date]; ok {
			existing.PromptTokens += r.PromptTokens
			existing.CompletionTokens += r.CompletionTokens
			existing.CallCount += r.CallCount
			summary.ByDate[r.Date] = existing
		} else {
			summary.ByDate[r.Date] = TokenUsageStats{
				PromptTokens:     r.PromptTokens,
				CompletionTokens: r.CompletionTokens,
				CallCount:        r.CallCount,
			}
		}
	}

	return summary
}

// Stop 停止管理器（执行最终刷新）
func (m *Manager) Stop() {
	m.once.Do(func() {
		close(m.stopCh)
		m.buffer.Stop()
		glog.Logger().Info("token_usage: 管理器已停止")
	})
}