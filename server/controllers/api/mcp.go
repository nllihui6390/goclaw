package api

import (
	"encoding/json"
	"net/http"

	"go-claw/config"
	"go-claw/global"
)

// HandleMCPList 列出指定 agent 的 MCP Server（GET）
func HandleMCPList(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	agentName := r.URL.Query().Get("agent")
	if agentName == "" {
		agentName = "default"
	}

	servers := mcpSvc.ListServers(agentName)
	// 从 MCP Manager 获取连接状态
	gw := global.GetGateway()
	if gw != nil && gw.MCPMgr != nil {
		allTools := gw.MCPMgr.ListAllTools()
		for i := range servers {
			if tools, ok := allTools[servers[i].Name]; ok {
				servers[i].Connected = true
				servers[i].ToolsCount = len(tools)
			}
		}
	}

	writeJSON(rw, http.StatusOK, servers)
}

// HandleMCPCreate 创建 MCP Server（POST）
func HandleMCPCreate(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	agentName := r.URL.Query().Get("agent")
	if agentName == "" {
		agentName = "default"
	}

	var serverConfig config.MCPServerConfig
	if err := json.NewDecoder(r.Body).Decode(&serverConfig); err != nil {
		writeError(rw, http.StatusBadRequest, "invalid json")
		return
	}

	if serverConfig.Name == "" {
		writeError(rw, http.StatusBadRequest, "name is required")
		return
	}

	if err := mcpSvc.CreateServer(agentName, serverConfig); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	// 触发配置热重载
	global.ReloadConfigAndSyncAgents()

	writeJSON(rw, http.StatusOK, map[string]string{"status": "created"})
}

// HandleMCPUpdate 更新 MCP Server（POST）
func HandleMCPUpdate(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	agentName := r.URL.Query().Get("agent")
	serverName := r.URL.Query().Get("name")
	if agentName == "" {
		agentName = "default"
	}
	if serverName == "" {
		writeError(rw, http.StatusBadRequest, "name is required")
		return
	}

	var serverConfig config.MCPServerConfig
	if err := json.NewDecoder(r.Body).Decode(&serverConfig); err != nil {
		writeError(rw, http.StatusBadRequest, "invalid json")
		return
	}

	// 保持名称一致
	serverConfig.Name = serverName

	if err := mcpSvc.UpdateServer(agentName, serverName, serverConfig); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	// 触发配置热重载
	global.ReloadConfigAndSyncAgents()

	writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}

// HandleMCPDelete 删除 MCP Server（POST）
func HandleMCPDelete(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	agentName := r.URL.Query().Get("agent")
	serverName := r.URL.Query().Get("name")
	if agentName == "" {
		agentName = "default"
	}
	if serverName == "" {
		writeError(rw, http.StatusBadRequest, "name is required")
		return
	}

	if err := mcpSvc.DeleteServer(agentName, serverName); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	// 触发配置热重载
	global.ReloadConfigAndSyncAgents()

	writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
}

// HandleMCPToggle 启用/禁用 MCP Server（POST）
func HandleMCPToggle(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	agentName := r.URL.Query().Get("agent")
	serverName := r.URL.Query().Get("name")
	if agentName == "" {
		agentName = "default"
	}
	if serverName == "" {
		writeError(rw, http.StatusBadRequest, "name is required")
		return
	}

	newEnabled, err := mcpSvc.ToggleServer(agentName, serverName)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	// 触发配置热重载
	global.ReloadConfigAndSyncAgents()

	writeJSON(rw, http.StatusOK, map[string]interface{}{
		"status":  "toggled",
		"enabled": newEnabled,
	})
}

// HandleMCPTools 列出 MCP Server 的工具（GET）
func HandleMCPTools(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	serverName := r.URL.Query().Get("name")
	if serverName == "" {
		writeError(rw, http.StatusBadRequest, "name is required")
		return
	}

	gw := global.GetGateway()
	if gw == nil || gw.MCPMgr == nil {
		writeJSON(rw, http.StatusOK, []interface{}{})
		return
	}

	tools := mcpSvc.ListTools(gw.MCPMgr, serverName)
	writeJSON(rw, http.StatusOK, tools)
}