package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"go-claw/internal/store"
)

// SessionInfo 会话摘要信息
type SessionInfo struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	UserID    string `json:"user_id"`
	Agent     string `json:"agent"`
	Channel   string `json:"channel"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// SessionService 会话管理服务
type SessionService struct {
	agents AgentsProvider
}

// AgentsProvider 获取 Agent 实例的接口
type AgentsProvider interface {
	GetAgent(name string) (interface{}, bool)
}

// NewSessionService 创建会话服务
func NewSessionService(agents AgentsProvider) *SessionService {
	return &SessionService{agents: agents}
}

// List 列出所有会话
func (s *SessionService) List() []SessionInfo {
	sessions := []SessionInfo{}
	dataDir := "clawdata/workspaces"

	dirs, err := os.ReadDir(dataDir)
	if err != nil {
		return sessions
	}

	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		agentName := dir.Name()
		sessDir := filepath.Join(dataDir, agentName, "sessions")

		files, err := os.ReadDir(sessDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			// 跳过目录、非 JSON 文件、memories 文件
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") || strings.HasPrefix(f.Name(), "memories") {
				continue
			}

			filePath := filepath.Join(sessDir, f.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			var sessData map[string]interface{}
			if err := json.Unmarshal(data, &sessData); err != nil {
				continue
			}

			// 会话文件必须包含 messages 字段
			if _, hasMessages := sessData["messages"]; !hasMessages {
				continue
			}

			// 提取 session ID
			sessionID, _ := sessData["session_id"].(string)
			if sessionID == "" {
				sessionID, _ = sessData["id"].(string)
			}
			if sessionID == "" {
				continue
			}

			// 提取 user_id（向后兼容）
			userID, _ := sessData["user_id"].(string)
			if userID == "" {
				parts := strings.SplitN(sessionID, ":", 2)
				if len(parts) == 2 {
					userID = parts[1]
				}
			}

			name, _ := sessData["name"].(string)
			channel, _ := sessData["channel"].(string)
			createdAt, _ := sessData["created_at"].(string)
			updatedAt, _ := sessData["updated_at"].(string)

			sessions = append(sessions, SessionInfo{
				ID:        sessionID,
				SessionID: sessionID,
				Name:      name,
				UserID:    userID,
				Agent:     agentName,
				Channel:   channel,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			})
		}
	}

	return sessions
}

// Delete 删除会话
func (s *SessionService) Delete(sessionID string) error {
	dataDir := "clawdata/workspaces"
	safeName := store.SafeFileName(sessionID) + ".json"

	dirs, err := os.ReadDir(dataDir)
	if err != nil {
		return err
	}

	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		filePath := filepath.Join(dataDir, dir.Name(), "sessions", safeName)
		os.Remove(filePath)
	}

	return nil
}

// GetHistory 获取会话历史记录
func (s *SessionService) GetHistory(sessionID, agentName string) []map[string]string {
	// 通过 agents 接口获取 agent 实例并调用其方法
	_, ok := s.agents.GetAgent(agentName)
	if !ok {
		_, ok = s.agents.GetAgent("default")
	}
	if !ok {
		return nil
	}

	// 类型断言获取具体方法（需要 Agent 接口定义）
	// 这里返回空，具体实现由 Agent 提供
	return nil
}

// GetSessionData 获取会话详细数据
func (s *SessionService) GetSessionData(sessionID string) (map[string]interface{}, error) {
	dataDir := "clawdata/workspaces"
	safeName := store.SafeFileName(sessionID) + ".json"

	dirs, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, err
	}

	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		filePath := filepath.Join(dataDir, dir.Name(), "sessions", safeName)
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var sessData map[string]interface{}
		if err := json.Unmarshal(data, &sessData); err != nil {
			continue
		}

		return sessData, nil
	}

	return nil, os.ErrNotExist
}