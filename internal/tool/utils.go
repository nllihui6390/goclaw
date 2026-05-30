package tool

import (
	"os"
	"os/exec"
	"strings"
)

// pythonCmd 返回可用的 Python 命令
func pythonCmd() string {
	if _, err := exec.LookPath("python3"); err == nil {
		return "python3"
	}
	if _, err := exec.LookPath("python"); err == nil {
		return "python"
	}
	return "python"
}

// hasPyModule 检查 Python 模块是否可用
func hasPyModule(module string) bool {
	cmd := exec.Command(pythonCmd(), "-c", "import "+module)
	return cmd.Run() == nil
}

// getEnv 获取环境变量，带默认值
func getEnv(key, defaultVal string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return defaultVal
	}
	return val
}