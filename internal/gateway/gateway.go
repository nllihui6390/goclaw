package gateway

import (
	"context"
	"fmt"
	"go-claw/internal/agent"
	"go-claw/internal/channel"
	"sync"

	"go-claw/pkg/log"
)

// Gateway 网关核心
type Gateway struct {
	agents   map[string]*agent.Agent    // agent名称 -> agent实例
	channels map[string]channel.Channel // 渠道名称 -> 渠道实例
	router   *Router                    // 路由器
	bus      *AgentBus                  // Agent间消息总线
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
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

// RegisterChannel 注册渠道
func (g *Gateway) RegisterChannel(ch channel.Channel) error {
	if err := ch.Start(g.ctx); err != nil {
		return err
	}
	g.channels[ch.GetName()] = ch
	log.Logger().Info("渠道已注册", "name", ch.GetName())
	return nil
}

// AddRoute 添加路由规则
func (g *Gateway) AddRoute(rule RouteRule) {
	g.router.AddRule(rule)
}

// SetDefaultAgent 设置默认Agent
func (g *Gateway) SetDefaultAgent(name string) {
	g.router.SetDefaultAgent(name)
}

// Start 启动网关
func (g *Gateway) Start() error {
	for name, ch := range g.channels {
		g.wg.Add(1)
		go g.handleChannel(name, ch)
	}
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

			// 处理消息
			sessionID := fmt.Sprintf("%s:%s", msg.Channel, msg.From)
			response, err := ag.Process(g.ctx, sessionID, msg.Content)
			if err != nil {
				response = fmt.Sprintf("处理出错: %v", err)
				log.Logger().Error("消息处理失败", "err", err, "session", sessionID)
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
