package service

import (
	"encoding/json"
	"sync"
)

// ProviderInfo 供应商信息
type ProviderInfo struct {
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	BaseURL string                 `json:"base_url"`
	APIKey  string                 `json:"api_key"`
	Models  []interface{}          `json:"models"`
}

// ProviderService 供应商管理服务
type ProviderService struct {
	config *ConfigService
	mu     sync.RWMutex
}

// NewProviderService 创建供应商服务
func NewProviderService(config *ConfigService) *ProviderService {
	return &ProviderService{config: config}
}

// List 获取供应商列表
func (s *ProviderService) List() []ProviderInfo {
	providersCfg := s.config.GetProviders()
	providers := []ProviderInfo{}

	for name, pCfg := range providersCfg {
		p, _ := pCfg.(map[string]interface{})
		models, _ := p["models"].([]interface{})
		baseURL, _ := p["base_url"].(string)
		apiKey := maskAPIKey(p["api_key"])
		typ, _ := p["type"].(string)

		providers = append(providers, ProviderInfo{
			Name:    name,
			Type:    typ,
			BaseURL: baseURL,
			APIKey:  apiKey,
			Models:  models,
		})
	}

	return providers
}

// ListJSON 获取供应商列表 JSON 字符串
func (s *ProviderService) ListJSON() string {
	providers := s.List()
	data, _ := json.Marshal(providers)
	return string(data)
}

// maskAPIKey 部分隐藏 API Key
func maskAPIKey(key interface{}) string {
	if key == nil {
		return ""
	}
	s, _ := key.(string)
	if len(s) < 8 {
		return s
	}
	return s[:4] + "..." + s[len(s)-4:]
}