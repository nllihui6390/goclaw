package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// HandleChannels 返回渠道列表（GET）
func HandleChannels(rw http.ResponseWriter, r *http.Request) {
	// gateway 已在 InitServices 中设置
	channels := channelSvc.List()
	writeJSON(rw, http.StatusOK, channels)
}

// HandleChannelByID 更新指定渠道配置（PUT）
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

	var channelConfig map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&channelConfig); err != nil {
		writeError(rw, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := channelSvc.Update(name, channelConfig); err != nil {
		writeError(rw, http.StatusInternalServerError, "update failed")
		return
	}

	writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}