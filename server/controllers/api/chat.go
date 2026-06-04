package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// HandleCreateSession 创建新会话并返回 UUID（POST）
func HandleCreateSession(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body map[string]string
	json.NewDecoder(r.Body).Decode(&body)
	agentName := body["agent"]
	if agentName == "" {
		agentName = "default"
	}

	// sessionIndex 已在 InitServices 中设置
	result := chatSvc.CreateSession(agentName)
	rw.Header().Set("Content-Type", "application/json")
	rw.Write([]byte(result))
}

// HandleChatHistory 获取会话历史记录
func HandleChatHistory(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/chat/history/")
	sessionID := strings.TrimSuffix(path, "/")
	if sessionID == "" {
		json.NewEncoder(rw).Encode([]interface{}{})
		return
	}

	reqAgent := r.URL.Query().Get("agent")

	// agents/sessionIndex 已在 InitServices 中设置
	// 直接委托给 ChatService（Agent 内存 → Store → 磁盘兜底，与 Wails 模式共享同一逻辑）
	result := chatSvc.GetChatHistory(sessionID, reqAgent)

	rw.Header().Set("Content-Type", "application/json")
	rw.Write([]byte(result))
}