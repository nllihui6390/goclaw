package wails

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go-claw/global"
	"go-claw/internal/channel"
	"go-claw/internal/cron"
	"go-claw/internal/media"
	"go-claw/internal/security"
	glog "go-claw/pkg/log"
	"go-claw/server/service"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	global.ReloadConfigAndSyncAgents()
	return `{"status":"saved"}`
}

// ─────────── Agents ───────────

func (a *AppService) GetAgents() string {
	return a.agentSvc.ListJSON()
}

// CreateAgent 创建新 Agent（JSON 字符串）
func (a *AppService) CreateAgent(agentJSON string) string {
	var agentConfig map[string]interface{}
	if err := json.Unmarshal([]byte(agentJSON), &agentConfig); err != nil {
		return `{"error":"invalid JSON"}`
	}
	name, _ := agentConfig["name"].(string)
	if name == "" {
		return `{"error":"agent name required"}`
	}
	if err := a.agentSvc.Create(name, agentConfig); err != nil {
		return `{"error":"create failed"}`
	}
	global.ReloadAgent(name)
	return `{"status":"created"}`
}

// UpdateAgent 更新 Agent 配置（JSON 字符串）
func (a *AppService) UpdateAgent(name, agentJSON string) string {
	if err := a.agentSvc.UpdateJSON(name, agentJSON); err != nil {
		return `{"error":"update failed"}`
	}
	global.ReloadAgent(name)
	return `{"status":"updated"}`
}

// 删除 Agent 配置（DELETE）
func (a *AppService) DeleteAgent(name string) string {
	if name == "default" {
		return `{"error":"default agent cannot be deleted"}`
	}
	if err := a.agentSvc.Delete(name); err != nil {
		return `{"error":"delete failed"}`
	}
	// 从 gateway 中注销 agent
	// if gw := global.GetGateway(); gw != nil {
	// 	gw.UnregisterAgent(name)
	// }
	// 注销并且删除指定agent的配置文件
	global.RemoveAgentAndConfig(name)
	return `{"status":"deleted"}`
}

// ─────────── Channels ───────────
// GetChannels 获取渠道列表 JSON 字符串
func (a *AppService) GetChannels(agentName string) string {
	if agentName == "" {
		agentName = a.channelSvc.GetDefaultAgent()
	}
	channels := a.channelSvc.List(agentName)
	data, _ := json.Marshal(channels)
	return string(data)
}

// UpdateChannel 更新渠道配置（JSON 字符串）
func (a *AppService) UpdateChannel(agentName, channelName, configJSON string) string {
	if agentName == "" {
		agentName = a.channelSvc.GetDefaultAgent()
	}
	if err := a.channelSvc.UpdateJSON(agentName, channelName, configJSON); err != nil {
		return `{"error":"update failed"}`
	}
	return `{"status":"updated"}`
}

// ─────────── Providers ───────────

func (a *AppService) GetProviders() string {
	return a.providerSvc.ListJSON()
}

// TestProvider 测试模型连接和多模态能力
func (a *AppService) TestProvider(provider, model string) string {
	result := a.providerSvc.TestProvider(provider, model)
	data, _ := json.Marshal(result)
	return string(data)
}

// TestAllModels 测试供应商下所有模型的多模态能力
func (a *AppService) TestAllModels(provider string) string {
	results := a.providerSvc.TestAllModels(provider)
	data, _ := json.Marshal(results)
	return string(data)
}

// ─────────── Security Approvals ───────────

// GetPendingApprovals 获取待审批列表
func (a *AppService) GetPendingApprovals() string {
	approvalSvc := security.GetApprovalService()
	pending := approvalSvc.ListPending()
	data, _ := json.Marshal(pending)
	return string(data)
}

// ApproveRequest 批准请求
func (a *AppService) ApproveRequest(approvalID, approvedBy string) string {
	approvalSvc := security.GetApprovalService()
	if approvedBy == "" {
		approvedBy = "wails_user"
	}
	success := approvalSvc.Approve(approvalID, approvedBy)
	return fmt.Sprintf(`{"success": %t}`, success)
}

// DenyRequest 拒绝请求
func (a *AppService) DenyRequest(approvalID, deniedBy, reason string) string {
	approvalSvc := security.GetApprovalService()
	if deniedBy == "" {
		deniedBy = "wails_user"
	}
	success := approvalSvc.Deny(approvalID, deniedBy, reason)
	return fmt.Sprintf(`{"success": %t}`, success)
}

