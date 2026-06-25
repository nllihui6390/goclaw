package channel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go-claw/internal/media"
	"go-claw/pkg/log"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// LarkChannel 飞书机器人渠道（使用官方 SDK WebSocket 长连接模式）
type LarkChannel struct {
	*BotChannelBase
	appID     string
	appSecret string
	botPrefix string

	// 官方 SDK WebSocket 客户端
	wsClient *larkws.Client

	// HTTP 客户端（用于发送消息和上传文件，SDK 内部管理 token）
	larkClient *lark.Client

	stopChan chan struct{}

	sessionInfo        map[string]larkSession
	sessionInfoMu      sync.RWMutex
	pendingReactions   map[string]string // open_id -> message_id (用于处理后清除 reaction)
	pendingReactionsMu sync.Mutex
}

type larkSession struct {
	openID string // 用户 open_id 用于主动发送
}

// NewLarkChannel 创建飞书渠道
func NewLarkChannel(appID, appSecret, botPrefix string, display DisplayConfig) *LarkChannel {
	return &LarkChannel{
		BotChannelBase:   NewBotChannelBase("lark", "", display),
		appID:            appID,
		appSecret:        appSecret,
		botPrefix:        botPrefix,
		stopChan:         make(chan struct{}),
		sessionInfo:      make(map[string]larkSession),
		pendingReactions: make(map[string]string),
		// larkClient 在 Start() 中创建（需要屏蔽 SDK 日志）
	}
}

func (l *LarkChannel) Start(ctx context.Context) error {
	log.Logger().Info("[Lark] 启动官方SDK WebSocket长连接模式")

	// 飞书 SDK 内部日志硬编码输出到 os.Stdout，初始化期间临时重定向以屏蔽
	realStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)

	// 创建 HTTP 客户端
	l.larkClient = lark.NewClient(l.appID, l.appSecret)

	// 创建事件处理器
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			l.handleLarkMessage(ctx, event)
			return nil
		})

	// 创建 WebSocket 客户端
	l.wsClient = larkws.NewClient(l.appID, l.appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogger(&noopLogger{}),
	)

	// 恢复 stdout
	os.Stdout = realStdout

	// 启动客户端
	go func() {
		err := l.wsClient.Start(ctx)
		if err != nil {
			log.Logger().Error("[Lark] WebSocket客户端启动失败", "err", err)
		}
	}()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	log.Logger().Info("[Lark] WebSocket连接已启动")
	return nil
}

// noopLogger 空操作日志记录器，实现 larkcore.Logger 接口，用于屏蔽 SDK 内部日志
type noopLogger struct{}

func (n *noopLogger) Debug(ctx context.Context, args ...interface{}) {}
func (n *noopLogger) Info(ctx context.Context, args ...interface{})  {}
func (n *noopLogger) Warn(ctx context.Context, args ...interface{})  {}
func (n *noopLogger) Error(ctx context.Context, args ...interface{}) {}

