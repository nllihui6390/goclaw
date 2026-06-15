package agent

// =============================================
// AppendEvent — 从事件流重建消息（ msg.append_event）
//
// 事件与消息并非相互独立，而是同一数据的两种视图。
// reply_stream 产出的每个事件都可以通过 AppendEvent() 应用到 Msg 上，
// 从而增量地重建完整消息。
// 这保证了最终消息状态可以仅凭事件流完整还原。
//
// 典型使用模式（ 的流式界面示例）：
//
//	var msg *agent.Msg
//	for event := range eventStream {
//	    if start, ok := event.(agent.ReplyStartEvent); ok {
//	        msg = &agent.Msg{ID: start.ReplyID, ...}
//	    } else if msg != nil {
//	        msg.AppendEvent(event)
//	    }
//	}
//	// msg 现在包含完整的回复内容
// =============================================

import (
	"encoding/json"
	"fmt"
)

// AppendEvent 将事件应用到消息，逐步还原状态。
//
//	 的 msg.append_event() 方法，处理所有事件类型：
//
//		TextBlockStartEvent    → 追加新的空 TextBlock
//		TextBlockDeltaEvent    → 将 delta 拼接到对应块的文本
//		TextBlockEndEvent      → 无额外操作
//		ToolCallStartEvent     → 追加新的空参数 ToolCallBlock
//		ToolCallDeltaEvent     → 将 delta 拼接到工具调用的参数
//		ToolCallEndEvent       → 更新工具调用状态为 COMPLETE
//		ToolResultStartEvent   → 追加新的空输出 ToolResultBlock
//		ToolResultTextDeltaEvent → 将文本追加到工具结果的输出
//		ToolResultEndEvent     → 设置工具结果的最终状态
//		HintBlockEvent         → 将 HintBlock 追加到 content
//		RequireUserConfirmEvent → 将对应工具调用状态更新为 ASKING
//		ExternalExecutionResultEvent → 将 ToolResultBlock 追加到消息
//
// 参数：
//   - event: 任意事件类型（通过 interface{} 接收）
//
// 返回：
//   - error: 不支持的事件类型时返回错误，nil 表示成功应用
func (m *Msg) AppendEvent(event interface{}) error {
	switch e := event.(type) {
	case ReplyEndEvent:
		m.SetFinished()

	case ExceedMaxItersEvent:
		m.SetFinished()

	case TextBlockStartEvent:
		m.Content = append(m.Content, ContentBlock{
			Type:    BlockTypeText,
			Text:    "",
			BlockID: e.BlockID,
		})

	case TextBlockDeltaEvent:
		idx := m.findBlockByID(e.BlockID)
		if idx >= 0 {
			m.Content[idx].Text += e.Delta
		}

	case TextBlockEndEvent:
		// 文本块完成，无需额外操作

	case ThinkingBlockStartEvent:
		m.Content = append(m.Content, ContentBlock{
			Type:     BlockTypeThinking,
			Thinking: "",
			BlockID:  e.BlockID,
		})

	case ThinkingBlockDeltaEvent:
		idx := m.findBlockByID(e.BlockID)
		if idx >= 0 {
			m.Content[idx].Thinking += e.Delta
		}

	case ThinkingBlockEndEvent:
		// 思考块完成

	case DataBlockStartEvent:
		m.Content = append(m.Content, ContentBlock{
			Type:      BlockTypeData,
			MediaType: e.MediaType,
			BlockID:   e.BlockID,
			Source:    &DataSource{Type: DataSourceBase64, MediaType: e.MediaType},
		})

	case DataBlockDeltaEvent:
		idx := m.findBlockByID(e.BlockID)
		if idx >= 0 && m.Content[idx].Source != nil {
			m.Content[idx].Source.Data += e.Data
		}

	case DataBlockEndEvent:
		// 数据块完成

	case ToolCallStartEvent:
		m.Content = append(m.Content, ContentBlock{
			Type:          BlockTypeToolCall,
			ToolCallID:    e.ToolCallID,
			ToolCallName:  e.ToolCallName,
			ToolCallInput: "",
			ToolCallState: ToolCallStatePending,
		})

	case ToolCallDeltaEvent:
		idx := m.findToolCallByID(e.ToolCallID)
		if idx >= 0 {
			m.Content[idx].ToolCallInput += e.Delta
		}

	case ToolCallEndEvent:
		idx := m.findToolCallByID(e.ToolCallID)
		if idx >= 0 {
			m.Content[idx].ToolCallState = ToolCallStateComplete
		}

	case ToolResultStartEvent:
		m.Content = append(m.Content, ContentBlock{
			Type:             BlockTypeToolResult,
			ToolCallID:       e.ToolCallID,
			ToolResultOutput: []ContentBlock{},
			ToolResultState:  ToolResultStateRunning,
		})

	case ToolResultTextDeltaEvent:
		idx := m.findToolResultByID(e.ToolCallID)
		if idx >= 0 {
			textIdx := m.findTextInToolResult(idx)
			if textIdx >= 0 {
				m.Content[idx].ToolResultOutput[textIdx].Text += e.Delta
			} else {
				m.Content[idx].ToolResultOutput = append(
					m.Content[idx].ToolResultOutput, NewTextBlock(e.Delta))
			}
		}

	case ToolResultDataDeltaEvent:
		idx := m.findToolResultByID(e.ToolCallID)
		if idx >= 0 {
			if e.Data != "" {
				m.Content[idx].ToolResultOutput = append(m.Content[idx].ToolResultOutput,
					ContentBlock{
						Type: BlockTypeData, MediaType: e.MediaType, BlockID: e.BlockID,
						Source: &DataSource{Type: DataSourceBase64, Data: e.Data, MediaType: e.MediaType},
					})
			} else if e.URL != "" {
				m.Content[idx].ToolResultOutput = append(m.Content[idx].ToolResultOutput,
					ContentBlock{
						Type: BlockTypeData, MediaType: e.MediaType, BlockID: e.BlockID,
						Source: &DataSource{Type: DataSourceURL, URL: e.URL, MediaType: e.MediaType},
					})
			}
		}

	case ToolResultEndEvent:
		idx := m.findToolResultByID(e.ToolCallID)
		if idx >= 0 {
			m.Content[idx].ToolResultState = e.State
		}

	case HintBlockEvent:
		m.Content = append(m.Content, ContentBlock{
			Type:       BlockTypeHint,
			BlockID:    e.BlockID,
			Hint:       e.Hint,
			HintSource: e.Source,
		})

	case ModelCallStartEvent:
		if m.Metadata == nil {
			m.Metadata = make(map[string]interface{})
		}
		m.Metadata["model_name"] = e.ModelName

	case ModelCallEndEvent:
		m.Usage = &Usage{
			InputTokens:  e.InputTokens,
			OutputTokens: e.OutputTokens,
			TotalTokens:  e.InputTokens + e.OutputTokens,
		}

	case RequireUserConfirmEvent:
		for _, tc := range e.ToolCalls {
			idx := m.findToolCallByID(tc.ToolCallID)
			if idx >= 0 {
				m.Content[idx].ToolCallState = ToolCallStateAsking
			}
		}

	case ExternalExecutionResultEvent:
		m.Content = append(m.Content, e.ExecutionResults...)

	default:
		return fmt.Errorf("unsupported event type: %T", event)
	}

	return nil
}

