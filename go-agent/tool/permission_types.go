package tool

// =============================================
// 权限基础类型（tool 包定义，agent 和 tool 共享）
//
// 定义在 tool 包避免 agent ↔ tool 循环导入。
// Agent 的 PermissionChecker 在此基础上构建。
// =============================================

// PermissionAction 权限动作枚举（ 的 PermissionBehavior）。
type PermissionAction string

const (
	PermissionAllow       PermissionAction = "ALLOW"       // 允许执行
	PermissionDeny        PermissionAction = "DENY"        // 拒绝执行
	PermissionAsk         PermissionAction = "ASK"         // 需要用户确认
	PermissionExternal    PermissionAction = "EXTERNAL"    // 外部执行
	PermissionPassthrough PermissionAction = "PASSTHROUGH" // 交给引擎继续评估
)

// PermissionRule 权限规则。
//
// 定义某个工具在什么条件下允许/拒绝/确认。
// 规则有两种来源：静态预配置（userSettings/projectSettings），
// 或用户在 ASK 提示中接受建议规则而动态加入。
//
// 字段：
//   - ID: 规则唯一标识
//   - ToolName: 适用的工具名称
//   - RuleContent: 匹配模式（语义随 ToolName 变化）
//   - Action: 权限动作
//   - Source: 规则来源（"userSettings", "projectSettings", "session"）
//   - Accepted: 用户是否已接受
type PermissionRule struct {
	ID          string           `json:"id"`
	ToolName    string           `json:"tool_name"`
	Description string           `json:"description"`
	RuleContent string           `json:"rule_content,omitempty"` // Bash: "npm run:*", 文件: "src/**"
	Action      PermissionAction `json:"action"`
	Source      string           `json:"source"`
	CreatedAt   string           `json:"created_at"`
	Accepted    bool             `json:"accepted"`
}

// PermissionDecision 权限检查结果。
//
// 字段：
//   - Action: 最终决定动作
//   - Rules: 建议的权限规则（用户可接受）
//   - Reason: 决定原因
type PermissionDecision struct {
	Action       PermissionAction `json:"action"`
	Rules        []PermissionRule `json:"rules,omitempty"`
	Reason       string           `json:"reason"`
	BypassImmune bool             `json:"bypass_immune,omitempty"` // 安全检查标记
}

// =============================================
// PermissionMode — 全局策略模式（ 的 PermissionMode）
// =============================================

// PermissionMode 权限模式（全局静态策略）。
type PermissionMode string

const (
	// ModeDefault 默认模式：所有操作需要显式规则或用户确认。
	// 只读命令自动放行；其余走规则匹配，无命中则 ASK。
	ModeDefault PermissionMode = "DEFAULT"

	// ModeExplore 只读探索模式：放行只读工具和只读命令；任何修改自动拒绝。
	ModeExplore PermissionMode = "EXPLORE"

	// ModeAcceptEdits 接受编辑模式：自动放行工作目录内的文件操作和 filesystem 命令。
	// 只读命令自动放行；其余走规则匹配。
	ModeAcceptEdits PermissionMode = "ACCEPT_EDITS"

	// ModeBypass 绕过模式：跳过所有权限检查（除 DENY/ASK 规则外）。
	// 工具的安全检查（bypass_immune）也被忽略。
	// 使用此模式时务必配置 DENY 规则保护关键路径。
	ModeBypass PermissionMode = "BYPASS"

	// ModeDontAsk 非交互模式：将任何 ASK 转为 DENY（无需用户回应）。
	// 适合无人值守/CI 场景。
	ModeDontAsk PermissionMode = "DONT_ASK"
)

// =============================================
// PermissionContext — 权限上下文
// =============================================

// PermissionContext 权限上下文（ 的 PermissionContext）。
//
// 携带 mode、规则和工作目录，传递给权限检查器。
//
// 字段：
//   - Mode: 全局模式
//   - AllowRules: 按工具名分组的允许规则
//   - DenyRules: 按工具名分组的拒绝规则
//   - AskRules: 按工具名分组的确认规则
//   - WorkingDirectories: 配置的工作目录（用于 ACCEPT_EDITS 模式判断）
type PermissionContext struct {
	Mode               PermissionMode              `json:"mode"`
	AllowRules         map[string][]PermissionRule `json:"allow_rules,omitempty"`
	DenyRules          map[string][]PermissionRule `json:"deny_rules,omitempty"`
	AskRules           map[string][]PermissionRule `json:"ask_rules,omitempty"`
	WorkingDirectories map[string]WorkingDirectory `json:"working_directories,omitempty"`
}

