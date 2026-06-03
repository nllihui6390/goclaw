package wails

import (
	"go-claw/internal/agent"
	"go-claw/server/service"
)

// ChatService Wails 对话服务
type ChatService struct {
	chatSvc *service.ChatService
}

// NewChatService 创建 ChatService
func NewChatService(agents map[string]*agent.Agent) *ChatService {
	return &ChatService{
		chatSvc: service.NewChatService(agents, nil),
	}
}

// SetSessionService 注入 SessionService（用于磁盘兜底）
func (c *ChatService) SetSessionService(s *service.SessionService) {
	c.chatSvc.SetSessionService(s)
}

// SetAgents 注入 Agent 实例
func (c *ChatService) SetAgents(agents map[string]*agent.Agent) {
	c.chatSvc.SetAgents(agents)
}

// SendMessage 对话接口，返回完整响应
func (c *ChatService) SendMessage(sessionID, content, agentName string) string {
	return c.chatSvc.SendMessage(sessionID, content, agentName)
}

// GetChatHistory 获取指定会话的历史消息
func (c *ChatService) GetChatHistory(sessionID, agentName string) string {
	return c.chatSvc.GetChatHistory(sessionID, agentName)
}