package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// QRCodeResult 二维码获取结果
type QRCodeResult struct {
	QrcodeImg  string `json:"qrcode_img"`  // base64-encoded PNG
	PollToken  string `json:"poll_token"`   // 用于轮询状态的 token
}

// PollResult 轮询状态结果
type PollResult struct {
	Status     string                 `json:"status"`      // waiting/scanned/confirmed/expired
	Credentials map[string]string     `json:"credentials"` // bot_token, base_url (仅 confirmed 时有值)
}

// QRCodeService 二维码服务
type QRCodeService struct {
	config *ConfigService
}

// NewQRCodeService 创建二维码服务
func NewQRCodeService(config *ConfigService) *QRCodeService {
	return &QRCodeService{config: config}
}

const (
	wechatDefaultBaseURL = "https://ilinkai.weixin.qq.com"
	qrcodeStatusTimeout  = 60 // 秒
)

// FetchQRCode 获取渠道的二维码（目前仅支持 wechat）
func (s *QRCodeService) FetchQRCode(channel string) (*QRCodeResult, error) {
	if channel != "wechat" {
		return nil, fmt.Errorf("QR code not supported for channel: %s", channel)
	}

	// 获取配置中的 base_url（可能自定义）
	baseURL := s.getWechatBaseURL()

	// 调用 iLink API 获取二维码
	url := baseURL + "/ilink/bot/get_bot_qrcode?bot_type=3"
	resp, err := s.wechatAPIGet(url)
	if err != nil {
		return nil, fmt.Errorf("fetch QR code failed: %v", err)
	}

	qrcodeStr, _ := resp["qrcode"].(string)
	qrcodeImgContent, _ := resp["qrcode_img_content"].(string)

	if qrcodeStr == "" && qrcodeImgContent == "" {
		return nil, fmt.Errorf("WeChat returned empty QR code data")
	}

	// 构造扫描 URL
	var scanURL string
	if strings.HasPrefix(qrcodeImgContent, "http") {
		scanURL = qrcodeImgContent
	} else if qrcodeStr != "" {
		scanURL = fmt.Sprintf("https://liteapp.weixin.qq.com/q/7GiQu1?qrcode=%s&bot_type=3", qrcodeStr)
	} else {
		scanURL = qrcodeImgContent
	}

	// 生成二维码 PNG 图片
	var qrcodeImgB64 string
	// 检查 qrcode_img_content 是否已经是 base64 图片
	if qrcodeImgContent != "" && !strings.HasPrefix(qrcodeImgContent, "http") {
		// 尝试解码，如果成功说明是 base64 图片数据
		_, err := base64.StdEncoding.DecodeString(qrcodeImgContent)
		if err == nil {
			qrcodeImgB64 = qrcodeImgContent
		}
	}

	// 如果没有现成的图片，从 scan URL 生成
	if qrcodeImgB64 == "" && scanURL != "" {
		png, err := qrcode.Encode(scanURL, qrcode.Medium, 256)
		if err != nil {
			return nil, fmt.Errorf("generate QR code image failed: %v", err)
		}
		qrcodeImgB64 = base64.StdEncoding.EncodeToString(png)
	}

	return &QRCodeResult{
		QrcodeImg: qrcodeImgB64,
		PollToken: qrcodeStr,
	}, nil
}

// PollQRCodeStatus 轮询二维码扫描状态
func (s *QRCodeService) PollQRCodeStatus(channel, token string) (*PollResult, error) {
	if channel != "wechat" {
		return nil, fmt.Errorf("QR code not supported for channel: %s", channel)
	}

	if token == "" {
		return nil, fmt.Errorf("poll token is empty")
	}

	baseURL := s.getWechatBaseURL()
	url := fmt.Sprintf("%s/ilink/bot/get_qrcode_status?qrcode=%s", baseURL, token)

	resp, err := s.wechatAPIGet(url)
	if err != nil {
		return nil, fmt.Errorf("poll QR code status failed: %v", err)
	}

	status, _ := resp["status"].(string)
	credentials := make(map[string]string)

	if status == "confirmed" {
		botToken, _ := resp["bot_token"].(string)
		baseURLVal, _ := resp["baseurl"].(string)
		credentials["bot_token"] = botToken
		credentials["base_url"] = baseURLVal
	}

	return &PollResult{
		Status:      status,
		Credentials: credentials,
	}, nil
}

// getWechatBaseURL 获取微信 API base URL（从配置或默认）
func (s *QRCodeService) getWechatBaseURL() string {
	channels := s.config.GetChannels()
	if channels != nil {
		if wechatCfg, ok := channels["wechat"].(map[string]interface{}); ok {
			if baseURL, ok := wechatCfg["base_url"].(string); ok && baseURL != "" {
				return baseURL
			}
		}
	}
	return wechatDefaultBaseURL
}

// wechatAPIGet 调用微信 iLink API (GET)
func (s *QRCodeService) wechatAPIGet(url string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 设置 iLink Bot 请求头
	for k, v := range s.makeWechatHeaders() {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: time.Duration(qrcodeStatusTimeout) * time.Second}
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

// makeWechatHeaders 生成 iLink Bot API 请求头
func (s *QRCodeService) makeWechatHeaders() map[string]string {
	// X-WECHAT-UIN: base64(str(random_uint32)) — 防重放，每次请求生成
	uinBytes := make([]byte, 4)
	rand.Read(uinBytes)
	uinVal := uint32(uinBytes[0])<<24 | uint32(uinBytes[1])<<16 | uint32(uinBytes[2])<<8 | uint32(uinBytes[3])
	uinB64 := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", uinVal)))

	return map[string]string{
		"Content-Type":      "application/json",
		"AuthorizationType": "ilink_bot_token",
		"X-WECHAT-UIN":      uinB64,
	}
}