package channel

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-claw/pkg/log"
)

const (
	wechatDefaultBaseURL = "https://ilinkai.weixin.qq.com"
	wechatChannelVersion = "2.0.1"
	wechatGetUpdatesTimeout = 45 // 长轮询超时秒数
	wechatPollInterval      = 1  // 轮询间隔秒数
)

// WeChatChannel 微信个人 iLink Bot 渠道
type WeChatChannel struct {
	*BotChannelBase
	botToken    string
	botPrefix   string
	baseURL     string
	mediaDir    string
	tokenFile   string

	client      *http.Client
	stopChan    chan struct{}

	// 上下文 token（回复消息必需）
	contextTokens   map[string]string // user_id -> context_token
	contextTokensMu sync.RWMutex

	// 消息去重
	processedIDs   map[string]time.Time
	processedIDsMu sync.Mutex

	// 扫码登录相关
	qrCode       string
	loginDone    chan struct{}
}

// NewWeChatChannel 创建微信渠道
func NewWeChatChannel(botToken, botPrefix, baseURL, mediaDir, tokenFile string, display DisplayConfig) *WeChatChannel {
	if baseURL == "" {
		baseURL = wechatDefaultBaseURL
	}
	if tokenFile == "" {
		tokenFile = "clawdata/wechat_bot_token"
	}
	return &WeChatChannel{
		BotChannelBase: NewBotChannelBase("wechat", "", display),
		botToken:       botToken,
		botPrefix:      botPrefix,
		baseURL:        baseURL,
		mediaDir:       mediaDir,
		tokenFile:      tokenFile,
		client:         &http.Client{Timeout: time.Duration(wechatGetUpdatesTimeout) * time.Second},
		stopChan:       make(chan struct{}),
		contextTokens:  make(map[string]string),
		processedIDs:   make(map[string]time.Time),
		loginDone:      make(chan struct{}),
	}
}

func (w *WeChatChannel) Start(ctx context.Context) error {
	// 尝试从文件加载 token
	if w.botToken == "" {
		w.loadTokenFromFile()
	}

	// 如果还是没有 token，尝试扫码登录
	if w.botToken == "" {
		log.Logger().Info("[WeChat] 未配置 bot_token，尝试扫码登录")
		if err := w.qrCodeLogin(ctx); err != nil {
			return fmt.Errorf("[WeChat] 扫码登录失败: %v", err)
		}
	}

	// 加载持久化的 context_tokens
	w.loadContextTokens()

	log.Logger().Info("[WeChat] iLink Bot 已启动", "base_url", w.baseURL)
	go w.pollLoop(ctx)
	return nil
}

func (w *WeChatChannel) Stop() error {
	// 先停止基类（关闭 msgChan）
	w.BotChannelBase.Stop()
	close(w.stopChan)
	log.Logger().Info("[WeChat] 已停止")
	return nil
}

// ─────────────────── Token 管理 ───────────────────

func (w *WeChatChannel) loadTokenFromFile() {
	data, err := os.ReadFile(w.tokenFile)
	if err != nil {
		return
	}
	token := strings.TrimSpace(string(data))
	if token != "" {
		w.botToken = token
		log.Logger().Info("[WeChat] 已从文件加载 bot_token", "file", w.tokenFile)
	}
}

func (w *WeChatChannel) saveTokenToFile() {
	os.MkdirAll(filepath.Dir(w.tokenFile), 0755)
	os.WriteFile(w.tokenFile, []byte(w.botToken), 0600)
}

// contextTokenFile 返回 context_tokens 持久化文件路径
func (w *WeChatChannel) contextTokenFile() string {
	return filepath.Join(filepath.Dir(w.tokenFile), "wechat_context_tokens.json")
}

func (w *WeChatChannel) loadContextTokens() {
	data, err := os.ReadFile(w.contextTokenFile())
	if err != nil {
		return
	}
	var tokens map[string]string
	if err := json.Unmarshal(data, &tokens); err != nil {
		return
	}
	w.contextTokensMu.Lock()
	for k, v := range tokens {
		w.contextTokens[k] = v
	}
	w.contextTokensMu.Unlock()
	log.Logger().Info("[WeChat] 已加载 context_tokens", "count", len(tokens))
}

