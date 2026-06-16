package channel

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go-claw/pkg/log"
)

// ConsoleChannel Web 前端聊天渠道（经 Gateway 路由到 Agent）
type ConsoleChannel struct {
	name        string
	botPrefix   string
	msgChan     chan Message
	mu          sync.RWMutex
	responses   map[string]chan Response
	streamResps map[string]chan string
	fileEvents  map[string]chan ToolEvent
	display     DisplayConfig
	enabled     bool
	stopped     atomic.Bool
	closed      atomic.Bool
}

// NewConsoleChannel 创建 Console 渠道
func NewConsoleChannel(botPrefix string, display DisplayConfig) *ConsoleChannel {
	return &ConsoleChannel{
		name:        "console",
		botPrefix:   botPrefix,
		msgChan:     make(chan Message, 100),
		responses:   make(map[string]chan Response),
		streamResps: make(map[string]chan string),
		fileEvents:  make(map[string]chan ToolEvent),
		display:     display,
	}
}

func (w *ConsoleChannel) GetName() string                                     { return w.name }
func (w *ConsoleChannel) GetDisplay() DisplayConfig                           { return w.display }
func (w *ConsoleChannel) SetEnabled(v bool)                                   { w.enabled = v }
func (w *ConsoleChannel) IsEnabled() bool                                     { return w.enabled }
func (w *ConsoleChannel) IsStopped() bool                                     { return w.stopped.Load() }
func (w *ConsoleChannel) Receive(ctx context.Context) (<-chan Message, error) { return w.msgChan, nil }

// PushMessage 安全地发送消息到 msgChan
func (w *ConsoleChannel) PushMessage(msg Message) bool {
	if w.stopped.Load() {
		return false
	}
	defer func() {
		if recover() != nil {
		}
	}()
	log.Logger().Info("[Console] 收到消息", "msg_id", msg.ID, "agent", msg.Agent, "from", msg.From, "content", msg.Content)
	select {
	case w.msgChan <- msg:
		return true
	default:
		return false
	}
}

// PrepareStream 注册流式 SSE 响应通道，返回 cleanup 用于请求结束时清理
func (w *ConsoleChannel) PrepareStream(session string) (streamCh chan string, fileCh chan ToolEvent, cleanup func()) {
	streamCh = make(chan string, 32)
	fileCh = make(chan ToolEvent, 100) // 增大缓冲区，避免事件密集发送时丢失
	w.mu.Lock()
	w.streamResps[session] = streamCh
	w.fileEvents[session] = fileCh
	w.mu.Unlock()
	cleanup = func() {
		w.mu.Lock()
		if ch, exists := w.streamResps[session]; exists && ch == streamCh {
			delete(w.streamResps, session)
		}
		if fch, exists := w.fileEvents[session]; exists && fch == fileCh {
			delete(w.fileEvents, session)
		}
		w.mu.Unlock()
	}
	return streamCh, fileCh, cleanup
}

// PrepareBlocking 注册阻塞式响应通道
func (w *ConsoleChannel) PrepareBlocking(session string) (respCh chan Response, cleanup func()) {
	respCh = make(chan Response, 1)
	w.mu.Lock()
	w.responses[session] = respCh
	w.mu.Unlock()
	cleanup = func() {
		w.mu.Lock()
		delete(w.responses, session)
		w.mu.Unlock()
	}
	return respCh, cleanup
}

// Start 满足 Channel 接口；HTTP 路由由 server/controllers/api 统一注册
func (w *ConsoleChannel) Start(ctx context.Context) error {
	return nil
}

func (w *ConsoleChannel) Stop() error {
	w.stopped.Store(true)
	if !w.closed.Swap(true) {
		close(w.msgChan)
	}
	return nil
}

func (w *ConsoleChannel) Send(ctx context.Context, resp Response) error {
	resp.Content = ExtractFileBlockDescription(resp.Content)
	if w.botPrefix != "" {
		resp.Content = w.botPrefix + "  " + resp.Content
	}

	w.mu.Lock()
	ch, exists := w.responses[resp.To]
	streamCh := w.streamResps[resp.To]
	fileCh := w.fileEvents[resp.To]
	if streamCh != nil {
		delete(w.streamResps, resp.To)
		streamCh <- resp.Content
		close(streamCh)
	}
	// 关闭 fileCh 以确保 SSE handler 能通过 range 退出
	if fileCh != nil {
		delete(w.fileEvents, resp.To)
		close(fileCh)
	}
	w.mu.Unlock()

	if exists {
		select {
		case ch <- resp:
		case <-time.After(5 * time.Second):
		}
	}

	return nil
}

// SendProactive 向活跃 SSE 连接主动推送内容
func (w *ConsoleChannel) SendProactive(ctx context.Context, userID, content string) error {
	w.mu.RLock()
	streamCh := w.streamResps[userID]
	w.mu.RUnlock()

	if streamCh != nil {
		func() {
			defer func() {
				if recover() != nil {
				}
			}()
			select {
			case streamCh <- content:
			default:
			}
		}()
		return nil
	}

	log.Logger().Warn("[Console] 主动消息发送失败：无活跃连接", "user", userID)
	return fmt.Errorf("[Console] 无法主动发送消息给 %s（无活跃 SSE 连接）", userID)
}

// SendToolEvent 发送工具执行事件（所有事件类型经 SSE 实时推送）
func (w *ConsoleChannel) SendToolEvent(event ToolEvent) error {
	// 根据显示配置过滤事件
	if !w.display.ShouldShowToolEvent(event.Type) {
		return nil
	}

	w.mu.RLock()
	ch := w.fileEvents[event.To]
	w.mu.RUnlock()
	if ch != nil {
		// channel 可能已被 Send() 关闭，用 recover 防止 panic
		defer func() {
			if recover() != nil {
			}
		}()
		// 带超时的阻塞发送，避免缓冲区满时丢失事件
		select {
		case ch <- event:
		case <-time.After(5 * time.Second):
			// 超时丢弃（极端情况：SSE handler 卡住超过5秒）
		}
	}
	return nil
}
