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
	"strings"
	"sync"
	"time"

	"go-claw/internal/media"
	"go-claw/pkg/log"

	"github.com/gorilla/websocket"
)

// ─────────────────── DingTalk 机器人渠道（Stream v2 协议）───────────────────

// DingTalkChannel 钉钉机器人渠道（Stream WebSocket 模式，sessionWebhook 回复）
type DingTalkChannel struct {
	*BotChannelBase
	clientID     string
	clientSecret string
	botPrefix    string

	conn     *websocket.Conn
	connMu   sync.Mutex
	stopChan chan struct{}

	// 每个用户的 sessionWebhook（用于发回复）
	sessionWebhooks   map[string]dingtalkSession
	sessionWebhooksMu sync.RWMutex
}

type dingtalkSession struct {
	webhook  string // sessionWebhook URL
	senderID string
	chatID   string
}

// NewDingTalkChannel 创建钉钉渠道
func NewDingTalkChannel(clientID, clientSecret, botPrefix string, display DisplayConfig) *DingTalkChannel {
	return &DingTalkChannel{
		BotChannelBase:   NewBotChannelBase("dingtalk", "", display),
		clientID:         clientID,
		clientSecret:     clientSecret,
		botPrefix:        botPrefix,
		stopChan:         make(chan struct{}),
		sessionWebhooks:  make(map[string]dingtalkSession),
	}
}

// ─────────────────── Stream 连接 ───────────────────

type streamConnResp struct {
	Endpoint string `json:"endpoint"`
	Ticket   string `json:"ticket"`
}

func (d *DingTalkChannel) Start(ctx context.Context) error {
	log.Logger().Info("[DingTalk] 启动Stream模式")
	if err := d.connect(ctx); err != nil {
		return err
	}
	go d.receiveLoop(ctx)
	log.Logger().Info("[DingTalk] Stream连接成功")
	return nil
}

func (d *DingTalkChannel) Stop() error {
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

func (d *DingTalkChannel) connect(ctx context.Context) error {
	// Step 1: 注册连接获取 endpoint + ticket
	subs := []map[string]string{
		{"topic": "*", "type": "EVENT"},
		{"topic": "/v1.0/im/bot/messages/get", "type": "CALLBACK"},
	}
	body := map[string]interface{}{
		"clientId":     d.clientID,
		"clientSecret": d.clientSecret,
		"subscriptions": subs,
		"ua":           "go-claw/1.0",
	}
	jsonData, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://api.dingtalk.com/v1.0/gateway/connections/open",
		bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("获取Stream凭证失败: %v", err)
	}
	defer resp.Body.Close()

	var result streamConnResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析Stream凭证失败: %v", err)
	}
	if result.Endpoint == "" || result.Ticket == "" {
		return fmt.Errorf("钉钉返回空endpoint或ticket (status=%d)", resp.StatusCode)
	}

	// Step 2: WebSocket 连接
	wsURL := result.Endpoint
	if !strings.Contains(wsURL, "?") {
		wsURL += "?"
	} else {
		wsURL += "&"
	}
	wsURL += "ticket=" + result.Ticket

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %v", err)
	}

	d.connMu.Lock()
	if d.conn != nil {
		d.conn.Close()
	}
	d.conn = conn
	d.connMu.Unlock()
	return nil
}

// ─────────────────── 消息接收 ───────────────────

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
			log.Logger().Error("[DingTalk] WS读取错误", "err", err)
			time.Sleep(5 * time.Second)
			if err2 := d.connect(ctx); err2 != nil {
				log.Logger().Error("[DingTalk] 重连失败", "err", err2)
			}
			continue
		}

		d.handleStreamFrame(data)
	}
}

// streamFrame DingTalk Stream v1.0 协议帧
type streamFrame struct {
	SpecVersion string            `json:"specVersion"`
	Type        string            `json:"type"`
	Headers     map[string]string `json:"headers"`
	Data        string            `json:"data"` // JSON 字符串
}