func (w *WeChatChannel) saveContextTokens() {
	w.contextTokensMu.RLock()
	data, _ := json.Marshal(w.contextTokens)
	w.contextTokensMu.RUnlock()
	os.MkdirAll(filepath.Dir(w.contextTokenFile()), 0755)
	os.WriteFile(w.contextTokenFile(), data, 0644)
}

// ─────────────────── 扫码登录 ───────────────────

func (w *WeChatChannel) qrCodeLogin(ctx context.Context) error {
	// 1. 获取二维码
	resp, err := w.apiGet(ctx, "/ilink/bot/get_bot_qrcode?bot_type=3")
	if err != nil {
		return fmt.Errorf("获取二维码失败: %v", err)
	}
	qrcodeStr, _ := resp["qrcode"].(string)
	w.qrCode = qrcodeStr

	// 在控制台显示二维码
	qrURL, _ := resp["url"].(string)
	qrImgField, _ := resp["qrcode_img_content"].(string)

	if qrURL != "" {
		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════╗")
		fmt.Println("║          WeChat iLink Bot 扫码登录                        ║")
		fmt.Println("╠══════════════════════════════════════════════════════════╣")
		fmt.Printf("║  QR Code URL: %-44s ║\n", qrURL)
		fmt.Println("║                                                          ║")
		fmt.Println("║  请用手机微信扫描上述链接中的二维码完成登录                    ║")
		fmt.Println("╚══════════════════════════════════════════════════════════╝")
		fmt.Println()
		log.Logger().Info("[WeChat] 请在控制台扫描二维码登录", "url", qrURL)
	} else if qrImgField != "" {
		if strings.HasPrefix(qrImgField, "http") {
			fmt.Println()
			fmt.Println("╔══════════════════════════════════════════════════════════╗")
			fmt.Println("║          WeChat iLink Bot 扫码登录                        ║")
			fmt.Println("╠══════════════════════════════════════════════════════════╣")
			fmt.Printf("║  QR Code URL: %-44s ║\n", qrImgField)
			fmt.Println("║                                                          ║")
			fmt.Println("║  请用手机微信扫描上述链接中的二维码完成登录                    ║")
			fmt.Println("╚══════════════════════════════════════════════════════════╝")
			fmt.Println()
		} else {
			// base64 解码保存为文件
			imgData, err := base64.StdEncoding.DecodeString(qrImgField)
			if err != nil {
				imgData, err = base64.RawStdEncoding.DecodeString(qrImgField)
			}
			if err != nil {
				log.Logger().Warn("[WeChat] 二维码解码失败", "preview", qrImgField[:min(100, len(qrImgField))])
			} else {
				qrFile := filepath.Join(filepath.Dir(w.tokenFile), "wechat_qrcode.png")
				os.MkdirAll(filepath.Dir(qrFile), 0755)
				os.WriteFile(qrFile, imgData, 0644)
				fmt.Println()
				fmt.Println("╔══════════════════════════════════════════════════════════╗")
				fmt.Println("║          WeChat iLink Bot 扫码登录                        ║")
				fmt.Println("╠══════════════════════════════════════════════════════════╣")
				fmt.Printf("║  二维码已保存到: %-40s ║\n", qrFile)
				fmt.Println("║                                                          ║")
				fmt.Println("║  请用手机微信扫描该二维码文件完成登录                         ║")
				fmt.Println("╚══════════════════════════════════════════════════════════╝")
				fmt.Println()
			}
		}
	} else {
		log.Logger().Warn("[WeChat] 未获取到二维码，API 响应", "resp", fmt.Sprintf("%v", resp))
	}
	log.Logger().Info("[WeChat] 等待扫码确认...", "qrcode", qrcodeStr[:8]+"...")

	// 2. 轮询等待扫码确认
	pollInterval := 2 * time.Second
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.stopChan:
			return fmt.Errorf("渠道已停止")
		case <-time.After(pollInterval):
		}

		resp, err := w.apiGet(ctx, "/ilink/bot/get_qrcode_status?qrcode="+w.qrCode)
		if err != nil {
			log.Logger().Info("[WeChat] 查询扫码状态失败", "err", err)
			continue
		}
		status, _ := resp["status"].(string)
		switch status {
		case "confirmed":
			w.botToken, _ = resp["bot_token"].(string)
			if baseURL, ok := resp["baseurl"].(string); ok && baseURL != "" {
				w.baseURL = baseURL
			}
			w.saveTokenToFile()
			log.Logger().Info("[WeChat] 扫码登录成功")
			return nil
		case "expired":
			return fmt.Errorf("二维码已过期，请重新启动")
		case "scanned":
			log.Logger().Info("[WeChat] 已扫描，请在手机上确认登录")
		}
	}
	return fmt.Errorf("扫码登录超时（5分钟）")
}

