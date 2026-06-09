package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-claw/global"
)

// HandleAgents 返回 Agent 列表（GET）
func HandleAgents(rw http.ResponseWriter, r *http.Request) {
	agents := agentSvc.List()
	writeJSON(rw, http.StatusOK, agents)
}

// HandleAgentByID 更新/删除指定 Agent 配置（PUT/DELETE）
func HandleAgentByID(rw http.ResponseWriter, r *http.Request) {
	// 解析 URL: /api/v1/agents/{name}
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		writeError(rw, http.StatusBadRequest, "agent name required")
		return
	}

	if r.Method == http.MethodDelete {
		if name == "default" {
			writeError(rw, http.StatusForbidden, "default agent cannot be deleted")
			return
		}
		if err := agentSvc.Delete(name); err != nil {
			writeError(rw, http.StatusInternalServerError, "delete failed")
			return
		}
		// 从 gateway 中注销 agent
		if gw := global.GetGateway(); gw != nil {
			gw.UnregisterAgent(name)
		}
		global.ReloadConfig()
		writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}

	if r.Method != http.MethodPut {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var updateData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		writeError(rw, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := agentSvc.Update(name, updateData); err != nil {
		writeError(rw, http.StatusInternalServerError, "update failed")
		return
	}
	global.ReloadConfig()
	writeJSON(rw, http.StatusOK, updateData)
}