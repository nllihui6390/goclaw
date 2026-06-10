package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// 1x1 红色像素 PNG 的 base64 编码（用于多模态测试）
const tinyRedPixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="

// ModelTestResult 模型测试结果
type ModelTestResult struct {
	Success      bool   `json:"success"`
	ResponseText string `json:"response_text,omitempty"`
	LatencyMs    int    `json:"latency_ms"`
	Error        string `json:"error,omitempty"`
	ImageTested  bool   `json:"image_tested"`
	ImageSuccess bool   `json:"image_success,omitempty"`
	ImageError   string `json:"image_error,omitempty"`
}

// TestProvider 测试指定的模型（文字 + 图片多模态测试）
// 测试目的：验证模型连接是否正常，以及模型是否支持多模态输入
// 不管前端配置的 supports_image 是什么，都会实际发送图片测试来验证模型能力
func (s *ProviderService) TestProvider(providerName, modelName string) *ModelTestResult {
	providers := s.config.GetProviders()
	pRaw, ok := providers[providerName]
	if !ok {
		return &ModelTestResult{Success: false, Error: "provider 不存在: " + providerName}
	}
	p, _ := pRaw.(map[string]interface{})

	baseURL, _ := p["base_url"].(string)
	if baseURL == "" {
		return &ModelTestResult{Success: false, Error: "API 地址未配置"}
	}
	apiKey, _ := p["api_key"].(string)
	typ, _ := p["type"].(string)

	// Ollama base_url 补齐 /v1
	if typ == "ollama" && !strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}

	// 确定测试模型名
	if modelName == "" {
		models, _ := p["models"].([]interface{})
		if len(models) > 0 {
			if m, ok := models[0].(map[string]interface{}); ok {
				modelName, _ = m["name"].(string)
			}
		}
		if modelName == "" {
			return &ModelTestResult{Success: false, Error: "未找到模型配置，请先添加模型"}
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 第一步：文字测试
	result := callLLMTest(client, ctx, baseURL, apiKey, modelName, []map[string]interface{}{
		{
			"role":    "user",
			"content": "Say hello in one word",
		},
	})

	if !result.Success {
		return result
	}

	// 第二步：图片测试（不管前端配置如何，都实际发送来验证模型能力）
	result.ImageTested = true
	imgResult := callLLMTest(client, ctx, baseURL, apiKey, modelName, []map[string]interface{}{
		{
			"role": "user",
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "Describe this image in one word",
				},
				{
					"type": "image_url",
					"image_url": map[string]interface{}{
						"url": "data:image/png;base64," + tinyRedPixelPNG,
					},
				},
			},
		},
	})
	result.ImageSuccess = imgResult.Success
	result.ImageError = imgResult.Error

	return result
}

// callLLMTest 发送一次测试请求
func callLLMTest(client *http.Client, ctx context.Context, baseURL, apiKey, model string, messages []map[string]interface{}) *ModelTestResult {
	reqBody := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  200,
		"temperature": 0.7,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return &ModelTestResult{Success: false, Error: "构建请求失败: " + err.Error()}
	}

	url := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return &ModelTestResult{Success: false, Error: "创建请求失败: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	startTime := time.Now()
	resp, err := client.Do(req)
	latency := int(time.Since(startTime).Milliseconds())

	if err != nil {
		return &ModelTestResult{Success: false, LatencyMs: latency, Error: fmt.Sprintf("请求失败: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errBody)
		errMsg := errBody.Error.Message
		if errMsg == "" {
			errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return &ModelTestResult{Success: false, LatencyMs: latency, Error: errMsg}
	}

	var llmResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return &ModelTestResult{Success: false, LatencyMs: latency, Error: "解析响应失败: " + err.Error()}
	}
	if len(llmResp.Choices) == 0 {
		return &ModelTestResult{Success: false, LatencyMs: latency, Error: "LLM 返回空响应"}
	}

	return &ModelTestResult{
		Success:      true,
		ResponseText: llmResp.Choices[0].Message.Content,
		LatencyMs:    latency,
	}
}