// ─────────────────── API 请求 ───────────────────

func (w *WeChatChannel) makeHeaders() map[string]string {
	uinBytes := make([]byte, 4)
	rand.Read(uinBytes)
	uinB64 := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", uint32(uinBytes[0])<<24|uint32(uinBytes[1])<<16|uint32(uinBytes[2])<<8|uint32(uinBytes[3]))))

	headers := map[string]string{
		"Content-Type":     "application/json",
		"AuthorizationType": "ilink_bot_token",
		"X-WECHAT-UIN":     uinB64,
	}
	if w.botToken != "" {
		headers["Authorization"] = "Bearer " + w.botToken
	}
	return headers
}

func (w *WeChatChannel) apiGet(ctx context.Context, path string) (map[string]any, error) {
	url := w.baseURL + path
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	for k, v := range w.makeHeaders() {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(body, &result)
	return result, nil
}

func (w *WeChatChannel) apiPost(ctx context.Context, path string, body map[string]any) (map[string]any, error) {
	jsonData, _ := json.Marshal(body)
	url := w.baseURL + path
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	for k, v := range w.makeHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return result, nil
}

// ─────────────────── 消息轮询 ───────────────────

func (w *WeChatChannel) pollLoop(ctx context.Context) {
	cursor := ""
	for {
		select {
		case <-w.stopChan:
			return
		case <-ctx.Done():
			return
		default:
		}

		log.Logger().Debug("[WeChat] 开始拉取消息", "cursor", cursor)
		msgs, newCursor, err := w.getUpdates(ctx, cursor)
		if err != nil {
			log.Logger().Error("[WeChat] 拉取消息失败", "err", err)
			time.Sleep(time.Duration(wechatPollInterval) * time.Second)
			continue
		}
		log.Logger().Debug("[WeChat] 拉取消息返回", "msgs_count", len(msgs), "new_cursor", newCursor)
		cursor = newCursor

		for _, msg := range msgs {
			w.handleMessage(msg)
		}
	}
}

func (w *WeChatChannel) getUpdates(ctx context.Context, cursor string) ([]map[string]any, string, error) {
	body := map[string]any{
		"get_updates_buf": cursor,
		"base_info": map[string]string{
			"channel_version": wechatChannelVersion,
		},
	}

	resp, err := w.apiPost(ctx, "/ilink/bot/getupdates", body)
	if err != nil {
		return nil, cursor, err
	}

	// 记录返回的关键字段用于调试
	retVal, _ := resp["ret"].(float64)
	log.Logger().Debug("[WeChat] getUpdates 响应", "ret", int(retVal), "has_msgs", resp["msgs"] != nil)

	msgsRaw, _ := resp["msgs"].([]any)
	var msgs []map[string]any
	for _, m := range msgsRaw {
		if msgMap, ok := m.(map[string]any); ok {
			msgs = append(msgs, msgMap)
		}
	}

	newCursor, _ := resp["get_updates_buf"].(string)
	if newCursor == "" {
		newCursor = cursor // 保持旧 cursor 避免回到起点
	}
	return msgs, newCursor, nil
}

// ─────────────────── 消息解析 ───────────────────

type wechatInMsg struct {
	MsgID        string
	FromUserID   string // 发送者 ID (xxx@im.wechat)
	ToUserID     string // 机器人 ID
	GroupID      string // 群 ID（群聊时）
	Content      string
	MsgType      int    // 1=text, 2=image, 3=voice, 4=file, 5=video
	ContextToken string // 回复时必须携带
	Timestamp    int64
	ImageURL     string // 图片 URL（用于构建 ImageBlock）
}

func (w *WeChatChannel) parseMessage(msg map[string]any) *wechatInMsg {
	m := &wechatInMsg{}

	// 消息 ID
	if msgID, ok := msg["msg_id"].(string); ok {
		m.MsgID = msgID
	}

	// 用户信息
	if fromUser, ok := msg["from_user_id"].(string); ok {
		m.FromUserID = fromUser
	}
	if toUser, ok := msg["to_user_id"].(string); ok {
		m.ToUserID = toUser
	}

	// 群信息
	if groupID, ok := msg["group_id"].(string); ok {
		m.GroupID = groupID
	}

	// context_token（回复必需）
	if ct, ok := msg["context_token"].(string); ok {
		m.ContextToken = ct
	}

	// 解析 item_list
	items, _ := msg["item_list"].([]any)
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(float64)
		switch int(itemType) {
		case 1: // text
			if textItem, ok := itemMap["text_item"].(map[string]any); ok {
				m.Content, _ = textItem["text"].(string)
			}
			m.MsgType = 1
		case 2: // image
			m.MsgType = 2
			if imageItem, ok := itemMap["image_item"].(map[string]any); ok {
				if media, ok := imageItem["media"].(map[string]any); ok {
					// 尝试获取图片 URL
					if url, ok := media["url"].(string); ok && url != "" {
						m.Content = url
						m.ImageURL = url
					} else if cdnUrl, ok := media["cdn_url"].(string); ok && cdnUrl != "" {
						m.Content = "[image]"
						m.ImageURL = cdnUrl
					} else {
						// 有 media 但无 URL，标记为图片
						m.Content = "[image]"
					}
				}
			}
			// 兜底：尝试从 item 中直接获取 url
			if m.Content == "" || m.Content == "[image]" {
				if url, ok := itemMap["url"].(string); ok && url != "" {
					m.Content = "[image]"
					m.ImageURL = url
				}
			}
		case 4: // file
			m.MsgType = 4
			if fileItem, ok := itemMap["file_item"].(map[string]any); ok {
				m.Content, _ = fileItem["file_name"].(string)
			}
		default:
			m.MsgType = int(itemType)
		}
	}

	// 时间戳
	if ts, ok := msg["create_time"].(float64); ok {
		m.Timestamp = int64(ts)
	}

	return m
}

