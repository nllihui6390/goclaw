package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// HandleAgentFiles 列出/读写 Agent 工作空间文件
func HandleAgentFiles(rw http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agent-files/")
	parts := strings.SplitN(path, "/", 2)
	agentName := parts[0]
	fileName := ""
	if len(parts) > 1 {
		fileName = parts[1]
	}

	agentDir := filepath.Join("clawdata", "workspaces", agentName)
	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		writeError(rw, http.StatusNotFound, "agent workspace not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if fileName == "" {
			files := fileSvc.List(agentName)
			writeJSON(rw, http.StatusOK, files)
		} else {
			content, err := fileSvc.Read(agentName, fileName)
			if err != nil {
				writeError(rw, http.StatusNotFound, "file not found")
				return
			}
			rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
			rw.Write([]byte(content))
		}

	case http.MethodPut:
		if fileName == "" {
			writeError(rw, http.StatusBadRequest, "filename required")
			return
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		content := body["content"]
		if err := fileSvc.Write(agentName, fileName, content); err != nil {
			writeError(rw, http.StatusInternalServerError, "write failed")
			return
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "saved"})

	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}