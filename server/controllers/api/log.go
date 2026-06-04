package api

import (
	"net/http"

	"go-claw/global"
)

// HandleLogs 返回最新日志内容（GET，最多 50KB）
func HandleLogs(rw http.ResponseWriter, r *http.Request) {
	data := logSvc.Get()
	rw.Header().Set("Content-Type", "text/plain")
	rw.Write([]byte(data))
}

// HandleStatus 返回系统运行状态（GET）
func HandleStatus(rw http.ResponseWriter, r *http.Request) {
	status := statusSvc.Get()
	writeJSON(rw, http.StatusOK, status)
}

// HandleRestart 重启系统（POST）
func HandleRestart(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := global.Restart(); err != nil {
		writeJSON(rw, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"status": "restarted"})
}