// ─────────────────── 消息处理 ───────────────────

func (w *WeChatChannel) handleMessage(msg map[string]any) {
	m := w.parseMessage(msg)

	// 打印解析结果用于调试
	log.Logger().Debug("[WeChat] 消息解析结果", "msg_id", m.MsgID[:min(16, len(m.MsgID))],
		"from", m.FromUserID[:min(20, len(m.FromUserID))],
		"type", m.MsgType, "content_len", len(m.Content), "has_token", m.ContextToken != "")

	if m.FromUserID == "" {
		log.Logger().Info("[WeChat] 跳过消息（无发送者）")
		return
	}

	// 去重（msg_id 可能为空，用 context_token 作为唯一标识）
	dedupKey := m.MsgID
	if dedupKey == "" {
		dedupKey = m.ContextToken
	}
	if dedupKey == "" {
		// 最后兜底：内容+时间戳哈希
		dedupKey = fmt.Sprintf("%s_%d", m.Content[:min(32, len(m.Content))], m.Timestamp)
	}
	w.processedIDsMu.Lock()
	if _, exists := w.processedIDs[dedupKey]; exists {
		w.processedIDsMu.Unlock()
		return
	}
	w.processedIDs[dedupKey] = time.Now()
	// 定期清理过期 ID
	if len(w.processedIDs) > 2000 {
		cutoff := time.Now().Add(-30 * time.Minute)
		for id, t := range w.processedIDs {
			if t.Before(cutoff) {
				delete(w.processedIDs, id)
			}
		}
	}
	w.processedIDsMu.Unlock()

	// 保存 context_token 并持久化
	if m.ContextToken != "" {
		w.contextTokensMu.Lock()
		w.contextTokens[m.FromUserID] = m.ContextToken
		w.contextTokensMu.Unlock()
		w.saveContextTokens()
	}

	// 确定会话 ID
	sessionKey := m.FromUserID
	if m.GroupID != "" {
		sessionKey = "group:" + m.GroupID
	}

	if m.MsgType == 1 && m.Content != "" {
		log.Logger().Info("[WeChat] 收到文本消息", "from", m.FromUserID[:20]+"...", "content", m.Content)
	} else {
		log.Logger().Info("[WeChat] 收到消息", "from", m.FromUserID[:20]+"...", "type", m.MsgType)
	}

	channelMsg := Message{
		ID:        m.MsgID,
		Channel:   w.name,
		From:      sessionKey,
		Content:   m.Content,
		Timestamp: m.Timestamp / 1000,
		Metadata: map[string]any{
			"wechat_user_id": m.FromUserID,
			"group_id":       m.GroupID,
			"msg_type":       m.MsgType,
			"context_token":  m.ContextToken,
		},
	}

	// 构建 ContentBlocks（图片等多模态）
	if m.ImageURL != "" {
		channelMsg.Blocks = append(channelMsg.Blocks, NewImageBlockURL(m.ImageURL))
	}

	w.PushMessage(channelMsg)
}

