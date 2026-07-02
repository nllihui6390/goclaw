package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Tracer 分布式追踪器。
//
// 提供 OpenTelemetry 追踪支持，包括：
//   - 模型调用追踪
//   - 工具调用追踪
//   - 推理循环追踪
//   - 上下文压缩追踪
type Tracer struct {
	tracer trace.Tracer
}

// NewTracer 创建新的追踪器。
//
// 参数：
//   - name: 追踪器名称
//
// 返回：
//   - *Tracer: 追踪器实例
func NewTracer(name string) *Tracer {
	return &Tracer{
		tracer: otel.Tracer(name),
	}
}

// StartInferenceSpan 开始推理追踪 span。
//
// 参数：
//   - ctx: 上下文
//   - agentName: Agent 名称
//   - iteration: 迭代次数
//
// 返回：
//   - context.Context: 带追踪信息的上下文
//   - trace.Span: span 对象
func (t *Tracer) StartInferenceSpan(ctx context.Context, agentName string, iteration int) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "agent.inference",
		trace.WithAttributes(
			attribute.String("agent.name", agentName),
			attribute.Int("iteration", iteration),
		),
	)
}

// StartModelCallSpan 开始模型调用追踪 span。
//
// 参数：
//   - ctx: 上下文
//   - agentName: Agent 名称
//   - modelName: 模型名称
//   - promptTokens: 提示词 Token 数
//
// 返回：
//   - context.Context: 带追踪信息的上下文
//   - trace.Span: span 对象
func (t *Tracer) StartModelCallSpan(ctx context.Context, agentName, modelName string, promptTokens int) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "agent.model_call",
		trace.WithAttributes(
			attribute.String("agent.name", agentName),
			attribute.String("model.name", modelName),
			attribute.Int("prompt_tokens", promptTokens),
		),
	)
}

// StartToolCallSpan 开始工具调用追踪 span。
//
// 参数：
//   - ctx: 上下文
//   - agentName: Agent 名称
//   - toolName: 工具名称
//   - params: 工具参数（JSON 字符串）
//
// 返回：
//   - context.Context: 带追踪信息的上下文
//   - trace.Span: span 对象
func (t *Tracer) StartToolCallSpan(ctx context.Context, agentName, toolName, params string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "agent.tool_call",
		trace.WithAttributes(
			attribute.String("agent.name", agentName),
			attribute.String("tool.name", toolName),
			attribute.String("tool.params", params),
		),
	)
}

// StartContextCompressionSpan 开始上下文压缩追踪 span。
//
// 参数：
//   - ctx: 上下文
//   - agentName: Agent 名称
//   - messageCount: 待压缩消息数
//
// 返回：
//   - context.Context: 带追踪信息的上下文
//   - trace.Span: span 对象
func (t *Tracer) StartContextCompressionSpan(ctx context.Context, agentName string, messageCount int) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "agent.context_compression",
		trace.WithAttributes(
			attribute.String("agent.name", agentName),
			attribute.Int("message_count", messageCount),
		),
	)
}

// RecordModelResult 记录模型调用结果。
//
// 参数：
//   - span: span 对象
//   - completionTokens: 完成 Token 数
//   - hasToolCalls: 是否有工具调用
func (t *Tracer) RecordModelResult(span trace.Span, completionTokens int, hasToolCalls bool) {
	span.SetAttributes(
		attribute.Int("completion_tokens", completionTokens),
		attribute.Bool("has_tool_calls", hasToolCalls),
	)
}

// RecordToolResult 记录工具调用结果。
//
// 参数：
//   - span: span 对象
//   - success: 是否成功
//   - result: 结果摘要
func (t *Tracer) RecordToolResult(span trace.Span, success bool, result string) {
	if len(result) > 256 {
		result = result[:256] + "..."
	}
	span.SetAttributes(
		attribute.Bool("success", success),
		attribute.String("result", result),
	)
}

// RecordError 记录错误。
//
// 参数：
//   - span: span 对象
//   - err: 错误
func (t *Tracer) RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error", err.Error()))
	}
}

// StartSessionSpan 开始会话追踪 span。
//
// 参数：
//   - ctx: 上下文
//   - agentName: Agent 名称
//   - sessionID: 会话 ID
//
// 返回：
//   - context.Context: 带追踪信息的上下文
//   - trace.Span: span 对象
func (t *Tracer) StartSessionSpan(ctx context.Context, agentName, sessionID string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "agent.session",
		trace.WithAttributes(
			attribute.String("agent.name", agentName),
			attribute.String("session.id", sessionID),
		),
	)
}

// GetSpanID 获取当前 span ID。
//
// 参数：
//   - ctx: 上下文
//
// 返回：
//   - string: span ID（十六进制）
func (t *Tracer) GetSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// GetTraceID 获取当前 trace ID。
//
// 参数：
//   - ctx: 上下文
//
// 返回：
//   - string: trace ID（十六进制）
func (t *Tracer) GetTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}