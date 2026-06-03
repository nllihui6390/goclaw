package api

import (
	"net/http"
)

// HandleProviders 返回 LLM 供应商列表（GET）
func HandleProviders(rw http.ResponseWriter, r *http.Request) {
	providers := providerSvc.List()
	writeJSON(rw, http.StatusOK, providers)
}