// handleLarkMessage 处理飞书消息事件
func (l *LarkChannel) handleLarkMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) {
	senderID := event.Event.Sender.SenderId.OpenId
	messageID := event.Event.Message.MessageId
	msgType := event.Event.Message.MessageType

	chatID := event.Event.Message.ChatId
	chatType := event.Event.Message.ChatType

	// 会话ID区分单聊和群聊，群聊加 app 后缀防止多机器人冲突
	sessionKey := *senderID
	if chatType != nil && *chatType == "group" {
		appSuffix := l.appID
		if len(appSuffix) > 4 {
			appSuffix = appSuffix[len(appSuffix)-4:]
		}
		sessionKey = fmt.Sprintf("%s_%s", appSuffix, *chatID)
	}

	// 存储会话信息（用 sessionKey 索引，包含 open_id 用于发送回复）
	l.sessionInfoMu.Lock()
	l.sessionInfo[sessionKey] = larkSession{
		openID: *senderID,
	}
	l.sessionInfoMu.Unlock()

	// 记录消息ID用于后续移除 reaction
	l.pendingReactionsMu.Lock()
	l.pendingReactions[sessionKey] = *messageID
	l.pendingReactionsMu.Unlock()

	// 只处理文本消息
	if *msgType != "text" {
		log.Logger().Debug("[Lark] 收到非文本消息", "msg_type", *msgType)
		// 图片/文件等非文本消息也转发给 Agent
		content, blocks := l.extractNonTextContent(event)
		if content == "" {
			return
		}
		log.Logger().Info("[Lark] 收到非文本消息", "msg_id", *messageID, "msg_type", *msgType, "content", content)

		msg := Message{
			ID:        *messageID,
			Channel:   l.name,
			From:      sessionKey,
			Content:   content,
			Timestamp: time.Now().Unix(),
			Blocks:    blocks,
			Metadata: map[string]any{
				"open_id":    *senderID,
				"message_id": *messageID,
				"chat_id":    *chatID,
				"chat_type":  *chatType,
				"msg_type":   *msgType,
			},
		}
		l.PushMessage(msg)
		l.addReaction(*messageID, "OK")
		return
	}

	// 解析消息内容
	contentStr := event.Event.Message.Content
	var contentObj map[string]string
	if err := json.Unmarshal([]byte(*contentStr), &contentObj); err != nil {
		log.Logger().Error("[Lark] 解析消息内容失败", "err", err)
		return
	}

	content, ok := contentObj["text"]
	if !ok || content == "" {
		return
	}

	log.Logger().Info("[Lark] 收到文本消息", "msg_id", *messageID, "open_id", *senderID, "chat_id", *chatID, "chat_type", *chatType, "content", content)

	// 给用户消息添加处理中的 reaction 图标
	l.addReaction(*messageID, "OK")

	msg := Message{
		ID:        *messageID,
		Channel:   l.name,
		From:      sessionKey,
		Content:   content,
		Timestamp: time.Now().Unix(),
		Metadata: map[string]any{
			"open_id":    *senderID,
			"message_id": *messageID,
			"chat_id":    *chatID,
			"chat_type":  *chatType,
			"msg_type":   *msgType,
		},
	}

	l.PushMessage(msg)
}

// extractNonTextContent 提取非文本消息（图片、文件等）的内容标识和内容块
func (l *LarkChannel) extractNonTextContent(event *larkim.P2MessageReceiveV1) (string, ContentBlocks) {
	msg := event.Event.Message

	// 飞书 SDK 的消息结构
	// image 消息: content 是 {"image_key": "xxx"}
	// file 消息: content 是 {"file_key": "xxx", "file_name": "xxx"}
	// audio 消息: content 是 {"file_key": "xxx"}
	contentStr := msg.Content
	if contentStr == nil {
		return "", nil
	}
	var contentObj map[string]string
	if err := json.Unmarshal([]byte(*contentStr), &contentObj); err != nil {
		return "", nil
	}

	msgType := event.Event.Message.MessageType
	if msgType == nil {
		return "", nil
	}
	switch *msgType {
	case "image":
		if imageKey, ok := contentObj["image_key"]; ok && imageKey != "" {
			// 通过 SDK API 下载图片并转为 base64
			base64Data, mediaType := l.downloadImageAsBase64(imageKey)
			if base64Data != "" {
				blocks := ContentBlocks{NewImageBlockBase64(base64Data, mediaType)}
				return "[image]", blocks
			}
			return "[image_key:" + imageKey + "]", nil
		}
		return "[image]", nil
	case "file":
		fileName, _ := contentObj["file_name"]
		if fileName == "" {
			fileName = "[file]"
		}
		if fileKey, ok := contentObj["file_key"]; ok && fileKey != "" {
			return "[file:" + fileName + ",key:" + fileKey + "]", nil
		}
		return "[file:" + fileName + "]", nil
	default:
		return fmt.Sprintf("[%s message]", *msgType), nil
	}
}

