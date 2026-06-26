package agent

import (
	"fmt"
	"sync"
	"time"
)

// =============================================
// 内置 StreamHandler 实现
// =============================================

// LoggingStreamHandler 日志流处理器
//
// 将流式文本增量输出到 slog logger，便于调试和审计。
// 适合需要完整对话审计日志的场景。
type LoggingStreamHandler struct {
	mu     sync.Mutex
	prefix string
}

// NewLoggingStreamHandler 创建日志流处理器。
//
// 参数：
//   - prefix: 日志前缀（如 "[Agent]"）
//
// 返回：
//   - *LoggingStreamHandler: 处理器实例
func NewLoggingStreamHandler(prefix string) *LoggingStreamHandler {
	if prefix == "" {
		prefix = "[Stream]"
	}
	return &LoggingStreamHandler{prefix: prefix}
}

// Handle 实现 StreamHandler 接口。
//
// 参数：
//   - chunk: 增量文本内容
func (h *LoggingStreamHandler) Handle(chunk string) {
	if chunk == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Printf("%s %s", h.prefix, chunk)
}

// MetricsStreamHandler 指标流处理器
//
// 累计流式文本的 token 数、字符数、耗时等指标，
// 可用于 Prometheus exporter 或内部监控。
type MetricsStreamHandler struct {
	mu            sync.Mutex
	totalChars    int
	totalTokens   int
	totalChunks   int
	startTime     time.Time
	lastChunkTime time.Time
	byRole        map[string]int // role -> chunk count
}

// NewMetricsStreamHandler 创建指标流处理器。
//
// 返回：
//   - *MetricsStreamHandler: 处理器实例
func NewMetricsStreamHandler() *MetricsStreamHandler {
	return &MetricsStreamHandler{
		byRole: make(map[string]int),
	}
}

// Handle 实现 StreamHandler 接口。
func (h *MetricsStreamHandler) Handle(chunk string) {
	if chunk == "" {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.startTime.IsZero() {
		h.startTime = time.Now()
	}
	h.lastChunkTime = time.Now()

	h.totalChars += len([]rune(chunk))
	// 简单 token 估算：中文 1 char/token，英文 4 chars/token
	h.totalTokens += estimateTokens(chunk)
	h.totalChunks++

	// 按角色统计
	for _, role := range []string{"assistant", "user", "system"} {
		if len(chunk) > 0 {
			h.byRole[role]++
			break
		}
	}
}

// GetMetrics 获取当前指标。
//
// 返回：
//   - totalChars: 总字符数
//   - totalTokens: 估算总 token 数
//   - totalChunks: 总 chunk 数
//   - elapsed: 流式处理耗时
func (h *MetricsStreamHandler) GetMetrics() (totalChars, totalTokens, totalChunks int, elapsed time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.totalChars, h.totalTokens, h.totalChunks, time.Since(h.startTime)
}

// Reset 重置所有指标。
func (h *MetricsStreamHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.totalChars = 0
	h.totalTokens = 0
	h.totalChunks = 0
	h.startTime = time.Time{}
	h.lastChunkTime = time.Time{}
	h.byRole = make(map[string]int)
}

// EventBridgeStreamHandler 事件桥接流处理器
//
// 将流式文本事件桥接到一个 channel，供外部消费者监听。
// 适合需要解耦流式输出的场景（如前端 SSE 推送）。
type EventBridgeStreamHandler struct {
	ch chan StreamEvent
	mu sync.Mutex
}

// StreamEvent 流式事件数据结构。
type StreamEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Chunk     string    `json:"chunk"`
	IsLast    bool      `json:"is_last"`
}

// NewEventBridgeStreamHandler 创建事件桥接处理器。
//
// 参数：
//   - bufferSize: channel 缓冲区大小（默认 100）
//
// 返回：
//   - *EventBridgeStreamHandler: 处理器实例
//   - <-chan StreamEvent: 事件 channel（只读）
func NewEventBridgeStreamHandler(bufferSize int) (*EventBridgeStreamHandler, <-chan StreamEvent) {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	bridge := &EventBridgeStreamHandler{
		ch: make(chan StreamEvent, bufferSize),
	}
	return bridge, bridge.ch
}

// Handle 实现 StreamHandler 接口。
func (h *EventBridgeStreamHandler) Handle(chunk string) {
	if chunk == "" {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	event := StreamEvent{
		Timestamp: time.Now(),
		Chunk:     chunk,
		IsLast:    false,
	}

	select {
	case h.ch <- event:
	default:
		// channel 满了，丢弃旧事件
		select {
		case <-h.ch:
		default:
		}
		h.ch <- event
	}
}

// Close 关闭 channel，标记最后一个事件。
func (h *EventBridgeStreamHandler) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	event := StreamEvent{
		Timestamp: time.Now(),
		IsLast:    true,
	}
	select {
	case h.ch <- event:
	default:
	}
	close(h.ch)
}

// estimateTokens 简单估算 chunk 的 token 数。
func estimateTokens(s string) int {
	runes := []rune(s)
	if len(runes) == 0 {
		return 0
	}
	// 保守估算：3 字符/token
	return len(runes)/3 + 1
}
