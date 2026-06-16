package channel

import "fmt"

// Renderer 消息渲染器：将 ToolEvent 渲染为渠道可发送的文本
type Renderer struct {
	Style RenderStyle
}

// RenderStyle 渲染风格配置
type RenderStyle struct {
	ShowToolDetails    bool // 是否展示工具调用的详细参数和结果
	SupportsMarkdown   bool // 渠道是否支持 Markdown
	UseEmoji           bool // 是否使用 Emoji
	FilterToolMessages bool // 是否过滤工具调用消息
	FilterThinking     bool // 是否过滤思考内容
}

// DefaultRenderStyle 默认渲染风格
func DefaultRenderStyle() RenderStyle {
	return RenderStyle{
		ShowToolDetails:    true,
		SupportsMarkdown:   true,
		UseEmoji:           true,
		FilterToolMessages: false,
		FilterThinking:     false,
	}
}

// RenderToolEvent 将 ToolEvent 渲染为渠道可发送的文本
func (r *Renderer) RenderToolEvent(event ToolEvent) string {
	if event.Type == ToolEventThinking && r.Style.FilterThinking {
		return ""
	}
	if (event.Type == ToolEventCalling || event.Type == ToolEventResult || event.Type == ToolEventError) && r.Style.FilterToolMessages {
		return ""
	}

	switch event.Type {
	case ToolEventThinking:
		if event.Thinking == "" {
			return ""
		}
		if r.Style.UseEmoji {
			return fmt.Sprintf("💭 **思考中**\n%s", truncateStr(event.Thinking, 500))
		}
		return fmt.Sprintf("**思考中**\n%s", truncateStr(event.Thinking, 500))

	case ToolEventCalling:
		args := event.Args
		if !r.Style.ShowToolDetails {
			args = "..."
		}
		prefix := "🔧 **"
		suffix := "**"
		if !r.Style.UseEmoji {
			prefix = "**"
		}
		return fmt.Sprintf("%s%s%s\n```\n%s\n```", prefix, event.ToolName, suffix, truncateStr(args, 500))

	case ToolEventResult:
		result := event.Result
		if !r.Style.ShowToolDetails {
			result = "..."
		}
		prefix := "✅ **"
		suffix := "**"
		if !r.Style.UseEmoji {
			prefix = "**"
		}
		return fmt.Sprintf("%s%s%s\n```\n%s\n```", prefix, event.ToolName, suffix, truncateStr(result, 500))

	case ToolEventError:
		prefix := "❌ **"
		suffix := "**"
		if !r.Style.UseEmoji {
			prefix = "**"
		}
		return fmt.Sprintf("%s%s%s\n```\n%s\n```", prefix, event.ToolName, suffix, event.Error)

	case ToolEventFile:
		return fmt.Sprintf("📎 已发送文件: %s", event.ToolName)

	case ToolEventContent:
		return "" // 结构化内容块不渲染为文本

	case ToolEventText:
		// 流式文本增量：直接返回文本内容
		if event.Thinking != "" {
			return event.Thinking
		}
		return ""
	}

	return ""
}

// truncateStr 截断字符串到指定最大长度
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
