package tool

import (
	"context"
	"fmt"
	"go-claw/internal/cron"
	"time"
)

// CronStatusTool 管理内部 cron 任务系统
type CronStatusTool struct {
	manager *cron.Manager
	executor cron.Executor
}

func NewCronStatusTool(manager *cron.Manager) *CronStatusTool {
	return &CronStatusTool{manager: manager}
}

// SetExecutor 设置执行器（用于立即执行任务）
func (t *CronStatusTool) SetExecutor(executor cron.Executor) {
	t.executor = executor
}

func (t *CronStatusTool) Name() string {
	return "cron_status"
}

func (t *CronStatusTool) Description() string {
	return `查询和管理 go-claw 程序内部的定时任务。

⚠️ 重要：这是 go-claw 程序内部的定时任务系统，不是操作系统的 crontab 或 schtasks。
查看定时任务请用此工具，不要用 crontab -l 或 schtasks 命令。

常用操作：
- 查看任务列表: cron_status(action="list")
- 查看任务详情: cron_status(action="get", id="任务ID")
- 新增任务: cron_status(action="add", name="任务名", schedule="09:00", type="agent", agent_name="default", content="任务内容")
- 立即执行: cron_status(action="run", id="任务ID")
- 删除任务: cron_status(action="delete", id="任务ID")

schedule 格式：
- "09:00" - 每天 9:00 执行
- "@every 5m" - 每 5 分钟执行
- "@every 1h" - 每 1 小时执行

type 类型：
- "text" - 发送文本消息到指定渠道
- "agent" - 调用 AI Agent 处理任务内容`
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

	if t.manager == nil {
		return "Cron 系统未启用。在 config.json 中设置 `cron.enabled: true` 并配置 jobs 数组来启用。", nil
	}

	switch action {
	case "list":
		return t.listJobs()
	case "get":
		return t.getJob(params)
	case "add":
		return t.addJob(params)
	case "update":
		return t.updateJob(params)
	case "delete":
		return t.deleteJob(params)
	case "enable":
		return t.enableJob(params)
	case "disable":
		return t.disableJob(params)
	case "run":
		return t.runJob(ctx, params)
	default:
		return "", fmt.Errorf("未知操作: %s (支持: list, get, add, update, delete, enable, disable, run)", action)
	}
}

func (t *CronStatusTool) listJobs() (string, error) {
	jobs := t.manager.ListJobs()
	if len(jobs) == 0 {
		return "当前没有配置任何定时任务。使用 cron_status(action=\"add\", ...) 新增任务。", nil
	}

	result := "## 定时任务列表\n\n"
	result += "| ID | 名称 | 调度 | 类型 | 启用 | 下次执行 | 上次执行 |\n"
	result += "|----|------|------|------|------|----------|----------|\n"
	for _, job := range jobs {
		enabled := "✅"
		if !job.Enabled {
			enabled = "❌"
		}
		nextRun := job.NextRun.Format("2006-01-02 15:04")
		lastRun := "-"
		if !job.LastRun.IsZero() {
			lastRun = job.LastRun.Format("2006-01-02 15:04")
		}
		agentInfo := ""
		if job.Type == cron.JobTypeAgent && job.AgentName != "" {
			agentInfo = " → " + job.AgentName
		}
		result += fmt.Sprintf("| %s | %s | %s | %s%s | %s | %s | %s |\n",
			job.ID, job.Name, job.Schedule, job.Type, agentInfo, enabled, nextRun, lastRun)
	}
	return result, nil
}

func (t *CronStatusTool) getJob(params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	job, err := t.manager.GetJob(id)
	if err != nil {
		return err.Error(), nil
	}

	result := fmt.Sprintf("## 任务详情: %s\n\n", job.ID)
	result += fmt.Sprintf("- **名称**: %s\n", job.Name)
	result += fmt.Sprintf("- **调度**: %s\n", job.Schedule)
	result += fmt.Sprintf("- **类型**: %s\n", job.Type)
	if job.Type == cron.JobTypeAgent {
		result += fmt.Sprintf("- **Agent**: %s\n", job.AgentName)
	}
	result += fmt.Sprintf("- **内容**: %s\n", truncStr(job.Content, 100))
	result += fmt.Sprintf("- **会话**: %s\n", job.SessionID)
	result += fmt.Sprintf("- **启用**: %v\n", job.Enabled)
	if job.ActiveStart != "" {
		result += fmt.Sprintf("- **活跃时段**: %s - %s\n", job.ActiveStart, job.ActiveEnd)
	}
	result += fmt.Sprintf("- **下次执行**: %s\n", job.NextRun.Format("2006-01-02 15:04"))
	if !job.LastRun.IsZero() {
		result += fmt.Sprintf("- **上次执行**: %s\n", job.LastRun.Format("2006-01-02 15:04"))
	}
	return result, nil
}

func (t *CronStatusTool) addJob(params map[string]interface{}) (string, error) {
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

	// 生成唯一 ID
	id := fmt.Sprintf("job_%s_%d", name, time.Now().UnixNano())

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

	t.manager.AddJob(job)

	return fmt.Sprintf("✅ 任务已添加\n- ID: %s\n- 名称: %s\n- 调度: %s\n- 下次执行: %s",
		id, name, schedule, job.NextRun.Format("2006-01-02 15:04")), nil
}

func (t *CronStatusTool) updateJob(params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	existing, err := t.manager.GetJob(id)
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

	t.manager.UpdateJob(existing)

	return fmt.Sprintf("✅ 任务已更新\n- ID: %s\n- 名称: %s\n- 下次执行: %s",
		id, existing.Name, existing.NextRun.Format("2006-01-02 15:04")), nil
}

func (t *CronStatusTool) deleteJob(params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	t.manager.RemoveJob(id)
	return fmt.Sprintf("✅ 任务已删除: %s", id), nil
}

func (t *CronStatusTool) enableJob(params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	t.manager.EnableJob(id)
	return fmt.Sprintf("✅ 任务已启用: %s", id), nil
}

func (t *CronStatusTool) disableJob(params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	t.manager.DisableJob(id)
	return fmt.Sprintf("✅ 任务已禁用: %s", id), nil
}

func (t *CronStatusTool) runJob(ctx context.Context, params map[string]interface{}) (string, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	err := t.manager.RunJobNow(id)
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