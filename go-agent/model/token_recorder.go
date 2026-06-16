package model

import (
	"context"
)

// TokenRecorder Token 使用量记录接口
// 实现此接口并注入到 TokenRecordingModel，每次模型调用后自动记录 token 使用量
type TokenRecorder interface {
	// Record 记录一次模型调用的 token 使用量
	Record(providerID, modelName string, inputTokens, outputTokens int)
}

// TokenRecordingModel 包装 ChatModel，在每次调用后记录 token 使用量
// 类似 CoPaw 的 TokenRecordingModelWrapper：拦截所有模型调用并记录 usage
//   - Call(): 从 Response.Usage 直接获取，准确记录
//   - Stream(): 从流式 chunk 中捕获 usage，流结束后记录
type TokenRecordingModel struct {
	inner      ChatModel
	recorder   TokenRecorder
	providerID string
}

// NewTokenRecordingModel 创建带 token 记录功能的模型包装器
func NewTokenRecordingModel(inner ChatModel, recorder TokenRecorder, providerID string) *TokenRecordingModel {
	return &TokenRecordingModel{
		inner:      inner,
		recorder:   recorder,
		providerID: providerID,
	}
}

// modelUsage 模型 usage 数据
type modelUsage struct {
	inputTokens  int
	outputTokens int
}

// Call 调用模型（非流式）并记录 usage
func (m *TokenRecordingModel) Call(ctx context.Context, messages []Msg) (*Response, error) {
	resp, err := m.inner.Call(ctx, messages)
	if err != nil {
		return nil, err
	}
	// 非流式调用直接记录 usage
	if m.recorder != nil && resp.Usage.TotalTokens > 0 {
		m.recorder.Record(m.providerID, m.GetName(), resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}
	return resp, nil
}

// Stream 调用模型（流式），从 chunk 中捕获 usage，流结束后记录
// 如果 provider 返回 stream_options.include_usage，能拿到准确值
// 如果不返回，则不记录（由 agent 层通过 ReplyEnd 聚合处理）
func (m *TokenRecordingModel) Stream(ctx context.Context, messages []Msg) (<-chan StreamChunk, error) {
	innerCh, err := m.inner.Stream(ctx, messages)
	if err != nil {
		return nil, err
	}
	outCh := make(chan StreamChunk, 100)

	go func() {
		defer close(outCh)
		var lastUsage *modelUsage

		for chunk := range innerCh {
			outCh <- chunk
			// 从流 chunk 中捕获 usage
			if chunk.InputTokens > 0 || chunk.OutputTokens > 0 {
				lastUsage = &modelUsage{chunk.InputTokens, chunk.OutputTokens}
			}
		}

		// 流结束后记录（如果捕获到了 usage）
		if lastUsage != nil && m.recorder != nil {
			m.recorder.Record(m.providerID, m.GetName(), lastUsage.inputTokens, lastUsage.outputTokens)
		}
	}()

	return outCh, nil
}

// GetName 返回模型名称
func (m *TokenRecordingModel) GetName() string {
	return m.inner.GetName()
}

// GetProvider 返回提供商名称
func (m *TokenRecordingModel) GetProvider() string {
	return m.inner.GetProvider()
}

// SetTools 设置工具定义
func (m *TokenRecordingModel) SetTools(tools []ToolDefinition) {
	if setter, ok := m.inner.(ToolSetter); ok {
		setter.SetTools(tools)
	}
}