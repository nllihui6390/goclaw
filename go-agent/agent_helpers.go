package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nllihui6390/go-agent/memory"
	"github.com/nllihui6390/go-agent/model"
	"github.com/nllihui6390/go-agent/tool"
)

// =============================================
// Observe — 观察消息
// =============================================

// Observe 将消息注入 Agent 上下文而不触发推理。
//
// 适用于多 Agent 场景：一个 Agent 观察另一个 Agent 的输出。
//
// 参数：
//   - msgs: 要注入的消息（可变参数，可传入多条）
//
// 示例：
//
//	// 在 多 Agent 场景中
//	primaryAgent.Reply(ctx, msg)
//	secondaryAgent.Observe(primaryAgent.GetSession().GetHistory()...)
func (a *Agent) Observe(msgs ...Msg) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, msg := range msgs {
		a.session.AddMessage(msg)
	}
}

// =============================================
// CompressContext — 上下文压缩
// =============================================

// CompressContext 手动触发上下文压缩。
// 当 token 用量超过 trigger_ratio × context_size 时压缩上下文。
// 可以为本次调用传入临时 ContextConfig 覆盖默认配置。
//
// 压缩流程：
//  1. 计算当前 token 总量
//  2. 与阈值比较
//  3. 切分消息：较旧部分压缩，最近部分保留
//  4. 生成结构化摘要
//  5. Offload 被压缩的消息（如果配置了 Offloader）
//  6. 更新会话（只保留 reserve 部分 + 摘要）
//
// 参数：
//   - ctx: 上下文
//   - config: 临时压缩配置，nil 则使用 Agent 的默认配置
//
// 返回：
//   - error: 压缩错误，nil 表示成功或无需压缩
//
// 示例：
//
//	// 使用默认配置
//	ag.CompressContext(ctx, nil)
//
//	// 使用临时配置（更激进压缩）
//	ag.CompressContext(ctx, &agent.ContextConfig{
//	    TriggerRatio: 0.5,
//	    ReserveRatio: 0.1,
//	})
func (a *Agent) CompressContext(ctx context.Context, config *ContextConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if config != nil {
		a.contextMgr.config = config
	}

	history := a.session.GetHistory()
	reserveMsgs, compressed, err := a.contextMgr.CheckAndCompress(
		ctx, a.session.GetID(), history, a.config.SystemPrompt)
	if err != nil {
		return err
	}

	if !compressed {
		return nil
	}

	summary := &Summary{
		TaskOverview:         "Previous conversation compressed",
		CurrentState:         "Continuing from compressed context",
		ImportantDiscoveries: []string{"Context was compressed due to token limit"},
		NextSteps:            []string{"Continue conversation"},
		ContextToPreserve:    "See offloaded context for details",
		CreatedAt:            nowISO(),
		TokenCount:           200,
	}

	a.contextMgr.SetSummary(summary)

	a.session.Clear()
	for _, msg := range reserveMsgs {
		a.session.AddMessage(msg)
	}

	return nil
}

// =============================================
// 模型调用（带重试和备用模型）
// =============================================

// callModelWithRetry 调用模型，主模型失败时自动重试并切换到备用模型。
//
// 重试策略：
//  1. 使用主模型调用，失败则重试（最多 MaxRetries 次）
//  2. 每次重试之间等待 RetryDelay 毫秒
//  3. 主模型全部失败后，尝试 FallbackModel（如果配置了）
//
// 参数：
//   - ctx: 上下文
//   - messages: 发送给模型的消息列表
//
// 返回：
//   - *model.Response: 模型响应
//   - error: 所有尝试失败后的最终错误
func (a *Agent) callModelWithRetry(ctx context.Context, messages []model.Msg) (*model.Response, error) {
	var lastErr error

	for retry := 0; retry < a.config.ModelConfig.MaxRetries; retry++ {
		response, err := a.config.Model.Call(ctx, messages)
		if err == nil {
			return response, nil
		}
		lastErr = err

		if retry < a.config.ModelConfig.MaxRetries-1 {
			time.Sleep(time.Duration(a.config.ModelConfig.RetryDelay) * time.Millisecond)
		}
	}

	if a.config.ModelConfig.FallbackModel != nil {
		response, err := a.config.ModelConfig.FallbackModel.Call(ctx, messages)
		if err == nil {
			return response, nil
		}
		return nil, fmt.Errorf("both primary and fallback models failed: primary=%v, fallback=%v", lastErr, err)
	}

	return nil, fmt.Errorf("model call failed after %d retries: %v", a.config.ModelConfig.MaxRetries, lastErr)
}

