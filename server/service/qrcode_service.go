package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// QRCodeResult 二维码获取结果
type QRCodeResult struct {
	QrcodeImg string `json:"qrcode_img"` // base64-encoded PNG
	PollToken string `json:"poll_token"`  // 用于轮询状态的 token
}

// PollResult 轮询状态结果
type PollResult struct {
	Status      string            `json:"status"`      // waiting/scanned/confirmed/expired
	Credentials map[string]string `json:"credentials"` // 扫描成功后返回的凭据
}

// QRCodeHandler 渠道二维码处理器接口
type QRCodeHandler interface {
	// FetchQRCode 获取二维码，返回二维码图片和轮询 token
	FetchQRCode(params url.Values) (*QRCodeResult, error)
	// PollQRCodeStatus 轮询扫描状态
	PollQRCodeStatus(token string, params url.Values) (*PollResult, error)
}

// QRCodeService 二维码服务
type QRCodeService struct {
	config   *ConfigService
	handlers map[string]QRCodeHandler
}

// NewQRCodeService 创建二维码服务
func NewQRCodeService(config *ConfigService) *QRCodeService {
	svc := &QRCodeService{
		config:   config,
		handlers: make(map[string]QRCodeHandler),
	}
	// 注册各渠道 handler
	svc.handlers["wechat"] = &WeChatQRHandler{config: config}
	svc.handlers["dingtalk"] = &DingTalkQRHandler{}
	svc.handlers["feishu"] = &FeishuQRHandler{Domain: "feishu"}
	svc.handlers["wecom"] = &WeComQRHandler{}
	return svc
}

// RegisterHandler 注册自定义二维码处理器（用于扩展新渠道）
func (s *QRCodeService) RegisterHandler(channel string, h QRCodeHandler) {
	s.handlers[channel] = h
}

// FetchQRCode 获取渠道的二维码
func (s *QRCodeService) FetchQRCode(channel string, params url.Values) (*QRCodeResult, error) {
	h, ok := s.handlers[channel]
	if !ok {
		return nil, fmt.Errorf("QR code not supported for channel: %s", channel)
	}
	return h.FetchQRCode(params)
}

// PollQRCodeStatus 轮询二维码扫描状态
func (s *QRCodeService) PollQRCodeStatus(channel, token string, params url.Values) (*PollResult, error) {
	h, ok := s.handlers[channel]
	if !ok {
		return nil, fmt.Errorf("QR code not supported for channel: %s", channel)
	}
	return h.PollQRCodeStatus(token, params)
}

// fetchQRCodeImage 通用二维码生成：从 scanURL 生成 base64 PNG
func fetchQRCodeImage(scanURL string) (string, error) {
	png, err := qrcode.Encode(scanURL, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("generate QR code image failed: %v", err)
	}
	return base64.StdEncoding.EncodeToString(png), nil
}

// apiGet 通用 HTTP GET 请求
func apiGet(urlStr string, headers map[string]string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// apiPost 通用 HTTP POST 请求
func apiPost(urlStr, contentType string, body []byte, headers map[string]string) (map[string]interface{}, error) {
	req, err := http.NewRequest("POST", urlStr, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// randomHex 生成 n 字节随机十六进制字符串
func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// randomUint32B64 生成 base64 编码的随机 uint32 字符串
func randomUint32B64() string {
	b := make([]byte, 4)
	rand.Read(b)
	uinVal := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", uinVal)))
}
