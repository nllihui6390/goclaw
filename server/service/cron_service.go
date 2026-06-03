package service

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

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
	Schedule    string                 `json:"schedule"`
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
	mu       sync.RWMutex
	dataFile string
	executor *CronExecutor
	config   *ConfigService
}

// NewCronService 创建定时任务服务
func NewCronService(config *ConfigService) *CronService {
	return &CronService{
		dataFile: "clawdata/cron_jobs.json",
		config:   config,
	}
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

	return jobs
}

// Save 保存定时任务
func (s *CronService) Save(job CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.dataFile)
	var jobs []CronJob
	if err == nil {
		json.Unmarshal(data, &jobs)
	}

	if job.ID != "" {
		for i, j := range jobs {
			if j.ID == job.ID {
				jobs[i] = job
				break
			}
		}
	} else {
		job.ID = "cron-" + time.Now().Format("20060102150405")
		jobs = append(jobs, job)
	}

	data, _ = json.MarshalIndent(jobs, "", "  ")
	return os.WriteFile(s.dataFile, data, 0644)
}

// SaveJSON 保存定时任务（JSON 字符串输入）
func (s *CronService) SaveJSON(jobJSON string) error {
	var job CronJob
	if err := json.Unmarshal([]byte(jobJSON), &job); err != nil {
		return err
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

	sessionID := job.SessionID
	if sessionID == "" {
		sessionID = "console:cron"
	}

	go func() {
		logger := glog.Logger()
		logger.Info("[Cron] 手动执行任务", "id", job.ID, "name", job.Name, "type", job.Type)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		switch job.Type {
		case "text":
			if s.executor.SendMsg != nil {
				if err := s.executor.SendMsg(ctx, sessionID, job.Content); err != nil {
					logger.Warn("[Cron] 文本任务发送失败", "id", id, "err", err)
				} else {
					logger.Info("[Cron] 文本任务已发送", "id", id, "session", sessionID)
				}
			}

		case "agent":
			if s.executor.ProcessMsg != nil {
				result, err := s.executor.ProcessMsg(ctx, job.AgentName, sessionID, job.Content)
				if err != nil {
					logger.Warn("[Cron] Agent 任务执行失败", "id", id, "err", err)
					return
				}
				logger.Info("[Cron] Agent 任务执行完成", "id", id, "result_len", len(result))
				if s.executor.SendMsg != nil {
					if err := s.executor.SendMsg(ctx, sessionID, result); err != nil {
						logger.Warn("[Cron] Agent 结果发送失败", "id", id, "err", err)
					} else {
						logger.Info("[Cron] Agent 结果已发送", "id", id, "session", sessionID)
					}
				}
			}

		default:
			logger.Warn("[Cron] 未知任务类型", "type", job.Type)
		}
	}()
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
