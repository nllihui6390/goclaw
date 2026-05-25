package gateway

import (
	"go-claw/internal/channel"
	"strings"
)

// Router 消息路由器 — 手动指定模式
type Router struct {
	defaultAgent string
}

// NewRouter 创建路由器
func NewRouter() *Router {
	return &Router{
		defaultAgent: "default",
	}
}

// SetDefaultAgent 设置默认Agent
func (r *Router) SetDefaultAgent(name string) {
	r.defaultAgent = name
}

// Route 路由消息 — 优先使用消息中指定的Agent，否则使用默认
func (r *Router) Route(msg channel.Message) string {
	if msg.Agent != "" {
		return msg.Agent
	}
	return r.defaultAgent
}

// RouteRule 保留兼容性（已废弃，不再使用关键词路由）
type RouteRule struct {
	ChannelPattern string
	UserPattern    string
	KeywordPattern string
	AgentName      string
}

// AddRule 保留兼容性（空操作）
func (r *Router) AddRule(rule RouteRule) {
	// 废弃：不再使用自动规则路由
}

// IsCommand 判断是否为控制台命令
func IsCommand(text string) (string, string, bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	parts := strings.Fields(text[1:])
	if len(parts) == 0 {
		return "", "", false
	}
	if len(parts) == 1 {
		return parts[0], "", true
	}
	return parts[0], strings.Join(parts[1:], " "), true
}
