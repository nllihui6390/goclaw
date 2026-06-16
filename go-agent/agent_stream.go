package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/nllihui6390/go-agent/model"
	"github.com/nllihui6390/go-agent/tool"
)

// =============================================
// ReplyStream — 流式推理
// =============================================

// ReplyStream 流式处理输入，返回事件 channel。
// 返回的事件流遵循 start → delta → end 模式：
//   - ReplyStartEvent：回复开始
//   - ModelCallStartEvent：模型调用开始
//   - TextBlockStartEvent → TextBlockDeltaEvent × N → TextBlockEndEvent：文本流式输出
//   - ToolCallStartEvent → ToolCallDeltaEvent → ToolCallEndEvent：工具调用
//   - ToolResultStartEvent → ToolResultTextDeltaEvent → ToolResultEndEvent：工具结果
//   - ReplyEndEvent：回复完成
//
// 参数：
//   - ctx: 上下文（取消 ctx 会停止事件流）
//   - inputs: Msg 或 []Msg（不支持事件恢复，使用 Reply 进行恢复）
//
// 返回：
//   - <-chan interface{}: 事件流 channel（只读），channel 在处理完成后自动关闭
//   - error: 启动错误（非处理错误）
//
// 示例：
//
//	stream, err := ag.ReplyStream(ctx, agent.UserMsg("user", "Hello"))
//	for event := range stream {
//	    if delta, ok := agent.GetDeltaText(event); ok {
//	        fmt.Print(delta)
//	    }
//	}
func (a *Agent) ReplyStream(ctx context.Context, inputs interface{}) (<-chan interface{}, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch input := inputs.(type) {
	case Msg:
		a.session.AddMessage(input)
	case []Msg:
		for _, msg := range input {
			a.session.AddMessage(msg)
		}
	default:
		return nil, fmt.Errorf("stream does not support input type: %T, use Reply for confirm/external events", inputs)
	}

	output := make(chan interface{}, 200)
	go a.processReplyStream(ctx, output)

	return output, nil
}

