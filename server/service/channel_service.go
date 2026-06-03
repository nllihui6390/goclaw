package service

import (
	"encoding/json"
	"sync"
)

// ChannelInfo 渠道信息
type ChannelInfo struct {
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	Enabled  bool                   `json:"enabled"`
	Status   string                 `json:"status"`
	Config   map[string]interface{} `json:"config"`
}

// ChannelService 渠道管理服务
type ChannelService struct {
	config *ConfigService
	mu     sync.RWMutex
}

// NewChannelService 创建渠道服务
func NewChannelService(config *ConfigService) *ChannelService {
	return &ChannelService{config: config}
}

// channelTypes 渠道类型映射
var channelTypes = map[string]string{
	"console":   "console",
	"webhook":   "http",
	"websocket": "ws",
	"lark":      "lark",
	"dingtalk":  "dingtalk",
	"wecom":     "wecom",
	"wechat":    "wechat",
}

// List 获取渠道列表
func (s *ChannelService) List() []ChannelInfo {
	channelsCfg := s.config.GetChannels()
	channels := []ChannelInfo{}

	for name, chCfg := range channelsCfg {
		ch, _ := chCfg.(map[string]interface{})
		enabled, _ := ch["enabled"].(bool)
		status := "disconnected"
		if enabled {
			status = "connected"
		}

		chType := channelTypes[name]
		if chType == "" {
			chType = "unknown"
		}

		channels = append(channels, ChannelInfo{
			Name:    name,
			Type:    chType,
			Enabled: enabled,
			Status:  status,
			Config:  ch,
		})
	}

	return channels
}

// ListJSON 获取渠道列表 JSON 字符串
func (s *ChannelService) ListJSON() string {
	channels := s.List()
	data, _ := json.Marshal(channels)
	return string(data)
}

// Update 更新渠道配置
func (s *ChannelService) Update(name string, channelConfig map[string]interface{}) error {
	return s.config.UpdateChannel(name, channelConfig)
}

// UpdateJSON 更新渠道配置（JSON 字符串）
func (s *ChannelService) UpdateJSON(name, configJSON string) error {
	var channelConfig map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &channelConfig); err != nil {
		return err
	}
	return s.Update(name, channelConfig)
}