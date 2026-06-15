// Package agent 提供独立的 AI Agent 核心实现。
//
// Agent 是一个无状态的推理-行动循环引擎，将模型、工具、权限系统、
// 人机交互、上下文管理、中间件、状态管理和事件系统整合到统一接口中。
//
// 核心 API：
//   - Reply：同步推理-行动循环，返回完整 assistant Msg
//   - ReplyStream：流式版本，逐一产出 Event（start → delta → end）
//   - Observe：将消息注入上下文而不触发推理
//   - CompressContext：上下文压缩（手动或自动）
//
// 使用示例：
//
//	ag := agent.NewAgent(*agent.DefaultConfig("assistant", llm, "You are helpful.").
//	    WithTools(tools).WithMaxIters(10))
//
//	// 同步调用
//	reply, err := ag.Reply(ctx, agent.UserMsg("user", "Hello"))
//
//	// 流式调用
//	for event := range ag.ReplyStream(ctx, agent.UserMsg("user", "Hello")) {
//	    switch e := event.(type) {
//	    case agent.TextBlockDeltaEvent:
//	        fmt.Print(e.Delta)
//	    }
//	}
package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/nllihui6390/go-agent/memory"
	"github.com/nllihui6390/go-agent/tool"
)

// =============================================
// 回调类型
// =============================================

// ToolCallHandler 工具调用回调函数类型。
// 每次工具执行完成时被调用（无论成功或失败）。
//
// 参数：
//   - toolName: 被调用的工具名称
//   - params: 工具调用时传入的参数
//   - result: 工具执行返回的结果字符串
type ToolCallHandler func(toolName string, params map[string]interface{}, result string)

// StreamHandler 流式输出回调函数类型。
// 每次收到模型流式文本增量时被调用。
//
// 参数：
//   - chunk: 增量文本内容（单个 token 或片段）
type StreamHandler func(chunk string)

// =============================================
// Agent 结构体
// =============================================

// Agent 是独立的 AI Agent 实现。
//
// 它将 LLM 推理、工具调用、权限检查、上下文压缩、中间件和事件流
// 整合到一个统一的接口中。Agent 不依赖任何 gateway/channel/webhook 层，
// 可直接被任何 Go 项目导入使用。
//
// Agent 是并发安全的，内部使用互斥锁保护状态。
//
// 核心方法：
//   - Reply：同步推理循环
//   - ReplyStream：流式推理循环（事件驱动）
//   - Observe：静默添加消息
//   - CompressContext：手动触发上下文压缩
type Agent struct {
	config          Config           // Agent 配置（所有组件通过 Config 注入）
	session         *Session         // 当前会话（对话历史 + 持久化）
	contextMgr      *ContextManager  // 上下文管理器（压缩 + 截断 + 卸载）
	middlewareChain *MiddlewareChain // 中间件链（5 阶段钩子）
	mu              sync.Mutex       // 互斥锁，保证并发安全
}

// =============================================
// 构造函数
// =============================================

// NewAgent 创建新的 Agent 实例。
//
// 自动填充未设置的默认值：
//   - Tools：空注册表
//   - Memory：SimpleMemory（内存存储，1000 条上限）
//   - SessionStore：InMemorySessionStore
//   - ContextConfig：DefaultContextConfig()（trigger_ratio=0.8, reserve_ratio=0.1）
//   - Offloader：NoOpOffloader（不持久化，直接丢弃）
//   - ModelConfig：重试 3 次，延迟 1 秒
//   - ReActConfig：最大 10 次迭代
//   - Permission：DefaultPermissionChecker
//   - Middlewares：空列表
//
// 参数：
//   - cfg: Agent 配置（使用 DefaultConfig + Builder 模式构建）
//
// 返回：
//   - *Agent: 初始化完成的 Agent 实例
//
// 示例：
//
//	ag := agent.NewAgent(*agent.DefaultConfig("assistant", llm, "system prompt").
//	    WithTools(tools).WithMemory(mem).WithMaxIters(10))
func NewAgent(cfg Config) *Agent {
	if cfg.Tools == nil {
		cfg.Tools = tool.NewRegistry()
	}
	if cfg.Memory == nil {
		cfg.Memory = memory.NewSimpleMemory()
	}
	if cfg.SessionStore == nil {
		cfg.SessionStore = NewInMemorySessionStore()
	}
	if cfg.ContextConfig == nil {
		cfg.ContextConfig = DefaultContextConfig()
	}
	if cfg.Offloader == nil {
		cfg.Offloader = NewNoOpOffloader()
	}
	if cfg.ModelConfig == nil {
		cfg.ModelConfig = &ModelConfig{MaxRetries: 3, RetryDelay: 1000}
	}
	if cfg.ReActConfig == nil {
		cfg.ReActConfig = &ReActConfig{MaxIters: 10, HandleReject: "retry"}
	}
	if cfg.Permission == nil {
		cfg.Permission = NewDefaultPermissionChecker()
	}
	if cfg.Middlewares == nil {
		cfg.Middlewares = make([]Middleware, 0)
	}

	middlewareChain := NewMiddlewareChain()
	for _, m := range cfg.Middlewares {
		middlewareChain.Add(m)
	}

	maxTokens := 4096 // TODO: 从 cfg.Model 获取实际的最大上下文长度

	contextMgr := NewContextManager(cfg.ContextConfig, maxTokens, cfg.Offloader)

	return &Agent{
		config:          cfg,
		session:         NewSession(cfg.SessionStore),
		contextMgr:      contextMgr,
		middlewareChain: middlewareChain,
	}
}

