package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-claw/server/service"
)

// HandleCronJobs 定时任务列表（GET）/ 添加任务（POST）
func HandleCronJobs(rw http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobs := cronSvc.List()
		writeJSON(rw, http.StatusOK, jobs)
	case http.MethodPost:
		var newJob service.CronJob
		if err := json.NewDecoder(r.Body).Decode(&newJob); err != nil {
			writeError(rw, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := cronSvc.Save(newJob); err != nil {
			writeError(rw, http.StatusInternalServerError, "save failed")
			return
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "created"})
	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleCronJobByID 更新/删除/立即执行定时任务（PUT/DELETE/POST）
func HandleCronJobByID(rw http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/cron/jobs/"), "/")
	id := parts[0]
	if id == "" {
		writeError(rw, http.StatusBadRequest, "job id required")
		return
	}

	switch r.Method {
	case http.MethodPost:
		// 异步立即执行定时任务
		cronSvc.Run(id)
		writeJSON(rw, http.StatusOK, map[string]string{"status": "executed"})
	case http.MethodDelete:
		if err := cronSvc.Delete(id); err != nil {
			writeError(rw, http.StatusInternalServerError, "delete failed")
			return
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
	case http.MethodPut:
		var updatedJob service.CronJob
		if err := json.NewDecoder(r.Body).Decode(&updatedJob); err != nil {
			writeError(rw, http.StatusBadRequest, "invalid JSON")
			return
		}
		updatedJob.ID = id
		if err := cronSvc.Save(updatedJob); err != nil {
			writeError(rw, http.StatusInternalServerError, "update failed")
			return
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}