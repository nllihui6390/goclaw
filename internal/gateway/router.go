package gateway

import (
    "regexp"
    "strings"
    "go-claw/internal/channel"
)

// RouteRule 路由规则
type RouteRule struct {
    ChannelPattern string   // 渠道名称正则
    UserPattern    string   // 用户正则
    KeywordPattern string   // 关键词正则
    AgentName      string   // 目标Agent
}

// Router 消息路由器
type Router struct {
    rules []RouteRule
    defaultAgent string
}

// NewRouter 创建路由器
func NewRouter() *Router {
    return &Router{
        rules: []RouteRule{},
        defaultAgent: "default",
    }
}

// AddRule 添加路由规则
func (r *Router) AddRule(rule RouteRule) {
    r.rules = append(r.rules, rule)
}

// SetDefaultAgent 设置默认Agent
func (r *Router) SetDefaultAgent(name string) {
    r.defaultAgent = name
}

// Route 路由消息到合适的Agent
func (r *Router) Route(msg channel.Message) string {
    for _, rule := range r.rules {
        // 匹配渠道
        if rule.ChannelPattern != "" {
            matched, _ := regexp.MatchString(rule.ChannelPattern, msg.Channel)
            if !matched {
                continue
            }
        }
        
        // 匹配用户
        if rule.UserPattern != "" {
            matched, _ := regexp.MatchString(rule.UserPattern, msg.From)
            if !matched {
                continue
            }
        }
        
        // 匹配关键词
        if rule.KeywordPattern != "" {
            if !strings.Contains(strings.ToLower(msg.Content), strings.ToLower(rule.KeywordPattern)) {
                continue
            }
        }
        
        return rule.AgentName
    }
    
    return r.defaultAgent
}