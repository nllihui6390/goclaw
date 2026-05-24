package agent

import (
	"context"
	"fmt"
	"go-claw/internal/memory"
	"go-claw/internal/tool"
	"strings"
	"time"
)

// Config Agent配置（原有基础上添加记忆配置）
type Config struct {
	Name          string
	SystemPrompt  string
	Model         string
	APIKey        string
	BaseURL       string
	Tools         []tool.Tool
	MaxIterations int
	Memory        memory.Memory // 添加记忆组件
}

// Agent AI智能体（更新版）
type Agent struct {
	config     *Config
	runtime    *Runtime
	sessionMgr *SessionManager
	memory     memory.Memory
}

// NewAgent 创建Agent（更新版）
func NewAgent(cfg *Config) *Agent {
	return &Agent{
		config:     cfg,
		runtime:    NewRuntime(cfg),
		sessionMgr: NewSessionManager(),
		memory:     cfg.Memory,
	}
}

// Process 处理用户消息（带记忆版本）
func (a *Agent) Process(ctx context.Context, sessionID, userMessage string) (string, error) {
	// 获取或创建会话
	session := a.sessionMgr.GetOrCreate(sessionID)

	// 1. 检索相关记忆（短期 + 长期）
	var relevantMemories []string
	if a.memory != nil {
		results, err := a.memory.Retrieve(ctx, userMessage, sessionID, 5)
		if err == nil && len(results) > 0 {
			for _, res := range results {
				relevantMemories = append(relevantMemories,
					fmt.Sprintf("[%s] %s", res.Entry.Type, res.Entry.Content))
			}
		}
	}

	// 2. 构建增强的提示词
	enhancedMessage := userMessage
	if len(relevantMemories) > 0 {
		memoryContext := "相关记忆:\n" + strings.Join(relevantMemories, "\n")
		enhancedMessage = memoryContext + "\n\n用户问题: " + userMessage
	}

	// 3. 添加用户消息到历史
	session.AddMessage("user", enhancedMessage)

	// 4. 执行思考-行动循环
	finalResponse, err := a.runtime.Execute(ctx, session, a.config.Tools, a.config.MaxIterations)
	if err != nil {
		return "", err
	}

	// 5. 将交互存储为记忆
	if a.memory != nil {
		// 存储用户消息
		userMemory := memory.MemoryEntry{
			Content:    userMessage,
			Type:       "short_term",
			SessionID:  sessionID,
			UserID:     session.User,
			Metadata:   map[string]interface{}{"role": "user"},
			Importance: 0.5,
			CreatedAt:  time.Now(),
		}
		a.memory.Store(ctx, userMemory)

		// 存储助手响应
		assistantMemory := memory.MemoryEntry{
			Content:    finalResponse,
			Type:       "short_term",
			SessionID:  sessionID,
			UserID:     session.User,
			Metadata:   map[string]interface{}{"role": "assistant"},
			Importance: 0.6,
			CreatedAt:  time.Now(),
		}
		a.memory.Store(ctx, assistantMemory)
	}

	// 6. 添加助手响应到历史
	session.AddMessage("assistant", finalResponse)

	return finalResponse, nil
}

// GetMemories 获取会话记忆（新增方法）
func (a *Agent) GetMemories(ctx context.Context, sessionID string, limit int) ([]memory.MemoryEntry, error) {
	if a.memory == nil {
		return nil, fmt.Errorf("记忆组件未启用")
	}
	return a.memory.GetRecent(ctx, sessionID, limit)
}

// ClearMemories 清除会话记忆（新增方法）
func (a *Agent) ClearMemories(ctx context.Context, sessionID string) error {
	if a.memory == nil {
		return fmt.Errorf("记忆组件未启用")
	}
	return a.memory.ClearSession(ctx, sessionID)
}