// GetSecurityConfig 获取安全配置
func (a *AppService) GetSecurityConfig() string {
	configJSON := a.configSvc.GetJSON()
	var fullConfig map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &fullConfig); err != nil {
		return `{}`
	}

	securityCfg, ok := fullConfig["security"]
	if !ok {
		securityCfg = map[string]interface{}{
			"enabled":             false,
			"deny_shell_inject":   false,
			"deny_sensitive_path": false,
			"guard_browser":       false,
			"allowed_paths":       []string{},
		}
	}

	data, _ := json.Marshal(securityCfg)
	return string(data)
}

// UpdateSecurityConfig 更新安全配置
func (a *AppService) UpdateSecurityConfig(configJSON string) string {
	var securityCfg map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &securityCfg); err != nil {
		return `{"success": false, "error": "invalid json"}`
	}

	// 获取完整配置
	fullConfigJSON := a.configSvc.GetJSON()
	var fullConfig map[string]interface{}
	if err := json.Unmarshal([]byte(fullConfigJSON), &fullConfig); err != nil {
		return `{"success": false, "error": "failed to parse config"}`
	}

	// 更新 security 部分
	fullConfig["security"] = securityCfg

	// 保存完整配置
	if err := a.configSvc.Save(fullConfig); err != nil {
		return fmt.Sprintf(`{"success": false, "error": "%s"}`, err.Error())
	}

	return `{"success": true}`
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

func (a *AppService) UploadSkill(filename, base64Data string) string {
	// 解码 base64
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Sprintf(`{"error":"解码文件失败: %s"}`, err.Error())
	}

	result, err := a.skillSvc.ImportFromZip(data)
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON)
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
	// 同步到调度器（cron.Manager），确保内存和文件一致
	a.syncCronToManager(job)
	return `{"status":"saved"}`
}

func (a *AppService) DeleteCronJob(id string) string {
	if err := a.cronSvc.Delete(id); err != nil {
		return `{"error":"delete failed"}`
	}
	// 从调度器中移除
	if mgr := global.GetApp().CronMgr; mgr != nil {
		mgr.RemoveJob(id)
	}
	return `{"status":"deleted"}`
}

func (a *AppService) RunCronJob(id string) string {
	// 优先使用调度器立即执行
	if mgr := global.GetApp().CronMgr; mgr != nil {
		if err := mgr.RunJobNow(id); err != nil {
			return `{"error":"` + err.Error() + `"}`
		}
		return `{"status":"executed"}`
	}
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

// syncCronToManager 将保存的任务同步到调度器内存中，确保调度器拿到最新值
func (a *AppService) syncCronToManager(job service.CronJob) {
	mgr := global.GetApp().CronMgr
	if mgr == nil {
		return
	}
	jobType := cron.JobTypeText
	if job.Type == "agent" {
		jobType = cron.JobTypeAgent
	}
	cronJob := &cron.Job{
		ID:          job.ID,
		Name:        job.Name,
		Type:        jobType,
		Schedule:    job.Schedule,
		Content:     job.Content,
		AgentName:   job.AgentName,
		SessionID:   job.SessionID,
		Enabled:     job.Enabled,
		LastRun:     job.LastRun,
		NextRun:     job.NextRun,
		ActiveStart: job.ActiveStart,
		ActiveEnd:   job.ActiveEnd,
	}
	// 新任务（无 ID 或 manager 中不存在）用 AddJob，已有任务用 UpdateJob
	if job.ID == "" {
		mgr.AddJob(cronJob)
	} else if _, err := mgr.GetJob(job.ID); err != nil {
		mgr.AddJob(cronJob)
	} else {
		mgr.UpdateJob(cronJob)
	}
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

// ─────────── File Preview ───────────

// GetMedia 读取文件内容返回 base64 编码（用于 Wails 模式下的图片/文件显示）
// 返回 JSON: {"base64": "...", "mime": "image/png"}
// 前端解码 base64 后创建 Blob URL 用于显示或下载
func (a *AppService) GetMedia(path string) string {
	if path == "" {
		return `{"error":"path is required"}`
	}
	// 将 file:// URL 转换为本地路径
	localPath := path
	if strings.HasPrefix(path, "file://") || strings.HasPrefix(path, "file:") {
		localPath = channel.FileURLToLocalPath(path)
	}
	// 安全检查：防止路径穿越
	cleanPath := filepath.Clean(localPath)
	if strings.Contains(cleanPath, "..") {
		return `{"error":"invalid file path"}`
	}
	// 读取文件
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return fmt.Sprintf(`{"error":"file not found: %s"}`, err.Error())
	}
	// 获取 MIME 类型
	mime := media.GetMediaType(cleanPath)
	// 返回 base64 编码
	b64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf(`{"base64":"%s","mime":"%s","size":%d}`, b64, mime, len(data))
}

// PreviewFile 读取文件并返回 base64 数据 URL（用于 Wails 模式下的图片预览）
func (a *AppService) PreviewFile(path string) string {
	if path == "" {
		return `{"error":"path is required"}`
	}

	// 将 file:// URL 转换为本地路径
	localPath := path
	if strings.HasPrefix(path, "file://") || strings.HasPrefix(path, "file:") {
		localPath = channel.FileURLToLocalPath(path)
	}
	// 安全检查：防止路径穿越
	cleanPath := filepath.Clean(localPath)
	if strings.Contains(cleanPath, "..") {
		return `{"error":"invalid file path"}`
	}
	// 读取文件
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return fmt.Sprintf(`{"error":"file not found: %s"}`, err.Error())
	}
	// 获取 MIME 类型
	mime := media.GetMediaType(filepath.Base(cleanPath))
	// 转换为 base64 数据 URL
	base64Data := base64.StdEncoding.EncodeToString(data)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mime, base64Data)
	return fmt.Sprintf(`{"dataUrl":"%s","mime":"%s","size":%d}`, dataURL, mime, len(data))
}

