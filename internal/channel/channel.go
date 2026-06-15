package channel

import "context"

// DisplayConfig 渠道显示控制配置
type DisplayConfig struct {
	ShowToolMessages bool // 显示工具调用和输出消息
	ShowThinking    bool // 显示模型思考/推理内容
	StreamOutput    bool // 流式输出
}

// DefaultDisplayConfig 默认显示配置（全开）
func DefaultDisplayConfig() DisplayConfig {
	return DisplayConfig{
		ShowToolMessages: true,
		ShowThinking:    true,
		StreamOutput:    true,
	}
}

// ShouldShowToolMessage 判断是否应显示工具类事件
func (d DisplayConfig) ShouldShowToolEvent(eventType ToolEventType) bool {
	// Guard 事件始终显示（安全审批通知，不受过滤影响）
	if eventType == ToolEventGuard {
		return true
	}
	if !d.ShowToolMessages {
		// 关闭时：不显示 calling、result、error
		if eventType == ToolEventCalling || eventType == ToolEventResult || eventType == ToolEventError {
			return false
		}
	}
	if !d.ShowThinking {
		if eventType == ToolEventThinking {
			return false
		}
	}
	return true
}

// Message 标准化消息
type Message struct {
	ID        string
	Channel   string
	From      string
	Content   string
	Agent     string                 // 目标Agent名称（由用户指定，为空时使用默认）
	Timestamp int64
	Metadata  map[string]any
	Blocks    ContentBlocks          // 结构化内容块（多模态：图片/文件等）
}

// Response 响应消息
type Response struct {
	Content string
	Channel string
	To      string
}

// ToolEventType 工具事件类型
type ToolEventType string

const (
	ToolEventThinking ToolEventType = "thinking" // Agent正在思考
	ToolEventCalling  ToolEventType = "calling"  // 正在调用工具
	ToolEventResult   ToolEventType = "result"   // 工具返回结果
	ToolEventError    ToolEventType = "error"    // 工具执行出错
	ToolEventFile     ToolEventType = "file"     // 文件发送事件
	ToolEventContent  ToolEventType = "content"  // 结构化内容块事件
	ToolEventGuard    ToolEventType = "guard"    // 安全守卫拦截事件
	ToolEventText     ToolEventType = "text"     // 流式文本增量（go-agent ReplyStream 逐 token 推送）
)

// ToolEvent 工具执行事件（用于实时输出工具调用过程）
type ToolEvent struct {
	Type     ToolEventType // 事件类型
	ToolName string        // 工具名称
	Args     string        // 工具参数（JSON）
	Result   string        // 工具返回结果
	Error    string        // 错误信息
	Thinking string        // 思考内容
	Content  ContentBlocks // 结构化内容块（用于 StructuredTool 返回结果）
	To       string        // 目标用户ID（用于指定发送对象）
	// Guard 事件专用字段
	GuardReason   string // 守卫拦截原因
	GuardMessage  string // 守卫提示消息（给用户看）
	ApprovalID    string // 审批ID
	ApprovalState string // 审批状态：pending/approved/denied
}

// ToolEventHandler 工具事件回调函数
type ToolEventHandler func(event ToolEvent)

// Channel 渠道接口
type Channel interface {
	Send(ctx context.Context, resp Response) error
	SendToolEvent(event ToolEvent) error // 发送工具执行事件
	Receive(ctx context.Context) (<-chan Message, error)
	GetName() string
	Start(ctx context.Context) error
	Stop() error
}

// ProactiveSender 主动消息发送接口（渠道可选实现）
// 用于不需要用户先发消息就能推送消息的场景（如定时任务结果推送）
type ProactiveSender interface {
	SendProactive(ctx context.Context, userID, content string) error
}

// ControlResponse 控制台控制响应
type ControlResponse struct {
	Message string
	IsCtrl  bool // true表示这是控制响应，不是AI回复
}