func (l *LarkChannel) Stop() error {
	// 先停止基类（关闭 msgChan）
	l.BotChannelBase.Stop()
	close(l.stopChan)
	// 官方 SDK 的 wsClient 没有 Stop 方法，关闭 stopChan 即可
	log.Logger().Info("[Lark] 已停止")
	return nil
}

func (l *LarkChannel) GetName() string {
	return l.name
}

func (l *LarkChannel) Receive(ctx context.Context) (<-chan Message, error) {
	return l.msgChan, nil
}

// Send 发送响应（调用飞书消息API）
func (l *LarkChannel) Send(ctx context.Context, resp Response) error {
	// 解析 sessionKey → open_id（群聊时 resp.To 是 chat_id，单聊时是 open_id）
	sendTo := l.resolveOpenID(resp.To)

	// 检查是否包含文件
	if strings.Contains(resp.Content, "[FILE_BLOCK]") {
		fileInfo := ParseFileBlock(resp.Content)
		if fileInfo != nil && fileInfo.Path != "" {
			// 上传文件
			fileKey, err := l.uploadFile(ctx, fileInfo.Path)
			if err == nil && fileKey != "" {
				// 发送文件消息
				err = l.sendFileMessage(ctx, sendTo, fileKey)
				if err == nil {
					log.Logger().Info("[Lark] 文件消息已发送", "to", sendTo, "filename", fileInfo.Filename)
					l.clearPendingReaction(resp.To)
					return nil
				}
				log.Logger().Warn("[Lark] 文件消息发送失败，回退到文本", "err", err)
			} else {
				log.Logger().Warn("[Lark] 文件上传失败，回退到文本", "err", err)
			}
		}
	}

	// 发送文本消息
	content := resp.Content
	if strings.Contains(content, "[FILE_BLOCK]") {
		content = ExtractFileBlockDescription(content)
	}
	if l.botPrefix != "" {
		content = l.botPrefix + "  " + content
	}

	if detectTable(content) {
		// 包含表格 → 混合 markdown + 原生 table 元素的卡片
		chunks := buildCardContentChunks(content)
		for _, chunk := range chunks {
			if err := l.sendInteractiveCard(ctx, sendTo, chunk); err != nil {
				log.Logger().Error("[Lark] 发送卡片消息失败", "err", err)
				l.clearPendingReaction(resp.To)
				return err
			}
		}
		l.clearPendingReaction(resp.To)
		log.Logger().Debug("[Lark] 消息已发送", "to", sendTo)
		return nil
	}

	// 无表格 → 简单卡片
	card := larkcard.NewMessageCard().
		Config(larkcard.NewMessageCardConfig().WideScreenMode(true).Build()).
		Elements([]larkcard.MessageCardElement{
			larkcard.NewMessageCardMarkdown().Content(headingToBold(content)).Build(),
		}).
		Build()
	cardJSON, _ := card.String()

	if err := l.sendInteractiveCard(ctx, sendTo, cardJSON); err != nil {
		log.Logger().Error("[Lark] 发送卡片消息失败", "err", err)
		l.clearPendingReaction(resp.To)
		return err
	}

	l.clearPendingReaction(resp.To)
	log.Logger().Debug("[Lark] 消息已发送", "to", sendTo)
	return nil
}

// resolveOpenID 将 sessionKey 解析为发送消息用的 open_id
func (l *LarkChannel) resolveOpenID(sessionKey string) string {
	l.sessionInfoMu.RLock()
	session, ok := l.sessionInfo[sessionKey]
	l.sessionInfoMu.RUnlock()
	if ok {
		return session.openID
	}
	// 如果 sessionInfo 中没有，可能是单聊场景，sessionKey 本身就是 open_id
	return sessionKey
}