// =============================================
// Reply — 同步推理-行动循环
// =============================================

// Reply 同步处理输入，运行完整的推理-行动循环，返回最终 assistant Msg。
//
// inputs 支持多种类型：
//   - Msg：开始新的 reply，处理用户消息
//   - []Msg：批量添加消息到上下文后开始 reply
//   - UserConfirmResultEvent：从暂停状态恢复，处理用户确认结果
//   - ExternalExecutionResultEvent：从暂停状态恢复，处理外部执行结果
//
// 参数：
//   - ctx: 上下文（用于取消和超时控制）
//   - inputs: 输入消息或事件
//
// 返回：
//   - *Msg: 最终的 assistant 回复消息（包含完整的内容块）
//   - error: 处理过程中的错误
//
// 示例：
//
//	reply, err := ag.Reply(ctx, agent.UserMsg("user", "Hello!"))
//	fmt.Println(reply.GetTextContent())
func (a *Agent) Reply(ctx context.Context, inputs interface{}) (*Msg, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.middlewareChain.Execute(ctx, PhaseReply, a, inputs); err != nil {
		return nil, err
	}

	switch input := inputs.(type) {
	case Msg:
		return a.replyFromMessage(ctx, input)
	case []Msg:
		for _, msg := range input {
			a.session.AddMessage(msg)
		}
		if len(input) > 0 {
			return a.replyFromMessage(ctx, input[len(input)-1])
		}
		return nil, fmt.Errorf("empty message list")
	case UserConfirmResultEvent:
		return a.replyFromConfirm(ctx, input)
	case ExternalExecutionResultEvent:
		return a.replyFromExternal(ctx, input)
	default:
		return nil, fmt.Errorf("unsupported input type: %T", inputs)
	}
}

// replyFromMessage 从用户消息开始新的 reply。
//
// 执行流程：
//  1. 存储用户消息到会话
//  2. 自动检查并压缩上下文（如果超过阈值）
//  3. 构建三层模型输入（System Prompt + Summary + Context）
//  4. 进入推理-执行循环：
//     a. 调用模型（带重试和备用模型）
//     b. 检查是否有工具调用
//     c. 无工具调用 → 返回最终回复
//     d. 有工具调用 → 权限检查 → 执行 → 结果注入 → 继续循环
//
// 参数：
//   - ctx: 上下文
//   - msg: 用户消息
//
// 返回：
//   - *Msg: 回复消息
//   - error: 错误信息

// =============================================
// 公开辅助方法
// =============================================

// GetSession 获取当前会话。
//
// 返回：
//   - *Session: 当前 Agent 的会话（含历史消息和存储）
func (a *Agent) GetSession() *Session {
	return a.session
}

// ClearSession 清除当前会话和上下文摘要。
func (a *Agent) ClearSession() {
	a.session.Clear()
	a.contextMgr.SetSummary(nil)
}

// GetName 获取 Agent 名称。
//
// 返回：
//   - string: Agent 名称（来自 Config.Name）
func (a *Agent) GetName() string {
	return a.config.Name
}

// GetConfig 获取 Agent 配置的副本。
//
// 返回：
//   - *Config: 配置指针
func (a *Agent) GetConfig() *Config {
	return &a.config
}

// GetContextManager 获取上下文管理器。
//
// 返回：
//   - *ContextManager: 上下文管理器（含摘要、压缩配置、TokenCounter）
func (a *Agent) GetContextManager() *ContextManager {
	return a.contextMgr
}

// =============================================
// 向后兼容
// =============================================

// Process 处理消息（向后兼容，等同于 Reply）。
//
// 参数：
//   - ctx: 上下文
//   - msg: 用户消息
//
// 返回：
//   - *Msg: assistant 回复
//   - error: 处理错误
func (a *Agent) Process(ctx context.Context, msg Msg) (*Msg, error) {
	return a.Reply(ctx, msg)
}

// Stream 流式处理消息（向后兼容，等同于 ReplyStream）。
//
// 参数：
//   - ctx: 上下文
//   - msg: 用户消息
//
// 返回：
//   - <-chan interface{}: 事件流
//   - error: 启动错误
func (a *Agent) Stream(ctx context.Context, msg Msg) (<-chan interface{}, error) {
	return a.ReplyStream(ctx, msg)
}
