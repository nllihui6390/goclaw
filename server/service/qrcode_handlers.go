package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ─────────────────── WeChat Handler (existing logic) ───────────────────

const wechatDefaultBaseURL = "https://ilinkai.weixin.qq.com"

// WeChatQRHandler 微信扫码登录处理器
type WeChatQRHandler struct {
	config *ConfigService
}

func (h *WeChatQRHandler) FetchQRCode(params url.Values) (*QRCodeResult, error) {
	baseURL := h.getWechatBaseURL()
	urlStr := baseURL + "/ilink/bot/get_bot_qrcode?bot_type=3"
	resp, err := apiGet(urlStr, h.makeWechatHeaders())
	if err != nil {
		return nil, fmt.Errorf("fetch QR code failed: %v", err)
	}

	qrcodeStr, _ := resp["qrcode"].(string)
	qrcodeImgContent, _ := resp["qrcode_img_content"].(string)

	if qrcodeStr == "" && qrcodeImgContent == "" {
		return nil, fmt.Errorf("WeChat returned empty QR code data")
	}

	var scanURL string
	if strings.HasPrefix(qrcodeImgContent, "http") {
		scanURL = qrcodeImgContent
	} else if qrcodeStr != "" {
		scanURL = fmt.Sprintf("https://liteapp.weixin.qq.com/q/7GiQu1?qrcode=%s&bot_type=3", qrcodeStr)
	} else {
		scanURL = qrcodeImgContent
	}

	var qrcodeImgB64 string
	if qrcodeImgContent != "" && !strings.HasPrefix(qrcodeImgContent, "http") {
		_, err := base64.StdEncoding.DecodeString(qrcodeImgContent)
		if err == nil {
			qrcodeImgB64 = qrcodeImgContent
		}
	}

	if qrcodeImgB64 == "" && scanURL != "" {
		var err error
		qrcodeImgB64, err = fetchQRCodeImage(scanURL)
		if err != nil {
			return nil, err
		}
	}

	return &QRCodeResult{
		QrcodeImg: qrcodeImgB64,
		PollToken: qrcodeStr,
	}, nil
}