// botMessageBody 机器人消息 body
type botMessageBody struct {
	MsgID          string `json:"msgId"`
	MsgType        string `json:"msgtype"`
	Text           struct {
		Content string `json:"content"`
	} `json:"text"`
	PictureUrls []struct {
		DownloadCode string `json:"downloadCode"`
		PreviewURL   string `json:"previewUrl"`
	} `json:"pictureUrls"`
	Attachments []struct {
		DownloadCode string `json:"downloadCode"`
		FileName     string `json:"fileName"`
		FileSize     int64  `json:"fileSize"`
	} `json:"attachments"`
	SenderStaffId  string `json:"senderStaffId"`
	SenderNick     string `json:"senderNick"`
	SessionWebhook string `json:"sessionWebhook"`
	CreateTime     int64  `json:"createTime"`
	ChatId         string `json:"chatid"`
	IsInAtList     bool   `json:"isInAtList"`
	RobotCode      string `json:"robotCode"`
}

func (d *DingTalkChannel) handleStreamFrame(data []byte) {
	var frame streamFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		log.Logger().Error("[DingTalk] 解析Stream帧失败", "err", err)
		return
	}

	topic := frame.Headers["topic"]
	log.Logger().Info("[DingTalk] Stream帧", "type", frame.Type, "topic", topic)

	// 发送 ACK 响应（防止钉钉重试）
	if frame.Headers["messageId"] != "" {
		d.sendAck(frame.Headers["messageId"])
	}

	if topic != "/v1.0/im/bot/messages/get" {
		return
	}

	var msg botMessageBody
	if err := json.Unmarshal([]byte(frame.Data), &msg); err != nil {
		log.Logger().Error("[DingTalk] 解析消息body失败", "err", err)
		return
	}

	d.handleBotMessage(&msg)
}

// sendAck 发送 ACK 响应给钉钉
func (d *DingTalkChannel) sendAck(messageID string) {
	ack := map[string]interface{}{
		"code":    200,
		"headers": map[string]string{"messageId": messageID},
		"message": "OK",
		"data":    "",
	}
	jsonData, _ := json.Marshal(ack)

	d.connMu.Lock()
	conn := d.conn
	d.connMu.Unlock()

	if conn != nil {
		if err := conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
			log.Logger().Warn("[DingTalk] 发送ACK失败", "err", err)
		}
	}
}

func (d *DingTalkChannel) handleBotMessage(msg *botMessageBody) {
	// 提取文本和媒体内容
	var content string
	var blocks ContentBlocks

	switch msg.MsgType {
	case "text":
		content = strings.TrimSpace(msg.Text.Content)

	case "picture":
		content = "[image]"
		if len(msg.PictureUrls) > 0 {
			if u := msg.PictureUrls[0].PreviewURL; u != "" {
				content = "[image]" // preview URL 可能有时效，标记为图片
				blocks = append(blocks, NewImageBlockURL(u))
			}
		}

	case "audio":
		content = "[audio]"

	case "video":
		content = "[video]"

	case "file":
		if len(msg.Attachments) > 0 {
			fn := msg.Attachments[0].FileName
			if fn == "" {
				fn = "file"
			}
			content = fmt.Sprintf("[file:%s]", fn)
		} else {
			content = "[file]"
		}

	case "richText":
		content = "[richText]"

	default:
		content = fmt.Sprintf("[%s message]", msg.MsgType)
	}

	if content == "" {
		return
	}

	msgID := msg.MsgID
	if msgID == "" {
		msgID = fmt.Sprintf("dingtalk-%d", time.Now().UnixNano())
	}
	senderID := msg.SenderStaffId
	if senderID == "" {
		senderID = "unknown"
	}

	log.Logger().Info("[DingTalk] 收到消息", "msg_id", msgID, "sender", senderID,
		"nick", msg.SenderNick, "type", msg.MsgType)

	// 存储 sessionWebhook 用于回复
	if msg.SessionWebhook != "" {
		d.sessionWebhooksMu.Lock()
		d.sessionWebhooks[senderID] = dingtalkSession{
			webhook:  msg.SessionWebhook,
			senderID: senderID,
			chatID:   msg.ChatId,
		}
		d.sessionWebhooksMu.Unlock()
	}

	chnMsg := Message{
		ID:        msgID,
		Channel:   d.name,
		From:      senderID,
		Content:   content,
		Timestamp: msg.CreateTime / 1000,
		Blocks:    blocks,
		Metadata: map[string]interface{}{
			"sender_nick":     msg.SenderNick,
			"sender_staff_id": senderID,
			"chat_id":         msg.ChatId,
			"msg_type":        msg.MsgType,
			"robot_code":      msg.RobotCode,
		},
	}

	d.PushMessage(chnMsg)
}

// ─────────────────── 发送回复（sessionWebhook）───────────────────

