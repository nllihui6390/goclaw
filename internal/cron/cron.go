package cron

import (
	"context"
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

// Stop 停止任务调度
func (m *Manager) Stop() {
	close(m.stopChan)
	m.wg.Wait()
	glog.Logger().Info("[Cron] 定时任务管理器已停止")
}

// checkAndRun 检查并执行到期任务
func (m *Manager) checkAndRun() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
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
			m.runJob(job)
			job.LastRun = now
			job.NextRun = m.parseSchedule(job.Schedule)
		}
	}
}

// runJob 执行单个任务
func (m *Manager) runJob(job *Job) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger := glog.Logger()
	logger.Info("[Cron] 执行任务", "id", job.ID, "name", job.Name)

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
	// - "*/5 * * * *" - 标准 cron 表达式

	now := time.Now()

	if strings.HasPrefix(schedule, "@every ") {
		duration := parseDuration(schedule[7:])
		return now.Add(duration)
	}

	// HH:MM 格式
	if len(schedule) == 5 && schedule[2] == ':' {
		hour := parseInt(schedule[:2])
		minute := parseInt(schedule[3:5])
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if next.Before(now) {
			next = next.Add(24 * time.Hour)
		}
		return next
	}

	// 默认每分钟检查一次
	return now.Add(1 * time.Minute)
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