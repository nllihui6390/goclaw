package api

import (
	"encoding/json"
	"net/http"
)

// HandleProviders 返回 LLM 供应商列表（GET）
func HandleProviders(rw http.ResponseWriter, r *http.Request) {
	providers := providerSvc.List()
	writeJSON(rw, http.StatusOK, providers)
}

// HandleProviderTest 测试单个模型的多模态能力（POST）
func HandleProviderTest(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(rw, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Provider == "" {
		writeError(rw, http.StatusBadRequest, "provider required")
		return
	}
	result := providerSvc.TestProvider(req.Provider, req.Model)
	writeJSON(rw, http.StatusOK, result)
}

// HandleProviderTestAll 测试供应商下所有模型的多模态能力（POST）
func HandleProviderTestAll(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(rw, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Provider == "" {
		writeError(rw, http.StatusBadRequest, "provider required")
		return
	}
	results := providerSvc.TestAllModels(req.Provider)
	writeJSON(rw, http.StatusOK, results)
}