package channel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go-claw/pkg/log"

	"github.com/gorilla/websocket"
)

// WebSocket 命令常量 (对标 Python SDK types.py WsCmd)
const (
	WsCmdSubscribe       = "aibot_subscribe"           // 认证订阅
	WsCmdHeartbeat       = "ping"                      // 心跳
	WsCmdResponse        = "aibot_respond_msg"         // 回复消息
	WsCmdResponseWelcome = "aibot_respond_welcome_msg" // 回复欢迎语
	WsCmdResponseUpdate  = "aibot_respond_update_msg"  // 更新模板卡片
	WsCmdSendMsg         = "aibot_send_msg"            // 主动发送消息
	WsCmdCallback        = "aibot_msg_callback"        // 消息推送回调
	WsCmdEventCallback   = "aibot_event_callback"      // 事件推送回调
)

const DefaultWsURL = "wss://openws.work.weixin.qq.com"

// WeComChannel 企业微信机器人渠道（WebSocket 长连接模式）
type WeComChannel struct {
	*BotChannelBase
	botID  string
	secret string

	conn     *websocket.Conn
	connMu   sync.Mutex
	stopChan chan struct{}

	authenticated bool
	authMu        sync.Mutex

	heartbeatInterval time.Duration
	maxMissedPong     int
	missedPongCount   int
	heartbeatStop     chan struct{}
	heartbeatMu       sync.Mutex

	reconnectBaseDelay   time.Duration
	reconnectMaxDelay    time.Duration
	maxReconnectAttempts int
	reconnectAttempts    int
	isManualClose        bool

	pendingAcks     map[string]chan error
	pendingAcksMu   sync.Mutex
	replyAckTimeout time.Duration

	reqIDCounter int64
	reqIDMu      sync.Mutex

	sessionInfo   map[string]sessionData
	sessionInfoMu sync.RWMutex
}

type sessionData struct {
	chatID   string
	chatType string
	userID   string
	reqID    string
	streamID string // 流式消息ID
	thinking bool   // 是否已发送思考状态
}

// NewWeComChannel 创建企业微信渠道
func NewWeComChannel(botID, secret string, display DisplayConfig) *WeComChannel {
	return &WeComChannel{
		BotChannelBase:       NewBotChannelBase("wecom", "", display),
		botID:                botID,
		secret:               secret,
		stopChan:             make(chan struct{}),
		heartbeatInterval:    30 * time.Second,
		maxMissedPong:        2,
		reconnectBaseDelay:   1 * time.Second,
		reconnectMaxDelay:    30 * time.Second,
		maxReconnectAttempts: 10,
		pendingAcks:          make(map[string]chan error),
		replyAckTimeout:      5 * time.Second,
		sessionInfo:          make(map[string]sessionData),
	}
}

func (w *WeComChannel) Start(ctx context.Context) error {
	log.Logger().Info("[WeCom] 启动WebSocket长连接模式")
	w.isManualClose = false
	w.reconnectAttempts = 0
	return w.connect(ctx)
}

func (w *WeComChannel) connect(ctx context.Context) error {
	w.cleanupConnection()

	log.Logger().Info("[WeCom] 连接WebSocket", "url", DefaultWsURL)

	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(DefaultWsURL, nil)
	if err != nil {
		log.Logger().Error("[WeCom] WebSocket连接失败", "err", err)
		w.scheduleReconnect(ctx)
		return err
	}

	w.connMu.Lock()
	w.conn = conn
	w.connMu.Unlock()

	w.missedPongCount = 0

	log.Logger().Info("[WeCom] WebSocket连接成功，发送认证请求")
	w.sendAuth()
	go w.receiveLoop(ctx)

	return nil
}

func (w *WeComChannel) cleanupConnection() {
	w.connMu.Lock()
	if w.conn != nil {
		w.conn.Close()
		w.conn = nil
	}
	w.connMu.Unlock()

	w.stopHeartbeat()
	w.clearPendingMessages("Connection closed")
}

func (w *WeComChannel) Stop() error {
	w.isManualClose = true
	close(w.stopChan)
	w.cleanupConnection()
	log.Logger().Info("[WeCom] 已停止")
	return nil
}

