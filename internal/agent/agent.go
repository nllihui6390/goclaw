package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-claw/internal/channel"
	"go-claw/internal/inbox"
	"go-claw/internal/memory"
	"go-claw/internal/security"
	"go-claw/internal/skill"
	"go-claw/internal/store"
	"go-claw/internal/tool"
	glog "go-claw/pkg/log"
)

// WorkspaceLoader 工作空间加载器接口
type WorkspaceLoader interface {
	LoadSystemPrompt() string
	IsBootstrapNeeded() bool                // 检查是否需要首次引导
	MarkBootstrapCompleted() error          // 标记引导完成
	GetBootstrapGuidance() string           // 获取引导提示词
	LoadDailyMemory() string                // 加载今日记忆
	AppendDailyMemory(content string) error // 追加到今日记忆
}

// Config Agent配置
type AgentConfig struct {
	Name                  string
	SystemPrompt          string
	Model                 string
	APIKey                string
	BaseURL               string
	ProviderType          string // 供应商类型: openai, ollama, anthropic, azure
	Tools                 []tool.Tool
	MaxIterations         int
	MaxTokens             int // 最大上下文 Token 数，0=不限（默认32000）
	Memory                memory.Memory
	Store                 store.Store
	WorkspaceLoader       WorkspaceLoader     // 工作空间人设文件加载器
	WorkspaceDir          string              // 工作空间目录路径（用于缓存文件）
	SkillRegistry         *skill.Registry     // 技能注册中心（用于系统提示词注入）
	MaxContextMessages    int     // 未压缩消息数触发阈值，0=默认20
	CompactThresholdRatio float64 // 压缩触发比例，0=不压缩（默认0.8）
	ReserveThresholdRatio float64 // 压缩后保留比例（默认0.15）
	ToolResultMaxBytes    int     // 工具结果最大字节数，0=不限（默认20000）
	ToolResultExemptTools []string            // 裁剪豁免工具名列表
	ToolResultExemptExts  []string            // 裁剪豁免文件扩展名列表
	SupportsImage         bool                // 模型是否支持图片输入
	SupportsVideo         bool                // 模型是否支持视频输入
	ToolGuard             *security.ToolGuard // 工具安全守卫
	InboxStore            *inbox.Store        // Inbox 事件通知存储
	// ConfigProvider 动态配置提供器：每次调用 LLM 时获取最新 model/apiKey/baseURL/providerType
	// 优先使用此函数，降级使用 Model/APIKey/BaseURL/ProviderType 字段
	ConfigProvider func() (model, apiKey, baseURL, providerType string)
}

// Agent AI智能体
type Agent struct {
	config     *AgentConfig
	runtime    *Runtime
	sessionMgr *SessionManager
	memory     memory.Memory
}

// NewAgent 创建Agent
func NewAgent(cfg *AgentConfig) *Agent {
	runtime := NewRuntime(cfg)
	if cfg.WorkspaceDir != "" {
		runtime.SetWorkspaceDir(cfg.WorkspaceDir)
	}
	return &Agent{
		config:     cfg,
		runtime:    runtime,
		sessionMgr: NewSessionManager(cfg.Store),
		memory:     cfg.Memory,
	}
}

// Process 处理用户消息
func (a *Agent) Process(ctx context.Context, sessionID, userMessage string) (string, error) {
	return a.ProcessWithHandler(ctx, sessionID, userMessage, nil)
}

// Name 返回 Agent 名称
func (a *Agent) Name() string {
	return a.config.Name
}

// GetInfo 获取 Agent 信息描述
func (a *Agent) GetInfo() string {
	return fmt.Sprintf("Agent %s (model: %s, provider: %s)", a.config.Name, a.config.Model, a.config.ProviderType)
}

// SetSkillRegistry 设置技能注册中心（用于热重载）
func (a *Agent) SetSkillRegistry(reg *skill.Registry) {
	a.config.SkillRegistry = reg
	a.runtime.SetSkillRegistry(reg)
}

