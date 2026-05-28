package cron

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	glog "go-claw/pkg/log"
)

// JobType 任务类型
type JobType string

const (
	JobTypeText  JobType = "text"   // 纯文本任务（发送消息）
	JobTypeAgent JobType = "agent"  // Agent 任务（调用 Agent 处理）
)

// Job 定时任务定义
type Job struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Type         JobType       `json:"type"`
	Schedule     string        `json:"schedule"`     // cron 表达式或间隔
	Content      string        `json:"content"`      // 任务内容
	AgentName    string        `json:"agent_name"`   // Agent 名称（仅 agent 类型）
	SessionID    string        `json:"session_id"`   // 目标会话 ID
	Enabled      bool          `json:"enabled"`
	LastRun      time.Time     `json:"last_run"`
	NextRun      time.Time     `json:"next_run"`
	ActiveStart  string        `json:"active_start"` // 活跃时段开始 (HH:MM)
	ActiveEnd    string        `json:"active_end"`   // 活跃时段结束 (HH:MM)
}

// Executor 任务执行器接口
type Executor interface {
	ExecuteText(ctx context.Context, sessionID, content string) error
	ExecuteAgent(ctx context.Context, agentName, sessionID, content string) (string, error)
}

// Manager 定时任务管理器
type Manager struct {
	jobs     map[string]*Job
	executor Executor
	mu       sync.RWMutex
	stopChan chan struct{}
	wg       sync.WaitGroup
	jobWg    sync.WaitGroup // 等待正在执行的任务
}

// NewManager 创建定时任务管理器
func NewManager(executor Executor) *Manager {
	return &Manager{
		jobs:     make(map[string]*Job),
		executor: executor,
		stopChan: make(chan struct{}),
	}
}

// AddJob 添加任务
func (m *Manager) AddJob(job *Job) {
	m.mu.Lock()
	defer m.mu.Unlock()

	job.NextRun = m.parseSchedule(job.Schedule)
	m.jobs[job.ID] = job

	glog.Logger().Info("[Cron] 任务已添加", "id", job.ID, "name", job.Name, "next_run", job.NextRun)
}

// RemoveJob 删除任务
func (m *Manager) RemoveJob(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.jobs, id)
	glog.Logger().Info("[Cron] 任务已删除", "id", id)
}

// ListJobs 列出所有任务
func (m *Manager) ListJobs() []Job {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var jobs []Job
	for _, j := range m.jobs {
		jobs = append(jobs, *j)
	}
	return jobs
}

// EnableJob 启用任务
func (m *Manager) EnableJob(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, exists := m.jobs[id]; exists {
		job.Enabled = true
		job.NextRun = m.parseSchedule(job.Schedule)
	}
}

// DisableJob 禁用任务
func (m *Manager) DisableJob(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, exists := m.jobs[id]; exists {
		job.Enabled = false
	}
}

// UpdateJob 更新任务（通过删除再添加实现）
func (m *Manager) UpdateJob(job *Job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.jobs[job.ID]; exists {
		job.NextRun = m.parseSchedule(job.Schedule)
		m.jobs[job.ID] = job
		glog.Logger().Info("[Cron] 任务已更新", "id", job.ID, "name", job.Name, "next_run", job.NextRun)
	}
}

// RunJobNow 立即执行任务（独立线程，不阻塞调用方）
func (m *Manager) RunJobNow(id string) error {
	m.mu.RLock()
	job, exists := m.jobs[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("任务不存在: %s", id)
	}

	glog.Logger().Info("[Cron] 手动触发任务", "id", id, "name", job.Name)
	m.runJobAsync(job)

	m.mu.Lock()
	job.LastRun = time.Now()
	m.mu.Unlock()

	return nil
}

// GetJob 获取单个任务
func (m *Manager) GetJob(id string) (*Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if job, exists := m.jobs[id]; exists {
		return job, nil
	}
	return nil, fmt.Errorf("任务不存在: %s", id)
}

