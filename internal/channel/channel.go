package channel

import "context"

// Message 标准化消息
type Message struct {
	ID        string
	Channel   string
	From      string
	Content   string
	Agent     string                 // 目标Agent名称（由用户指定，为空时使用默认）
	Timestamp int64
	Metadata  map[string]any
}

// Response 响应消息
type Response struct {
	Content string
	Channel string
	To      string
}

// Channel 渠道接口
type Channel interface {
	Send(ctx context.Context, resp Response) error
	Receive(ctx context.Context) (<-chan Message, error)
	GetName() string
	Start(ctx context.Context) error
	Stop() error
}

// ControlResponse 控制台控制响应
type ControlResponse struct {
	Message string
	IsCtrl  bool // true表示这是控制响应，不是AI回复
}