// ProcessWithHandler 处理用户消息（带工具事件回调）
func (a *Agent) ProcessWithHandler(ctx context.Context, sessionID, userMessage string, handler ToolEventHandler) (string, error) {
	return a.ProcessWithBlocks(ctx, sessionID, userMessage, nil, handler)
}

// ProcessWithBlocks 处理用户消息，支持传入结构化内容块（多模态：图片/文件等）
func (a *Agent) ProcessWithBlocks(ctx context.Context, sessionID, userMessage string, blocks channel.ContentBlocks, handler ToolEventHandler) (string, error) {
	logger := glog.Logger()
	logger.Info("[Agent] 开始处理消息",
		"agent", a.config.Name,
		"session", sessionID,
		"model", a.config.Model,
		"provider", a.config.ProviderType,
		"msg_len", len(userMessage))

	session := a.sessionMgr.GetOrCreate(sessionID)
	// 从 context 获取真实 channel/user 覆盖 session（GetOrCreate 对 UUID 会话猜不准）
	if ch := GetChannelFromCtx(ctx); ch != "" {
		session.SetChannel(ch)
	}
	if user := GetUserFromCtx(ctx); user != "" {
		session.SetUser(user)
	}
	// 同步更新 SessionID = channel:user
	session.SetSessionID(session.Channel + ":" + session.UserID)
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

	// 构建发送给 LLM 的消息（原始消息 + 记忆上下文）
	llmMessage := userMessage
	if len(relevantMemories) > 0 {
		memoryContext := "相关记忆:\n" + strings.Join(relevantMemories, "\n")
		llmMessage = memoryContext + "\n\n用户问题: " + userMessage
		logger.Debug("[Agent] 消息已增强，加入记忆上下文", "memory_count", len(relevantMemories))
	}

	// 会话历史只保存原始用户消息，不保存增强后的记忆上下文
	if len(blocks) > 0 {
		// 有结构化内容块时，将其与文本消息合并保存
		userBlocks := channel.ParseFileMarkers(userMessage)
		// 将传入的 blocks（如图片）放到前面
		userBlocks = append(blocks, userBlocks...)
		session.AddMessage("user", userBlocks)
	} else {
		session.AddMessage("user", channel.ParseFileMarkers(userMessage))
	}
	logger.Debug("[Agent] 用户消息已添加到会话", "history_len", len(session.Messages), "blocks", len(blocks))

	// 执行运行时（传入增强后的消息供 LLM 使用，但不在会话历史中污染）
	// 收集内容块（用于追加到最终响应，确保 session 持久化）
	contentBlocks := channel.ContentBlocks{}
	// 收集工具执行过程（用于 Metadata 持久化，供前端折叠显示）
	var thinkingParts []string
	type toolExecRecord struct {
		Name          string `json:"name"`
		Args          string `json:"args"`
		Result        string `json:"result,omitempty"`
		Error         string `json:"error,omitempty"`
		Status        string `json:"status"` // "success", "error", "guard"
		ApprovalID    string `json:"approval_id,omitempty"`
		ApprovalState string `json:"approval_state,omitempty"` // pending/approved/denied
		GuardMessage  string `json:"guard_message,omitempty"`
	}
	var toolExecRecords []toolExecRecord
	var currentToolArgs string
	// 用于 guard 事件去重：相同 approval_id 只保留最后一条
	guardEventIndex := make(map[string]int) // approval_id -> index in toolExecRecords

	wrappedHandler := func(event ToolEvent) {
		// 收集内容块事件
		if event.Type == "content" && len(event.Content) > 0 {
			contentBlocks = append(contentBlocks, event.Content...)
		}
		// 收集思考内容
		if event.Type == "thinking" && event.Thinking != "" {
			thinkingParts = append(thinkingParts, event.Thinking)
		}
		// 收集工具调用开始
		if event.Type == "calling" {
			currentToolArgs = event.Args
		}
		// 收集工具执行结果
		if event.Type == "result" {
			toolExecRecords = append(toolExecRecords, toolExecRecord{
				Name:   event.ToolName,
				Args:   currentToolArgs,
				Result: event.Result,
				Status: "success",
			})
		}
		// 收集工具执行错误
		if event.Type == "error" {
			toolExecRecords = append(toolExecRecords, toolExecRecord{
				Name:   event.ToolName,
				Args:   currentToolArgs,
				Error:  event.Error,
				Status: "error",
			})
		}
		// 收集安全守卫事件（审批通知）
		// 去重逻辑：相同 approval_id 的事件只保留最后一条（更新而非追加）
		if event.Type == "guard" {
			record := toolExecRecord{
				Name:          event.ToolName,
				Args:          event.Args,
				Status:        "guard",
				ApprovalID:    event.ApprovalID,
				ApprovalState: event.ApprovalState,
				GuardMessage:  event.GuardMessage,
			}
			if event.ApprovalID != "" {
				// 有 approval_id，检查是否已存在
				if idx, exists := guardEventIndex[event.ApprovalID]; exists {
					// 更新已存在的记录（保留最新状态）
					toolExecRecords[idx] = record
				} else {
					// 新记录
					guardEventIndex[event.ApprovalID] = len(toolExecRecords)
					toolExecRecords = append(toolExecRecords, record)
				}
			} else {
				// 无 approval_id（Deny 直接拒绝的情况），直接追加
				toolExecRecords = append(toolExecRecords, record)
			}
		}
		// 调用原始 handler
		if handler != nil {
			handler(event)
		}
	}

	logger.Info("[Agent] 开始执行Runtime",
		"tools_count", len(a.config.Tools),
		"max_iterations", a.config.MaxIterations,
		"blocks_count", len(blocks))

	// 如果有记忆上下文或多模态 blocks，将增强后的消息传入 Runtime
	// ExecuteWithEnhancedMessage 会临时替换会话中最后一条 user 消息，构建完 LLM 请求后恢复
	var enhancedMsg string
	if llmMessage != userMessage || len(blocks) > 0 {
		enhancedMsg = llmMessage
	}

	finalResponse, err := a.runtime.ExecuteWithEnhancedMessage(ctx, session, enhancedMsg, a.config.Tools, a.config.MaxIterations, wrappedHandler)
	if err != nil {
		logger.Error("[Agent] Runtime执行失败", "err", err)
		return "", err
	}

	// 将内容块放到响应开头（确保 session 持久化包含图片等媒体信息）
	// 然后追加 LLM 的文本响应
	if len(contentBlocks) > 0 {
		contentBlocks = append(contentBlocks, channel.NewTextBlock(finalResponse))
	} else {
		contentBlocks = channel.ContentBlocksFromText(finalResponse)
	}

	// assistant 角色不存储图片 base64 数据（会撑大 session 文件且 LLM 无法处理）
	contentBlocks = channel.StripImageBlocks(contentBlocks)
	// 合并所有连续的 TextBlock 为一个（工具结果 + LLM 响应合并）
	contentBlocks = channel.MergeTextBlocks(contentBlocks)

	logger.Info("[Agent] Runtime执行完成", "response_len", len(finalResponse), "blocks_count", len(contentBlocks))

	// 存储记忆
	if a.memory != nil {
		a.memory.Store(ctx, memory.MemoryEntry{
			Content:    userMessage,
			Type:       "short_term",
			SessionID:  sessionID,
			UserID:     session.UserID,
			Metadata:   map[string]interface{}{"role": "user"},
			Importance: 0.5,
			CreatedAt:  time.Now(),
		})
		a.memory.Store(ctx, memory.MemoryEntry{
			Content:    finalResponse,
			Type:       "short_term",
			SessionID:  sessionID,
			UserID:     session.UserID,
			Metadata:   map[string]interface{}{"role": "assistant"},
			Importance: 0.6,
			CreatedAt:  time.Now(),
		})
		logger.Debug("[Agent] 对话已存入记忆")
	}

	session.AddMessage("assistant", contentBlocks)

	// 将工具执行过程保存到 Metadata（供前端折叠显示）
	if len(thinkingParts) > 0 || len(toolExecRecords) > 0 {
		lastIdx := len(session.Messages) - 1
		if lastIdx >= 0 && session.Messages[lastIdx].Role == "assistant" {
			metadata := map[string]interface{}{}
			if len(thinkingParts) > 0 {
				metadata["thinking"] = strings.Join(thinkingParts, "\n\n")
			}
			if len(toolExecRecords) > 0 {
				metadata["tool_calls"] = toolExecRecords
			}
			session.Messages[lastIdx].Metadata = metadata
			// 触发持久化
			session.Persist()
		}
	}

	logger.Info("[Agent] 消息处理完成", "session", sessionID)

	// 自动记忆提取：对话后提取关键信息存入长期记忆
	go a.extractMemoryAsync(userMessage, finalResponse, sessionID, session.UserID)

	return finalResponse, nil
}

