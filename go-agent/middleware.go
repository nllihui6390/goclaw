package agent

import (
	"context"
	"fmt"
)

// =============================================
// 中间件系统（ 的 Middleware）
//
// AgentScope 在关键生命周期阶段提供中间件钩子：
// - on_reply      → reply 开始前
// - on_reasoning  → 推理前
// - on_acting     → 行动前
// - on_model_call → 模型调用前/后
// - on_system_prompt → 组装 system prompt 时
//
//  Context Manager 钩子系统：
// - pre_reply     → 最终回复前（注入记忆、检索上下文）
// - post_reply    → 最终回复后（后台摘要任务）
// - pre_reasoning → 每次推理前（检查上下文、压缩）
// - post_acting   → 每次工具执行后（截断工具结果）
// =============================================

// MiddlewarePhase 中间件阶段
type MiddlewarePhase string

const (
	PhaseReply        MiddlewarePhase = "reply"         // reply 开始前
	PhasePreReply     MiddlewarePhase = "pre_reply"     // 最终回复前（）
	PhasePostReply    MiddlewarePhase = "post_reply"    // 最终回复后（）
	PhaseReasoning    MiddlewarePhase = "reasoning"     // 推理前
	PhasePreReasoning MiddlewarePhase = "pre_reasoning" // 每次模型调用前（）
	PhaseActing       MiddlewarePhase = "acting"        // 行动前
	PhasePostActing   MiddlewarePhase = "post_acting"   // 工具执行后（）
	PhaseModelCall    MiddlewarePhase = "model_call"    // 模型调用前/后
	PhaseSystemPrompt MiddlewarePhase = "system_prompt" // 组装 system prompt
)

// Middleware 中间件接口
type Middleware interface {
	// Name 中间件名称
	Name() string

	// Phase 中间件阶段
	Phase() MiddlewarePhase

	// Execute 执行中间件逻辑
	// 返回 error 时中断后续中间件和主流程
	Execute(ctx context.Context, agent *Agent, data interface{}) error
}

// =============================================
// 中间件链（链式调用）
// =============================================

// MiddlewareChain 中间件链
type MiddlewareChain struct {
	middlewares map[MiddlewarePhase][]Middleware
}

// NewMiddlewareChain 创建中间件链
func NewMiddlewareChain() *MiddlewareChain {
	return &MiddlewareChain{
		middlewares: make(map[MiddlewarePhase][]Middleware),
	}
}

// Add 添加中间件
func (c *MiddlewareChain) Add(middleware Middleware) {
	phase := middleware.Phase()
	c.middlewares[phase] = append(c.middlewares[phase], middleware)
}

// Execute 执行指定阶段的中间件链
func (c *MiddlewareChain) Execute(ctx context.Context, phase MiddlewarePhase, agent *Agent, data interface{}) error {
	middlewares := c.middlewares[phase]
	for _, m := range middlewares {
		if err := m.Execute(ctx, agent, data); err != nil {
			return err
		}
	}
	return nil
}

// GetByPhase 获取指定阶段的中间件列表
func (c *MiddlewareChain) GetByPhase(phase MiddlewarePhase) []Middleware {
	return c.middlewares[phase]
}

// =============================================
// 内置中间件
// =============================================

// SystemPromptMiddleware system prompt 组装中间件
// 示例：注入动态上下文（工作目录、时间、环境变量）
type SystemPromptMiddleware struct {
	name       string
	promptFunc func(ctx context.Context, agent *Agent, prompt *string) error
}

// NewSystemPromptMiddleware 创建 system prompt 中间件
func NewSystemPromptMiddleware(name string, promptFunc func(ctx context.Context, agent *Agent, prompt *string) error) *SystemPromptMiddleware {
	return &SystemPromptMiddleware{
		name:       name,
		promptFunc: promptFunc,
	}
}

func (m *SystemPromptMiddleware) Name() string           { return m.name }
func (m *SystemPromptMiddleware) Phase() MiddlewarePhase { return PhaseSystemPrompt }
func (m *SystemPromptMiddleware) Execute(ctx context.Context, agent *Agent, data interface{}) error {
	// data 是 *string（指向 system prompt）
	if promptPtr, ok := data.(*string); ok {
		return m.promptFunc(ctx, agent, promptPtr)
	}
	return nil
}

// LoggingMiddleware 日志中间件
// 示例：记录每次 reply 的输入和输出
type LoggingMiddleware struct {
	name   string
	logger func(phase MiddlewarePhase, agentName string, data interface{})
}

// NewLoggingMiddleware 创建日志中间件
func NewLoggingMiddleware(name string, logger func(phase MiddlewarePhase, agentName string, data interface{})) *LoggingMiddleware {
	return &LoggingMiddleware{
		name:   name,
		logger: logger,
	}
}

func (m *LoggingMiddleware) Name() string           { return m.name }
func (m *LoggingMiddleware) Phase() MiddlewarePhase { return PhaseReply }
func (m *LoggingMiddleware) Execute(ctx context.Context, agent *Agent, data interface{}) error {
	if m.logger != nil {
		m.logger(PhaseReply, agent.GetName(), data)
	}
	return nil
}

// RateLimitMiddleware 限流中间件
// 示例：限制每分钟最大调用次数
type RateLimitMiddleware struct {
	name      string
	maxPerMin int
	counter   map[string]int // 按 agent 名计数
	lastReset int64          // 上次重置时间
}

// NewRateLimitMiddleware 创建限流中间件
func NewRateLimitMiddleware(name string, maxPerMin int) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		name:      name,
		maxPerMin: maxPerMin,
		counter:   make(map[string]int),
	}
}

