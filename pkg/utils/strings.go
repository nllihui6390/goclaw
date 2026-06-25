package utils

import (
	"encoding/json"
	"strings"
)

// Truncate 截断字符串到指定最大长度，超出部分省略号代替
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// TruncateJSON 序列化 JSON 并截断到指定最大长度
func TruncateJSON(v any, n int) string {
	data, _ := json.Marshal(v)
	s := string(data)
	return Truncate(s, n)
}

// ContainsColon 检查字符串是否包含冒号
func ContainsColon(s string) bool {
	return strings.ContainsRune(s, ':')
}
