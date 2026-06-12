package api

import (
	"encoding/json"
	"net/http"

	"go-claw/config"
)

// HandleGetEnvVars GET /api/v1/getEnvVars - 列出所有环境变量
func HandleGetEnvVars(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	data := envVarSvc.ListWithSource()
	writeJSON(rw, http.StatusOK, data)
}

// HandleCreateEnvVars POST /api/v1/createEnvVars - 添加新环境变量
func HandleCreateEnvVars(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var entry config.EnvVarEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		writeError(rw, http.StatusBadRequest, "无效的JSON")
		return
	}

	if entry.Key == "" {
		writeError(rw, http.StatusBadRequest, "key 不能为空")
		return
	}

	if err := envVarSvc.Save(entry); err != nil {
		writeError(rw, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, map[string]string{"status": "ok", "key": entry.Key})
}

// HandleUpdateEnvVars POST /api/v1/updateEnvVars - 更新指定环境变量
func HandleUpdateEnvVars(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var entry config.EnvVarEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		writeError(rw, http.StatusBadRequest, "无效的JSON")
		return
	}

	if entry.Key == "" {
		writeError(rw, http.StatusBadRequest, "key 不能为空")
		return
	}

	if err := envVarSvc.Update(entry); err != nil {
		writeError(rw, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleDeleteEnvVars POST /api/v1/deleteEnvVars - 删除指定环境变量
func HandleDeleteEnvVars(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(rw, http.StatusBadRequest, "无效的JSON")
		return
	}

	if req.Key == "" {
		writeError(rw, http.StatusBadRequest, "key 不能为空")
		return
	}

	if err := envVarSvc.Delete(req.Key); err != nil {
		writeError(rw, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleReloadEnvVars POST /api/v1/reloadEnvVars - 强制重新加载环境变量
func HandleReloadEnvVars(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := envVarSvc.ReloadEnvVarsFile(); err != nil {
		writeError(rw, http.StatusInternalServerError, "重载失败")
		return
	}

	writeJSON(rw, http.StatusOK, map[string]string{"status": "ok"})
}