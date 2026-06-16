package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go-claw/internal/agent"
	"go-claw/internal/channel"
	"go-claw/internal/mcp"
	"go-claw/internal/store"
	"go-claw/pkg/log"
	"go-claw/pkg/utils"
)

// Gateway 网关核心
type Gateway struct {
	mu           sync.RWMutex             // 保护 agents/channels 并发访问
	agents       map[string]*agent.Agent  // agent名称 -> agent实例
	channels     map[string]channel.Channel // 渠道名称 → 渠道实例
	router       *Router                    // 路由器
	bus          *AgentBus                  // Agent间消息总线
	sessionIndex *store.SessionIndex        // 会话索引（channel:user → UUID）
	MCPMgr       *mcp.Manager              // MCP 管理器
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
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.agents
}

// SendProactiveMessage 发送主动消息（实现 ProactiveBus 接口）
// sessionID 格式：
//   - `channel:user` (全局渠道如 console)
//   - `agent:channel:user` (per-agent 渠道)
//   - `channel:user` 且渠道为 per-agent 类型时，尝试用 default agent 查找
func (g *Gateway) SendProactiveMessage(ctx context.Context, sessionID, message string) error {
	logger := log.Logger()
	// 解析 sessionID
	channelName, user := g.parseSessionID(sessionID)
	if channelName == "" {
		return fmt.Errorf("invalid session ID format: %s", sessionID)
	}

	// 查找渠道
	ch := g.findChannel(channelName)
	if ch == nil {
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

// parseSessionID 解析 sessionID，返回渠道名和用户
// 支持三种格式：
//   - `agent:channel:user` → 返回 `agent:channel`, `user`
//   - `channel:user` → 返回 `channel`, `user`
func (g *Gateway) parseSessionID(sessionID string) (channelName, user string) {
	// 找到所有冒号位置
	colons := []int{}
	for i, c := range sessionID {
		if c == ':' {
			colons = append(colons, i)
		}
	}

	if len(colons) < 1 {
		return "", ""
	}

	// 如果有两个冒号，可能是 agent:channel:user 格式
	if len(colons) >= 2 {
		// 检查第一部分是否是 agent 名
		potentialAgent := sessionID[:colons[0]]
		g.mu.RLock()
		_, isAgent := g.agents[potentialAgent]
		g.mu.RUnlock()
		if isAgent {
			// agent:channel:user 格式
			channelName = sessionID[:colons[1]] // agent:channel
			user = sessionID[colons[1]+1:]
			return channelName, user
		}
	}

	// channel:user 格式
	channelName = sessionID[:colons[0]]
	user = sessionID[colons[0]+1:]
	return channelName, user
}

// findChannel 查找渠道实例
// 优先精确匹配，然后尝试 default:channel，最后搜索所有 agent:channel 组合
func (g *Gateway) findChannel(channelName string) channel.Channel {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// 1. 精确匹配
	if ch, exists := g.channels[channelName]; exists {
		return ch
	}

	// 2. 如果是简单渠道名，尝试 default:channel
	if !containsColon(channelName) {
		defaultAgent := g.router.defaultAgent
		if defaultAgent != "" {
			key := defaultAgent + ":" + channelName
			if ch, exists := g.channels[key]; exists {
				return ch
			}
		}

		// 3. 搜索所有 agent:channel 组合
		for key, ch := range g.channels {
			// 检查 key 是否以 :channelName 结尾
			if strings.HasSuffix(key, ":"+channelName) {
				return ch
			}
		}
	}

	return nil
}

// containsColon 检查字符串是否包含冒号
func containsColon(s string) bool {
	for _, c := range s {
		if c == ':' {
			return true
		}
	}
	return false
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
	g.mu.Lock()
	g.agents[name] = ag
	g.mu.Unlock()
	log.Logger().Info("Agent已注册", "name", name)
}

// UnregisterAgent 注销Agent
func (g *Gateway) UnregisterAgent(name string) {
	g.mu.Lock()
	delete(g.agents, name)
	g.mu.Unlock()
	log.Logger().Info("Agent已注销", "name", name)
}

// RegisterChannel 注册渠道并启动消息处理（全局渠道，如 console）
func (g *Gateway) RegisterChannel(ch channel.Channel) error {
	if err := ch.Start(g.ctx); err != nil {
		return err
	}
	g.mu.Lock()
	g.channels[ch.GetName()] = ch
	g.mu.Unlock()
	g.wg.Add(1)
	go g.handleChannel(ch.GetName(), ch, "")
	log.Logger().Info("渠道已注册", "name", ch.GetName())
	return nil
}

// RegisterChannelForAgent 注册 per-agent 渠道（如 default:lark, weather:wecom）
// agentName 用于路由：该渠道收到的消息自动路由到所属 Agent
func (g *Gateway) RegisterChannelForAgent(agentName string, ch channel.Channel) error {
	if err := ch.Start(g.ctx); err != nil {
		return err
	}
	key := agentName + ":" + ch.GetName()
	g.mu.Lock()
	g.channels[key] = ch
	g.mu.Unlock()
	g.wg.Add(1)
	go g.handleChannel(key, ch, agentName)
	log.Logger().Info("渠道已注册(per-agent)", "key", key, "agent", agentName)
	return nil
}

// RegisterChannelWithoutServer 注册渠道但不启动其自带 HTTP 服务器（共用外部 mux）
func (g *Gateway) RegisterChannelWithoutServer(ch channel.Channel) {
	g.mu.Lock()
	g.channels[ch.GetName()] = ch
	g.mu.Unlock()
	g.wg.Add(1)
	go g.handleChannel(ch.GetName(), ch, "")
	log.Logger().Info("渠道已注册(无自带服务)", "name", ch.GetName())
}

// UnregisterChannel 注销渠道（支持全局渠道名和 agent:channel 格式）
func (g *Gateway) UnregisterChannel(name string) {
	g.mu.Lock()
	ch, exists := g.channels[name]
	if !exists {
		g.mu.Unlock()
		return
	}
	delete(g.channels, name)
	g.mu.Unlock()
	ch.Stop()
	log.Logger().Info("渠道已注销", "name", name)
}

// UnregisterChannelForAgent 注销 per-agent 渠道
func (g *Gateway) UnregisterChannelForAgent(agentName, channelName string) {
	key := agentName + ":" + channelName
	g.UnregisterChannel(key)
}

// GetChannel 获取已注册的渠道实例（未注册返回 nil）
func (g *Gateway) GetChannel(name string) channel.Channel {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.channels[name]
}

// HasChannel 检查渠道是否已注册（支持全局渠道名和 agent:channel 格式）
func (g *Gateway) HasChannel(name string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, exists := g.channels[name]
	return exists
}

// HasChannelForAgent 检查 per-agent 渠道是否已注册
func (g *Gateway) HasChannelForAgent(agentName, channelName string) bool {
	return g.HasChannel(agentName + ":" + channelName)
}

// GetRegisteredChannels 获取已注册的渠道名称列表
func (g *Gateway) GetRegisteredChannels() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
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
	g.mu.RLock()
	channelsCopy := make([]channel.Channel, 0, len(g.channels))
	for _, ch := range g.channels {
		channelsCopy = append(channelsCopy, ch)
	}
	g.mu.RUnlock()
	for _, ch := range channelsCopy {
		ch.Stop()
	}
	g.wg.Wait()
}

// CleanupExpiredSessions 清理所有Agent的过期会话
func (g *Gateway) CleanupExpiredSessions(ttlMinutes int) {
	g.mu.RLock()
	agentsCopy := make([]*agent.Agent, 0, len(g.agents))
	for _, ag := range g.agents {
		agentsCopy = append(agentsCopy, ag)
	}
	g.mu.RUnlock()
	for _, ag := range agentsCopy {
		ag.CleanupExpiredSessions(ttlMinutes)
	}
}

// GetAgent 获取指定Agent（外部访问用）
func (g *Gateway) GetAgent(name string) *agent.Agent {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.agents[name]
}

// handleChannel 处理单个渠道的消息
// agentName 参数：per-agent 渠道自动路由到该 Agent；空字符串表示全局渠道（console），走正常路由
func (g *Gateway) handleChannel(channelKey string, ch channel.Channel, agentName string) {
	defer g.wg.Done()

	msgChan, err := ch.Receive(g.ctx)
	if err != nil {
		log.Logger().Error("获取消息通道失败", "channel", channelKey, "err", err)
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
			// per-agent 渠道：直接使用绑定的 agentName
			// 全局渠道（console）：使用 msg.Agent 或默认
			targetAgent := agentName
			if targetAgent == "" {
				targetAgent = g.router.Route(msg)
			} else {
				// per-agent 渠道：确保 msg.Agent 被设置
				if msg.Agent == "" {
					msg.Agent = agentName
				}
			}

			g.mu.RLock()
			ag, exists := g.agents[targetAgent]
			if !exists {
				log.Logger().Warn("目标Agent不存在，回退到默认Agent",
					"requested", targetAgent,
					"default", g.router.defaultAgent)
				targetAgent = g.router.defaultAgent
				ag, exists = g.agents[targetAgent]
			}
			g.mu.RUnlock()
			if !exists {
				log.Logger().Error("默认Agent也不存在", "agentName", targetAgent)
				continue
			}

			// 获取会话 ID：渠道消息走索引映射，webhook/console 直接使用 msg.From
			sessionID := msg.From
			// 如果 msg.From 不是 UUID 格式（如 wecom:userid），通过索引转为 UUID
			if !utils.IsUUID(msg.From) {
				var isNew bool
				sessionID, isNew = g.sessionIndex.LookupOrCreate(msg.Channel, msg.From, targetAgent)
				if isNew {
					log.Logger().Info("[Gateway] 新会话", "uuid", sessionID, "channel", msg.Channel, "user", msg.From)
				}
			} else if g.sessionIndex != nil {
				g.sessionIndex.EnsureEntry(sessionID, msg.Channel, msg.From, targetAgent)
			}

			// 注入 Channel 和目标用户到 context
			msgCtx := channel.WithChannel(g.ctx, ch)
			msgCtx = channel.WithToUser(msgCtx, msg.From)
			msgCtx = agent.WithChannel(msgCtx, msg.Channel)
			msgCtx = agent.WithUser(msgCtx, msg.From)

			var hadTextStream bool
			handler := func(event channel.ToolEvent) {
				if event.Type == channel.ToolEventText {
					hadTextStream = true
				}
				event.To = msg.From
				ch.SendToolEvent(event)
			}

			response, err := ag.ProcessWithBlocks(msgCtx, sessionID, msg.Content, msg.Blocks, handler)
			if err != nil {
				response = fmt.Sprintf("处理出错: %v", err)
				log.Logger().Error("消息处理失败", "err", err, "session", sessionID)
			}

			// 统一记录会话活动
			if g.sessionIndex != nil {
				g.sessionIndex.RecordSession(sessionID, msg.Channel, msg.From, targetAgent, msg.Content)
			}

			// 流式模式下文本已通过 ToolEventText 实时推送，末尾仅发空串触发流关闭避免重复
		sendContent := response
		if hadTextStream {
			sendContent = ""
		}
		ch.Send(g.ctx, channel.Response{
			Content: sendContent,
			Channel: msg.Channel,
			To:      msg.From,
		})
		}
	}
}
