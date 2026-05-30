package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ManageConfigTool 配置管理工具
type ManageConfigTool struct{}

func NewManageConfigTool() *ManageConfigTool {
	return &ManageConfigTool{}
}

func (t *ManageConfigTool) Name() string {
	return "manage_config"
}

func (t *ManageConfigTool) Description() string {
	return `读取和修改 go-claw 的配置文件 config.json。
支持查看配置、修改配置项、重启后生效。

调用格式：
- manage_config(action="get", path="gateway")  # 查看配置
- manage_config(action="get", path="agents.0.name")  # 查看特定配置项
- manage_config(action="set", path="gateway.session_ttl", value="60")  # 修改配置
- manage_config(action="list")  # 列出所有配置项

参数说明：
- action: 操作类型: get, set, list（必填）
- path: 配置路径，支持点号分隔嵌套（如 agents.0.name）
- value: 设置的值（action=set 时必填）

注意：
- 修改后需要重启或开启热加载才能生效
- 设置 GOCLAW_HOT_RELOAD=true 开启热加载`
}

func (t *ManageConfigTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "操作类型: get, set, list",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "配置路径，支持点号分隔嵌套",
			},
			"value": map[string]interface{}{
				"type":        "string",
				"description": "设置的值（JSON 格式或字符串）",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ManageConfigTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	action, ok := params["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("缺少 action 参数")
	}

	configPath := "config.json"

	switch strings.ToLower(action) {
	case "list":
		return t.listConfig(configPath)
	case "get":
		path, ok := params["path"].(string)
		if !ok || path == "" {
			return "", fmt.Errorf("缺少 path 参数")
		}
		return t.getConfig(configPath, path)
	case "set":
		path, ok := params["path"].(string)
		if !ok || path == "" {
			return "", fmt.Errorf("缺少 path 参数")
		}
		value, ok := params["value"].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("缺少 value 参数")
		}
		return t.setConfig(configPath, path, value)
	default:
		return "", fmt.Errorf("未知操作: %s (支持: get, set, list)", action)
	}
}

func (t *ManageConfigTool) listConfig(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("解析配置文件失败: %v", err)
	}

	result := "## 配置项列表\n\n"
	result += formatConfigTree(config, 0)

	return result, nil
}

func (t *ManageConfigTool) getConfig(configPath, path string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("解析配置文件失败: %v", err)
	}

	value, err := getNestedValue(config, path)
	if err != nil {
		return "", err
	}

	// 格式化输出
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("## 配置项: %s\n\n%v", path, value), nil
	}

	return fmt.Sprintf("## 配置项: %s\n\n%s", path, string(formatted)), nil
}

func (t *ManageConfigTool) setConfig(configPath, path, value string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 解析 value
	var parsedValue interface{}
	// 先尝试解析为 JSON
	if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
		// 不是 JSON，作为字符串处理
		parsedValue = value
	}

	// 设置嵌套值
	if err := setNestedValue(config, path, parsedValue); err != nil {
		return "", err
	}

	// 写回配置文件
	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return "", fmt.Errorf("写入配置文件失败: %v", err)
	}

	return fmt.Sprintf("✅ 配置已修改\n路径: %s\n值: %v\n\n如需立即生效，请设置 GOCLAW_HOT_RELOAD=true 或重启服务", path, parsedValue), nil
}

func getNestedValue(config map[string]interface{}, path string) (interface{}, error) {
	keys := strings.Split(path, ".")
	current := interface{}(config)

	for _, key := range keys {
		// 处理数组索引
		if isNumeric(key) {
			idx := mustAtoi(key)
			arr, ok := current.([]interface{})
			if !ok || idx >= len(arr) {
				return nil, fmt.Errorf("路径 %s 不存在（索引超出范围）", path)
			}
			current = arr[idx]
			continue
		}

		obj, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("路径 %s 不存在", path)
		}
		val, exists := obj[key]
		if !exists {
			return nil, fmt.Errorf("配置项 %s 不存在", path)
		}
		current = val
	}

	return current, nil
}

func setNestedValue(config map[string]interface{}, path string, value interface{}) error {
	keys := strings.Split(path, ".")
	current := interface{}(config)

	// 遍历到最后一层
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]

		if isNumeric(key) {
			idx := mustAtoi(key)
			arr, ok := current.([]interface{})
			if !ok || idx >= len(arr) {
				return fmt.Errorf("路径 %s 不存在", path)
			}
			current = arr[idx]
		} else {
			obj, ok := current.(map[string]interface{})
			if !ok {
				return fmt.Errorf("路径 %s 不存在", path)
			}
			val, exists := obj[key]
			if !exists {
				// 创建新节点
				obj[key] = map[string]interface{}{}
				val = obj[key]
			}
			current = val
		}
	}

	// 设置最后一层
	lastKey := keys[len(keys)-1]
	obj, ok := current.(map[string]interface{})
	if !ok {
		return fmt.Errorf("无法设置值，路径 %s 的父级不是对象", path)
	}
	obj[lastKey] = value

	return nil
}

func formatConfigTree(config map[string]interface{}, depth int) string {
	var sb strings.Builder
	prefix := strings.Repeat("  ", depth)

	for key, value := range config {
		switch v := value.(type) {
		case map[string]interface{}:
			sb.WriteString(fmt.Sprintf("%s- %s:\n", prefix, key))
			sb.WriteString(formatConfigTree(v, depth+1))
		case []interface{}:
			sb.WriteString(fmt.Sprintf("%s- %s: [数组, %d 项]\n", prefix, key, len(v)))
		default:
			sb.WriteString(fmt.Sprintf("%s- %s: %v\n", prefix, key, v))
		}
	}

	return sb.String()
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func mustAtoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func init() {
	GlobalRegistry.Register("manage_config", func() Tool {
		return NewManageConfigTool()
	})
}