// ─────────────────── Send 发送消息 ───────────────────

func (w *WeChatChannel) Send(ctx context.Context, resp Response) error {
	// 处理 [FILE_BLOCK] — 微信暂不支持直接发送文件，转为文本描述
	content := resp.Content
	if strings.Contains(content, "[FILE_BLOCK]") {
		content = ExtractFileBlockDescription(content)
	}
	content = w.botPrefix + content

	// 获取 context_token
	w.contextTokensMu.RLock()
	contextToken := w.contextTokens[resp.To]
	w.contextTokensMu.RUnlock()

	if contextToken == "" {
		return fmt.Errorf("[WeChat] 未找到用户 %s 的 context_token", resp.To)
	}

	// 微信文本限制 2048 字符，超过分批发送
	chunks := splitWechatText(content, 2000)
	for _, chunk := range chunks {
		if err := w.sendText(ctx, resp.To, chunk, contextToken); err != nil {
			log.Logger().Error("[WeChat] 发送消息失败", "err", err)
			return err
		}
	}

	log.Logger().Debug("[WeChat] 消息已发送", "to", resp.To[:20]+"...")
	return nil
}

func (w *WeChatChannel) sendText(ctx context.Context, toUserID, text, contextToken string) error {
	clientID := generateWechatClientID()

	body := map[string]any{
		"msg": map[string]any{
			"to_user_id":    toUserID,
			"client_id":     clientID,
			"message_type":  2,
			"message_state": 2,
			"context_token": contextToken,
			"item_list": []map[string]any{
				{
					"type": 1,
					"text_item": map[string]string{
						"text": text,
					},
				},
			},
		},
		"base_info": map[string]string{
			"channel_version": wechatChannelVersion,
		},
	}

	resp, err := w.apiPost(ctx, "/ilink/bot/sendmessage", body)
	if err != nil {
		return err
	}

	ret, _ := resp["ret"].(float64)
	if int(ret) != 0 {
		errMsg, _ := resp["errmsg"].(string)
		return fmt.Errorf("sendmessage failed: ret=%d, msg=%s", int(ret), errMsg)
	}

	return nil
}

