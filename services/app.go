package services

import (
	"encoding/json"
	"os"
)

// AppService 管理类服务（配置、状态）
type AppService struct{}

// GetConfig 读取 config.json
func (a *AppService) GetConfig() string {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return "{}"
	}
	return string(data)
}

// SaveConfig 保存 config.json
func (a *AppService) SaveConfig(jsonStr string) string {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		return "invalid json: " + err.Error()
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile("config.json", data, 0644); err != nil {
		return "save failed: " + err.Error()
	}
	return "ok"
}

// GetLogs 读取日志
func (a *AppService) GetLogs() string {
	data, err := os.ReadFile("logs/app.log")
	if err != nil {
		return "暂无日志"
	}
	if len(data) > 20000 {
		data = data[len(data)-20000:]
	}
	return string(data)
}

// GetStatus 系统状态
func (a *AppService) GetStatus() string {
	return `{"status":"running","mode":"desktop"}`
}

// GetChannels 渠道列表（简化版）
func (a *AppService) GetChannels() string {
	return `[
		{"name":"desktop","type":"desktop","enabled":true,"status":"connected"},
		{"name":"lark","type":"lark","enabled":false,"status":"disconnected"},
		{"name":"wecom","type":"wecom","enabled":false,"status":"disconnected"},
		{"name":"wechat","type":"wechat","enabled":false,"status":"disconnected"}
	]`
}
