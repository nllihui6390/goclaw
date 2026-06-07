package tool

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ExecTool 执行命令工具
type ExecTool struct{}

func (t *ExecTool) Name() string {
	return "execute_command"
}

func (t *ExecTool) Description() string {
	osInfo := fmt.Sprintf("当前操作系统: %s/%s", runtime.GOOS, runtime.GOARCH)
	shellHint := ""
	switch runtime.GOOS {
	case "windows":
		shellHint = "请使用 Windows 命令（dir、type、tasklist、findstr、ipconfig、netstat 等），通过 cmd.exe 执行\n注意: 避免在命令中使用嵌套引号，如 python -c \"print('x')\" 可能失败，建议写成脚本文件后执行"
	case "darwin":
		shellHint = "请使用 macOS 命令（ls、cat、ps、grep、sw_vers、networksetup 等），通过 sh 执行\n注意: macOS 与 Linux 命令基本相同，但系统信息命令不同（如 sw_vers）"
	default:
		shellHint = "请使用 Linux 命令（ls、cat、ps、grep、uname、ifconfig 等），通过 sh 执行"
	}

	return fmt.Sprintf(`执行 shell 命令并返回输出。默认超时60秒（pip install 等耗时操作可正常执行）。

⚠️ 系统环境: %s
%s

重要规则:
1. 读取文件请用 read_file 工具，不要用 cat/type 命令
2. 写文件请用 write_file 工具，不要用 echo/printf 重定向
3. 查看定时任务请用 cron_status 工具，不要用 crontab/schtasks 命令
4. 本工具适合系统信息查询、脚本执行、pip install 等操作
5. 根据操作系统类型选择正确的命令格式
6. Windows 下避免使用嵌套引号的 python -c 命令，建议先写脚本文件再执行

调用格式: execute_command(command="shell命令")
示例:
- Windows: execute_command(command="dir") 或 execute_command(command="pip install requests")
- macOS: execute_command(command="ls -la") 或 execute_command(command="sw_vers")
- Linux: execute_command(command="ls -la") 或 execute_command(command="pip install numpy")`, osInfo, shellHint)
}

func (t *ExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": fmt.Sprintf("要执行的命令（当前系统 %s/%s，请使用对应系统命令格式）", runtime.GOOS, runtime.GOARCH),
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

	// 安全检查：禁止危险命令（clawdata/tmp 目录下的删除操作除外）
	commandLower := strings.ToLower(command)
	firstWord := ""
	if parts := strings.Fields(commandLower); len(parts) > 0 {
		base := parts[0]
		if idx := strings.LastIndex(base, "/"); idx >= 0 {
			base = base[idx+1:]
		}
		if idx := strings.LastIndex(base, "\\"); idx >= 0 {
			base = base[idx+1:]
		}
		firstWord = base
	}

	// 检查是否只针对 clawdata/tmp 目录的删除操作
	isTmpCleanup := isTmpDirCommand(commandLower)

	dangerousCommands := []string{"rm", "dd", "mkfs", "sudo", "chmod", "chown", "format", "rmdir", "del"}
	for _, d := range dangerousCommands {
		if firstWord == d {
			if isTmpCleanup && (d == "rm" || d == "del" || d == "rmdir") {
				// 允许删除 clawdata/tmp 目录下的文件
				continue
			}
			return "", fmt.Errorf("禁止执行危险命令: %s", d)
		}
	}

	if !isTmpCleanup {
		dangerousPatterns := []string{"del /f", "del /q", "rd /s"}
		for _, p := range dangerousPatterns {
			if strings.Contains(commandLower, p) {
				return "", fmt.Errorf("禁止执行危险命令模式: %s", p)
			}
		}
	}

	// 命令执行超时：最多60秒（pip install 等操作需要更长时间）
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 根据操作系统选择 shell 执行
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(timeoutCtx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(timeoutCtx, "sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("命令执行超时（60秒）: %s", command)
	}
	if err != nil {
		return "", fmt.Errorf("命令执行失败: %v, 输出: %s", err, string(output))
	}

	return string(output), nil
}

// isTmpDirCommand 检查命令是否只针对 clawdata/temp 目录
func isTmpDirCommand(commandLower string) bool {
	if globalDataDir == "" {
		return false
	}
	tempDir := strings.ToLower(globalDataDir + "/temp")
	tempDirWin := strings.ToLower(globalDataDir + "\\temp")

	return strings.Contains(commandLower, tempDir) || strings.Contains(commandLower, tempDirWin)
}