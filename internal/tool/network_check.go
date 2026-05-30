package tool

import (
	"context"
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

	result := fmt.Sprintf("## Ping 测试\n\n目标: %s\n\n%s", target, string(output))

	// 解析延迟
	re := regexp.MustCompile(`time=([0-9.]+)\s*ms`)
	matches := re.FindAllStringSubmatch(string(output), -1)
	if len(matches) > 0 {
		var latencies []string
		for _, m := range matches {
			latencies = append(latencies, m[1])
		}
		result += fmt.Sprintf("\n延迟: %s ms", strings.Join(latencies, ", "))
	}

	if err != nil {
		result += fmt.Sprintf("\n⚠️ %v", err)
	}

	return result, nil
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
		result := fmt.Sprintf("## DNS 反向解析\n\nIP: %s\n域名:\n", target)
		for _, name := range names {
			result += fmt.Sprintf("- %s\n", name)
		}
		return result, nil
	}

	result := fmt.Sprintf("## DNS 解析\n\n域名: %s\nIP 地址:\n", target)
	for _, ip := range ips {
		result += fmt.Sprintf("- %s\n", ip)
	}
	return result, nil
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
	if err != nil {
		return fmt.Sprintf("## 端口检测\n\n目标: %s:%s\n状态: ❌ 无法连接\n错误: %v", host, port, err), nil
	}
	defer conn.Close()

	return fmt.Sprintf("## 端口检测\n\n目标: %s:%s\n状态: ✅ 端口开放", host, port), nil
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

	result := fmt.Sprintf("## HTTP 连通测试\n\n目标: %s\n耗时: %v\n", target, elapsed)

	if err != nil {
		result += fmt.Sprintf("状态: ❌ 连接失败\n错误: %v", err)
		return result, nil
	}

	statusCode := strings.TrimSpace(string(output))
	result += fmt.Sprintf("HTTP 状态码: %s", statusCode)

	return result, nil
}

func init() {
	GlobalRegistry.Register("network_check", func() Tool {
		return NewNetworkCheckTool()
	})
}
