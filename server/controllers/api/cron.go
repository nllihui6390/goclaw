package api

import (
	"encoding/json"
	"go-claw/global"
	"go-claw/internal/cron"
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
		id, err := cronSvc.Save(newJob)
		if err != nil {
			writeError(rw, http.StatusInternalServerError, "save failed")
			return
		}
		newJob.ID = id
		// 同步到调度器（cron.Manager），确保内存和文件一致
		syncCronToManager(newJob)
		writeJSON(rw, http.StatusOK, map[string]string{"status": "created", "id": id})
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
		// 优先使用调度器立即执行
		if mgr := global.GetApp().CronMgr; mgr != nil {
			mgr.RunJobNow(id)
		} else {
			cronSvc.Run(id)
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "executed"})
	case http.MethodDelete:
		if err := cronSvc.Delete(id); err != nil {
			writeError(rw, http.StatusInternalServerError, "delete failed")
			return
		}
		// 从调度器中移除
		if mgr := global.GetApp().CronMgr; mgr != nil {
			mgr.RemoveJob(id)
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
	case http.MethodPut:
		var updatedJob service.CronJob
		if err := json.NewDecoder(r.Body).Decode(&updatedJob); err != nil {
			writeError(rw, http.StatusBadRequest, "invalid JSON")
			return
		}
		updatedJob.ID = id
		if _, err := cronSvc.Save(updatedJob); err != nil {
			writeError(rw, http.StatusInternalServerError, "update failed")
			return
		}
		// 同步到调度器（cron.Manager），确保内存和文件一致
		syncCronToManager(updatedJob)
		writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// syncCronToManager 将保存的任务同步到调度器内存中，确保调度器拿到最新值
func syncCronToManager(job service.CronJob) {
	mgr := global.GetApp().CronMgr
	if mgr == nil {
		return
	}
	jobType := cron.JobTypeText
	if job.Type == "agent" {
		jobType = cron.JobTypeAgent
	}
	cronJob := &cron.Job{
		ID:          job.ID,
		Name:        job.Name,
		Type:        jobType,
		Schedule:    job.Schedule,
		Content:     job.Content,
		AgentName:   job.AgentName,
		SessionID:   job.SessionID,
		Enabled:     job.Enabled,
		LastRun:     job.LastRun,
		NextRun:     job.NextRun,
		ActiveStart: job.ActiveStart,
		ActiveEnd:   job.ActiveEnd,
	}
	if job.ID == "" {
		mgr.AddJob(cronJob)
	} else if _, err := mgr.GetJob(job.ID); err != nil {
		mgr.AddJob(cronJob)
	} else {
		mgr.UpdateJob(cronJob)
	}
}