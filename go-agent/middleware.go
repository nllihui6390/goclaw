package agent

import (
	"context"
	"fmt"
	"time"
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
	// Agent 生命周期钩子
	PhaseAgentCreated   MiddlewarePhase = "agent_created"   // Agent 创建后
	PhaseAgentDestroyed MiddlewarePhase = "agent_destroyed" // Agent 销毁前
	PhaseSessionStarted MiddlewarePhase = "session_started" // 会话启动时
	PhaseSessionEnded   MiddlewarePhase = "session_ended"   // 会话结束时
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
// 函数式中间件（通用工厂）
// =============================================

// FunctionalMiddleware 函数式中间件
// 通过 name、phase、executeFunc 直接创建中间件，
// 替代 PreReplyMiddleware、PostReplyMiddleware 等重复的包装类
type FunctionalMiddleware struct {
	name        string
	phase       MiddlewarePhase
	executeFunc func(ctx context.Context, agent *Agent, data interface{}) error
}

// NewFunctionalMiddleware 创建函数式中间件
func NewFunctionalMiddleware(name string, phase MiddlewarePhase, executeFunc func(ctx context.Context, agent *Agent, data interface{}) error) *FunctionalMiddleware {
	return &FunctionalMiddleware{
		name:        name,
		phase:       phase,
		executeFunc: executeFunc,
	}
}

func (m *FunctionalMiddleware) Name() string           { return m.name }
func (m *FunctionalMiddleware) Phase() MiddlewarePhase { return m.phase }
func (m *FunctionalMiddleware) Execute(ctx context.Context, agent *Agent, data interface{}) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, agent, data)
	}
	return nil
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
// 便捷中间件创建函数（基于 FunctionalMiddleware）
// =============================================

// NewPreReplyMiddleware 回复前中间件。
//
//	pre_reply 钩子：
//
// 在 Agent 发出最终回复前执行，用于注入记忆检索结果、
// 动态上下文等。
func NewPreReplyMiddleware(name string, preReply func(ctx context.Context, agent *Agent, data interface{}) error) Middleware {
	return NewFunctionalMiddleware(name, PhasePreReply, preReply)
}

// NewPostReplyMiddleware 回复后中间件。
//
//	post_reply 钩子：
//
// 在 Agent 发出最终回复后执行，用于后台记忆摘要、
// 状态持久化等。
func NewPostReplyMiddleware(name string, postReply func(ctx context.Context, agent *Agent, data interface{}) error) Middleware {
	return NewFunctionalMiddleware(name, PhasePostReply, postReply)
}

// NewPreReasoningMiddleware 推理前中间件。
//
//	pre_reasoning 钩子：
//
// 在每次模型调用前执行，用于检查上下文大小、
// 触发自动压缩、截断过长消息等。
func NewPreReasoningMiddleware(name string, preReasoning func(ctx context.Context, agent *Agent, data interface{}) error) Middleware {
	return NewFunctionalMiddleware(name, PhasePreReasoning, preReasoning)
}

// NewPostActingMiddleware 工具执行后中间件。
//
//	post_acting 钩子：
//
// 在每次工具执行完成后执行，用于截断过大的工具结果、
// 记录工具调用统计、安全审计等。
func NewPostActingMiddleware(name string, postActing func(ctx context.Context, agent *Agent, data interface{}) error) Middleware {
	return NewFunctionalMiddleware(name, PhasePostActing, postActing)
}

// NewAgentCreatedMiddleware Agent 创建后中间件。
//
// 在 Agent 构造完成、首次可用之前触发，
// 可用于初始化资源、注册钩子、预热缓存等。
func NewAgentCreatedMiddleware(name string, onCreate func(ctx context.Context, agent *Agent) error) Middleware {
	return NewFunctionalMiddleware(name, PhaseAgentCreated, func(ctx context.Context, agent *Agent, data interface{}) error {
		return onCreate(ctx, agent)
	})
}

// NewAgentDestroyedMiddleware Agent 销毁前中间件。
//
// 在 Agent 被回收之前触发，
// 可用于释放资源、保存状态、断开连接等。
func NewAgentDestroyedMiddleware(name string, onDestroy func(ctx context.Context, agent *Agent) error) Middleware {
	return NewFunctionalMiddleware(name, PhaseAgentDestroyed, func(ctx context.Context, agent *Agent, data interface{}) error {
		return onDestroy(ctx, agent)
	})
}

// NewSessionStartedMiddleware 会话启动中间件。
//
// 在每次 Reply/ReplyStream 开始时触发，
// 可用于加载会话上下文、预热资源、记录指标等。
func NewSessionStartedMiddleware(name string, onStart func(ctx context.Context, agent *Agent, sessionID string) error) Middleware {
	return NewFunctionalMiddleware(name, PhaseSessionStarted, func(ctx context.Context, agent *Agent, data interface{}) error {
		if sessionID, ok := data.(string); ok {
			return onStart(ctx, agent, sessionID)
		}
		return nil
	})
}

// NewSessionEndedMiddleware 会话结束中间件。
//
// 在每次 Reply/ReplyStream 结束后触发，
// 可用于持久化会话、清理临时资源、触发摘要等。
func NewSessionEndedMiddleware(name string, onEnd func(ctx context.Context, agent *Agent, sessionID string, duration time.Duration) error) Middleware {
	return NewFunctionalMiddleware(name, PhaseSessionEnded, func(ctx context.Context, agent *Agent, data interface{}) error {
		if d, ok := data.(struct {
			SessionID string
			Duration  time.Duration
		}); ok {
			return onEnd(ctx, agent, d.SessionID, d.Duration)
		}
		return nil
	})
}
