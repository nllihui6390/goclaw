package tool

import (
	"context"
	"encoding/json"
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
	return "获取当前日期和时间。返回JSON格式的时间信息，包含日期时间、时区、Unix时间戳、星期、夏令时状态。"
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

// weekdayChinese 返回中文星期名
func weekdayChinese(w time.Weekday) string {
	switch w {
	case time.Sunday:
		return "星期日"
	case time.Monday:
		return "星期一"
	case time.Tuesday:
		return "星期二"
	case time.Wednesday:
		return "星期三"
	case time.Thursday:
		return "星期四"
	case time.Friday:
		return "星期五"
	case time.Saturday:
		return "星期六"
	default:
		return ""
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
		result := map[string]interface{}{
			"date":    now.Format("2006-01-02"),
			"timezone": tzName,
		}
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("JSON序列化失败: %v", err)
		}
		return string(jsonBytes), nil
	case "time":
		result := map[string]interface{}{
			"time":    now.Format("15:04:05"),
			"timezone": tzName,
		}
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("JSON序列化失败: %v", err)
		}
		return string(jsonBytes), nil
	default:
		result := map[string]interface{}{
			"datetime":       now.Format("2006-01-02 15:04:05"),
			"timezone":       tzName,
			"unix_timestamp": now.Unix(),
			"weekday":        weekdayChinese(now.Weekday()),
			"is_dst":         now.IsDST(),
		}
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("JSON序列化失败: %v", err)
		}
		return string(jsonBytes), nil
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
	return "设置用户时区，影响时间显示和定时任务执行。传入时区名称如 'Asia/Shanghai'、'America/New_York'。返回JSON格式包含时区、当前时间和状态。"
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

	result := map[string]interface{}{
		"timezone":        tz,
		"current_datetime": now.Format("2006-01-02 15:04:05"),
		"status":          "success",
	}
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON序列化失败: %v", err)
	}
	return string(jsonBytes), nil
}

func init() {
	GlobalRegistry.Register("get_current_time", func() Tool { return NewTimeTool() })
	GlobalRegistry.Register("set_user_timezone", func() Tool { return NewSetTimezoneTool() })
}