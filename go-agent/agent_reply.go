package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nllihui6390/go-agent/model"
)

func (a *Agent) replyFromMessage(ctx context.Context, msg Msg) (*Msg, error) {
	a.session.AddMessage(msg)

	// PhasePreReply: 最终回复前（ pre_reply 钩子）
	if err := a.middlewareChain.Execute(ctx, PhasePreReply, a, msg); err != nil {
		return nil, err
	}

	// 初始压缩检查
	if err := a.autoCompressContext(ctx); err != nil {
		fmt.Printf("Context compression failed: %v\n", err)
	}

	hasToolHistory := false // 本轮是否已执行过工具

	for iterCount := 0; iterCount < a.config.ReActConfig.MaxIters; iterCount++ {
		// 循环内压缩检测（ _compress_memory_if_needed）
		a.compressMemoryIfNeeded(ctx)

		// 推理步骤
		assistantMsg, err := a.reasoningStep(ctx, iterCount)
		if err != nil {
			return nil, err
		}

		// === 退出判断（参考 AgentScope ReActAgent.reply 的 exit conditions）===

		if !assistantMsg.HasToolCalls() {
			// 模型返回纯文本，没有工具调用

			if !hasToolHistory {
				// 情况1: 首轮就是纯文本 → 简单对话，直接返回
				// （如用户说"你好"，模型回复"你好！有什么可以帮你的？"）
				assistantMsg.SetFinished()
				a.storeToMemory(ctx, msg, *assistantMsg)
				a.middlewareChain.Execute(ctx, PhasePostReply, a, assistantMsg)
				return assistantMsg, nil
			}

			// 情况2: 已执行过工具，模型现在返回纯文本
			// 需要判断：是最终回复？还是需要继续？
			//  _auto_continue_if_text_only：
			// 注入提示问模型"还需工具吗？"，给一次额外的推理机会

			if a.config.ReActConfig.AutoContinueEnabled {
				continueMsg, shouldContinue := a.autoContinueIfTextOnly(ctx, assistantMsg)
				if shouldContinue {
					// 模型被提示后决定调用工具 → 继续循环
					assistantMsg = continueMsg
					hasToolHistory = true
					// 提取新的工具调用并执行
					toolCalls := assistantMsg.GetToolCallBlocks()
					for _, tcBlock := range toolCalls {
						tc := model.ToolCall{ID: tcBlock.ToolCallID, Name: tcBlock.ToolCallName}
						json.Unmarshal([]byte(tcBlock.ToolCallInput), &tc.Params)
						a.actingStep(ctx, tc)
					}
					continue
				}
				// 模型确认已完成 → 返回文本回复
			}

			assistantMsg.SetFinished()
			a.storeToMemory(ctx, msg, *assistantMsg)
			a.middlewareChain.Execute(ctx, PhasePostReply, a, assistantMsg)
			return assistantMsg, nil
		}

		// === 执行工具调用===
		hasToolHistory = true
		toolCalls := assistantMsg.GetToolCallBlocks()
		for _, tcBlock := range toolCalls {
			tc := model.ToolCall{ID: tcBlock.ToolCallID, Name: tcBlock.ToolCallName}
			json.Unmarshal([]byte(tcBlock.ToolCallInput), &tc.Params)
			a.actingStep(ctx, tc)
		}
		// 工具执行后必定继续循环，让模型看到工具结果
		// （对应 AgentScope：acting 后 always continue loop）
	}

	// 达到最大迭代次数 → 结构化汇总（ _summarizing）
	return a.summarizingStep(ctx, msg)
}

// replyFromConfirm 从用户确认结果恢复 reply。

func (a *Agent) replyFromConfirm(ctx context.Context, event UserConfirmResultEvent) (*Msg, error) {
	for _, result := range event.ConfirmResults {
		if result.Approved {
			// 执行工具
		} else {
			// 拒绝，返回错误结果给 LLM
		}
	}
	modelMessages := a.buildModelMessages()
	return a.continueReply(ctx, modelMessages)
}

// replyFromExternal 从外部执行结果恢复 reply。
//
// 将外部系统返回的 ToolResultBlock 注入会话上下文，
// 然后继续推理循环。
//
// 参数：
//   - ctx: 上下文
//   - event: 外部执行结果事件
//
// 返回：
//   - *Msg: 继续推理后的回复
//   - error: 错误信息
func (a *Agent) replyFromExternal(ctx context.Context, event ExternalExecutionResultEvent) (*Msg, error) {
	for _, result := range event.ExecutionResults {
		msg := Msg{
			ID:        generateID("msg"),
			Role:      RoleAssistant,
			Content:   []ContentBlock{result},
			CreatedAt: nowISO(),
		}
		a.session.AddMessage(msg)
	}

	modelMessages := a.buildModelMessages()
	return a.continueReply(ctx, modelMessages)
}

// continueReply 从当前状态继续推理循环。
//
// 每次迭代：
//  1. 检查迭代次数上限
//  2. 调用模型（带重试）
//  3. 无工具调用 → 返回最终结果
//  4. 有工具调用 → 权限检查 → 执行 → 继续
//
// 参数：
//   - ctx: 上下文
//   - modelMessages: 当前的模型消息列表
//
// 返回：
//   - *Msg: 最终回复
//   - error: 错误信息
func (a *Agent) continueReply(ctx context.Context, modelMessages []model.Msg) (*Msg, error) {
	iterCount := 0
	for {
		if iterCount >= a.config.ReActConfig.MaxIters {
			reply := &Msg{
				ID:        generateID("msg"),
				Name:      a.config.Name,
				Role:      RoleAssistant,
				Content:   []ContentBlock{NewTextBlock("I've reached the maximum number of iterations.")},
				CreatedAt: nowISO(),
			}
			reply.SetFinished()
			a.session.AddMessage(*reply)
			return reply, nil
		}
		iterCount++

		response, err := a.callModelWithRetry(ctx, modelMessages)
		if err != nil {
			return nil, err
		}

		if len(response.ToolCalls) == 0 {
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
			a.session.AddMessage(*reply)
			return reply, nil
		}

		result, paused, err := a.handleToolCallsWithPermission(ctx, response.ToolCalls)
		if err != nil {
			return nil, err
		}
		if paused {
			return result, nil
		}

		modelMessages = a.buildModelMessages()
	}
}