func (d *DingTalkChannel) Send(ctx context.Context, resp Response) error {
	content := resp.Content
	if strings.Contains(content, "[FILE_BLOCK]") {
		content = ExtractFileBlockDescription(content)
	}
	if d.botPrefix != "" {
		content = d.botPrefix + "  " + content
	}

	// 用 sessionWebhook 发送（不需要 access token）
	d.sessionWebhooksMu.RLock()
	sess, ok := d.sessionWebhooks[resp.To]
	d.sessionWebhooksMu.RUnlock()

	if ok && sess.webhook != "" {
		err := d.sendViaWebhook(ctx, sess.webhook, content)
		if err == nil {
			log.Logger().Info("[DingTalk] 消息已发送(webhook)", "to", resp.To)
			return nil
		}
		log.Logger().Warn("[DingTalk] webhook发送失败，回退到OpenAPI", "err", err)
	}

	// 回退：Open API batchSend（需要 access token）
	return d.sendViaOpenAPI(ctx, resp.To, content)
}

// sendViaWebhook 通过 sessionWebhook 发送（推荐方式）
func (d *DingTalkChannel) sendViaWebhook(ctx context.Context, webhook, content string) error {
	body := map[string]interface{}{
		"msgtype": "text",
		"text":    map[string]string{"content": content},
	}
	jsonData, _ := json.Marshal(body)
	return d.httpPost(ctx, webhook, jsonData, nil)
}

// sendViaOpenAPI 通过 Open API batchSend 发送（需要 access token）
func (d *DingTalkChannel) sendViaOpenAPI(ctx context.Context, userID, content string) error {
	token, err := d.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取access_token失败: %v", err)
	}

	reqBody := map[string]interface{}{
		"robotCode": d.clientID,
		"userIds":   []string{userID},
		"msgType":   "text",
		"msgParam":  toJSON(map[string]string{"content": content}),
	}
	jsonData, _ := json.Marshal(reqBody)

	return d.httpPost(ctx,
		"https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend",
		jsonData,
		map[string]string{"x-acs-dingtalk-access-token": token})
}

// ─────────────────── Access Token ───────────────────

var (
	dingAccessToken    string
	dingAccessTokenExp time.Time
	dingAccessTokenMu  sync.RWMutex
)

