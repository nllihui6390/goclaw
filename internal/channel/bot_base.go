package channel

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go-claw/pkg/log"
)

// BotChannelBase 机器人渠道共享基础
type BotChannelBase struct {
	name             string
	port             string
	server           *http.Server
	msgChan          chan Message
	mu               sync.RWMutex
	pendingResponses map[string]chan Response
	client           *http.Client
	display          DisplayConfig // 显示控制配置
	stopped          atomic.Bool   // 是否已停止
	closed           atomic.Bool   // msgChan 是否已关闭（防止重复 close panic）
}

// NewBotChannelBase 创建机器人渠道基础
func NewBotChannelBase(name, port string, display DisplayConfig) *BotChannelBase {
	return &BotChannelBase{
		name:             name,
		port:             port,
		msgChan:          make(chan Message, 100),
		pendingResponses: make(map[string]chan Response),
		client:           &http.Client{Timeout: 10 * time.Second},
		display:          display,
	}
}

// GetDisplay 获取显示配置
func (b *BotChannelBase) GetDisplay() DisplayConfig {
	return b.display
}

func (b *BotChannelBase) GetName() string                                       { return b.name }
func (b *BotChannelBase) SetName(name string)                                   { b.name = name } // 设置渠道名（用于 per-agent 命名）
func (b *BotChannelBase) Receive(ctx context.Context) (<-chan Message, error)   { return b.msgChan, nil }

// StartHTTPServer 启动HTTP服务器
func (b *BotChannelBase) StartHTTPServer(handler http.HandlerFunc) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)
	mux.HandleFunc("/health", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("OK"))
	})

	b.server = &http.Server{Addr: ":" + b.port, Handler: mux}
	go func() {
		if err := b.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Logger().Error("Bot HTTP服务器错误", "channel", b.name, "err", err)
		}
	}()
	log.Logger().Info("Bot渠道已启动", "channel", b.name, "port", b.port)
	return nil
}

// StopHTTPServer 停止HTTP服务器
func (b *BotChannelBase) StopHTTPServer() error {
	if b.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return b.server.Shutdown(ctx)
	}
	return nil
}

// PushMessage 推送消息到msgChan
// 如果渠道已停止，返回 false 表示消息未被推送
func (b *BotChannelBase) PushMessage(msg Message) bool {
	if b.stopped.Load() {
		return false
	}
	b.msgChan <- msg
	return true
}

// Stop 停止渠道（关闭 msgChan 让 handleChannel 退出）
func (b *BotChannelBase) Stop() {
	// 1. 设置停止标志，阻止新消息进入
	b.stopped.Store(true)

	// 2. 关闭 msgChan（让 handleChannel goroutine 退出）
	//    使用 atomic 防止重复 close panic
	if !b.closed.Swap(true) {
		close(b.msgChan)
	}

	// 3. 停止 HTTP 服务器
	b.StopHTTPServer()
}

// IsStopped 检查渠道是否已停止
func (b *BotChannelBase) IsStopped() bool {
	return b.stopped.Load()
}

// RegisterPendingResponse 注册同步等待响应通道
func (b *BotChannelBase) RegisterPendingResponse(msgID string) chan Response {
	ch := make(chan Response, 1)
	b.mu.Lock()
	b.pendingResponses[msgID] = ch
	b.mu.Unlock()
	return ch
}

// CleanupPendingResponse 清理等待响应通道
func (b *BotChannelBase) CleanupPendingResponse(msgID string) {
	b.mu.Lock()
	delete(b.pendingResponses, msgID)
	b.mu.Unlock()
}

// SendToPendingResponse 发送响应到等待通道
func (b *BotChannelBase) SendToPendingResponse(resp Response) bool {
	b.mu.RLock()
	ch, exists := b.pendingResponses[resp.To]
	b.mu.RUnlock()
	if exists {
		ch <- resp
		return true
	}
	return false
}

// HTTPClient 返回共享HTTP客户端
func (b *BotChannelBase) HTTPClient() *http.Client {
	return b.client
}