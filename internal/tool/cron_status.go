package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"go-claw/internal/cron"
	"go-claw/pkg/utils"
)

// 全局 cron manager（在 main.go 中设置）
var globalCronManager *cron.Manager

// SetGlobalCronManager 设置全局 cron manager
func SetGlobalCronManager(manager *cron.Manager) {
	globalCronManager = manager
}

// CronStatusTool 管理内部 cron 任务系统
type CronStatusTool struct {
	manager  *cron.Manager
	executor cron.Executor
}

func NewCronStatusTool(manager *cron.Manager) *CronStatusTool {
	return &CronStatusTool{manager: manager}
}

func (t *CronStatusTool) Name() string {
	return "cron_status"
}

func (t *CronStatusTool) Description() string {
	return `管理 go-claw 内部的定时任务系统。
	执行操作时必须调用对应工具，绝不能只声称已执行而不实际调用工具
⚠️ 重要：用户要求"执行"任务时，必须调用 action="run"，不能只描述任务信息！

操作说明：
- list: 查看所有任务列表
- run: 立即执行指定任务（需要 id 参数）← 执行任务用这个！
- get: 查看单个任务详情（需要 id）
- add: 新增定时任务
- delete: 删除任务

示例：
- cron_status(action="list")  ← 先查看列表获取任务ID
- cron_status(action="run", id="cron_1_每日问候")  ← 然后执行！
- cron_status(action="get", id="cron_1_每日问候")
- cron_status(action="add", name="早报", schedule="0 7 * * *", type="agent", agent_name="default", content="生成每日早报")

schedule 格式：
- "0 7 * * *" - 每天 7:00（标准5字段cron: 分 时 日 月 周）
- "0 * * * *" - 每小时整点
- "*/15 * * * *" - 每 15 分钟
- "0 9-17 * * 1-5" - 工作日 9-17 点整点
- "09:00" - 每天 9:00（简写格式）
- "@every 5m" - 每 5 分钟

type: "text" 发送消息 | "agent" 调用AI处理`
}

func (t *CronStatusTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "操作类型: list, get, add, update, delete, enable, disable, run",
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "任务ID",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "任务名称",
			},
			"schedule": map[string]interface{}{
				"type":        "string",
				"description": "调度时间: @every 5m, 09:00, 或 cron 表达式",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "任务类型: text (发送消息) 或 agent (调用AI)",
			},
			"agent_name": map[string]interface{}{
				"type":        "string",
				"description": "Agent名称（type=agent时必填）",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "任务内容",
			},
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "目标会话ID（默认 console:cron）",
			},
			"enabled": map[string]interface{}{
				"type":        "boolean",
				"description": "是否启用",
			},
			"active_start": map[string]interface{}{
				"type":        "string",
				"description": "活跃时段开始 HH:MM",
			},
			"active_end": map[string]interface{}{
				"type":        "string",
				"description": "活跃时段结束 HH:MM",
			},
		},
		"required": []string{"action"},
	}
}

func (t *CronStatusTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	action, ok := params["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("缺少 action 参数")
	}

	// 获取 manager（实例优先，否则使用全局）
	mgr := t.manager
	if mgr == nil {
		mgr = globalCronManager
	}

	if mgr == nil {
		return "Cron 系统未启用。在 config.json 中设置 `cron.enabled: true` 并配置 jobs 数组来启用。", nil
	}

	switch action {
	case "list":
		return t.listJobs(mgr)
	case "get":
		return t.getJob(mgr, params)
	case "add":
		return t.addJob(mgr, params)
	case "update":
		return t.updateJob(mgr, params)
	case "delete":
		return t.deleteJob(mgr, params)
	case "run":
		return t.runJob(ctx, mgr, params)
	default:
		return "", fmt.Errorf("未知操作: %s (支持: list, get, add, update, delete, run)", action)
	}
}

func (t *CronStatusTool) listJobs(mgr *cron.Manager) (string, error) {
	jobs := mgr.ListJobs()
	if len(jobs) == 0 {
		return "当前没有配置任何定时任务。使用 cron_status(action=\"add\", ...) 新增任务。", nil
	}

	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化任务列表失败: %w", err)
	}
	return string(data), nil
}

func (t *CronStatusTool) getJob(mgr *cron.Manager, params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	job, err := mgr.GetJob(id)
	if err != nil {
		return err.Error(), nil
	}

	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化任务详情失败: %w", err)
	}
	return string(data), nil
}

