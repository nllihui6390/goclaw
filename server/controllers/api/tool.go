package api

import (
	"net/http"
)

// HandleTools 返回工具列表（GET）
func HandleTools(rw http.ResponseWriter, r *http.Request) {
	tools := toolSvc.List()
	writeJSON(rw, http.StatusOK, tools)
}