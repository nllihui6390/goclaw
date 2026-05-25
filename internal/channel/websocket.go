package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go-claw/pkg/log"
)

// WebSocketChannel WebSocket渠道
type WebSocketChannel struct {
	name     string
	port     string
	server   *http.Server
	msgChan  chan Message
	mu       sync.RWMutex
	upgrader websocket.Upgrader
	conns    map[string]*wsConn
}

type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// NewWebSocketChannel 创建WebSocket渠道
func NewWebSocketChannel(port string) *WebSocketChannel {
	return &WebSocketChannel{
		name:    "websocket",
		port:    port,
		msgChan: make(chan Message, 100),
		conns:   make(map[string]*wsConn),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (w *WebSocketChannel) GetName() string       { return w.name }
func (w *WebSocketChannel) Receive(ctx context.Context) (<-chan Message, error) { return w.msgChan, nil }

func (w *WebSocketChannel) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", w.handleWS)
	mux.HandleFunc("/health", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("OK"))
	})
	w.server = &http.Server{Addr: ":" + w.port, Handler: mux}
	go func() {
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Logger().Error("WebSocket服务器错误", "err", err)
		}
	}()
	log.Logger().Info("WebSocket渠道已启动", "port", w.port)
	return nil
}

func (w *WebSocketChannel) Stop() error {
	if w.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return w.server.Shutdown(ctx)
	}
	return nil
}

func (w *WebSocketChannel) Send(ctx context.Context, resp Response) error {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for sid, wc := range w.conns {
		if wc == nil {
			continue
		}
		if resp.To == "" || resp.To == sid {
			wc.mu.Lock()
			wc.conn.WriteJSON(map[string]string{"type": "response", "content": resp.Content})
			wc.mu.Unlock()
		}
	}
	return nil
}

func (w *WebSocketChannel) handleWS(rw http.ResponseWriter, r *http.Request) {
	conn, err := w.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		log.Logger().Error("WebSocket升级失败", "err", err)
		return
	}
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		sessionID = fmt.Sprintf("ws-%d", time.Now().UnixNano())
	}
	ws := &wsConn{conn: conn}
	w.mu.Lock()
	w.conns[sessionID] = ws
	w.mu.Unlock()
	log.Logger().Info("WebSocket连接已建立", "session", sessionID)
	defer func() {
		conn.Close()
		w.mu.Lock()
		delete(w.conns, sessionID)
		w.mu.Unlock()
	}()
	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var req struct {
			Content string `json:"content"`
			ID      string `json:"id,omitempty"`
		}
		if err := json.Unmarshal(msgBytes, &req); err != nil {
			conn.WriteJSON(map[string]string{"type": "error", "content": "无效的消息格式"})
			continue
		}
		msgID := req.ID
		if msgID == "" {
			msgID = fmt.Sprintf("ws-msg-%d", time.Now().UnixNano())
		}
		w.msgChan <- Message{
			ID: msgID, Channel: w.name, From: sessionID,
			Content: req.Content, Timestamp: time.Now().Unix(),
		}
	}
}
