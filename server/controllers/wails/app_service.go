package wails

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go-claw/global"
	"go-claw/server/service"
)

// AppService Wails 管理服务
type AppService struct {
	// service instances
	configSvc   *service.ConfigService
	sessionSvc  *service.SessionService
	agentSvc    *service.AgentService
	channelSvc  *service.ChannelService
	providerSvc *service.ProviderService
	toolSvc     *service.ToolService
	skillSvc    *service.SkillService
	cronSvc     *service.CronService
	logSvc      *service.LogService
	statusSvc   *service.StatusService
	fileSvc     *service.FileService
	qrcodeSvc   *service.QRCodeService
}

// NewAppService 创建 AppService
func NewAppService() *AppService {
	a := &AppService{}
	a.initServices()
	return a
}

func (a *AppService) initServices() {
	a.configSvc = service.NewConfigService()
	a.agentSvc = service.NewAgentService(a.configSvc)
	a.agentSvc.SetDeleteDirFunc(func(name string) error {
		wsBase := a.configSvc.WorkspaceBase()
		return os.RemoveAll(filepath.Join(wsBase, name))
	})
	a.channelSvc = service.NewChannelService(a.configSvc)
	a.providerSvc = service.NewProviderService(a.configSvc)
	a.toolSvc = service.NewToolService(a.configSvc)
	a.skillSvc = service.NewSkillService(a.configSvc)
	a.cronSvc = service.NewCronService(a.configSvc)
	a.sessionSvc = service.NewSessionService(nil, a.configSvc)
	a.logSvc = service.NewLogService()
	a.statusSvc = service.NewStatusService()
	a.fileSvc = service.NewFileService(a.configSvc)
	a.qrcodeSvc = service.NewQRCodeService(a.configSvc)

	// 从 global 获取依赖（初始化时设置，无需每次请求时注入）
	gw := global.GetGateway()
	if gw != nil {
		a.channelSvc.SetGateway(gw)
	}
	si := global.GetSessionIndex()
	if si != nil {
		a.sessionSvc.SetSessionIndex(si)
		a.cronSvc.SetSessionIndex(si)
	}

	// 设置 cron executor（从 global 获取 gateway）
	a.cronSvc.SetExecutor(&service.CronExecutor{
		SendMsg: func(ctx context.Context, sessionID, message string) error {
			return global.GetGateway().SendProactiveMessage(ctx, sessionID, message)
		},
		ProcessMsg: func(ctx context.Context, agentName, sessionID, content string) (string, error) {
			agents := global.GetGateway().GetAgents()
			ag := agents["default"]
			if agentName != "" {
				if ag2, ok := agents[agentName]; ok {
					ag = ag2
				}
			}
			if ag == nil {
				return "", fmt.Errorf("agent %s not found", agentName)
			}
			return ag.Process(ctx, sessionID, content)
		},
	})
}

// ─────────── Config ───────────

func (a *AppService) GetConfig() string {
	return a.configSvc.GetJSON()
}

func (a *AppService) SaveConfig(configJSON string) string {
	if err := a.configSvc.SaveJSON(configJSON); err != nil {
		return `{"error":"save failed"}`
	}
	return `{"status":"saved"}`
}

// ─────────── Agents ───────────

func (a *AppService) GetAgents() string {
	return a.agentSvc.ListJSON()
}

func (a *AppService) UpdateAgent(name, agentJSON string) string {
	if err := a.agentSvc.UpdateJSON(name, agentJSON); err != nil {
		return `{"error":"update failed"}`
	}
	return `{"status":"updated"}`
}

func (a *AppService) DeleteAgent(name string) string {
	if name == "default" {
		return `{"error":"default agent cannot be deleted"}`
	}
	if err := a.agentSvc.Delete(name); err != nil {
		return `{"error":"delete failed"}`
	}
	// 从 gateway 中注销 agent
	if gw := global.GetGateway(); gw != nil {
		gw.UnregisterAgent(name)
	}
	return `{"status":"deleted"}`
}

// ─────────── Channels ───────────

func (a *AppService) GetChannels() string {
	// gateway 已在 initServices 中设置
	channels := a.channelSvc.List()
	data, _ := json.Marshal(channels)
	return string(data)
}

func (a *AppService) UpdateChannel(name, configJSON string) string {
	if err := a.channelSvc.UpdateJSON(name, configJSON); err != nil {
		return `{"error":"update failed"}`
	}
	return `{"status":"updated"}`
}

// ─────────── Providers ───────────

func (a *AppService) GetProviders() string {
	return a.providerSvc.ListJSON()
}

// ─────────── Tools ───────────

