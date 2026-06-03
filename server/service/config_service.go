package service

import (
	"encoding/json"
	"os"
	"sync"
)

// ConfigService 配置管理服务
type ConfigService struct {
	mu       sync.RWMutex
	config   map[string]interface{}
	watchers []func()
}

// NewConfigService 创建配置服务
func NewConfigService() *ConfigService {
	s := &ConfigService{
		config: make(map[string]interface{}),
	}
	s.load()
	return s
}

// load 从磁盘读取 config.json
func (s *ConfigService) load() {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.config)
}

// Get 获取完整配置
func (s *ConfigService) Get() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetJSON 获取配置 JSON 字符串
func (s *ConfigService) GetJSON() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, _ := json.Marshal(s.config)
	return string(data)
}

// Save 保存配置
func (s *ConfigService) Save(config map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	data, _ := json.MarshalIndent(config, "", "  ")
	err := os.WriteFile("config.json", data, 0644)
	if err != nil {
		return err
	}

	// 通知观察者
	for _, w := range s.watchers {
		w()
	}
	return nil
}

// SaveJSON 保存配置 JSON 字符串
func (s *ConfigService) SaveJSON(configJSON string) error {
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return err
	}
	return s.Save(config)
}

// Reload 重新加载配置
func (s *ConfigService) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	for _, w := range s.watchers {
		w()
	}
	return nil
}

// AddWatcher 添加配置变更观察者
func (s *ConfigService) AddWatcher(w func()) {
	s.watchers = append(s.watchers, w)
}

// GetAgents 获取 Agent 配置列表
func (s *ConfigService) GetAgents() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents, _ := s.config["agents"].([]interface{})
	result := make([]map[string]interface{}, 0, len(agents))
	for _, a := range agents {
		if ag, ok := a.(map[string]interface{}); ok {
			result = append(result, ag)
		}
	}
	return result
}

// GetChannels 获取渠道配置
func (s *ConfigService) GetChannels() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channels, _ := s.config["channels"].(map[string]interface{})
	return channels
}

// GetProviders 获取供应商配置
func (s *ConfigService) GetProviders() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	providers, _ := s.config["providers"].(map[string]interface{})
	return providers
}

// UpdateAgent 更新 Agent 配置
func (s *ConfigService) UpdateAgent(name string, agentConfig map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	agentConfig["name"] = name

	agents, _ := s.config["agents"].([]interface{})
	found := false
	for i, a := range agents {
		if ag, ok := a.(map[string]interface{}); ok && ag["name"] == name {
			agents[i] = agentConfig
			found = true
			break
		}
	}
	if !found {
		agents = append(agents, agentConfig)
	}
	s.config["agents"] = agents

	data, _ := json.MarshalIndent(s.config, "", "  ")
	return os.WriteFile("config.json", data, 0644)
}

// DeleteAgent 删除 Agent
func (s *ConfigService) DeleteAgent(name string) error {
	if name == "default" {
		return nil // 不允许删除 default
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	agents, _ := s.config["agents"].([]interface{})
	filtered := make([]interface{}, 0, len(agents))
	for _, a := range agents {
		if ag, ok := a.(map[string]interface{}); ok && ag["name"] != name {
			filtered = append(filtered, a)
		}
	}
	s.config["agents"] = filtered

	data, _ := json.MarshalIndent(s.config, "", "  ")
	return os.WriteFile("config.json", data, 0644)
}

// UpdateChannel 更新渠道配置
func (s *ConfigService) UpdateChannel(name string, channelConfig map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config["channels"] == nil {
		s.config["channels"] = make(map[string]interface{})
	}
	s.config["channels"].(map[string]interface{})[name] = channelConfig

	data, _ := json.MarshalIndent(s.config, "", "  ")
	return os.WriteFile("config.json", data, 0644)
}