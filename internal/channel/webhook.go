package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// WebhookChannel HTTP Webhook渠道
type WebhookChannel struct {
	name      string
	port      string
	server    *http.Server
	msgChan   chan Message
	mu        sync.RWMutex
	responses map[string]chan Response
}

// NewWebhookChannel 创建Webhook渠道
func NewWebhookChannel(port string) *WebhookChannel {
	return &WebhookChannel{
		name:      "webhook",
		port:      port,
		msgChan:   make(chan Message, 100),
		responses: make(map[string]chan Response),
	}
}

func (w *WebhookChannel) GetName() string {
	return w.name
}

func (w *WebhookChannel) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// 接收消息端点
	mux.HandleFunc("/webhook", w.handleWebhook)

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	w.server = &http.Server{
		Addr:    ":" + w.port,
		Handler: mux,
	}

	go func() {
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Webhook服务器错误: %v\n", err)
		}
	}()

	return nil
}

func (w *WebhookChannel) Stop() error {
	if w.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return w.server.Shutdown(ctx)
	}
	return nil
}

func (w *WebhookChannel) Receive(ctx context.Context) (<-chan Message, error) {
	return w.msgChan, nil
}

func (w *WebhookChannel) Send(ctx context.Context, resp Response) error {
	// Webhook渠道发送响应（如果有关联的响应通道）
	w.mu.RLock()
	ch, exists := w.responses[resp.To]
	w.mu.RUnlock()

	if exists {
		select {
		case ch <- resp:
		case <-time.After(5 * time.Second):
		}
	}

	return nil
}

func (w *WebhookChannel) handleWebhook(rw http.ResponseWriter, r *http.Request) {
	var req struct {
		User    string `json:"user"`
		Message string `json:"message"`
		ID      string `json:"id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request", http.StatusBadRequest)
		return
	}

	msgID := req.ID
	if msgID == "" {
		msgID = fmt.Sprintf("webhook-%d", time.Now().UnixNano())
	}

	// 创建响应通道
	respChan := make(chan Response, 1)
	w.mu.Lock()
	w.responses[msgID] = respChan
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		delete(w.responses, msgID)
		w.mu.Unlock()
	}()

	// 发送消息到Agent
	w.msgChan <- Message{
		ID:        msgID,
		Channel:   w.name,
		From:      req.User,
		Content:   req.Message,
		Timestamp: time.Now().Unix(),
	}

	// 等待响应
	select {
	case resp := <-respChan:
		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]string{
			"response": resp.Content,
		})
	case <-time.After(30 * time.Second):
		http.Error(rw, "Timeout", http.StatusGatewayTimeout)
	}
}
