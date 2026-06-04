package service

import (
	"encoding/json"
	"sort"
	"sync"
)

// ChannelInfo 渠道信息
type ChannelInfo struct {
	Name    string                 `json:"name"`
	Key     string                 `json:"key"`     // 渠道标识（用于前端匹配）
	Type    string                 `json:"type"`
	Enabled bool                   `json:"enabled"`
	Status  string                 `json:"status"`  // "connected" 或 "disconnected"
	Config  map[string]interface{} `json:"config"`
}

// GatewayProvider 获取 Gateway 已注册渠道的接口
type GatewayProvider interface {
	HasChannel(name string) bool
}

// ChannelService 渠道管理服务
type ChannelService struct {
	config  *ConfigService
	gateway GatewayProvider
	mu      sync.RWMutex
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

// List 获取渠道列表
func (s *ChannelService) List() []ChannelInfo {
	channelsCfg := s.config.GetChannels()
	channels := []ChannelInfo{}

	s.mu.RLock()
	gw := s.gateway
	s.mu.RUnlock()

	for name, chCfg := range channelsCfg {
		ch, _ := chCfg.(map[string]interface{})
		enabled, _ := ch["enabled"].(bool)

		// 动态获取实际连接状态
		status := "disconnected"
		if gw != nil && gw.HasChannel(name) {
			status = "connected"
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

	// 固定排序：console 优先，其他按预定义顺序
	sort.Slice(channels, func(i, j int) bool {
		order := map[string]int{"console": 0, "lark": 1, "dingtalk": 2, "wecom": 3, "wechat": 4}
		oi, _ := order[channels[i].Key]
		oj, _ := order[channels[j].Key]
		return oi < oj
	})

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