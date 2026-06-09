package api

import (
	"encoding/json"
	"net/http"

	"go-claw/global"
)

// HandleConfig 读取/保存 config.json（GET/PUT）
func HandleConfig(rw http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rw.Header().Set("Content-Type", "application/json")
		rw.Write([]byte(configSvc.GetJSON()))
		return
	}

	if r.Method == http.MethodPut {
		var config map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeError(rw, http.StatusBadRequest, "无效的JSON")
			return
		}
		if err := configSvc.Save(config); err != nil {
			writeError(rw, http.StatusInternalServerError, "保存失败")
			return
		}
		// 保存后重新加载配置（更新 global.G_CONFIG + app.Config，Agent 下次请求自动用新模型）
		global.ReloadConfigAndSyncAgents()
		writeJSON(rw, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
}

// HandleConfigReload 触发配置热重载（POST）
func HandleConfigReload(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	configSvc.Reload()
	writeJSON(rw, http.StatusOK, map[string]string{"status": "reload triggered"})
}
