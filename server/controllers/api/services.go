package api

import (
	"encoding/json"
	"net/http"

	"go-claw/server/service"
)

var (
	sessionSvc  *service.SessionService
	configSvc   *service.ConfigService
	agentSvc    *service.AgentService
	channelSvc  *service.ChannelService
	providerSvc *service.ProviderService
	toolSvc     *service.ToolService
	skillSvc    *service.SkillService
	cronSvc     *service.CronService
	logSvc      *service.LogService
	statusSvc   *service.StatusService
	fileSvc     *service.FileService
)

var servicesInited bool

// InitServices 初始化所有 service 实例（幂等，重复调用不重建）
func InitServices() {
	if servicesInited {
		return
	}
	servicesInited = true
	configSvc = service.NewConfigService()
	agentSvc = service.NewAgentService(configSvc)
	channelSvc = service.NewChannelService(configSvc)
	providerSvc = service.NewProviderService(configSvc)
	toolSvc = service.NewToolService(configSvc)
	skillSvc = service.NewSkillService(configSvc)
	cronSvc = service.NewCronService(configSvc)
	sessionSvc = service.NewSessionService(nil, configSvc)
	logSvc = service.NewLogService()
	statusSvc = service.NewStatusService()
	fileSvc = service.NewFileService(configSvc)
}

// SetCronExecutor 设置定时任务执行器（由 main 注入）
func SetCronExecutor(executor func(id string)) {
	if cronSvc != nil {
		cronSvc.SetExecutor(executor)
	}
}

// writeJSON 写入 JSON 响应
func writeJSON(rw http.ResponseWriter, status int, data interface{}) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	json.NewEncoder(rw).Encode(data)
}

// writeError 写入 JSON 错误响应
func writeError(rw http.ResponseWriter, status int, msg string) {
	writeJSON(rw, status, map[string]string{"error": msg})
}