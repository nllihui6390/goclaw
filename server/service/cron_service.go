package service

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"go-claw/internal/agent"
	"go-claw/internal/store"
	"go-claw/pkg/utils"
	glog "go-claw/pkg/log"
)

// CronJob 定时任务定义
type CronJob struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Content     string                 `json:"content"`
	SessionID   string                 `json:"session_id"`
	AgentName   string                 `json:"agent_name"`
	Enabled     bool                   `json:"enabled"`
	LastRun     string                 `json:"last_run,omitempty"`
	NextRun     string                 `json:"next_run,omitempty"`
	Schedule    string                 `json:"schedule"`
	ActiveStart string                 `json:"active_start,omitempty"`
	ActiveEnd   string                 `json:"active_end,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CronExecutor 定时任务执行所需回调（HTTP/Wails 各自注入）
type CronExecutor struct {
	// SendMsg 发送消息到指定会话（对应 text 类型任务）
	SendMsg func(ctx context.Context, sessionID, message string) error
	// ProcessMsg 调用 Agent 处理消息（对应 agent 类型任务）
	ProcessMsg func(ctx context.Context, agentName, sessionID, content string) (string, error)
}

// CronService 定时任务服务
type CronService struct {
	mu           sync.RWMutex
	dataFile     string
	executor     *CronExecutor
	config       *ConfigService
	sessionIndex *store.SessionIndex
}

// NewCronService 创建定时任务服务
func NewCronService(config *ConfigService) *CronService {
	return &CronService{
		dataFile: "clawdata/cron_jobs.json",
		config:   config,
	}
}

// SetSessionIndex 注入会话索引（用于 cron 执行时映射 channel:user → UUID）
func (s *CronService) SetSessionIndex(idx *store.SessionIndex) {
	s.sessionIndex = idx
}

// SetExecutor 设置执行器
func (s *CronService) SetExecutor(e *CronExecutor) {
	s.executor = e
}

// List 获取所有定时任务
func (s *CronService) List() []CronJob {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.dataFile)
	if err != nil {
		return nil
	}

	var jobs []CronJob
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil
	}

	// 为没有 NextRun 的任务计算下次执行时间
	for i := range jobs {
		if jobs[i].NextRun == "" && jobs[i].Schedule != "" {
			jobs[i].NextRun = computeNextRun(jobs[i].Schedule)
		}
	}

	return jobs
}

// Save 保存定时任务，返回生成的 ID（新任务自动生成 UUID）
func (s *CronService) Save(job CronJob) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.dataFile)
	var jobs []CronJob
	if err == nil {
		json.Unmarshal(data, &jobs)
	}

	// 计算下次执行时间
	job.NextRun = computeNextRun(job.Schedule)

	if job.ID != "" {
		for i, j := range jobs {
			if j.ID == job.ID {
				jobs[i] = job
				break
			}
		}
	} else {
		job.ID = utils.UUID()
		jobs = append(jobs, job)
	}

	data, _ = json.MarshalIndent(jobs, "", "  ")
	if err := os.WriteFile(s.dataFile, data, 0644); err != nil {
		return "", err
	}
	return job.ID, nil
}

// SaveJSON 保存定时任务（JSON 字符串输入），返回生成的 ID
func (s *CronService) SaveJSON(jobJSON string) (string, error) {
	var job CronJob
	if err := json.Unmarshal([]byte(jobJSON), &job); err != nil {
		return "", err
	}
	return s.Save(job)
}

// Delete 删除定时任务
func (s *CronService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.dataFile)
	if err != nil {
		return nil
	}

	var jobs []CronJob
	json.Unmarshal(data, &jobs)

	filtered := make([]CronJob, 0, len(jobs))
	for _, j := range jobs {
		if j.ID != id {
			filtered = append(filtered, j)
		}
	}

	data, _ = json.MarshalIndent(filtered, "", "  ")
	return os.WriteFile(s.dataFile, data, 0644)
}

// Run 异步执行指定定时任务（HTTP/Wails 通用）
func (s *CronService) Run(id string) {
	if s.executor == nil {
		return
	}

	// 读取任务
	data, err := os.ReadFile(s.dataFile)
	if err != nil {
		return
	}
	var jobs []CronJob
	if err := json.Unmarshal(data, &jobs); err != nil {
		return
	}

	var job *CronJob
	for i := range jobs {
		if jobs[i].ID == id {
			job = &jobs[i]
			break
		}
	}
	if job == nil {
		return
	}

	// 发送消息用的原始会话 ID（channel:user 格式，SendProactiveMessage 需要）
	sendSessionID := job.SessionID
	if sendSessionID == "" {
		sendSessionID = "console:cron"
	}
	// Cron 任务独立会话：用 cron:channel:user 作为索引键，与正常聊天隔离
	sendParts := strings.SplitN(sendSessionID, ":", 2)
	cronChannel, cronUser := "cron", sendSessionID
	if len(sendParts) == 2 {
		cronChannel, cronUser = sendParts[0], sendParts[1]
	}
	// Agent 处理用的会话 ID（映射到 UUID，用于存储，独立于正常聊天）
	processSessionID := sendSessionID
	if s.sessionIndex != nil && !utils.IsUUID(processSessionID) {
		uuid, _ := s.sessionIndex.LookupOrCreate("cron:"+cronChannel, cronUser, job.AgentName)
		processSessionID = uuid
	}

	go func() {
		logger := glog.Logger()
		logger.Info("[Cron] 手动执行任务", "id", job.ID, "name", job.Name, "type", job.Type)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// 注入 channel/user 到 context（用于 Session 正确记录来源）
		ctx = agent.WithChannel(ctx, cronChannel)
		ctx = agent.WithUser(ctx, cronUser)

		switch job.Type {
		case "text":
			if s.executor.SendMsg != nil {
				if err := s.executor.SendMsg(ctx, sendSessionID, job.Content); err != nil {
					logger.Warn("[Cron] 文本任务发送失败", "id", id, "err", err)
				} else {
					logger.Info("[Cron] 文本任务已发送", "id", id, "session", sendSessionID)
				}
			}

		case "agent":
			if s.executor.ProcessMsg != nil {
				result, err := s.executor.ProcessMsg(ctx, job.AgentName, processSessionID, job.Content)
				if err != nil {
					logger.Warn("[Cron] Agent 任务执行失败", "id", id, "err", err)
					return
				}
				logger.Info("[Cron] Agent 任务执行完成", "id", id, "result_len", len(result))
				// 更新会话索引
				if s.sessionIndex != nil {
					s.sessionIndex.UpdateName(processSessionID, job.Content, job.AgentName)
					s.sessionIndex.Touch(processSessionID)
				}
				if s.executor.SendMsg != nil {
					if err := s.executor.SendMsg(ctx, sendSessionID, result); err != nil {
						logger.Warn("[Cron] Agent 结果发送失败", "id", id, "err", err)
					} else {
						logger.Info("[Cron] Agent 结果已发送", "id", id, "session", sendSessionID)
					}
				}
			}

		default:
			logger.Warn("[Cron] 未知任务类型", "type", job.Type)
		}

		// 更新 LastRun
		s.updateLastRun(id)
	}()
}

// updateLastRun 更新任务的 LastRun 时间
func (s *CronService) updateLastRun(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.dataFile)
	if err != nil {
		return
	}
	var jobs []CronJob
	if err := json.Unmarshal(data, &jobs); err != nil {
		return
	}

	for i := range jobs {
		if jobs[i].ID == id {
			jobs[i].LastRun = time.Now().Format(time.RFC3339)
			break
		}
	}

	data, _ = json.MarshalIndent(jobs, "", "  ")
	os.WriteFile(s.dataFile, data, 0644)
}

// GetEnabled 获取启用状态
func (s *CronService) GetEnabled() bool {
	cfg := s.config.Get()
	cronCfg, _ := cfg["cron"].(map[string]interface{})
	enabled := true
	if v, ok := cronCfg["enabled"]; ok {
		enabled = v == true
	}
	return enabled
}

// SetEnabled 设置启用状态
func (s *CronService) SetEnabled(enabled bool) error {
	cfg := s.config.Get()
	if cfg["cron"] == nil {
		cfg["cron"] = make(map[string]interface{})
	}
	cfg["cron"].(map[string]interface{})["enabled"] = enabled
	return s.config.Save(cfg)
}

// computeNextRun 计算下次执行时间
func computeNextRun(schedule string) string {
	now := time.Now()

	// @every Nd 格式
	if strings.HasPrefix(schedule, "@every ") {
		d, err := time.ParseDuration(schedule[7:])
		if err != nil {
			return ""
		}
		return now.Add(d).Format(time.RFC3339)
	}

	// HH:MM 格式 (如 "09:00")
	if len(schedule) == 5 && schedule[2] == ':' {
		hour := parseInt(schedule[:2])
		minute := parseInt(schedule[3:5])
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if next.Before(now) || next.Equal(now) {
			next = next.Add(24 * time.Hour)
		}
		return next.Format(time.RFC3339)
	}

	// 标准 5 字段 cron 表达式
	fields := strings.Fields(schedule)
	if len(fields) == 5 {
		next := parseCronExpr(fields, now)
		return next.Format(time.RFC3339)
	}

	return ""
}

// parseInt 解析整数
func parseInt(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

// parseCronExpr 解析标准 5 字段 cron 表达式
func parseCronExpr(fields []string, now time.Time) time.Time {
	minuteField := fields[0]
	hourField := fields[1]
	dayField := fields[2]
	monthField := fields[3]
	weekField := fields[4]

	start := now.Add(1 * time.Minute)
	t := time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), start.Minute(), 0, 0, start.Location())
	deadline := now.AddDate(1, 0, 0)

	for t.Before(deadline) {
		if !cronFieldMatches(monthField, int(t.Month())) {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !cronFieldMatches(dayField, t.Day()) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		weekday := int(t.Weekday())
		if !cronFieldMatches(weekField, weekday) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !cronFieldMatches(hourField, t.Hour()) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if !cronFieldMatches(minuteField, t.Minute()) {
			nextMin := findNextMatch(minuteField, t.Minute(), 60)
			if nextMin == -1 || nextMin <= t.Minute() {
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			} else {
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), nextMin, 0, 0, t.Location())
			}
			continue
		}
		return t
	}
	return now.AddDate(1, 0, 0)
}

// cronFieldMatches 检查 cron 字段是否匹配
func cronFieldMatches(field string, value int) bool {
	if field == "*" {
		return true
	}
	if strings.HasPrefix(field, "*/") {
		step := parseInt(field[2:])
		if step == 0 {
			return true
		}
		return value%step == 0
	}
	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		start := parseInt(parts[0])
		end := parseInt(parts[1])
		return value >= start && value <= end
	}
	if strings.Contains(field, ",") {
		for _, v := range strings.Split(field, ",") {
			if parseInt(v) == value {
				return true
			}
		}
		return false
	}
	return parseInt(field) == value
}

// findNextMatch 找到大于 current 的最小匹配值
func findNextMatch(field string, current int, maxVal int) int {
	for v := current + 1; v < maxVal; v++ {
		if cronFieldMatches(field, v) {
			return v
		}
	}
	return -1
}