// generateReqID 生成唯一请求 ID (格式: {prefix}_{timestamp}_{random})
func (w *WeComChannel) generateReqID(prefix string) string {
	w.reqIDMu.Lock()
	w.reqIDCounter++
	ts := time.Now().UnixMilli()
	w.reqIDMu.Unlock()

	return fmt.Sprintf("%s_%d_%s", prefix, ts, generateRandomString(8))
}

func generateRandomString(length int) string {
	b := make([]byte, (length+1)/2)
	rand.Read(b)
	return hex.EncodeToString(b)[:length]
}

// sendAuth 发送认证帧
func (w *WeComChannel) sendAuth() {
	frame := map[string]any{
		"cmd": WsCmdSubscribe,
		"headers": map[string]string{
			"req_id": w.generateReqID(WsCmdSubscribe),
		},
		"body": map[string]string{
			"bot_id": w.botID,
			"secret": w.secret,
		},
	}

	// 打印认证帧内容用于调试
	data, _ := json.Marshal(frame)
	log.Logger().Debug("[WeCom] 发送认证帧", "json", string(data))

	w.sendFrame(frame)
	log.Logger().Debug("[WeCom] 认证帧已发送")
}

// sendFrame 发送 WebSocket 帧
func (w *WeComChannel) sendFrame(frame map[string]any) error {
	w.connMu.Lock()
	conn := w.conn
	w.connMu.Unlock()

	if conn == nil {
		return fmt.Errorf("WebSocket未连接")
	}

	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// receiveLoop 消息接收循环
func (w *WeComChannel) receiveLoop(ctx context.Context) {
	for {
		select {
		case <-w.stopChan:
			return
		case <-ctx.Done():
			return
		default:
		}

		w.connMu.Lock()
		conn := w.conn
		w.connMu.Unlock()

		if conn == nil {
			time.Sleep(time.Second)
			continue
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Logger().Error("[WeCom] WebSocket读取错误", "err", err)
			w.stopHeartbeat()
			w.clearPendingMessages(fmt.Sprintf("连接关闭: %v", err))
			if !w.isManualClose {
				w.scheduleReconnect(ctx)
			}
			return
		}

		w.handleFrame(data)
	}
}

// handleFrame 处理收到的 WebSocket 帧 (对标 Python SDK ws.py _handle_frame)
func (w *WeComChannel) handleFrame(data []byte) {
	var frame map[string]any
	if err := json.Unmarshal(data, &frame); err != nil {
		log.Logger().Error("[WeCom] JSON解析失败", "err", err)
		return
	}

	cmd, _ := frame["cmd"].(string)

	// 消息推送回调
	if cmd == WsCmdCallback {
		w.handleMessageCallback(frame)
		return
	}

	// 事件推送回调
	if cmd == WsCmdEventCallback {
		w.handleEventCallback(frame)
		return
	}

	// 无 cmd 的帧：认证响应、心跳响应或回复回执
	headers, _ := frame["headers"].(map[string]any)
	reqID, _ := headers["req_id"].(string)

	// 检查是否是回复消息的回执
	w.pendingAcksMu.Lock()
	ackChan, hasPending := w.pendingAcks[reqID]
	w.pendingAcksMu.Unlock()

	if hasPending {
		w.handleReplyAck(reqID, frame, ackChan)
		return
	}

	// 认证响应: req_id 以 "aibot_subscribe" 开头
	if len(reqID) >= len(WsCmdSubscribe) && reqID[:len(WsCmdSubscribe)] == WsCmdSubscribe {
		w.handleAuthResponse(frame)
		return
	}

	// 心跳响应: req_id 以 "ping" 开头
	if len(reqID) >= len(WsCmdHeartbeat) && reqID[:len(WsCmdHeartbeat)] == WsCmdHeartbeat {
		w.handleHeartbeatResponse(frame)
		return
	}

	log.Logger().Warn("[WeCom] 收到未知帧", "frame", truncateJSON(frame))
}

// handleAuthResponse 处理认证响应
func (w *WeComChannel) handleAuthResponse(frame map[string]any) {
	errcode, _ := frame["errcode"].(float64)
	errmsg, _ := frame["errmsg"].(string)

	if int(errcode) != 0 {
		log.Logger().Error("[WeCom] 认证失败", "errcode", int(errcode), "errmsg", errmsg)
		return
	}

	w.authMu.Lock()
	w.authenticated = true
	w.authMu.Unlock()

	w.startHeartbeat()
	log.Logger().Info("[WeCom] 认证成功")
}

// handleHeartbeatResponse 处理心跳响应
func (w *WeComChannel) handleHeartbeatResponse(frame map[string]any) {
	errcode, _ := frame["errcode"].(float64)
	if int(errcode) != 0 {
		errmsg, _ := frame["errmsg"].(string)
		log.Logger().Warn("[WeCom] 心跳响应错误", "errcode", int(errcode), "errmsg", errmsg)
		return
	}

	w.heartbeatMu.Lock()
	w.missedPongCount = 0
	w.heartbeatMu.Unlock()

	log.Logger().Debug("[WeCom] 收到心跳响应")
}

// handleMessageCallback 处理消息推送回调
func (w *WeComChannel) handleMessageCallback(frame map[string]any) {
	body, _ := frame["body"].(map[string]any)
	if body == nil {
		return
	}

	msgType, _ := body["msgtype"].(string)
	msgID, _ := body["msgid"].(string)
	aibotID, _ := body["aibotid"].(string)
	chatID, _ := body["chatid"].(string)
	chatType, _ := body["chattype"].(string)

	from, _ := body["from"].(map[string]any)
	userID, _ := from["userid"].(string)

	headers, _ := frame["headers"].(map[string]any)
	reqID, _ := headers["req_id"].(string)

	// 存储会话信息
	w.sessionInfoMu.Lock()
	w.sessionInfo[userID] = sessionData{
		chatID:   chatID,
		chatType: chatType,
		userID:   userID,
		reqID:    reqID,
	}
	w.sessionInfoMu.Unlock()

	if msgType != "text" {
		log.Logger().Debug("[WeCom] 收到非文本消息", "msgtype", msgType)
		return
	}

	text, _ := body["text"].(map[string]any)
	content, _ := text["content"].(string)
	if content == "" {
		return
	}

	log.Logger().Info("[WeCom] 收到文本消息",
		"msg_id", msgID, "chat_id", chatID, "user_id", userID, "content", content)

	msg := Message{
		ID:        msgID,
		Channel:   w.name,
		From:      userID,
		Content:   content,
		Timestamp: time.Now().Unix(),
		Metadata: map[string]any{
			"chatid":   chatID,
			"chattype": chatType,
			"userid":   userID,
			"req_id":   reqID,
			"aibotid":  aibotID,
		},
	}

	w.PushMessage(msg)
}

// handleEventCallback 处理事件推送回调
func (w *WeComChannel) handleEventCallback(frame map[string]any) {
	body, _ := frame["body"].(map[string]any)
	if body == nil {
		return
	}

	event, _ := body["event"].(map[string]any)
	eventType, _ := event["eventtype"].(string)

	log.Logger().Info("[WeCom] 收到事件回调", "eventtype", eventType)

	if eventType == "enter_chat" {
		log.Logger().Debug("[WeCom] 用户进入会话")
	} else if eventType == "disconnected_event" {
		log.Logger().Warn("[WeCom] 收到连接断开事件")
	}
}

// startHeartbeat 启动心跳
func (w *WeComChannel) startHeartbeat() {
	w.heartbeatMu.Lock()
	if w.heartbeatStop != nil {
		select {
		case <-w.heartbeatStop:
		default:
			close(w.heartbeatStop)
		}
	}
	w.heartbeatStop = make(chan struct{})
	stop := w.heartbeatStop
	w.heartbeatMu.Unlock()

	go func() {
		ticker := time.NewTicker(w.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-w.stopChan:
				return
			case <-ticker.C:
				w.sendHeartbeat()
			}
		}
	}()

	log.Logger().Debug("[WeCom] 心跳定时器已启动", "interval", w.heartbeatInterval)
}

