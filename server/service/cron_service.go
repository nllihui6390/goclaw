package service

import (
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

// CronService 定时任务服务
type CronService struct {
	mu        sync.RWMutex
	dataFile  string
	executor  func(id string)
	config    *ConfigService
}

// NewCronService 创建定时任务服务
func NewCronService(config *ConfigService) *CronService {
	return &CronService{
		dataFile: "clawdata/cron_jobs.json",
		config:   config,
	}
}

// SetExecutor 设置执行器
func (s *CronService) SetExecutor(executor func(id string)) {
	s.executor = executor
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

	// 如果有 ID 则更新，否则追加
	if job.ID != "" {
		for i, j := range jobs {
			if j.ID == job.ID {
				jobs[i] = job
				break
			}
		}
	} else {
		// 生成新 ID
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

// Run 执行定时任务
func (s *CronService) Run(id string) {
	if s.executor == nil {
		return
	}

	go func() {
		glog.Logger().Info("[Cron] 手动执行任务", "id", id)
		s.executor(id)
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