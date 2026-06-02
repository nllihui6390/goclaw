package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

// handleConfig 读取/保存 config.json（GET/PUT）
func handleConfig(rw http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data, err := os.ReadFile("config.json")
		if err != nil {
			writeError(rw, http.StatusInternalServerError, "读取配置失败")
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Write(data)
		return
	}
	if r.Method == http.MethodPut {
		var config map[string]any
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeError(rw, http.StatusBadRequest, "无效的JSON")
			return
		}
		data, _ := json.MarshalIndent(config, "", "  ")
		if err := os.WriteFile("config.json", data, 0644); err != nil {
			writeError(rw, http.StatusInternalServerError, "保存失败")
			return
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
}

// handleConfigReload 触发配置热重载（POST）
func handleConfigReload(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"status": "reload triggered"})
}

// handleAgents 返回 Agent 列表（GET）
func handleAgents(rw http.ResponseWriter, r *http.Request) {
	agents := []map[string]any{
		{"name": "default", "provider": "", "model": "", "tools": []string{}, "status": "running"},
	}
	writeJSON(rw, http.StatusOK, agents)
}

// handleAgentByID 更新指定 Agent 配置（PUT）
func handleAgentByID(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}

// handleChannels 返回渠道列表及连接状态（GET）
func handleChannels(rw http.ResponseWriter, r *http.Request) {
	channels := []map[string]any{
		{"name": "console", "type": "console", "enabled": true, "status": "connected"},
		{"name": "lark", "type": "lark", "enabled": false, "status": "disconnected"},
		{"name": "dingtalk", "type": "dingtalk", "enabled": false, "status": "disconnected"},
		{"name": "wecom", "type": "wecom", "enabled": false, "status": "disconnected"},
		{"name": "wechat", "type": "wechat", "enabled": false, "status": "disconnected"},
	}
	writeJSON(rw, http.StatusOK, channels)
}

// handleChannelByID 更新指定渠道配置（PUT）
func handleChannelByID(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}

// handleProviders 返回 LLM 供应商列表（GET）
func handleProviders(rw http.ResponseWriter, r *http.Request) {
	writeJSON(rw, http.StatusOK, []map[string]any{})
}

// handleTools 返回工具列表（GET）
func handleTools(rw http.ResponseWriter, r *http.Request) {
	writeJSON(rw, http.StatusOK, []map[string]any{})
}

// handleSkills 返回 Skill 技能列表（GET）
func handleSkills(rw http.ResponseWriter, r *http.Request) {
	writeJSON(rw, http.StatusOK, []map[string]any{})
}

// handleCronJobs 定时任务列表（GET）/ 添加任务（POST）
func handleCronJobs(rw http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		writeJSON(rw, http.StatusOK, map[string]string{"status": "created"})
		return
	}
	writeJSON(rw, http.StatusOK, []map[string]any{})
}

// handleCronJobByID 更新/删除/立即执行定时任务（PUT/DELETE/POST）
func handleCronJobByID(rw http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/cron/jobs/"), "/")
	if len(parts) > 1 && parts[0] != "" {
		if parts[1] == "run" {
			writeJSON(rw, http.StatusOK, map[string]string{"status": "executed"})
			return
		}
	}
	switch r.Method {
	case http.MethodPut:
		writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleLogs 返回最新日志内容（GET，最多 50KB）
func handleLogs(rw http.ResponseWriter, r *http.Request) {
	data, _ := os.ReadFile("logs/app.log")
	if len(data) > 50000 {
		data = data[len(data)-50000:]
	}
	rw.Header().Set("Content-Type", "text/plain")
	rw.Write(data)
}

// handleStatus 返回系统运行状态（GET）
func handleStatus(rw http.ResponseWriter, r *http.Request) {
	writeJSON(rw, http.StatusOK, map[string]any{
		"status": "running",
		"uptime": startTime.Format(time.RFC3339),
	})
}

// handleSessions 返回会话列表（GET）
func handleSessions(rw http.ResponseWriter, r *http.Request) {
	writeJSON(rw, http.StatusOK, []map[string]any{})
}

// handleSessionByID 查看/删除指定会话（GET/DELETE）
func handleSessionByID(rw http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	writeJSON(rw, http.StatusOK, map[string]any{})
}
