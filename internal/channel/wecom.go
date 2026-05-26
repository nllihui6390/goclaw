package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go-claw/pkg/log"

	"github.com/gorilla/websocket"
)

// WeComChannel 企业微信机器人渠道（WebSocket 长连接模式）
type WeComChannel struct {
	*BotChannelBase
	botID  string
	secret string

	conn      *websocket.Conn
	connMu    sync.Mutex
	stopChan  chan struct{}
	reqID     int
	reqIDMu   sync.Mutex
	reqIDs    map[string]string // userID -> reqID 映射
	reqIDsMu  sync.RWMutex
}

// NewWeComChannel 创建企业微信渠道
func NewWeComChannel(botID, secret string) *WeComChannel {
	return &WeComChannel{
		BotChannelBase: NewBotChannelBase("wecom", ""), // 不需要端口
		botID:          botID,
		secret:         secret,
		stopChan:       make(chan struct{}),
		reqIDs:         make(map[string]string),
	}
}

func (w *WeComChannel) Start(ctx context.Context) error {
	log.Logger().Info("[WeCom] 启动WebSocket长连接模式")

	// 连接企业微信 WebSocket
	url := "wss://openws.work.weixin.qq.com"

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %v", err)
	}

	w.connMu.Lock()
	w.conn = conn
	w.connMu.Unlock()

	// 发送订阅请求
	if err := w.subscribe(conn); err != nil {
		conn.Close()
		return err
	}

	// 启动消息接收循环
	go w.receiveLoop(ctx)

	// 启动心跳
	go w.heartbeatLoop(ctx)

	log.Logger().Info("[WeCom] WebSocket连接成功")
	return nil
}

func (w *WeComChannel) Stop() error {
	close(w.stopChan)
	w.connMu.Lock()
	if w.conn != nil {
		w.conn.Close()
		w.conn = nil
	}
	w.connMu.Unlock()
	return nil
}

// nextReqID 生成请求ID
func (w *WeComChannel) nextReqID() string {
	w.reqIDMu.Lock()
	w.reqID++
	id := fmt.Sprintf("req-%d-%d", time.Now().UnixMilli(), w.reqID)
	w.reqIDMu.Unlock()
	return id
}

// subscribe 发送订阅请求
func (w *WeComChannel) subscribe(conn *websocket.Conn) error {
	req := map[string]any{
		"cmd": "aibot_subscribe",
		"headers": map[string]string{
			"req_id": w.nextReqID(),
		},
		"body": map[string]string{
			"bot_id": w.botID,
			"secret": w.secret,
		},
	}
	data, _ := json.Marshal(req)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("发送订阅请求失败: %v", err)
	}

	// 等待订阅响应
	_, respData, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("读取订阅响应失败: %v", err)
	}

	var resp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		return fmt.Errorf("解析订阅响应失败: %v", err)
	}
	if resp.ErrCode != 0 {
		return fmt.Errorf("订阅失败: %s", resp.ErrMsg)
	}

	log.Logger().Info("[WeCom] 订阅成功")
	return nil
}

func (w *WeComChannel) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.sendPing()
		}
	}
}

func (w *WeComChannel) sendPing() {
	w.connMu.Lock()
	conn := w.conn
	w.connMu.Unlock()

	if conn == nil {
		return
	}

	req := map[string]any{
		"cmd": "ping",
		"headers": map[string]string{
			"req_id": w.nextReqID(),
		},
	}
	data, _ := json.Marshal(req)
	conn.WriteMessage(websocket.TextMessage, data)
}

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
			time.Sleep(5 * time.Second)
			w.reconnect(ctx)
			continue
		}

		w.handleMessage(data)
	}
}

func (w *WeComChannel) reconnect(ctx context.Context) {
	url := "wss://openws.work.weixin.qq.com"

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		log.Logger().Error("[WeCom] 重连失败", "err", err)
		return
	}

	if err := w.subscribe(conn); err != nil {
		conn.Close()
		log.Logger().Error("[WeCom] 重连订阅失败", "err", err)
		return
	}

	w.connMu.Lock()
	if w.conn != nil {
		w.conn.Close()
	}
	w.conn = conn
	w.connMu.Unlock()

	log.Logger().Info("[WeCom] WebSocket重连成功")
}

