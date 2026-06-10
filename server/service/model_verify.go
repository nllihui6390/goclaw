package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// 16x16 红色像素 PNG 的 base64 编码（用于多模态测试，宽高需 >10）
const tinyRedPixelPNG = "iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAIAAACQkWg2AAAAG0lEQVR4nGL5z0AaYCJR/aiGUQ1DSAMgAAD//0leASLPFPe5AAAAAElFTkSuQmCC"

// ModelTestResult 模型多模态测试结果
type ModelTestResult struct {
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	LatencyMs  int    `json:"latency_ms"`
	ConfigUpdated bool `json:"config_updated,omitempty"`
}

// TestAllModels 测试指定供应商下所有模型的多模态能力
func (s *ProviderService) TestAllModels(providerName string) []*ModelTestResult {
	providers := s.config.GetProviders()
	pRaw, ok := providers[providerName]
	if !ok {
		return []*ModelTestResult{{
			Provider: providerName,
			Success:  false,
			Error:    "provider 不存在: " + providerName,
		}}
	}
	p, _ := pRaw.(map[string]interface{})

	models, _ := p["models"].([]interface{})
	if len(models) == 0 {
		return []*ModelTestResult{{
			Provider: providerName,
			Success:  false,
			Error:    "该供应商下没有模型",
		}}
	}

	results := make([]*ModelTestResult, 0, len(models))
	for _, m := range models {
		model, _ := m.(map[string]interface{})
		modelName, _ := model["name"].(string)
		if modelName == "" {
			continue
		}
		result := s.TestProvider(providerName, modelName)
		results = append(results, result)
	}
	return results
}

// TestProvider 测试指定模型的多模态能力（只测试图片）
// 测试成功后自动更新 config 中的 supports_image 字段
func (s *ProviderService) TestProvider(providerName, modelName string) *ModelTestResult {
	providers := s.config.GetProviders()
	pRaw, ok := providers[providerName]
	if !ok {
		return &ModelTestResult{Provider: providerName, Model: modelName, Success: false, Error: "provider 不存在: " + providerName}
	}
	p, _ := pRaw.(map[string]interface{})

	baseURL, _ := p["base_url"].(string)
	if baseURL == "" {
		return &ModelTestResult{Provider: providerName, Model: modelName, Success: false, Error: "API 地址未配置"}
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
			return &ModelTestResult{Provider: providerName, Success: false, Error: "未找到模型配置，请先添加模型"}
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 发送图片测试请求
	result := s.testImageCapability(client, ctx, baseURL, apiKey, modelName, providerName)
	return result
}

// testImageCapability 测试模型的多模态图片能力
func (s *ProviderService) testImageCapability(client *http.Client, ctx context.Context, baseURL, apiKey, model, providerName string) *ModelTestResult {
	messages := []map[string]interface{}{
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
	}

	reqBody := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  200,
		"temperature": 0.7,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return &ModelTestResult{Provider: providerName, Model: model, Success: false, Error: "构建请求失败: " + err.Error()}
	}

	url := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return &ModelTestResult{Provider: providerName, Model: model, Success: false, Error: "创建请求失败: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	startTime := time.Now()
	resp, err := client.Do(req)
	latency := int(time.Since(startTime).Milliseconds())

	if err != nil {
		return &ModelTestResult{Provider: providerName, Model: model, Success: false, LatencyMs: latency, Error: fmt.Sprintf("请求失败: %v", err)}
	}
	defer resp.Body.Close()

	// 解析响应
	var llmResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&llmResp)

	// 判断是否成功
	success := false
	errMsg := ""

	if resp.StatusCode != 200 {
		// HTTP 错误
		if llmResp.Error != nil {
			errMsg = llmResp.Error.Message
		} else {
			errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		// 连接错误不更新配置
		if isConnectionError(resp.StatusCode, errMsg) {
			return &ModelTestResult{Provider: providerName, Model: model, Success: false, LatencyMs: latency, Error: errMsg}
		}
	} else if llmResp.Error != nil {
		// 200 但有 error 字段
		errMsg = llmResp.Error.Message
	} else if len(llmResp.Choices) > 0 && llmResp.Choices[0].Message.Content != "" {
		// 成功
		success = true
	} else {
		errMsg = "LLM 返回空响应"
	}

	// 更新 config 中的 supports_image
	configUpdated := s.updateModelImageSupport(providerName, model, success)

	return &ModelTestResult{
		Provider:      providerName,
		Model:         model,
		Success:       success,
		Error:         errMsg,
		LatencyMs:     latency,
		ConfigUpdated: configUpdated,
	}
}

// isConnectionError 判断是否是连接错误（不应该更新配置）
func isConnectionError(statusCode int, errMsg string) bool {
	// 5xx 服务器错误
	if statusCode >= 500 {
		return true
	}
	// 401/403 认证错误
	if statusCode == 401 || statusCode == 403 {
		return true
	}
	// 404 模型不存在
	if statusCode == 404 && strings.Contains(errMsg, "model") {
		return true
	}
	return false
}

// updateModelImageSupport 更新模型的 supports_image 配置
func (s *ProviderService) updateModelImageSupport(providerName, modelName string, supportsImage bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 直接操作内部 config map（已持有锁）
	providers, _ := s.config.config["providers"].(map[string]interface{})
	if providers == nil {
		return false
	}
	pRaw, ok := providers[providerName]
	if !ok {
		return false
	}
	p, ok := pRaw.(map[string]interface{})
	if !ok {
		return false
	}

	models, ok := p["models"].([]interface{})
	if !ok {
		return false
	}

	// 找到对应模型并更新
	updated := false
	for _, m := range models {
		model, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		if model["name"] == modelName {
			model["supports_image"] = supportsImage
			updated = true
			break
		}
	}

	if updated {
		// 直接写磁盘（避免调用 Save 导致死锁，因为已持有 s.mu）
		data, _ := json.MarshalIndent(s.config.config, "", "  ")
		os.WriteFile("config.json", data, 0644)
		s.config.NotifyWatchers()
	}

	return updated
}