// =============================================
// 内部查找方法
// =============================================

// findBlockByID 根据 BlockID 在消息内容中查找块索引。
//
// 参数：
//   - blockID: 块唯一标识符
//
// 返回：
//   - int: 找到的索引，-1 表示未找到
func (m *Msg) findBlockByID(blockID string) int {
	for i, block := range m.Content {
		if block.BlockID == blockID {
			return i
		}
	}
	return -1
}

// findToolCallByID 根据 ToolCallID 查找工具调用块索引。
//
// 参数：
//   - toolCallID: 工具调用 ID
//
// 返回：
//   - int: 索引或 -1
func (m *Msg) findToolCallByID(toolCallID string) int {
	for i, block := range m.Content {
		if block.Type == BlockTypeToolCall && block.ToolCallID == toolCallID {
			return i
		}
	}
	return -1
}

// findToolResultByID 根据 ToolCallID 查找工具结果块索引。
//
// 参数：
//   - toolCallID: 工具调用 ID
//
// 返回：
//   - int: 索引或 -1
func (m *Msg) findToolResultByID(toolCallID string) int {
	for i, block := range m.Content {
		if block.Type == BlockTypeToolResult && block.ToolCallID == toolCallID {
			return i
		}
	}
	return -1
}

// findTextInToolResult 在工具结果块中查找第一个 TextBlock 的索引。
//
// 参数：
//   - resultIdx: 工具结果块在 Content 中的索引
//
// 返回：
//   - int: TextBlock 在 ToolResultOutput 中的索引，-1 表示未找到
func (m *Msg) findTextInToolResult(resultIdx int) int {
	output := m.Content[resultIdx].ToolResultOutput
	for i, block := range output {
		if block.Type == BlockTypeText {
			return i
		}
	}
	return -1
}

