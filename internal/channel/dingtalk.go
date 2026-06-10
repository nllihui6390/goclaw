package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

	// 文件上传相关
	accessToken    string
	accessTokenExp time.Time
	accessTokenMu  sync.RWMutex
}

type dingtalkSession struct {
	conversationID string // chatid 用于主动发送
}

// NewDingTalkChannel 创建钉钉渠道
func NewDingTalkChannel(clientID, clientSecret string, display DisplayConfig) *DingTalkChannel {
	return &DingTalkChannel{
		BotChannelBase: NewBotChannelBase("dingtalk", "", display), // 不需要端口
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
	// 先停止基类（关闭 msgChan）
	d.BotChannelBase.Stop()
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
		Pictures         []struct {
			DownloadCode string `json:"downloadCode"`
			PreviewURL   string `json:"previewUrl"`
		} `json:"pictureUrls"`
		AudioContent     string `json:"audioContent"`
		VideoContent     string `json:"videoContent"`
		FileDownloadCode string `json:"fileDownloadCode"`
		FileName         string `json:"fileName"`
		RichTextContent  string `json:"richTextContent"`
		SenderStaffId    string `json:"senderStaffId"`
		SenderNick       string `json:"senderNick"`
		SessionWebhook   string `json:"sessionWebhook"`
		CreateTime       int64  `json:"createTime"`
		ChatId           string `json:"chatid"`
	}

	if err := json.Unmarshal(payload.Data, &msgData); err != nil {
		log.Logger().Error("[DingTalk] 消息数据解析失败", "err", err)
		return
	}

	// 处理不同消息类型
	var content string
	var blocks ContentBlocks
	switch msgData.MsgType {
	case "text":
		content = msgData.Text.Content
	case "picture":
		if len(msgData.Pictures) > 0 {
			content = "[image]"
			if msgData.Pictures[0].PreviewURL != "" {
				content = "[image_url:" + msgData.Pictures[0].PreviewURL + "]"
				blocks = append(blocks, NewImageBlockURL(msgData.Pictures[0].PreviewURL))
			}
		} else {
			content = "[image]"
		}
	case "audio":
		content = "[audio:" + msgData.AudioContent + "]"
	case "video":
		content = "[video:" + msgData.VideoContent + "]"
	case "file":
		fileName := msgData.FileName
		if fileName == "" {
			fileName = "[file]"
		}
		content = "[file:" + fileName + ",code:" + msgData.FileDownloadCode + "]"
	case "richText":
		content = "[richText]"
	default:
		content = "[" + msgData.MsgType + " message]"
	}

	if content == "" {
		return
	}

	msgID := fmt.Sprintf("dingtalk-%d", msgData.CreateTime)
	log.Logger().Info("[DingTalk] 收到消息", "msg_id", msgID, "sender", msgData.SenderNick, "msg_type", msgData.MsgType, "content", content)

	// 创建消息
	msg := Message{
		ID:        msgID,
		Channel:   d.name,
		From:      msgData.SenderStaffId,
		Content:   content,
		Timestamp: msgData.CreateTime / 1000,
		Blocks:    blocks,
		Metadata: map[string]any{
			"sender_nick":     msgData.SenderNick,
			"session_webhook": msgData.SessionWebhook,
			"sender_staff_id": msgData.SenderStaffId,
			"chat_id":         msgData.ChatId,
			"msg_type":        msgData.MsgType,
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

// Send 发送响应（通过钉钉 API）
func (d *DingTalkChannel) Send(ctx context.Context, resp Response) error {
	// 检查是否包含文件
	if strings.Contains(resp.Content, "[FILE_BLOCK]") {
		fileInfo := ParseFileBlock(resp.Content)
		if fileInfo != nil && fileInfo.Path != "" {
			// 上传文件
			mediaID, err := d.uploadFile(ctx, fileInfo.Path)
			if err == nil && mediaID != "" {
				// 发送文件消息
				err = d.sendFileMessage(ctx, resp.To, mediaID)
				if err == nil {
					log.Logger().Info("[DingTalk] 文件消息已发送", "to", resp.To, "filename", fileInfo.Filename)
					return nil
				}
				log.Logger().Warn("[DingTalk] 文件消息发送失败，回退到文本", "err", err)
			} else {
				log.Logger().Warn("[DingTalk] 文件上传失败，回退到文本", "err", err)
			}
		}
	}

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

// SendFile 实现 FileSender 接口 - 直接发送文件
func (d *DingTalkChannel) SendFile(ctx context.Context, to string, info *FileBlockInfo) (bool, error) {
	// URL 类型暂不支持直接发送，走回退
	if info.FileType == "url" {
		return false, nil
	}

	// 上传文件
	mediaID, err := d.uploadFile(ctx, info.Path)
	if err != nil {
		return true, fmt.Errorf("文件上传失败: %w", err)
	}

	// 发送文件消息
	if err := d.sendFileMessage(ctx, to, mediaID); err != nil {
		return true, err
	}

	log.Logger().Info("[DingTalk] 文件消息已发送", "to", to, "filename", info.Filename)
	return true, nil
}

// getAccessToken 获取钉钉 access_token
func (d *DingTalkChannel) getAccessToken(ctx context.Context) (string, error) {
	d.accessTokenMu.RLock()
	if d.accessToken != "" && time.Now().Before(d.accessTokenExp) {
		token := d.accessToken
		d.accessTokenMu.RUnlock()
		return token, nil
	}
	d.accessTokenMu.RUnlock()

	url := "https://api.dingtalk.com/v1.0/oauth2/accessToken"
	reqBody := map[string]string{
		"appKey":    d.clientID,
		"appSecret": d.clientSecret,
	}
	jsonData, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int64  `json:"expireIn"`
	}
	json.Unmarshal(body, &result)

	if result.AccessToken == "" {
		return "", fmt.Errorf("获取钉钉 access_token 失败")
	}

	d.accessTokenMu.Lock()
	d.accessToken = result.AccessToken
	d.accessTokenExp = time.Now().Add(time.Duration(result.ExpireIn-300) * time.Second)
	d.accessTokenMu.Unlock()

	return result.AccessToken, nil
}

// uploadFile 上传文件到钉钉，返回 mediaId
func (d *DingTalkChannel) uploadFile(ctx context.Context, filePath string) (string, error) {
	token, err := d.getAccessToken(ctx)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("media", filepath.Base(filePath))
	if err != nil {
		return "", err
	}
	part.Write(data)
	writer.Close()

	url := "https://api.dingtalk.com/v1.0/robot/oToMessages/mediaUpload?access_token=" + token
	req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := d.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		MediaId string `json:"mediaId"`
	}
	json.Unmarshal(respBody, &result)

	if result.MediaId == "" {
		return "", fmt.Errorf("钉钉上传文件失败: %s", string(respBody))
	}

	return result.MediaId, nil
}

// sendFileMessage 发送文件消息
func (d *DingTalkChannel) sendFileMessage(ctx context.Context, userID, mediaID string) error {
	url := "https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend"

	reqBody := map[string]any{
		"robotCode": d.clientID,
		"userIds":   []string{userID},
		"msgType":   "file",
		"msgParam":  string(mustJSON(map[string]string{"mediaId": mediaID})),
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("钉钉API返回 %d: %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}

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