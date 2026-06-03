package wails

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"go-claw/internal/agent"
	"go-claw/server/service"
	glog "go-claw/pkg/log"
)

// AppService Wails 管理服务
type AppService struct {
	mu      sync.Mutex
	agents  map[string]*agent.Agent
	sendMsg func(ctx context.Context, sessionID, message string) error

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
	a.channelSvc = service.NewChannelService(a.configSvc)
	a.providerSvc = service.NewProviderService(a.configSvc)
	a.toolSvc = service.NewToolService(a.configSvc)
	a.skillSvc = service.NewSkillService(a.configSvc)
	a.cronSvc = service.NewCronService(a.configSvc)
	a.sessionSvc = service.NewSessionService(nil, a.configSvc)
	a.logSvc = service.NewLogService()
	a.statusSvc = service.NewStatusService()
	a.fileSvc = service.NewFileService(a.configSvc)
}

// SetAgents 注入 Agent 实例
func (a *AppService) SetAgents(agents map[string]*agent.Agent) {
	a.mu.Lock()
	a.agents = agents
	a.mu.Unlock()
}

// SetSender 注入消息发送器（用于定时任务手动执行）
func (a *AppService) SetSender(sender func(ctx context.Context, sessionID, message string) error) {
	a.sendMsg = sender
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
	return `{"status":"deleted"}`
}

// ─────────── Channels ───────────

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

// ─────────── Skills ───────────

func (a *AppService) GetSkills() string {
	return a.skillSvc.ListJSON()
}

// ─────────── Sessions ───────────

func (a *AppService) GetSessions() string {
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
	// 从文件读取任务配置
	dataFile := "clawdata/cron_jobs.json"
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return `{"error":"无法读取任务列表"}`
	}
	var jobs []map[string]interface{}
	if err := json.Unmarshal(data, &jobs); err != nil {
		return `{"error":"任务列表解析失败"}`
	}

	var job map[string]interface{}
	for _, j := range jobs {
		if j["id"] == id {
			job = j
			break
		}
	}
	if job == nil {
		return `{"error":"任务不存在"}`
	}

	name, _ := job["name"].(string)
	jobType, _ := job["type"].(string)
	content, _ := job["content"].(string)
	sessionID, _ := job["session_id"].(string)
	agentName, _ := job["agent_name"].(string)

	go func() {
		logger := glog.Logger()
		logger.Info("[Cron] 手动执行任务", "id", id, "name", name, "type", jobType)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		switch jobType {
		case "text":
			if a.sendMsg != nil {
				if err := a.sendMsg(ctx, sessionID, content); err != nil {
					logger.Warn("[Cron] 文本任务发送失败", "id", id, "err", err)
				} else {
					logger.Info("[Cron] 文本任务已发送", "id", id, "session", sessionID)
				}
			}

		case "agent":
			ag := a.agents["default"]
			if agentName != "" {
				if ag2, ok := a.agents[agentName]; ok {
					ag = ag2
				}
			}
			if ag == nil {
				logger.Warn("[Cron] Agent 未找到", "agent", agentName)
				return
			}
			result, err := ag.Process(ctx, sessionID, content)
			if err != nil {
				logger.Warn("[Cron] Agent 任务执行失败", "id", id, "err", err)
				return
			}
			logger.Info("[Cron] Agent 任务执行完成", "id", id, "result_len", len(result))
			if a.sendMsg != nil {
				if err := a.sendMsg(ctx, sessionID, result); err != nil {
					logger.Warn("[Cron] Agent 结果发送失败", "id", id, "session", sessionID, "err", err)
				} else {
					logger.Info("[Cron] Agent 结果已发送", "id", id, "session", sessionID)
				}
			}

		default:
			logger.Warn("[Cron] 未知任务类型", "type", jobType)
		}
	}()

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