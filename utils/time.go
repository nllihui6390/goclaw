package utils

import "time"

// ParseTimeSafe 安全解析 RFC3339 时间字符串，失败返回零值
func ParseTimeSafe(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// FormatTimeSafe 格式化时间为 RFC3339，零值返回空字符串
func FormatTimeSafe(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
