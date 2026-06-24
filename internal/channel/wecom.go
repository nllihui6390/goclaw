package channel

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-claw/internal/media"
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

	// 文件上传命令（长连接模式分片上传）
	WsCmdUploadInit   = "aibot_upload_media_init"   // 初始化上传
	WsCmdUploadChunk  = "aibot_upload_media_chunk"  // 上传分片
	WsCmdUploadFinish = "aibot_upload_media_finish" // 完成上传
)

const (
	maxChunkSize = 512 * 1024 // 单个分片最大 512KB（Base64 编码前）
	maxChunks    = 100        // 最多 100 个分片
)

const DefaultWsURL = "wss://openws.work.weixin.qq.com"

// WeComChannel 企业微信机器人渠道（WebSocket 长连接模式）
type WeComChannel struct {
	*BotChannelBase
	botID     string
	secret    string
	botPrefix string

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

	// 文件上传相关（WebSocket 分片上传）
	pendingUploadResponses   map[string]chan map[string]any
	pendingUploadResponsesMu sync.Mutex
	// 文件上传互斥锁：防止并发上传触发 45033 限流
	uploadMu sync.Mutex
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
func NewWeComChannel(botID, secret, botPrefix string, display DisplayConfig) *WeComChannel {
	return &WeComChannel{
		BotChannelBase:         NewBotChannelBase("wecom", "", display),
		botID:                  botID,
		secret:                 secret,
		botPrefix:              botPrefix,
		stopChan:               make(chan struct{}),
		heartbeatInterval:      30 * time.Second,
		maxMissedPong:          2,
		reconnectBaseDelay:     1 * time.Second,
		reconnectMaxDelay:      30 * time.Second,
		maxReconnectAttempts:   10,
		pendingAcks:            make(map[string]chan error),
		replyAckTimeout:        5 * time.Second,
		sessionInfo:            make(map[string]sessionData),
		pendingUploadResponses: make(map[string]chan map[string]any),
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
	// 先停止基类（关闭 msgChan）
	w.BotChannelBase.Stop()
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
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}

	w.connMu.Lock()
	defer w.connMu.Unlock()

	if w.conn == nil {
		return fmt.Errorf("WebSocket未连接")
	}

	return w.conn.WriteMessage(websocket.TextMessage, data)
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

	// 无 cmd 的帧：认证响应、心跳响应、回复回执或上传响应
	headersRaw, _ := frame["headers"]
	headers, ok := headersRaw.(map[string]any)
	if !ok {
		log.Logger().Warn("[WeCom] headers类型断言失败", "headers_type", fmt.Sprintf("%T", headersRaw))
		headers = nil
	}
	reqID := ""
	if headers != nil {
		reqIDRaw, _ := headers["req_id"]
		reqID, ok = reqIDRaw.(string)
		if !ok {
			log.Logger().Warn("[WeCom] req_id类型断言失败", "req_id_type", fmt.Sprintf("%T", reqIDRaw), "req_id_raw", reqIDRaw)
		}
	}

	log.Logger().Debug("[WeCom] 无cmd帧解析", "req_id", reqID, "errcode", frame["errcode"])

	// 检查是否是上传响应
	w.pendingUploadResponsesMu.Lock()
	uploadChan, hasUploadPending := w.pendingUploadResponses[reqID]
	w.pendingUploadResponsesMu.Unlock()

	if hasUploadPending {
		uploadChan <- frame
		w.pendingUploadResponsesMu.Lock()
		delete(w.pendingUploadResponses, reqID)
		w.pendingUploadResponsesMu.Unlock()
		return
	}

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

	// 成功回执（errcode=0）：中间帧的 ack，无需警告
	if errcode, _ := frame["errcode"].(float64); errcode == 0 {
		log.Logger().Debug("[WeCom] 中间帧回执", "req_id", reqID)
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
		// 图片/文件等非文本消息也转发给 Agent
		content, blocks := wecomExtractNonTextContent(body, msgType)
		if content == "" {
			return
		}
		log.Logger().Info("[WeCom] 收到非文本消息",
			"msg_id", msgID, "msgtype", msgType, "content", content)

		msg := Message{
			ID:        msgID,
			Channel:   w.name,
			From:      userID,
			Content:   content,
			Timestamp: time.Now().Unix(),
			Blocks:    blocks,
			Metadata: map[string]any{
				"chatid":   chatID,
				"chattype": chatType,
				"userid":   userID,
				"req_id":   reqID,
				"aibotid":  aibotID,
				"msg_type": msgType,
			},
		}
		w.PushMessage(msg)
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

// wecomExtractNonTextContent 提取企业微信非文本消息的内容标识和内容块
func wecomExtractNonTextContent(body map[string]any, msgType string) (string, ContentBlocks) {
	switch msgType {
	case "image":
		if imageMap, ok := body["image"].(map[string]any); ok {
			if mediaID, ok := imageMap["media_id"].(string); ok && mediaID != "" {
				// 企业微信图片需要先下载再转 base64
				// 暂时返回 media_id 占位，后续通过 API 下载
				return "[image_media:" + mediaID + "]", nil
			}
		}
		return "[image]", nil
	case "file":
		fileName := ""
		if fileMap, ok := body["file"].(map[string]any); ok {
			if name, ok := fileMap["filename"].(string); ok {
				fileName = name
			}
		}
		if fileName == "" {
			fileName = "[file]"
		}
		return "[file:" + fileName + "]", nil
	case "voice":
		return "[voice]", nil
	case "video":
		if videoMap, ok := body["video"].(map[string]any); ok {
			if mediaID, ok := videoMap["media_id"].(string); ok && mediaID != "" {
				return "[video_media:" + mediaID + "]", nil
			}
		}
		return "[video]", nil
	default:
		return "[" + msgType + " message]", nil
	}
}
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

	// 检查是否包含 [FILE_BLOCK] 且是本地文件
	if strings.Contains(resp.Content, "[FILE_BLOCK]") {
		fileInfo := ParseFileBlock(resp.Content)
		if fileInfo != nil && fileInfo.FileType != "url" && fileInfo.Path != "" {
			// 判断文件类型（图片用 image 类型发送）
			uploadType := "file"
			msgType := "file"
			mime := media.GetMediaType(fileInfo.Filename)
			if strings.HasPrefix(mime, "image/") {
				uploadType = "image"
				msgType = "image"
			}

			// 尝试上传并发送文件
			mediaID, err := w.uploadFileWithType(fileInfo.Path, uploadType)
			if err == nil && mediaID != "" {
				// 构建消息帧
				var body map[string]any
				if msgType == "image" {
					body = map[string]any{
						"msgtype": "image",
						"image": map[string]any{
							"media_id": mediaID,
						},
					}
				} else {
					body = map[string]any{
						"msgtype": "file",
						"file": map[string]any{
							"media_id": mediaID,
						},
					}
				}

				fileFrame := map[string]any{
					"cmd": WsCmdResponse,
					"headers": map[string]string{
						"req_id": reqID,
					},
					"body": body,
				}
				log.Logger().Info("[WeCom] 发送文件消息", "user", resp.To, "filename", fileInfo.Filename, "media_id", mediaID, "type", msgType)
				if err := w.sendAndWaitAck(reqID, fileFrame); err != nil {
					log.Logger().Warn("[WeCom] 文件消息发送失败，回退到文本", "err", err)
				} else {
					return nil
				}
			} else {
				log.Logger().Warn("[WeCom] 文件上传失败，回退到文本发送", "err", err)
			}
		}
	}

	// 普通文本消息
	sendContent := extractFileContent(resp.Content)
	if w.botPrefix != "" {
		sendContent = w.botPrefix + "  " + sendContent
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
				"content": sendContent,
			},
		},
	}

	log.Logger().Info("[WeCom] 发送最终响应", "user", resp.To, "content_len", len(sendContent), "req_id", reqID)
	return w.sendAndWaitAck(reqID, frame)
}

