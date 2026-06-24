package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/nllihui6390/go-agent/model"
	"github.com/nllihui6390/go-agent/tool"
)

// =============================================
// ReAct 循环核心步骤（ _reasoning / _acting 分离）
// =============================================

// reasoningStep 推理步骤。
//
// 流程：
//  1. 构建模型输入（含深拷贝）
//  2. 注入工具定义
//  3. 执行 PhasePreReasoning 钩子
//  4. 调用模型（带重试）
//  5. 将助手消息存入 session（立即存储， finally 块）
//  6. 清理 HINT 标记消息
//  7. 返回助手消息
//
// 参数：
//   - ctx: 上下文
//   - iterCount: 当前迭代计数
//
// 返回：
//   - *Msg: 助手消息（含文本/思考/工具调用）
//   - error: 错误信息
func (a *Agent) reasoningStep(ctx context.Context, iterCount int) (*Msg, error) {
	modelMessages := a.buildModelMessages()
	a.injectToolsToModel()

	if err := a.middlewareChain.Execute(ctx, PhasePreReasoning, a, modelMessages); err != nil {
		return nil, err
	}
	if err := a.middlewareChain.Execute(ctx, PhaseReasoning, a, modelMessages); err != nil {
		return nil, err
	}

	response, err := a.callModelWithRetry(ctx, modelMessages)
	if err != nil {
		return nil, err
	}

	// 构建助手消息
	assistantMsg := &Msg{
		ID:        generateID("msg"),
		Name:      a.config.Name,
		Role:      RoleAssistant,
		Content:   buildAssistantContent(response),
		CreatedAt: nowISO(),
		Usage: &Usage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			TotalTokens:  response.Usage.TotalTokens,
		},
	}

	// 立即存入 session（ finally 块：memory.add(msg)）
	a.session.AddMessage(*assistantMsg)

	// 清理临时 HINT 消息（ delete_by_mark(HINT)）
	a.session.ClearHintMessages()

	return assistantMsg, nil
}

// actingStep 行动步骤。
//
// 流程：
//  1. 执行 PhaseActing 钩子
//  2. 权限检查（PermissionChecker.Check）
//  3. 执行工具
//  4. 将工具结果存入 session（立即存储， finally 块）
//  5. 执行 PhasePostActing 钩子
//  6. 返回结果块
//
// 参数：
//   - ctx: 上下文
//   - tc: 工具调用
//
// 返回：
//   - ContentBlock: 工具结果块
//   - error: 错误信息
func (a *Agent) actingStep(ctx context.Context, tc model.ToolCall) (ContentBlock, error) {
	if err := a.middlewareChain.Execute(ctx, PhaseActing, a, tc); err != nil {
		return ContentBlock{}, err
	}

	t, _ := a.config.Tools.Get(tc.Name)
	decision, err := a.config.Permission.Check(ctx, t, tc.Params, nil)
	if err != nil {
		resultBlock := NewToolResultTextBlock(tc.ID, "Permission check error: "+err.Error())
		resultBlock.ToolResultState = ToolResultStateError
		return resultBlock, nil
	}

	var resultBlock ContentBlock

	switch decision.Action {
	case tool.PermissionAllow:
		result, err := a.executeTool(ctx, tc)
		if a.config.OnToolCall != nil {
			a.config.OnToolCall(tc.Name, tc.Params, result)
		}
		if err != nil {
			resultBlock = NewToolResultTextBlock(tc.ID, "Error: "+err.Error())
			resultBlock.ToolResultState = ToolResultStateError
		} else {
			resultBlock = NewToolResultTextBlock(tc.ID, result)
			resultBlock.ToolResultState = ToolResultStateSuccess
			keepResult, _, truncated := a.contextMgr.TruncateToolResult(ctx, a.session.GetID(), tc.Name, resultBlock,
			a.config.ContextConfig.ToolResultExemptTools, a.config.ContextConfig.ToolResultExemptExts)
			if truncated {
				resultBlock = keepResult
			}
		}
	case tool.PermissionAsk:
		resultBlock = NewToolResultTextBlock(tc.ID, "Tool call requires user confirmation.")
		resultBlock.ToolResultState = ToolResultStateDenied
	case tool.PermissionDeny:
		resultBlock = NewToolResultTextBlock(tc.ID, "Permission denied: "+decision.Reason)
		resultBlock.ToolResultState = ToolResultStateDenied
	case tool.PermissionExternal:
		resultBlock = NewToolResultTextBlock(tc.ID, "External execution required.")
		resultBlock.ToolResultState = ToolResultStateDenied
	}

	// 将工具调用+结果存入 session（ finally 块：memory.add(tool_res_msg)）
	tcBlock := NewToolCallBlock(tc.ID, tc.Name, ParamsToJSON(tc.Params))
	assistantMsg := &Msg{
		ID:        generateID("msg"),
		Name:      a.config.Name,
		Role:      RoleAssistant,
		Content:   []ContentBlock{tcBlock, resultBlock},
		CreatedAt: nowISO(),
	}
	a.session.AddMessage(*assistantMsg)

	// PhasePostActing: 工具执行后（ post_acting 钩子，截断工具结果）
	a.middlewareChain.Execute(ctx, PhasePostActing, a, resultBlock)

	return resultBlock, nil
}