func (h *WeChatQRHandler) PollQRCodeStatus(token string, params url.Values) (*PollResult, error) {
	if token == "" {
		return nil, fmt.Errorf("poll token is empty")
	}
	baseURL := h.getWechatBaseURL()
	urlStr := fmt.Sprintf("%s/ilink/bot/get_qrcode_status?qrcode=%s", baseURL, token)

	resp, err := apiGet(urlStr, h.makeWechatHeaders())
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

func (h *WeChatQRHandler) getWechatBaseURL() string {
	defaultAgent := "default"
	channels := h.config.GetChannels(defaultAgent)
	if channels != nil {
		if wechatCfg, ok := channels["wechat"].(map[string]interface{}); ok {
			if baseURL, ok := wechatCfg["base_url"].(string); ok && baseURL != "" {
				return baseURL
			}
		}
	}
	return wechatDefaultBaseURL
}

func (h *WeChatQRHandler) makeWechatHeaders() map[string]string {
	uinB64 := randomUint32B64()
	return map[string]string{
		"Content-Type":      "application/json",
		"AuthorizationType": "ilink_bot_token",
		"X-WECHAT-UIN":      uinB64,
	}
}

// ─────────────────── DingTalk Handler ───────────────────

const dingTalkOAPIBase = "https://oapi.dingtalk.com"

// DingTalkQRHandler 钉钉扫码登录处理器（Device Registration Flow）
type DingTalkQRHandler struct{}

func (h *DingTalkQRHandler) FetchQRCode(params url.Values) (*QRCodeResult, error) {
	// Step 1: Init - get nonce (with source parameter)
	initResp, err := apiPost(
		dingTalkOAPIBase+"/app/registration/init",
		"application/json",
		[]byte(`{"source":"GoClaw"}`),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("dingtalk init failed: %v", err)
	}
	nonce, _ := initResp["nonce"].(string)
	if nonce == "" {
		return nil, fmt.Errorf("dingtalk returned empty nonce (response: %v)", initResp)
	}

	// Step 2: Begin - get device code and QR scan URL
	beginResp, err := apiPost(
		dingTalkOAPIBase+"/app/registration/begin",
		"application/json",
		[]byte(fmt.Sprintf(`{"nonce":"%s"}`, nonce)),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("dingtalk begin failed: %v", err)
	}
	deviceCode, _ := beginResp["device_code"].(string)
	if deviceCode == "" {
		return nil, fmt.Errorf("dingtalk returned empty device_code")
	}

	scanURL, _ := beginResp["verification_uri_complete"].(string)
	if scanURL == "" {
		return nil, fmt.Errorf("dingtalk returned no scan URL")
	}

	qrcodeImgB64, err := fetchQRCodeImage(scanURL)
	if err != nil {
		return nil, err
	}

	return &QRCodeResult{
		QrcodeImg: qrcodeImgB64,
		PollToken: deviceCode,
	}, nil
}

func (h *DingTalkQRHandler) PollQRCodeStatus(token string, params url.Values) (*PollResult, error) {
	if token == "" {
		return nil, fmt.Errorf("poll token is empty")
	}

	resp, err := apiPost(
		dingTalkOAPIBase+"/app/registration/poll",
		"application/json",
		[]byte(fmt.Sprintf(`{"device_code":"%s"}`, token)),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("dingtalk poll failed: %v", err)
	}

	status, _ := resp["status"].(string)
	credentials := make(map[string]string)

	switch strings.ToUpper(status) {
	case "SUCCESS":
		clientID, _ := resp["client_id"].(string)
		clientSecret, _ := resp["client_secret"].(string)
		credentials["client_id"] = clientID
		credentials["client_secret"] = clientSecret
		status = "success"
	case "FAIL":
		failReason, _ := resp["fail_reason"].(string)
		credentials["fail_reason"] = failReason
		status = "fail"
	case "EXPIRED":
		status = "expired"
	default:
		status = "waiting"
	}

	return &PollResult{
		Status:      status,
		Credentials: credentials,
	}, nil
}

// ─────────────────── Feishu/Lark Handler ───────────────────

const feishuAPIBase = "https://accounts.feishu.cn"
const larkAPIBase = "https://accounts.larksuite.com"

// FeishuQRHandler 飞书扫码登录处理器（RFC 8628 Device Authorization Grant）
type FeishuQRHandler struct {
	Domain string // "feishu" (中国) or "lark" (国际)
}

func (h *FeishuQRHandler) baseURL() string {
	if h.Domain == "lark" {
		return larkAPIBase
	}
	return feishuAPIBase
}

func (h *FeishuQRHandler) FetchQRCode(params url.Values) (*QRCodeResult, error) {
	// Support domain override from params
	if d := params.Get("domain"); d == "lark" || d == "feishu" {
		h.Domain = d
	}
	endpoint := h.baseURL() + "/oauth/v1/app/registration"

	// Step 1: Init
	initData := url.Values{}
	initData.Set("action", "init")
	initResp, err := apiPost(endpoint, "application/x-www-form-urlencoded", []byte(initData.Encode()), nil)
	if err != nil {
		return nil, fmt.Errorf("feishu init failed: %v", err)
	}
	// Verify client_secret auth is supported
	if methods, ok := initResp["supported_auth_methods"].([]interface{}); ok {
		found := false
		for _, m := range methods {
			if s, _ := m.(string); s == "client_secret" {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("feishu does not support client_secret auth")
		}
	}

	// Step 2: Begin
	beginData := url.Values{}
	beginData.Set("action", "begin")
	beginData.Set("archetype", "PersonalAgent")
	beginData.Set("auth_method", "client_secret")
	beginData.Set("request_user_info", "open_id")
	beginResp, err := apiPost(endpoint, "application/x-www-form-urlencoded", []byte(beginData.Encode()), nil)
	if err != nil {
		return nil, fmt.Errorf("feishu begin failed: %v", err)
	}

	deviceCode, _ := beginResp["device_code"].(string)
	if deviceCode == "" {
		return nil, fmt.Errorf("feishu returned empty device_code")
	}

	scanURL, _ := beginResp["verification_uri_complete"].(string)
	if scanURL == "" {
		if uri, _ := beginResp["verification_uri"].(string); uri != "" {
			scanURL = uri
		}
	}
	if scanURL == "" {
		return nil, fmt.Errorf("feishu returned no scan URL")
	}

	qrcodeImgB64, err := fetchQRCodeImage(scanURL)
	if err != nil {
		return nil, err
	}

	return &QRCodeResult{
		QrcodeImg: qrcodeImgB64,
		PollToken: deviceCode,
	}, nil
}

func (h *FeishuQRHandler) PollQRCodeStatus(token string, params url.Values) (*PollResult, error) {
	if token == "" {
		return nil, fmt.Errorf("poll token is empty")
	}

	pollData := url.Values{}
	pollData.Set("action", "poll")
	pollData.Set("device_code", token)

	resp, err := apiPost(h.baseURL()+"/oauth/v1/app/registration", "application/x-www-form-urlencoded", []byte(pollData.Encode()), nil)
	if err != nil {
		return nil, fmt.Errorf("feishu poll failed: %v", err)
	}

	// Check for error field
	if errType, _ := resp["error"].(string); errType != "" {
		status := "waiting"
		switch errType {
		case "expired_token", "invalid_grant":
			status = "expired"
		case "access_denied":
			status = "fail"
		}
		return &PollResult{Status: status}, nil
	}

	// Success - extract credentials
	clientID, _ := resp["client_id"].(string)
	clientSecret, _ := resp["client_secret"].(string)

	credentials := make(map[string]string)
	if clientID != "" {
		credentials["app_id"] = clientID
		credentials["app_secret"] = clientSecret
	}

	status := "waiting"
	if len(credentials) > 0 {
		status = "success"
	}

	return &PollResult{Status: status, Credentials: credentials}, nil
}

// ─────────────────── WeCom Handler ───────────────────

// WeComQRHandler 企业微信扫码登录处理器（HTML 解析 + JSON API）
type WeComQRHandler struct{}

func (h *WeComQRHandler) FetchQRCode(params url.Values) (*QRCodeResult, error) {
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	state := randomHex(16)
	qrURL := fmt.Sprintf("https://work.weixin.qq.com/ai/qc/gen?source=goclaw&state=%s&timestamp=%s", state, timestamp)

	htmlContent, err := httpGetRaw(qrURL, nil)
	if err != nil {
		return nil, fmt.Errorf("wecom gen failed: %v", err)
	}

	scode, authURL, err := parseWeComSettings(htmlContent)
	if err != nil {
		return nil, fmt.Errorf("wecom parse settings failed: %v", err)
	}
	if scode == "" || authURL == "" {
		return nil, fmt.Errorf("wecom returned empty scode or auth_url")
	}

	qrcodeImgB64, err := fetchQRCodeImage(authURL)
	if err != nil {
		return nil, err
	}

	return &QRCodeResult{QrcodeImg: qrcodeImgB64, PollToken: scode}, nil
}

func (h *WeComQRHandler) PollQRCodeStatus(token string, params url.Values) (*PollResult, error) {
	if token == "" {
		return nil, fmt.Errorf("poll token is empty")
	}

	resp, err := apiGet(fmt.Sprintf("https://work.weixin.qq.com/ai/qc/query_result?scode=%s", token), nil)
	if err != nil {
		return nil, fmt.Errorf("wecom poll failed: %v", err)
	}

	data, _ := resp["data"].(map[string]interface{})
	if data == nil {
		return nil, fmt.Errorf("wecom returned empty data")
	}

	status, _ := data["status"].(string)
	credentials := make(map[string]string)

	if strings.ToLower(status) == "success" {
		if botInfo, ok := data["bot_info"].(map[string]interface{}); ok {
			if botID, _ := botInfo["botid"].(string); botID != "" {
				credentials["bot_id"] = botID
			}
			if secret, _ := botInfo["secret"].(string); secret != "" {
				credentials["secret"] = secret
			}
		}
	}

	return &PollResult{Status: status, Credentials: credentials}, nil
}

// ─────────────────── Helpers ───────────────────

// httpGetRaw 获取原始 HTTP 响应内容（非 JSON）
func httpGetRaw(urlStr string, headers map[string]string) (string, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// parseWeComSettings 从 HTML 中解析 window.settings JSON
func parseWeComSettings(html string) (scode, authURL string, err error) {
	idx := strings.Index(html, "window.settings")
	if idx == -1 {
		return "", "", fmt.Errorf("window.settings not found")
	}
	start := strings.Index(html[idx:], "{")
	if start == -1 {
		return "", "", fmt.Errorf("no JSON found after window.settings")
	}
	start += idx

	braceCount := 0
	end := start
	for i := start; i < len(html); i++ {
		switch html[i] {
		case '{':
			braceCount++
		case '}':
			braceCount--
			if braceCount == 0 {
				end = i + 1
				goto done
			}
		}
	}
done:
	if end <= start {
		return "", "", fmt.Errorf("unbalanced JSON in window.settings")
	}

	var settings map[string]interface{}
	if err := json.Unmarshal([]byte(html[start:end]), &settings); err != nil {
		return "", "", fmt.Errorf("parse window.settings failed: %v", err)
	}

	scode, _ = settings["scode"].(string)
	authURL, _ = settings["auth_url"].(string)
	return scode, authURL, nil
}

