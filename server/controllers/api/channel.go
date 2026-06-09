package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// HandleChannels 返回渠道列表（GET）
// Query param: agent - 指定 agent 名称（可选，默认使用 default_agent）
func HandleChannels(rw http.ResponseWriter, r *http.Request) {
	agentName := r.URL.Query().Get("agent")
	if agentName == "" {
		agentName = channelSvc.GetDefaultAgent()
	}
	channels := channelSvc.List(agentName)
	writeJSON(rw, http.StatusOK, channels)
}

// HandleChannelByID 更新指定渠道配置（PUT）
// Query param: agent - 指定 agent 名称（必需）
func HandleChannelByID(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 解析 URL: /api/v1/channels/{name}
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/channels/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		writeError(rw, http.StatusBadRequest, "channel name required")
		return
	}

	// 获取 agent 参数
	agentName := r.URL.Query().Get("agent")
	if agentName == "" {
		agentName = channelSvc.GetDefaultAgent()
	}

	var channelConfig map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&channelConfig); err != nil {
		writeError(rw, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := channelSvc.Update(agentName, name, channelConfig); err != nil {
		writeError(rw, http.StatusInternalServerError, "update failed")
		return
	}
	// 返回更新成功的响应
	writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}