// SendProactive 主动发送消息（使用持久化的 context_token）
func (w *WeChatChannel) SendProactive(ctx context.Context, userID, content string) error {
	w.contextTokensMu.RLock()
	contextToken := w.contextTokens[userID]
	w.contextTokensMu.RUnlock()

	if contextToken == "" {
		return fmt.Errorf("[WeChat] 未找到 %s 的 context_token（需要用户先发一条消息）", userID[:min(20, len(userID))])
	}

	content = w.botPrefix + content
	chunks := splitWechatText(content, 2000)
	for _, chunk := range chunks {
		if err := w.sendText(ctx, userID, chunk, contextToken); err != nil {
			return err
		}
	}

	log.Logger().Info("[WeChat] 主动消息已发送", "user", userID[:20]+"...")
	return nil
}

func (w *WeChatChannel) SendToolEvent(event ToolEvent) error {
	return nil // 微信不支持工具事件显示
}

// ─────────────────── 辅助函数 ───────────────────

func generateWechatClientID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func splitWechatText(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}
	var chunks []string
	for len(text) > maxLen {
		chunks = append(chunks, text[:maxLen])
		text = text[maxLen:]
	}
	chunks = append(chunks, text)
	return chunks
}

// ─────────────────── 文件发送 (FileSender 接口) ───────────────────

// pkcs7Pad PKCS7 填充
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// aesECBEncrypt AES-ECB 加密
func aesECBEncrypt(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	data = pkcs7Pad(data, aes.BlockSize)
	encrypted := make([]byte, len(data))
	for i := 0; i < len(data); i += aes.BlockSize {
		block.Encrypt(encrypted[i:i+aes.BlockSize], data[i:i+aes.BlockSize])
	}
	return encrypted, nil
}

// detectWechatMediaType 根据扩展名判断媒体类型: 1=image, 2=video, 3=file, 4=voice
func detectWechatMediaType(filename string) int {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return 1 // image
	case ".mp4", ".mov", ".avi", ".mkv":
		return 2 // video
	case ".amr", ".mp3", ".wav", ".ogg", ".opus":
		return 4 // voice
	default:
		return 3 // file
	}
}

// uploadMedia 上传文件到微信 CDN，返回 sendmessage 所需的 media 字段
func (w *WeChatChannel) uploadMedia(ctx context.Context, filePath, toUserID string) (map[string]any, error) {
	// 读取原始文件
	rawData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %v", err)
	}

	rawSize := len(rawData)
	rawMD5 := fmt.Sprintf("%x", md5.Sum(rawData))
	mediaType := detectWechatMediaType(filePath)

	// 生成 AES 密钥 (16 字节)
	aesKeyRaw := make([]byte, 16)
	rand.Read(aesKeyRaw)
	aesKeyHex := hex.EncodeToString(aesKeyRaw)                           // 32 hex chars → API 用
	aesKeyForMsg := base64.StdEncoding.EncodeToString([]byte(aesKeyHex)) // base64(hex) → 消息体用
	filekey := hex.EncodeToString(randomBytes(16))                       // 16 bytes random hex

	// AES-ECB 加密
	encryptedData, err := aesECBEncrypt(rawData, aesKeyRaw)
	if err != nil {
		return nil, fmt.Errorf("AES加密失败: %v", err)
	}
	encryptedSize := len(encryptedData)

	// 获取上传 URL
	uploadResp, err := w.apiPost(ctx, "/ilink/bot/getuploadurl", map[string]any{
		"filekey":      filekey,
		"media_type":   mediaType,
		"to_user_id":   toUserID,
		"rawsize":      rawSize,
		"rawfilemd5":   rawMD5,
		"filesize":     encryptedSize,
		"aeskey":       aesKeyHex,
		"no_need_thumb": true,
		"base_info":    map[string]string{"channel_version": wechatChannelVersion},
	})
	if err != nil {
		return nil, fmt.Errorf("获取上传URL失败: %v", err)
	}

	// 获取上传地址
	uploadURL, _ := uploadResp["upload_full_url"].(string)
	if uploadURL == "" {
		uploadParam, _ := uploadResp["upload_param"].(string)
		if uploadParam != "" {
			uploadURL = fmt.Sprintf("https://novac2c.cdn.weixin.qq.com/c2c/upload?encrypted_query_param=%s&filekey=%s",
				uploadParam, filekey)
		}
	}
	if uploadURL == "" {
		return nil, fmt.Errorf("未获取到上传地址: %v", uploadResp)
	}

	// HTTP PUT 上传加密文件到 CDN
	uploadReq, _ := http.NewRequestWithContext(ctx, "POST", uploadURL, bytes.NewReader(encryptedData))
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	client := &http.Client{Timeout: 120 * time.Second}
	uploadHttpResp, err := client.Do(uploadReq)
	if err != nil {
		return nil, fmt.Errorf("CDN上传失败: %v", err)
	}
	defer uploadHttpResp.Body.Close()

	if uploadHttpResp.StatusCode != 200 {
		body, _ := io.ReadAll(uploadHttpResp.Body)
		return nil, fmt.Errorf("CDN上传返回 %d: %s", uploadHttpResp.StatusCode, string(body))
	}

	// 从响应头获取 encrypt_query_param
	encryptQueryParam := uploadHttpResp.Header.Get("X-Encrypted-Param")
	if encryptQueryParam == "" {
		encryptQueryParam = uploadHttpResp.Header.Get("x-encrypted-param")
	}
	if encryptQueryParam == "" {
		return nil, fmt.Errorf("CDN未返回 encrypt_query_param")
	}

	log.Logger().Info("[WeChat] 文件上传成功", "filename", filepath.Base(filePath),
		"raw_size", rawSize, "encrypted_size", encryptedSize)

	return map[string]any{
		"encrypt_query_param": encryptQueryParam,
		"aes_key":             aesKeyForMsg,
		"filesize":            encryptedSize,
	}, nil
}