// SendFile 实现 FileSender 接口 - 直接发送文件
func (l *LarkChannel) SendFile(ctx context.Context, to string, info *FileBlockInfo) (bool, error) {
	sendTo := l.resolveOpenID(to)
	// URL 类型暂不支持直接发送，走回退
	if info.FileType == "url" {
		return false, nil
	}

	// 判断文件类型（图片用 image 类型发送）
	mime := media.GetMediaType(info.Filename)
	isImage := strings.HasPrefix(mime, "image/")

	var key string
	var err error

	if isImage {
		// 上传图片（使用 Image API，返回 image_key）
		key, err = l.uploadImage(ctx, info.Path)
		if err != nil {
			return true, fmt.Errorf("图片上传失败: %w", err)
		}
		// 发送图片消息
		if err := l.sendImageMessage(ctx, sendTo, key); err != nil {
			return true, err
		}
		log.Logger().Info("[Lark] 图片消息已发送", "to", sendTo, "filename", info.Filename, "image_key", key)
	} else {
		// 上传文件（使用 File API，返回 file_key）
		key, err = l.uploadFile(ctx, info.Path)
		if err != nil {
			return true, fmt.Errorf("文件上传失败: %w", err)
		}
		// 发送文件消息
		if err := l.sendFileMessage(ctx, sendTo, key); err != nil {
			return true, err
		}
		log.Logger().Info("[Lark] 文件消息已发送", "to", sendTo, "filename", info.Filename, "file_key", key)
	}

	return true, nil
}