// SendFile 实现 FileSender 接口 - 直接发送文件
func (w *WeComChannel) SendFile(ctx context.Context, to string, info *FileBlockInfo) (bool, error) {
	// URL 类型暂不支持直接发送，走回退
	if info.FileType == "url" {
		return false, nil
	}

	// 判断文件类型（图片用 image 类型，其他用 file 类型）
	uploadType := "file"
	msgType := "file"
	mime := media.GetMediaType(info.Filename)
	if strings.HasPrefix(mime, "image/") {
		uploadType = "image"
		msgType = "image"
	}

	// 上传文件
	mediaID, err := w.uploadFileWithType(info.Path, uploadType)
	if err != nil {
		return true, fmt.Errorf("文件上传失败: %w", err)
	}

	// 获取用户的 session 信息（用于发送 WebSocket 消息）
	w.sessionInfoMu.RLock()
	session, ok := w.sessionInfo[to]
	w.sessionInfoMu.RUnlock()

	if !ok {
		return true, fmt.Errorf("用户会话不存在: %s", to)
	}

	reqID := session.reqID

	// 构建消息帧（根据类型使用不同的消息格式）
	var body map[string]any
	if msgType == "image" {
		body = map[string]any{
			"msgtype": "image",
			"image": map[string]any{
				"media_id": mediaID,
			},
		}
	} else {
		body = map[string]any{
			"msgtype": "file",
			"file": map[string]any{
				"media_id": mediaID,
			},
		}
	}

	fileFrame := map[string]any{
		"cmd": WsCmdResponse,
		"headers": map[string]string{
			"req_id": reqID,
		},
		"body": body,
	}

	log.Logger().Info("[WeCom] 发送文件消息", "user", to, "filename", info.Filename, "media_id", mediaID, "type", msgType)
	if err := w.sendAndWaitAck(reqID, fileFrame); err != nil {
		return true, err
	}

	return true, nil
}

