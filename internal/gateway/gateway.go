package gateway

import (
	"context"
	"fmt"
	"go-claw/internal/agent"
	"go-claw/internal/channel"
	"go-claw/internal/store"
	"go-claw/utils"
	"sync"

	"go-claw/pkg/log"
)

// Gateway 网关核心
type Gateway struct {
	agents       map[string]*agent.Agent    // agent名称 -> agent实例
	channels     map[string]channel.Channel // 渠道名称 → 渠道实例
	router       *Router                    // 路由器
	bus          *AgentBus                  // Agent间消息总线
	sessionIndex *store.SessionIndex       // 会话索引（channel:user → UUID）
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// SetSessionIndex 设置会话索引
func (g *Gateway) SetSessionIndex(idx *store.SessionIndex) {
	g.sessionIndex = idx
}

// GetSessionIndex 获取会话索引
func (g *Gateway) GetSessionIndex() *store.SessionIndex {
	return g.sessionIndex
}

// GetAgents 获取所有 Agent 实例（供 proactive 等外部模块使用）
func (g *Gateway) GetAgents() map[string]*agent.Agent {
	return g.agents
}

// SendProactiveMessage 发送主动消息（实现 ProactiveBus 接口）
func (g *Gateway) SendProactiveMessage(ctx context.Context, sessionID, message string) error {
	logger := log.Logger()
	// 解析 sessionID (格式: "channel:user")
	parts := splitSessionID(sessionID)
	if len(parts) != 2 {
		return fmt.Errorf("invalid session ID format: %s", sessionID)
	}
	channelName, user := parts[0], parts[1]

	ch, exists := g.channels[channelName]
	if !exists {
		return fmt.Errorf("channel not found: %s", channelName)
	}

	logger.Info("[Gateway] 发送主动消息", "channel", channelName, "user", user, "msg_len", len(message))

	// 优先使用 ProactiveSender 接口（适合 WeCom 等需要主动推送的渠道）
	if ps, ok := ch.(channel.ProactiveSender); ok {
		return ps.SendProactive(ctx, user, message)
	}

	// 其他渠道使用普通 Send
	return ch.Send(ctx, channel.Response{
		Content: message,
		Channel: channelName,
		To:      user,
	})
}

// splitSessionID 解析 sessionID
func splitSessionID(sessionID string) []string {
	idx := 0
	for i, c := range sessionID {
		if c == ':' {
			idx = i
			break
		}
	}
	if idx == 0 {
		return nil
	}
	return []string{sessionID[:idx], sessionID[idx+1:]}
}

// NewGateway 创建网关
func NewGateway() *Gateway {
	ctx, cancel := context.WithCancel(context.Background())
	return &Gateway{
		agents:   make(map[string]*agent.Agent),
		channels: make(map[string]channel.Channel),
		router:   NewRouter(),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// RegisterAgent 注册Agent
func (g *Gateway) RegisterAgent(name string, ag *agent.Agent) {
	g.agents[name] = ag
	log.Logger().Info("Agent已注册", "name", name)
}

// RegisterChannel 注册渠道并启动消息处理
func (g *Gateway) RegisterChannel(ch channel.Channel) error {
	if err := ch.Start(g.ctx); err != nil {
		return err
	}
	g.channels[ch.GetName()] = ch
	g.wg.Add(1)
	go g.handleChannel(ch.GetName(), ch)
	log.Logger().Info("渠道已注册", "name", ch.GetName())
	return nil
}

// RegisterChannelWithoutServer 注册渠道但不启动其自带 HTTP 服务器（共用外部 mux）
func (g *Gateway) RegisterChannelWithoutServer(ch channel.Channel) {
	g.channels[ch.GetName()] = ch
	g.wg.Add(1)
	go g.handleChannel(ch.GetName(), ch)
	log.Logger().Info("渠道已注册(无自带服务)", "name", ch.GetName())
}

// UnregisterChannel 注销渠道
func (g *Gateway) UnregisterChannel(name string) {
	ch, exists := g.channels[name]
	if !exists {
		return
	}
	ch.Stop()
	delete(g.channels, name)
	log.Logger().Info("渠道已注销", "name", name)
}

// HasChannel 检查渠道是否已注册
func (g *Gateway) HasChannel(name string) bool {
	_, exists := g.channels[name]
	return exists
}

// GetRegisteredChannels 获取已注册的渠道名称列表
func (g *Gateway) GetRegisteredChannels() []string {
	names := make([]string, 0, len(g.channels))
	for name := range g.channels {
		names = append(names, name)
	}
	return names
}

// AddRoute 添加路由规则
func (g *Gateway) AddRoute(rule RouteRule) {
	g.router.AddRule(rule)
}

// SetDefaultAgent 设置默认Agent
func (g *Gateway) SetDefaultAgent(name string) {
	g.router.SetDefaultAgent(name)
}

// Start 启动网关（handleChannel 已在 RegisterChannel/RegisterChannelWithoutServer 中启动）
func (g *Gateway) Start() error {
	return nil
}

// Stop 停止网关
func (g *Gateway) Stop() {
	g.cancel()
	for _, ch := range g.channels {
		ch.Stop()
	}
	g.wg.Wait()
}

// CleanupExpiredSessions 清理所有Agent的过期会话
func (g *Gateway) CleanupExpiredSessions(ttlMinutes int) {
	for _, ag := range g.agents {
		ag.CleanupExpiredSessions(ttlMinutes)
	}
}

// GetAgent 获取指定Agent（外部访问用）
func (g *Gateway) GetAgent(name string) *agent.Agent {
	return g.agents[name]
}

// handleChannel 处理单个渠道的消息
func (g *Gateway) handleChannel(channelName string, ch channel.Channel) {
	defer g.wg.Done()

	msgChan, err := ch.Receive(g.ctx)
	if err != nil {
		log.Logger().Error("获取消息通道失败", "channel", channelName, "err", err)
		return
	}

	for {
		select {
		case <-g.ctx.Done():
			return
		case msg, ok := <-msgChan:
			if !ok {
				return
			}

			// 路由到合适的Agent
			agentName := g.router.Route(msg)
			ag, exists := g.agents[agentName]
			if !exists {
				log.Logger().Warn("目标Agent不存在，回退到默认Agent",
					"requested", agentName,
					"default", g.router.defaultAgent)
				agentName = g.router.defaultAgent
				ag, exists = g.agents[agentName]
				if !exists {
					log.Logger().Error("默认Agent也不存在", "agentName", agentName)
					continue
				}
			}

			// 获取会话 ID：渠道消息走索引映射，webhook/console 直接使用 msg.From
			sessionID := msg.From
			// 如果 msg.From 不是 UUID 格式（如 wecom:userid），通过索引转为 UUID
			if !utils.IsUUID(msg.From) {
				var isNew bool
				sessionID, isNew = g.sessionIndex.LookupOrCreate(msg.Channel, msg.From, agentName)
				if isNew {
					log.Logger().Info("[Gateway] 新会话", "uuid", sessionID, "channel", msg.Channel, "user", msg.From)
				}
			} else if g.sessionIndex != nil {
				g.sessionIndex.EnsureEntry(sessionID, msg.Channel, msg.From, agentName)
			}

			// 注入 Channel 和目标用户到 context
			msgCtx := channel.WithChannel(g.ctx, ch)
			msgCtx = channel.WithToUser(msgCtx, msg.From)
			msgCtx = agent.WithChannel(msgCtx, msg.Channel)
			msgCtx = agent.WithUser(msgCtx, msg.From)

			handler := func(event agent.ToolEvent) {
				ch.SendToolEvent(channel.ToolEvent{
					Type:     channel.ToolEventType(event.Type),
					ToolName: event.ToolName,
					Args:     event.Args,
					Result:   event.Result,
					Error:    event.Error,
					Thinking: event.Thinking,
					To:       msg.From,
				})
			}

			response, err := ag.ProcessWithHandler(msgCtx, sessionID, msg.Content, handler)
			if err != nil {
				response = fmt.Sprintf("处理出错: %v", err)
				log.Logger().Error("消息处理失败", "err", err, "session", sessionID)
			}
			// 统一记录会话活动
			if g.sessionIndex != nil {
				g.sessionIndex.RecordSession(sessionID, msg.Channel, msg.From, agentName, msg.Content)
			}

			// 发送响应
			ch.Send(g.ctx, channel.Response{
				Content: response,
				Channel: msg.Channel,
				To:      msg.From,
			})
		}
	}
}
