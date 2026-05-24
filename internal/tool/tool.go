package tool

import "context"

// Tool 工具接口
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}