// =============================================
// 工具调用处理（含权限检查）
// =============================================

// handleToolCallsWithPermission 处理工具调用列表，逐一进行权限检查和执行。
//
//  1. 执行 PhaseActing 中间件
//  2. 调用 PermissionChecker.Check() 进行权限判断
//  3. 根据判断结果：
//     - ALLOW：直接执行工具，结果截断检查
//     - ASK：在 Reply 中返回拒绝（同步模式下无法暂停），在 ReplyStream 中发出 RequireUserConfirmEvent
//     - DENY：返回拒绝结果给 LLM
//     - EXTERNAL：在 ReplyStream 中发出 RequireExternalExecutionEvent
//
// 参数：
//   - ctx: 上下文
//   - toolCalls: 模型返回的工具调用列表
//
// 返回：
//   - *Msg: nil 表示正常继续；非 nil 表示暂停等待外部事件
//   - bool: true 表示暂停（需要用户确认或外部执行）
//   - error: 处理错误
func (a *Agent) handleToolCallsWithPermission(ctx context.Context, toolCalls []model.ToolCall) (*Msg, bool, error) {
	assistantBlocks := []ContentBlock{}

	for _, tc := range toolCalls {
		if err := a.middlewareChain.Execute(ctx, PhaseActing, a, tc); err != nil {
			return nil, false, err
		}

		t, _ := a.config.Tools.Get(tc.Name)
		decision, err := a.config.Permission.Check(ctx, t, tc.Params, nil)
		if err != nil {
			resultBlock := NewToolResultTextBlock(tc.ID, "Permission check error: "+err.Error())
			resultBlock.ToolResultState = ToolResultStateError
			assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, fmt.Sprintf("%v", tc.Params)))
			assistantBlocks = append(assistantBlocks, resultBlock)
			continue
		}

		switch decision.Action {
		case tool.PermissionAllow:
			result, err := a.executeTool(ctx, tc)
			if a.config.OnToolCall != nil {
				a.config.OnToolCall(tc.Name, tc.Params, result)
			}

			var resultBlock ContentBlock
			if err != nil {
				resultBlock = NewToolResultTextBlock(tc.ID, "Tool execution error: "+err.Error())
				resultBlock.ToolResultState = ToolResultStateError
			} else {
				resultBlock = NewToolResultTextBlock(tc.ID, result)
				keepResult, _, truncated := a.contextMgr.TruncateToolResult(ctx, a.session.GetID(), tc.Name, resultBlock,
					a.config.ContextConfig.ToolResultExemptTools, a.config.ContextConfig.ToolResultExemptExts)
				if truncated {
					resultBlock = keepResult
				}
			}
			assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, fmt.Sprintf("%v", tc.Params)))
			assistantBlocks = append(assistantBlocks, resultBlock)

		case tool.PermissionAsk:
			resultBlock := NewToolResultTextBlock(tc.ID, "Tool call requires user confirmation. Use ReplyStream for interactive confirmation.")
			resultBlock.ToolResultState = ToolResultStateDenied
			assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, fmt.Sprintf("%v", tc.Params)))
			assistantBlocks = append(assistantBlocks, resultBlock)

		case tool.PermissionDeny:
			resultBlock := NewToolResultTextBlock(tc.ID, "Permission denied: "+decision.Reason)
			resultBlock.ToolResultState = ToolResultStateDenied
			assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, fmt.Sprintf("%v", tc.Params)))
			assistantBlocks = append(assistantBlocks, resultBlock)

		case tool.PermissionExternal:
			resultBlock := NewToolResultTextBlock(tc.ID, "Tool requires external execution. Use ReplyStream for interactive mode.")
			resultBlock.ToolResultState = ToolResultStateDenied
			assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, fmt.Sprintf("%v", tc.Params)))
			assistantBlocks = append(assistantBlocks, resultBlock)
		}
	}

	assistantMsg := Msg{
		ID:        generateID("msg"),
		Name:      a.config.Name,
		Role:      RoleAssistant,
		Content:   assistantBlocks,
		CreatedAt: nowISO(),
	}
	a.session.AddMessage(assistantMsg)

	return nil, false, nil
}

// =============================================
// 消息构建（三层结构）
// =============================================

