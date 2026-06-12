package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"go-claw/config"
	"go-claw/global"
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

// Get 获取完整根配置（不含 agents 详情）
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

// Save 保存根配置（不含 agents 详情）
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
	// 清除 agent 配置缓存
	config.InvalidateAllAgentCache()
	for _, w := range s.watchers {
		w()
	}
	return nil
}

// AddWatcher 添加配置变更观察者
func (s *ConfigService) AddWatcher(w func()) {
	s.watchers = append(s.watchers, w)
}

// WorkspaceBase 从配置获取工作空间根目录
func (s *ConfigService) WorkspaceBase() string {
	dataDir := global.GetDataDir()
	workspace := "workspaces"
	gateway, _ := s.config["gateway"].(map[string]interface{})
	if gateway != nil {
		if v, ok := gateway["workspace"].(string); ok && v != "" {
			workspace = v
		}
	}
	return filepath.Join(dataDir, workspace)
}

// GetAgents 获取 Agent 配置列表
// 从 profiles 获取引用，再从 agent.json 加载完整配置
func (s *ConfigService) GetAgents() []map[string]interface{} {
	s.mu.RLock()
	agentsSection, _ := s.config["agents"].(map[string]interface{})
	s.mu.RUnlock()

	workspaceDir := s.WorkspaceBase()

	result := make([]map[string]interface{}, 0)

	// 从 profiles 获取 agent 名称列表
	profiles, _ := agentsSection["profiles"].(map[string]interface{})
	if profiles == nil {
		return result
	}

	for name, profileData := range profiles {
		profile, _ := profileData.(map[string]interface{})
		entry := map[string]interface{}{
			"name":    name,
			"enabled": true,
		}
		if profile != nil {
			if v, ok := profile["enabled"]; ok {
				entry["enabled"] = v
			}
		}

		// 从 agent.json 加载完整配置
		agentCfg, err := config.LoadAgentConfig(workspaceDir, name)
		if err == nil {
			// 将 AgentConfig 转为 map
			agentData, _ := json.Marshal(agentCfg)
			var agentMap map[string]interface{}
			json.Unmarshal(agentData, &agentMap)
			// 合并 profile 信息
			for k, v := range agentMap {
				entry[k] = v
			}
		}

		result = append(result, entry)
	}

	// 按 order 数组排序（先添加的排前面）
	orderArr, _ := agentsSection["order"].([]interface{})
	orderMap := make(map[string]int)
	for i, v := range orderArr {
		if name, ok := v.(string); ok {
			orderMap[name] = i
		}
	}
	sort.Slice(result, func(i, j int) bool {
		oi, oki := orderMap[result[i]["name"].(string)]
		oj, okj := orderMap[result[j]["name"].(string)]
		if !oki {
			oi = 999
		}
		if !okj {
			oj = 999
		}
		return oi < oj
	})

	return result
}

// GetChannels 获取指定 agent 的渠道配置
func (s *ConfigService) GetChannels(agentName string) map[string]interface{} {
	workspaceDir := s.WorkspaceBase()

	agentCfg, err := config.LoadAgentConfig(workspaceDir, agentName)
	if err != nil {
		// 返回默认空渠道配置
		return map[string]interface{}{
			"console":  map[string]interface{}{"enabled": true},
			"lark":     map[string]interface{}{"enabled": false},
			"dingtalk": map[string]interface{}{"enabled": false},
			"wecom":    map[string]interface{}{"enabled": false},
			"wechat":   map[string]interface{}{"enabled": false},
		}
	}

	channels := agentCfg.Channels
	config.NormalizeChannelsConfig(&channels)
	return mergeChannelsWithDefaults(channels)
}