// uploadFile 上传文件到飞书，返回 file_key（使用官方 SDK）
func (l *LarkChannel) uploadFile(ctx context.Context, filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	filename := filepath.Base(filePath)

	// 根据文件扩展名确定 file_type
	ext := strings.ToLower(filepath.Ext(filePath))
	fileType := "stream" // 默认使用 stream 类型
	switch ext {
	case ".pdf":
		fileType = larkim.CreateFileFileTypePdf
	case ".doc", ".docx":
		fileType = larkim.CreateFileFileTypeDoc
	case ".xls", ".xlsx":
		fileType = larkim.CreateFileFileTypeXls
	case ".ppt", ".pptx":
		fileType = larkim.CreateFileFileTypePpt
	case ".mp4":
		fileType = larkim.CreateFileFileTypeMp4
	case ".opus":
		fileType = larkim.CreateFileFileTypeOpus
	}

	// 使用官方 SDK 上传文件
	resp, err := l.larkClient.Im.File.Create(ctx, larkim.NewCreateFileReqBuilder().
		Body(larkim.NewCreateFileReqBodyBuilder().
			FileType(fileType).
			FileName(filename).
			File(bytes.NewReader(data)).
			Build()).
		Build())

	if err != nil {
		return "", fmt.Errorf("飞书SDK上传失败: %v", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("飞书上传失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return *resp.Data.FileKey, nil
}

// sendFileMessage 发送文件消息
func (l *LarkChannel) sendFileMessage(ctx context.Context, receiveID, fileKey string) error {
	// 使用官方 SDK 发送文件消息
	content, _ := json.Marshal(map[string]string{"file_key": fileKey})

	resp, err := l.larkClient.Im.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.CreateMessageV1ReceiveIDTypeOpenId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType(larkim.MsgTypeFile).
			Content(string(content)).
			Build()).
		Build())

	if err != nil {
		return err
	}

	if !resp.Success() {
		return fmt.Errorf("飞书API返回错误: %s (code: %d)", resp.Msg, resp.Code)
	}

	return nil
}

// uploadImage 上传图片到飞书，返回 image_key（使用官方 SDK）
func (l *LarkChannel) uploadImage(ctx context.Context, filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	// 使用官方 SDK 上传图片
	resp, err := l.larkClient.Im.V1.Image.Create(ctx, larkim.NewCreateImageReqBuilder().
		Body(larkim.NewCreateImageReqBodyBuilder().
			ImageType("message").
			Image(bytes.NewReader(data)).
			Build()).
		Build())

	if err != nil {
		return "", fmt.Errorf("飞书SDK上传图片失败: %v", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("飞书上传图片失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	if resp.Data == nil || resp.Data.ImageKey == nil {
		return "", fmt.Errorf("飞书上传图片失败: 响应数据为空")
	}

	return *resp.Data.ImageKey, nil
}

// downloadImageAsBase64 通过 image_key 下载飞书图片并转为 base64
func (l *LarkChannel) downloadImageAsBase64(imageKey string) (base64Data, mediaType string) {
	// 使用飞书 Open API 直接下载图片
	// GET /open-apis/im/v1/images?image_key=xxx
	url := "https://open.feishu.cn/open-apis/im/v1/images?image_key=" + imageKey
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Logger().Warn("[Lark] 创建请求失败", "err", err)
		return "", ""
	}
	// 飞书 SDK 的 HTTP 客户端内部管理 token，这里用 SDK client 的 transport
	// 但最简单的方式是直接调用 SDK 提供的下载 API
	// 由于 SDK 没有暴露 GetMessageResource，使用 HTTP 直接下载
	client := l.HTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		log.Logger().Warn("[Lark] 下载图片失败", "err", err)
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Logger().Warn("[Lark] 下载图片失败", "status", resp.StatusCode)
		return "", ""
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Logger().Warn("[Lark] 读取图片数据失败", "err", err)
		return "", ""
	}
	base64Data = base64.StdEncoding.EncodeToString(data)
	// 根据 Content-Type 判断 mediaType
	mediaType = "image/png" // 默认
	ct := resp.Header.Get("Content-Type")
	switch ct {
	case "image/jpeg":
		mediaType = "image/jpeg"
	case "image/gif":
		mediaType = "image/gif"
	case "image/webp":
		mediaType = "image/webp"
	}
	return base64Data, mediaType
}

// sendImageMessage 发送图片消息
func (l *LarkChannel) sendImageMessage(ctx context.Context, receiveID, imageKey string) error {
	// 使用官方 SDK 发送图片消息
	content, _ := json.Marshal(map[string]string{"image_key": imageKey})

	resp, err := l.larkClient.Im.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.CreateMessageV1ReceiveIDTypeOpenId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType(larkim.MsgTypeImage).
			Content(string(content)).
			Build()).
		Build())

	if err != nil {
		return err
	}

	if !resp.Success() {
		return fmt.Errorf("飞书API返回错误: %s (code: %d)", resp.Msg, resp.Code)
	}

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

	// 飞书 interactive 卡片消息
	if detectTable(content) {
		chunks := buildCardContentChunks(content)
		for _, chunk := range chunks {
			if err := l.sendInteractiveCard(ctx, session.openID, chunk); err != nil {
				return err
			}
		}
	} else {
		card := larkcard.NewMessageCard().
			Config(larkcard.NewMessageCardConfig().WideScreenMode(true).Build()).
			Elements([]larkcard.MessageCardElement{
				larkcard.NewMessageCardMarkdown().Content(headingToBold(content)).Build(),
			}).
			Build()
		cardJSON, _ := card.String()
		if err := l.sendInteractiveCard(ctx, session.openID, cardJSON); err != nil {
			return err
		}
	}

	log.Logger().Info("[Lark] 主动消息已发送", "user", userID)
	return nil
}

func (l *LarkChannel) SendToolEvent(event ToolEvent) error {
	if !l.display.ShouldShowToolEvent(event.Type) {
		return nil
	}

	// 需要知道发送给谁
	if event.To == "" {
		return nil
	}

	// 统一文件分发
	if event.Type == ToolEventContent && len(event.Content) > 0 {
		DispatchFileBlocks(context.Background(), event.Content, event.To, l)
	}

	renderer := Renderer{Style: RenderStyle{
		ShowToolDetails: true,
		SupportsMarkdown: true,
		UseEmoji:        true,
	}}
	content := renderer.RenderToolEvent(event)
	if content == "" {
		return nil
	}

	// 发送为轻量级卡片消息
	card := larkcard.NewMessageCard().
		Elements([]larkcard.MessageCardElement{
			larkcard.NewMessageCardLarkMd().Content(content).Build(),
		}).
		Build()
	cardJSON, _ := card.String()

	resp, err := l.larkClient.Im.Message.Create(context.Background(), larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.CreateMessageV1ReceiveIDTypeOpenId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(event.To).
			MsgType(larkim.MsgTypeInteractive).
			Content(string(cardJSON)).
			Build()).
		Build())

	if err != nil {
		log.Logger().Debug("[Lark] 发送工具事件失败", "err", err)
		return nil // 不阻断主流程
	}

	if !resp.Success() {
		log.Logger().Debug("[Lark] 发送工具事件失败", "code", resp.Code, "msg", resp.Msg)
	}

	return nil
}

