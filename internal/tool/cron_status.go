package tool

import (
	"context"
	"go-claw/internal/cron"
)

// CronStatusTool 查询内部 cron 任务状态
type CronStatusTool struct {
	manager *cron.Manager
}

func NewCronStatusTool(manager *cron.Manager) *CronStatusTool {
	return &CronStatusTool{manager: manager}
}

func (t *CronStatusTool) Name() string {
	return "cron_status"
}

func (t *CronStatusTool) Description() string {
	return "查询 go-claw 内部定时任务系统状态。显示所有已配置的 cron 任务及其执行状态。\n这是程序内部的定时任务系统，不是操作系统的 cron。"
}

func (t *CronStatusTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *CronStatusTool) Execute(ctx context.Context, _ map[string]interface{}) (string, error) {
	if t.manager == nil {
		return "Cron 系统未启用。在 config.json 中设置 `cron.enabled: true` 并配置 jobs 数组来启用。", nil
	}

	jobs := t.manager.ListJobs()
	if len(jobs) == 0 {
		return "当前没有配置任何定时任务。在 config.json 的 `cron.jobs` 数组中添加任务配置。", nil
	}

	result := "## go-claw 内部定时任务列表\n\n"
	result += "| 任务名 | 调度 | 类型 | 启用 | 下次执行 | 上次执行 |\n"
	result += "|--------|------|------|------|----------|----------|\n"
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
		result += "| " + job.Name + " | " + job.Schedule + " | " + string(job.Type) + " | " + enabled + " | " + nextRun + " | " + lastRun + " |\n"
	}
	return result, nil
}