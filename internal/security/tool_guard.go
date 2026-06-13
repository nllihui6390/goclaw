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
		// ── Shell 注入模式 ──
		`\$\([^)]+\)`,       // $(command)
		"`[^`]+`",           // `command`

		// ── Linux 危险命令 ──
		`\bchmod\s+777\b`,   // chmod 777（全开放权限）
		`\brm\s+-rf\s+/\b`,  // rm -rf /
		`\bdd\s+if=`,        // dd（磁盘操作）
		`\bmkfs\b`,          // 格式化磁盘
		`\bshutdown\b`,      // 关机
		`\breboot\b`,        // 重启
		`\binit\s+0\b`,      // init 0（关机）
		`\bpoweroff\b`,      // poweroff
		`\bhalt\b`,          // halt

		// ── Windows 危险命令 ──
		`\bicacls\b.*\bEveryone:F\b`,        // icacls Everyone:F（= chmod 777）
		`\bicacls\b.*\b/grant\b.*:F\b`,      // icacls /grant ...:F（完全控制）
		`\bformat\b\s+[A-Za-z]:`,            // format C:（格式化磁盘）
		`\bshutdown\b\s+/s`,                  // shutdown /s（Windows关机）
		`\bshutdown\b\s+/r`,                  // shutdown /r（Windows重启）
		`\bnet\s+user\b`,                     // net user（用户操作）
		`\bnet\s+localgroup\b`,               // net localgroup（组操作）
		`\breg\s+(add|delete|import)\b`,      // 注册表操作
		`\btaskkill\b\s+/f\b`,                // taskkill /f（强制杀进程）
		`\bsdelete\b`,                        // 安全删除工具
		`\bcd\b.*\b\\Windows\\System32\b`,    // 进入 System32
		`\bdel\b\s+/[sq]\b.*\b\\`,            // del /s /q（批量静默删除）
		`\brd\b\s+/[sq]\b.*\b\\`,             // rd /s /q（批量静默删除目录）
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
	// 支持两种工具名：注册名 "exec" 和实际工具名 "execute_command"
	if toolName != "exec" && toolName != "execute_command" {
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