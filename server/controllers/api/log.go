package api

import (
	"net/http"
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