package main

import (
	"context"
	"fmt"
	"log"

	"github.com/nllihui6390/go-agent"
	"github.com/nllihui6390/go-agent/memory"
	"github.com/nllihui6390/go-agent/model"
	"github.com/nllihui6390/go-agent/tool"
)

// CalculatorTool 计算器工具
type CalculatorTool struct{}

func (t *CalculatorTool) Name() string        { return "calculator" }
func (t *CalculatorTool) Description() string { return "执行基本算术计算" }
func (t *CalculatorTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"expression": map[string]interface{}{
				"type":        "string",
				"description": "算术表达式，如 '2 + 2' 或 '10 * 5'",
			},
		},
		"required": []string{"expression"},
	}
}
func (t *CalculatorTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	expr, ok := params["expression"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 expression 参数")
	}

	// 简单实现 - 实际应用中应该使用安全的表达式求值器
	switch expr {
	case "2 + 2":
		return "结果: 4", nil
	case "10 * 5":
		return "结果: 50", nil
	case "100 / 10":
		return "结果: 10", nil
	case "8 - 3":
		return "结果: 5", nil
	default:
		return fmt.Sprintf("错误: 不支持的表达式 %s", expr), nil
	}
}

func main() {
	fmt.Println("=== Go-Agent 演示 ===")

	// 1. 创建模型配置（使用环境变量）
	// apiKey := os.Getenv("OPENAI_API_KEY")
	// if apiKey == "" {
	// 	fmt.Println("警告: 未设置 OPENAI_API_KEY，使用模拟模型。")
	// 	demoMockModel()
	// 	return
	// }

	// 2. 创建 OpenAI 模型
	llm := model.NewOpenAIModel(model.ModelConfig{
		Model:     "deepseek-v4-oc",
		APIKey:    "API Key", // 替换为真实 API Key
		BaseURL:   "https://eaichat.ctyun.cn/ai/platform/v2/cp",
		Timeout:   30,
		MaxTokens: 500,
	})

	// 3. 创建工具注册表
	tools := tool.NewRegistry()
	tools.Register(&CalculatorTool{})

	// 4. 创建内存
	mem := memory.NewSimpleMemory()

	// 5. 创建 Agent（使用 BYPASS 模式自动允许工具调用）
	ag := agent.NewAgent(*agent.DefaultConfig("demo-agent", llm, "你是一个有帮助的助手。当用户问数学计算问题时，你必须使用 calculator 工具来计算，不要直接回答。").WithTools(tools).WithMemory(mem).WithMaxIters(10).WithPermission(agent.NewPermissionChecker(&tool.PermissionContext{
		Mode: tool.ModeBypass,
	})))

	fmt.Printf("Agent 已创建: %s\n", ag.GetName())
	fmt.Printf("模型: %s (%s)\n", llm.GetName(), llm.GetProvider())
	fmt.Printf("工具: 已注册 %d 个\n", tools.Count())

	// 6. 示例对话
	ctx := context.Background()

	messages := []string{
		"你好！",
		"用计算器计算 2 + 2",
		"用计算器帮我计算 10 * 5",
	}

	for _, msg := range messages {
		fmt.Printf("\n用户: %s\n", msg)

		// 使用流式调用以观察过程
		eventStream, err := ag.ReplyStream(ctx, agent.UserMsg("user", msg))
		if err != nil {
			log.Printf("错误: %v\n", err)
			continue
		}

		fmt.Printf("助手: ")
		for event := range eventStream {
			switch e := event.(type) {
			case agent.ReplyStartEvent:
				// 回复开始
			case agent.ModelCallStartEvent:
				fmt.Printf("\n  [调用模型: %s]\n", e.ModelName)
			case agent.TextBlockStartEvent:
				// 文本块开始
			case agent.TextBlockDeltaEvent:
				fmt.Print(e.Delta)
			case agent.TextBlockEndEvent:
				// 文本块结束
			case agent.ThinkingBlockStartEvent:
				fmt.Printf("\n  [思考开始]\n")
			case agent.ThinkingBlockDeltaEvent:
				fmt.Printf("%s", e.Delta)
			case agent.ThinkingBlockEndEvent:
				fmt.Printf("\n  [思考结束]\n")
			case agent.ToolCallStartEvent:
				fmt.Printf("\n  [调用工具: %s, ID: %s]\n", e.ToolCallName, e.ToolCallID)
			case agent.ToolCallDeltaEvent:
				fmt.Printf("    参数: %s\n", string(e.Delta))
			case agent.ToolCallEndEvent:
				fmt.Printf("  [工具调用结束]\n")
			case agent.ToolResultStartEvent:
				fmt.Printf("  [工具结果开始: %s]\n", e.ToolCallID)
			case agent.ToolResultTextDeltaEvent:
				fmt.Printf("    结果: %s\n", e.Delta)
			case agent.ToolResultEndEvent:
				fmt.Printf("  [工具结果结束, 状态: %s]\n", e.State)
			case agent.ModelCallEndEvent:
				// 模型调用结束
			case agent.ReplyEndEvent:
				fmt.Printf("\n  [回复结束]\n")
			}
		}
	}

	fmt.Println("\n=== 演示完成 ===")
}