// uploadFileWithType 上传文件到企业微信（通过 WebSocket 分片上传），返回 media_id
// uploadType: "file" 或 "image"
func (w *WeComChannel) uploadFileWithType(filePath string, uploadType string) (string, error) {
	// 串行化文件上传，避免并发触发 WeCom 45033 限流
	w.uploadMu.Lock()
	defer w.uploadMu.Unlock()

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	totalSize := len(data)
	if totalSize < 5 {
		return "", fmt.Errorf("文件太小，至少需要 5 字节")
	}
	if totalSize > 20*1024*1024 { // 20MB
		return "", fmt.Errorf("文件太大，最大支持 20MB")
	}

	filename := filepath.Base(filePath)

	// 计算分片数量
	totalChunks := (totalSize + maxChunkSize - 1) / maxChunkSize
	if totalChunks > maxChunks {
		return "", fmt.Errorf("文件分片数 %d 超过最大限制 %d", totalChunks, maxChunks)
	}

	log.Logger().Info("[WeCom] 开始分片上传", "filename", filename, "total_size", totalSize, "total_chunks", totalChunks, "type", uploadType)

	// 1. 初始化上传
	uploadID, err := w.uploadMediaInit(filename, totalSize, totalChunks, uploadType)
	if err != nil {
		return "", fmt.Errorf("初始化上传失败: %v", err)
	}

	log.Logger().Debug("[WeCom] 上传初始化成功", "upload_id", uploadID)

	// 2. 逐片上传
	for i := 0; i < totalChunks; i++ {
		start := i * maxChunkSize
		end := start + maxChunkSize
		if end > totalSize {
			end = totalSize
		}

		chunkData := data[start:end]
		if err := w.uploadMediaChunk(uploadID, i, chunkData); err != nil {
			return "", fmt.Errorf("上传分片 %d 失败: %v", i, err)
		}

		log.Logger().Debug("[WeCom] 分片上传成功", "chunk_index", i, "upload_id", uploadID)
	}

	// 3. 完成上传
	mediaID, err := w.uploadMediaFinish(uploadID)
	if err != nil {
		return "", fmt.Errorf("完成上传失败: %v", err)
	}

	log.Logger().Info("[WeCom] 文件上传成功", "media_id", mediaID, "filename", filename)
	return mediaID, nil
}

// uploadMediaInit 初始化上传，返回 upload_id
func (w *WeComChannel) uploadMediaInit(filename string, totalSize, totalChunks int, uploadType string) (string, error) {
	reqID := w.generateReqID(WsCmdUploadInit)

	frame := map[string]any{
		"cmd": WsCmdUploadInit,
		"headers": map[string]string{
			"req_id": reqID,
		},
		"body": map[string]any{
			"type":         uploadType,
			"filename":     filename,
			"total_size":   totalSize,
			"total_chunks": totalChunks,
		},
	}

	resp, err := w.sendFrameAndWaitResponse(reqID, frame)
	if err != nil {
		return "", err
	}

	body, _ := resp["body"].(map[string]any)
	uploadID, _ := body["upload_id"].(string)
	if uploadID == "" {
		return "", fmt.Errorf("响应中缺少 upload_id: %v", resp)
	}

	return uploadID, nil
}

// uploadMediaChunk 上传单个分片
func (w *WeComChannel) uploadMediaChunk(uploadID string, chunkIndex int, data []byte) error {
	reqID := w.generateReqID(WsCmdUploadChunk)

	// Base64 编码（使用标准库）
	base64Data := base64.StdEncoding.EncodeToString(data)

	frame := map[string]any{
		"cmd": WsCmdUploadChunk,
		"headers": map[string]string{
			"req_id": reqID,
		},
		"body": map[string]any{
			"upload_id":   uploadID,
			"chunk_index": chunkIndex,
			"base64_data": base64Data,
		},
	}

	resp, err := w.sendFrameAndWaitResponse(reqID, frame)
	if err != nil {
		return err
	}

	errcode, _ := resp["errcode"].(float64)
	if int(errcode) != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return fmt.Errorf("分片上传失败: %s (code: %d)", errmsg, int(errcode))
	}

	return nil
}

