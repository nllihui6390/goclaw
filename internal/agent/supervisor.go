package agent

import (
	"context"
	"fmt"
	"strings"

	"go-claw/internal/memory"
	"go-claw/internal/store"
)

// SupervisorConfig 监督者配置
type SupervisorConfig struct {
	Name         string
	SystemPrompt string
	Model        string
	APIKey       string
	BaseURL      string
	MaxIterations int
	Memory       memory.Memory
	Store        store.Store
	SubAgents    map[string]*Agent // 子Agent
}

// SupervisorAgent 监督者Agent（接收用户消息 → 分发给子 agent → 汇总返回）
type SupervisorAgent struct {
	config     *SupervisorConfig
	runtime    *Runtime
	sessionMgr *SessionManager
	memory     memory.Memory
	subAgents  map[string]*Agent
}

// NewSupervisorAgent 创建监督者
func NewSupervisorAgent(cfg *SupervisorConfig) *SupervisorAgent {
	s := &SupervisorAgent{
		config:     cfg,
		runtime:    NewRuntime(&Config{
			Model:   cfg.Model,
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
		}),
		sessionMgr: NewSessionManager(cfg.Store),
		memory:     cfg.Memory,
		subAgents:  cfg.SubAgents,
	}
	return s
}

// Process 处理用户消息（监督模式：判断意图 → 分发 → 汇总）
func (s *SupervisorAgent) Process(ctx context.Context, sessionID, userMessage string) (string, error) {
	session := s.sessionMgr.GetOrCreate(sessionID)

	// 1. 判断应该由哪个子Agent处理
	targetAgent, err := s.routeToAgent(ctx, userMessage)
	if err != nil {
		return "", err
	}

	if targetAgent == nil {
		// 没有匹配的子Agent，监督者自己处理
		return s.processSelf(ctx, session, userMessage)
	}

	// 2. 分发到子Agent
	response, err := targetAgent.Process(ctx, sessionID, userMessage)
	if err != nil {
		return "", err
	}

	// 3. 记录交互
	session.AddMessage("user", userMessage)
	session.AddMessage("assistant", fmt.Sprintf("[由 %s 处理] %s", targetAgent.config.Name, response))

	return response, nil
}

func (s *SupervisorAgent) routeToAgent(ctx context.Context, userMessage string) (*Agent, error) {
	// 构建路由提示词
	var sb strings.Builder
	sb.WriteString("你是一个任务分类器。根据用户的消息，判断应该由哪个子Agent处理。\n")
	sb.WriteString("可用的子Agent有：\n")
	for name, ag := range s.subAgents {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", name, ag.config.SystemPrompt))
	}
	sb.WriteString("\n请只返回Agent名称，不要返回其他内容。\n")
	sb.WriteString("用户消息: " + userMessage)

	// 调用LLM判断
	msgs := []ChatMessage{
		{Role: "system", Content: sb.String()},
	}

	resp, err := s.runtime.callLLM(ctx, msgs, nil)
	if err != nil {
		return nil, err
	}

	// 匹配Agent名称
	for name := range s.subAgents {
		if strings.Contains(strings.ToLower(resp.Content), strings.ToLower(name)) {
			return s.subAgents[name], nil
		}
	}

	return nil, nil
}

func (s *SupervisorAgent) processSelf(ctx context.Context, session *Session, userMessage string) (string, error) {
	messages := []ChatMessage{
		{Role: "system", Content: s.config.SystemPrompt},
	}

	for _, msg := range session.Messages {
		messages = append(messages, ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	resp, err := s.runtime.callLLM(ctx, messages, nil)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// ListSubAgents 列出子Agent
func (s *SupervisorAgent) ListSubAgents() []string {
	var names []string
	for name := range s.subAgents {
		names = append(names, name)
	}
	return names
}
