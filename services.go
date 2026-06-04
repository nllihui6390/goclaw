package main

import (
	"context"
	"sync"

	"go-claw/internal/agent"
	"go-claw/internal/gateway"
	"go-claw/internal/store"
	wailsCtrl "go-claw/server/controllers/wails"
)

// ChatService Wails 对话服务（薄包装层，保持在 main 包以保持绑定名 main.ChatService）
type ChatService struct {
	inner *wailsCtrl.ChatService
	mu    sync.Mutex
}

func NewChatService(agents map[string]*agent.Agent) *ChatService {
	return &ChatService{inner: wailsCtrl.NewChatService(agents)}
}

func (c *ChatService) SetAgents(agents map[string]*agent.Agent) { c.inner.SetAgents(agents) }
func (c *ChatService) CreateSession(agentName string) string    { return c.inner.CreateSession(agentName) }
func (c *ChatService) SetSessionIndex(idx *store.SessionIndex)  { c.inner.SetSessionIndex(idx) }
func (c *ChatService) SendMessage(sessionID, content, agentName string) string {
	return c.inner.SendMessage(sessionID, content, agentName)
}
func (c *ChatService) GetChatHistory(sessionID, agentName string) string {
	return c.inner.GetChatHistory(sessionID, agentName)
}

// AppService Wails 管理服务（薄包装层，保持在 main 包以保持绑定名 main.AppService）
type AppService struct {
	inner *wailsCtrl.AppService
	mu    sync.Mutex
}

func NewAppService() *AppService { return &AppService{inner: wailsCtrl.NewAppService()} }
func (a *AppService) SetAgents(agents map[string]*agent.Agent) { a.inner.SetAgents(agents) }
func (a *AppService) SetSender(sender func(ctx context.Context, sessionID, message string) error) {
	a.inner.SetSender(sender)
}
func (a *AppService) SetSessionIndex(idx *store.SessionIndex)  { a.inner.SetSessionIndex(idx) }
func (a *AppService) SetGateway(gw *gateway.Gateway)          { a.inner.SetGateway(gw) }

func (a *AppService) GetConfig() string                   { return a.inner.GetConfig() }
func (a *AppService) SaveConfig(configJSON string) string { return a.inner.SaveConfig(configJSON) }
func (a *AppService) GetAgents() string                   { return a.inner.GetAgents() }
func (a *AppService) UpdateAgent(name, agentJSON string) string {
	return a.inner.UpdateAgent(name, agentJSON)
}
func (a *AppService) DeleteAgent(name string) string        { return a.inner.DeleteAgent(name) }
func (a *AppService) GetChannels() string                   { return a.inner.GetChannels() }
func (a *AppService) UpdateChannel(name, configJSON string) string {
	return a.inner.UpdateChannel(name, configJSON)
}
func (a *AppService) GetProviders() string                  { return a.inner.GetProviders() }
func (a *AppService) GetTools() string                      { return a.inner.GetTools() }
func (a *AppService) GetSkills() string                     { return a.inner.GetSkills() }
func (a *AppService) GetSessions() string                   { return a.inner.GetSessions() }
func (a *AppService) DeleteSession(sessionID string) string { return a.inner.DeleteSession(sessionID) }
func (a *AppService) GetCronJobs() string                   { return a.inner.GetCronJobs() }
func (a *AppService) SaveCronJob(jobJSON string) string     { return a.inner.SaveCronJob(jobJSON) }
func (a *AppService) DeleteCronJob(id string) string        { return a.inner.DeleteCronJob(id) }
func (a *AppService) RunCronJob(id string) string           { return a.inner.RunCronJob(id) }
func (a *AppService) GetCronEnabled() string                { return a.inner.GetCronEnabled() }
func (a *AppService) SetCronEnabled(enabled string) string  { return a.inner.SetCronEnabled(enabled) }
func (a *AppService) GetLogs() string                       { return a.inner.GetLogs() }
func (a *AppService) GetStatus() string                     { return a.inner.GetStatus() }
func (a *AppService) GetAgentFiles(agentName string) string { return a.inner.GetAgentFiles(agentName) }
func (a *AppService) ReadAgentFile(agentName, fileName string) string {
	return a.inner.ReadAgentFile(agentName, fileName)
}
func (a *AppService) WriteAgentFile(agentName, fileName, content string) string {
	return a.inner.WriteAgentFile(agentName, fileName, content)
}