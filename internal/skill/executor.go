package skill

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	glog "go-claw/pkg/log"
)

// Executor Skill 执行器
type Executor struct {
	registry *Registry
}

// NewExecutor 创建执行器
func NewExecutor(registry *Registry) *Executor {
	return &Executor{registry: registry}
}

// Execute 执行指定 Skill
func (e *Executor) Execute(ctx context.Context, skillName string, args map[string]string) (string, error) {
	logger := glog.Logger()

	skill, exists := e.registry.Get(skillName)
	if !exists {
		return "", fmt.Errorf("Skill 不存在: %s", skillName)
	}

	logger.Info("[Skill] 开始执行", "name", skill.Name, "emoji", skill.Emoji())

	// 检查是否有脚本可用
	if len(skill.Scripts) > 0 {
		return e.executeScript(ctx, skill, args)
	}

	// 无脚本时，根据执行步骤生成提示信息给 AI
	return e.generateGuidance(skill, args), nil
}

// executeScript 执行 Skill 的脚本
func (e *Executor) executeScript(ctx context.Context, skill *Skill, args map[string]string) (string, error) {
	logger := glog.Logger()

	// 找到匹配的脚本（优先 .sh -> .py -> .js）
	script := skill.Scripts[0]
	for _, s := range skill.Scripts {
		if strings.HasSuffix(s, ".sh") {
			script = s
			break
		}
	}

	logger.Info("[Skill] 执行脚本", "script", script, "args_count", len(args))

	// 构建命令
	var cmd *exec.Cmd
	if strings.HasSuffix(script, ".py") {
		argsList := buildArgsList(args)
		cmd = exec.CommandContext(ctx, "python3", script)
		cmd.Args = append(cmd.Args, argsList...)
	} else if strings.HasSuffix(script, ".sh") {
		// 设置环境变量传递参数
		env := os.Environ()
		for k, v := range args {
			env = append(env, fmt.Sprintf("SKILL_%s=%s", strings.ToUpper(k), v))
		}
		cmd = exec.CommandContext(ctx, "bash", script)
		cmd.Env = env
	} else if strings.HasSuffix(script, ".js") {
		argsList := buildArgsList(args)
		cmd = exec.CommandContext(ctx, "node", script)
		cmd.Args = append(cmd.Args, argsList...)
	} else {
		return "", fmt.Errorf("不支持的脚本类型: %s", script)
	}

	// 执行并捕获输出
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	elapsed := time.Since(startTime)

	if err != nil {
		logger.Error("[Skill] 脚本执行失败",
			"name", skill.Name,
			"script", script,
			"err", err,
			"stderr", stderr.String(),
			"elapsed_ms", elapsed.Milliseconds())
		return "", fmt.Errorf("Skill 执行失败: %v\n%s", err, stderr.String())
	}

	output := stdout.String()
	logger.Info("[Skill] 执行成功",
		"name", skill.Name,
		"output_len", len(output),
		"elapsed_ms", elapsed.Milliseconds())

	// 如果有输出格式定义，格式化结果
	if skill.OutputFormat != "" {
		return fmt.Sprintf("%s %s\n%s", skill.Emoji(), skill.Name, output), nil
	}

	return output, nil
}

// generateGuidance 为 AI 生成执行指导（无脚本时）
func (e *Executor) generateGuidance(skill *Skill, args map[string]string) string {
	var guidance strings.Builder

	guidance.WriteString(fmt.Sprintf("## %s %s\n\n", skill.Emoji(), skill.Name))
	guidance.WriteString(fmt.Sprintf("**描述**: %s\n\n", skill.Description))

	if skill.CoreCapabilities != "" {
		guidance.WriteString(fmt.Sprintf("### 核心能力\n%s\n\n", skill.CoreCapabilities))
	}

	if skill.ExecutionWorkflow != "" {
		// 替换变量占位符
		steps := SubstituteVariables(skill.ExecutionWorkflow, args)
		guidance.WriteString(fmt.Sprintf("### 执行步骤\n%s\n\n", steps))
	}

	if skill.InputRequirements != "" {
		guidance.WriteString(fmt.Sprintf("### 输入要求\n%s\n\n", skill.InputRequirements))
	}

	if skill.OutputFormat != "" {
		format := SubstituteVariables(skill.OutputFormat, args)
		guidance.WriteString(fmt.Sprintf("### 输出格式\n%s\n\n", format))
	}

	if skill.ErrorHandling != "" {
		guidance.WriteString(fmt.Sprintf("### 异常处理\n%s\n\n", skill.ErrorHandling))
	}

	return guidance.String()
}

// ListSkills 列出所有可用 Skill
func (e *Executor) ListSkills() string {
	return e.registry.SkillSummary()
}

func buildArgsList(args map[string]string) []string {
	var list []string
	for k, v := range args {
		list = append(list, fmt.Sprintf("--%s=%s", k, v))
	}
	return list
}