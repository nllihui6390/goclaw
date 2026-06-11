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

// HTTPRequestResult HTTP请求结果JSON结构
type HTTPRequestResult struct {
	StatusCode    int               `json:"status_code"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	ContentLength int               `json:"content_length"`
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
	truncated := false
	if len(content) > 50000 {
		content = content[:50000]
		truncated = true
	}

	// 构建响应头
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	result := HTTPRequestResult{
		StatusCode:    resp.StatusCode,
		Headers:       headers,
		Body:          content,
		ContentLength: len(body),
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %v", err)
	}

	output := string(jsonBytes)
	if truncated {
		output += "\n// 注: 响应体已截断至50000字符"
	}

	return output, nil
}

func init() {
	GlobalRegistry.Register("http_request", func() Tool {
		return NewHTTPRequestTool()
	})
}