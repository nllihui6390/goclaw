package agent

import (
	"context"
	"fmt"

	"github.com/nllihui6390/go-agent/tool"
)

// =============================================
// 权限系统（ 的 Permission System）
//
// 权限系统拦截 Agent 的每一次工具调用，基于三层机制决定结果：
//   1. Rules —— 显式 allow/deny/ask 模式，最高优先级
//   2. Mode —— 全局静态策略（DEFAULT/EXPLORE/ACCEPT_EDITS/BYPASS/DONT_ASK）
//   3. Built-in Checks —— 工具自身基于输入做的动态分析
//
// 各 Mode 的决策流程：
//
//	DEFAULT: DenyRules → AskRules → tool.CheckPermissions → AllowRules → ASK
//	EXPLORE: DenyRules → AskRules → CheckReadOnly → true=ALLOW, false=DENY
//	ACCEPT_EDITS: DenyRules → AskRules → CheckReadOnly(true→ALLOW) → tool.CheckPermissions → AllowRules → ASK
//	BYPASS:    DenyRules → AskRules → tool.CheckPermissions(非ASK=allow) → AllowRules → ALLOW
//	DONT_ASK:  DenyRules → AskRules(→DENY) → tool.CheckPermissions(ASK→DENY,safety→DENY) → AllowRules → DENY
// =============================================

// PermissionChecker 权限检查器接口（ 的权限引擎）。
//
// 使用 tool.PermissionAction、tool.PermissionRule、tool.PermissionContext 等类型。
type PermissionChecker interface {
	// Check 检查工具调用权限（整合 mode + rules + built-in checks）。
	//
	// 参数：
	//   - ctx: 上下文
	//   - t: 被调用的工具（可从中获取 ToolPermissionProvider）
	//   - toolInput: 工具输入参数
	//   - context: 权限上下文（mode、规则、工作目录）
	//
	// 返回：
	//   - *tool.PermissionDecision: 最终决定
	//   - error: 检查错误
	Check(ctx context.Context, t tool.Tool, toolInput map[string]interface{}, context *tool.PermissionContext) (*tool.PermissionDecision, error)

	// AddRule 添加权限规则。
	AddRule(ctx context.Context, rule tool.PermissionRule) error

	// RemoveRule 移除权限规则。
	RemoveRule(ctx context.Context, ruleID string) error

	// GetRules 获取所有规则。
	GetRules(ctx context.Context) ([]tool.PermissionRule, error)

	// AcceptRules 接受建议规则（持久化）。
	AcceptRules(ctx context.Context, rules []tool.PermissionRule) error

	// GetContext 获取当前权限上下文。
	GetContext() *tool.PermissionContext

	// SetMode 切换权限模式。
	SetMode(mode tool.PermissionMode)
}

// =============================================
// DefaultPermissionChecker — 完整权限引擎
// =============================================

// DefaultPermissionChecker 完整权限引擎（ 的 Permission Engine）。
//
// 整合 Mode + Rules + Built-in Checks 三层判断。
type DefaultPermissionChecker struct {
	context       *tool.PermissionContext
	acceptedRules []tool.PermissionRule
}

// NewDefaultPermissionChecker 创建默认权限检查器。
func NewDefaultPermissionChecker() *DefaultPermissionChecker {
	return &DefaultPermissionChecker{
		context:       tool.NewPermissionContext(),
		acceptedRules: make([]tool.PermissionRule, 0),
	}
}

// NewPermissionChecker 创建带配置的权限检查器。
//
// 参数：
//   - context: 权限上下文（mode + 规则 + 工作目录）
func NewPermissionChecker(context *tool.PermissionContext) *DefaultPermissionChecker {
	if context == nil {
		context = tool.NewPermissionContext()
	}
	return &DefaultPermissionChecker{
		context:       context,
		acceptedRules: make([]tool.PermissionRule, 0),
	}
}

