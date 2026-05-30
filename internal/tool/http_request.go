package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPRequestTool HTTP请求工具
type HTTPRequestTool struct{}

func NewHTTPRequestTool() *HTTPRequestTool {
	return &HTTPRequestTool{}
}

func (t *HTTPRequestTool) Name() string {
	return "http_request"
}

func (t *HTTPRequestTool) Description() string {
	return `发起 HTTP 请求获取网页内容或调用 API。
支持 GET、POST、PUT、DELETE 方法。
可设置请求头、请求体、超时时间。

调用格式：
- http_request(url="https://example.com")  # 默认 GET
- http_request(url="https://api.example.com", method="POST", body="{"key":"value"}", headers={"Content-Type":"application/json"})

参数说明：
- url: 请求地址（必填）
- method: GET/POST/PUT/DELETE（默认 GET）
- headers: 请求头，JSON 格式字符串
- body: 请求体（POST/PUT 时使用）
- timeout: 超时秒数（默认 30）`
}

func (t *HTTPRequestTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "请求地址",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "请求方法: GET, POST, PUT, DELETE",
			},
			"headers": map[string]interface{}{
				"type":        "string",
				"description": "请求头 JSON 字符串，如 {\"Content-Type\":\"application/json\"}",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "请求体内容",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "超时秒数，默认 30",
			},
		},
		"required": []string{"url"},
	}
}

func (t *HTTPRequestTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	urlStr, ok := params["url"].(string)
	if !ok || urlStr == "" {
		return "", fmt.Errorf("缺少 url 参数")
	}

	// 验证 URL
	_, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("无效的 URL: %v", err)
	}

	method := "GET"
	if m, ok := params["method"].(string); ok && m != "" {
		method = strings.ToUpper(m)
	}

	timeout := 30
	if t, ok := params["timeout"].(float64); ok {
		timeout = int(t)
	}

	// 创建请求
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	var req *http.Request

	bodyStr, _ := params["body"].(string)
	if bodyStr != "" && (method == "POST" || method == "PUT") {
		req, err = http.NewRequest(method, urlStr, strings.NewReader(bodyStr))
	} else {
		req, err = http.NewRequest(method, urlStr, nil)
	}
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头
	if headersStr, ok := params["headers"].(string); ok && headersStr != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersStr), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	// 截断过长内容
	content := string(body)
	if len(content) > 50000 {
		content = content[:50000] + "\n... [内容过长已截断]"
	}

	result := fmt.Sprintf("状态码: %d\n响应头: %s\n\n%s", resp.StatusCode, formatHeaders(resp.Header), content)
	return result, nil
}

func formatHeaders(h http.Header) string {
	var parts []string
	for k, v := range h {
		if len(v) > 0 {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v[0]))
		}
	}
	return strings.Join(parts, "; ")
}

func init() {
	GlobalRegistry.Register("http_request", func() Tool {
		return NewHTTPRequestTool()
	})
}