// =============================================
// ReplyStream 辅助函数
// =============================================

// ReplyStreamHandler 事件流处理回调函数类型。
type ReplyStreamHandler func(event interface{})

// StreamToMsg 从事件 channel 重建完整消息。
//
// 这是构建流式界面的典型模式：
//
//	msg := agent.StreamToMsg(eventStream)
//	// msg 现在包含完整的回复内容
//
// 参数：
//   - events: 事件流 channel（只读，处理完成后关闭）
//
// 返回：
//   - *Msg: 从事件流重建的完整消息，events 为空时返回 nil
func StreamToMsg(events <-chan interface{}) *Msg {
	var msg *Msg

	for event := range events {
		switch e := event.(type) {
		case ReplyStartEvent:
			msg = &Msg{
				ID:        e.ReplyID,
				Name:      e.Name,
				Role:      Role(e.Role),
				Content:   []ContentBlock{},
				CreatedAt: e.CreatedAt,
			}
		default:
			if msg != nil {
				msg.AppendEvent(event)
			}
		}
	}

	return msg
}

// =============================================
// 事件增量数据提取（前端实时显示）
// =============================================

// GetDeltaText 从 TextBlockDeltaEvent 提取增量文本。
//
// 参数：
//   - event: 任意事件
//
// 返回：
//   - string: 增量文本内容
//   - bool: 是否成功提取（false 表示不是 TextBlockDeltaEvent）
func GetDeltaText(event interface{}) (string, bool) {
	if e, ok := event.(TextBlockDeltaEvent); ok {
		return e.Delta, true
	}
	return "", false
}

// GetDeltaThinking 从 ThinkingBlockDeltaEvent 提取增量思考。
//
// 参数：
//   - event: 任意事件
//
// 返回：
//   - string: 增量思考文本
//   - bool: 是否成功提取
func GetDeltaThinking(event interface{}) (string, bool) {
	if e, ok := event.(ThinkingBlockDeltaEvent); ok {
		return e.Delta, true
	}
	return "", false
}

// GetToolCallName 从 ToolCallStartEvent 提取工具调用名称。
//
// 参数：
//   - event: 任意事件
//
// 返回：
//   - string: 工具名称
//   - bool: 是否成功提取
func GetToolCallName(event interface{}) (string, bool) {
	if e, ok := event.(ToolCallStartEvent); ok {
		return e.ToolCallName, true
	}
	return "", false
}

// GetToolResultState 从 ToolResultEndEvent 提取最终状态。
//
// 参数：
//   - event: 任意事件
//
// 返回：
//   - ToolResultState: 最终状态
//   - bool: 是否成功提取
func GetToolResultState(event interface{}) (ToolResultState, bool) {
	if e, ok := event.(ToolResultEndEvent); ok {
		return e.State, true
	}
	return "", false
}

