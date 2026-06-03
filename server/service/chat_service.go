package service

import (
	"context"
	"encoding/json"
	"sync"

	"go-claw/internal/agent"
)

// ChatService 聊天服务
type ChatService struct {
	agents map[string]*agent.Agent
	mu     sync.RWMutex
}

// NewChatService 创建聊天服务
func NewChatService(agents map[string]*agent.Agent) *ChatService {
	return &ChatService{agents: agents}
}

// SetAgents 更新 Agent 实例（用于配置热重载）
func (c *ChatService) SetAgents(agents map[string]*agent.Agent) {
	c.mu.Lock()
	c.agents = agents
	c.mu.Unlock()
}

// SendMessage 发送消息并返回完整响应
func (c *ChatService) SendMessage(sessionID, content, agentName string) string {
	c.mu.RLock()
	ag := c.agents["default"]
	if agentName != "" {
		if a, ok := c.agents[agentName]; ok {
			ag = a
		}
	}
	c.mu.RUnlock()

	if ag == nil {
		return "Agent 未初始化"
	}

	result, err := ag.Process(context.Background(), sessionID, content)
	if err != nil {
		return "Error: " + err.Error()
	}
	return result
}

// GetChatHistory 获取会话历史
func (c *ChatService) GetChatHistory(sessionID, agentName string) string {
	c.mu.RLock()
	ag := c.agents["default"]
	if agentName != "" {
		if a, ok := c.agents[agentName]; ok {
			ag = a
		}
	}
	c.mu.RUnlock()

	if ag == nil {
		return "[]"
	}

	// 先尝试从内存获取
	msgs, exists := ag.GetSessionMessages(sessionID)
	if exists {
		result := make([]map[string]string, 0, len(msgs))
		for _, m := range msgs {
			if m.Role == "user" || m.Role == "assistant" {
				result = append(result, map[string]string{
					"role":    m.Role,
					"content": m.Content,
				})
			}
		}
		data, _ := json.Marshal(result)
		return string(data)
	}

	// 从 Store 加载
	st := ag.GetStore()
	if st == nil {
		return "[]"
	}

	sessData, err := st.GetSession(context.Background(), sessionID)
	if err != nil || sessData == nil {
		return "[]"
	}

	result := make([]map[string]string, 0, len(sessData.Messages))
	for _, m := range sessData.Messages {
		if m.Role == "user" || m.Role == "assistant" {
			result = append(result, map[string]string{
				"role":    m.Role,
				"content": m.Content,
			})
		}
	}
	data, _ := json.Marshal(result)
	return string(data)
}