// ─────────────────── Markdown 表格 → 飞书原生 table 组件 ───────────────────

// parseMarkdownTable 解析 GFM 表格行，返回飞书 table 元素 JSON
func parseMarkdownTable(tableLines []string) map[string]any {
	var lines []string
	for _, ln := range tableLines {
		if strings.TrimSpace(ln) != "" {
			lines = append(lines, ln)
		}
	}
	if len(lines) < 2 {
		return nil
	}

	sepIdx := -1
	isSepRe := regexp.MustCompile(`^\s*\|[\s:\-|]+\|\s*$`)
	for i, ln := range lines {
		if isSepRe.MatchString(ln) {
			sepIdx = i
			break
		}
	}
	if sepIdx <= 0 {
		return nil
	}

	splitRow := func(line string) []string {
		s := strings.TrimSpace(line)
		s = strings.TrimPrefix(s, "|")
		s = strings.TrimSuffix(s, "|")
		cells := strings.Split(s, "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		return cells
	}

	headers := splitRow(lines[0])
	if len(headers) == 0 {
		return nil
	}

	parseAlignment := func(sepLine string) []string {
		var aligns []string
		for _, cell := range splitRow(sepLine) {
			c := strings.TrimSpace(cell)
			if strings.HasPrefix(c, ":") && strings.HasSuffix(c, ":") {
				aligns = append(aligns, "center")
			} else if strings.HasSuffix(c, ":") {
				aligns = append(aligns, "right")
			} else {
				aligns = append(aligns, "left")
			}
		}
		return aligns
	}
	alignments := parseAlignment(lines[sepIdx])

	cols := make([]map[string]any, len(headers))
	for i, h := range headers {
		align := "left"
		if i < len(alignments) {
			align = alignments[i]
		}
		cols[i] = map[string]any{
			"name":             fmt.Sprintf("col%d", i),
			"display_name":     h,
			"width":            "auto",
			"horizontal_align": align,
		}
	}

	rows := make([]map[string]any, 0)
	boldRe := regexp.MustCompile(`[*_]{1,2}(.+?)[*_]{1,2}`)
	for _, line := range lines[sepIdx+1:] {
		cells := splitRow(line)
		row := make(map[string]any)
		for i := range headers {
			cellText := ""
			if i < len(cells) {
				cellText = cells[i]
			}
			cellText = boldRe.ReplaceAllString(cellText, "$1")
			row[cols[i]["name"].(string)] = cellText
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return nil
	}

	pageSize := len(rows)
	if pageSize < 10 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}

	return map[string]any{
		"tag":       "table",
		"page_size": pageSize,
		"columns":   cols,
		"rows":      rows,
	}
}

var headingRe = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)

func headingToBold(text string) string {
	return headingRe.ReplaceAllString(text, "**$1**")
}

const maxTablesPerCard = 5