// compressMemoryIfNeeded 循环内压缩检测（ _compress_memory_if_needed）。
//
// 在每次推理前调用，检查上下文 token 数是否超过阈值。
// 如果超过，触发压缩，标记旧消息为 MsgMarkCompressed。
func (a *Agent) compressMemoryIfNeeded(ctx context.Context) {
	history := a.session.GetHistoryExcludeMarks(MsgMarkCompressed)
	systemPrompt := a.config.SystemPrompt

	totalTokens := a.contextMgr.counter.CountTokens(systemPrompt)
	if a.contextMgr.summary != nil {
		totalTokens += a.contextMgr.summary.TokenCount
	}
	totalTokens += a.contextMgr.counter.CountMessagesTokens(history)

	triggerThreshold := int(float64(a.contextMgr.maxTokens) * a.contextMgr.config.TriggerRatio)
	if totalTokens < triggerThreshold {
		return
	}

	// 触发压缩
	reserveTokens := int(float64(a.contextMgr.maxTokens) * a.contextMgr.config.ReserveRatio)
	compressMsgs, _ := a.contextMgr.splitMessages(history, reserveTokens)
	if len(compressMsgs) == 0 {
		return
	}

	// Offload 待压缩消息
	if a.contextMgr.offloader != nil {
		_, err := a.contextMgr.offloader.OffloadContext(ctx, a.session.GetID(), compressMsgs)
		if err != nil {
			fmt.Printf("Offload context failed: %v\n", err)
		}
	}

	// 标记已压缩消息（ mark_messages_compressed）
	compressedIDs := make([]string, len(compressMsgs))
	for i, m := range compressMsgs {
		compressedIDs[i] = m.ID
	}
	a.session.MarkMessagesCompressed(compressedIDs)

	// 生成摘要
	summary := &Summary{
		TaskOverview:         "Previous conversation compressed during reasoning loop",
		CurrentState:         "Continuing from compressed context",
		ImportantDiscoveries: []string{"Context was auto-compressed in loop"},
		NextSteps:            []string{"Continue conversation"},
		ContextToPreserve:    "See offloaded context for details",
		CreatedAt:            nowISO(),
	}
	a.contextMgr.SetSummary(summary)
}

