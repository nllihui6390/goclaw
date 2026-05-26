package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-claw/internal/memory"
	"go-claw/internal/store"
	"go-claw/internal/tool"
	glog "go-claw/pkg/log"
)

// Config Agent配置
type Config struct {
	Name          string
	SystemPrompt  string
	Model         string
	APIKey        string
	BaseURL       string
	ProviderType  string            // 供应商类型: openai, ollama, anthropic, azure
	Tools         []tool.Tool
	MaxIterations int
	MaxTokens     int               // 最大上下文 Token 数，0=不限（默认32000）
	Memory        memory.Memory
	Store         store.Store
}

// Agent AI智能体
type Agent struct {
	config     *Config
	runtime    *Runtime
	sessionMgr *SessionManager
	memory     memory.Memory
}

// NewAgent 创建Agent
func NewAgent(cfg *Config) *Agent {
	return &Agent{
		config:     cfg,
		runtime:    NewRuntime(cfg),
		sessionMgr: NewSessionManager(cfg.Store),
		memory:     cfg.Memory,
	}
}

// Process 处理用户消息
func (a *Agent) Process(ctx context.Context, sessionID, userMessage string) (string, error) {
	return a.ProcessWithHandler(ctx, sessionID, userMessage, nil)
}

// ProcessWithHandler 处理用户消息（带工具事件回调）
func (a *Agent) ProcessWithHandler(ctx context.Context, sessionID, userMessage string, handler ToolEventHandler) (string, error) {
	logger := glog.Logger()
	logger.Info("[Agent] 开始处理消息",
		"agent", a.config.Name,
		"session", sessionID,
		"model", a.config.Model,
		"provider", a.config.ProviderType,
		"msg_len", len(userMessage))

	session := a.sessionMgr.GetOrCreate(sessionID)
	logger.Debug("[Agent] 会话已获取/创建", "session_id", sessionID, "msg_count", len(session.Messages))

	// 检索相关记忆
	var relevantMemories []string
	if a.memory != nil {
		results, err := a.memory.Retrieve(ctx, userMessage, sessionID, 5)
		if err != nil {
			logger.Warn("[Agent] 记忆检索失败", "err", err)
		} else if len(results) > 0 {
			logger.Debug("[Agent] 检索到相关记忆", "count", len(results))
			for _, res := range results {
				relevantMemories = append(relevantMemories,
					fmt.Sprintf("[%s] %s", res.Entry.Type, res.Entry.Content))
			}
		}
	}

	enhancedMessage := userMessage
	if len(relevantMemories) > 0 {
		memoryContext := "相关记忆:\n" + strings.Join(relevantMemories, "\n")
		enhancedMessage = memoryContext + "\n\n用户问题: " + userMessage
		logger.Debug("[Agent] 消息已增强，加入记忆上下文")
	}

	session.AddMessage("user", enhancedMessage)
	logger.Debug("[Agent] 用户消息已添加到会话", "history_len", len(session.Messages))

	// 执行运行时
	logger.Info("[Agent] 开始执行Runtime",
		"tools_count", len(a.config.Tools),
		"max_iterations", a.config.MaxIterations)

	finalResponse, err := a.runtime.Execute(ctx, session, a.config.Tools, a.config.MaxIterations, handler)
	if err != nil {
		logger.Error("[Agent] Runtime执行失败", "err", err)
		return "", err
	}

	logger.Info("[Agent] Runtime执行完成", "response_len", len(finalResponse))

	// 存储记忆
	if a.memory != nil {
		a.memory.Store(ctx, memory.MemoryEntry{
			Content:    userMessage,
			Type:       "short_term",
			SessionID:  sessionID,
			UserID:     session.User,
			Metadata:   map[string]interface{}{"role": "user"},
			Importance: 0.5,
			CreatedAt:  time.Now(),
		})
		a.memory.Store(ctx, memory.MemoryEntry{
			Content:    finalResponse,
			Type:       "short_term",
			SessionID:  sessionID,
			UserID:     session.User,
			Metadata:   map[string]interface{}{"role": "assistant"},
			Importance: 0.6,
			CreatedAt:  time.Now(),
		})
		logger.Debug("[Agent] 对话已存入记忆")
	}

	session.AddMessage("assistant", finalResponse)
	logger.Info("[Agent] 消息处理完成", "session", sessionID)

	return finalResponse, nil
}

// GetMemories 获取会话记忆
func (a *Agent) GetMemories(ctx context.Context, sessionID string, limit int) ([]memory.MemoryEntry, error) {
	if a.memory == nil {
		return nil, fmt.Errorf("记忆组件未启用")
	}
	return a.memory.GetRecent(ctx, sessionID, limit)
}

// ClearMemories 清除会话记忆
func (a *Agent) ClearMemories(ctx context.Context, sessionID string) error {
	if a.memory == nil {
		return fmt.Errorf("记忆组件未启用")
	}
	return a.memory.ClearSession(ctx, sessionID)
}

// CleanupExpiredSessions 清理过期会话
func (a *Agent) CleanupExpiredSessions(ttlMinutes int) {
	a.sessionMgr.CleanupExpired(ttlMinutes)
}

// ListSessions 列出所有会话
func (a *Agent) ListSessions() []SessionSummary {
	sessions := a.sessionMgr.ListSessions()
	var summaries []SessionSummary
	for i := range sessions {
		summaries = append(summaries, SessionSummary{
			ID:        sessions[i].ID,
			Channel:   sessions[i].Channel,
			User:      sessions[i].User,
			CreatedAt: sessions[i].CreatedAt,
			UpdatedAt: sessions[i].UpdatedAt,
		})
	}
	return summaries
}

// GetSessionMessages 获取会话消息历史
func (a *Agent) GetSessionMessages(sessionID string) ([]SessionMessage, bool) {
	s, exists := a.sessionMgr.GetSession(sessionID)
	if !exists {
		return nil, false
	}
	msgs := make([]SessionMessage, 0, len(s.Messages))
	s.mu.RLock()
	for _, m := range s.Messages {
		msgs = append(msgs, SessionMessage{
			Role:      m.Role,
			Content:   m.Content,
			Timestamp: m.Timestamp,
		})
	}
	s.mu.RUnlock()
	return msgs, true
}

// DeleteSession 删除会话
func (a *Agent) DeleteSession(id string) error {
	return a.sessionMgr.DeleteSession(id)
}

// SessionMessage 会话消息
type SessionMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// SessionSummary 会话摘要
type SessionSummary struct {
	ID        string
	Channel   string
	User      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
