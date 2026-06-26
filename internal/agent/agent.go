package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-claw/internal/channel"
	"go-claw/internal/inbox"
	"go-claw/internal/memory"
	"go-claw/internal/security"
	"go-claw/internal/skill"
	"go-claw/internal/store"
	"go-claw/internal/tool"
	glog "go-claw/pkg/log"

	goAgent "github.com/nllihui6390/go-agent"
	goTool "github.com/nllihui6390/go-agent/tool"
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
	WorkspaceLoader       WorkspaceLoader      // 工作空间人设文件加载器
	WorkspaceDir          string               // 工作空间目录路径（用于缓存文件）
	SkillRegistry         *skill.SkillRegistry // 技能注册中心（用于系统提示词注入）
	MaxContextMessages    int                  // 未压缩消息数触发阈值，0=默认20
	CompactThresholdRatio float64              // 压缩触发比例，0=不压缩（默认0.8）
	ReserveThresholdRatio float64              // 压缩后保留比例（默认0.15）
	ToolResultMaxBytes    int                  // 工具结果最大字节数，0=不限（默认20000）
	ToolResultExemptTools []string             // 裁剪豁免工具名列表
	ToolResultExemptExts  []string             // 裁剪豁免文件扩展名列表
	SupportsImage         bool                 // 模型是否支持图片输入
	SupportsVideo         bool                 // 模型是否支持视频输入
	ToolGuard             *security.ToolGuard  // 工具安全守卫
	InboxStore            *inbox.Store         // Inbox 事件通知存储
	// ConfigProvider 动态配置提供器：每次调用 LLM 时获取最新 model/apiKey/baseURL/providerType
	// 优先使用此函数，降级使用 Model/APIKey/BaseURL/ProviderType 字段
	ConfigProvider func() (model, apiKey, baseURL, providerType string)
	// RateLimit 速率限制配置（防 429）：0=使用默认值，-1=不限制
	RateLimitMaxConcurrent  int // 最大并发请求数（默认 10）
	RateLimitMaxQPM         int // 每分钟最大请求数（默认 60）
	RateLimitAcquireTimeout int // 获取槽位超时秒数（默认 30）
}

// Agent AI智能体
type Agent struct {
	config     *AgentConfig
	runtime    *Runtime
	sessionMgr *SessionManager
	memory     memory.Memory

	// go-agent 循环引擎
	goAgent   *goAgent.Agent
	goAgentMu sync.Mutex
}

// NewAgent 创建Agent
func NewAgent(cfg *AgentConfig) *Agent {
	runtime := NewRuntime(cfg)
	if cfg.WorkspaceDir != "" {
		runtime.SetWorkspaceDir(cfg.WorkspaceDir)
	}
	a := &Agent{
		config:     cfg,
		runtime:    runtime,
		sessionMgr: NewSessionManager(cfg.Store),
		memory:     cfg.Memory,
	}
	a.initGoAgent()
	return a
}

