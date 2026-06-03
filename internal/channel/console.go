package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-claw/internal/store"
	"go-claw/pkg/log"
)

// ConsoleChannel HTTP Webhook渠道
type ConsoleChannel struct {
	name        string
	port        string
	server      *http.Server
	msgChan     chan Message
	mu          sync.RWMutex
	responses   map[string]chan Response
	streamResps map[string]chan string
	authToken   string
	display     DisplayConfig // 显示控制配置
	enabled     bool          // 是否允许聊天

	// Metrics
	reqCount   int64
	errorCount int64
	avgLatency time.Duration
	metricsMu  sync.RWMutex
}

// NewConsoleChannel 创建Webhook渠道
func NewConsoleChannel(port, authToken string, display DisplayConfig) *ConsoleChannel {
	return &ConsoleChannel{
		name:        "console",
		port:        port,
		msgChan:     make(chan Message, 100),
		responses:   make(map[string]chan Response),
		streamResps: make(map[string]chan string),
		authToken:   authToken,
		display:     display,
	}
}

func (w *ConsoleChannel) GetName() string                                        { return w.name }
func (w *ConsoleChannel) SetEnabled(v bool)                                       { w.enabled = v }
func (w *ConsoleChannel) Receive(ctx context.Context) (<-chan Message, error)    { return w.msgChan, nil }

func (w *ConsoleChannel) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/chat", w.handleChat)
	mux.HandleFunc("/api/v1/sessions", w.handleSessions)
	mux.HandleFunc("/api/v1/sessions/", w.handleSessionByID)
	mux.HandleFunc("/webhook", w.handleWebhook)
	mux.HandleFunc("/health", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("OK"))
	})
	mux.HandleFunc("/metrics", w.handleMetrics)

	w.server = &http.Server{
		Addr:    ":" + w.port,
		Handler: w.authMiddleware(mux),
	}

	go func() {
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Logger().Error("Webhook服务器错误", "err", err)
		}
	}()

	log.Logger().Info("Webhook渠道已启动", "port", w.port)
	return nil
}

func (w *ConsoleChannel) Stop() error {
	if w.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return w.server.Shutdown(ctx)
	}
	return nil
}