// processReplyStream 在后台 goroutine 中处理流式回复。
//
// 产生完整的事件序列：
//  1. ReplyStartEvent
//  2. 上下文压缩检查
//  3. 推理-执行循环：
//     a. ModelCallStartEvent → 流式文本/工具调用事件 → ModelCallEndEvent
//     b. 工具结果事件（含权限检查）
//  4. ReplyEndEvent 或 ExceedMaxItersEvent
//
// 参数：
//   - ctx: 上下文（取消会停止处理）
//   - output: 事件输出 channel
func (a *Agent) processReplyStream(ctx context.Context, output chan<- interface{}) {
	defer close(output)

	replyID := generateID("reply")
	sessionID := a.session.GetID()

	output <- NewReplyStartEvent(replyID, sessionID, a.config.Name)

	if err := a.autoCompressContext(ctx); err != nil {
		fmt.Printf("Context compression failed: %v\n", err)
	}

	iterCount := 0
	totalReplyInputTokens := 0
	totalReplyOutputTokens := 0
	for {
		if iterCount >= a.config.ReActConfig.MaxIters {
			output <- NewExceedMaxItersEvent(replyID, a.config.Name)
			output <- NewReplyEndEventWithTokens(replyID, sessionID, totalReplyInputTokens, totalReplyOutputTokens)
			return
		}
		iterCount++

		modelMessages := a.buildModelMessages()

		// 注入工具定义到模型
		a.injectToolsToModel()
		// 模型调用开始事件
		output <- NewModelCallStartEvent(replyID, a.config.Model.GetName())
		// 流式调用模型
		stream, err := a.config.Model.Stream(ctx, modelMessages)
		if err != nil {
			// 处理重试逻辑-默认重试3次
			for retry := 0; retry < a.config.ModelConfig.MaxRetries; retry++ {
				time.Sleep(time.Duration(a.config.ModelConfig.RetryDelay) * time.Millisecond)
				stream, err = a.config.Model.Stream(ctx, modelMessages)
				if err == nil {
					break
				}
			}
			if err != nil {
				// 创建回复结束事件
				output <- NewReplyEndEventWithTokens(replyID, sessionID, totalReplyInputTokens, totalReplyOutputTokens)
				return
			}
		}

		var fullContent string
		var fullThinking string
		var savedThinking string // thinking 可能被后续 content chunk 清空，提前保存
		var toolCalls []model.ToolCall
		var totalInputTokens, totalOutputTokens int // 累积本轮 token 使用量
		blockID := generateID("blk")
		thinkingBlockID := generateID("think")
		// 创建文本块开始事件
		output <- NewTextBlockStartEvent(replyID, blockID)

		for chunk := range stream {
			if chunk.Error != nil {
				break
			}

			// 累积 token 使用量
			if chunk.InputTokens > 0 || chunk.OutputTokens > 0 {
				totalInputTokens += chunk.InputTokens
				totalOutputTokens += chunk.OutputTokens
			}

			if chunk.ToolCall != nil {
				tcID := chunk.ToolCall.ID
				// 创建工具调用开始事件
				output <- NewToolCallStartEvent(replyID, tcID, chunk.ToolCall.Name)
				if fullContent != "" {
					// 创建文本块结束事件
					output <- NewTextBlockEndEvent(replyID, blockID)
					fullContent = ""
				}
				savedThinking = fullThinking
				if fullThinking != "" {
					// 思考快速结束事件
					output <- NewThinkingBlockEndEvent(replyID, thinkingBlockID)
					fullThinking = ""
				}
				// 解析工具调用参数为 JSON 字符串，并发送增量事件
				if paramsJSON, err := model.ParamsToJSON(chunk.ToolCall.Params); err == nil {
					// 创建工具调用参数增量事件
					output <- NewToolCallDeltaEvent(replyID, tcID, paramsJSON)
				}
				// 创建工具调用结束事件
				output <- NewToolCallEndEvent(replyID, tcID)
				toolCalls = append(toolCalls, *chunk.ToolCall)
			} else if chunk.Thinking != "" {
				if fullThinking == "" {
					output <- NewThinkingBlockStartEvent(replyID, thinkingBlockID)
				}
				fullThinking += chunk.Thinking
				output <- NewThinkingBlockDeltaEvent(replyID, thinkingBlockID, chunk.Thinking)
			} else if chunk.Content != "" {
				if fullThinking != "" {
					savedThinking = fullThinking
					output <- NewThinkingBlockEndEvent(replyID, thinkingBlockID)
					fullThinking = ""
				}
				fullContent += chunk.Content
				output <- NewTextBlockDeltaEvent(replyID, blockID, chunk.Content)
			}
			
		}

		if fullThinking != "" {
			output <- NewThinkingBlockEndEvent(replyID, thinkingBlockID)
		}
		if fullContent != "" || iterCount == 1 {
			output <- NewTextBlockEndEvent(replyID, blockID)
		}
		// 创建模型调用结束事件
		output <- NewModelCallEndEvent(replyID, totalInputTokens, totalOutputTokens)
		totalReplyInputTokens += totalInputTokens
		totalReplyOutputTokens += totalOutputTokens

		if len(toolCalls) > 0 {
			// 构建助手消息中的内容块（工具调用 + 工具结果）
			assistantBlocks := []ContentBlock{}
			if fullContent != "" {
				assistantBlocks = append(assistantBlocks, NewTextBlock(fullContent))
			}
			if st := coalesce(savedThinking, fullThinking); st != "" {
				assistantBlocks = append(assistantBlocks, NewThinkingBlock(st))
			}

			for _, tc := range toolCalls {
				t, _ := a.config.Tools.Get(tc.Name)
				decision, err := a.config.Permission.Check(ctx, t, tc.Params, nil)
				if err != nil {
					output <- NewToolResultStartEvent(replyID, tc.ID, tc.Name)
					output <- NewToolResultTextDeltaEvent(replyID, tc.ID, "Permission check error: "+err.Error())
					output <- NewToolResultEndEvent(replyID, tc.ID, ToolResultStateError)
					resultBlock := NewToolResultTextBlock(tc.ID, "Permission check error: "+err.Error())
					resultBlock.ToolResultState = ToolResultStateError
					assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, ParamsToJSON(tc.Params)))
					assistantBlocks = append(assistantBlocks, resultBlock)
					continue
				}

				switch decision.Action {
				case tool.PermissionAllow:
					output <- NewToolResultStartEvent(replyID, tc.ID, tc.Name)
					result, err := a.executeTool(ctx, tc)
					if a.config.OnToolCall != nil {
						a.config.OnToolCall(tc.Name, tc.Params, result)
					}
					if err != nil {
						output <- NewToolResultTextDeltaEvent(replyID, tc.ID, "Error: "+err.Error())
						output <- NewToolResultEndEvent(replyID, tc.ID, ToolResultStateError)
						resultBlock := NewToolResultTextBlock(tc.ID, "Error: "+err.Error())
						resultBlock.ToolResultState = ToolResultStateError
						assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, ParamsToJSON(tc.Params)))
						assistantBlocks = append(assistantBlocks, resultBlock)
					} else {
						output <- NewToolResultTextDeltaEvent(replyID, tc.ID, result)
						output <- NewToolResultEndEvent(replyID, tc.ID, ToolResultStateSuccess)
						resultBlock := NewToolResultTextBlock(tc.ID, result)
						resultBlock.ToolResultState = ToolResultStateSuccess
						assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, ParamsToJSON(tc.Params)))
						assistantBlocks = append(assistantBlocks, resultBlock)
					}

				case tool.PermissionAsk:
					output <- NewRequireUserConfirmEvent(replyID, []ContentBlock{
						NewToolCallBlock(tc.ID, tc.Name, ParamsToJSON(tc.Params)),
					})
					resultBlock := NewToolResultTextBlock(tc.ID, "Permission denied")
					resultBlock.ToolResultState = ToolResultStateDenied
					assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, ParamsToJSON(tc.Params)))
					assistantBlocks = append(assistantBlocks, resultBlock)

				case tool.PermissionDeny:
					output <- NewToolResultStartEvent(replyID, tc.ID, tc.Name)
					output <- NewToolResultTextDeltaEvent(replyID, tc.ID, "Permission denied")
					output <- NewToolResultEndEvent(replyID, tc.ID, ToolResultStateDenied)
					resultBlock := NewToolResultTextBlock(tc.ID, "Permission denied")
					resultBlock.ToolResultState = ToolResultStateDenied
					assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, ParamsToJSON(tc.Params)))
					assistantBlocks = append(assistantBlocks, resultBlock)

				case tool.PermissionExternal:
					output <- NewRequireExternalExecutionEvent(replyID, []ContentBlock{
						NewToolCallBlock(tc.ID, tc.Name, ParamsToJSON(tc.Params)),
					})
					resultBlock := NewToolResultTextBlock(tc.ID, "External execution required")
					resultBlock.ToolResultState = ToolResultStateDenied
					assistantBlocks = append(assistantBlocks, NewToolCallBlock(tc.ID, tc.Name, ParamsToJSON(tc.Params)))
					assistantBlocks = append(assistantBlocks, resultBlock)
				}
			}

			// 将助手消息（含工具调用和结果）存入 session
			assistantMsg := Msg{
				ID:        generateID("msg"),
				Name:      a.config.Name,
				Role:      RoleAssistant,
				Content:   assistantBlocks,
				CreatedAt: nowISO(),
					Usage: &Usage{
						InputTokens:  totalInputTokens,
						OutputTokens: totalOutputTokens,
						TotalTokens:  totalInputTokens + totalOutputTokens,
					},
			}
			a.session.AddMessage(assistantMsg)

			continue
		}
		blocks := make([]ContentBlock, 0, 2)
		if st := coalesce(savedThinking, fullThinking); st != "" {
			blocks = append(blocks, NewThinkingBlock(st))
		}
		if fullContent != "" {
			blocks = append(blocks, NewTextBlock(fullContent))
		}
		finalMsg := &Msg{
			ID:        replyID,
			Name:      a.config.Name,
			Role:      RoleAssistant,
			Content:   blocks,
			CreatedAt: nowISO(),
				Usage: &Usage{
					InputTokens:  totalInputTokens,
					OutputTokens: totalOutputTokens,
					TotalTokens:  totalInputTokens + totalOutputTokens,
				},
		}
		finalMsg.SetFinished()
		a.session.AddMessage(*finalMsg)
		// 创建回复结束事件
		output <- NewReplyEndEventWithTokens(replyID, sessionID, totalReplyInputTokens, totalReplyOutputTokens)
		return
	}
}

// coalesce 返回第一个非空字符串
func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
