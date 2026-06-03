package api

import (
	"net/http"
)

// HandleSkills 返回技能列表（GET）
func HandleSkills(rw http.ResponseWriter, r *http.Request) {
	skills := skillSvc.ListRaw()
	result := map[string]interface{}{
		"skill_dir": skillSvc.GetSkillDir(),
		"skills":    skills,
		"total":     len(skills),
	}
	writeJSON(rw, http.StatusOK, result)
}