// Package security 提供内容级安全检查接口。
//
// 与 go-agent/tool.ToolPermissionProvider（权限门控）互补：
//   - PermissionChecker：这个工具能不能调？（粗粒度：模式+规则+只读判定）
//   - ToolGuard：这次调用的参数有没有危险？（细粒度：命令注入/敏感路径）
//
// 两者在 PermissionChecker 框架下统一协作：
//   PermissionChecker.Check() → 先跑 mode/rules → 再跑 ToolGuard 内容检查 → 汇总决定
package security

import (
	"context"
	"fmt"
	"strings"
)

// Decision 安全检查决策类型。
type Decision string

const (
	DecisionApprove Decision = "approve" // 安全，放行
	DecisionGuard   Decision = "guard"   // 有风险，需用户确认
	DecisionDeny    Decision = "deny"    // 危险，拒绝
)

// GuardResult 安全检查结果。
type GuardResult struct {
	Decision Decision // approve/guard/deny
	Reason   string   // 原因（机器可读）
	Message  string   // 给用户的提示消息（人类可读）
}

// Guardian 内容安全守卫接口。
//
// 每个 Guardian 负责检测一类威胁：
//   - ShellEvasionGuardian：命令注入 $(cmd)、反引号、chmod 777 等
//   - FilePathGuardian：敏感路径 /etc/passwd、~/.ssh/id_rsa、.env 等
//   - RuleGuardian：自定义规则匹配
//
// Guardian 与 tool.ToolPermissionProvider 的关系：
//   Guardian 作用于所有工具（全局），ToolPermissionProvider 作用于单个工具（局部）。
//   两者在 PermissionChecker 框架下协同：先 mode/rules → Guardian 内容检查 → ToolPermissionProvider。
type Guardian interface {
	// Name 返回守卫名称。
	Name() string
	// Check 检查工具调用参数是否安全。
	Check(ctx context.Context, toolName string, params map[string]interface{}) GuardResult
}

// Engine 安全守卫引擎。
//
// 管理一组 Guardian，对所有工具调用执行内容安全检查。
// 与 PermissionChecker 协作而非替代：PermissionChecker 处理权限门控，
// Engine 处理内容安检。
type Engine struct {
	guardians []Guardian
	enabled   bool
}

// NewEngine 创建安全守卫引擎。
func NewEngine(guardians ...Guardian) *Engine {
	return &Engine{
		guardians: guardians,
		enabled:   true,
	}
}

// SetEnabled 设置是否启用。
func (e *Engine) SetEnabled(v bool) { e.enabled = v }

// IsEnabled 检查是否启用。
func (e *Engine) IsEnabled() bool { return e.enabled }

// AddGuardian 添加守卫。
func (e *Engine) AddGuardian(g Guardian) {
	e.guardians = append(e.guardians, g)
}

// RemoveGuardian 移除守卫。
func (e *Engine) RemoveGuardian(name string) bool {
	for i, g := range e.guardians {
		if g.Name() == name {
			e.guardians = append(e.guardians[:i], e.guardians[i+1:]...)
			return true
		}
	}
	return false
}

// Guardians 返回守卫列表（用于 PermissionChecker 集成）。
func (e *Engine) Guardians() []Guardian { return e.guardians }

// Check 执行所有守卫检查，返回第一个非 Approve 的结果。
//
// 如果有守卫返回 Guard 或 Deny，立即返回（短路逻辑）。
// 所有守卫都 Approve 时返回 Approve。
//
// 参数：
//   - ctx: 上下文
//   - toolName: 工具名称
//   - params: 工具参数
//
// 返回：
//   - GuardResult: 汇总检查结果
func (e *Engine) Check(ctx context.Context, toolName string, params map[string]interface{}) GuardResult {
	if !e.enabled || len(e.guardians) == 0 {
		return GuardResult{Decision: DecisionApprove}
	}

	for _, g := range e.guardians {
		result := g.Check(ctx, toolName, params)
		if result.Decision != DecisionApprove {
			return result
		}
	}

	return GuardResult{Decision: DecisionApprove}
}

// FormatGuardMessage 格式化守卫消息为 LLM 可读的提示。
//
// 当工具被 Guard 或 Deny 时，将 GuardResult 格式化为
// system instruction 注入到工具结果中，引导 LLM 正确处理。
func FormatGuardMessage(result GuardResult, toolName string) string {
	switch result.Decision {
	case DecisionDeny:
		return fmt.Sprintf(
			"操作被拒绝: %s\n\n"+
				"⚠️ 该命令已被安全守卫拦截，未执行。请不要重复尝试相同的危险命令。",
			result.Message,
		)
	case DecisionGuard:
		return fmt.Sprintf(
			"操作需要确认: %s\n\n"+
				"原因: %s",
			result.Message, result.Reason,
		)
	default:
		return ""
	}
}

// ContinueSuffix 根据守卫结果决定工具结果后缀。
//
// 安全拒绝的结果需要特殊提示，避免 LLM 误解为执行成功并重复尝试。
func ContinueSuffix(resultText string) string {
	if strings.HasPrefix(resultText, "操作被拒绝:") ||
		strings.HasPrefix(resultText, "操作被用户拒绝:") {
		return "\n\n⚠️ 该命令已被安全守卫拦截，未执行。请不要重复尝试相同的危险命令。"
	}
	return "\n\n如果任务尚未完成，请继续执行。"
}

// NoOpEngine 返回一个始终放行的空引擎。
func NoOpEngine() *Engine {
	return &Engine{enabled: false}
}