// extractMemoryAsync 异步提取记忆（不阻塞主流程）
func (a *Agent) extractMemoryAsync(userMsg, assistantMsg, sessionID, user string) {
	logger := glog.Logger()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 调用 LLM 提取关键信息
	extracted := a.runtime.ExtractMemory(ctx, userMsg, assistantMsg)
	if extracted == "" {
		return
	}

	// 存入长期记忆
	if a.memory != nil {
		a.memory.Store(ctx, memory.MemoryEntry{
			Content:    extracted,
			Type:       "long_term",
			SessionID:  sessionID,
			UserID:     user,
			Metadata:   map[string]interface{}{"source": "auto_extraction"},
			Importance: 0.8,
			CreatedAt:  time.Now(),
		})
		logger.Info("[Agent] 自动记忆提取完成", "len", len(extracted))
	}

	// 追加到每日记忆
	if a.config.WorkspaceLoader != nil {
		if err := a.config.WorkspaceLoader.AppendDailyMemory("- " + extracted); err != nil {
			logger.Warn("[Agent] 写入每日记忆失败", "err", err)
		}
	}
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

// GetStore 返回底层 Store 实例（用于从文件加载历史记录）
func (a *Agent) GetStore() store.Store {
	return a.config.Store
}

// ListSessions 列出所有会话
func (a *Agent) ListSessions() []SessionSummary {
	sessions := a.sessionMgr.ListSessions()
	var summaries []SessionSummary
	for i := range sessions {
		summaries = append(summaries, SessionSummary{
			ID:        sessions[i].ID,
			Channel:   sessions[i].Channel,
			SessionID: sessions[i].SessionID,
			Name:      sessions[i].Name,
			UserID:    sessions[i].UserID,
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
			Metadata:  m.Metadata,
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

// ─────────── context 传递 channel/user ───────────

type ctxKey int

const (
	ctxSessionChannel ctxKey = iota
	ctxSessionUser
)

// WithChannel 将渠道名注入 context
func WithChannel(ctx context.Context, channel string) context.Context {
	return context.WithValue(ctx, ctxSessionChannel, channel)
}

// WithUser 将用户 ID 注入 context
func WithUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, ctxSessionUser, user)
}

// GetChannelFromCtx 从 context 读取渠道名
func GetChannelFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxSessionChannel).(string); ok {
		return v
	}
	return ""
}

// GetUserFromCtx 从 context 读取用户 ID
func GetUserFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxSessionUser).(string); ok {
		return v
	}
	return ""
}

// SessionMessage 会话消息
type SessionMessage struct {
	Role      string
	Content   channel.ContentBlocks
	Metadata  map[string]interface{} // 扩展字段（thinking、tool_calls 等）
	Timestamp time.Time
}

// SessionSummary 会话摘要
type SessionSummary struct {
	ID        string
	SessionID string
	Name      string
	UserID    string
	Channel   string
	CreatedAt time.Time
	UpdatedAt time.Time
}