// =============================================
// 事件类型判断（前端实时显示辅助）
// =============================================

// IsReplyStart 判断是否是回复开始事件。
func IsReplyStart(event interface{}) bool {
	_, ok := event.(ReplyStartEvent)
	return ok
}

// IsReplyEnd 判断是否是回复结束事件。
func IsReplyEnd(event interface{}) bool {
	_, ok := event.(ReplyEndEvent)
	return ok
}

// IsTextDelta 判断是否是文本增量事件。
func IsTextDelta(event interface{}) bool {
	_, ok := event.(TextBlockDeltaEvent)
	return ok
}

// IsToolCallStart 判断是否是工具调用开始事件。
func IsToolCallStart(event interface{}) bool {
	_, ok := event.(ToolCallStartEvent)
	return ok
}

// =============================================
// JSON 序列化辅助
// =============================================

// EventEnvelope 事件包装结构（用于 JSON 序列化，携带类型信息）。
//
// 字段：
//   - Type: 事件类型标识
//   - Event: 实际事件数据
type EventEnvelope struct {
	Type  EventType   `json:"type"`  // 事件类型标识
	Event interface{} `json:"event"` // 实际事件数据
}

// MarshalEvent 将事件序列化为 JSON（含类型标识）。
//
// 参数：
//   - event: 任意事件
//
// 返回：
//   - []byte: JSON 字节
//   - error: 序列化错误
func MarshalEvent(event interface{}) ([]byte, error) {
	etype := getEventType(event)
	envelope := EventEnvelope{
		Type:  etype,
		Event: event,
	}
	return json.Marshal(envelope)
}

// getEventType 通过类型断言获取事件类型标识符。
//
// 参数：
//   - event: 任意事件
//
// 返回：
//   - EventType: 事件类型标识
func getEventType(event interface{}) EventType {
	switch event.(type) {
	case ReplyStartEvent:
		return EventTypeReplyStart
	case ReplyEndEvent:
		return EventTypeReplyEnd
	case ExceedMaxItersEvent:
		return EventTypeExceedMaxIters
	case TextBlockStartEvent:
		return EventTypeTextBlockStart
	case TextBlockDeltaEvent:
		return EventTypeTextBlockDelta
	case TextBlockEndEvent:
		return EventTypeTextBlockEnd
	case ThinkingBlockStartEvent:
		return EventTypeThinkingBlockStart
	case ThinkingBlockDeltaEvent:
		return EventTypeThinkingBlockDelta
	case ThinkingBlockEndEvent:
		return EventTypeThinkingBlockEnd
	case DataBlockStartEvent:
		return EventTypeDataBlockStart
	case DataBlockDeltaEvent:
		return EventTypeDataBlockDelta
	case DataBlockEndEvent:
		return EventTypeDataBlockEnd
	case ToolCallStartEvent:
		return EventTypeToolCallStart
	case ToolCallDeltaEvent:
		return EventTypeToolCallDelta
	case ToolCallEndEvent:
		return EventTypeToolCallEnd
	case ToolResultStartEvent:
		return EventTypeToolResultStart
	case ToolResultTextDeltaEvent:
		return EventTypeToolResultTextDelta
	case ToolResultDataDeltaEvent:
		return EventTypeToolResultDataDelta
	case ToolResultEndEvent:
		return EventTypeToolResultEnd
	case ModelCallStartEvent:
		return EventTypeModelCallStart
	case ModelCallEndEvent:
		return EventTypeModelCallEnd
	case RequireUserConfirmEvent:
		return EventTypeRequireUserConfirm
	case RequireExternalExecutionEvent:
		return EventTypeRequireExternalExecution
	case UserConfirmResultEvent:
		return EventTypeUserConfirmResult
	case ExternalExecutionResultEvent:
		return EventTypeExternalExecutionResult
	case HintBlockEvent:
		return EventTypeHintBlock
	case CustomEvent:
		return EventTypeCustom
	default:
		return EventType("UNKNOWN")
	}
}
