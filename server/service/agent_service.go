package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"go-claw/config"
)

// AgentService Agent 管理服务
type AgentService struct {
	config  *ConfigService
	mu      sync.RWMutex
	delDir  func(name string) error // 删除 agent 目录的回调
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

// Get 获取单个 Agent 配置
func (s *AgentService) Get(name string) map[string]interface{} {
	workspaceDir := s.config.WorkspaceBase()
	agentCfg, err := config.LoadAgentConfig(workspaceDir, name)
	if err != nil {
		return nil
	}
	agentData, _ := json.Marshal(agentCfg)
	var agentMap map[string]interface{}
	json.Unmarshal(agentData, &agentMap)
	return agentMap
}

// GetJSON 获取单个 Agent 配置 JSON 字符串
func (s *AgentService) GetJSON(name string) string {
	agent := s.Get(name)
	if agent == nil {
		return ""
	}
	data, _ := json.Marshal(agent)
	return string(data)
}

// Update 更新 Agent 配置
func (s *AgentService) Update(name string, agentConfig map[string]interface{}) error {
	return s.config.UpdateAgent(name, agentConfig)
}

// UpdateJSON 更新 Agent 配置（JSON 字符串）
func (s *AgentService) UpdateJSON(name, agentJSON string) error {
	var agentConfig map[string]interface{}
	if err := json.Unmarshal([]byte(agentJSON), &agentConfig); err != nil {
		return err
	}
	return s.Update(name, agentConfig)
}

// Delete 删除 Agent（从配置中移除并删除工作空间目录）
func (s *AgentService) Delete(name string) error {
	if err := s.config.DeleteAgent(name); err != nil {
		return err
	}
	// 删除 agent 工作空间目录
	if s.delDir != nil {
		s.delDir(name)
	} else {
		// 默认删除行为
		workspaceDir := s.config.WorkspaceBase()
		os.RemoveAll(filepath.Join(workspaceDir, name))
	}
	return nil
}

// Create 创建新 Agent
func (s *AgentService) Create(name string, agentConfig map[string]interface{}) error {
	workspaceDir := s.config.WorkspaceBase()

	// 确保 agent 目录存在
	os.MkdirAll(filepath.Join(workspaceDir, name), 0755)

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
			"wechat":   map[string]interface{}{"enabled": false},
		}
	}

	return s.config.UpdateAgent(name, agentConfig)
}

// ListNames 获取 Agent 名称列表
func (s *AgentService) ListNames() []string {
	return s.config.ListAgentNames()
}