package api

import (
	"encoding/json"
	"net/http"
)

// HandleSkillPool 返回全量技能池（GET）
func HandleSkillPool(rw http.ResponseWriter, r *http.Request) {
	result := skillSvc.PoolJSON()
	rw.Header().Set("Content-Type", "application/json")
	rw.Write([]byte(result))
}

// HandleSkillScan 扫描技能目录并更新技能池（POST）
func HandleSkillScan(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	reg, err := skillSvc.Scan()
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	result := map[string]interface{}{
		"skill_dir": skillSvc.GetSkillDir(),
		"skills":    reg.Skills,
		"total":     len(reg.Skills),
		"message":   "扫描完成",
	}
	writeJSON(rw, http.StatusOK, result)
}

// HandleSkillEnabled 获取或设置指定 agent 的启用技能（GET/PUT）
func HandleSkillEnabled(rw http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	if agent == "" {
		writeError(rw, http.StatusBadRequest, "agent parameter required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		result := skillSvc.GetEnabledSkillsJSON(agent)
		rw.Header().Set("Content-Type", "application/json")
		rw.Write([]byte(result))

	case http.MethodPut:
		var skills []string
		if err := json.NewDecoder(r.Body).Decode(&skills); err != nil {
			writeError(rw, http.StatusBadRequest, "invalid JSON: expected array of skill names")
			return
		}
		if err := skillSvc.SetEnabledSkills(agent, skills); err != nil {
			writeError(rw, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})

	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}