// Check 完整权限检查（ 各 Mode 的决策流程图）。
//
// 决策流程：
//  1. DenyRules 匹配 → DENY（所有 mode 下都生效）
//  2. AskRules 匹配 → ASK（所有 mode 下都生效，DONT_ASK 下转 DENY）
//  3. 已接受规则匹配 → 返回规则动作
//  4. check_read_only（EXPLORE 和 ACCEPT_EDITS 下）→ ALLOW/DENY
//  5. tool.CheckPermissions（DEFAULT/ACCEPT_EDITS/BYPASS/DONT_ASK 下）
//  6. AllowRules 匹配 → ALLOW
//  7. 默认：DEFAULT→ASK, EXPLORE→DENY, ACCEPT_EDITS→ASK, BYPASS→ALLOW, DONT_ASK→DENY
//
// 参数：
//   - ctx: 上下文
//   - t: 被调用的工具
//   - toolInput: 工具输入
//   - context: 权限上下文（nil 使用默认值）
//
// 返回：
//   - *tool.PermissionDecision: 决定
//   - error: 错误
func (c *DefaultPermissionChecker) Check(ctx context.Context, t tool.Tool, toolInput map[string]interface{}, context *tool.PermissionContext) (*tool.PermissionDecision, error) {
	if context == nil {
		context = c.context
	}
	perms := tool.GetToolPermissionProvider(t)
	mode := context.Mode
	toolName := t.Name()

	// === 第1步：DenyRules（所有 mode 下始终生效） ===
	if denyRules, ok := context.DenyRules[toolName]; ok {
		for _, rule := range denyRules {
			if c.matchRule(rule, t, toolInput) {
				return &tool.PermissionDecision{Action: tool.PermissionDeny, Reason: "matched deny rule: " + rule.Description}, nil
			}
		}
	}

	// === 第2步：AskRules（所有 mode 下始终生效，DONT_ASK 下转 DENY） ===
	if askRules, ok := context.AskRules[toolName]; ok {
		for _, rule := range askRules {
			if c.matchRule(rule, t, toolInput) {
				if mode == tool.ModeDontAsk {
					return &tool.PermissionDecision{Action: tool.PermissionDeny, Reason: "ask rule converted to deny in DONT_ASK mode"}, nil
				}
				return &tool.PermissionDecision{Action: tool.PermissionAsk, Reason: "matched ask rule: " + rule.Description}, nil
			}
		}
	}

	// === 第3步：已接受规则（用户之前确认过的） ===
	for _, rule := range c.acceptedRules {
		if rule.ToolName == toolName && c.matchRule(rule, t, toolInput) {
			return &tool.PermissionDecision{Action: rule.Action, Reason: "matched accepted rule: " + rule.Description}, nil
		}
	}

	// === 第4步：Mode 特定逻辑 ===
	switch mode {
	case tool.ModeExplore:
		// EXPLORE: 只读判定 → true=ALLOW, false=DENY
		readOnly := c.isReadOnly(t, toolInput)
		if readOnly {
			return &tool.PermissionDecision{Action: tool.PermissionAllow, Reason: "read-only tool in EXPLORE mode"}, nil
		}
		return &tool.PermissionDecision{Action: tool.PermissionDeny, Reason: "non-read-only tool blocked in EXPLORE mode"}, nil

	case tool.ModeAcceptEdits:
		// ACCEPT_EDITS: 只读 → ALLOW；然后 checkPermissions；然后 AllowRules
		readOnly := c.isReadOnly(t, toolInput)
		if readOnly {
			return &tool.PermissionDecision{Action: tool.PermissionAllow, Reason: "read-only tool in ACCEPT_EDITS mode"}, nil
		}
		// 只读之外，走工具自身检查
		if perms != nil {
			dec, err := perms.CheckPermissions(ctx, toolInput, *context)
			if err != nil {
				return nil, err
			}
			if dec != nil && dec.Action != tool.PermissionPassthrough {
				if dec.Action == tool.PermissionAsk && dec.BypassImmune {
					return &tool.PermissionDecision{Action: tool.PermissionAsk, BypassImmune: true, Reason: dec.Reason}, nil
				}
				return &tool.PermissionDecision{Action: dec.Action, Reason: dec.Reason}, nil
			}
		}

	case tool.ModeBypass:
		// BYPASS: 跳过所有检查（包括 bypass_immune）。只保留 deny/ask 规则（已在上面处理）
		if perms != nil {
			dec, err := perms.CheckPermissions(ctx, toolInput, *context)
			if err != nil {
				return nil, err
			}
			if dec != nil && dec.Action == tool.PermissionDeny {
				return &tool.PermissionDecision{Action: tool.PermissionDeny, Reason: dec.Reason}, nil
			}
		}

	case tool.ModeDontAsk:
		// DONT_ASK: 工具的安全 ASK（bypass_immune）转为 DENY
		if perms != nil {
			dec, err := perms.CheckPermissions(ctx, toolInput, *context)
			if err != nil {
				return nil, err
			}
			if dec != nil && dec.Action != tool.PermissionPassthrough {
				if dec.Action == tool.PermissionAsk {
					return &tool.PermissionDecision{Action: tool.PermissionDeny, Reason: "safety ASK converted to DENY in DONT_ASK mode: " + dec.Reason}, nil
				}
				return &tool.PermissionDecision{Action: dec.Action, Reason: dec.Reason}, nil
			}
		}

	default: // ModeDefault
		if perms != nil {
			dec, err := perms.CheckPermissions(ctx, toolInput, *context)
			if err != nil {
				return nil, err
			}
			if dec != nil && dec.Action != tool.PermissionPassthrough {
				if dec.Action == tool.PermissionAllow {
					return &tool.PermissionDecision{Action: tool.PermissionAllow, Reason: dec.Reason}, nil
				}
				if dec.Action == tool.PermissionDeny {
					return &tool.PermissionDecision{Action: tool.PermissionDeny, Reason: dec.Reason}, nil
				}
				// ASK（含 bypass_immune）
				return &tool.PermissionDecision{Action: tool.PermissionAsk, BypassImmune: dec.BypassImmune, Reason: dec.Reason}, nil
			}
		}
	}

	// === 第5步：AllowRules ===
	if allowRules, ok := context.AllowRules[toolName]; ok {
		for _, rule := range allowRules {
			if c.matchRule(rule, t, toolInput) {
				return &tool.PermissionDecision{Action: tool.PermissionAllow, Reason: "matched allow rule: " + rule.Description}, nil
			}
		}
	}

	// === 第6步：默认兜底 ===
	switch mode {
	case tool.ModeExplore, tool.ModeDontAsk:
		return &tool.PermissionDecision{Action: tool.PermissionDeny, Reason: "default deny in " + string(mode) + " mode"}, nil
	case tool.ModeBypass:
		return &tool.PermissionDecision{Action: tool.PermissionAllow, Reason: "default allow in BYPASS mode"}, nil
	case tool.ModeAcceptEdits:
		// ACCEPT_EDITS: 检查工作目录
		if c.isInWorkingDirectory(toolInput, context) {
			return &tool.PermissionDecision{Action: tool.PermissionAllow, Reason: "file operation within working directory"}, nil
		}
		return &tool.PermissionDecision{Action: tool.PermissionAsk, Reason: "file operation outside working directory, confirmation required"}, nil
	default:
		// DEFAULT: 返回 ASK + 建议规则
		var rules []tool.PermissionRule
		if perms != nil {
			rules = perms.GenerateSuggestions(toolInput)
		}
		if len(rules) == 0 {
			rules = generateSimpleSuggestion(toolName, toolInput)
		}
		return &tool.PermissionDecision{Action: tool.PermissionAsk, Rules: rules, Reason: "no matching rule for tool: " + toolName}, nil
	}
}

