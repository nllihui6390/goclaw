package api

import (
	"net/http"
	"strings"
)

// HandleSessions 会话列表（GET）
func HandleSessions(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessions := sessionSvc.List()
	writeJSON(rw, http.StatusOK, sessions)
}

// HandleSessionByID 查看/删除指定会话（GET/DELETE）
func HandleSessionByID(rw http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	sessionID = strings.TrimSuffix(sessionID, "/")
	if sessionID == "" {
		writeJSON(rw, http.StatusBadRequest, map[string]string{"error": "session id required"})
		return
	}

	if r.Method == http.MethodDelete {
		err := sessionSvc.Delete(sessionID)
		if err != nil {
			writeError(rw, http.StatusInternalServerError, "delete failed")
			return
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}

	if r.Method == http.MethodGet {
		sessData, err := sessionSvc.GetSessionData(sessionID)
		if err != nil {
			writeJSON(rw, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(rw, http.StatusOK, sessData)
		return
	}

	writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
}