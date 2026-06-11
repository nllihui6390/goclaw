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
	agents       AgentsProvider
	config       *ConfigService
	sessionIndex *store.SessionIndex
	// deleteSessionFromAgents 从所有 Agent 内存中删除会话的回调（由外部注入）
	deleteSessionFromAgents func(sessionID string)
}

// AgentsProvider 获取 Agent 实例的接口
type AgentsProvider interface {
	GetAgent(name string) (interface{}, bool)
}

// NewSessionService 创建会话服务
func NewSessionService(agents AgentsProvider, config *ConfigService) *SessionService {
	return &SessionService{agents: agents, config: config}
}

// SetSessionIndex 注入会话索引
func (s *SessionService) SetSessionIndex(idx *store.SessionIndex) {
	s.sessionIndex = idx
}

// SetDeleteSessionFromAgents 注入从 Agent 内存删除会话的回调
func (s *SessionService) SetDeleteSessionFromAgents(fn func(sessionID string)) {
	s.deleteSessionFromAgents = fn
}

// List 列出所有会话（优先从 SessionIndex 读取，降级扫描磁盘）
func (s *SessionService) List() []SessionInfo {
	// 优先从 SessionIndex 读取
	if s.sessionIndex != nil {
		entries := s.sessionIndex.List()
		result := make([]SessionInfo, 0, len(entries))
		for _, e := range entries {
			result = append(result, SessionInfo{
				ID:        e.ID,
				SessionID: e.ID,
				Name:      e.Name,
				UserID:    e.UserID,
				Agent:     e.Agent,
				Channel:   e.Channel,
				CreatedAt: e.CreatedAt,
				UpdatedAt: e.UpdatedAt,
			})
		}
		return result
	}

	// 降级：扫描磁盘
	sessions := []SessionInfo{}
	dataDir := s.config.WorkspaceBase()

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

// Delete 删除会话（磁盘文件 + 索引条目 + Agent 内存）
func (s *SessionService) Delete(sessionID string) error {
	// 删除索引条目
	if s.sessionIndex != nil {
		s.sessionIndex.Delete(sessionID)
	}

	// 清除所有 Agent 内存中的会话
	if s.deleteSessionFromAgents != nil {
		s.deleteSessionFromAgents(sessionID)
	}

	// 删除磁盘文件
	dataDir := s.config.WorkspaceBase()
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

// GetHistoryFromDisk 从磁盘直接读取会话文件（兜底方案，用于 cron 等渠道的会话）
func (s *SessionService) GetHistoryFromDisk(sessionID string) []map[string]string {
	wsBase := s.config.WorkspaceBase()
	safeName := store.SafeFileName(sessionID) + ".json"

	// 尝试读取指定文件的辅助函数
	readFile := func(filePath string) []map[string]string {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		var sess map[string]interface{}
		if err := json.Unmarshal(data, &sess); err != nil {
			return nil
		}
		msgs, _ := sess["messages"].([]interface{})
		result := make([]map[string]string, 0, len(msgs))
		for _, m := range msgs {
			msg, _ := m.(map[string]interface{})
			role, _ := msg["role"].(string)
			content, _ := msg["content"].(string)
			if role == "user" || role == "assistant" {
				result = append(result, map[string]string{"role": role, "content": content})
			}
		}
		return result
	}

	entries, err := os.ReadDir(wsBase)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessDir := filepath.Join(wsBase, entry.Name(), "sessions")

		// 1. safeName 查找（标准路径）
		if result := readFile(filepath.Join(sessDir, safeName)); len(result) > 0 {
			return result
		}

		// 2. 兜底：遍历目录下所有 JSON 文件，按 session id 匹配
		files, _ := os.ReadDir(sessDir)
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") || f.Name() == "memories.json" {
				continue
			}
			if result := readFile(filepath.Join(sessDir, f.Name())); len(result) > 0 {
				// 验证文件内容中的 id 是否匹配
				data, _ := os.ReadFile(filepath.Join(sessDir, f.Name()))
				var sess map[string]interface{}
				json.Unmarshal(data, &sess)
				if id, _ := sess["id"].(string); id == sessionID {
					return result
				}
			}
		}
	}
	return nil
}

// GetSessionData 获取会话详细数据
func (s *SessionService) GetSessionData(sessionID string) (map[string]interface{}, error) {
	dataDir := s.config.WorkspaceBase()
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