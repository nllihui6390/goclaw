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

// LarkChannel 飞书机器人渠道（WebSocket 客户端模式）
type LarkChannel struct {
	*BotChannelBase
	appID     string
	appSecret string

	conn       *websocket.Conn
	connMu     sync.Mutex
	tokenCache string
	tokenExpiry time.Time
	tokenMu    sync.RWMutex
	stopChan   chan struct{}

	sessionInfo   map[string]larkSession
	sessionInfoMu sync.RWMutex
}

type larkSession struct {
	openID string // 用户 open_id 用于主动发送
}

// NewLarkChannel 创建飞书渠道
func NewLarkChannel(appID, appSecret string) *LarkChannel {
	return &LarkChannel{
		BotChannelBase: NewBotChannelBase("lark", ""), // 不需要端口
		appID:          appID,
		appSecret:      appSecret,
		stopChan:       make(chan struct{}),
		sessionInfo:    make(map[string]larkSession),
	}
}

func (l *LarkChannel) Start(ctx context.Context) error {
	log.Logger().Info("[Lark] 启动WebSocket客户端模式")

	// 获取 token
	token, err := l.getTenantAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取token失败: %v", err)
	}

	// 连接 WebSocket
	url := "wss://open.feishu.cn/open-apis/event/v2/stream/"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %v", err)
	}

	l.connMu.Lock()
	l.conn = conn
	l.connMu.Unlock()

	// 启动消息接收循环
	go l.receiveLoop(ctx)

	log.Logger().Info("[Lark] WebSocket连接成功")
	return nil
}

func (l *LarkChannel) Stop() error {
	close(l.stopChan)
	l.connMu.Lock()
	if l.conn != nil {
		l.conn.Close()
		l.conn = nil
	}
	l.connMu.Unlock()
	return nil
}

func (l *LarkChannel) receiveLoop(ctx context.Context) {
	for {
		select {
		case <-l.stopChan:
			return
		case <-ctx.Done():
			return
		default:
		}

		l.connMu.Lock()
		conn := l.conn
		l.connMu.Unlock()

		if conn == nil {
			time.Sleep(time.Second)
			continue
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Logger().Error("[Lark] WebSocket读取错误", "err", err)
			// 尝试重连
			time.Sleep(5 * time.Second)
			l.reconnect(ctx)
			continue
		}

		l.handleMessage(data)
	}
}

func (l *LarkChannel) reconnect(ctx context.Context) {
	token, err := l.getTenantAccessToken(ctx)
	if err != nil {
		log.Logger().Error("[Lark] 重连获取token失败", "err", err)
		return
	}

	url := "wss://open.feishu.cn/open-apis/event/v2/stream/"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		log.Logger().Error("[Lark] 重连失败", "err", err)
		return
	}

	l.connMu.Lock()
	if l.conn != nil {
		l.conn.Close()
	}
	l.conn = conn
	l.connMu.Unlock()

	log.Logger().Info("[Lark] WebSocket重连成功")
}

func (l *LarkChannel) handleMessage(data []byte) {
	var payload struct {
		Schema string `json:"schema"`
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event map[string]any `json:"event"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		log.Logger().Error("[Lark] JSON解析失败", "err", err)
		return
	}

	// 心跳响应
	if payload.Header.EventType == "" {
		return
	}

	// 只处理消息事件
	if payload.Header.EventType != "im.message.receive_v1" {
		return
	}

	event := payload.Event
	message, _ := event["message"].(map[string]any)
	sender, _ := event["sender"].(map[string]any)
	senderId, _ := sender["sender_id"].(map[string]any)

	openID, _ := senderId["open_id"].(string)
	messageID, _ := message["message_id"].(string)
	msgType, _ := message["msg_type"].(string)

	// 只处理文本消息
	var content string
	if msgType == "text" {
		contentStr, _ := message["content"].(string)
		var contentObj map[string]any
		if json.Unmarshal([]byte(contentStr), &contentObj) == nil {
			content, _ = contentObj["text"].(string)
		}
	} else {
		return
	}

	if content == "" || openID == "" {
		return
	}

	log.Logger().Info("[Lark] 收到消息", "msg_id", messageID, "open_id", openID, "content", content)

	msg := Message{
		ID:        messageID,
		Channel:   l.name,
		From:      openID,
		Content:   content,
		Timestamp: time.Now().Unix(),
		Metadata: map[string]any{
			"open_id":    openID,
			"message_id": messageID,
			"msg_type":   msgType,
		},
	}

	// 存储会话信息用于主动发送
	l.sessionInfoMu.Lock()
	l.sessionInfo[openID] = larkSession{
		openID: openID,
	}
	l.sessionInfoMu.Unlock()

	l.PushMessage(msg)
}

// Send 发送响应（调用飞书消息API）
func (l *LarkChannel) Send(ctx context.Context, resp Response) error {
	token, err := l.getTenantAccessToken(ctx)
	if err != nil {
		return err
	}

	url := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id"
	reqBody := map[string]any{
		"receive_id": resp.To,
		"msg_type":   "text",
		"content":    string(mustJSON(map[string]string{"text": resp.Content})),
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	httpResp, err := l.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		respBody, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("飞书API返回 %d: %s", httpResp.StatusCode, truncateStr(string(respBody), 200))
	}

	log.Logger().Debug("[Lark] 消息已发送", "to", resp.To)
	return nil
}

// SendProactive 主动发送消息（飞书 WebSocket 模式）
func (l *LarkChannel) SendProactive(ctx context.Context, userID, content string) error {
	l.sessionInfoMu.RLock()
	session, ok := l.sessionInfo[userID]
	l.sessionInfoMu.RUnlock()

	if !ok {
		return fmt.Errorf("[Lark] 未找到用户 %s 的会话信息（用户需先与机器人对话一次）", userID)
	}

	token, err := l.getTenantAccessToken(ctx)
	if err != nil {
		return err
	}

	url := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id"
	reqBody := map[string]any{
		"receive_id": session.openID,
		"msg_type":   "text",
		"content":    string(mustJSON(map[string]string{"text": content})),
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	httpResp, err := l.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		respBody, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("飞书API返回 %d: %s", httpResp.StatusCode, truncateStr(string(respBody), 200))
	}

	log.Logger().Info("[Lark] 主动消息已发送", "user", userID)
	return nil
}

func (l *LarkChannel) SendToolEvent(event ToolEvent) error {
	return nil
}

// getTenantAccessToken 获取飞书 tenant_access_token
func (l *LarkChannel) getTenantAccessToken(ctx context.Context) (string, error) {
	l.tokenMu.RLock()
	if l.tokenCache != "" && time.Now().Before(l.tokenExpiry) {
		token := l.tokenCache
		l.tokenMu.RUnlock()
		return token, nil
	}
	l.tokenMu.RUnlock()

	url := "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
	reqBody := map[string]string{
		"app_id":     l.appID,
		"app_secret": l.appSecret,
	}
	jsonData, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Code             int    `json:"code"`
		Message          string `json:"message"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire           int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("飞书token错误: %s", result.Message)
	}

	l.tokenMu.Lock()
	l.tokenCache = result.TenantAccessToken
	l.tokenExpiry = time.Now().Add(time.Duration(result.Expire-300) * time.Second)
	l.tokenMu.Unlock()

	return result.TenantAccessToken, nil
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}