// SendFile 实现 FileSender 接口
func (w *WeChatChannel) SendFile(ctx context.Context, to string, info *FileBlockInfo) (bool, error) {
	if info.FileType == "url" {
		return false, nil // URL 类型需要额外处理
	}

	// 获取 context_token
	w.contextTokensMu.RLock()
	contextToken := w.contextTokens[to]
	w.contextTokensMu.RUnlock()
	if contextToken == "" {
		return true, fmt.Errorf("未找到用户 %s 的 context_token", to)
	}

	// 上传文件到 CDN
	media, err := w.uploadMedia(ctx, info.Path, to)
	if err != nil {
		return true, err
	}

	// 发送文件消息
	clientID := generateWechatClientID()
	filename := info.Filename
	if filename == "" {
		filename = filepath.Base(info.Path)
	}

	itemType := 4 // file
	itemKey := "file_item"
	itemBody := map[string]any{
		"media": map[string]any{
			"encrypt_query_param": media["encrypt_query_param"],
			"aes_key":             media["aes_key"],
			"encrypt_type":        1,
		},
		"file_name": filename,
		"len":       fmt.Sprintf("%d", media["filesize"]),
	}

	// 图片使用 image_item
	if detectWechatMediaType(info.Path) == 1 {
		itemType = 2
		itemKey = "image_item"
		itemBody = map[string]any{
			"media": map[string]any{
				"encrypt_query_param": media["encrypt_query_param"],
				"aes_key":             media["aes_key"],
				"encrypt_type":        1,
			},
			"mid_size": media["filesize"],
		}
	}

	body := map[string]any{
		"msg": map[string]any{
			"to_user_id":    to,
			"client_id":     clientID,
			"message_type":  2,
			"message_state": 2,
			"context_token": contextToken,
			"item_list": []map[string]any{
				{
					"type":     itemType,
					itemKey: itemBody,
				},
			},
		},
		"base_info": map[string]string{
			"channel_version": wechatChannelVersion,
		},
	}

	resp, err := w.apiPost(ctx, "/ilink/bot/sendmessage", body)
	if err != nil {
		return true, err
	}

	ret, _ := resp["ret"].(float64)
	if int(ret) != 0 {
		errMsg, _ := resp["errmsg"].(string)
		return true, fmt.Errorf("发送文件失败: ret=%d, msg=%s", int(ret), errMsg)
	}

	log.Logger().Info("[WeChat] 文件消息已发送", "to", to[:20]+"...", "filename", filename)
	return true, nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

