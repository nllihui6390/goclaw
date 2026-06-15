package main

import (
	"context"
	"fmt"

	"github.com/nllihui6390/go-agent"
	"github.com/nllihui6390/go-agent/memory"
	"github.com/nllihui6390/go-agent/model"
	"github.com/nllihui6390/go-agent/tool"
)

// EchoTool 示例工具
type EchoTool struct{}

func (t *EchoTool) Name() string        { return "echo" }
func (t *EchoTool) Description() string { return "回显输入文本" }
func (t *EchoTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"text": map[string]interface{}{
				"type":        "string",
				"description": "要回显的文本",
			},
		},
		"required": []string{"text"},
	}
}
func (t *EchoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	text, ok := params["text"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 text 参数")
	}
	return "回显: " + text, nil
}

func main() {
	// =============================================
	// 示例 1: AgentScope 风格的消息创建
	// =============================================
	fmt.Println("=== 示例 1: AgentScope 风格的消息创建 ===")
	messageExample()

	// =============================================
	// 示例 2: ContentBlock 类型化内容
	// =============================================
	fmt.Println("\n=== 示例 2: ContentBlock 类型化内容 ===")
	contentBlockExample()

	// =============================================
	// 示例 3: 事件流处理
	// =============================================
	fmt.Println("\n=== 示例 3: 事件流处理 ===")
	eventExample()

	// =============================================
	// 示例 4: 会话管理
	// =============================================
	fmt.Println("\n=== 示例 4: 会话管理 ===")
	sessionExample()

	// =============================================
	// 示例 5: 完整 Agent 使用
	// =============================================
	fmt.Println("\n=== 示例 5: 完整 Agent 使用 ===")
	agentExample()
}

func messageExample() {
	// AgentScope 风格的消息工厂函数
	userMsg := agent.UserMsg("user", "你好，最近怎么样？")
	assistantMsg := agent.AssistantMsg("assistant", "我很好，谢谢！")
	systemMsg := agent.SystemMsg("system", "你是一个有帮助的助手。")

	fmt.Printf("用户消息: role=%s, content=%s\n", userMsg.Role, userMsg.GetTextContent())
	fmt.Printf("助手消息: role=%s, content=%s\n", assistantMsg.Role, assistantMsg.GetTextContent())
	fmt.Printf("系统消息: role=%s, content=%s\n", systemMsg.Role, systemMsg.GetTextContent())

	// 向后兼容的便捷函数
	simpleUserMsg := agent.NewUserMsg("简单消息")
	fmt.Printf("简单用户消息: %s\n", simpleUserMsg.GetTextContent())

	// 多模态消息（文本 + 图片）
	multimodalMsg := agent.UserMsg("user", []agent.ContentBlock{
		agent.NewTextBlock("描述这张图片："),
		agent.NewDataBlockURL("https://example.com/image.png", "image/png"),
	})
	fmt.Printf("多模态消息: %d 个内容块\n", len(multimodalMsg.Content))
}

func contentBlockExample() {
	// 创建各种类型的内容块
	textBlock := agent.NewTextBlock("这是文本内容")
	thinkingBlock := agent.NewThinkingBlock("让我思考一下...")
	toolCallBlock := agent.NewToolCallBlock("call_123", "echo", `{"text": "hello"}`)
	toolResultBlock := agent.NewToolResultTextBlock("call_123", "回显: hello")
	hintBlock := agent.NewHintBlock("hint_1", "之前对话的上下文", "memory")

	fmt.Printf("文本块: type=%s, text=%s\n", textBlock.Type, textBlock.Text)
	fmt.Printf("思考块: type=%s, thinking=%s\n", thinkingBlock.Type, thinkingBlock.Thinking)
	fmt.Printf("工具调用块: type=%s, name=%s, id=%s\n", toolCallBlock.Type, toolCallBlock.ToolCallName, toolCallBlock.ToolCallID)
	fmt.Printf("工具结果块: type=%s, state=%s\n", toolResultBlock.Type, toolResultBlock.ToolResultState)
	fmt.Printf("提示块: type=%s, source=%s\n", hintBlock.Type, hintBlock.HintSource)

	// 创建包含多种内容块的消息
	msg := agent.AssistantMsg("assistant", []agent.ContentBlock{
		textBlock,
		thinkingBlock,
		toolCallBlock,
		toolResultBlock,
	})

	fmt.Printf("\n多内容块消息:\n")
	fmt.Printf("  包含文本: %v\n", msg.HasContentBlocks(agent.BlockTypeText))
	fmt.Printf("  包含思考: %v\n", msg.HasContentBlocks(agent.BlockTypeThinking))
	fmt.Printf("  包含工具调用: %v\n", msg.HasToolCalls())
	fmt.Printf("  包含工具结果: %v\n", msg.HasToolResults())
	fmt.Printf("  文本内容: %s\n", msg.GetTextContent())
}

