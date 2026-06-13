package wails

import (
	"go-claw/global"
	"go-claw/server/service"
)

// ChatService Wails 对话服务
type ChatService struct {
	chatSvc *service.ChatService
}

// NewChatService 创建 ChatService
func NewChatService() *ChatService {
	c := &ChatService{
		chatSvc: service.NewChatService(nil, nil),
	}
	// 从 global 获取依赖
	gw := global.GetGateway()
	if gw != nil {
		c.chatSvc.SetAgents(gw.GetAgents())
	}
	si := global.GetSessionIndex()
	if si != nil {
		c.chatSvc.SetSessionIndex(si)
	}
	return c
}

// CreateSession 创建新会话，返回 UUID
func (c *ChatService) CreateSession(agentName string) string {
	// 从 global 获取最新 sessionIndex
	c.chatSvc.SetSessionIndex(global.GetSessionIndex())
	return c.chatSvc.CreateSession(agentName)
}

// SendMessage 对话接口，返回完整响应
func (c *ChatService) SendMessage(sessionID, content, agentName string) string {
	// agents 从 global 获取（动态获取以支持热加载）
	c.chatSvc.SetAgents(global.GetGateway().GetAgents())
	return c.chatSvc.SendMessage(sessionID, content, agentName)
}

// GetChatHistory 获取指定会话的历史消息
func (c *ChatService) GetChatHistory(sessionID, agentName string) string {
	return c.chatSvc.GetChatHistory(sessionID, agentName)
}

// GetLatestSession 获取指定 agent 的最新 session ID
func (c *ChatService) GetLatestSession(agentName string) string {
	return c.chatSvc.GetLatestSession(agentName)
}