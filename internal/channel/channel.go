package channel

import "context"

// Message 标准化消息
type Message struct {
	ID        string
	Channel   string
	From      string
	Content   string
	Timestamp int64
	Metadata  map[string]interface{}
}

// Response 响应消息
type Response struct {
	Content string
	Channel string
	To      string
}

// Channel 渠道接口
type Channel interface {
	// Send 发送消息
	Send(ctx context.Context, resp Response) error
	// Receive 接收消息（返回消息通道）
	Receive(ctx context.Context) (<-chan Message, error)
	// GetName 获取渠道名称
	GetName() string
	// Start 启动渠道
	Start(ctx context.Context) error
	// Stop 停止渠道
	Stop() error
}