func eventExample() {
	// 创建事件流（模拟 Agent 执行过程）
	replyID := "reply_001"
	sessionID := "session_001"

	// 生命周期事件
	replyStart := agent.NewReplyStartEvent(replyID, sessionID, "assistant")
	fmt.Printf("事件: %T, reply_id=%s\n", replyStart, replyStart.ReplyID)

	// 文本流式事件 (start → delta → end)
	blockID := "block_001"
	textStart := agent.NewTextBlockStartEvent(replyID, blockID)
	textDelta1 := agent.NewTextBlockDeltaEvent(replyID, blockID, "你好")
	textDelta2 := agent.NewTextBlockDeltaEvent(replyID, blockID, " 世界")
	textEnd := agent.NewTextBlockEndEvent(replyID, blockID)

	fmt.Printf("文本块开始: block_id=%s\n", textStart.BlockID)
	fmt.Printf("文本块增量: delta=%s\n", textDelta1.Delta)
	fmt.Printf("文本块增量: delta=%s\n", textDelta2.Delta)
	fmt.Printf("文本块结束: block_id=%s\n", textEnd.BlockID)

	// 工具调用事件
	toolCallStart := agent.NewToolCallStartEvent(replyID, "tc_001", "echo")
	toolCallEnd := agent.NewToolCallEndEvent(replyID, "tc_001")
	fmt.Printf("工具调用开始: name=%s\n", toolCallStart.ToolCallName)
	fmt.Printf("工具调用结束: id=%s\n", toolCallEnd.ToolCallID)

	// 工具结果事件
	toolResultStart := agent.NewToolResultStartEvent(replyID, "tc_001", "echo")
	toolResultDelta := agent.NewToolResultTextDeltaEvent(replyID, "tc_001", "回显: hello")
	toolResultEnd := agent.NewToolResultEndEvent(replyID, "tc_001", agent.ToolResultStateSuccess)
	fmt.Printf("工具结果开始: name=%s\n", toolResultStart.ToolCallName)
	fmt.Printf("工具结果增量: delta=%s\n", toolResultDelta.Delta)
	fmt.Printf("工具结果结束: state=%s\n", toolResultEnd.State)

	// 回复结束
	replyEnd := agent.NewReplyEndEvent(replyID, sessionID)
	fmt.Printf("回复结束: reply_id=%s\n", replyEnd.ReplyID)

	// 演示从事件流重建消息
	fmt.Println("\n--- 从事件流重建消息 ---")
	events := []interface{}{
		replyStart,
		textStart,
		textDelta1,
		textDelta2,
		textEnd,
		toolCallStart,
		toolCallEnd,
		toolResultStart,
		toolResultDelta,
		toolResultEnd,
		replyEnd,
	}

	msg := &agent.Msg{
		ID:        replyID,
		Name:      "assistant",
		Role:      agent.RoleAssistant,
		Content:   []agent.ContentBlock{},
		CreatedAt: replyStart.CreatedAt,
	}

	for _, event := range events {
		if !agent.IsReplyStart(event) && !agent.IsReplyEnd(event) {
			msg.AppendEvent(event)
		}
	}

	fmt.Printf("重建的消息: role=%s, text=%s, blocks=%d\n",
		msg.Role, msg.GetTextContent(), len(msg.Content))
}

func sessionExample() {
	// 创建会话
	session := agent.NewSession(agent.NewInMemorySessionStore())

	// 添加消息（使用新的消息格式）
	session.AddMessage(agent.UserMsg("user", "第一条消息"))
	session.AddMessage(agent.AssistantMsg("assistant", "第一条回复"))
	session.AddMessage(agent.UserMsg("user", "第二条消息"))

	// 获取历史
	history := session.GetHistory()
	fmt.Printf("会话 ID: %s\n", session.GetID())
	fmt.Printf("历史记录长度: %d\n", len(history))

	for i, msg := range history {
		fmt.Printf("  %d. [%s] %s\n", i+1, msg.Role, msg.GetTextContent())
	}

	// 清除会话
	session.Clear()
	fmt.Println("会话已清除，历史记录长度:", len(session.GetHistory()))
}

func agentExample() {
	// 创建模型（需要真实 API Key）
	llm := model.NewOpenAIModel(model.ModelConfig{
		Model:   "deepseek-v4-oc",
		APIKey:  "API Key", // 替换为真实 API Key
		BaseURL: "https://eaichat.ctyun.cn/ai/platform/v2/cp",
		Timeout: 30,
	})

	// 创建工具注册表
	tools := tool.NewRegistry()
	tools.Register(&EchoTool{})

	// 创建内存
	mem := memory.NewSimpleMemory()

	// 创建 Agent
	ag := agent.NewAgent(*agent.DefaultConfig("assistant", llm, "You are a helpful assistant.").WithTools(tools).WithMemory(mem).WithMaxIters(10))

	fmt.Printf("Agent 已创建: %s\n", ag.GetName())
	fmt.Printf("模型: %s (%s)\n", llm.GetName(), llm.GetProvider())
	fmt.Printf("工具: 已注册 %d 个\n", tools.Count())

	// 同步调用示例（需要真实 API Key 才能运行）
	// ctx := context.Background()
	// userMsg := agent.UserMsg("user", "你好！")
	// reply, err := ag.Reply(ctx, userMsg)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Printf("回复: %s\n", reply.GetTextContent())

	// 流式调用示例
	// eventStream, err := ag.ReplyStream(ctx, agent.UserMsg("user", "给我讲个故事"))
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// for event := range eventStream {
	// 	switch e := event.(type) {
	// 	case agent.TextBlockDeltaEvent:
	// 		fmt.Print(e.Delta)
	// 	case agent.ToolCallStartEvent:
	// 		fmt.Printf("\n[调用工具: %s]\n", e.ToolCallName)
	// 	case agent.ReplyEndEvent:
	// 		fmt.Println("\n[完成]")
	// 	}
	// }

	fmt.Println("(设置 OPENAI_API_KEY 以运行实际的 API 调用)")
}
