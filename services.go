package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"go-claw/internal/agent"
)

// ─────────── Wails3 Services ───────────

// ChatService 对话服务，前端通过 Wails3 bridge 直接调用 Go 函数
type ChatService struct {
	mu     sync.Mutex
	agents map[string]*agent.Agent
}

// SetAgents 注入 Agent 实例
func (c *ChatService) SetAgents(agents map[string]*agent.Agent) {
	c.agents = agents
}

// SendMessage 对话接口，返回完整响应（Wails3 桌面模式）
func (c *ChatService) SendMessage(sessionID, content, agentName string) string {
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

// GetChatHistory 获取指定会话的历史消息
func (c *ChatService) GetChatHistory(sessionID string) string {
	c.mu.Lock()
	ag := c.agents["default"]
	c.mu.Unlock()

	if ag == nil {
		return "[]"
	}

	// 先尝试从内存中的 SessionManager 获取
	msgs, exists := ag.GetSessionMessages(sessionID)
	if exists {
		result := make([]map[string]string, 0, len(msgs))
		for _, m := range msgs {
			if m.Role == "user" || m.Role == "assistant" {
				result = append(result, map[string]string{
					"role":    m.Role,
					"content": m.Content,
				})
			}
		}
		data, _ := json.Marshal(result)
		return string(data)
	}

	// 从 Store 文件加载
	st := ag.GetStore()
	if st == nil {
		return "[]"
	}

	sessData, err := st.GetSession(context.Background(), sessionID)
	if err != nil || sessData == nil {
		return "[]"
	}

	result := make([]map[string]string, 0, len(sessData.Messages))
	for _, m := range sessData.Messages {
		if m.Role == "user" || m.Role == "assistant" {
			result = append(result, map[string]string{
				"role":    m.Role,
				"content": m.Content,
			})
		}
	}
	data, _ := json.Marshal(result)
	return string(data)
}

// AppService 管理服务
type AppService struct{}

func (a *AppService) GetConfig() string {
	data, _ := os.ReadFile("config.json")
	return string(data)
}

// GetLogs 返回最新日志内容（最多 50KB）
func (a *AppService) GetLogs() string {
	data, err := os.ReadFile("logs/app.log")
	if err != nil {
		return "暂无日志"
	}
	if len(data) > 50000 {
		data = data[len(data)-50000:]
	}
	return string(data)
}

// GetStatus 返回系统运行状态
func (a *AppService) GetStatus() string {
	status := map[string]string{
		"status": "running",
	}
	data, _ := json.Marshal(status)
	return string(data)
}

// GetAgents 返回 Agent 列表（从 config.json 读取）
func (a *AppService) GetAgents() string {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return "[]"
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "[]"
	}
	agents, _ := cfg["agents"].([]interface{})
	result, _ := json.Marshal(agents)
	return string(result)
}

// GetSessions 返回会话列表（扫描文件目录）
func (a *AppService) GetSessions() string {
	sessions := []map[string]string{}
	dataDir := "clawdata/workspaces"
	if dirs, err := os.ReadDir(dataDir); err == nil {
		for _, dir := range dirs {
			if dir.IsDir() {
				sessDir := dataDir + "/" + dir.Name() + "/sessions"
				if files, err := os.ReadDir(sessDir); err == nil {
					for _, f := range files {
						if strings.HasSuffix(f.Name(), ".json") && f.Name() != "memories.json" {
							sessionID := strings.TrimSuffix(f.Name(), ".json")
							sessions = append(sessions, map[string]string{
								"id":    sessionID,
								"agent": dir.Name(),
							})
						}
					}
				}
			}
		}
	}
	result, _ := json.Marshal(sessions)
	return string(result)
}

// DeleteSession 删除指定会话文件
func (a *AppService) DeleteSession(sessionID string) string {
	dataDir := "clawdata/workspaces"
	if dirs, err := os.ReadDir(dataDir); err == nil {
		for _, dir := range dirs {
			if dir.IsDir() {
				filePath := dataDir + "/" + dir.Name() + "/sessions/" + sessionID + ".json"
				os.Remove(filePath)
			}
		}
	}
	return `{"status":"deleted"}`
}