// Start 启动任务调度
func (m *Manager) Start() {
	glog.Logger().Info("[Cron] 定时任务管理器已启动")

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.checkAndRun()
			case <-m.stopChan:
				return
			}
		}
	}()
}

// Stop 停止任务调度（等待正在执行的任务完成）
func (m *Manager) Stop() {
	close(m.stopChan)
	m.wg.Wait()
	m.jobWg.Wait()
	glog.Logger().Info("[Cron] 定时任务管理器已停止")
}

// checkAndRun 检查并执行到期任务
func (m *Manager) checkAndRun() {
	m.mu.Lock()
	now := time.Now()
	var jobsToRun []*Job
	for _, job := range m.jobs {
		if !job.Enabled {
			continue
		}
		// 检查活跃时段
		if !m.inActiveHours(job, now) {
			continue
		}
		// 检查是否到期
		if now.After(job.NextRun) || now.Equal(job.NextRun) {
			jobsToRun = append(jobsToRun, job)
			job.LastRun = now
			job.NextRun = m.parseSchedule(job.Schedule)
		}
	}
	m.mu.Unlock()

	// 在独立 goroutine 中执行每个任务
	for _, job := range jobsToRun {
		m.runJobAsync(job)
	}
}

// runJobAsync 在独立 goroutine 中执行任务，不阻塞调度线程
func (m *Manager) runJobAsync(job *Job) {
	m.jobWg.Add(1)
	go func() {
		defer m.jobWg.Done()
		m.executeJob(job)
	}()
}

// executeJob 执行单个任务（在独立 goroutine 中运行）
func (m *Manager) executeJob(job *Job) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	logger := glog.Logger()
	logger.Info("[Cron] 执行任务（独立线程）", "id", job.ID, "name", job.Name)

	switch job.Type {
	case JobTypeText:
		if err := m.executor.ExecuteText(ctx, job.SessionID, job.Content); err != nil {
			logger.Warn("[Cron] 任务执行失败", "id", job.ID, "err", err)
		}
	case JobTypeAgent:
		result, err := m.executor.ExecuteAgent(ctx, job.AgentName, job.SessionID, job.Content)
		if err != nil {
			logger.Warn("[Cron] Agent 任务执行失败", "id", job.ID, "err", err)
		} else {
			logger.Info("[Cron] Agent 任务执行完成", "id", job.ID, "result_len", len(result))
		}
	}
}

// parseSchedule 解析调度表达式
func (m *Manager) parseSchedule(schedule string) time.Time {
	// 支持格式:
	// - "@every 5m" - 每 5 分钟
	// - "@every 1h" - 每 1 小时
	// - "HH:MM" - 每天 HH:MM
	// - "0 7 * * *" - 标准 5 字段 cron 表达式（分 时 日 月 周）

	now := time.Now()

	if strings.HasPrefix(schedule, "@every ") {
		duration := parseDuration(schedule[7:])
		return now.Add(duration)
	}

	// HH:MM 格式 (如 "07:00", "21:00")
	if len(schedule) == 5 && schedule[2] == ':' {
		hour := parseInt(schedule[:2])
		minute := parseInt(schedule[3:5])
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if next.Before(now) {
			next = next.Add(24 * time.Hour)
		}
		return next
	}

	// 标准 5 字段 cron 表达式: 分 时 日 月 周
	// 如 "0 7 * * *" = 每天 7:00, "0 * * * *" = 每小时整点, "*/5 * * * *" = 每 5 分钟
	fields := strings.Fields(schedule)
	if len(fields) == 5 {
		return m.parseCronExpr(fields, now)
	}

	// 默认每分钟检查一次（无法解析的格式）
	return now.Add(1 * time.Minute)
}

