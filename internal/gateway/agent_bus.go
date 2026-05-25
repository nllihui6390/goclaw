package gateway

import (
	"sync"

	"go-claw/pkg/log"
)

// AgentEvent Agent间事件
type AgentEvent struct {
	SourceAgent string                 `json:"source"`
	TargetAgent string                 `json:"target"`
	Type        string                 `json:"type"`
	Payload     map[string]interface{} `json:"payload"`
}

// AgentEventHandler Agent事件处理器
type AgentEventHandler func(event AgentEvent)

// AgentBus Agent间消息总线
type AgentBus struct {
	mu       sync.RWMutex
	handlers map[string][]AgentEventHandler // target -> handlers
}

// NewAgentBus 创建消息总线
func NewAgentBus() *AgentBus {
	return &AgentBus{
		handlers: make(map[string][]AgentEventHandler),
	}
}

// Subscribe 订阅指定Agent的事件
func (ab *AgentBus) Subscribe(target string, handler AgentEventHandler) {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	ab.handlers[target] = append(ab.handlers[target], handler)
}

// Emit 发送事件
func (ab *AgentBus) Emit(event AgentEvent) {
	ab.mu.RLock()
	handlers := ab.handlers[event.TargetAgent]
	ab.mu.RUnlock()

	for _, h := range handlers {
		go h(event)
	}

	log.Logger().Debug("Agent事件已发送",
		"source", event.SourceAgent,
		"target", event.TargetAgent,
		"type", event.Type)
}
