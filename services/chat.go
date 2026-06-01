package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go-claw/internal/agent"
	"go-claw/internal/channel"
)

// ChatService Wails3 对话服务
type ChatService struct {
	mu          sync.Mutex
	agents      map[string]*agent.Agent
	msgChan     chan channel.Message
	sessions    map[string]string // sessionID → last response
}

// SetAgents 注入 Agent 列表（由 main_desktop.go 调用）
func (c *ChatService) SetAgents(agents map[string]*agent.Agent) {
	c.agents = agents
}

// SendMessage 流式对话 — Wails3 自动将 chan 转为前端 AsyncGenerator
func (c *ChatService) SendMessage(sessionID, content, agentName string) chan string {
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		c.mu.Lock()
		ag := c.agents["default"]
		if agentName != "" {
			if a, ok := c.agents[agentName]; ok {
				ag = a
			}
		}
		c.mu.Unlock()

		if ag == nil {
			ch <- "Agent 未初始化，请检查配置"
			return
		}

		result, err := ag.Process(context.Background(), sessionID, content)
		if err != nil {
			ch <- fmt.Sprintf("处理出错: %v", err)
			return
		}

		// 模拟流式输出（Wails3 前端逐字显示）
		runes := []rune(result)
		for i := 0; i < len(runes); {
			end := i + 8
			if end > len(runes) {
				end = len(runes)
			}
			ch <- string(runes[i:end])
			i = end
			time.Sleep(20 * time.Millisecond)
		}
	}()
	return ch
}

// SendMessageSync 非流式对话，直接返回完整结果
func (c *ChatService) SendMessageSync(sessionID, content, agentName string) string {
	c.mu.Lock()
	ag := c.agents["default"]
	if agentName != "" {
		if a, ok := c.agents[agentName]; ok {
			ag = a
		}
	}
	c.mu.Unlock()

	if ag == nil {
		return "Agent 未初始化"
	}

	result, err := ag.Process(context.Background(), sessionID, content)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return result
}
