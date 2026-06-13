package token_usage

// TokenUsageStats Token 使用统计
type TokenUsageStats struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CallCount        int `json:"call_count"`
}

// TokenUsageRecord 单条 Token 使用记录（按日期 + Provider + Model）
type TokenUsageRecord struct {
	Date            string `json:"date"`             // YYYY-MM-DD
	ProviderID      string `json:"provider_id"`      // Provider ID
	Model           string `json:"model"`            // Model name
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CallCount        int    `json:"call_count"`
}

// TokenUsageByModel 按模型聚合的统计
type TokenUsageByModel struct {
	ProviderID      string `json:"provider_id"`
	Model           string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CallCount        int    `json:"call_count"`
}

// TokenUsageSummary 聚合的 Token 使用摘要
type TokenUsageSummary struct {
	TotalPromptTokens     int                          `json:"total_prompt_tokens"`
	TotalCompletionTokens int                          `json:"total_completion_tokens"`
	TotalCalls            int                          `json:"total_calls"`
	ByModel               map[string]TokenUsageByModel `json:"by_model"` // key: provider:model
	ByDate                map[string]TokenUsageStats   `json:"by_date"`  // key: YYYY-MM-DD
}

// UsageEvent Token 使用事件（放入 channel 的数据）
type UsageEvent struct {
	ProviderID      string
	ModelName       string
	PromptTokens     int
	CompletionTokens int
	DateStr         string // YYYY-MM-DD
	Timestamp       string // ISO-8601
}

// UsageEntry 存储中的条目格式
type UsageEntry struct {
	ProviderID      string `json:"provider_id"`
	ModelName       string `json:"model_name"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CallCount        int    `json:"call_count"`
}