package service

import (
	"context"
	"encoding/json"
	"sync"

	"go-claw/internal/agent"
	"go-claw/internal/store"
	"go-claw/utils"
)

// ChatService 聊天服务
type ChatService struct {
	agents       map[string]*agent.Agent
	mu           sync.RWMutex
	sessionSvc   *SessionService
	sessionIndex *store.SessionIndex
}

// NewChatService 创建聊天服务
func NewChatService(agents map[string]*agent.Agent, sessionSvc *SessionService) *ChatService {
	return &ChatService{agents: agents, sessionSvc: sessionSvc}
}

// SetSessionIndex 注入会话索引
func (c *ChatService) SetSessionIndex(idx *store.SessionIndex) {
	c.mu.Lock()
	c.sessionIndex = idx
	c.mu.Unlock()
}

// SetSessionService 注入 SessionService（用于历史记录磁盘兜底）
func (c *ChatService) SetSessionService(s *SessionService) {
	c.mu.Lock()
	c.sessionSvc = s
	c.mu.Unlock()
}

// SetAgents 更新 Agent 实例（用于配置热重载）
func (c *ChatService) SetAgents(agents map[string]*agent.Agent) {
	c.mu.Lock()
	c.agents = agents
	c.mu.Unlock()
}

// CreateSession 创建新会话，返回 UUID 并注册到索引
func (c *ChatService) CreateSession(agentName string) string {
	id := utils.UUID()
	if agentName == "" {
		agentName = "default"
	}
	if c.sessionIndex != nil {
		c.sessionIndex.EnsureEntry(id, "console", id, agentName)
	}
	data, _ := json.Marshal(map[string]string{"session_id": id})
	return string(data)
}

// SendMessage 发送消息并返回完整响应
func (c *ChatService) SendMessage(sessionID, content, agentName string) string {
	c.mu.RLock()
	ag := c.agents["default"]
	if agentName != "" {
		if a, ok := c.agents[agentName]; ok {
			ag = a
		}
	}
	c.mu.RUnlock()

	if ag == nil {
		return "Agent 未初始化"
	}

	// 注入 channel/user 到 context（供 Session 正确记录来源）
	ctx := context.Background()
	ctx = agent.WithChannel(ctx, "console")
	ctx = agent.WithUser(ctx, sessionID)
	result, err := ag.Process(ctx, sessionID, content)
	if err != nil {
		return "Error: " + err.Error()
	}
	// 统一记录会话活动（HTTP 和 Wails 模式共用）
	if c.sessionIndex != nil {
		c.sessionIndex.RecordSession(sessionID, "console", sessionID, agentName, content)
	}
	return result
}

// GetChatHistory 获取会话历史
// 查找顺序：指定 Agent 内存 → 指定 Agent Store → 遍历所有 Agent → 磁盘兜底
func (c *ChatService) GetChatHistory(sessionID, agentName string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 从单个 Agent 提取历史的辅助函数
	tryAgent := func(ag *agent.Agent) []map[string]string {
		if ag == nil {
			return nil
		}
		// 内存
		if msgs, exists := ag.GetSessionMessages(sessionID); exists {
			result := make([]map[string]string, 0, len(msgs))
			for _, m := range msgs {
				if m.Role == "user" || m.Role == "assistant" {
					result = append(result, map[string]string{
						"role": m.Role, "content": m.Content,
					})
				}
			}
			return result
		}
		// Store
		if st := ag.GetStore(); st != nil {
			if sessData, err := st.GetSession(context.Background(), sessionID); err == nil && sessData != nil {
				result := make([]map[string]string, 0, len(sessData.Messages))
				for _, m := range sessData.Messages {
					if m.Role == "user" || m.Role == "assistant" {
						result = append(result, map[string]string{
							"role": m.Role, "content": m.Content,
						})
					}
				}
				if len(result) > 0 {
					return result
				}
			}
		}
		return nil
	}

	// 1. 先查指定 Agent
	if agentName != "" {
		if ag, ok := c.agents[agentName]; ok {
			if result := tryAgent(ag); result != nil {
				data, _ := json.Marshal(result)
				return string(data)
			}
		}
	} else {
		if ag, ok := c.agents["default"]; ok {
			if result := tryAgent(ag); result != nil {
				data, _ := json.Marshal(result)
				return string(data)
			}
		}
	}

	// 2. 遍历其他 Agent
	for name, ag := range c.agents {
		if name == agentName || (agentName == "" && name == "default") {
			continue
		}
		if result := tryAgent(ag); result != nil {
			data, _ := json.Marshal(result)
			return string(data)
		}
	}

	// 3. 磁盘兜底
	if c.sessionSvc != nil {
		if result := c.sessionSvc.GetHistoryFromDisk(sessionID); result != nil {
			data, _ := json.Marshal(result)
			return string(data)
		}
	}

	return "[]"
}