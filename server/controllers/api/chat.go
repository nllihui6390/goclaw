package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-claw/internal/agent"
)

// SetChatAgents 将 Gateway Agent 注入到 ChatService（由 main.go 调用）
func SetChatAgents(agents map[string]*agent.Agent) {
	if chatSvc != nil {
		chatSvc.SetAgents(agents)
	}
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

	// 直接委托给 ChatService（Agent 内存 → Store → 磁盘兜底，与 Wails 模式共享同一逻辑）
	result := chatSvc.GetChatHistory(sessionID, reqAgent)

	rw.Header().Set("Content-Type", "application/json")
	rw.Write([]byte(result))
}