// stopHeartbeat 停止心跳
func (w *WeComChannel) stopHeartbeat() {
	w.heartbeatMu.Lock()
	if w.heartbeatStop != nil {
		select {
		case <-w.heartbeatStop:
		default:
			close(w.heartbeatStop)
		}
		w.heartbeatStop = nil
	}
	w.heartbeatMu.Unlock()
}

// sendHeartbeat 发送心跳
func (w *WeComChannel) sendHeartbeat() {
	w.heartbeatMu.Lock()
	if w.missedPongCount >= w.maxMissedPong {
		w.heartbeatMu.Unlock()
		log.Logger().Warn("[WeCom] 连续未收到心跳响应，将重连", "missed", w.missedPongCount)
		w.connMu.Lock()
		if w.conn != nil {
			w.conn.Close()
		}
		w.connMu.Unlock()
		return
	}
	w.missedPongCount++
	count := w.missedPongCount
	w.heartbeatMu.Unlock()

	frame := map[string]any{
		"cmd": WsCmdHeartbeat,
		"headers": map[string]string{
			"req_id": w.generateReqID(WsCmdHeartbeat),
		},
	}

	if err := w.sendFrame(frame); err != nil {
		log.Logger().Error("[WeCom] 发送心跳失败", "err", err)
		return
	}

	log.Logger().Debug("[WeCom] 心跳已发送", "awaiting_pong", count)
}

