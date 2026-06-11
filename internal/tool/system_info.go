package tool

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"time"
)

// SystemInfo represents the complete system information
type SystemInfo struct {
	OS       string      `json:"os,omitempty"`
	Hostname string      `json:"hostname,omitempty"`
	Arch     string      `json:"arch,omitempty"`
	GoVersion string     `json:"go_version,omitempty"`
	CPU      *CPUInfo    `json:"cpu,omitempty"`
	Memory   *MemoryInfo `json:"memory,omitempty"`
	Disk     []DiskInfo  `json:"disk,omitempty"`
	Uptime   string      `json:"uptime,omitempty"`
	Process  *ProcessInfo `json:"process,omitempty"`
}

// CPUInfo contains CPU information
type CPUInfo struct {
	Cores     int    `json:"cores"`
	ModelName string `json:"model_name,omitempty"`
}

// MemoryInfo contains memory information
type MemoryInfo struct {
	AllocMB      float64 `json:"alloc_mb"`
	TotalAllocMB float64 `json:"total_alloc_mb"`
	SysMB        float64 `json:"sys_mb"`
	NumGC        uint32  `json:"num_gc"`
	MemTotal     string  `json:"mem_total,omitempty"`
	MemAvailable string  `json:"mem_available,omitempty"`
}

// DiskInfo contains disk information
type DiskInfo struct {
	Device string `json:"device,omitempty"`
	Path   string `json:"path,omitempty"`
}

// ProcessInfo contains process information
type ProcessInfo struct {
	PID         int `json:"pid"`
	Goroutines  int `json:"goroutines"`
}

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

	var result interface{}

	switch category {
	case "cpu":
		result = map[string]interface{}{
			"cpu": getCPUInfo(),
		}
	case "memory":
		result = map[string]interface{}{
			"memory": getMemoryInfo(),
		}
	case "disk":
		result = map[string]interface{}{
			"disk": getDiskInfo(),
		}
	case "process":
		result = map[string]interface{}{
			"process": getProcessInfo(),
		}
	default:
		result = getAllInfo()
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}

func getAllInfo() *SystemInfo {
	info := &SystemInfo{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
		Uptime:    time.Now().Format("2006-01-02 15:04:05"),
	}

	hostname, err := os.Hostname()
	if err == nil {
		info.Hostname = hostname
	}

	info.CPU = getCPUInfo()
	info.Memory = getMemoryInfo()
	info.Disk = getDiskInfo()

	return info
}

func getCPUInfo() *CPUInfo {
	cpu := &CPUInfo{
		Cores: runtime.NumCPU(),
	}

	// 通过读取 /proc/cpuinfo 获取更详细的 CPU 信息
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "model name") {
					parts := strings.Split(line, ":")
					if len(parts) >= 2 {
						cpu.ModelName = strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}

	return cpu
}

func getMemoryInfo() *MemoryInfo {
	// Go runtime 内存统计
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	mem := &MemoryInfo{
		AllocMB:      float64(m.Alloc) / 1024 / 1024,
		TotalAllocMB: float64(m.TotalAlloc) / 1024 / 1024,
		SysMB:        float64(m.Sys) / 1024 / 1024,
		NumGC:        m.NumGC,
	}

	// 系统物理内存
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "MemTotal:") {
					mem.MemTotal = line
				} else if strings.HasPrefix(line, "MemAvailable:") {
					mem.MemAvailable = line
				}
			}
		}
	}

	return mem
}

func getDiskInfo() []DiskInfo {
	var disks []DiskInfo

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
						disks = append(disks, DiskInfo{
							Device: device,
							Path:   path,
						})
					}
				}
			}
		}
	} else {
		// 非 Linux 系统返回工作目录
		disks = append(disks, DiskInfo{
			Path: mustGetwd(),
		})
	}

	return disks
}

func getProcessInfo() *ProcessInfo {
	return &ProcessInfo{
		PID:        os.Getpid(),
		Goroutines: runtime.NumGoroutine(),
	}
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