func (w *WeComChannel) handleMessage(data []byte) {
	var payload struct {
		Cmd     string          `json:"cmd"`
		Headers json.RawMessage `json:"headers"`
		Body    json.RawMessage `json:"body"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}

	switch payload.Cmd {
	case "aibot_msg_callback":
		w.handleMsgCallback(payload.Headers, payload.Body)
	case "aibot_event_callback":
		w.handleEventCallback(payload.Headers, payload.Body)
	}
}

func (w *WeComChannel) handleMsgCallback(headers, body json.RawMessage) {
	var msgBody struct {
		MsgID    string `json:"msgid"`
		AiBotID  string `json:"aibotid"`
		ChatID   string `json:"chatid"`
		ChatType string `json:"chattype"`
		From     struct {
			UserID string `json:"userid"`
		} `json:"from"`
		MsgType string `json:"msgtype"`
		Text    struct {
			Content string `json:"content"`
		} `json:"text"`
	}

	if err := json.Unmarshal(body, &msgBody); err != nil {
		log.Logger().Error("[WeCom] 消息解析失败", "err", err)
		return
	}

	// 只处理文本消息
	if msgBody.MsgType != "text" || msgBody.Text.Content == "" {
		return
	}

	log.Logger().Info("[WeCom] 收到消息", "msg_id", msgBody.MsgID, "from", msgBody.From.UserID, "content", msgBody.Text.Content)

	// 提取 req_id 用于回复
	var hdr struct {
		ReqID string `json:"req_id"`
	}
	json.Unmarshal(headers, &hdr)

	// 存储 req_id，Send 时使用
	w.reqIDsMu.Lock()
	w.reqIDs[msgBody.From.UserID] = hdr.ReqID
	w.reqIDsMu.Unlock()

	msg := Message{
		ID:        msgBody.MsgID,
		Channel:   w.name,
		From:      msgBody.From.UserID,
		Content:   msgBody.Text.Content,
		Timestamp: time.Now().Unix(),
	}

	w.PushMessage(msg)
}

func (w *WeComChannel) handleEventCallback(headers, body json.RawMessage) {
	var eventBody struct {
		MsgType string `json:"msgtype"`
		Event   struct {
			EventType string `json:"eventtype"`
		} `json:"event"`
	}

	if err := json.Unmarshal(body, &eventBody); err != nil {
		return
	}

	// 连接断开事件
	if eventBody.Event.EventType == "disconnected_event" {
		log.Logger().Warn("[WeCom] 收到连接断开事件")
		return
	}

	// 进入会话事件
	if eventBody.Event.EventType == "enter_chat" {
		log.Logger().Info("[WeCom] 用户进入会话")
		// 可以回复欢迎语
	}
}

// Send 发送响应
func (w *WeComChannel) Send(ctx context.Context, resp Response) error {
	w.connMu.Lock()
	conn := w.conn
	w.connMu.Unlock()

	if conn == nil {
		return fmt.Errorf("WebSocket未连接")
	}

	// 从 reqIDs map 获取 req_id
	w.reqIDsMu.RLock()
	reqID, exists := w.reqIDs[resp.To]
	w.reqIDsMu.RUnlock()

	if !exists {
		reqID = w.nextReqID()
	}

	// 发送消息响应
	req := map[string]any{
		"cmd": "aibot_respond_msg",
		"headers": map[string]string{
			"req_id": reqID,
		},
		"body": map[string]any{
			"msgtype": "text",
			"text": map[string]string{
				"content": resp.Content,
			},
		},
	}

	data, _ := json.Marshal(req)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return err
	}

	// 清理已使用的 req_id
	w.reqIDsMu.Lock()
	delete(w.reqIDs, resp.To)
	w.reqIDsMu.Unlock()

	log.Logger().Debug("[WeCom] 消息已发送", "to", resp.To)
	return nil
}

func (w *WeComChannel) SendToolEvent(event ToolEvent) error {
	return nil
}