// scheduleReconnect 安排重连 (指数退避)
func (w *WeComChannel) scheduleReconnect(ctx context.Context) {
	if w.isManualClose {
		return
	}
	if w.maxReconnectAttempts != -1 && w.reconnectAttempts >= w.maxReconnectAttempts {
		log.Logger().Error("[WeCom] 达到最大重连次数", "attempts", w.maxReconnectAttempts)
		return
	}

	w.reconnectAttempts++

	delay := w.reconnectBaseDelay * time.Duration(1<<(w.reconnectAttempts-1))
	if delay > w.reconnectMaxDelay {
		delay = w.reconnectMaxDelay
	}

	log.Logger().Info("[WeCom] 准备重连", "delay", delay, "attempt", w.reconnectAttempts)
	time.Sleep(delay)

	if w.isManualClose {
		return
	}
	w.connect(ctx)
}

// handleReplyAck 处理回复回执
func (w *WeComChannel) handleReplyAck(reqID string, frame map[string]any, ackChan chan error) {
	w.pendingAcksMu.Lock()
	delete(w.pendingAcks, reqID)
	w.pendingAcksMu.Unlock()

	errcode, _ := frame["errcode"].(float64)
	errmsg, _ := frame["errmsg"].(string)

	if int(errcode) != 0 {
		log.Logger().Warn("[WeCom] 回复回执错误", "req_id", reqID, "errcode", int(errcode), "errmsg", errmsg)
		ackChan <- fmt.Errorf("回复失败: %s (code: %d)", errmsg, int(errcode))
	} else {
		log.Logger().Debug("[WeCom] 收到回复回执", "req_id", reqID)
		ackChan <- nil
	}
}

// clearPendingMessages 清理所有待处理的消息
func (w *WeComChannel) clearPendingMessages(reason string) {
	w.pendingAcksMu.Lock()
	for reqID, ackChan := range w.pendingAcks {
		ackChan <- fmt.Errorf("%s", reason)
		delete(w.pendingAcks, reqID)
	}
	w.pendingAcksMu.Unlock()
}

// Send 发送最终响应 (stream finish=true)
func (w *WeComChannel) Send(ctx context.Context, resp Response) error {
	w.sessionInfoMu.RLock()
	session, ok := w.sessionInfo[resp.To]
	w.sessionInfoMu.RUnlock()

	if !ok {
		return fmt.Errorf("[WeCom] 未找到用户 %s 的会话信息", resp.To)
	}

	reqID := session.reqID
	streamID := session.streamID
	if streamID == "" {
		streamID = w.generateReqID("stream")
	}

	frame := map[string]any{
		"cmd": WsCmdResponse,
		"headers": map[string]string{
			"req_id": reqID,
		},
		"body": map[string]any{
			"msgtype": "stream",
			"stream": map[string]any{
				"id":      streamID,
				"finish":  true,
				"content": resp.Content,
			},
		},
	}

	log.Logger().Info("[WeCom] 发送最终响应", "user", resp.To, "content_len", len(resp.Content))
	return w.sendAndWaitAck(reqID, frame)
}