func (d *DingTalkChannel) getAccessToken(ctx context.Context) (string, error) {
	dingAccessTokenMu.RLock()
	if dingAccessToken != "" && time.Now().Before(dingAccessTokenExp) {
		tok := dingAccessToken
		dingAccessTokenMu.RUnlock()
		return tok, nil
	}
	dingAccessTokenMu.RUnlock()

	reqBody, _ := json.Marshal(map[string]string{
		"appKey":    d.clientID,
		"appSecret": d.clientSecret,
	})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://api.dingtalk.com/v1.0/oauth2/accessToken",
		bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int64  `json:"expireIn"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.AccessToken == "" {
		return "", fmt.Errorf("access_token为空")
	}

	dingAccessTokenMu.Lock()
	dingAccessToken = result.AccessToken
	dingAccessTokenExp = time.Now().Add(time.Duration(result.ExpireIn-300) * time.Second)
	dingAccessTokenMu.Unlock()

	return result.AccessToken, nil
}

// ─────────────────── 文件收发 ───────────────────

// SendFile 实现 FileSender 接口
// 注意：webhook 机器人不支持 media 上传 API（只有企业应用可以）
// 图片通过文件预览 URL 嵌入 markdown 发送
func (d *DingTalkChannel) SendFile(ctx context.Context, to string, info *FileBlockInfo) (bool, error) {
	d.sessionWebhooksMu.RLock()
	sess, ok := d.sessionWebhooks[to]
	d.sessionWebhooksMu.RUnlock()
	if !ok || sess.webhook == "" {
		return false, fmt.Errorf("无 sessionWebhook")
	}

	// 读取文件数据
	var data []byte
	var err error
	if info.FileType == "url" {
		data, err = d.downloadFile(ctx, info.Path)
	} else {
		data, err = os.ReadFile(info.Path)
	}
	if err != nil || len(data) == 0 {
		log.Logger().Warn("[DingTalk] 读取文件失败", "err", err)
		text := fmt.Sprintf("📎 %s（文件读取失败）", info.Filename)
		return true, d.sendViaWebhook(ctx, sess.webhook, text)
	}

	// 判断媒体类型
	mimeType := media.GetMediaType(info.Filename)
	uploadType := "file"
	if strings.HasPrefix(mimeType, "image/") {
		uploadType = "image"
	} else if strings.HasPrefix(mimeType, "audio/") {
		uploadType = "voice"
	} else if strings.HasPrefix(mimeType, "video/") {
		uploadType = "video"
	}

	// 上传到钉钉 OAPI media/upload
	mediaID, err := d.uploadMediaOAPI(ctx, data, uploadType, info.Filename)
	if err != nil {
		log.Logger().Warn("[DingTalk] 媒体上传失败", "err", err)
		text := fmt.Sprintf("📎 %s（上传失败: %v）", info.Filename, err)
		return true, d.sendViaWebhook(ctx, sess.webhook, text)
	}

	log.Logger().Info("[DingTalk] 文件上传成功", "media_id", mediaID, "type", uploadType)

	// 通过 Open API batchSend 发送文件（webhook 不支持 file/image msgtype）
	ext := "bin"
	if dot := strings.LastIndex(info.Filename, "."); dot != -1 {
		ext = info.Filename[dot+1:]
	}
	// 钉钉 batchSend 不支持 sampleImage/sampleImageMsg，统一用 sampleFile 发送
	// 图片会作为文件附件显示，点击可在浏览器中打开
	return true, d.batchSendOTO(ctx, to, "sampleFile",
		fmt.Sprintf(`{"mediaId":"%s","fileName":"%s","fileType":"%s"}`, mediaID, info.Filename, ext))
}

// batchSendOTO 通过 Open API 发送一对一消息（支持 sampleFile/sampleImageMsg 等）
func (d *DingTalkChannel) batchSendOTO(ctx context.Context, userID, msgKey, msgParam string) error {
	token, err := d.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取token失败: %v", err)
	}

	body := map[string]interface{}{
		"robotCode": d.clientID,
		"userIds":   []string{userID},
		"msgKey":    msgKey,
		"msgParam":  msgParam,
	}
	jsonData, _ := json.Marshal(body)

	log.Logger().Info("[DingTalk] batchSendOTO", "user", userID, "msgKey", msgKey, "paramLen", len(msgParam))

	err = d.httpPost(ctx,
		"https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend",
		jsonData,
		map[string]string{"x-acs-dingtalk-access-token": token})
	if err != nil {
		log.Logger().Error("[DingTalk] batchSendOTO失败", "err", err)
	}
	return err
}

// uploadMediaOAPI 通过钉钉旧版 OAPI 上传媒体文件，返回 media_id
func (d *DingTalkChannel) uploadMediaOAPI(ctx context.Context, data []byte, mediaType, filename string) (string, error) {
	token, err := d.getAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("获取token失败: %v", err)
	}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, _ := w.CreateFormFile("media", filename)
	part.Write(data)
	w.Close()

	url := fmt.Sprintf("https://oapi.dingtalk.com/media/upload?access_token=%s&type=%s", token, mediaType)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, &b)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := d.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		MediaID string `json:"media_id"`
	}
	json.Unmarshal(body, &result)

	if result.ErrCode != 0 || result.MediaID == "" {
		return "", fmt.Errorf("errcode=%d %s", result.ErrCode, string(body))
	}
	return result.MediaID, nil
}

// downloadFile 从 URL 下载文件
func (d *DingTalkChannel) downloadFile(ctx context.Context, urlStr string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	resp, err := d.HTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (d *DingTalkChannel) SendProactive(ctx context.Context, userID, content string) error {
	token, err := d.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取access_token失败: %v", err)
	}

	reqBody := map[string]interface{}{
		"robotCode": d.clientID,
		"userIds":   []string{userID},
		"msgType":   "text",
		"msgParam":  toJSON(map[string]string{"content": content}),
	}
	jsonData, _ := json.Marshal(reqBody)

	return d.httpPost(ctx,
		"https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend",
		jsonData,
		map[string]string{"x-acs-dingtalk-access-token": token})
}

func (d *DingTalkChannel) SendToolEvent(event ToolEvent) error {
	return nil
}

// ─────────────────── Helpers ───────────────────

func (d *DingTalkChannel) httpPost(ctx context.Context, url string, body []byte, headers map[string]string) error {
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := d.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("钉钉API返回 %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	return nil
}

func toJSON(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