// initGoAgent 创建 go-agent 循环引擎
func (a *Agent) initGoAgent() {
	cfg := a.config

	// 注册工具到 go-agent Registry
	toolReg := goTool.NewRegistry()
	for _, t := range cfg.Tools {
		toolReg.Register(t)
	}

	// 创建 go-agent Config
	agentCfg := goAgent.DefaultConfig(cfg.Name, a.runtime.chatModel, cfg.SystemPrompt).
		WithTools(toolReg).
		WithMaxIters(cfg.MaxIterations).
		WithWorkspaceDir(cfg.WorkspaceDir).
		WithSupportsImage(cfg.SupportsImage).
		WithSupportsVideo(cfg.SupportsVideo)

	if cfg.Memory != nil {
		agentCfg = agentCfg.WithMemory(memory.AsAgentMem(cfg.Memory))
	}
	if cfg.SkillRegistry != nil {
		agentCfg = agentCfg.WithSkills(skill.ConvertToGoAgentRegistry(cfg.SkillRegistry))
	}
	if cfg.WorkspaceLoader != nil {
		agentCfg = agentCfg.WithPersonaLoader(cfg.WorkspaceLoader)
	}

	// 接入 go-claw 的上下文截断配置
	if cfg.ToolResultMaxBytes > 0 || len(cfg.ToolResultExemptTools) > 0 || len(cfg.ToolResultExemptExts) > 0 {
		// go-claw 用字节数，go-agent 用 token 数。保守换算：1 token ≈ 3 字节
		limit := cfg.ToolResultMaxBytes / 3
		if limit <= 0 {
			limit = 100000 // 兜底：不截断
		}
		agentCfg = agentCfg.WithContextConfig(&goAgent.ContextConfig{
			ToolResultLimit:       limit,
			ToolResultExemptTools: cfg.ToolResultExemptTools,
			ToolResultExemptExts:  cfg.ToolResultExemptExts,
		})
	}

	// 接入 go-claw 的上下文压缩配置
	if cfg.CompactThresholdRatio > 0 || cfg.ReserveThresholdRatio > 0 {
		// 合并已有 ContextConfig（如果上面已设置了截断参数）
		ctxCfg := agentCfg.ContextConfig
		if ctxCfg == nil {
			ctxCfg = &goAgent.ContextConfig{}
		}
		if cfg.CompactThresholdRatio > 0 {
			ctxCfg.TriggerRatio = cfg.CompactThresholdRatio
		}
		if cfg.ReserveThresholdRatio > 0 {
			ctxCfg.ReserveRatio = cfg.ReserveThresholdRatio
		}
		agentCfg = agentCfg.WithContextConfig(ctxCfg)
	}

	a.goAgent = goAgent.NewAgent(*agentCfg)

	// 默认允许所有工具执行（go-claw 通过 ToolGuard 做安全检查，不走 PermissionChecker）
	if perm, ok := a.goAgent.GetConfig().Permission.(*goAgent.DefaultPermissionChecker); ok {
		perm.SetMode(goTool.ModeBypass)
	}
}

// refreshGoAgent 热切换 ChatModel（动态配置变更时调用）
func (a *Agent) refreshGoAgent() {
	a.runtime.refreshChatModel()
	// go-agent 的 model 已经通过 runtime.chatModel 共享，无需重建
}

// convertSessionToGoAgent 将 go-claw 会话历史加载到 go-agent session
func (a *Agent) loadSessionToGoAgent(session *Session) {
	a.goAgentMu.Lock()
	defer a.goAgentMu.Unlock()

	goSession := a.goAgent.GetSession()
	goSession.Clear()

	// 跳过后添加的消息：当前轮的用户消息会由 ReplyStream 自己 AddMessage，
	// 避免会话中同一消息被加载两次导致模型输出重复
	msgs := session.Messages
	if len(msgs) > 0 && msgs[len(msgs)-1].Role == "user" {
		msgs = msgs[:len(msgs)-1]
	}

	// 不再硬编码截断历史消息数量。
	// 由 go-agent 的 autoCompressContext 在 ReplyStream 开始时自动检测 token 用量
	// 并触发 LLM 辅助压缩。这里只加载全部消息。
	for _, msg := range msgs {
		goMsg := goAgent.Msg{
			ID:        fmt.Sprintf("msg_%s_%d", msg.Role, len(goSession.GetHistory())),
			Name:      a.config.Name,
			Role:      goAgent.Role(msg.Role),
			Content:   channelBlocksToAgentBlocks(msg.Content),
			Metadata:  msg.Metadata,
			CreatedAt: msg.Timestamp.Format(time.RFC3339),
		}
		if msg.ToolCallID != "" {
			if msg.Role == "tool" {
				goMsg.Name = msg.ToolCallID
			} else if msg.Name != "" {
				goMsg.Name = msg.Name
			}
		}
		goSession.AddMessage(goMsg)
	}
}

