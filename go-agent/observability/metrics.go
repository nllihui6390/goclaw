package observability

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics 指标收集器。
//
// 收集 Agent 运行时指标，包括：
//   - 模型调用次数和耗时
//   - 工具调用次数和耗时
//   - 上下文压缩次数
//   - 会话数量
//   - 错误统计
type Metrics struct {
	registry *prometheus.Registry

	modelCallsTotal     *prometheus.CounterVec
	modelCallsLatency   *prometheus.HistogramVec
	modelCallsErrors    *prometheus.CounterVec

	toolCallsTotal      *prometheus.CounterVec
	toolCallsLatency    *prometheus.HistogramVec
	toolCallsErrors     *prometheus.CounterVec

	contextCompressions *prometheus.CounterVec
	sessionsActive      *prometheus.GaugeVec

	tokenUsageTotal     *prometheus.CounterVec
	tokenUsagePrompt    *prometheus.CounterVec
	tokenUsageCompletion *prometheus.CounterVec

	inferenceIterations *prometheus.HistogramVec

	startedAt time.Time
	requestsTotal atomic.Int64
}

// NewMetrics 创建新的指标收集器。
//
// 返回：
//   - *Metrics: 指标收集器实例
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,
		startedAt: time.Now(),
	}

	m.modelCallsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "go_agent_model_calls_total",
		Help: "Total number of model calls",
	}, []string{"agent_name", "model_name", "status"})
	reg.MustRegister(m.modelCallsTotal)

	m.modelCallsLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "go_agent_model_calls_latency_seconds",
		Help: "Latency of model calls",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
	}, []string{"agent_name", "model_name"})
	reg.MustRegister(m.modelCallsLatency)

	m.modelCallsErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "go_agent_model_calls_errors_total",
		Help: "Total number of model call errors",
	}, []string{"agent_name", "model_name", "error_type"})
	reg.MustRegister(m.modelCallsErrors)

	m.toolCallsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "go_agent_tool_calls_total",
		Help: "Total number of tool calls",
	}, []string{"agent_name", "tool_name", "status"})
	reg.MustRegister(m.toolCallsTotal)

	m.toolCallsLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "go_agent_tool_calls_latency_seconds",
		Help: "Latency of tool calls",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5},
	}, []string{"agent_name", "tool_name"})
	reg.MustRegister(m.toolCallsLatency)

	m.toolCallsErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "go_agent_tool_calls_errors_total",
		Help: "Total number of tool call errors",
	}, []string{"agent_name", "tool_name", "error_type"})
	reg.MustRegister(m.toolCallsErrors)

	m.contextCompressions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "go_agent_context_compressions_total",
		Help: "Total number of context compressions",
	}, []string{"agent_name", "trigger"})
	reg.MustRegister(m.contextCompressions)

	m.sessionsActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "go_agent_sessions_active",
		Help: "Number of active sessions",
	}, []string{"agent_name"})
	reg.MustRegister(m.sessionsActive)

	m.tokenUsageTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "go_agent_token_usage_total",
		Help: "Total token usage",
	}, []string{"agent_name", "model_name"})
	reg.MustRegister(m.tokenUsageTotal)

	m.tokenUsagePrompt = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "go_agent_token_usage_prompt",
		Help: "Prompt token usage",
	}, []string{"agent_name", "model_name"})
	reg.MustRegister(m.tokenUsagePrompt)

	m.tokenUsageCompletion = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "go_agent_token_usage_completion",
		Help: "Completion token usage",
	}, []string{"agent_name", "model_name"})
	reg.MustRegister(m.tokenUsageCompletion)

	m.inferenceIterations = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "go_agent_inference_iterations",
		Help: "Number of iterations per inference",
		Buckets: []float64{1, 2, 3, 5, 10, 20},
	}, []string{"agent_name"})
	reg.MustRegister(m.inferenceIterations)

	return m
}

