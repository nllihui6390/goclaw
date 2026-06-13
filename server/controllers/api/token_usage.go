package api

import (
	"net/http"
	"strings"
	"time"
)

// HandleTokenUsage 获取 Token 使用量摘要（GET）
func HandleTokenUsage(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 解析查询参数
	query := r.URL.Query()
	startDate := query.Get("start_date")
	endDate := query.Get("end_date")
	modelName := query.Get("model")
	providerID := query.Get("provider")

	// 默认 30 天
	if startDate == "" || endDate == "" {
		now := time.Now()
		endDate = now.Format("2006-01-02")
		startDate = now.AddDate(0, 0, -30).Format("2006-01-02")
	}

	summary := tokenUsageSvc.GetSummary(startDate, endDate, modelName, providerID)
	writeJSON(rw, http.StatusOK, summary)
}

// HandleTokenUsageDetails 获取原始 Token 使用记录（GET）
func HandleTokenUsageDetails(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 解析查询参数
	query := r.URL.Query()
	startDate := query.Get("start_date")
	endDate := query.Get("end_date")
	modelName := query.Get("model")
	providerID := query.Get("provider")

	// 默认 30 天
	if startDate == "" || endDate == "" {
		now := time.Now()
		endDate = now.Format("2006-01-02")
		startDate = now.AddDate(0, 0, -30).Format("2006-01-02")
	}

	records := tokenUsageSvc.GetDetails(startDate, endDate, modelName, providerID)
	writeJSON(rw, http.StatusOK, records)
}

// parseDateRange 解析日期范围（可选辅助函数）
func parseDateRange(startDate, endDate string) (start, end time.Time) {
	var err error
	if startDate != "" {
		start, err = time.Parse("2006-01-02", startDate)
		if err != nil {
			start = time.Now().AddDate(0, 0, -30)
		}
	} else {
		start = time.Now().AddDate(0, 0, -30)
	}

	if endDate != "" {
		end, err = time.Parse("2006-01-02", endDate)
		if err != nil {
			end = time.Now()
		}
	} else {
		end = time.Now()
	}

	// 确保 start <= end
	if start.After(end) {
		start, end = end, start
	}

	return start, end
}

// HandleTokenUsageByID 占位：按 ID 查询（如需要）
func HandleTokenUsageByID(rw http.ResponseWriter, r *http.Request) {
	// 前端目前不使用此接口
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/token-usage/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeJSON(rw, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"id": id, "status": "not_implemented"})
}