// isReadOnly 判断工具调用是否只读。
func (c *DefaultPermissionChecker) isReadOnly(t tool.Tool, toolInput map[string]interface{}) bool {
	// 1. 工具实现了 CheckReadOnly → 动态判定
	if perms := tool.GetToolPermissionProvider(t); perms != nil {
		return perms.CheckReadOnly(toolInput)
	}
	// 2. 静态属性
	readOnly, _, _, _ := tool.GetToolProperties(t)
	return readOnly
}

// isInWorkingDirectory 检查文件操作是否在配置的工作目录内。
func (c *DefaultPermissionChecker) isInWorkingDirectory(toolInput map[string]interface{}, context *tool.PermissionContext) bool {
	path, _ := toolInput["file_path"].(string)
	if path == "" {
		return false
	}
	for _, wd := range context.WorkingDirectories {
		if contains(path, wd.Path) {
			return true
		}
	}
	return false
}

// matchRule 检查规则是否匹配工具调用。
func (c *DefaultPermissionChecker) matchRule(rule tool.PermissionRule, t tool.Tool, toolInput map[string]interface{}) bool {
	// 工具名必须匹配
	if rule.ToolName != "" && rule.ToolName != t.Name() {
		return false
	}

	// 如果工具实现了 MatchRule，交给工具自定义匹配
	if perms := tool.GetToolPermissionProvider(t); perms != nil {
		return perms.MatchRule(rule.RuleContent, toolInput)
	}

	// 默认：参数精确匹配
	if rule.RuleContent != "" {
		for _, v := range toolInput {
			if fmt.Sprintf("%v", v) == rule.RuleContent {
				return true
			}
		}
		return false
	}

	return true
}

// AddRule 添加权限规则。
func (c *DefaultPermissionChecker) AddRule(ctx context.Context, rule tool.PermissionRule) error {
	c.acceptedRules = append(c.acceptedRules, rule)
	return nil
}

// RemoveRule 移除权限规则。
func (c *DefaultPermissionChecker) RemoveRule(ctx context.Context, ruleID string) error {
	for i, rule := range c.acceptedRules {
		if rule.ID == ruleID {
			c.acceptedRules = append(c.acceptedRules[:i], c.acceptedRules[i+1:]...)
			return nil
		}
	}
	return nil
}

// GetRules 获取所有规则。
func (c *DefaultPermissionChecker) GetRules(ctx context.Context) ([]tool.PermissionRule, error) {
	return c.acceptedRules, nil
}

// AcceptRules 接受建议规则。
func (c *DefaultPermissionChecker) AcceptRules(ctx context.Context, rules []tool.PermissionRule) error {
	for _, rule := range rules {
		rule.Accepted = true
		c.acceptedRules = append(c.acceptedRules, rule)
	}
	return nil
}

// GetContext 获取权限上下文。
func (c *DefaultPermissionChecker) GetContext() *tool.PermissionContext {
	return c.context
}

// SetMode 切换权限模式。
func (c *DefaultPermissionChecker) SetMode(mode tool.PermissionMode) {
	c.context.Mode = mode
}

// contains 字符串包含检查。
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// generateSimpleSuggestion 生成简单建议规则（工具未实现 GenerateSuggestions 时使用）。
func generateSimpleSuggestion(toolName string, toolInput map[string]interface{}) []tool.PermissionRule {
	return []tool.PermissionRule{{
		ID: generateID("rule"), ToolName: toolName,
		Description: fmt.Sprintf("Allow %s with these parameters", toolName),
		Action:      tool.PermissionAllow, Source: "session",
		CreatedAt: nowISO(), Accepted: false,
	}}
}
