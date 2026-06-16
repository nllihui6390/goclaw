package token_usage

import (
	"sync"
	"time"

	glog "go-claw/pkg/log"
)

// Buffer 异步缓冲区，定期刷新到磁盘
type Buffer struct {
	storage      *Storage
	eventCh      chan UsageEvent
	diskCache    map[string]map[string]UsageEntry
	cacheMu      sync.RWMutex
	cacheLoaded  bool
	flushInterval time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewBuffer 创建缓冲区
func NewBuffer(storage *Storage) *Buffer {
	return &Buffer{
		storage:       storage,
		eventCh:       make(chan UsageEvent, 10000), // 缓冲队列
		diskCache:     make(map[string]map[string]UsageEntry),
		flushInterval: 10 * time.Second,
		stopCh:        make(chan struct{}),
	}
}

// Start 启动后台消费者和刷新任务
func (b *Buffer) Start() {
	// 同步加载磁盘缓存，避免与前端请求竞态导致空数据
	b.seedCache()
	b.wg.Add(2)
	go b.consumerLoop()
	go b.flushLoop()
	glog.Logger().Info("token_usage: 缓冲区已启动", "days_loaded", len(b.diskCache))
}

// Stop 停止缓冲区
func (b *Buffer) Stop() {
	close(b.stopCh)
	b.wg.Wait()
	// 最终刷新
	b.flushOnce()
	glog.Logger().Info("token_usage: 缓冲区已停止")
}

// Enqueue 放入事件（非阻塞，fire-and-forget）
func (b *Buffer) Enqueue(event UsageEvent) {
	select {
	case b.eventCh <- event:
	default:
		glog.Logger().Warn("token_usage: 队列已满，丢弃事件",
			"provider", event.ProviderID,
			"model", event.ModelName)
	}
}

// GetMergedData 获取合并后的数据视图（磁盘缓存 + 队列中待处理事件）
func (b *Buffer) GetMergedData() map[string]map[string]UsageEntry {
	b.cacheMu.RLock()
	defer b.cacheMu.RUnlock()

	// 深拷贝磁盘缓存
	result := make(map[string]map[string]UsageEntry)
	for date, dayData := range b.diskCache {
		result[date] = make(map[string]UsageEntry)
		for key, entry := range dayData {
			result[date][key] = entry
		}
	}

	return result
}

// consumerLoop 消费者循环，处理队列中的事件
func (b *Buffer) consumerLoop() {
	defer b.wg.Done()

	// 先加载磁盘缓存
	b.seedCache()

	for {
		select {
		case <-b.stopCh:
			// 停止前清空队列
			b.drainQueue()
			return
		case event := <-b.eventCh:
			b.applyEvent(event)
		}
	}
}

// flushLoop 定期刷新循环
func (b *Buffer) flushLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.flushOnce()
		}
	}
}

// seedCache 从磁盘加载缓存
func (b *Buffer) seedCache() {
	b.cacheMu.Lock()
	defer b.cacheMu.Unlock()

	if b.cacheLoaded {
		return
	}

	b.diskCache = b.storage.Load()
	b.cacheLoaded = true
	glog.Logger().Debug("token_usage: 缓存已从磁盘加载")
}

// applyEvent 将事件应用到缓存
func (b *Buffer) applyEvent(event UsageEvent) {
	b.cacheMu.Lock()
	defer b.cacheMu.Unlock()

	compositeKey := event.ProviderID + ":" + event.ModelName

	// 获取或创建日期桶
	dayBucket, ok := b.diskCache[event.DateStr]
	if !ok {
		dayBucket = make(map[string]UsageEntry)
		b.diskCache[event.DateStr] = dayBucket
	}

	// 获取或创建条目
	entry, ok := dayBucket[compositeKey]
	if !ok {
		entry = UsageEntry{
			ProviderID: event.ProviderID,
			ModelName:  event.ModelName,
		}
	}

	// 累加
	entry.PromptTokens += event.PromptTokens
	entry.CompletionTokens += event.CompletionTokens
	entry.CallCount++

	dayBucket[compositeKey] = entry
}

// drainQueue 清空队列中剩余的事件
func (b *Buffer) drainQueue() {
	for {
		select {
		case event := <-b.eventCh:
			b.applyEvent(event)
		default:
			return
		}
	}
}

// flushOnce 执行一次刷新
func (b *Buffer) flushOnce() {
	b.cacheMu.RLock()
	// 深拷贝
	snapshot := make(map[string]map[string]UsageEntry)
	for date, dayData := range b.diskCache {
		snapshot[date] = make(map[string]UsageEntry)
		for key, entry := range dayData {
			snapshot[date][key] = entry
		}
	}
	b.cacheMu.RUnlock()

	b.storage.Save(snapshot)
}