// SendToolEvent 发送工具事件（流式中间帧）
func (w *WeComChannel) SendToolEvent(event ToolEvent) error {
	if event.To == "" {
		return nil
	}

	w.sessionInfoMu.RLock()
	session, ok := w.sessionInfo[event.To]
	w.sessionInfoMu.RUnlock()

	if !ok {
		return nil
	}

	reqID := session.reqID
	// 首次事件时生成 streamID 并存储
	if session.streamID == "" {
		streamID := w.generateReqID("stream")
		session.streamID = streamID
		w.sessionInfoMu.Lock()
		w.sessionInfo[event.To] = session
		w.sessionInfoMu.Unlock()
	}

	streamID := session.streamID

	var content string
	switch event.Type {
	case ToolEventThinking:
		content = "💭 思考中..."
	case ToolEventCalling:
		content = fmt.Sprintf("🔧 调用工具: %s", event.ToolName)
	case ToolEventResult:
		// 工具结果不发送中间帧，避免过长
		return nil
	case ToolEventError:
		content = fmt.Sprintf("❌ %s: %s", event.ToolName, event.Error)
	default:
		return nil
	}

	frame := map[string]any{
		"cmd": WsCmdResponse,
		"headers": map[string]string{
			"req_id": reqID,
		},
		"body": map[string]any{
			"msgtype": "stream",
			"stream": map[string]any{
				"id":      streamID,
				"finish":  false,
				"content": content,
			},
		},
	}

	log.Logger().Debug("[WeCom] 发送中间帧", "type", event.Type, "content", content)
	return w.sendFrame(frame) // 中间帧不等 ack
}

// sendAndWaitAck 发送消息并等待回执
func (w *WeComChannel) sendAndWaitAck(reqID string, frame map[string]any) error {
	ackChan := make(chan error, 1)

	w.pendingAcksMu.Lock()
	w.pendingAcks[reqID] = ackChan
	w.pendingAcksMu.Unlock()

	if err := w.sendFrame(frame); err != nil {
		w.pendingAcksMu.Lock()
		delete(w.pendingAcks, reqID)
		w.pendingAcksMu.Unlock()
		return err
	}

	select {
	case err := <-ackChan:
		return err
	case <-time.After(w.replyAckTimeout):
		w.pendingAcksMu.Lock()
		delete(w.pendingAcks, reqID)
		w.pendingAcksMu.Unlock()
		return fmt.Errorf("[WeCom] 回复回执超时 (%v)", w.replyAckTimeout)
	}
}


// SendProactive 主动发送消息（不需要用户先发消息）
// 使用 aibot_send_msg 命令，严格遵循官方文档格式
// 单聊不需要 sessionInfo（chatid=userID, chat_type=1）
// 群聊需要 sessionInfo 来获取群聊 chatid 和 chat_type
func (w *WeComChannel) SendProactive(ctx context.Context, userID, content string) error {
	reqID := w.generateReqID(WsCmdSendMsg)

	// 默认单聊: chatid = userID, chat_type = 1
	chatID := userID
	chatType := uint32(1)

	// 如果有群聊会话信息，使用群聊参数
	w.sessionInfoMu.RLock()
	session, ok := w.sessionInfo[userID]
	w.sessionInfoMu.RUnlock()

	if ok && session.chatID != "" && session.chatType == "group" {
		chatID = session.chatID
		chatType = uint32(2)
	}

	// 严格按官方文档格式
	frame := map[string]any{
		"cmd": WsCmdSendMsg,
		"headers": map[string]string{
			"req_id": reqID,
		},
		"body": map[string]any{
			"chatid":    chatID,
			"chat_type": chatType,
			"msgtype":   "markdown",
			"markdown": map[string]string{
				"content": content,
			},
		},
	}

	log.Logger().Info("[WeCom] 主动发送消息", "user", userID, "chat_id", chatID, "chat_type", chatType, "content_len", len(content))
	return w.sendAndWaitAck(reqID, frame)
}

func truncateJSON(v any) string {
	data, _ := json.Marshal(v)
	s := string(data)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
