package agent

import (
	"context"
	"fmt"
	"sync"
)

// RouterRule 路由规则。
//
// 定义如何将请求路由到特定的 Agent。
type RouterRule struct {
	Name     string             // 规则名称
	Match    func(msg Msg) bool // 匹配函数
	Target   string             // 目标 Agent 名称
	Priority int                // 优先级（数字越大优先级越高）
}

// AgentRouter Agent 路由器。
//
// 根据预定义规则将请求路由到合适的 Agent。
// 支持优先级匹配、加权路由和故障转移。
type AgentRouter struct {
	pool          *AgentPool
	rules         []RouterRule
	defaultTarget string
	mu            sync.RWMutex
}

// NewAgentRouter 创建新的 Agent 路由器。
//
// 参数：
//   - pool: Agent 池
//   - defaultTarget: 默认目标 Agent 名称
//
// 返回：
//   - *AgentRouter: 路由器实例
func NewAgentRouter(pool *AgentPool, defaultTarget string) *AgentRouter {
	return &AgentRouter{
		pool:          pool,
		rules:         make([]RouterRule, 0),
		defaultTarget: defaultTarget,
	}
}

// AddRule 添加路由规则。
//
// 参数：
//   - rule: 路由规则
func (r *AgentRouter) AddRule(rule RouterRule) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rules = append(r.rules, rule)
	r.sortRules()
}

// RemoveRule 移除指定名称的路由规则。
//
// 参数：
//   - name: 规则名称
func (r *AgentRouter) RemoveRule(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, rule := range r.rules {
		if rule.Name == name {
			r.rules = append(r.rules[:i], r.rules[i+1:]...)
			break
		}
	}
}

// Route 根据消息路由到合适的 Agent。
//
// 参数：
//   - msg: 消息
//
// 返回：
//   - *Agent: 目标 Agent（未找到返回 nil）
//   - string: 路由规则名称
func (r *AgentRouter) Route(msg Msg) (*Agent, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, rule := range r.rules {
		if rule.Match(msg) {
			agent := r.pool.Get(rule.Target)
			if agent != nil {
				return agent, rule.Name
			}
		}
	}

	if r.defaultTarget != "" {
		return r.pool.Get(r.defaultTarget), "default"
	}

	return nil, ""
}

// RouteOrCreate 根据消息路由到合适的 Agent，不存在则创建。
//
// 参数：
//   - msg: 消息
//
// 返回：
//   - *Agent: 目标 Agent
//   - string: 路由规则名称
//   - error: 创建错误
func (r *AgentRouter) RouteOrCreate(msg Msg) (*Agent, string, error) {
	r.mu.RLock()
	rules := make([]RouterRule, len(r.rules))
	copy(rules, r.rules)
	defaultTarget := r.defaultTarget
	r.mu.RUnlock()

	for _, rule := range rules {
		if rule.Match(msg) {
			agent, err := r.pool.GetOrCreate(rule.Target)
			if err != nil {
				return nil, "", err
			}
			return agent, rule.Name, nil
		}
	}

	if defaultTarget != "" {
		agent, err := r.pool.GetOrCreate(defaultTarget)
		if err != nil {
			return nil, "", err
		}
		return agent, "default", nil
	}

	return nil, "", fmt.Errorf("no matching rule and no default target")
}

// Reply 通过路由器转发消息并获取同步回复。
//
// 参数：
//   - ctx: 上下文
//   - msg: 消息
//
// 返回：
//   - *Msg: 回复消息
//   - error: 错误
func (r *AgentRouter) Reply(ctx context.Context, msg Msg) (*Msg, error) {
	agent, _, err := r.RouteOrCreate(msg)
	if err != nil {
		return nil, err
	}

	return agent.Reply(ctx, msg)
}

// ReplyStream 通过路由器转发消息并获取流式回复。
//
// 参数：
//   - ctx: 上下文
//   - msg: 消息
//
// 返回：
//   - <-chan interface{}: 事件流 channel
func (r *AgentRouter) ReplyStream(ctx context.Context, msg Msg) (<-chan interface{}, error) {
	agent, _, err := r.RouteOrCreate(msg)
	if err != nil {
		return nil, err
	}

	return agent.ReplyStream(ctx, msg)
}

// ListRules 获取所有路由规则。
//
// 返回：
//   - []RouterRule: 规则列表
func (r *AgentRouter) ListRules() []RouterRule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rules := make([]RouterRule, len(r.rules))
	copy(rules, r.rules)
	return rules
}

// ClearRules 清空所有路由规则。
func (r *AgentRouter) ClearRules() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rules = make([]RouterRule, 0)
}

// sortRules 按优先级排序规则。
func (r *AgentRouter) sortRules() {
	for i := 0; i < len(r.rules)-1; i++ {
		for j := i + 1; j < len(r.rules); j++ {
			if r.rules[j].Priority > r.rules[i].Priority {
				r.rules[i], r.rules[j] = r.rules[j], r.rules[i]
			}
		}
	}
}

// =============================================
// 预定义路由规则工厂
// =============================================

// RuleByKeyword 创建基于关键词的路由规则。
//
// 参数：
//   - name: 规则名称
//   - keywords: 关键词列表
//   - target: 目标 Agent 名称
//   - priority: 优先级
//
// 返回：
//   - RouterRule: 路由规则
func RuleByKeyword(name string, keywords []string, target string, priority int) RouterRule {
	return RouterRule{
		Name: name,
		Match: func(msg Msg) bool {
			text := msg.GetTextContent()
			for _, kw := range keywords {
				if containsIgnoreCase(text, kw) {
					return true
				}
			}
			return false
		},
		Target:   target,
		Priority: priority,
	}
}

// RuleByUser 创建基于用户 ID 的路由规则。
//
// 参数：
//   - name: 规则名称
//   - userID: 用户 ID
//   - target: 目标 Agent 名称
//   - priority: 优先级
//
// 返回：
//   - RouterRule: 路由规则
func RuleByUser(name string, userID string, target string, priority int) RouterRule {
	return RouterRule{
		Name: name,
		Match: func(msg Msg) bool {
			return msg.Name == userID
		},
		Target:   target,
		Priority: priority,
	}
}

// RuleByRole 创建基于角色的路由规则。
//
// 参数：
//   - name: 规则名称
//   - role: 角色
//   - target: 目标 Agent 名称
//   - priority: 优先级
//
// 返回：
//   - RouterRule: 路由规则
func RuleByRole(name string, role Role, target string, priority int) RouterRule {
	return RouterRule{
		Name: name,
		Match: func(msg Msg) bool {
			return msg.Role == role
		},
		Target:   target,
		Priority: priority,
	}
}

// RuleAlways 创建始终匹配的路由规则。
//
// 参数：
//   - name: 规则名称
//   - target: 目标 Agent 名称
//   - priority: 优先级
//
// 返回：
//   - RouterRule: 路由规则
func RuleAlways(name string, target string, priority int) RouterRule {
	return RouterRule{
		Name: name,
		Match: func(msg Msg) bool {
			return true
		},
		Target:   target,
		Priority: priority,
	}
}

// containsIgnoreCase 忽略大小写检查字符串是否包含子串。
func containsIgnoreCase(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

// equalIgnoreCase 忽略大小写比较两个字符串。
func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if toLower(a[i]) != toLower(b[i]) {
			return false
		}
	}
	return true
}

// toLower 将字符转换为小写。
func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}
