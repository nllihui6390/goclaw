package tool

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ExecTool 执行命令工具（跨平台版本）
type ExecTool struct{}

func (t *ExecTool) Name() string {
	return "execute_command"
}

func (t *ExecTool) Description() string {
	return "在系统上执行shell命令（仅限只读操作）"
}

func (t *ExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "要执行的命令，如：ls -la (Linux/Mac) 或 dir (Windows)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ExecTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	command, ok := params["command"].(string)
	if !ok {
		return "", fmt.Errorf("缺少命令参数")
	}

	// 安全检查：禁止危险命令
	dangerous := []string{"rm", "dd", "mkfs", "sudo", "chmod", "chown", "del /f", "format", "rd /s", "rmdir", "del"}
	for _, d := range dangerous {
		if strings.Contains(strings.ToLower(command), d) {
			return "", fmt.Errorf("禁止执行危险命令: %s", d)
		}
	}

	// 根据操作系统转换命令
	if runtime.GOOS == "windows" {
		command = translateToWindows(command)
	}

	var cmd *exec.Cmd

	// 根据操作系统选择正确的shell
	if runtime.GOOS == "windows" {
		// Windows 使用 cmd
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		// Linux/Mac 使用 sh
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("命令执行失败: %v, 输出: %s", err, string(output))
	}

	return string(output), nil
}

// translateToWindows 将常见的Linux命令转换为Windows命令
func translateToWindows(cmd string) string {
	// 分割命令和参数
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return cmd
	}

	// 命令映射表
	translations := map[string]string{
		"ls":    "dir",
		"ll":    "dir", // ll 通常等同于 ls -l
		"pwd":   "cd",
		"cat":   "type",
		"echo":  "echo",
		"ps":    "tasklist",
		"grep":  "findstr",
		"wc":    "find /c /v \"\"", // 简单处理，不完全准确
		"mkdir": "mkdir",
		"rmdir": "rmdir",
		"cp":    "copy",
		"mv":    "move",
	}

	// 处理带参数的命令
	firstCmd := parts[0]
	if translated, exists := translations[firstCmd]; exists {
		// 特殊处理 ls -l 和 ll
		if firstCmd == "ls" && len(parts) > 1 {
			if parts[1] == "-l" || parts[1] == "-la" {
				return "dir"
			}
			if parts[1] == "-a" {
				return "dir /a"
			}
		}

		// 替换命令，保留参数
		if len(parts) > 1 {
			// 转换参数
			args := translateArgs(parts[1:])
			return translated + " " + strings.Join(args, " ")
		}
		return translated
	}

	return cmd
}

// translateArgs 转换命令参数
func translateArgs(args []string) []string {
	var translated []string
	for _, arg := range args {
		// 转换常见的参数
		switch arg {
		case "-l", "-la":
			// 在Windows dir中这些参数不需要
			continue
		case "-a":
			translated = append(translated, "/a")
		case "-r":
			translated = append(translated, "/r")
		case "-f":
			translated = append(translated, "/f")
		default:
			translated = append(translated, arg)
		}
	}
	return translated
}