// demoMockModel 演示使用模拟模型
func demoMockModel() {
	fmt.Println("\n=== 模拟模型演示 ===")

	// 创建模拟模型
	mock := &MockModel{
		name:     "mock-gpt",
		provider: "mock",
	}

	// 创建工具注册表
	tools := tool.NewRegistry()
	tools.Register(&CalculatorTool{})

	// 创建 Agent
	ag := agent.NewAgent(*agent.DefaultConfig("mock-agent", mock, "You are a helpful assistant.").WithTools(tools).WithMaxIters(10))

	ctx := context.Background()

	// 测试工具调用
	fmt.Println("\n测试工具调用...")
	reply, err := ag.Reply(ctx, agent.UserMsg("user", "计算 2 + 2"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("助手: %s\n", reply.GetTextContent())

	// 检查会话
	session := ag.GetSession()
	fmt.Printf("\n会话 ID: %s\n", session.GetID())
	fmt.Printf("会话历史: %d 条消息\n", len(session.GetHistory()))

	// 演示事件流
	fmt.Println("\n--- 事件流演示 ---")
	eventStream, err := ag.ReplyStream(ctx, agent.UserMsg("user", "你好"))
	if err != nil {
		log.Fatal(err)
	}

	for event := range eventStream {
		switch e := event.(type) {
		case agent.ReplyStartEvent:
			fmt.Printf("[回复开始: %s]\n", e.ReplyID)
		case agent.TextBlockDeltaEvent:
			fmt.Print(e.Delta)
		case agent.ToolCallStartEvent:
			fmt.Printf("\n[调用工具: %s]\n", e.ToolCallName)
		case agent.ToolResultEndEvent:
			fmt.Printf("[工具结果: %s]\n", e.State)
		case agent.ReplyEndEvent:
			fmt.Println("\n[完成]")
		}
	}

	fmt.Println("\n=== 模拟演示完成 ===")
}

// MockModel 模拟模型实现
type MockModel struct {
	name     string
	provider string
}

func (m *MockModel) Call(ctx context.Context, messages []model.Msg) (*model.Response, error) {
	// 模拟响应
	return &model.Response{
		Content: "我可以帮你计算，我会使用计算器工具。",
		ToolCalls: []model.ToolCall{
			{
				ID:   "call_123",
				Name: "calculator",
				Params: map[string]interface{}{
					"expression": "2 + 2",
				},
			},
		},
		Usage: model.Usage{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
		StopReason: "tool_calls",
	}, nil
}

func (m *MockModel) Stream(ctx context.Context, messages []model.Msg) (<-chan model.StreamChunk, error) {
	// 模拟流式响应
	ch := make(chan model.StreamChunk, 10)
	go func() {
		ch <- model.StreamChunk{
			Type:    "content",
			Content: "你好！",
		}
		ch <- model.StreamChunk{
			Type:    "content",
			Content: "有什么可以帮你的？",
		}
		ch <- model.StreamChunk{Type: "done"}
		close(ch)
	}()
	return ch, nil
}

func (m *MockModel) GetName() string {
	return m.name
}

func (m *MockModel) GetProvider() string {
	return m.provider
}
