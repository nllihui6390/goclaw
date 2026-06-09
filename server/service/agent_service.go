package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"go-claw/config"
	"go-claw/internal/workspace"
)

// AgentService Agent 管理服务
type AgentService struct {
	config *ConfigService
	mu     sync.RWMutex
	delDir func(name string) error // 删除 agent 目录的回调
}

// NewAgentService 创建 Agent 服务
func NewAgentService(config *ConfigService) *AgentService {
	return &AgentService{config: config}
}

// SetDeleteDirFunc 设置删除 agent 目录的回调函数
func (s *AgentService) SetDeleteDirFunc(fn func(name string) error) {
	s.delDir = fn
}

// List 获取 Agent 列表（含完整配置）
func (s *AgentService) List() []map[string]interface{} {
	return s.config.GetAgents()
}

// ListJSON 获取 Agent 列表 JSON 字符串
func (s *AgentService) ListJSON() string {
	agents := s.List()
	data, _ := json.Marshal(agents)
	return string(data)
}

// Update 更新 Agent 配置（写入 agent.json + 更新 profiles）
func (s *AgentService) Update(name string, agentConfig map[string]interface{}) error {
	workspaceDir := s.config.WorkspaceBase()

	// 确保 agent 目录存在
	os.MkdirAll(filepath.Join(workspaceDir, name), 0755)

	// 将 map 转为 AgentConfig 结构体
	agentData, _ := json.Marshal(agentConfig)
	var agentCfg config.AgentConfig
	if err := json.Unmarshal(agentData, &agentCfg); err != nil {
		return err
	}

	// 确保 name 正确
	agentCfg.Name = name

	// 请求未带 channels 时：保留原渠道配置，若不存在则用默认配置
	if _, hasChannels := agentConfig["channels"]; !hasChannels {
		if existing, err := config.LoadAgentConfig(workspaceDir, name); err == nil {
			agentCfg.Channels = existing.Channels
		} else {
			agentCfg.Channels = config.GetDefaultChannelsConfig()
		}
	}

	// 保存 agent.json
	if err := config.SaveAgentConfig(workspaceDir, name, &agentCfg); err != nil {
		return err
	}

	// 更新根配置 profiles（确保 agent 在 profiles 中）
	s.config.AddProfile(name)

	// 通知观察者
	s.config.NotifyWatchers()
	return nil
}

// UpdateJSON 更新 Agent 配置（JSON 字符串）
func (s *AgentService) UpdateJSON(name, agentJSON string) error {
	var agentConfig map[string]interface{}
	if err := json.Unmarshal([]byte(agentJSON), &agentConfig); err != nil {
		return err
	}
	return s.Update(name, agentConfig)
}

// Delete 删除 Agent（删除 agent.json + 从 profiles 移除 + 删除工作空间目录）
func (s *AgentService) Delete(name string) error {
	if name == "default" {
		return nil // 不允许删除 default
	}
	workspaceDir := s.config.WorkspaceBase()

	// 删除 agent.json
	config.DeleteAgentConfig(workspaceDir, name)

	// 从根配置 profiles 移除
	s.config.RemoveProfile(name)

	// 删除 agent 工作空间目录
	if s.delDir != nil {
		s.delDir(name)
	} else {
		os.RemoveAll(filepath.Join(workspaceDir, name))
	}

	// 通知观察者
	s.config.NotifyWatchers()
	return nil
}

// Create 创建新 Agent（初始化目录 + 人设文件 + 默认配置）
func (s *AgentService) Create(name string, agentConfig map[string]interface{}) error {
	workspaceDir := s.config.WorkspaceBase()
	agentDir := filepath.Join(workspaceDir, name)
	sessionsDir := filepath.Join(agentDir, "sessions")

	// 确保 agent 目录和 sessions 目录存在
	os.MkdirAll(agentDir, 0755)
	os.MkdirAll(sessionsDir, 0755)

	// 初始化人设文件（首次引导、AGENTS.md 等）
	workspace.InitPersonaFiles(agentDir)

	// 设置默认值
	if agentConfig == nil {
		agentConfig = make(map[string]interface{})
	}
	agentConfig["name"] = name
	if _, ok := agentConfig["system_prompt"]; !ok {
		agentConfig["system_prompt"] = "你是一个有用的AI助手。"
	}
	if _, ok := agentConfig["tools"]; !ok {
		agentConfig["tools"] = []string{"weather", "exec", "read_file", "write_file", "edit_file", "get_current_time"}
	}
	if _, ok := agentConfig["channels"]; !ok {
		agentConfig["channels"] = map[string]interface{}{
			"console":  map[string]interface{}{"enabled": true},
			"lark":     map[string]interface{}{"enabled": false},
			"dingtalk": map[string]interface{}{"enabled": false},
			"wecom":    map[string]interface{}{"enabled": false},
			"wechat":   map[string]interface{}{"enabled": false, "bot_token_file": "clawdata/wechat_bot_token", "media_dir": "clawdata/media/wechat"},
		}
	}

	// 调用 Update 保存配置
	return s.Update(name, agentConfig)
}

// ListNames 获取 Agent 名称列表
func (s *AgentService) ListNames() []string {
	return s.config.ListAgentNames()
}
