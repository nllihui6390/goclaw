package service

import (
	"encoding/json"
	"sync"
)

// AgentService Agent 管理服务
type AgentService struct {
	config *ConfigService
	mu     sync.RWMutex
}

// NewAgentService 创建 Agent 服务
func NewAgentService(config *ConfigService) *AgentService {
	return &AgentService{config: config}
}

// List 获取 Agent 列表
func (s *AgentService) List() []map[string]interface{} {
	return s.config.GetAgents()
}

// ListJSON 获取 Agent 列表 JSON 字符串
func (s *AgentService) ListJSON() string {
	agents := s.List()
	data, _ := json.Marshal(agents)
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

// Delete 删除 Agent
func (s *AgentService) Delete(name string) error {
	return s.config.DeleteAgent(name)
}