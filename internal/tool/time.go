package tool

import (
	"context"
	"fmt"
	"time"
)

// TimeTool 获取当前时间工具
type TimeTool struct {
	userTimezone string
}

// NewTimeTool 创建时间工具
func NewTimeTool() *TimeTool {
	return &TimeTool{}
}

func (t *TimeTool) Name() string {
	return "get_current_time"
}

func (t *TimeTool) Description() string {
	return "获取当前日期和时间。返回格式化的日期时间字符串，包含时区信息。"
}

func (t *TimeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"format": map[string]interface{}{
				"type":        "string",
				"description": "时间格式，可选：'full'(完整)、'date'(仅日期)、'time'(仅时间)，默认 'full'",
			},
		},
	}
}

func (t *TimeTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	format := "full"
	if f, ok := params["format"].(string); ok {
		format = f
	}

	now := time.Now()
	loc := now.Location()
	tzName := loc.String()
	if tzName == "" || tzName == "Local" {
		tzName = "系统本地时区"
	}

	switch format {
	case "date":
		return fmt.Sprintf("当前日期: %s\n时区: %s", now.Format("2006-01-02"), tzName), nil
	case "time":
		return fmt.Sprintf("当前时间: %s\n时区: %s", now.Format("15:04:05"), tzName), nil
	default:
		return fmt.Sprintf("当前时间: %s\n日期: %s\n时间: %s\n时区: %s\nUnix时间戳: %d",
			now.Format("2006-01-02 15:04:05"),
			now.Format("2006-01-02"),
			now.Format("15:04:05"),
			tzName,
			now.Unix()), nil
	}
}

// SetTimezoneTool 设置用户时区工具
type SetTimezoneTool struct{}

func NewSetTimezoneTool() *SetTimezoneTool {
	return &SetTimezoneTool{}
}

func (t *SetTimezoneTool) Name() string {
	return "set_user_timezone"
}

func (t *SetTimezoneTool) Description() string {
	return "设置用户时区，影响时间显示和定时任务执行。传入时区名称如 'Asia/Shanghai'、'America/New_York'。"
}

func (t *SetTimezoneTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"timezone": map[string]interface{}{
				"type":        "string",
				"description": "IANA 时区名称，如 'Asia/Shanghai'、'America/New_York'、'Europe/London'",
			},
		},
		"required": []string{"timezone"},
	}
}

func (t *SetTimezoneTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	tz, ok := params["timezone"].(string)
	if !ok || tz == "" {
		return "", fmt.Errorf("缺少 timezone 参数")
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return "", fmt.Errorf("无效的时区名称: %s。请使用 IANA 时区名称，如 'Asia/Shanghai'", tz)
	}

	now := time.Now().In(loc)
	return fmt.Sprintf("时区已设置为: %s\n当前时间: %s", tz, now.Format("2006-01-02 15:04:05")), nil
}

func init() {
	GlobalRegistry.Register("get_current_time", func() Tool { return NewTimeTool() })
	GlobalRegistry.Register("set_user_timezone", func() Tool { return NewSetTimezoneTool() })
}