// autoContinueIfTextOnly 文本响应自动续接（ _auto_continue_if_text_only）。
//
// 当模型返回纯文本且本轮已执行过工具时调用。
// 注入语言匹配的提示，让模型自我审查：
// "还需工具吗？→ 调用工具 / 已完成？→ 简短收尾"
//
// 返回：
//   - *Msg: 续接后的助手消息（可能包含工具调用）
//   - bool: true=产生了新工具调用需继续循环, false=保留原消息结束
func (a *Agent) autoContinueIfTextOnly(ctx context.Context, currentMsg *Msg) (*Msg, bool) {
	// 检查配置
	if !a.config.ReActConfig.AutoContinueEnabled {
		return currentMsg, false
	}

	// 构造函数提示（含上一轮助手文本尾部上下文）
	tailText := currentMsg.GetTextContent()
	if len(tailText) > 600 {
		tailText = tailText[len(tailText)-600:]
	}
	tailText = strings.TrimSpace(tailText)

	hintBody := "<system-hint>上轮助手仅文字、未调工具。请结合上下文与下方 <previous-assistant-tail> 在本轮推理中判断：仍需执行则立刻调用工具；已完结则简短收尾回复。需要操作时勿只输出计划或代码块。</system-hint>"
	if tailText != "" {
		hintBody += "\n\n<previous-assistant-tail>\n" + tailText + "\n</previous-assistant-tail>"
	}

	a.session.AddHintMessage(hintBody)

	// 额外一次推理（最多额外 _AUTO_CONTINUE_MAX_EXTRA 次推理，
	// 但如果一次提示后模型仍然纯文本，同一条提示再试也没有意义，直接保留原消息）
	nextMsg, err := a.reasoningStep(ctx, 0)
	if err == nil && nextMsg.HasToolCalls() {
		return nextMsg, true
	}

	return currentMsg, false
}

// summarizingStep 汇总步骤（ _summarizing）。
//
// 当达到最大迭代次数且没有最终回复时调用。
// 尝试生成结构化摘要而非简单错误文本。
//
// 参数：
//   - ctx: 上下文
//   - userMsg: 原始用户消息
//
// 返回：
//   - *Msg: 汇总回复
//   - error: 错误信息
func (a *Agent) summarizingStep(ctx context.Context, userMsg Msg) (*Msg, error) {
	// 注入汇总提示（ _ROUND_END_NOTICE）
	summarizePrompt := "Please summarize what has been accomplished so far and what remains to be done, based on the conversation above. Be concise but complete."
	a.session.AddHintMessage(summarizePrompt)

	// 尝试最后一次模型调用
	modelMessages := a.buildModelMessages()
	a.injectToolsToModel()
	response, err := a.callModelWithRetry(ctx, modelMessages)
	a.session.ClearHintMessages()

	if err != nil {
		// 模型调用失败，返回简单文本
		reply := &Msg{
			ID:        generateID("msg"),
			Name:      a.config.Name,
			Role:      RoleAssistant,
			Content:   []ContentBlock{NewTextBlock("已达到最大迭代次数。当前任务可能需要更多步骤来完成。")},
			CreatedAt: nowISO(),
		}
		reply.SetFinished()
		a.session.AddMessage(*reply)
		return reply, nil
	}

	reply := &Msg{
		ID:        generateID("msg"),
		Name:      a.config.Name,
		Role:      RoleAssistant,
		Content:   []ContentBlock{NewTextBlock(response.Content)},
		CreatedAt: nowISO(),
		Usage: &Usage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			TotalTokens:  response.Usage.TotalTokens,
		},
	}
	reply.SetFinished()
	a.storeToMemory(ctx, userMsg, *reply)
	a.session.AddMessage(*reply)

	a.middlewareChain.Execute(ctx, PhasePostReply, a, reply)
	return reply, nil
}

// buildAssistantContent 从模型响应构建助手消息内容块列表。
func buildAssistantContent(response *model.Response) []ContentBlock {
	blocks := make([]ContentBlock, 0)

	// 文本内容
	if response.Content != "" {
		blocks = append(blocks, NewTextBlock(response.Content))
	}

	// 工具调用
	for _, tc := range response.ToolCalls {
		paramsJSON, _ := model.ParamsToJSON(tc.Params)
		blocks = append(blocks, NewToolCallBlock(tc.ID, tc.Name, paramsJSON))
	}

	return blocks
}
