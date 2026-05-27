package proactive

import (
	"context"
	"sync"
	"time"

	"go-claw/internal/agent"
	"go-claw/internal/memory"
	glog "go-claw/pkg/log"
)

// ProactiveManager 主动模式管理器
type ProactiveManager struct {
	config     Config
	agents     map[string]*agent.Agent
	memory     memory.Memory
	bus        ProactiveBus // 用于发送主动消息
	lastActive map[string]time.Time
	mu         sync.RWMutex
	stopChan   chan struct{}
}

// Config 主动模式配置
type Config struct {
	Enabled     bool
	IdleMinutes int    // 空闲多少分钟后触发
	AgentName   string // 使用哪个 Agent 执行主动任务
	CheckInterval int  // 检查间隔（秒），默认 60
}

// ProactiveBus 主动消息发送通道
type ProactiveBus interface {
	SendProactiveMessage(ctx context.Context, sessionID, message string) error
}

// NewManager 创建主动模式管理器
func NewManager(cfg Config, agents map[string]*agent.Agent, mem memory.Memory, bus ProactiveBus) *ProactiveManager {
	return &ProactiveManager{
		config:     cfg,
		agents:     agents,
		memory:     mem,
		bus:        bus,
		lastActive: make(map[string]time.Time),
		stopChan:   make(chan struct{}),
	}
}

// Start 启动主动模式监控
func (m *ProactiveManager) Start() {
	if !m.config.Enabled {
		return
	}

	logger := glog.Logger()
	logger.Info("[Proactive] 主动模式已启动", "idle_minutes", m.config.IdleMinutes)

	checkInterval := m.config.CheckInterval
	if checkInterval == 0 {
		checkInterval = 60
	}

	go func() {
		ticker := time.NewTicker(time.Duration(checkInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.checkIdleSessions()
			case <-m.stopChan:
				logger.Info("[Proactive] 主动模式已停止")
				return
			}
		}
	}()
}

// Stop 停止主动模式
func (m *ProactiveManager) Stop() {
	close(m.stopChan)
}

// UpdateActivity 更新会话活动时间
func (m *ProactiveManager) UpdateActivity(sessionID string) {
	m.mu.Lock()
	m.lastActive[sessionID] = time.Now()
	m.mu.Unlock()
}

// checkIdleSessions 检查空闲会话并触发主动任务
func (m *ProactiveManager) checkIdleSessions() {
	logger := glog.Logger()
	m.mu.RLock()
	defer m.mu.RUnlock()

	idleThreshold := time.Duration(m.config.IdleMinutes) * time.Minute

	for sessionID, lastTime := range m.lastActive {
		idleDuration := time.Since(lastTime)
		if idleDuration >= idleThreshold {
			logger.Info("[Proactive] 会话空闲超阈值，触发主动任务",
				"session", sessionID,
				"idle_minutes", int(idleDuration.Minutes()))

			// 分析记忆，提取可主动做的事情
			go m.executeProactiveTask(sessionID)
		}
	}
}

// executeProactiveTask 执行主动任务
func (m *ProactiveManager) executeProactiveTask(sessionID string) {
	logger := glog.Logger()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 获取目标 Agent
	targetAgent, exists := m.agents[m.config.AgentName]
	if !exists {
		logger.Warn("[Proactive] 目标 Agent 不存在", "agent", m.config.AgentName)
		return
	}

	// 从记忆中提取可能的主动任务
	proactivePrompt := `分析最近的对话记忆，判断是否有需要主动跟进的事项。

可能的主动任务类型：
1. 提醒用户完成未完成的任务
2. 提供相关的新信息或建议
3. 询问用户是否需要继续之前的话题

如果没有需要主动跟进的事项，返回"无"。
如果有，返回一条简洁的主动消息（不超过50字）。`

	// 调用 Agent 分析记忆
	response, err := targetAgent.Process(ctx, sessionID, proactivePrompt)
	if err != nil {
		logger.Warn("[Proactive] 主动任务分析失败", "err", err)
		return
	}

	// 如果有有意义的内容，发送给用户
	if response != "" && response != "无" {
		if err := m.bus.SendProactiveMessage(ctx, sessionID, response); err != nil {
			logger.Warn("[Proactive] 发送主动消息失败", "err", err)
		} else {
			logger.Info("[Proactive] 主动消息已发送", "session", sessionID, "msg_len", len(response))
			// 更新活动时间，避免重复发送
			m.UpdateActivity(sessionID)
		}
	}
}