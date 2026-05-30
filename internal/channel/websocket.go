package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go-claw/pkg/log"

	"github.com/gorilla/websocket"
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
	display  DisplayConfig // 显示控制配置
}

type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// NewWebSocketChannel 创建WebSocket渠道
func NewWebSocketChannel(port string, display DisplayConfig) *WebSocketChannel {
	return &WebSocketChannel{
		name:    "websocket",
		port:    port,
		msgChan: make(chan Message, 100),
		conns:   make(map[string]*wsConn),
		display: display,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (w *WebSocketChannel) GetName() string { return w.name }
func (w *WebSocketChannel) Receive(ctx context.Context) (<-chan Message, error) {
	return w.msgChan, nil
}

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
	// 处理文件发送
	if strings.Contains(resp.Content, "[FILE_BLOCK]") {
		fileInfo := ParseFileBlock(resp.Content)
		if fileInfo != nil {
			w.mu.RLock()
			defer w.mu.RUnlock()
			for sid, wc := range w.conns {
				if wc == nil {
					continue
				}
				if resp.To == "" || resp.To == sid {
					wc.mu.Lock()
					wc.conn.WriteJSON(map[string]interface{}{
						"type":     "file",
						"filename": fileInfo.Filename,
						"path":     fileInfo.Path,
						"size":     fileInfo.Size,
						"fileType": fileInfo.FileType,
					})
					wc.mu.Unlock()
				}
			}
			return nil
		}
	}

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

// SendFile 实现 FileSender 接口 - 直接发送文件
func (w *WebSocketChannel) SendFile(ctx context.Context, to string, info *FileBlockInfo) (bool, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for sid, wc := range w.conns {
		if wc == nil {
			continue
		}
		if to == "" || to == sid {
			wc.mu.Lock()
			wc.conn.WriteJSON(map[string]interface{}{
				"type":     "file",
				"filename": info.Filename,
				"path":     info.Path,
				"size":     info.Size,
				"fileType": info.FileType,
			})
			wc.mu.Unlock()
		}
	}

	log.Logger().Info("[WebSocket] 文件消息已发送", "to", to, "filename", info.Filename)
	return true, nil
}

// SendProactive 主动发送消息到指定 WebSocket 连接
func (w *WebSocketChannel) SendProactive(ctx context.Context, userID, content string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	wc, ok := w.conns[userID]
	if !ok {
		return fmt.Errorf("[WebSocket] 未找到连接 %s", userID)
	}

	wc.mu.Lock()
	err := wc.conn.WriteJSON(map[string]string{"type": "response", "content": content})
	wc.mu.Unlock()

	if err != nil {
		return fmt.Errorf("[WebSocket] 发送消息失败: %w", err)
	}

	log.Logger().Info("[WebSocket] 主动消息已发送", "session", userID)
	return nil
}

// SendToolEvent 发送工具执行事件（根据显示配置过滤）
func (w *WebSocketChannel) SendToolEvent(event ToolEvent) error {
	if !w.display.ShouldShowToolEvent(event.Type) {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, wc := range w.conns {
		if wc == nil {
			continue
		}
		wc.mu.Lock()
		wc.conn.WriteJSON(map[string]interface{}{
			"type":       "tool_event",
			"event_type": event.Type,
			"tool_name":  event.ToolName,
			"args":       event.Args,
			"result":     event.Result,
			"error":      event.Error,
			"thinking":   event.Thinking,
		})
		wc.mu.Unlock()
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
