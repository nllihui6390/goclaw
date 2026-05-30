package tool

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// SystemInfoTool 系统信息工具
type SystemInfoTool struct{}

func NewSystemInfoTool() *SystemInfoTool {
	return &SystemInfoTool{}
}

func (t *SystemInfoTool) Name() string {
	return "system_info"
}

func (t *SystemInfoTool) Description() string {
	return `获取系统运行状态信息，包括 CPU、内存、磁盘使用情况。

调用格式：
- system_info(category="all")  # 获取全部信息
- system_info(category="cpu")  # 仅 CPU 信息
- system_info(category="memory")  # 仅内存信息
- system_info(category="disk")  # 仅磁盘信息
- system_info(category="process")  # 进程列表

参数说明：
- category: 信息类别: all, cpu, memory, disk, process（默认 all）`
}

func (t *SystemInfoTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"category": map[string]interface{}{
				"type":        "string",
				"description": "信息类别: all, cpu, memory, disk, process",
			},
		},
	}
}

func (t *SystemInfoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	category := "all"
	if cat, ok := params["category"].(string); ok && cat != "" {
		category = strings.ToLower(cat)
	}

	var sb strings.Builder
	sb.WriteString("## 系统信息\n\n")

	switch category {
	case "cpu":
		sb.WriteString(cpuInfo())
	case "memory":
		sb.WriteString(memoryInfo())
	case "disk":
		sb.WriteString(diskInfo())
	case "process":
		sb.WriteString(processInfo())
	default:
		sb.WriteString(basicInfo())
		sb.WriteString("\n")
		sb.WriteString(cpuInfo())
		sb.WriteString("\n")
		sb.WriteString(memoryInfo())
		sb.WriteString("\n")
		sb.WriteString(diskInfo())
	}

	return sb.String(), nil
}

func basicInfo() string {
	var sb strings.Builder
	sb.WriteString("### 基本信息\n")
	sb.WriteString(fmt.Sprintf("- 操作系统: %s\n", runtime.GOOS))
	sb.WriteString(fmt.Sprintf("- 架构: %s\n", runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("- Go 版本: %s\n", runtime.Version()))
	sb.WriteString(fmt.Sprintf("- CPU 核数: %d\n", runtime.NumCPU()))
	sb.WriteString(fmt.Sprintf("- 当前时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))

	hostname, err := os.Hostname()
	if err == nil {
		sb.WriteString(fmt.Sprintf("- 主机名: %s\n", hostname))
	}

	return sb.String()
}

func cpuInfo() string {
	var sb strings.Builder
	sb.WriteString("### CPU 信息\n")
	sb.WriteString(fmt.Sprintf("- 逻辑核数: %d\n", runtime.NumCPU()))

	// 通过 exec 获取更详细的 CPU 信息
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			lines := strings.Split(string(data), "\n")
			modelName := ""
			for _, line := range lines {
				if strings.HasPrefix(line, "model name") {
					parts := strings.Split(line, ":")
					if len(parts) >= 2 {
						modelName = strings.TrimSpace(parts[1])
					}
				}
			}
			if modelName != "" {
				sb.WriteString(fmt.Sprintf("- CPU 型号: %s\n", modelName))
			}
		}
	}

	return sb.String()
}

func memoryInfo() string {
	var sb strings.Builder
	sb.WriteString("### 内存信息\n")

	// Go runtime 内存统计
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	sb.WriteString(fmt.Sprintf("- Go 进程已分配: %.2f MB\n", float64(m.Alloc)/1024/1024))
	sb.WriteString(fmt.Sprintf("- Go 进程总分配: %.2f MB\n", float64(m.TotalAlloc)/1024/1024))
	sb.WriteString(fmt.Sprintf("- Go 进程系统占用: %.2f MB\n", float64(m.Sys)/1024/1024))
	sb.WriteString(fmt.Sprintf("- GC 次数: %d\n", m.NumGC))

	// 系统物理内存
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "MemTotal:") || strings.HasPrefix(line, "MemAvailable:") {
					sb.WriteString("- " + strings.TrimSpace(line) + "\n")
				}
			}
		}
	}

	return sb.String()
}

func diskInfo() string {
	var sb strings.Builder
	sb.WriteString("### 磁盘信息\n")

	if runtime.GOOS == "linux" {
		// 读取 /proc/mounts 获取挂载点
		if data, err := os.ReadFile("/proc/mounts"); err == nil {
			mounts := strings.Split(string(data), "\n")
			for _, mount := range mounts {
				parts := strings.Fields(mount)
				if len(parts) >= 2 {
					device := parts[0]
					path := parts[1]
					// 只关注真实磁盘
					if strings.HasPrefix(device, "/dev/") && !strings.Contains(path, "snap") {
						sb.WriteString(fmt.Sprintf("- %s → %s\n", device, path))
					}
				}
			}
		}
	} else {
		sb.WriteString(fmt.Sprintf("- 工作目录: %s\n", mustGetwd()))
	}

	return sb.String()
}

func processInfo() string {
	var sb strings.Builder
	sb.WriteString("### 进程信息\n")
	sb.WriteString(fmt.Sprintf("- Go 进程 Goroutine 数: %d\n", runtime.NumGoroutine()))

	// 当前进程信息
	sb.WriteString(fmt.Sprintf("- 进程 PID: %d\n", os.Getpid()))

	return sb.String()
}

func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return dir
}

func init() {
	GlobalRegistry.Register("system_info", func() Tool {
		return NewSystemInfoTool()
	})
}