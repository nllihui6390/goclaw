package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RunCodeTool 运行代码片段工具（沙箱执行）
type RunCodeTool struct{}

func NewRunCodeTool() *RunCodeTool {
	return &RunCodeTool{}
}

func (t *RunCodeTool) Name() string {
	return "run_code"
}

func (t *RunCodeTool) Description() string {
	return `运行 Python 或 JavaScript 代码片段并返回输出结果。
适合数据分析、复杂计算、文本处理等任务。

调用格式：
- run_code(language="python", code="print(2**10)")  # 运行 Python
- run_code(language="javascript", code="console.log(Math.pow(2,10))")  # 运行 JS

参数说明：
- language: python 或 javascript（必填）
- code: 代码内容（必填）
- timeout: 超时秒数，默认 10

注意事项：
- Python 需要系统安装 python3
- JavaScript 需要系统安装 node
- 代码在临时文件中执行，执行后自动清理
- 禁止网络访问和文件写入敏感路径`
}

func (t *RunCodeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"language": map[string]interface{}{
				"type":        "string",
				"description": "编程语言: python 或 javascript",
			},
			"code": map[string]interface{}{
				"type":        "string",
				"description": "代码内容",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "超时秒数，默认10",
			},
		},
		"required": []string{"language", "code"},
	}
}

// RunCodeResult 代码执行结果JSON结构
type RunCodeResult struct {
	Language string `json:"language"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

func (t *RunCodeTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	language, ok := params["language"].(string)
	if !ok || language == "" {
		return "", fmt.Errorf("缺少 language 参数")
	}

	code, ok := params["code"].(string)
	if !ok || code == "" {
		return "", fmt.Errorf("缺少 code 参数")
	}

	timeout := 10
	if t, ok := params["timeout"].(float64); ok && t > 0 {
		timeout = int(t)
	}

	language = strings.ToLower(language)

	var cmdName string
	var fileExt string
	var wrapper string

	switch language {
	case "python", "python3":
		cmdName = "python3"
		// 尝试 python3，如果不存在用 python
		if _, err := exec.LookPath("python3"); err != nil {
			cmdName = "python"
		}
		fileExt = ".py"
		wrapper = code
		language = "python"
	case "javascript", "js", "node":
		cmdName = "node"
		fileExt = ".js"
		wrapper = code
		language = "javascript"
	default:
		return "", fmt.Errorf("不支持的语言: %s (支持: python, javascript)", language)
	}

	// 检查运行环境是否可用
	if _, err := exec.LookPath(cmdName); err != nil {
		return "", fmt.Errorf("未找到 %s 运行环境，请先安装", cmdName)
	}

	// 安全检查：禁止危险操作
	if containsDangerousCode(code) {
		return "", fmt.Errorf("代码包含危险操作，禁止执行")
	}

	// 写入临时文件（在 clawdata/tmp 目录）
	tmpFile := getTmpFile("goclaw_run_"+fmt.Sprintf("%d", time.Now().UnixNano()), fileExt)
	if err := os.WriteFile(tmpFile, []byte(wrapper), 0644); err != nil {
		return "", fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile)

	// 执行
	ctxExec, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxExec, cmdName, tmpFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	errOutput := stderr.String()

	// 截断过长输出
	if len(output) > 20000 {
		output = output[:20000]
	}
	if len(errOutput) > 5000 {
		errOutput = errOutput[:5000]
	}

	result := RunCodeResult{
		Language: language,
		Stdout:   output,
		Stderr:   errOutput,
		Success:  err == nil,
	}

	if err != nil {
		if ctxExec.Err() == context.DeadlineExceeded {
			result.Error = fmt.Sprintf("执行超时（%d秒）", timeout)
		} else {
			result.Error = err.Error()
		}
		result.ExitCode = 1
	} else {
		result.ExitCode = 0
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %v", err)
	}

	return string(jsonBytes), nil
}

func containsDangerousCode(code string) bool {
	dangerous := []string{
		"os.system",
		"subprocess",
		"exec(",
		"eval(",
		"__import__",
		"rm -rf",
		"del /",
		"format(",
		"child_process",
		"require('child_process')",
		"fs.unlink",
		"fs.rm",
		"net.Server",
		"http.Server",
	}
	lower := strings.ToLower(code)
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return true
		}
	}
	return false
}

func init() {
	GlobalRegistry.Register("run_code", func() Tool {
		return NewRunCodeTool()
	})
}