// WorkingDirectory 工作目录配置。
//
// 字段：
//   - Path: 目录绝对路径
//   - Source: 来源标识（"userSettings", "projectSettings"）
type WorkingDirectory struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

// NewPermissionContext 创建默认权限上下文。
//
// 返回：
//   - *PermissionContext: DEFAULT 模式，规则列表为空
func NewPermissionContext() *PermissionContext {
	return &PermissionContext{
		Mode:               ModeDefault,
		AllowRules:         make(map[string][]PermissionRule),
		DenyRules:          make(map[string][]PermissionRule),
		AskRules:           make(map[string][]PermissionRule),
		WorkingDirectories: make(map[string]WorkingDirectory),
	}
}

// =============================================
// 危险路径列表（ 的危险路径保护）
// =============================================

// DangerousFiles 敏感文件列表（操作这些文件需要额外确认）。
//
// 分类：
//   - Shell 配置: .bashrc, .zshrc, .bash_profile, .profile
//   - Git 配置: .gitconfig, .gitmodules
//   - SSH 密钥: id_rsa, id_ed25519, authorized_keys
//   - 凭证: .env, .npmrc, .pypirc, .aws/credentials
var DangerousFiles = []string{
	".bashrc", ".zshrc", ".bash_profile", ".profile",
	".gitconfig", ".gitmodules",
	".env", ".env.local", ".npmrc", ".pypirc",
}

// DangerousDirs 敏感目录列表（操作这些目录需要额外确认）。
var DangerousDirs = []string{
	".git", ".ssh", ".claude", ".vscode", ".aws", ".kube",
}

// IsDangerousPath 检查路径是否在危险列表中。
//
// 参数：
//   - path: 要检查的路径
//
// 返回：
//   - bool: 是否危险
//   - string: 匹配的危险条目
func IsDangerousPath(path string) (bool, string) {
	for _, f := range DangerousFiles {
		if path == f || contains(path, "/"+f) || contains(path, "\\"+f) {
			return true, f
		}
	}
	for _, d := range DangerousDirs {
		if contains(path, d+"/") || contains(path, d+"\\") {
			return true, d
		}
	}
	return false, ""
}

// contains 字符串包含检查。
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || searchSubstring(s, substr))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// =============================================
// 只读命令列表（ 的只读命令检测）
// =============================================

// ReadOnlyCommands 常见的只读命令集合。
//
// 分类：
//   - Git: status, log, diff, show, branch, blame, grep, reflog
//   - 文件: ls, cat, head, tail, grep, rg, find, tree, stat, wc, pwd, which
//   - Docker: ps, images, logs, inspect, info
//   - GitHub: repo view, issue list, pr list, status
//   - 包管理: list, show, --version
var ReadOnlyCommands = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true,
	"grep": true, "rg": true, "find": true, "tree": true,
	"stat": true, "wc": true, "pwd": true, "which": true,
	"echo": true, "printf": true, "date": true, "hostname": true,
	"uname": true, "whoami": true, "id": true, "env": true,
	"ps": true, "top": true, "free": true, "df": true, "du": true,
}

// GitReadOnlySubcommands Git 只读子命令。
var GitReadOnlySubcommands = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true,
	"branch": true, "blame": true, "grep": true, "reflog": true,
}

// IsReadOnlyCommand 判断命令是否只读（简单关键词匹配）。
//
// 参数：
//   - command: 命令字符串
//
// 返回：
//   - bool: 是否只读
func IsReadOnlyCommand(command string) bool {
	// 检查输出重定向
	if contains(command, ">") {
		return false
	}

	// 解析第一个 token
	parts := splitCommand(command)
	if len(parts) == 0 {
		return false
	}

	base := parts[0]

	// git 子命令检查
	if base == "git" && len(parts) > 1 {
		return GitReadOnlySubcommands[parts[1]]
	}

	// docker 子命令检查
	if base == "docker" && len(parts) > 1 {
		switch parts[1] {
		case "ps", "images", "logs", "inspect", "info":
			return true
		}
	}

	return ReadOnlyCommands[base]
}

// splitCommand 简单拆分命令字符串。
func splitCommand(cmd string) []string {
	var parts []string
	current := ""
	inQuote := false
	for _, ch := range cmd {
		switch {
		case ch == '"' || ch == '\'':
			inQuote = !inQuote
		case ch == ' ' && !inQuote:
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		default:
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
