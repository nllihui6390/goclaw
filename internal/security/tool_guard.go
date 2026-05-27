package security

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Decision 安全决策
type Decision string

const (
	DecisionApprove Decision = "approve" // 允许执行
	DecisionGuard   Decision = "guard"   // 需要用户确认
	DecisionDeny    Decision = "deny"    // 拒绝执行
)

// GuardResult 守卫检查结果
type GuardResult struct {
	Decision Decision
	Reason   string
	Message  string // 给用户的提示消息
}

// Guardian 守卫接口
type Guardian interface {
	Check(ctx context.Context, toolName string, params map[string]interface{}) GuardResult
	Name() string
}

// ToolGuard 工具守卫引擎
type ToolGuard struct {
	guardians []Guardian
	enabled   bool
}

// NewToolGuard 创建工具守卫
func NewToolGuard() *ToolGuard {
	return &ToolGuard{
		guardians: make([]Guardian, 0),
		enabled:   true,
	}
}

// AddGuardian 添加守卫
func (tg *ToolGuard) AddGuardian(g Guardian) {
	tg.guardians = append(tg.guardians, g)
}

// SetEnabled 设置是否启用
func (tg *ToolGuard) SetEnabled(enabled bool) {
	tg.enabled = enabled
}

// Check 检查工具调用
func (tg *ToolGuard) Check(ctx context.Context, toolName string, params map[string]interface{}) GuardResult {
	if !tg.enabled {
		return GuardResult{Decision: DecisionApprove}
	}

	for _, g := range tg.guardians {
		result := g.Check(ctx, toolName, params)
		if result.Decision != DecisionApprove {
			return result
		}
	}

	return GuardResult{Decision: DecisionApprove}
}

// ShellEvasionGuardian Shell 命令注入检测守卫
type ShellEvasionGuardian struct {
	dangerousPatterns []*regexp.Regexp
}

// NewShellEvasionGuardian 创建 Shell 注入检测守卫
func NewShellEvasionGuardian() *ShellEvasionGuardian {
	patterns := []string{
		`\$\([^)]+\)`,       // $(command)
		"`[^`]+`",           // `command`
		`\|\s*\w+`,          // pipe to command
		`;\s*\w+`,           // semicolon command
		`&&\s*\w+`,          // AND command
		`\|\|\s*\w+`,        // OR command
		`>\s*/`,             // redirect to root
		`<\s*/`,             // redirect from root
		`\bchmod\s+777\b`,   // dangerous chmod
		`\brm\s+-rf\s+/\b`,  // rm -rf /
		`\bdd\s+if=`,        // dd command
		`\bmkfs\b`,          // format disk
		`\bshutdown\b`,      // shutdown
		`\breboot\b`,        // reboot
		`\binit\s+0\b`,      // init 0
	}

	compiled := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		compiled[i] = regexp.MustCompile(p)
	}

	return &ShellEvasionGuardian{dangerousPatterns: compiled}
}

func (g *ShellEvasionGuardian) Name() string {
	return "shell_evasion"
}

func (g *ShellEvasionGuardian) Check(_ context.Context, toolName string, params map[string]interface{}) GuardResult {
	if toolName != "exec" {
		return GuardResult{Decision: DecisionApprove}
	}

	command, _ := params["command"].(string)
	if command == "" {
		return GuardResult{Decision: DecisionApprove}
	}

	for _, pattern := range g.dangerousPatterns {
		if pattern.MatchString(command) {
			return GuardResult{
				Decision: DecisionGuard,
				Reason:   fmt.Sprintf("检测到潜在危险的命令模式: %s", pattern.String()),
				Message:  fmt.Sprintf("命令包含潜在危险操作: %s\n是否允许执行？", command),
			}
		}
	}

	return GuardResult{Decision: DecisionApprove}
}

// FileGuardian 文件访问守卫
type FileGuardian struct {
	protectedPaths []string
	allowedPaths   []string
}

// NewFileGuardian 创建文件访问守卫
func NewFileGuardian() *FileGuardian {
	return &FileGuardian{
		protectedPaths: []string{
			"/etc/passwd",
			"/etc/shadow",
			"/etc/hosts",
			"~/.ssh/id_rsa",
			"~/.ssh/authorized_keys",
			".env",
			"credentials",
			"secrets",
		},
		allowedPaths: []string{},
	}
}

func (g *FileGuardian) Name() string {
	return "file_access"
}

func (g *FileGuardian) Check(_ context.Context, toolName string, params map[string]interface{}) GuardResult {
	if toolName != "read_file" && toolName != "write_file" && toolName != "edit_file" && toolName != "append_file" {
		return GuardResult{Decision: DecisionApprove}
	}

	path, _ := params["path"].(string)
	if path == "" {
		return GuardResult{Decision: DecisionApprove}
	}

	pathLower := strings.ToLower(path)

	for _, protected := range g.protectedPaths {
		if strings.Contains(pathLower, strings.ToLower(protected)) {
			return GuardResult{
				Decision: DecisionDeny,
				Reason:   fmt.Sprintf("禁止访问受保护路径: %s", protected),
				Message:  fmt.Sprintf("禁止访问敏感文件: %s", path),
			}
		}
	}

	return GuardResult{Decision: DecisionApprove}
}

// RuleGuardian 基于规则的守卫
type RuleGuardian struct {
	rules []Rule
}

// Rule 安全规则
type Rule struct {
	Name      string
	ToolMatch string            // 工具名匹配（支持通配符）
	ParamCond map[string]string // 参数条件
	Action    Decision
	Reason    string
}

// NewRuleGuardian 创建规则守卫
func NewRuleGuardian() *RuleGuardian {
	return &RuleGuardian{
		rules: []Rule{
			{
				Name:      "deny_browser_automation_on_prod",
				ToolMatch: "browser_use",
				Action:    DecisionGuard,
				Reason:    "浏览器自动化操作需要确认",
			},
		},
	}
}

func (g *RuleGuardian) Name() string {
	return "rule_based"
}

func (g *RuleGuardian) Check(_ context.Context, toolName string, params map[string]interface{}) GuardResult {
	for _, rule := range g.rules {
		if rule.ToolMatch == toolName || rule.ToolMatch == "*" {
			if rule.Action == DecisionDeny {
				return GuardResult{
					Decision: DecisionDeny,
					Reason:   rule.Reason,
					Message:  fmt.Sprintf("规则 '%s' 拒绝此操作", rule.Name),
				}
			}
			if rule.Action == DecisionGuard {
				return GuardResult{
					Decision: DecisionGuard,
					Reason:   rule.Reason,
					Message:  fmt.Sprintf("规则 '%s' 需要确认: %s", rule.Name, rule.Reason),
				}
			}
		}
	}
	return GuardResult{Decision: DecisionApprove}
}

// AddRule 添加规则
func (g *RuleGuardian) AddRule(rule Rule) {
	g.rules = append(g.rules, rule)
}