// ─────────── File Download ───────────

// DownloadFile 打开本地文件或 URL（桌面模式用系统默认程序打开）
func (a *AppService) DownloadFile(path, filename string) string {
	// URL 类型：用系统默认浏览器打开
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		if err := exec.Command("open", path).Start(); err != nil {
			// Windows
			exec.Command("cmd", "/c", "start", path).Start()
		}
		return `{"status":"opened"}`
	}

	// 将 file:// URL 转换为本地路径
	localPath := path
	if strings.HasPrefix(path, "file://") || strings.HasPrefix(path, "file:") {
		localPath = channel.FileURLToLocalPath(path)
	}

	// 本地文件：检查是否存在
	if _, err := os.Stat(localPath); err != nil {
		return fmt.Sprintf(`{"error":"文件不存在: %s"}`, localPath)
	}

	// 用系统默认程序打开文件所在目录（让用户自己选择操作）
	dir := filepath.Dir(localPath)
	var cmd *exec.Cmd
	if _, err := exec.LookPath("explorer"); err == nil {
		cmd = exec.Command("explorer", dir)
	} else if _, err := exec.LookPath("open"); err == nil {
		cmd = exec.Command("open", dir)
	} else {
		cmd = exec.Command("xdg-open", dir)
	}
	cmd.Start()
	return fmt.Sprintf(`{"status":"opened","path":"%s","filename":"%s"}`, localPath, filename)
}

// UploadChatFile 上传聊天文件（Wails 桌面端，接收 base64 编码）
func (a *AppService) UploadChatFile(sessionID, filename, base64Data string) string {
	logger := glog.Logger()
	logger.Info("UploadChatFile", "session", sessionID, "filename", filename)

	if sessionID == "" {
		sessionID = "default"
	}

	// 解码 base64
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return `{"error":"base64解码失败"}`
	}

	// 创建上传目录
	uploadDir := filepath.Join("clawdata", "uploads", sessionID)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return `{"error":"创建目录失败"}`
	}

	// 生成安全文件名
	safeFilename := filepath.Base(filepath.Clean(filename))
	if strings.Contains(safeFilename, "..") {
		return `{"error":"非法文件名"}`
	}

	// 保存文件
	destPath := filepath.Join(uploadDir, safeFilename)
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return `{"error":"保存文件失败"}`
	}

	// 生成 file:// URL
	fileURL := channel.PathToFileURL(destPath)
	mime := media.GetMediaType(safeFilename)
	isImage := strings.HasPrefix(mime, "image/")

	logger.Info("UploadChatFile", "path", fileURL, "size", len(data), "is_image", isImage)

	result := map[string]any{
		"status":    "ok",
		"path":      fileURL,
		"localPath": destPath,
		"filename":  safeFilename,
		"size":      len(data),
		"mime":      mime,
		"is_image":  isImage,
	}
	jsonData, _ := json.Marshal(result)
	return string(jsonData)
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
