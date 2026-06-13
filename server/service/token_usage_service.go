package service

import (
	"time"

	"go-claw/internal/token_usage"
)

// TokenUsageService Token 使用量服务
type TokenUsageService struct {
	manager *token_usage.Manager
}

// NewTokenUsageService 创建 Token 使用量服务
func NewTokenUsageService(dataDir string) *TokenUsageService {
	// 初始化 token_usage 管理器
	token_usage.Init(dataDir)
	return &TokenUsageService{
		manager: token_usage.GetManager(),
	}
}

// GetDetails 获取原始 Token 使用记录
func (s *TokenUsageService) GetDetails(startDate, endDate, modelName, providerID string) []token_usage.TokenUsageRecord {
	// 默认 30 天
	if startDate == "" || endDate == "" {
		now := time.Now()
		endDate = now.Format("2006-01-02")
		startDate = now.AddDate(0, 0, -30).Format("2006-01-02")
	}
	return s.manager.GetDetails(startDate, endDate, modelName, providerID)
}

// GetSummary 获取聚合 Token 使用摘要
func (s *TokenUsageService) GetSummary(startDate, endDate, modelName, providerID string) *token_usage.TokenUsageSummary {
	// 默认 30 天
	if startDate == "" || endDate == "" {
		now := time.Now()
		endDate = now.Format("2006-01-02")
		startDate = now.AddDate(0, 0, -30).Format("2006-01-02")
	}
	return s.manager.GetSummary(startDate, endDate, modelName, providerID)
}

// Record 记录一次 LLM 调用的 Token 使用量
func (s *TokenUsageService) Record(providerID, modelName string, promptTokens, completionTokens int) {
	s.manager.Record(providerID, modelName, promptTokens, completionTokens)
}

// Stop 停止服务
func (s *TokenUsageService) Stop() {
	s.manager.Stop()
}