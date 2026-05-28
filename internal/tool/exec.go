package tool

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ExecTool 执行命令工具（跨平台版本）
type ExecTool struct{}

func (t *ExecTool) Name() string {
	return "execute_command"
}

func (t *ExecTool) Description() string {
	return "执行shell命令并返回输出。超时10秒。" +
		"\n重要规则:" +
		"\n1. 读取文件请用 read_file 工具，不要用 cat/type 命令" +
		"\n2. 写文件请用 write_file 工具，不要用 echo/printf 重定向" +
		"\n3. 查看定时任务请用 cron_status 工具，不要用 crontab/schtasks 命令" +
		"\n4. 本工具适合系统信息查询（ls/dir、ps/tasklist、df、whoami等）和脚本执行" +
		"\n5. Windows 下常见 Linux 命令自动转换（ls→dir, ps→tasklist, grep→findstr）" +
		"\n调用格式: execute_command(command=\"shell命令\")" +
		"\n示例: execute_command(command=\"ls -la\") 或 execute_command(command=\"dir\")"
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

	// 安全检查：禁止危险命令（只匹配独立命令，不匹配路径中的子串）
	commandLower := strings.ToLower(command)
	// 提取第一个词作为命令名
	firstWord := ""
	if parts := strings.Fields(commandLower); len(parts) > 0 {
		// 廻除路径部分，只取最后的命令名
		base := parts[0]
		if idx := strings.LastIndex(base, "/"); idx >= 0 {
			base = base[idx+1:]
		}
		if idx := strings.LastIndex(base, "\\"); idx >= 0 {
			base = base[idx+1:]
		}
		firstWord = base
	}

	dangerousCommands := []string{"rm", "dd", "mkfs", "sudo", "chmod", "chown", "format", "rmdir"}
	for _, d := range dangerousCommands {
		if firstWord == d {
			return "", fmt.Errorf("禁止执行危险命令: %s", d)
		}
	}

	// 額外检查 Windows 特有的危险命令模式
	dangerousPatterns := []string{"del /f", "del /q", "rd /s"}
	for _, p := range dangerousPatterns {
		if strings.Contains(commandLower, p) {
			return "", fmt.Errorf("禁止执行危险命令模式: %s", p)
		}
	}

	// 命令执行超时：最多10秒
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 根据操作系统转换命令
	if runtime.GOOS == "windows" {
		command = translateToWindows(command)
	}

	var cmd *exec.Cmd

	// 根据操作系统选择正确的shell
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(timeoutCtx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(timeoutCtx, "sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("命令执行超时（10秒）: %s", command)
	}
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
