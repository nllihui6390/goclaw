package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// NetworkCheckTool 网络检测工具
type NetworkCheckTool struct{}

func NewNetworkCheckTool() *NetworkCheckTool {
	return &NetworkCheckTool{}
}

func (t *NetworkCheckTool) Name() string {
	return "network_check"
}

func (t *NetworkCheckTool) Description() string {
	return `检测网络连通性，支持 ping、DNS 解析、端口检测。

调用格式：
- network_check(action="ping", target="8.8.8.8")  # ping 测试
- network_check(action="dns", target="google.com")  # DNS 解析
- network_check(action="port", target="example.com:80")  # 端口检测
- network_check(action="http", target="https://example.com")  # HTTP 连通测试

参数说明：
- action: 操作类型: ping, dns, port, http（必填）
- target: 目标地址（必填）
- timeout: 超时秒数，默认 5`
}

func (t *NetworkCheckTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "操作类型: ping, dns, port, http",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "目标地址",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "超时秒数，默认5",
			},
		},
		"required": []string{"action", "target"},
	}
}

// PingResult ping测试结果
type PingResult struct {
	Target    string   `json:"target"`
	Latencies []string `json:"latencies"`
	Output    string   `json:"output"`
	Success   bool     `json:"success"`
	Error     string   `json:"error,omitempty"`
}

// DNSResult DNS解析结果
type DNSResult struct {
	Target  string   `json:"target"`
	IPs     []string `json:"ips,omitempty"`
	Domains []string `json:"domains,omitempty"`
	Reverse bool     `json:"reverse"`
}

// PortResult 端口检测结果
type PortResult struct {
	Host    string `json:"host"`
	Port    string `json:"port"`
	Open    bool   `json:"open"`
	Error   string `json:"error,omitempty"`
}

// HTTPResult HTTP连通测试结果
type HTTPResult struct {
	URL        string `json:"url"`
	StatusCode string `json:"status_code,omitempty"`
	Duration   string `json:"duration"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

func (t *NetworkCheckTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	action, ok := params["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("缺少 action 参数")
	}

	target, ok := params["target"].(string)
	if !ok || target == "" {
		return "", fmt.Errorf("缺少 target 参数")
	}

	timeout := 5
	if t, ok := params["timeout"].(float64); ok && t > 0 {
		timeout = int(t)
	}

	switch strings.ToLower(action) {
	case "ping":
		return t.ping(target, timeout)
	case "dns":
		return t.dnsLookup(target, timeout)
	case "port":
		return t.portCheck(target, timeout)
	case "http":
		return t.httpCheck(target, timeout)
	default:
		return "", fmt.Errorf("未知操作: %s (支持: ping, dns, port, http)", action)
	}
}

func (t *NetworkCheckTool) ping(target string, timeout int) (string, error) {
	// 使用系统 ping 命令
	var cmd *exec.Cmd
	count := "3"
	if timeout < 3 {
		count = fmt.Sprintf("%d", timeout)
	}

	cmd = exec.Command("ping", "-c", count, "-W", fmt.Sprintf("%d", timeout), target)
	output, err := cmd.CombinedOutput()

	result := PingResult{
		Target: target,
		Output: string(output),
	}

	// 解析延迟
	re := regexp.MustCompile(`time=([0-9.]+)\s*ms`)
	matches := re.FindAllStringSubmatch(string(output), -1)
	if len(matches) > 0 {
		for _, m := range matches {
			result.Latencies = append(result.Latencies, m[1])
		}
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonBytes), nil
}

func (t *NetworkCheckTool) dnsLookup(target string, timeout int) (string, error) {
	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// 先尝试作为域名解析
	ips, err := resolver.LookupHost(ctx, target)
	if err != nil {
		// 尝试反向解析
		names, err2 := resolver.LookupAddr(ctx, target)
		if err2 != nil {
			return "", fmt.Errorf("DNS 解析失败: %v", err)
		}
		result := DNSResult{
			Target:  target,
			Domains: names,
			Reverse: true,
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		return string(jsonBytes), nil
	}

	result := DNSResult{
		Target: target,
		IPs:    ips,
		Reverse: false,
	}
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonBytes), nil
}

func (t *NetworkCheckTool) portCheck(target string, timeout int) (string, error) {
	// 解析 host:port
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		// 如果没有端口，尝试常见端口
		host = target
		port = "80"
	}

	conn, err := net.DialTimeout("tcp", host+":"+port, time.Duration(timeout)*time.Second)
	result := PortResult{
		Host: host,
		Port: port,
		Open: err == nil,
	}
	if err != nil {
		result.Error = err.Error()
	}
	if conn != nil {
		conn.Close()
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonBytes), nil
}

func (t *NetworkCheckTool) httpCheck(target string, timeout int) (string, error) {
	start := time.Now()

	// 确保 URL 格式
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "https://" + target
	}

	cmd := exec.Command("curl", "-I", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--connect-timeout", fmt.Sprintf("%d", timeout), target)
	output, err := cmd.Output()

	elapsed := time.Since(start)

	result := HTTPResult{
		URL:      target,
		Duration: elapsed.String(),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.StatusCode = strings.TrimSpace(string(output))
		result.Success = true
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonBytes), nil
}

func init() {
	GlobalRegistry.Register("network_check", func() Tool {
		return NewNetworkCheckTool()
	})
}