// channelBlocksToAgentBlocks 转换 go-claw ContentBlocks → go-agent ContentBlock
func channelBlocksToAgentBlocks(blocks channel.ContentBlocks) []goAgent.ContentBlock {
	result := make([]goAgent.ContentBlock, 0, len(blocks))
	for _, b := range blocks {
		switch block := b.(type) {
		case *channel.TextBlock:
			result = append(result, goAgent.NewTextBlock(block.Text))
		case *channel.ImageBlock:
			if block.Source.Type == "url" {
				result = append(result, goAgent.NewDataBlockURL(block.Source.URL, block.Source.MediaType))
			} else {
				result = append(result, goAgent.NewDataBlockBase64(block.Source.Data, block.Source.MediaType))
			}
		case *channel.FileBlock:
			result = append(result, goAgent.NewTextBlock("[File: "+block.Filename+"]"))
		}
	}
	return result
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
func (a *Agent) SetSkillRegistry(reg *skill.SkillRegistry) {
	a.config.SkillRegistry = reg
	a.runtime.SetSkillRegistry(reg)

	// 同步更新 go-agent 的 Skills 配置
	a.goAgentMu.Lock()
	a.goAgent.GetConfig().Skills = skill.ConvertToGoAgentRegistry(reg)
	a.goAgentMu.Unlock()
}

// ProcessWithHandler 处理用户消息（带工具事件回调）
func (a *Agent) ProcessWithHandler(ctx context.Context, sessionID, userMessage string, handler channel.ToolEventHandler) (string, error) {
	return a.ProcessWithBlocks(ctx, sessionID, userMessage, nil, handler)
}

// ProcessWithBlocks 处理用户消息，支持传入结构化内容块（多模态：图片/文件等）
func (a *Agent) ProcessWithBlocks(ctx context.Context, sessionID, userMessage string, blocks channel.ContentBlocks, handler channel.ToolEventHandler) (string, error) {
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

	// 会话历史只保存原始用户消息
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

	logger.Info("[Agent] 开始执行 go-agent 循环",
		"tools_count", len(a.config.Tools),
		"max_iterations", a.config.MaxIterations,
		"blocks_count", len(blocks))

	// ─────────── 使用 go-agent ReAct 循环 ───────────
	// 1. 加载会话历史到 go-agent session
	a.loadSessionToGoAgent(session)

	// 2. 构建用户消息（含记忆上下文和多模态内容）
	userBlocks := channel.ParseFileMarkers(userMessage)
	userBlocks = append(blocks, userBlocks...)
	goBlocks := channelBlocksToAgentBlocks(userBlocks)

	goMsg := goAgent.UserMsg(session.UserID, goBlocks)

	// 3. 调用 go-agent 流式循环 → 同时收集回复 + 转发事件到前端
	eventCh, err := a.goAgent.ReplyStream(ctx, goMsg)
	if err != nil {
		logger.Error("[Agent] go-agent ReplyStream 启动失败", "err", err)
		return "", err
	}

	var finalResponse string
	var replyMsg *goAgent.Msg
	var currentToolName, currentToolArgs string
	var chatModelError bool

	// ─────────── 使用 go-agent AppendEvent 增量重建消息 ───────────
	for event := range eventCh {
		switch e := event.(type) {
		case goAgent.ReplyStartEvent:
			logger.Info("[Agent] ReplyStart", "replyID", e.ReplyID, "session", e.SessionID)
			replyMsg = &goAgent.Msg{
				ID:      e.ReplyID,
				Name:    e.Name,
				Role:    goAgent.Role(e.Role),
				Content: []goAgent.ContentBlock{},
			}
		case goAgent.TextBlockDeltaEvent:
			finalResponse += e.Delta
			if handler != nil {
				handler(channel.ToolEvent{Type: channel.ToolEventText, Thinking: e.Delta})
			}
		case goAgent.ThinkingBlockDeltaEvent:
			if handler != nil {
				handler(channel.ToolEvent{Type: channel.ToolEventThinking, Thinking: e.Delta})
			}
		case goAgent.ToolCallStartEvent:
			logger.Info("[Agent] ToolCallStart", "tool", e.ToolCallName)
			currentToolName = e.ToolCallName
			currentToolArgs = ""
		case goAgent.ToolCallDeltaEvent:
			currentToolArgs += e.Delta
			// 前端增量更新（参数逐步显示）
			if handler != nil {
				handler(channel.ToolEvent{Type: channel.ToolEventCalling, ToolName: currentToolName, Args: currentToolArgs})
			}
		case goAgent.ToolCallEndEvent:
			logger.Info("[Agent] ToolCallEnd", "tool", currentToolName)
		case goAgent.ToolResultStartEvent:
			logger.Info("[Agent] ToolResultStart", "tool", e.ToolCallName)

		case goAgent.ToolResultEndEvent:
			logger.Info("[Agent] ToolResultEnd", "tool", currentToolName, "state", e.State)
			if handler != nil {
				// 从重建的 replyMsg 中提取工具结果文本
				resultText := extractToolResultFromMsg(replyMsg, e.ToolCallID)
				evtType := channel.ToolEventResult
				if e.State == goAgent.ToolResultStateError {
					evtType = channel.ToolEventError
				}

				evt := channel.ToolEvent{Type: evtType, ToolName: currentToolName, Result: resultText}

				// 解析 agnes_image 和 send_file 的 JSON 结果，额外推送图片块
				// 保留 tool_result 事件（更新状态），同时发送 content 事件（推送图片）
				if resultText != "" {
					// agnes_image: {"success":true,"urls":["..."]}
					if currentToolName == "agnes_image" {
						var ir tool.AgnesImageResult
						if err := json.Unmarshal([]byte(resultText), &ir); err == nil && ir.Success && len(ir.URLs) > 0 {
							var blocks channel.ContentBlocks
							for _, u := range ir.URLs {
								blocks = append(blocks, channel.NewImageBlockURL(u))
							}
							handler(channel.ToolEvent{Type: channel.ToolEventContent, Content: blocks})
						}
					}
					// agnes_video: {"success":true,"urls":["..."]}
					if currentToolName == "agnes_video" {
						var vr tool.VideoResult
						if err := json.Unmarshal([]byte(resultText), &vr); err == nil && vr.Success && len(vr.URLs) > 0 {
							var blocks channel.ContentBlocks
							for _, u := range vr.URLs {
								blocks = append(blocks, channel.NewVideoBlockURL(u))
							}
							handler(channel.ToolEvent{Type: channel.ToolEventContent, Content: blocks})
						}
					}
					// send_file: {"status":"success","display_url":"...","filename":"...","is_url":true/false}
					if currentToolName == "send_file" {
						var sfResult struct {
							Status     string `json:"status"`
							DisplayURL string `json:"display_url"`
							Filename   string `json:"filename"`
							IsURL      bool   `json:"is_url"`
						}
						if err := json.Unmarshal([]byte(resultText), &sfResult); err == nil && sfResult.Status == "success" && sfResult.DisplayURL != "" {
							filename := sfResult.Filename
							if filename == "" {
								filename = filepath.Base(sfResult.DisplayURL)
							}
							block := channel.BlockFromURL(sfResult.DisplayURL, filename)
							if block != nil {
								handler(channel.ToolEvent{Type: channel.ToolEventContent, Content: channel.ContentBlocks{block}})
							}
						}
					}
				}

				handler(evt)
			}
		case goAgent.ReplyEndEvent:
			logger.Info("[Agent] ReplyEnd", "replyID", e.ReplyID, "session", e.SessionID,
				"totalInputTokens", e.TotalInputTokens, "totalOutputTokens", e.TotalOutputTokens)
			// LLM 调用失败时 tokens 为 0，标记错误
			if e.TotalInputTokens == 0 && e.TotalOutputTokens == 0 && finalResponse == "" {
				chatModelError = true
			}
		case goAgent.ModelCallStartEvent:
			logger.Info("[Agent] ModelCallStart", "model", e.ModelName)
		case goAgent.ModelCallEndEvent:
			logger.Info("[Agent] ModelCallEnd", "inputTokens", e.InputTokens, "outputTokens", e.OutputTokens)
		default:
			logger.Debug("[Agent] 未知事件", "type", fmt.Sprintf("%T", e))
		}

		// 增量应用到重建的消息（ReplyStart 已手动初始化 Msg，跳过重复构造）
		if replyMsg != nil {
			if _, isReplyStart := event.(goAgent.ReplyStartEvent); !isReplyStart {
				replyMsg.AppendEvent(event)
			}
		}
	}

	if finalResponse == "" && replyMsg != nil {
		// 无纯文本回复时，检查 replyMsg 中是否有 assistant 内容（不是从历史中捞旧消息）
		content := replyMsg.GetTextContent()
		if content != "" {
			finalResponse = content
		}
	}

	// LLM 调用失败时返回原始错误信息
	if chatModelError {
		if err := a.runtime.GetLastError(); err != nil {
			return "", fmt.Errorf("LLM 调用失败：%v", err)
		}
		return "", fmt.Errorf("LLM 调用失败：模型未返回有效内容（可能是 API 密钥无效或服务不可用）")
	}

	logger.Info("[Agent] go-agent 循环完成", "response_len", len(finalResponse))

	// 合并 go-agent session 中的工具调用历史回 go-claw session
	a.mergeGoAgentHistory(session, replyMsg)

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

	// mergeGoAgentHistory 已保存了完整的 assistant + tool 消息（含 metadata），
	// 这里不再重复保存。

	logger.Info("[Agent] 消息处理完成", "session", sessionID)

	// 自动记忆提取
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

// extractToolResultFromMsg 从已重建的 replyMsg 中获取指定 toolCallID 的工具结果文本。
func extractToolResultFromMsg(msg *goAgent.Msg, toolCallID string) string {
	if msg == nil {
		return ""
	}
	for _, b := range msg.Content {
		if b.Type == goAgent.BlockTypeToolResult && b.ToolCallID == toolCallID {
			var texts []string
			for _, out := range b.ToolResultOutput {
				if out.Type == goAgent.BlockTypeText {
					texts = append(texts, out.Text)
				}
			}
			return strings.Join(texts, "\n")
		}
	}
	return ""
}

// goAgentGetLastToolResult 从 go-agent session 中获取指定 tool_call_id 的最后一条工具结果文本
// （旧版实现，现已被 extractToolResultFromMsg 取代，保留以兼容其他调用点）
func goAgentGetLastToolResult(gag *goAgent.Agent, toolCallID string) string {
	for _, m := range gag.GetSession().GetHistory() {
		for _, b := range m.Content {
			if b.Type == goAgent.BlockTypeToolResult && b.ToolCallID == toolCallID {
				var texts []string
				for _, out := range b.ToolResultOutput {
					if out.Type == goAgent.BlockTypeText {
						texts = append(texts, out.Text)
					}
				}
				return strings.Join(texts, "\n")
			}
		}
	}
	return ""
}

// mergeGoAgentHistory 将 go-agent session 中本轮新增消息合并到 go-claw session。
// 正确保留：文本、工具调用/结果 metadata、思考内容。
func (a *Agent) mergeGoAgentHistory(clawSess *Session, replyMsg *goAgent.Msg) {
	goHistory := a.goAgent.GetSession().GetHistory()
	if len(goHistory) == 0 {
		return
	}

	// 找到最后一条 user 消息之后的新增消息
	startIdx := 0
	for i := len(goHistory) - 1; i >= 0; i-- {
		if goHistory[i].Role == goAgent.RoleUser {
			startIdx = i + 1
			break
		}
	}

	// 合并所有新增消息：收集文本、思考、工具调用，产出单条完整的 assistant 消息
	type tcRec struct {
		Name   string `json:"name"`
		Args   string `json:"args"`
		Result string `json:"result,omitempty"`
		Error  string `json:"error,omitempty"`
		Status string `json:"status"`
	}
	var allThinkParts []string
	var allTC []tcRec
	var finalText string
	var curName, curArgs string
	var allDataBlocks []channel.ContentBlock // 收集图片/文件数据块

	for i := startIdx; i < len(goHistory); i++ {
		m := goHistory[i]

		for _, b := range m.Content {
			switch b.Type {
			case goAgent.BlockTypeText:
				if b.Text != "" {
					finalText += b.Text
				}
			case goAgent.BlockTypeThinking:
				allThinkParts = append(allThinkParts, b.Thinking)
			case goAgent.BlockTypeToolCall:
				curName = b.ToolCallName
				curArgs = b.ToolCallInput
			case goAgent.BlockTypeToolResult:
				var text string
				for _, o := range b.ToolResultOutput {
					if o.Type == goAgent.BlockTypeText {
						text += o.Text
					} else if o.Type == goAgent.BlockTypeData {
						// 工具结果中的数据块（如 send_file 返回的图片）保存到历史
						if o.Source != nil {
							switch o.MediaType {
							case "image/png", "image/jpeg", "image/gif", "image/webp":
								allDataBlocks = append(allDataBlocks, channel.NewImageBlockURL(o.Source.URL))
							case "video/mp4", "video/webm":
								allDataBlocks = append(allDataBlocks, channel.NewVideoBlockURL(o.Source.URL))
							case "audio/mpeg", "audio/wav":
								allDataBlocks = append(allDataBlocks, channel.NewAudioBlockURL(o.Source.URL))
							default:
								allDataBlocks = append(allDataBlocks, channel.NewFileBlockURL(o.Source.URL, ""))
							}
						}
					}
				}
				// 从文本结果中提取图片 URL（如 agnes_image 返回的 JSON）
				// 注意：不在此处添加图片，图片由 send_file 统一添加到聊天记录
				// 从 send_file 结果中提取文件路径，转为 ContentBlock
				// 注意：如果前面 agnes_image 已经添加了同名 URL，这里跳过避免重复
				if text != "" && curName == "send_file" {
					var sfResult struct {
						Status     string `json:"status"`
						DisplayURL string `json:"display_url"`
						Path       string `json:"path"`
						Filename   string `json:"filename"`
						IsURL      bool   `json:"is_url"`
					}
					if err := json.Unmarshal([]byte(text), &sfResult); err == nil && sfResult.Status == "success" && sfResult.DisplayURL != "" {
						// 检查是否已经在 allDataBlocks 中存在相同 URL
						found := false
						for _, existing := range allDataBlocks {
							if img, ok := existing.(*channel.ImageBlock); ok {
								if img.Source.Type == "url" && img.Source.URL == sfResult.DisplayURL {
									found = true
									break
								}
							}
						}
						if found {
							continue
						}
						displayURL := sfResult.DisplayURL
						filename := sfResult.Filename
						if filename == "" {
							filename = filepath.Base(sfResult.Path)
						}
						block := channel.BlockFromURL(displayURL, filename)
						if block != nil {
							allDataBlocks = append(allDataBlocks, block)
						}
					}
				}
				st := "success"
				if b.ToolResultState == goAgent.ToolResultStateError {
					st = "error"
				} else if b.ToolResultState == goAgent.ToolResultStateDenied {
					st = "guard"
				}
				allTC = append(allTC, tcRec{Name: curName, Args: curArgs, Result: text, Status: st})
			}
		}

		// tool 角色消息单独追加
		if m.Role == "tool" {
			clawSess.AddTextMessage("tool", m.GetTextContent())
			if last := len(clawSess.Messages) - 1; last >= 0 && m.Name != "" {
				clawSess.Messages[last].Name = m.Name
				clawSess.Messages[last].ToolCallID = m.Name
			}
		}
	}

	// 保存一条完整的 assistant 消息（文本 + thinking + tool_calls + 数据块）
	if finalText != "" || len(allTC) > 0 || len(allThinkParts) > 0 || len(allDataBlocks) > 0 {
		// 构建完整的 content：数据块在前，文本在后
		var content channel.ContentBlocks
		content = append(content, allDataBlocks...)
		if finalText != "" {
			content = append(content, channel.NewTextBlock(finalText))
		}
		clawSess.AddMessage("assistant", content)
		if last := len(clawSess.Messages) - 1; last >= 0 {
			meta := map[string]interface{}{}
			if len(allThinkParts) > 0 {
				meta["thinking"] = strings.Join(allThinkParts, "")
			}
			if len(allTC) > 0 {
				meta["tool_calls"] = allTC
			}
			if len(meta) > 0 {
				clawSess.Messages[last].Metadata = meta
			}
		}
	}

	clawSess.Persist()
	_ = replyMsg
}