// RecordModelCall 记录模型调用。
//
// 参数：
//   - agentName: Agent 名称
//   - modelName: 模型名称
//   - latency: 耗时（秒）
//   - err: 错误（nil 表示成功）
func (m *Metrics) RecordModelCall(agentName, modelName string, latency float64, err error) {
	status := "success"
	if err != nil {
		status = "error"
		m.modelCallsErrors.WithLabelValues(agentName, modelName, getErrorType(err)).Inc()
	}
	m.modelCallsTotal.WithLabelValues(agentName, modelName, status).Inc()
	m.modelCallsLatency.WithLabelValues(agentName, modelName).Observe(latency)
}

// RecordToolCall 记录工具调用。
//
// 参数：
//   - agentName: Agent 名称
//   - toolName: 工具名称
//   - latency: 耗时（秒）
//   - err: 错误（nil 表示成功）
func (m *Metrics) RecordToolCall(agentName, toolName string, latency float64, err error) {
	status := "success"
	if err != nil {
		status = "error"
		m.toolCallsErrors.WithLabelValues(agentName, toolName, getErrorType(err)).Inc()
	}
	m.toolCallsTotal.WithLabelValues(agentName, toolName, status).Inc()
	m.toolCallsLatency.WithLabelValues(agentName, toolName).Observe(latency)
}

// RecordContextCompression 记录上下文压缩。
//
// 参数：
//   - agentName: Agent 名称
//   - trigger: 触发方式（auto/manual）
func (m *Metrics) RecordContextCompression(agentName, trigger string) {
	m.contextCompressions.WithLabelValues(agentName, trigger).Inc()
}

// SetActiveSessions 设置活跃会话数。
//
// 参数：
//   - agentName: Agent 名称
//   - count: 会话数
func (m *Metrics) SetActiveSessions(agentName string, count int) {
	m.sessionsActive.WithLabelValues(agentName).Set(float64(count))
}

// RecordTokenUsage 记录 Token 使用量。
//
// 参数：
//   - agentName: Agent 名称
//   - modelName: 模型名称
//   - promptTokens: 提示词 Token 数
//   - completionTokens: 完成 Token 数
func (m *Metrics) RecordTokenUsage(agentName, modelName string, promptTokens, completionTokens int) {
	m.tokenUsageTotal.WithLabelValues(agentName, modelName).Add(float64(promptTokens + completionTokens))
	m.tokenUsagePrompt.WithLabelValues(agentName, modelName).Add(float64(promptTokens))
	m.tokenUsageCompletion.WithLabelValues(agentName, modelName).Add(float64(completionTokens))
}

// RecordInferenceIterations 记录推理迭代次数。
//
// 参数：
//   - agentName: Agent 名称
//   - iterations: 迭代次数
func (m *Metrics) RecordInferenceIterations(agentName string, iterations int) {
	m.inferenceIterations.WithLabelValues(agentName).Observe(float64(iterations))
}

// IncRequest 增加请求计数。
func (m *Metrics) IncRequest() {
	m.requestsTotal.Add(1)
}

// GetRequestCount 获取请求总数。
//
// 返回：
//   - int64: 请求总数
func (m *Metrics) GetRequestCount() int64 {
	return m.requestsTotal.Load()
}

// Handler 返回 Prometheus HTTP handler。
//
// 返回：
//   - http.Handler: HTTP handler
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// StartServer 启动指标服务器。
//
// 参数：
//   - addr: 监听地址（如 ":9090"）
func (m *Metrics) StartServer(addr string) {
	http.Handle("/metrics", m.Handler())
	go func() {
		_ = http.ListenAndServe(addr, nil)
	}()
}

// getErrorType 获取错误类型字符串。
func getErrorType(err error) string {
	if err == nil {
		return "none"
	}
	msg := err.Error()
	if contains(msg, "timeout") {
		return "timeout"
	}
	if contains(msg, "rate limit") {
		return "rate_limit"
	}
	if contains(msg, "permission") {
		return "permission"
	}
	return "unknown"
}

// contains 检查字符串是否包含子串。
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}