// injectToolsToModel 将工具定义注入到模型。
//
// 如果模型实现了 ToolSetter 接口，则将 Agent 的工具注册表
// 转换为模型所需的 ToolDefinition 格式并注入。
func (a *Agent) injectToolsToModel() {
	setter, ok := a.config.Model.(model.ToolSetter)
	if !ok || a.config.Tools == nil {
		return
	}

	names := a.config.Tools.Names()
	tools := make([]model.ToolDefinition, 0, len(names))
	for _, name := range names {
		t, exists := a.config.Tools.Get(name)
		if !exists {
			continue
		}
		tools = append(tools, model.ToolDefinition{
			Type:        "function",
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}

	if len(tools) > 0 {
		setter.SetTools(tools)
	}
}

// buildModelMessages 构建模型输入消息列表。
//
// 三层结构：
//  1. System Prompt — 基础 system_prompt + skill 指令 + on_system_prompt middleware 转换
//  2. Summary — 已压缩历史的摘要（如有）
//  3. Context — 最近的未压缩消息
//
// 返回：
//   - []model.Msg: 完整的三层消息列表，可直接传给 ChatModel.Call()
func (a *Agent) buildModelMessages() []model.Msg {
	messages := make([]model.Msg, 0)

	systemPrompt := a.config.SystemPrompt

	if a.config.Skills != nil {
		skillPrompt := a.config.Skills.GetPrompt()
		if skillPrompt != "" {
			systemPrompt += "\n\n" + skillPrompt
		}
	}

	if err := a.middlewareChain.Execute(context.Background(), PhaseSystemPrompt, a, &systemPrompt); err != nil {
		fmt.Printf("System prompt middleware error: %v\n", err)
	}

	if systemPrompt != "" {
		messages = append(messages, model.Msg{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	summaryText := a.contextMgr.GetSummaryText()
	if summaryText != "" {
		messages = append(messages, model.Msg{
			Role:    "system",
			Content: summaryText,
		})
	}

	//  get_memory + filter by _MemoryMark.COMPRESSED：
	// 如果存在摘要，则排除已压缩的消息，只保留摘要 + 未压缩消息
	var history []Msg
	if summaryText != "" {
		history = a.session.GetHistoryExcludeMarks(MsgMarkCompressed)
	} else {
		history = a.session.GetHistory()
	}
	for _, msg := range history {
		m := msgToModelMsg(msg)
		// 如果消息同时有 tool_calls 和 tool_result，拆分为 assistant + tool 消息
		if m.ToolCalls != nil && msg.GetToolResultContent() != "" {
			// 1. Assistant 消息：只保留 tool_calls，不放 tool result
			assistantMsg := m
			assistantMsg.Content = msg.GetTextContent() // 仅文本，不含 tool result
			messages = append(messages, assistantMsg)
			// 2. 每个 tool call 对应一个 tool 消息（含结果）
			for _, tcBlock := range msg.Content {
				if tcBlock.Type == BlockTypeToolResult {
					resultText := ""
					for _, o := range tcBlock.ToolResultOutput {
						if o.Type == BlockTypeText {
							resultText += o.Text
						}
					}
					messages = append(messages, model.Msg{
						Role:       "tool",
						Name:       tcBlock.ToolCallID,
						ToolCallID: tcBlock.ToolCallID,
						Content:    resultText,
					})
				}
			}
		} else {
			messages = append(messages, m)
		}
	}

	// 深拷贝消息列表，避免修改存储历史（ normalize_messages）
	return cloneModelMessages(messages)
}

// cloneModelMessages 批量深拷贝模型消息列表。
//
//	normalize_messages_for_model_request：
//
// 每次 LLM 调用前深拷贝消息，确保 session 原始历史不被意外修改。
func cloneModelMessages(messages []model.Msg) []model.Msg {
	result := make([]model.Msg, len(messages))
	for i, m := range messages {
		result[i] = m
		if m.ToolCalls != nil {
			tcs := make([]model.ToolCall, len(m.ToolCalls))
			copy(tcs, m.ToolCalls)
			for j, tc := range tcs {
				if tc.Params != nil {
					tcs[j].Params = cloneInterfaceMap(tc.Params)
				}
			}
			result[i].ToolCalls = tcs
		}
		if m.Extra != nil {
			result[i].Extra = cloneInterfaceMap(m.Extra)
		}
	}
	return result
}

func cloneInterfaceMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// =============================================
// 内部辅助函数
// =============================================

// msgToModelMsg 将 agent.Msg 转换为 model.Msg。
//
// 处理以下转换细节：
//   - 提取所有 TextBlock 文本作为 Content
//   - 提取 ToolCallBlock 转换为 model.ToolCall（含 JSON 参数解析）
//   - role=tool 的消息设置 ToolCallID
//
// 参数：
//   - msg: agent 层消息
//
// 返回：
//   - model.Msg: model 层消息（可直接序列化为 OpenAI API 格式）
func msgToModelMsg(msg Msg) model.Msg {
	m := model.Msg{
		Role:    string(msg.Role),
		Content: msg.GetTextContent(),
		Name:    msg.Name,
	}

	if msg.Role == "tool" {
		m.ToolCallID = msg.Name
	}

	if msg.HasToolCalls() {
		toolCalls := make([]model.ToolCall, 0)
		for _, block := range msg.GetToolCallBlocks() {
			params := make(map[string]interface{})
			if block.ToolCallInput != "" {
				if err := json.Unmarshal([]byte(block.ToolCallInput), &params); err != nil {
					params = map[string]interface{}{"input": block.ToolCallInput}
				}
			}
			toolCalls = append(toolCalls, model.ToolCall{
				ID:     block.ToolCallID,
				Name:   block.ToolCallName,
				Params: params,
			})
		}
		m.ToolCalls = toolCalls
	}

	return m
}

// executeTool 执行单个工具调用。
//
// 从 ToolRegistry 中查找工具，传入参数并执行。
//
// 参数：
//   - ctx: 上下文
//   - tc: 工具调用（含名称、参数）
//
// 返回：
//   - string: 工具执行结果文本
//   - error: 工具未找到或执行错误
func (a *Agent) executeTool(ctx context.Context, tc model.ToolCall) (string, error) {
	t, ok := a.config.Tools.Get(tc.Name)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", tc.Name)
	}
	return t.Execute(ctx, tc.Params)
}

// storeToMemory 将交互存储到长期记忆。
//
// 将用户消息和 assistant 回复的文本内容存入 Memory，
// 类型为 "short_term"，后续可通过 Consolidate() 转为 "long_term"。
//
// 参数：
//   - ctx: 上下文
//   - userMsg: 用户消息
//   - replyMsg: assistant 回复消息
func (a *Agent) storeToMemory(ctx context.Context, userMsg, replyMsg Msg) {
	if a.config.Memory == nil {
		return
	}
	userText := userMsg.GetTextContent()
	if userText != "" {
		a.config.Memory.Store(ctx, userText, userText, "short_term")
	}
	replyText := replyMsg.GetTextContent()
	if replyText != "" {
		a.config.Memory.Store(ctx, replyText, replyText, "short_term")
	}
}

// joinMemoryItems 将记忆项列表拼接为单个字符串（用换行分隔）。
//
// 用于在 buildModelMessages 中将检索到的相关记忆注入系统提示词。
//
// 参数：
//   - items: 记忆项列表
//
// 返回：
//   - string: 拼接后的文本
func joinMemoryItems(items []memory.MemoryItem) string {
	var result string
	for _, item := range items {
		result += item.Content + "\n"
	}
	return result
}

// autoCompressContext 自动检查并压缩上下文。
//
// 在每次 Reply/ReplyStream 开始时调用。
// 当 token 数量超过阈值时自动触发压缩。
//
// 参数：
//   - ctx: 上下文
//
// 返回：
//   - error: 压缩错误，nil 表示成功或无需压缩
func (a *Agent) autoCompressContext(ctx context.Context) error {
	history := a.session.GetHistory()
	reserveMsgs, compressed, err := a.contextMgr.CheckAndCompress(
		ctx, a.session.GetID(), history, a.config.SystemPrompt)
	if err != nil || !compressed {
		return err
	}

	// 计算被压缩的消息 ID
	reserveIDs := make(map[string]bool, len(reserveMsgs))
	for _, msg := range reserveMsgs {
		reserveIDs[msg.ID] = true
	}

	// 标记被压缩的消息（ mark_messages_compressed）
	compressedIDs := make([]string, 0, len(history)-len(reserveMsgs))
	for _, msg := range history {
		if !reserveIDs[msg.ID] {
			compressedIDs = append(compressedIDs, msg.ID)
		}
	}

	if len(compressedIDs) > 0 {
		a.session.MarkMessagesCompressed(compressedIDs)
	}

	summary := &Summary{
		TaskOverview:         "Previous conversation compressed",
		CurrentState:         "Continuing from compressed context",
		ImportantDiscoveries: []string{"Context was auto-compressed"},
		NextSteps:            []string{"Continue conversation"},
		ContextToPreserve:    "See offloaded context for details",
		CreatedAt:            nowISO(),
	}
	a.contextMgr.SetSummary(summary)

	return nil
}
