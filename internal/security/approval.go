package security

import (
	"context"
	"sync"
	"time"
)

// ApprovalStatus 审批状态
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalDenied   ApprovalStatus = "denied"
	ApprovalExpired  ApprovalStatus = "expired"
)

// PendingApproval 待审批的请求
type PendingApproval struct {
	ID          string                 `json:"id"`
	ToolName    string                 `json:"tool_name"`
	Params      map[string]interface{} `json:"params"`
	Reason      string                 `json:"reason"`
	Message     string                 `json:"message"`
	Status      ApprovalStatus         `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
	ResolvedBy  string                 `json:"resolved_by,omitempty"`
	DenyReason  string                 `json:"deny_reason,omitempty"`
	resultChan  chan ApprovalResult    `json:"-"`
}

// ApprovalResult 审批结果
type ApprovalResult struct {
	Approved   bool
	DenyReason string
}

// ApprovalService 审批服务（全局单例）
type ApprovalService struct {
	mu       sync.RWMutex
	pending  map[string]*PendingApproval
	timeout  time.Duration
}

var (
	globalApprovalService *ApprovalService
	approvalOnce          sync.Once
)

// GetApprovalService 获取全局审批服务
func GetApprovalService() *ApprovalService {
	approvalOnce.Do(func() {
		globalApprovalService = &ApprovalService{
			pending: make(map[string]*PendingApproval),
			timeout: 5 * time.Minute, // 默认5分钟超时
		}
	})
	return globalApprovalService
}

// CreateApproval 创建新的审批请求
func (s *ApprovalService) CreateApproval(id, toolName string, params map[string]interface{}, reason, message string) *PendingApproval {
	s.mu.Lock()
	defer s.mu.Unlock()

	approval := &PendingApproval{
		ID:         id,
		ToolName:   toolName,
		Params:     params,
		Reason:     reason,
		Message:    message,
		Status:     ApprovalPending,
		CreatedAt:  time.Now(),
		resultChan: make(chan ApprovalResult, 1),
	}
	s.pending[id] = approval
	return approval
}

// GetApproval 获取审批请求
func (s *ApprovalService) GetApproval(id string) (*PendingApproval, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.pending[id]
	return a, ok
}

// ListPending 列出所有待审批请求
func (s *ApprovalService) ListPending() []*PendingApproval {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*PendingApproval, 0)
	for _, a := range s.pending {
		if a.Status == ApprovalPending {
			result = append(result, a)
		}
	}
	return result
}

// Approve 批准审批请求
func (s *ApprovalService) Approve(id, resolvedBy string) bool {
	s.mu.Lock()
	a, ok := s.pending[id]
	if !ok || a.Status != ApprovalPending {
		s.mu.Unlock()
		return false
	}

	now := time.Now()
	a.Status = ApprovalApproved
	a.ResolvedAt = &now
	a.ResolvedBy = resolvedBy
	s.mu.Unlock()

	// 发送结果
	a.resultChan <- ApprovalResult{Approved: true}
	return true
}

// Deny 拒绝审批请求
func (s *ApprovalService) Deny(id, resolvedBy, reason string) bool {
	s.mu.Lock()
	a, ok := s.pending[id]
	if !ok || a.Status != ApprovalPending {
		s.mu.Unlock()
		return false
	}

	now := time.Now()
	a.Status = ApprovalDenied
	a.ResolvedAt = &now
	a.ResolvedBy = resolvedBy
	a.DenyReason = reason
	s.mu.Unlock()

	// 发送结果
	a.resultChan <- ApprovalResult{Approved: false, DenyReason: reason}
	return true
}

// WaitForResult 等待审批结果（阻塞）
func (s *ApprovalService) WaitForResult(ctx context.Context, approval *PendingApproval) (ApprovalResult, error) {
	select {
	case result := <-approval.resultChan:
		return result, nil
	case <-ctx.Done():
		// 超时或取消
		s.mu.Lock()
		if approval.Status == ApprovalPending {
			now := time.Now()
			approval.Status = ApprovalExpired
			approval.ResolvedAt = &now
		}
		s.mu.Unlock()
		return ApprovalResult{Approved: false, DenyReason: "审批超时"}, ctx.Err()
	case <-time.After(s.timeout):
		// 超时
		s.mu.Lock()
		if approval.Status == ApprovalPending {
			now := time.Now()
			approval.Status = ApprovalExpired
			approval.ResolvedAt = &now
		}
		s.mu.Unlock()
		return ApprovalResult{Approved: false, DenyReason: "审批超时"}, nil
	}
}

// Remove 移除审批请求（清理用）
func (s *ApprovalService) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, id)
}

// Cleanup 清理过期的审批请求
func (s *ApprovalService) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	now := time.Now()
	for id, a := range s.pending {
		if a.Status == ApprovalPending && now.Sub(a.CreatedAt) > s.timeout {
			a.Status = ApprovalExpired
			a.ResolvedAt = &now
			close(a.resultChan)
			delete(s.pending, id)
			count++
		} else if a.Status != ApprovalPending {
			// 已解决的请求也清理
			delete(s.pending, id)
			count++
		}
	}
	return count
}

// SetTimeout 设置超时时间
func (s *ApprovalService) SetTimeout(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeout = d
}