// parseCronExpr 解析标准 5 字段 cron 表达式
// 格式: 分(0-59) 时(0-23) 日(1-31) 月(1-12) 周(0-6)
// 支持: *, 具体数字, */N 步进, a-b 范围, a,b,c 列表
func (m *Manager) parseCronExpr(fields []string, now time.Time) time.Time {
	minuteField := fields[0]
	hourField := fields[1]
	dayField := fields[2]
	monthField := fields[3]
	weekField := fields[4]

	// 从下一分钟开始搜索（确保不会立即再次触发）
	start := now.Add(1 * time.Minute)

	// 搜索最多 366 天内的下一个匹配时间
	t := time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), start.Minute(), 0, 0, start.Location())
	deadline := now.AddDate(1, 0, 0) // 最大搜索 1 年

	for t.Before(deadline) {
		// 检查月
		if !cronFieldMatches(monthField, int(t.Month())) {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		// 检查日
		if !cronFieldMatches(dayField, t.Day()) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}

		// 检查周 (0=Sunday, 1=Monday, ..., 6=Saturday)
		weekday := int(t.Weekday())
		if !cronFieldMatches(weekField, weekday) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}

		// 检查时
		if !cronFieldMatches(hourField, t.Hour()) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}

		// 检查分
		if !cronFieldMatches(minuteField, t.Minute()) {
			nextMin := findNextMatch(minuteField, t.Minute(), 60)
			if nextMin == -1 || nextMin <= t.Minute() {
				// 当前小时内没有匹配的分钟或已过，跳到下一小时
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			} else {
				// 下一个匹配分钟在当前小时内
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), nextMin, 0, 0, t.Location())
			}
			continue
		}

		// 所有字段匹配
		return t
	}

	// 超过 1 年未找到匹配时间，返回一个远期时间避免频繁触发
	return now.AddDate(1, 0, 0)
}

// findNextMatch 找到大于 current 的最小匹配值
// 如果没有这样的值，返回 -1
func findNextMatch(field string, current int, maxVal int) int {
	for v := current + 1; v < maxVal; v++ {
		if cronFieldMatches(field, v) {
			return v
		}
	}
	return -1
}

// cronFieldMatches 检查 cron 字段是否匹配给定值
// 支持: *, 具体数字, */N 步进, 范围 a-b, 列表 a,b,c
func cronFieldMatches(field string, value int) bool {
	if field == "*" {
		return true
	}

	// */N 步进
	if strings.HasPrefix(field, "*/") {
		step := parseInt(field[2:])
		if step == 0 {
			return true
		}
		return value%step == 0
	}

	// 范围 a-b
	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		start := parseInt(parts[0])
		end := parseInt(parts[1])
		return value >= start && value <= end
	}

	// 列表 a,b,c
	if strings.Contains(field, ",") {
		for _, v := range strings.Split(field, ",") {
			if parseInt(v) == value {
				return true
			}
		}
		return false
	}

	// 具体数字
	return parseInt(field) == value
}

// inActiveHours 检查是否在活跃时段内
func (m *Manager) inActiveHours(job *Job, now time.Time) bool {
	if job.ActiveStart == "" || job.ActiveEnd == "" {
		return true // 未设置活跃时段，始终活跃
	}

	startHour := parseInt(job.ActiveStart[:2])
	startMin := parseInt(job.ActiveStart[3:5])
	endHour := parseInt(job.ActiveEnd[:2])
	endMin := parseInt(job.ActiveEnd[3:5])

	currentHour := now.Hour()
	currentMin := now.Minute()

	// 转换为分钟数比较
	startMins := startHour*60 + startMin
	endMins := endHour*60 + endMin
	currentMins := currentHour*60 + currentMin

	// 处理跨午夜的情况
	if endMins < startMins {
		// 活跃时段跨午夜 (如 22:00 - 02:00)
		return currentMins >= startMins || currentMins <= endMins
	}

	return currentMins >= startMins && currentMins <= endMins
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 1 * time.Minute
	}
	return d
}

func parseInt(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}