func (m *RateLimitMiddleware) Name() string           { return m.name }
func (m *RateLimitMiddleware) Phase() MiddlewarePhase { return PhaseReply }
func (m *RateLimitMiddleware) Execute(ctx context.Context, agent *Agent, data interface{}) error {
	// 检查当前分钟是否超过限制
	// 简化实现：使用时间戳作为分钟标识
	now := nowISO()
	minuteKey := now[:16] // "2024-01-01T12:3" → 分钟级别

	key := agent.GetName() + ":" + minuteKey
	m.counter[key]++

	if m.counter[key] > m.maxPerMin {
		return fmt.Errorf("rate limit exceeded for agent %s: %d calls per minute (max: %d)",
			agent.GetName(), m.counter[key], m.maxPerMin)
	}

	return nil
}

// ErrorHandlingMiddleware 错误处理中间件
// 示例：捕获模型调用错误，记录日志
type ErrorHandlingMiddleware struct {
	name    string
	onError func(ctx context.Context, agent *Agent, err error) error
}

// NewErrorHandlingMiddleware 创建错误处理中间件
func NewErrorHandlingMiddleware(name string, onError func(ctx context.Context, agent *Agent, err error) error) *ErrorHandlingMiddleware {
	return &ErrorHandlingMiddleware{
		name:    name,
		onError: onError,
	}
}

func (m *ErrorHandlingMiddleware) Name() string           { return m.name }
func (m *ErrorHandlingMiddleware) Phase() MiddlewarePhase { return PhaseModelCall }
func (m *ErrorHandlingMiddleware) Execute(ctx context.Context, agent *Agent, data interface{}) error {
	// data 可能是错误
	if err, ok := data.(error); ok && m.onError != nil {
		return m.onError(ctx, agent, err)
	}
	return nil
}

// =============================================
// 新增中间件（ Context Manager 钩子）
// =============================================

// PreReplyMiddleware 回复前中间件。
//
//	pre_reply 钩子：
//
// 在 Agent 发出最终回复前执行，用于注入记忆检索结果、
// 动态上下文等。
type PreReplyMiddleware struct {
	name     string
	preReply func(ctx context.Context, agent *Agent, data interface{}) error
}

func NewPreReplyMiddleware(name string, preReply func(ctx context.Context, agent *Agent, data interface{}) error) *PreReplyMiddleware {
	return &PreReplyMiddleware{name: name, preReply: preReply}
}

func (m *PreReplyMiddleware) Name() string           { return m.name }
func (m *PreReplyMiddleware) Phase() MiddlewarePhase { return PhasePreReply }
func (m *PreReplyMiddleware) Execute(ctx context.Context, agent *Agent, data interface{}) error {
	if m.preReply != nil {
		return m.preReply(ctx, agent, data)
	}
	return nil
}

// PostReplyMiddleware 回复后中间件。
//
//	post_reply 钩子：
//
// 在 Agent 发出最终回复后执行，用于后台记忆摘要、
// 状态持久化等。
type PostReplyMiddleware struct {
	name      string
	postReply func(ctx context.Context, agent *Agent, data interface{}) error
}

func NewPostReplyMiddleware(name string, postReply func(ctx context.Context, agent *Agent, data interface{}) error) *PostReplyMiddleware {
	return &PostReplyMiddleware{name: name, postReply: postReply}
}

func (m *PostReplyMiddleware) Name() string           { return m.name }
func (m *PostReplyMiddleware) Phase() MiddlewarePhase { return PhasePostReply }
func (m *PostReplyMiddleware) Execute(ctx context.Context, agent *Agent, data interface{}) error {
	if m.postReply != nil {
		return m.postReply(ctx, agent, data)
	}
	return nil
}

// PreReasoningMiddleware 推理前中间件。
//
//	pre_reasoning 钩子：
//
// 在每次模型调用前执行，用于检查上下文大小、
// 触发自动压缩、截断过长消息等。
type PreReasoningMiddleware struct {
	name         string
	preReasoning func(ctx context.Context, agent *Agent, data interface{}) error
}

func NewPreReasoningMiddleware(name string, preReasoning func(ctx context.Context, agent *Agent, data interface{}) error) *PreReasoningMiddleware {
	return &PreReasoningMiddleware{name: name, preReasoning: preReasoning}
}

func (m *PreReasoningMiddleware) Name() string           { return m.name }
func (m *PreReasoningMiddleware) Phase() MiddlewarePhase { return PhasePreReasoning }
func (m *PreReasoningMiddleware) Execute(ctx context.Context, agent *Agent, data interface{}) error {
	if m.preReasoning != nil {
		return m.preReasoning(ctx, agent, data)
	}
	return nil
}

// PostActingMiddleware 工具执行后中间件。
//
//	post_acting 钩子：
//
// 在每次工具执行完成后执行，用于截断过大的工具结果、
// 记录工具调用统计、安全审计等。
type PostActingMiddleware struct {
	name       string
	postActing func(ctx context.Context, agent *Agent, data interface{}) error
}

func NewPostActingMiddleware(name string, postActing func(ctx context.Context, agent *Agent, data interface{}) error) *PostActingMiddleware {
	return &PostActingMiddleware{name: name, postActing: postActing}
}

func (m *PostActingMiddleware) Name() string           { return m.name }
func (m *PostActingMiddleware) Phase() MiddlewarePhase { return PhasePostActing }
func (m *PostActingMiddleware) Execute(ctx context.Context, agent *Agent, data interface{}) error {
	if m.postActing != nil {
		return m.postActing(ctx, agent, data)
	}
	return nil
}
