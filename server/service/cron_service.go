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
	"go-claw/utils"
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
		job.ID = utils.UUID()
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