// uploadMediaFinish 完成上传，返回 media_id
func (w *WeComChannel) uploadMediaFinish(uploadID string) (string, error) {
	reqID := w.generateReqID(WsCmdUploadFinish)

	frame := map[string]any{
		"cmd": WsCmdUploadFinish,
		"headers": map[string]string{
			"req_id": reqID,
		},
		"body": map[string]any{
			"upload_id": uploadID,
		},
	}

	resp, err := w.sendFrameAndWaitResponse(reqID, frame)
	if err != nil {
		return "", err
	}

	body, _ := resp["body"].(map[string]any)
	mediaID, _ := body["media_id"].(string)
	if mediaID == "" {
		return "", fmt.Errorf("响应中缺少 media_id: %v", resp)
	}

	return mediaID, nil
}

// sendFrameAndWaitResponse 发送帧并通过 pendingUploadResponses 等待响应
func (w *WeComChannel) sendFrameAndWaitResponse(reqID string, frame map[string]any) (map[string]any, error) {
	// 注册响应等待
	respChan := make(chan map[string]any, 1)

	w.pendingUploadResponsesMu.Lock()
	w.pendingUploadResponses[reqID] = respChan
	w.pendingUploadResponsesMu.Unlock()

	// 发送帧
	if err := w.sendFrame(frame); err != nil {
		w.pendingUploadResponsesMu.Lock()
		delete(w.pendingUploadResponses, reqID)
		w.pendingUploadResponsesMu.Unlock()
		return nil, fmt.Errorf("发送帧失败: %v", err)
	}

	// 等待响应（由 receiveLoop 中的 handleFrame 处理）
	select {
	case resp := <-respChan:
		errcode, _ := resp["errcode"].(float64)
		if int(errcode) != 0 {
			errmsg, _ := resp["errmsg"].(string)
			return nil, fmt.Errorf("%s (code: %d)", errmsg, int(errcode))
		}
		return resp, nil
	case <-time.After(30 * time.Second):
		w.pendingUploadResponsesMu.Lock()
		delete(w.pendingUploadResponses, reqID)
		w.pendingUploadResponsesMu.Unlock()
		return nil, fmt.Errorf("等待上传响应超时")
	}
}

// extractFileContent 从响应中提取 [FILE_BLOCK] 的内容，转为可发送的文本（用于回退）
func extractFileContent(content string) string {
	return ExtractFileBlockDescription(content)
}

// SendToolEvent 发送工具事件（流式中间帧）
func (w *WeComChannel) SendToolEvent(event ToolEvent) error {
	if !w.display.ShouldShowToolEvent(event.Type) {
		return nil
	}

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

	// 工具结果不发送中间帧，避免过长
	if event.Type == ToolEventResult {
		return nil
	}

	// 处理 ToolEventContent：如果是本地文件 URL，通过 SendFile 上传并发送
	if event.Type == ToolEventContent && len(event.Content) > 0 && event.To != "" {
		bgCtx := context.Background()
		for _, block := range event.Content {
			switch b := block.(type) {
			case *ImageBlock:
				if b.Source.Type == "url" && strings.HasPrefix(b.Source.URL, "file://") {
					localPath := FileURLToLocalPath(b.Source.URL)
					filename := filepath.Base(localPath)
					info := &FileBlockInfo{
						Filename: filename,
						FileType: "file",
						Path:     localPath,
					}
					supported, err := w.SendFile(bgCtx, event.To, info)
					if supported && err == nil {
						log.Logger().Info("[WeCom] 通过 ContentBlock 发送图片成功", "user", event.To, "filename", filename)
						return nil
					}
					if err != nil {
						log.Logger().Warn("[WeCom] 通过 ContentBlock 发送图片失败", "user", event.To, "filename", filename, "err", err)
					}
				}
			}
		}
	}

	renderer := Renderer{Style: RenderStyle{
		ShowToolDetails: false, // 企微流式中间帧不展示详情
		UseEmoji:        true,
	}}
	content := renderer.RenderToolEvent(event)
	if content == "" {
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

	log.Logger().Debug("[WeCom] 发送中间帧", "type", event.Type, "content_len", len(content), "content_preview", truncateStr(content, 80))
	return w.sendFrame(frame) // 中间帧不等 ack
}

// sendAndWaitAck 发送消息并等待回执
func (w *WeComChannel) sendAndWaitAck(reqID string, frame map[string]any) error {
	ackChan := make(chan error, 1)

	w.pendingAcksMu.Lock()
	w.pendingAcks[reqID] = ackChan
	log.Logger().Info("[WeCom] 注册ack等待", "req_id", reqID)
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
