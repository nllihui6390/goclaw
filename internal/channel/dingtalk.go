package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go-claw/pkg/log"

	"github.com/gorilla/websocket"
)

// DingTalkChannel 钉钉机器人渠道（Stream WebSocket 模式）
type DingTalkChannel struct {
	*BotChannelBase
	clientID     string
	clientSecret string

	conn     *websocket.Conn
	connMu   sync.Mutex
	stopChan chan struct{}

	sessionInfo   map[string]dingtalkSession
	sessionInfoMu sync.RWMutex
}

type dingtalkSession struct {
	conversationID string // chatid 用于主动发送
}

// NewDingTalkChannel 创建钉钉渠道
func NewDingTalkChannel(clientID, clientSecret string) *DingTalkChannel {
	return &DingTalkChannel{
		BotChannelBase: NewBotChannelBase("dingtalk", ""), // 不需要端口
		clientID:       clientID,
		clientSecret:   clientSecret,
		stopChan:       make(chan struct{}),
		sessionInfo:    make(map[string]dingtalkSession),
	}
}

func (d *DingTalkChannel) Start(ctx context.Context) error {
	log.Logger().Info("[DingTalk] 启动Stream模式")

	// 连接钉钉 Stream 服务
	url := fmt.Sprintf("wss://stream.dingtalk.com?clientId=%s&clientSecret=%s", d.clientID, d.clientSecret)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %v", err)
	}

	d.connMu.Lock()
	d.conn = conn
	d.connMu.Unlock()

	// 发送订阅请求
	d.subscribe(conn)

	// 启动消息接收循环
	go d.receiveLoop(ctx)

	log.Logger().Info("[DingTalk] Stream连接成功")
	return nil
}

func (d *DingTalkChannel) Stop() error {
	close(d.stopChan)
	d.connMu.Lock()
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
	d.connMu.Unlock()
	return nil
}

// subscribe 发送订阅请求
func (d *DingTalkChannel) subscribe(conn *websocket.Conn) {
	// 订阅机器人消息
	subscribeMsg := map[string]any{
		"clientId":          d.clientID,
		"type":              "SUBSCRIPTION",
		"subscriptionKeys":  []string{"EVENT"},  // 订阅事件
		"topicIds":          []string{"ROBOT"},  // 订阅机器人消息
	}
	data, _ := json.Marshal(subscribeMsg)
	conn.WriteMessage(websocket.TextMessage, data)
	log.Logger().Info("[DingTalk] 已发送订阅请求")
}

func (d *DingTalkChannel) receiveLoop(ctx context.Context) {
	for {
		select {
		case <-d.stopChan:
			return
		case <-ctx.Done():
			return
		default:
		}

		d.connMu.Lock()
		conn := d.conn
		d.connMu.Unlock()

		if conn == nil {
			time.Sleep(time.Second)
			continue
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Logger().Error("[DingTalk] WebSocket读取错误", "err", err)
			time.Sleep(5 * time.Second)
			d.reconnect(ctx)
			continue
		}

		d.handleMessage(data)
	}
}

func (d *DingTalkChannel) reconnect(ctx context.Context) {
	url := fmt.Sprintf("wss://stream.dingtalk.com?clientId=%s&clientSecret=%s", d.clientID, d.clientSecret)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		log.Logger().Error("[DingTalk] 重连失败", "err", err)
		return
	}

	d.connMu.Lock()
	if d.conn != nil {
		d.conn.Close()
	}
	d.conn = conn
	d.connMu.Unlock()

	d.subscribe(conn)
	log.Logger().Info("[DingTalk] Stream重连成功")
}

func (d *DingTalkChannel) handleMessage(data []byte) {
	var payload struct {
		Type        string `json:"type"`
		TopicId     string `json:"topicId"`
		Data        json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		log.Logger().Error("[DingTalk] JSON解析失败", "err", err)
		return
	}

	// 只处理机器人消息
	if payload.TopicId != "ROBOT" {
		return
	}

	// 解析消息数据
	var msgData struct {
		MsgType        string `json:"msgtype"`
		Text           struct {
			Content string `json:"content"`
		} `json:"text"`
		SenderStaffId  string `json:"senderStaffId"`
		SenderNick     string `json:"senderNick"`
		SessionWebhook string `json:"sessionWebhook"`
		CreateTime     int64  `json:"createTime"`
		ChatId         string `json:"chatid"`
	}

	if err := json.Unmarshal(payload.Data, &msgData); err != nil {
		log.Logger().Error("[DingTalk] 消息数据解析失败", "err", err)
		return
	}

	// 只处理文本消息
	if msgData.MsgType != "text" || msgData.Text.Content == "" {
		return
	}

	msgID := fmt.Sprintf("dingtalk-%d", msgData.CreateTime)
	log.Logger().Info("[DingTalk] 收到消息", "msg_id", msgID, "sender", msgData.SenderNick, "content", msgData.Text.Content)

	// 创建消息
	msg := Message{
		ID:        msgID,
		Channel:   d.name,
		From:      msgData.SenderStaffId,
		Content:   msgData.Text.Content,
		Timestamp: msgData.CreateTime / 1000,
		Metadata: map[string]any{
			"sender_nick":     msgData.SenderNick,
			"session_webhook": msgData.SessionWebhook,
			"sender_staff_id": msgData.SenderStaffId,
			"chat_id":         msgData.ChatId,
		},
	}

	// 存储会话信息用于主动发送
	d.sessionInfoMu.Lock()
	d.sessionInfo[msgData.SenderStaffId] = dingtalkSession{
		conversationID: msgData.ChatId,
	}
	d.sessionInfoMu.Unlock()

	d.PushMessage(msg)
}

// Send 发送响应（通过 sessionWebhook）
func (d *DingTalkChannel) Send(ctx context.Context, resp Response) error {
	// 优先使用 sessionWebhook（从 metadata 获取）
	// 如果没有，则通过钉钉 API 发送

	// 使用 HTTP POST 发送消息到钉钉
	url := "https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend"

	reqBody := map[string]any{
		"robotCode": d.clientID,
		"userIds":   []string{resp.To},
		"msgType":   "text",
		"msgParam":  string(mustJSON(map[string]string{"content": resp.Content})),
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := d.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		respBody, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("钉钉API返回 %d: %s", httpResp.StatusCode, truncateStr(string(respBody), 200))
	}

	log.Logger().Debug("[DingTalk] 消息已发送", "to", resp.To)
	return nil
}

// SendProactive 主动发送消息（钉钉 Stream 模式）
func (d *DingTalkChannel) SendProactive(ctx context.Context, userID, content string) error {
	d.sessionInfoMu.RLock()
	session, ok := d.sessionInfo[userID]
	d.sessionInfoMu.RUnlock()

	if !ok {
		return fmt.Errorf("[DingTalk] 未找到用户 %s 的会话信息（用户需先与机器人对话一次）", userID)
	}

	// 使用钉钉机器人 API 发送消息到群聊
	url := "https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend"

	reqBody := map[string]any{
		"robotCode": d.clientID,
		"userIds":   []string{userID},
		"msgType":   "text",
		"msgParam":  string(mustJSON(map[string]string{"content": content})),
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := d.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		respBody, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("钉钉API返回 %d: %s", httpResp.StatusCode, truncateStr(string(respBody), 200))
	}

	log.Logger().Info("[DingTalk] 主动消息已发送", "user", userID, "conversation_id", session.conversationID)
	return nil
}

func (d *DingTalkChannel) SendToolEvent(event ToolEvent) error {
	return nil
}