func buildCardContent(text string) []map[string]any {
	lines := strings.Split(text, "\n")
	var elements []map[string]any
	tableStartRe := regexp.MustCompile(`^\s*\|`)

	i := 0
	for i < len(lines) {
		line := lines[i]
		if tableStartRe.MatchString(line) {
			var tableBlock []string
			for i < len(lines) && tableStartRe.MatchString(lines[i]) {
				tableBlock = append(tableBlock, lines[i])
				i++
			}
			tableElem := parseMarkdownTable(tableBlock)
			if tableElem != nil {
				elements = append(elements, tableElem)
			} else {
				fallback := headingToBold(strings.Join(tableBlock, "\n"))
				elements = append(elements, map[string]any{
					"tag": "markdown", "content": fallback,
				})
			}
		} else {
			var textBlock []string
			for i < len(lines) && !tableStartRe.MatchString(lines[i]) {
				textBlock = append(textBlock, lines[i])
				i++
			}
			content := strings.TrimSpace(strings.Join(textBlock, "\n"))
			if content != "" {
				content = headingToBold(content)
				elements = append(elements, map[string]any{
					"tag": "markdown", "content": content,
				})
			}
		}
	}

	if len(elements) == 0 {
		elements = append(elements, map[string]any{
			"tag": "markdown", "content": headingToBold(text),
		})
	}
	return elements
}

func buildCardContentChunks(text string) []string {
	elements := buildCardContent(text)

	var chunks [][]map[string]any
	var cur []map[string]any
	tableCount := 0
	for _, elem := range elements {
		if elem["tag"] == "table" {
			tableCount++
			if tableCount > maxTablesPerCard && len(chunks) > 0 {
				chunks = append(chunks, cur)
				cur = nil
				tableCount = 1
			}
			cur = append(cur, elem)
		} else {
			cur = append(cur, elem)
		}
	}
	chunks = append(chunks, cur)

	var result []string
	for _, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}
		card := map[string]any{"elements": chunk}
		jsonBytes, _ := json.Marshal(card)
		result = append(result, string(jsonBytes))
	}
	return result
}

func detectTable(text string) bool {
	r, _ := regexp.Compile(`(?m)^\s*\|`)
	return r.MatchString(text)
}

func (l *LarkChannel) sendInteractiveCard(ctx context.Context, receiveID, cardJSON string) error {
	resp, err := l.larkClient.Im.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.CreateMessageV1ReceiveIDTypeOpenId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType(larkim.MsgTypeInteractive).
			Content(cardJSON).
			Build()).
		Build())

	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("飞书API返回错误: %s (code: %d)", resp.Msg, resp.Code)
	}
	return nil
}

// ─────────────────── 消息回执 (Reaction) ───────────────────

// addReaction 给消息添加表情回复（非阻塞）
func (l *LarkChannel) addReaction(messageID, emojiType string) {
	go func() {
		resp, err := l.larkClient.Im.MessageReaction.Create(context.Background(),
			larkim.NewCreateMessageReactionReqBuilder().
				MessageId(messageID).
				Body(larkim.NewCreateMessageReactionReqBodyBuilder().
					ReactionType(larkim.NewEmojiBuilder().EmojiType(emojiType).Build()).
					Build()).
				Build())
		if err != nil {
			log.Logger().Debug("[Lark] 添加 reaction 失败", "err", err)
			return
		}
		if !resp.Success() {
			log.Logger().Debug("[Lark] 添加 reaction 失败", "code", resp.Code, "msg", resp.Msg)
		}
	}()
}

// removeReaction 移除消息上的表情回复（非阻塞）
func (l *LarkChannel) removeReaction(messageID, emojiType string) {
	go func() {
		resp, err := l.larkClient.Im.MessageReaction.Delete(context.Background(),
			larkim.NewDeleteMessageReactionReqBuilder().
				MessageId(messageID).
				ReactionId(emojiType).
				Build())
		if err != nil {
			log.Logger().Debug("[Lark] 移除 reaction 失败", "err", err)
			return
		}
		if !resp.Success() {
			log.Logger().Debug("[Lark] 移除 reaction 失败", "code", resp.Code, "msg", resp.Msg)
		}
	}()
}

// clearPendingReaction 根据 open_id 移除之前添加的 reaction
func (l *LarkChannel) clearPendingReaction(openID string) {
	l.pendingReactionsMu.Lock()
	msgID, ok := l.pendingReactions[openID]
	if ok {
		delete(l.pendingReactions, openID)
	}
	l.pendingReactionsMu.Unlock()
	if ok {
		l.removeReaction(msgID, "OK")
	}
}