func (t *CronStatusTool) addJob(mgr *cron.Manager, params map[string]interface{}) (string, error) {
	name, ok := params["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("缺少 name 参数")
	}
	schedule, ok := params["schedule"].(string)
	if !ok || schedule == "" {
		return "", fmt.Errorf("缺少 schedule 参数")
	}
	jobTypeStr, ok := params["type"].(string)
	if !ok || jobTypeStr == "" {
		return "", fmt.Errorf("缺少 type 参数")
	}

	jobType := cron.JobTypeText
	if jobTypeStr == "agent" {
		jobType = cron.JobTypeAgent
	}

	content, _ := params["content"].(string)
	if content == "" {
		return "", fmt.Errorf("缺少 content 参数")
	}

	// 生成 UUID
	id := utils.UUID()

	job := &cron.Job{
		ID:        id,
		Name:      name,
		Schedule:  schedule,
		Type:      jobType,
		Content:   content,
		SessionID: getStringOr(params, "session_id", "console:cron"),
		Enabled:   true,
	}

	if jobType == cron.JobTypeAgent {
		job.AgentName = getStringOr(params, "agent_name", "default")
	}

	job.ActiveStart = getStringOr(params, "active_start", "")
	job.ActiveEnd = getStringOr(params, "active_end", "")

	mgr.AddJob(job)

	return fmt.Sprintf("✅ 任务已添加\n- ID: %s\n- 名称: %s\n- 调度: %s\n- 下次执行: %s",
		id, name, schedule, cron.ParseTime(job.NextRun).Format("2006-01-02 15:04")), nil
}

func (t *CronStatusTool) updateJob(mgr *cron.Manager, params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	existing, err := mgr.GetJob(id)
	if err != nil {
		return err.Error(), nil
	}

	// 更新字段（只更新提供的字段）
	if name, ok := params["name"].(string); ok && name != "" {
		existing.Name = name
	}
	if schedule, ok := params["schedule"].(string); ok && schedule != "" {
		existing.Schedule = schedule
	}
	if content, ok := params["content"].(string); ok {
		existing.Content = content
	}
	if sessionID, ok := params["session_id"].(string); ok {
		existing.SessionID = sessionID
	}
	if agentName, ok := params["agent_name"].(string); ok {
		existing.AgentName = agentName
	}
	if activeStart, ok := params["active_start"].(string); ok {
		existing.ActiveStart = activeStart
	}
	if activeEnd, ok := params["active_end"].(string); ok {
		existing.ActiveEnd = activeEnd
	}
	if enabled, ok := params["enabled"].(bool); ok {
		existing.Enabled = enabled
	}

	mgr.UpdateJob(existing)

	return fmt.Sprintf("✅ 任务已更新\n- ID: %s\n- 名称: %s\n- 下次执行: %s",
		id, existing.Name, cron.ParseTime(existing.NextRun).Format("2006-01-02 15:04")), nil
}

func (t *CronStatusTool) deleteJob(mgr *cron.Manager, params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	mgr.RemoveJob(id)
	return fmt.Sprintf("✅ 任务已删除: %s", id), nil
}

func (t *CronStatusTool) enableJob(mgr *cron.Manager, params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	mgr.EnableJob(id)
	return fmt.Sprintf("✅ 任务已启用: %s", id), nil
}

func (t *CronStatusTool) disableJob(mgr *cron.Manager, params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	mgr.DisableJob(id)
	return fmt.Sprintf("✅ 任务已禁用: %s", id), nil
}

func (t *CronStatusTool) runJob(ctx context.Context, mgr *cron.Manager, params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	err := mgr.RunJobNow(id)
	if err != nil {
		return err.Error(), nil
	}

	return fmt.Sprintf("✅ 任务已立即执行: %s\n执行结果已记录到日志。", id), nil
}

// 辅助函数
func getStringOr(params map[string]interface{}, key, defaultVal string) string {
	if v, ok := params[key]; ok {
		if s, ok2 := v.(string); ok2 && s != "" {
			return s
		}
	}
	return defaultVal
}

func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// init 在程序启动时注册工具
func init() {
	GlobalRegistry.Register("cron_status", func() Tool {
		return &CronStatusTool{}
	})
}
