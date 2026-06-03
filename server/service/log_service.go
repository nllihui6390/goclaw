package service

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// LogService 日志服务
type LogService struct {
	mu        sync.RWMutex
	maxSize   int
	logFile   string
}

// NewLogService 创建日志服务
func NewLogService() *LogService {
	return &LogService{
		logFile: "logs/app.log",
		maxSize: 50000, // 50KB
	}
}

// Get 获取最新日志内容
func (s *LogService) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.logFile)
	if err != nil {
		return "暂无日志"
	}

	if len(data) > s.maxSize {
		data = data[len(data)-s.maxSize:]
	}

	return string(data)
}

// GetWithLimit 获取指定大小的日志内容
func (s *LogService) GetWithLimit(maxSize int) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.logFile)
	if err != nil {
		return "暂无日志"
	}

	if maxSize > 0 && len(data) > maxSize {
		data = data[len(data)-maxSize:]
	}

	return string(data)
}

// StatusInfo 系统状态信息
type StatusInfo struct {
	Status string `json:"status"`
	Uptime string `json:"uptime"`
}

// StatusService 状态服务
type StatusService struct {
	startTime time.Time
}

// NewStatusService 创建状态服务
func NewStatusService() *StatusService {
	return &StatusService{
		startTime: time.Now(),
	}
}

// Get 获取系统状态
func (s *StatusService) Get() StatusInfo {
	return StatusInfo{
		Status: "running",
		Uptime: s.startTime.Format(time.RFC3339),
	}
}

// GetJSON 获取系统状态 JSON 字符串
func (s *StatusService) GetJSON() string {
	status := s.Get()
	data, _ := json.Marshal(status)
	return string(data)
}