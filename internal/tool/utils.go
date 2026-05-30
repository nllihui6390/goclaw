package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// globalDataDir 全局数据目录（在 bootstrap 中设置）
var globalDataDir string

// SetGlobalDataDir 设置全局数据目录
func SetGlobalDataDir(dataDir string) {
	globalDataDir = dataDir
	// 确保 temp 目录存在
	tempDir := filepath.Join(dataDir, "temp")
	os.MkdirAll(tempDir, 0755)
}

// getTmpDir 获取临时目录路径（优先使用 clawdata/temp，回退到系统临时目录）
func getTmpDir() string {
	if globalDataDir != "" {
		tempDir := filepath.Join(globalDataDir, "temp")
		os.MkdirAll(tempDir, 0755)
		return tempDir
	}
	return os.TempDir()
}

// getTmpFile 在临时目录生成临时文件路径
func getTmpFile(prefix, ext string) string {
	return filepath.Join(getTmpDir(), prefix+ext)
}

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