func (a *AppService) GetTools() string {
	return a.toolSvc.ListSimpleJSON()
}

// ─────────── Skills 技能池管理 ───────────

func (a *AppService) GetSkillPool() string {
	return a.skillSvc.PoolJSON()
}

func (a *AppService) ScanSkills() string {
	reg, err := a.skillSvc.Scan()
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	data, _ := json.Marshal(map[string]interface{}{
		"skill_dir": a.skillSvc.GetSkillDir(),
		"skills":    reg.Skills,
		"total":     len(reg.Skills),
		"message":   "扫描完成",
	})
	return string(data)
}

func (a *AppService) GetEnabledSkills(agent string) string {
	return a.skillSvc.GetEnabledSkillsJSON(agent)
}

func (a *AppService) SetEnabledSkills(agent, skillsJSON string) string {
	var skills []string
	if err := json.Unmarshal([]byte(skillsJSON), &skills); err != nil {
		return `{"error":"invalid JSON"}`
	}
	if err := a.skillSvc.SetEnabledSkills(agent, skills); err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	return `{"status":"updated"}`
}

// SetSkillChangedCallback 设置技能变化回调（用于动态重载 agent 技能）
func (a *AppService) SetSkillChangedCallback(cb func(agentName string, enabledSkills []string)) {
	a.skillSvc.OnSkillsChanged = cb
}

// ─────────── Sessions ───────────

func (a *AppService) GetSessions() string {
	// sessionIndex 已在 initServices 中设置
	sessions := a.sessionSvc.List()
	data, _ := json.Marshal(sessions)
	return string(data)
}

func (a *AppService) DeleteSession(sessionID string) string {
	if err := a.sessionSvc.Delete(sessionID); err != nil {
		return `{"error":"delete failed"}`
	}
	return `{"status":"deleted"}`
}

// ─────────── Cron ───────────

func (a *AppService) GetCronJobs() string {
	jobs := a.cronSvc.List()
	data, _ := json.Marshal(jobs)
	return string(data)
}

func (a *AppService) SaveCronJob(jobJSON string) string {
	var job service.CronJob
	if err := json.Unmarshal([]byte(jobJSON), &job); err != nil {
		return `{"error":"invalid json"}`
	}
	if err := a.cronSvc.Save(job); err != nil {
		return `{"error":"save failed"}`
	}
	return `{"status":"saved"}`
}

func (a *AppService) DeleteCronJob(id string) string {
	if err := a.cronSvc.Delete(id); err != nil {
		return `{"error":"delete failed"}`
	}
	return `{"status":"deleted"}`
}

func (a *AppService) RunCronJob(id string) string {
	a.cronSvc.Run(id)
	return `{"status":"executed"}`
}

func (a *AppService) GetCronEnabled() string {
	enabled := a.cronSvc.GetEnabled()
	data, _ := json.Marshal(map[string]bool{"enabled": enabled})
	return string(data)
}

func (a *AppService) SetCronEnabled(enabled string) string {
	val := enabled == "true"
	if err := a.cronSvc.SetEnabled(val); err != nil {
		return `{"error":"set failed"}`
	}
	return `{"status":"ok"}`
}

// ─────────── Logs & Status ───────────

func (a *AppService) GetLogs() string {
	return a.logSvc.Get()
}

func (a *AppService) GetStatus() string {
	return a.statusSvc.GetJSON()
}

// Restart 重启系统
func (a *AppService) Restart() string {
	if err := global.Restart(); err != nil {
		return `{"error":"` + err.Error() + `"}`
	}
	return `{"status":"restarted"}`
}

// ─────────── Agent Files ───────────

func (a *AppService) GetAgentFiles(agentName string) string {
	return a.fileSvc.ListJSON(agentName)
}

func (a *AppService) ReadAgentFile(agentName, fileName string) string {
	content, err := a.fileSvc.Read(agentName, fileName)
	if err != nil {
		return ""
	}
	return content
}

func (a *AppService) WriteAgentFile(agentName, fileName, content string) string {
	if err := a.fileSvc.Write(agentName, fileName, content); err != nil {
		return `{"error":"write failed"}`
	}
	return `{"status":"saved"}`
}

// ─────────── QR Code 扫码登录 ───────────

func (a *AppService) GetChannelQRCode(channel string) string {
	result, err := a.qrcodeSvc.FetchQRCode(channel)
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	data, _ := json.Marshal(result)
	return string(data)
}

func (a *AppService) GetChannelQRCodeStatus(channel, token string) string {
	result, err := a.qrcodeSvc.PollQRCodeStatus(channel, token)
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	data, _ := json.Marshal(result)
	return string(data)
}