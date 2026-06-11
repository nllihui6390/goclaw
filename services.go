package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-claw/internal/channel"
	wailsCtrl "go-claw/server/controllers/wails"
)

// ChatService Wails 对话服务（薄包装层，保持在 main 包以保持绑定名 main.ChatService）
type ChatService struct {
	inner *wailsCtrl.ChatService
}

func NewChatService() *ChatService {
	return &ChatService{inner: wailsCtrl.NewChatService()}
}

func (c *ChatService) CreateSession(agentName string) string { return c.inner.CreateSession(agentName) }
func (c *ChatService) SendMessage(sessionID, content, agentName string) string {
	return c.inner.SendMessage(sessionID, content, agentName)
}
func (c *ChatService) GetChatHistory(sessionID, agentName string) string {
	return c.inner.GetChatHistory(sessionID, agentName)
}

// AppService Wails 管理服务（薄包装层，保持在 main 包以保持绑定名 main.AppService）
type AppService struct {
	inner        *wailsCtrl.AppService
	saveFileFunc func(filename string) (string, error) // Wails 保存对话框回调
}

func NewAppService() *AppService { return &AppService{inner: wailsCtrl.NewAppService()} }

func (a *AppService) SetSaveFileFunc(fn func(filename string) (string, error)) {
	a.saveFileFunc = fn
}

func (a *AppService) GetConfig() string                   { return a.inner.GetConfig() }
func (a *AppService) SaveConfig(configJSON string) string { return a.inner.SaveConfig(configJSON) }

// Agent相关操作
func (a *AppService) GetAgents() string                   { return a.inner.GetAgents() }
func (a *AppService) CreateAgent(agentJSON string) string { return a.inner.CreateAgent(agentJSON) }
func (a *AppService) UpdateAgent(name, agentJSON string) string {
	return a.inner.UpdateAgent(name, agentJSON)
}
func (a *AppService) DeleteAgent(name string) string { return a.inner.DeleteAgent(name) }

// 频道相关操作
func (a *AppService) GetChannels(agentName string) string { return a.inner.GetChannels(agentName) }
func (a *AppService) UpdateChannel(agentName, channelName, configJSON string) string {
	return a.inner.UpdateChannel(agentName, channelName, configJSON)
}
func (a *AppService) GetProviders() string { return a.inner.GetProviders() }
func (a *AppService) TestProvider(provider, model string) string {
	return a.inner.TestProvider(provider, model)
}
func (a *AppService) GetTools() string     { return a.inner.GetTools() }
func (a *AppService) GetSkillPool() string { return a.inner.GetSkillPool() }
func (a *AppService) ScanSkills() string   { return a.inner.ScanSkills() }
func (a *AppService) UploadSkill(filename, base64 string) string {
	return a.inner.UploadSkill(filename, base64)
}
func (a *AppService) GetEnabledSkills(agent string) string { return a.inner.GetEnabledSkills(agent) }
func (a *AppService) SetEnabledSkills(agent, skillsJSON string) string {
	return a.inner.SetEnabledSkills(agent, skillsJSON)
}

func (a *AppService) SetSkillChangedCallback(cb func(agentName string, enabledSkills []string)) {
	a.inner.SetSkillChangedCallback(cb)
}

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
func (a *AppService) Restart() string                       { return a.inner.Restart() }
func (a *AppService) GetAgentFiles(agentName string) string { return a.inner.GetAgentFiles(agentName) }
func (a *AppService) ReadAgentFile(agentName, fileName string) string {
	return a.inner.ReadAgentFile(agentName, fileName)
}
func (a *AppService) WriteAgentFile(agentName, fileName, content string) string {
	return a.inner.WriteAgentFile(agentName, fileName, content)
}
func (a *AppService) GetChannelQRCode(channel string) string {
	return a.inner.GetChannelQRCode(channel)
}
func (a *AppService) GetChannelQRCodeStatus(channel, token string) string {
	return a.inner.GetChannelQRCodeStatus(channel, token)
}
func (a *AppService) GetChannelQRCodeWithParams(channel, paramsJSON string) string {
	return a.inner.GetChannelQRCodeWithParams(channel, paramsJSON)
}

// 桌面端下载文件（打开本地文件或 URL）
func (a *AppService) DownloadFile(path, filename string) string {
	// URL 类型 → 用浏览器打开（保持原逻辑）
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return a.inner.DownloadFile(path, filename)
	}

	// 本地文件 + 保存对话框可用
	if a.saveFileFunc != nil {
		localPath := path
		if strings.HasPrefix(path, "file://") || strings.HasPrefix(path, "file:") {
			localPath = channel.FileURLToLocalPath(path)
		}
		cleanPath := filepath.Clean(localPath)
		if _, err := os.Stat(cleanPath); err != nil {
			return fmt.Sprintf(`{"error":"文件不存在: %s"}`, cleanPath)
		}
		defaultFilename := filename
		if defaultFilename == "" {
			defaultFilename = filepath.Base(cleanPath)
		}
		savePath, err := a.saveFileFunc(defaultFilename)
		if err != nil || savePath == "" {
			return `{"status":"cancelled"}`
		}
		data, err := os.ReadFile(cleanPath)
		if err != nil {
			return fmt.Sprintf(`{"error":"读取源文件失败: %s"}`, err.Error())
		}
		if err := os.WriteFile(savePath, data, 0644); err != nil {
			return fmt.Sprintf(`{"error":"保存文件失败: %s"}`, err.Error())
		}
		return fmt.Sprintf(`{"status":"saved","path":"%s","filename":"%s"}`, savePath, filepath.Base(savePath))
	}

	// 回退：无保存对话框时打开目录
	return a.inner.DownloadFile(path, filename)
}
func (a *AppService) GetMedia(path string) string    { return a.inner.GetMedia(path) }
func (a *AppService) PreviewFile(path string) string { return a.inner.PreviewFile(path) }
func (a *AppService) UploadChatFile(sessionID, filename, base64Data string) string {
	return a.inner.UploadChatFile(sessionID, filename, base64Data)
}