// mergeChannelsWithDefaults 将 agent 渠道配置与默认值合并（用于 API 展示）
func mergeChannelsWithDefaults(channels config.ChannelsConfig) map[string]interface{} {
	channelsData, _ := json.Marshal(channels)
	var channelsMap map[string]interface{}
	json.Unmarshal(channelsData, &channelsMap)

	defaultsData, _ := json.Marshal(config.GetDefaultChannelsConfig())
	var defaultsMap map[string]interface{}
	json.Unmarshal(defaultsData, &defaultsMap)

	for name, defVal := range defaultsMap {
		defMap, _ := defVal.(map[string]interface{})
		existing, ok := channelsMap[name].(map[string]interface{})
		if !ok {
			channelsMap[name] = defMap
			continue
		}
		merged := make(map[string]interface{}, len(defMap))
		for k, v := range defMap {
			merged[k] = v
		}
		for k, v := range existing {
			merged[k] = v
		}
		channelsMap[name] = merged
	}
	return channelsMap
}

// GetProviders 获取供应商配置
func (s *ConfigService) GetProviders() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	providers, _ := s.config["providers"].(map[string]interface{})
	return providers
}

// AddProfile 将 agent 添加到根配置 profiles/order 中并写回 config.json
func (s *ConfigService) AddProfile(name string) {
	s.mu.Lock()
	agentsSection, _ := s.config["agents"].(map[string]interface{})
	if agentsSection == nil {
		agentsSection = make(map[string]interface{})
		s.config["agents"] = agentsSection
	}
	profiles, _ := agentsSection["profiles"].(map[string]interface{})
	if profiles == nil {
		profiles = make(map[string]interface{})
		agentsSection["profiles"] = profiles
	}
	if _, exists := profiles[name]; !exists {
		profiles[name] = map[string]interface{}{"enabled": true}
		order, _ := agentsSection["order"].([]interface{})
		order = append(order, name)
		agentsSection["order"] = order

		data, _ := json.MarshalIndent(s.config, "", "  ")
		os.WriteFile("config.json", data, 0644)
	}
	s.mu.Unlock()
}

// RemoveProfile 从根配置 profiles/order 中移除 agent 并写回 config.json
func (s *ConfigService) RemoveProfile(name string) {
	s.mu.Lock()
	agentsSection, _ := s.config["agents"].(map[string]interface{})
	if agentsSection != nil {
		profiles, _ := agentsSection["profiles"].(map[string]interface{})
		if profiles != nil {
			delete(profiles, name)
		}
		order, _ := agentsSection["order"].([]interface{})
		newOrder := make([]interface{}, 0)
		for _, o := range order {
			if o != name {
				newOrder = append(newOrder, o)
			}
		}
		agentsSection["order"] = newOrder

		data, _ := json.MarshalIndent(s.config, "", "  ")
		os.WriteFile("config.json", data, 0644)
	}
	s.mu.Unlock()
}

// NotifyWatchers 通知配置变更观察者
func (s *ConfigService) NotifyWatchers() {
	for _, w := range s.watchers {
		w()
	}
}

// UpdateChannel 更新指定 agent 的渠道配置
func (s *ConfigService) UpdateChannel(agentName, channelName string, channelConfig map[string]interface{}) error {
	workspaceDir := s.WorkspaceBase()

	// 加载 agent.json
	agentCfg, err := config.LoadAgentConfig(workspaceDir, agentName)
	if err != nil {
		return err
	}

	// 将 channelConfig 转为对应的类型并更新
	updateChannelField(&agentCfg.Channels, channelName, channelConfig)

	// 保存 agent.json
	return config.SaveAgentConfig(workspaceDir, agentName, agentCfg)
}

// updateChannelField 更新 ChannelsConfig 中指定渠道的字段
func updateChannelField(channels *config.ChannelsConfig, channelName string, cfg map[string]interface{}) {
	chData, _ := json.Marshal(cfg)
	switch channelName {
	case "console":
		json.Unmarshal(chData, &channels.Console)
	case "lark":
		json.Unmarshal(chData, &channels.Lark)
	case "dingtalk":
		json.Unmarshal(chData, &channels.DingTalk)
	case "wecom":
		json.Unmarshal(chData, &channels.WeCom)
	case "wechat":
		json.Unmarshal(chData, &channels.WeChat)
	}
}

// ListAgentNames 获取所有 agent 名称
func (s *ConfigService) ListAgentNames() []string {
	workspaceDir := s.WorkspaceBase()
	names, _ := config.ListAgentConfigs(workspaceDir)
	return names
}
