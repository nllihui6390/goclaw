package channel

import (
	"context"
	"net/http"
	"sync"
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
func (b *BotChannelBase) PushMessage(msg Message) {
	b.msgChan <- msg
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