func (w *ConsoleChannel) Send(ctx context.Context, resp Response) error {
	// 处理文件发送：在响应中标记文件信息
	resp.Content = ExtractFileBlockDescription(resp.Content)

	w.mu.Lock()
	ch, exists := w.responses[resp.To]
	streamCh := w.streamResps[resp.To]
	if streamCh != nil {
		// 流式模式：发送内容后关闭 channel，通知 SSE 循环结束
		delete(w.streamResps, resp.To)
		streamCh <- resp.Content
		close(streamCh)
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

// SendProactive 主动发送消息（Webhook 是请求-响应模式，无法主动推送）
func (w *ConsoleChannel) SendProactive(ctx context.Context, userID, content string) error {
	w.mu.RLock()
	streamCh := w.streamResps[userID]
	w.mu.RUnlock()

	if streamCh != nil {
		// 使用 recover 处理竞态：Send() 可能已关闭 channel
		func() {
			defer func() {
				if r := recover(); r != nil {
					// channel 已被 Send() 关闭，流已结束，忽略此次发送
				}
			}()
			select {
			case streamCh <- content:
			default:
			}
		}()
		return nil
	}

	log.Logger().Warn("[Webhook] 主动消息发送失败：无活跃连接", "user", userID)
	return fmt.Errorf("[Webhook] 无法主动发送消息给 %s（无活跃 SSE 连接）", userID)
}

// SendToolEvent 发送工具执行事件（webhook暂不支持实时输出，忽略）
func (w *ConsoleChannel) SendToolEvent(event ToolEvent) error {
	// Webhook 是请求-响应模式，无法实时推送工具事件
	// 如需支持，可通过 SSE stream 推送
	return nil
}

func (w *ConsoleChannel) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if w.authToken == "" {
			next.ServeHTTP(rw, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != w.authToken {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(rw).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(rw, r)
	})
}

func (w *ConsoleChannel) writeJSON(rw http.ResponseWriter, status int, data any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	json.NewEncoder(rw).Encode(data)
}

func (w *ConsoleChannel) writeError(rw http.ResponseWriter, status int, msg string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	json.NewEncoder(rw).Encode(map[string]string{"error": msg})
}

// chatRequest 请求体
type chatRequest struct {
	Session string `json:"session"`
	Content string `json:"content"`
	Agent   string `json:"agent,omitempty"`  // 指定目标Agent
	Stream  bool   `json:"stream"`
}

// handleChat POST /api/v1/chat
// HandleChat 公开的 chat handler（供外部 mux 注册）
func (w *ConsoleChannel) HandleChat(rw http.ResponseWriter, r *http.Request) {
	w.handleChat(rw, r)
}

func (w *ConsoleChannel) handleChat(rw http.ResponseWriter, r *http.Request) {
	if !w.enabled {
		w.writeError(rw, http.StatusServiceUnavailable, "console channel is disabled")
		return
	}
	if r.Method != http.MethodPost {
		w.writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.writeError(rw, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Content == "" {
		w.writeError(rw, http.StatusBadRequest, "content is required")
		return
	}

	// 也支持 X-Agent 请求头
	if req.Agent == "" {
		req.Agent = r.Header.Get("X-Agent")
	}

	msgID := fmt.Sprintf("rest-%d", time.Now().UnixNano())
	if req.Session == "" {
		req.Session = fmt.Sprintf("rest-user-%d", time.Now().UnixNano())
	}

	w.sendToAgent(msgID, req.Session, req.Content, req.Agent, req.Stream, rw, r)
}

func (w *ConsoleChannel) sendToAgent(msgID, session, content, agentName string, stream bool, rw http.ResponseWriter, r *http.Request) {
	if stream {
		streamCh := make(chan string, 32)
		w.mu.Lock()
		w.streamResps[session] = streamCh // 使用 session 匹配 gateway resp.To
		w.mu.Unlock()

		// 确保 SSE 循环退出时清理 streamResps（处理超时/客户端断开等异常情况）
		defer func() {
			w.mu.Lock()
			if ch, exists := w.streamResps[session]; exists && ch == streamCh {
				delete(w.streamResps, session)
			}
			w.mu.Unlock()
		}()

		go func() {
			w.msgChan <- Message{
				ID: msgID, Channel: w.name, From: session,
				Content: content, Agent: agentName, Timestamp: time.Now().Unix(),
			}
		}()

		rw.Header().Set("Content-Type", "text/event-stream")
		rw.Header().Set("Cache-Control", "no-cache")
		rw.Header().Set("Connection", "keep-alive")
		rw.WriteHeader(http.StatusOK)
		flusher := rw.(http.Flusher)

		fmt.Fprintf(rw, "event: start\ndata: {\"session\":\"%s\",\"id\":\"%s\"}\n\n", session, msgID)
		flusher.Flush()

		timeout := time.After(120 * time.Second)
		for {
			select {
			case content, ok := <-streamCh:
				if !ok {
					fmt.Fprintf(rw, "event: done\ndata: {}\n\n")
					flusher.Flush()
					return
				}
				data, _ := json.Marshal(map[string]string{"content": content})
				fmt.Fprintf(rw, "event: chunk\ndata: %s\n\n", data)
				flusher.Flush()
			case <-timeout:
				fmt.Fprintf(rw, "event: error\ndata: {\"error\":\"timeout\"}\n\n")
				flusher.Flush()
				return
			case <-r.Context().Done():
				return
			}
		}
	}

	// 阻塞模式
	respChan := make(chan Response, 1)
	w.mu.Lock()
	w.responses[session] = respChan // 使用 session 匹配 gateway resp.To
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.responses, session)
		w.mu.Unlock()
	}()

	w.msgChan <- Message{
		ID: msgID, Channel: w.name, From: session,
		Content: content, Agent: agentName, Timestamp: time.Now().Unix(),
	}

	select {
	case resp := <-respChan:
		w.writeJSON(rw, http.StatusOK, map[string]string{"response": resp.Content})
	case <-time.After(120 * time.Second):
		w.writeError(rw, http.StatusGatewayTimeout, "timeout")
	}
}

// handleMetrics GET /metrics
func (w *ConsoleChannel) handleMetrics(rw http.ResponseWriter, r *http.Request) {
	w.metricsMu.RLock()
	defer w.metricsMu.RUnlock()

	rw.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(rw, "# HELP go_claw_requests_total Total HTTP requests\n")
	fmt.Fprintf(rw, "# TYPE go_claw_requests_total counter\n")
	fmt.Fprintf(rw, "go_claw_requests_total %d\n", w.reqCount)
	fmt.Fprintf(rw, "# HELP go_claw_errors_total Total errors\n")
	fmt.Fprintf(rw, "# TYPE go_claw_errors_total counter\n")
	fmt.Fprintf(rw, "go_claw_errors_total %d\n", w.errorCount)
}

func (w *ConsoleChannel) RecordRequest(ok bool, latency time.Duration) {
	w.metricsMu.Lock()
	defer w.metricsMu.Unlock()
	w.reqCount++
	if !ok {
		w.errorCount++
	}
	w.avgLatency = (w.avgLatency*time.Duration(w.reqCount-1) + latency) / time.Duration(w.reqCount)
}

// handleSessions GET /api/v1/sessions
func (w *ConsoleChannel) handleSessions(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// 从磁盘读取完整会话信息
	sessions := []map[string]string{}
	dataDir := "clawdata/workspaces"
	if dirs, err := os.ReadDir(dataDir); err == nil {
		for _, dir := range dirs {
			if dir.IsDir() {
				sessDir := dataDir + "/" + dir.Name() + "/sessions"
				if files, err := os.ReadDir(sessDir); err == nil {
					for _, f := range files {
						if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") || strings.HasPrefix(f.Name(), "memories") {
							continue
						}
						filePath := filepath.Join(sessDir, f.Name())
						data, err := os.ReadFile(filePath)
						if err != nil {
							continue
						}
						var sessData map[string]interface{}
						if err := json.Unmarshal(data, &sessData); err != nil {
							continue
						}
						if _, hasMessages := sessData["messages"]; !hasMessages {
							continue
						}
						sessionID, _ := sessData["session_id"].(string)
						if sessionID == "" {
							sessionID, _ = sessData["id"].(string)
						}
						if sessionID == "" {
							continue
						}
						userID, _ := sessData["user_id"].(string)
						if userID == "" {
							parts := strings.SplitN(sessionID, ":", 2)
							if len(parts) == 2 {
								userID = parts[1]
							}
						}
						name, _ := sessData["name"].(string)
						channel, _ := sessData["channel"].(string)
						createdAt, _ := sessData["created_at"].(string)
						updatedAt, _ := sessData["updated_at"].(string)
						sessions = append(sessions, map[string]string{
							"id":         sessionID,
							"session_id": sessionID,
							"name":       name,
							"user_id":    userID,
							"agent":      dir.Name(),
							"channel":    channel,
							"created_at": createdAt,
							"updated_at": updatedAt,
						})
					}
				}
			}
		}
	}
	w.writeJSON(rw, http.StatusOK, sessions)
}

// handleSessionByID GET/DELETE /api/v1/sessions/{id}
func (w *ConsoleChannel) handleSessionByID(rw http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/"), "/")
	sessionID := parts[0]
	if sessionID == "" {
		w.writeError(rw, http.StatusBadRequest, "session id is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// 读取会话详情
		dataDir := "clawdata/workspaces"
		if dirs, err := os.ReadDir(dataDir); err == nil {
			for _, dir := range dirs {
				if dir.IsDir() {
					safeName := store.SafeFileName(sessionID) + ".json"
					filePath := filepath.Join(dataDir, dir.Name(), "sessions", safeName)
					if data, err := os.ReadFile(filePath); err == nil {
						var sessData map[string]interface{}
						if json.Unmarshal(data, &sessData) == nil {
							w.writeJSON(rw, http.StatusOK, sessData)
							return
						}
					}
				}
			}
		}
		w.writeJSON(rw, http.StatusNotFound, map[string]string{"error": "session not found"})
	case http.MethodDelete:
		// 删除会话文件
		dataDir := "clawdata/workspaces"
		safeName := store.SafeFileName(sessionID) + ".json"
		if dirs, err := os.ReadDir(dataDir); err == nil {
			for _, dir := range dirs {
				if dir.IsDir() {
					filePath := filepath.Join(dataDir, dir.Name(), "sessions", safeName)
					os.Remove(filePath)
				}
			}
		}
		w.writeJSON(rw, http.StatusOK, map[string]string{"deleted": sessionID})
	default:
		w.writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (w *ConsoleChannel) handleWebhook(rw http.ResponseWriter, r *http.Request) {
	var req struct {
		User    string `json:"user"`
		Message string `json:"message"`
		Agent   string `json:"agent,omitempty"`
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

	respChan := make(chan Response, 1)
	w.mu.Lock()
	w.responses[req.User] = respChan // 使用 req.User 匹配 gateway resp.To
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.responses, req.User)
		w.mu.Unlock()
	}()

	w.msgChan <- Message{
		ID: msgID, Channel: w.name, From: req.User,
		Content: req.Message, Agent: req.Agent, Timestamp: time.Now().Unix(),
	}

	select {
	case resp := <-respChan:
		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]string{"response": resp.Content})
	case <-time.After(60 * time.Second):
		http.Error(rw, "Timeout", http.StatusGatewayTimeout)
	}
}
