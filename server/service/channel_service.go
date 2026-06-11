package service

import (
	"encoding/json"
	"go-claw/global"
	"sort"
	"sync"
)

// ChannelInfo 渠道信息
type ChannelInfo struct {
	Name    string                 `json:"name"`
	Key     string                 `json:"key"` // 渠道标识（用于前端匹配）
	Type    string                 `json:"type"`
	Enabled bool                   `json:"enabled"`
	Status  string                 `json:"status"` // "connected" 或 "disconnected"
	Config  map[string]interface{} `json:"config"`
}

// GatewayProvider 获取 Gateway 已注册渠道的接口
type GatewayProvider interface {
	HasChannel(name string) bool
	HasChannelForAgent(agentName, channelName string) bool
}

// ChannelService 渠道管理服务
type ChannelService struct {
	config  *ConfigService
	gateway GatewayProvider
	mu      sync.RWMutex // 渠道配置变更回调（触发动态注册）
}

// NewChannelService 创建渠道服务
func NewChannelService(config *ConfigService) *ChannelService {
	return &ChannelService{config: config}
}

// SetGateway 注入 Gateway 以获取实际连接状态
func (s *ChannelService) SetGateway(gw GatewayProvider) {
	s.mu.Lock()
	s.gateway = gw
	s.mu.Unlock()
}

// channelTypes 渠道类型映射
var channelTypes = map[string]string{
	"console":  "console",
	"lark":     "lark",
	"dingtalk": "dingtalk",
	"wecom":    "wecom",
	"wechat":   "wechat",
}

// knownChannels 前端定义的渠道列表
var knownChannels = []string{"console", "lark", "dingtalk", "wecom", "wechat"}

// channelOrder 渠道排序顺序
var channelOrder = map[string]int{"console": 0, "lark": 1, "dingtalk": 2, "wecom": 3, "wechat": 4}

// defaultChannelConfig 各渠道的默认配置
var defaultChannelConfig = map[string]map[string]interface{}{
	"console": {
		"enabled":            true,
		"bot_prefix":         "",
		"show_tool_messages": true,
		"show_thinking":      true,
		"stream_output":      true,
	},
	"lark": {
		"enabled":            false,
		"app_id":             "",
		"app_secret":         "",
		"bot_prefix":         "",
		"show_tool_messages": false,
		"show_thinking":      false,
		"stream_output":      true,
	},
	"dingtalk": {
		"enabled":            false,
		"client_id":          "",
		"client_secret":      "",
		"bot_prefix":         "",
		"show_tool_messages": false,
		"show_thinking":      false,
		"stream_output":      true,
	},
	"wecom": {
		"enabled":            false,
		"bot_id":             "",
		"secret":             "",
		"bot_prefix":         "",
		"show_tool_messages": false,
		"show_thinking":      false,
		"stream_output":      true,
	},
	"wechat": {
		"enabled":            false,
		"bot_token":          "",
		"bot_token_file":     "clawdata/wechat_bot_token",
		"bot_prefix":         "",
		"base_url":           "",
		"media_dir":          "clawdata/media/wechat",
		"show_tool_messages": false,
		"show_thinking":      false,
		"stream_output":      true,
	},
}

// List 获取指定 agent 的渠道列表
func (s *ChannelService) List(agentName string) []ChannelInfo {
	channelsCfg := s.config.GetChannels(agentName)
	channels := []ChannelInfo{}

	s.mu.RLock()
	gw := s.gateway
	s.mu.RUnlock()

	// 遍历前端定义的所有渠道
	for _, name := range knownChannels {
		// 从后端配置获取，不存在则使用默认配置
		ch := defaultChannelConfig[name]
		if channelsCfg != nil {
			if cfg, ok := channelsCfg[name].(map[string]interface{}); ok {
				// 合并：后端配置覆盖默认值
				merged := make(map[string]interface{})
				for k, v := range ch {
					merged[k] = v
				}
				for k, v := range cfg {
					merged[k] = v
				}
				ch = merged
			}
		}

		enabled, _ := ch["enabled"].(bool)

		// 动态获取实际连接状态
		// Console 是全局渠道，Bot 渠道是 per-agent
		status := "disconnected"
		if gw != nil {
			if name == "console" {
				if enabled && gw.HasChannel("console") {
					status = "connected"
				}
			} else {
				if enabled && gw.HasChannelForAgent(agentName, name) {
					status = "connected"
				}
			}
		}

		chType := channelTypes[name]
		if chType == "" {
			chType = "unknown"
		}

		channels = append(channels, ChannelInfo{
			Name:    displayName(name),
			Key:     name,
			Type:    chType,
			Enabled: enabled,
			Status:  status,
			Config:  ch,
		})
	}

	// 固定排序：按预定义顺序
	sort.Slice(channels, func(i, j int) bool {
		oi, _ := channelOrder[channels[i].Key]
		oj, _ := channelOrder[channels[j].Key]
		return oi < oj
	})

	return channels
}

// ListJSON 获取渠道列表 JSON 字符串
func (s *ChannelService) ListJSON(agentName string) string {
	channels := s.List(agentName)
	data, _ := json.Marshal(channels)
	return string(data)
}

// Update 更新指定 agent 的渠道配置
func (s *ChannelService) Update(agentName, channelName string, channelConfig map[string]interface{}) error {
	err := s.config.UpdateChannel(agentName, channelName, channelConfig)
	if err == nil {
		// 重新加载配置并同步指定渠道
		global.ReloadConfigAndSyncSingleChannel(agentName, channelName)
	}

	return err
}

// UpdateJSON 更新渠道配置（JSON 字符串）
func (s *ChannelService) UpdateJSON(agentName, channelName, configJSON string) error {
	var channelConfig map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &channelConfig); err != nil {
		return err
	}
	return s.Update(agentName, channelName, channelConfig)
}

// displayName 渠道中文名称映射
var channelDisplayNames = map[string]string{
	"console":  "控制台",
	"lark":     "飞书",
	"dingtalk": "钉钉",
	"wecom":    "企业微信",
	"wechat":   "微信",
}

func displayName(key string) string {
	if name, ok := channelDisplayNames[key]; ok {
		return name
	}
	return key
}

// GetChannelConfig 获取指定 agent 的指定渠道配置
func (s *ChannelService) GetChannelConfig(agentName, channelName string) map[string]interface{} {
	channelsCfg := s.config.GetChannels(agentName)
	if channelsCfg == nil {
		return defaultChannelConfig[channelName]
	}
	if cfg, ok := channelsCfg[channelName].(map[string]interface{}); ok {
		return cfg
	}
	return defaultChannelConfig[channelName]
}

// GetDefaultAgent 获取默认 agent 名称
func (s *ChannelService) GetDefaultAgent() string {
	rootCfg := s.config.Get()
	agentsSection, _ := rootCfg["agents"].(map[string]interface{})
	if agentsSection != nil {
		if defaultAgent, ok := agentsSection["default_agent"].(string); ok && defaultAgent != "" {
			return defaultAgent
		}
	}
	// 兼容旧配置
	gateway, _ := rootCfg["gateway"].(map[string]interface{})
	if gateway != nil {
		if defaultAgent, ok := gateway["default_agent"].(string); ok && defaultAgent != "" {
			return defaultAgent
		}
	}
	return "default"
}
