package api

import (
	"encoding/json"
	"net/http"

	"go-claw/internal/security"
)

// HandleGetPendingApprovals 获取待审批列表
func HandleGetPendingApprovals(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	approvalSvc := security.GetApprovalService()
	pending := approvalSvc.ListPending()

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(pending)
}

// HandleApproveRequest 批准请求
func HandleApproveRequest(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ApprovalID string `json:"approval_id"`
		ApprovedBy string `json:"approved_by"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ApprovalID == "" {
		http.Error(rw, "approval_id is required", http.StatusBadRequest)
		return
	}

	if req.ApprovedBy == "" {
		req.ApprovedBy = "api_user"
	}

	approvalSvc := security.GetApprovalService()
	if !approvalSvc.Approve(req.ApprovalID, req.ApprovedBy) {
		http.Error(rw, "Approval not found or already resolved", http.StatusNotFound)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]string{"status": "approved"})
}

// HandleDenyRequest 拒绝请求
func HandleDenyRequest(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ApprovalID string `json:"approval_id"`
		DeniedBy   string `json:"denied_by"`
		Reason     string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ApprovalID == "" {
		http.Error(rw, "approval_id is required", http.StatusBadRequest)
		return
	}

	if req.DeniedBy == "" {
		req.DeniedBy = "api_user"
	}

	approvalSvc := security.GetApprovalService()
	if !approvalSvc.Deny(req.ApprovalID, req.DeniedBy, req.Reason) {
		http.Error(rw, "Approval not found or already resolved", http.StatusNotFound)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]string{"status": "denied"})
}
