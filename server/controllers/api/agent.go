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

// HandleDeleteAgent 删除指定 Agent 配置（DELETE）
func HandleDeleteAgent(rw http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/delete/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		writeError(rw, http.StatusBadRequest, "agent name required")
		return
	}

	if name == "default" {
		writeError(rw, http.StatusForbidden, "default agent cannot be deleted")
		return
	}
	if err := agentSvc.Delete(name); err != nil {
		writeError(rw, http.StatusInternalServerError, "delete failed")
		return
	}
	// 从 gateway 中注销 agent
	// if gw := global.GetGateway(); gw != nil {
	// 	gw.UnregisterAgent(name)
	// }
	// 注销并且删除指定agent的配置文件
	global.RemoveAgentAndConfig(name)
	writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
}

// HandleUpdateAgent 更新指定 Agent 配置（PUT）
func HandleUpdateAgent(rw http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/update/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		writeError(rw, http.StatusBadRequest, "agent name required")
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
	// 热加载单个 Agent 配置（热加载）
	global.ReloadAgent(name)
	writeJSON(rw, http.StatusOK, updateData)
}

// HandleCreateAgent 创建新 Agent（POST）
func HandleCreateAgent(rw http.ResponseWriter, r *http.Request) {
	var createData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&createData); err != nil {
		writeError(rw, http.StatusBadRequest, "invalid JSON")
		return
	}

	name, _ := createData["name"].(string)
	if name == "" {
		writeError(rw, http.StatusBadRequest, "agent name required")
		return
	}

	if err := agentSvc.Create(name, createData); err != nil {
		writeError(rw, http.StatusInternalServerError, "create failed")
		return
	}
	// 热加载单个 Agent 配置（热加载）
	global.ReloadAgent(name)
	writeJSON(rw, http.StatusOK, createData)
}
