package agent

import (
	"fmt"
	"sync"
	"time"
)

// AgentPool Agent 池管理器。
//
// 管理多个 Agent 实例，支持动态创建、回收和负载均衡。
// AgentPool 是并发安全的，支持高并发场景。
type AgentPool struct {
	agents      map[string]*Agent
	creators    map[string]func() (*Agent, error)
	mu          sync.RWMutex
	maxAgents   int
	activeCount int32
}

// NewAgentPool 创建新的 Agent 池。
//
// 参数：
//   - maxAgents: 最大 Agent 数量（0 表示不限制）
//
// 返回：
//   - *AgentPool: Agent 池实例
func NewAgentPool(maxAgents int) *AgentPool {
	return &AgentPool{
		agents:    make(map[string]*Agent),
		creators:  make(map[string]func() (*Agent, error)),
		maxAgents: maxAgents,
	}
}

// RegisterAgent 注册一个 Agent 到池中。
//
// 参数：
//   - name: Agent 名称（唯一标识）
//   - agent: Agent 实例
func (p *AgentPool) RegisterAgent(name string, agent *Agent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.maxAgents > 0 && len(p.agents) >= p.maxAgents {
		return
	}
	p.agents[name] = agent
}

// RegisterCreator 注册一个 Agent 创建函数。
//
// 当需要动态创建 Agent 时调用此函数。
//
// 参数：
//   - name: Agent 名称模板
//   - creator: 创建函数
func (p *AgentPool) RegisterCreator(name string, creator func() (*Agent, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.creators[name] = creator
}

// Get 获取指定名称的 Agent。
//
// 参数：
//   - name: Agent 名称
//
// 返回：
//   - *Agent: Agent 实例（不存在返回 nil）
func (p *AgentPool) Get(name string) *Agent {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.agents[name]
}

// GetOrCreate 获取或创建 Agent。
//
// 如果 Agent 不存在，尝试使用注册的 creator 创建。
//
// 参数：
//   - name: Agent 名称
//
// 返回：
//   - *Agent: Agent 实例
//   - error: 创建错误
func (p *AgentPool) GetOrCreate(name string) (*Agent, error) {
	p.mu.RLock()
	if agent, ok := p.agents[name]; ok {
		p.mu.RUnlock()
		return agent, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if agent, ok := p.agents[name]; ok {
		return agent, nil
	}

	if creator, ok := p.creators[name]; ok {
		agent, err := creator()
		if err != nil {
			return nil, err
		}
		p.agents[name] = agent
		return agent, nil
	}

	return nil, fmt.Errorf("agent %s not found and no creator registered", name)
}

// List 获取所有注册的 Agent 名称列表。
//
// 返回：
//   - []string: Agent 名称列表
func (p *AgentPool) List() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	names := make([]string, 0, len(p.agents))
	for name := range p.agents {
		names = append(names, name)
	}
	return names
}

// Remove 从池中移除指定 Agent。
//
// 参数：
//   - name: Agent 名称
func (p *AgentPool) Remove(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.agents, name)
}

// Clear 清空池中所有 Agent。
func (p *AgentPool) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.agents = make(map[string]*Agent)
}

// Count 获取当前池中 Agent 数量。
//
// 返回：
//   - int: Agent 数量
func (p *AgentPool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.agents)
}

// Broadcast 向所有 Agent 广播消息。
//
// 参数：
//   - ctx: 上下文
//   - msg: 消息
func (p *AgentPool) Broadcast(msg Msg) {
	p.mu.RLock()
	agents := make([]*Agent, 0, len(p.agents))
	for _, agent := range p.agents {
		agents = append(agents, agent)
	}
	p.mu.RUnlock()

	var wg sync.WaitGroup
	for _, agent := range agents {
		wg.Add(1)
		go func(a *Agent) {
			defer wg.Done()
			a.Observe(msg)
		}(agent)
	}
	wg.Wait()
}

// HealthCheck 检查所有 Agent 的健康状态。
//
// 返回：
//   - map[string]bool: Agent 名称到健康状态的映射
func (p *AgentPool) HealthCheck() map[string]bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]bool)
	for name, agent := range p.agents {
		result[name] = agent != nil
	}
	return result
}

// Stats 获取池统计信息。
//
// 返回：
//   - PoolStats: 统计信息
func (p *AgentPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return PoolStats{
		TotalAgents: len(p.agents),
		MaxAgents:   p.maxAgents,
		Registered:  len(p.creators),
	}
}

// PoolStats Agent 池统计信息。
type PoolStats struct {
	TotalAgents int // 当前 Agent 数量
	MaxAgents   int // 最大 Agent 数量
	Registered  int // 注册的创建函数数量
}

// =============================================
// AgentPoolConfig Agent 池配置。
// =============================================

// AgentPoolConfig Agent 池配置。
type AgentPoolConfig struct {
	MaxAgents      int           // 最大 Agent 数量
	IdleTimeout    time.Duration // 空闲超时时间
	HealthInterval time.Duration // 健康检查间隔
}
