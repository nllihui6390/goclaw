package agent

// =============================================
// 事件系统（ 的 Event 设计）
//
// 事件（Event）是消息的流式对应物。Agent 执行过程中产出一系列
// Event，表示增量进度——文本 token 到达、工具调用逐步构建、结果流式返回。
//
// 每个事件都是轻量且自包含的，共享 reply_id 关联到正在构建的消息。
// 事件遵循 start → delta → end 模式。
//
// 同一次回复中的所有事件共享相同的 reply_id。
// 在回复内部，用 block_id 关联文本/思考/数据块事件，
// 用 tool_call_id 关联工具调用和工具结果事件。
//
// 事件类型一览：
//   生命周期: ReplyStart, ReplyEnd, ExceedMaxIters
//   文本流式: TextBlockStart → TextBlockDelta → TextBlockEnd
//   思考流式: ThinkingBlockStart → ThinkingBlockDelta → ThinkingBlockEnd
//   数据流式: DataBlockStart → DataBlockDelta → DataBlockEnd
//   工具调用: ToolCallStart → ToolCallDelta → ToolCallEnd
//   工具结果: ToolResultStart → TextDelta/DataDelta → ToolResultEnd
//   模型调用: ModelCallStart → ModelCallEnd
//   人工介入: RequireUserConfirm, RequireExternalExecution,
//             UserConfirmResult, ExternalExecutionResult
//   一次性:   HintBlock, Custom
// =============================================

// =============================================
// EventType 事件类型标识
// =============================================

// EventType 事件类型标识字符串。
//
// 用于 JSON 序列化时的 type 字段，标识事件的具体类型。
type EventType string

const (
	// --- 生命周期事件 ---
	// EventTypeReplyStart Agent 开始新的回复
	EventTypeReplyStart EventType = "REPLY_START"
	// EventTypeReplyEnd Agent 完成回复
	EventTypeReplyEnd EventType = "REPLY_END"
	// EventTypeExceedMaxIters 达到最大推理-执行迭代次数
	EventTypeExceedMaxIters EventType = "EXCEED_MAX_ITERS"

	// --- 文本流式事件 ---
	// EventTypeTextBlockStart 新的文本块开始
	EventTypeTextBlockStart EventType = "TEXT_BLOCK_START"
	// EventTypeTextBlockDelta 增量文本内容到达
	EventTypeTextBlockDelta EventType = "TEXT_BLOCK_DELTA"
	// EventTypeTextBlockEnd 文本块完成
	EventTypeTextBlockEnd EventType = "TEXT_BLOCK_END"

	// --- 思考流式事件 ---
	// EventTypeThinkingBlockStart 新的思考块开始
	EventTypeThinkingBlockStart EventType = "THINKING_BLOCK_START"
	// EventTypeThinkingBlockDelta 增量思考内容到达
	EventTypeThinkingBlockDelta EventType = "THINKING_BLOCK_DELTA"
	// EventTypeThinkingBlockEnd 思考块完成
	EventTypeThinkingBlockEnd EventType = "THINKING_BLOCK_END"

	// --- 数据流式事件 ---
	// EventTypeDataBlockStart 新的数据块开始（图片、音频等）
	EventTypeDataBlockStart EventType = "DATA_BLOCK_START"
	// EventTypeDataBlockDelta 增量二进制数据到达
	EventTypeDataBlockDelta EventType = "DATA_BLOCK_DELTA"
	// EventTypeDataBlockEnd 数据块完成
	EventTypeDataBlockEnd EventType = "DATA_BLOCK_END"

	// --- 工具调用流式事件 ---
	// EventTypeToolCallStart Agent 开始一次工具调用
	EventTypeToolCallStart EventType = "TOOL_CALL_START"
	// EventTypeToolCallDelta 增量工具调用参数到达
	EventTypeToolCallDelta EventType = "TOOL_CALL_DELTA"
	// EventTypeToolCallEnd 工具调用参数完成
	EventTypeToolCallEnd EventType = "TOOL_CALL_END"

	// --- 工具结果流式事件 ---
	// EventTypeToolResultStart 工具开始执行
	EventTypeToolResultStart EventType = "TOOL_RESULT_START"
	// EventTypeToolResultTextDelta 工具的增量文本输出到达
	EventTypeToolResultTextDelta EventType = "TOOL_RESULT_TEXT_DELTA"
	// EventTypeToolResultDataDelta 工具的二进制数据输出到达
	EventTypeToolResultDataDelta EventType = "TOOL_RESULT_DATA_DELTA"
	// EventTypeToolResultEnd 工具执行完成
	EventTypeToolResultEnd EventType = "TOOL_RESULT_END"

	// --- 模型调用事件 ---
	// EventTypeModelCallStart 模型 API 调用开始
	EventTypeModelCallStart EventType = "MODEL_CALL_START"
	// EventTypeModelCallEnd 模型 API 调用完成
	EventTypeModelCallEnd EventType = "MODEL_CALL_END"

	// --- 人工介入事件 ---
	// EventTypeRequireUserConfirm Agent 暂停等待用户确认
	EventTypeRequireUserConfirm EventType = "REQUIRE_USER_CONFIRM"
	// EventTypeRequireExternalExecution Agent 暂停等待外部执行
	EventTypeRequireExternalExecution EventType = "REQUIRE_EXTERNAL_EXECUTION"
	// EventTypeUserConfirmResult 用户提供确认结果（输入事件）
	EventTypeUserConfirmResult EventType = "USER_CONFIRM_RESULT"
	// EventTypeExternalExecutionResult 外部系统提供执行结果（输入事件）
	EventTypeExternalExecutionResult EventType = "EXTERNAL_EXECUTION_RESULT"

	// --- 一次性事件 ---
	// EventTypeHintBlock 提示块注入到 Agent 上下文
	EventTypeHintBlock EventType = "HINT_BLOCK"
	// EventTypeCustom 通用可扩展事件
	EventTypeCustom EventType = "CUSTOM"
)

// =============================================
// Event 基础结构
// =============================================

// Event 所有事件的公共基类字段。
//
// Go 没有 Python 的继承，使用嵌入字段模式：每个具体事件类型
// 嵌入 Event，自动继承 ID、CreatedAt、ReplyID 三个字段。
type Event struct {
	ID        string `json:"id"`         // 唯一事件标识符
	CreatedAt string `json:"created_at"` // 创建时间（ISO 8601）
	ReplyID   string `json:"reply_id"`   // 关联到正在构建的消息 ID
}

// newEvent 创建事件基础字段（内部使用）。
//
// 参数：
//   - replyID: 关联的回复 ID
//
// 返回：
//   - Event: 带 ID 和时间戳的基础 Event
func newEvent(replyID string) Event {
	return Event{
		ID:        generateID("evt"),
		CreatedAt: nowISO(),
		ReplyID:   replyID,
	}
}

// =============================================
// 生命周期事件
// =============================================

// ReplyStartEvent Agent 开始新的回复。
//
// 字段：
//   - ReplyID: 回复消息 ID
//   - SessionID: 会话 ID
//   - Name: Agent 名称
//   - Role: Agent 角色（默认 "assistant"）
type ReplyStartEvent struct {
	Event
	SessionID string `json:"session_id"` // 会话 ID
	Name      string `json:"name"`       // Agent 名称
	Role      string `json:"role"`       // Agent 角色（默认 "assistant"）
}

// NewReplyStartEvent 创建回复开始事件。
//
// 参数：
//   - replyID: 回复唯一标识符
//   - sessionID: 会话 ID
//   - name: Agent 名称
//
// 返回：
//   - ReplyStartEvent: 回复开始事件
func NewReplyStartEvent(replyID, sessionID, name string) ReplyStartEvent {
	return ReplyStartEvent{
		Event:     newEvent(replyID),
		SessionID: sessionID,
		Name:      name,
		Role:      "assistant",
	}
}

// ReplyEndEvent Agent 完成回复。
//
// 字段：
//   - ReplyID: 回复消息 ID
//   - SessionID: 会话 ID
//   - TotalInputTokens: 本次回复累计输入 token 数（所有迭代的总和）
//   - TotalOutputTokens: 本次回复累计输出 token 数（所有迭代的总和）
type ReplyEndEvent struct {
	Event
	SessionID         string `json:"session_id"`          // 会话 ID
	TotalInputTokens  int    `json:"total_input_tokens"`  // 累计输入 token
	TotalOutputTokens int    `json:"total_output_tokens"` // 累计输出 token
}

// NewReplyEndEvent 创建回复结束事件。
//
// 参数：
//   - replyID: 回复 ID
//   - sessionID: 会话 ID
//
// 返回：
//   - ReplyEndEvent: 回复结束事件
func NewReplyEndEvent(replyID, sessionID string) ReplyEndEvent {
	return ReplyEndEvent{
		Event:     newEvent(replyID),
		SessionID: sessionID,
	}
}

// NewReplyEndEventWithTokens 创建带 token 统计的回复结束事件。
func NewReplyEndEventWithTokens(replyID, sessionID string, totalInputTokens, totalOutputTokens int) ReplyEndEvent {
	return ReplyEndEvent{
		Event:             newEvent(replyID),
		SessionID:         sessionID,
		TotalInputTokens:  totalInputTokens,
		TotalOutputTokens: totalOutputTokens,
	}
}

// ExceedMaxItersEvent 达到最大推理-执行迭代次数。
//
// 字段：
//   - ReplyID: 回复消息 ID
//   - Name: Agent 名称
type ExceedMaxItersEvent struct {
	Event
	Name string `json:"name"` // Agent 名称
}

// NewExceedMaxItersEvent 创建超最大迭代事件。
//
// 参数：
//   - replyID: 回复 ID
//   - name: Agent 名称
//
// 返回：
//   - ExceedMaxItersEvent: 超迭代事件
func NewExceedMaxItersEvent(replyID, name string) ExceedMaxItersEvent {
	return ExceedMaxItersEvent{
		Event: newEvent(replyID),
		Name:  name,
	}
}

// =============================================
// 文本流式事件（start → delta → end）
// =============================================

// TextBlockStartEvent 新的文本块开始。
//
// 字段：
//   - ReplyID: 回复 ID
//   - BlockID: 文本块唯一标识符
type TextBlockStartEvent struct {
	Event
	BlockID string `json:"block_id"` // 文本块唯一标识符
}

// NewTextBlockStartEvent 创建文本块开始事件。
//
// 参数：
//   - replyID: 回复 ID
//   - blockID: 文本块唯一标识符
//
// 返回：
//   - TextBlockStartEvent: 文本块开始事件
func NewTextBlockStartEvent(replyID, blockID string) TextBlockStartEvent {
	return TextBlockStartEvent{
		Event:   newEvent(replyID),
		BlockID: blockID,
	}
}

// TextBlockDeltaEvent 增量文本内容到达。
//
// 字段：
//   - ReplyID: 回复 ID
//   - BlockID: 关联的文本块 ID
//   - Delta: 增量文本内容（单个 token 或片段）
type TextBlockDeltaEvent struct {
	Event
	BlockID string `json:"block_id"` // 关联的文本块 ID
	Delta   string `json:"delta"`    // 增量文本内容
}

// NewTextBlockDeltaEvent 创建文本增量事件。
//
// 参数：
//   - replyID: 回复 ID
//   - blockID: 文本块 ID
//   - delta: 增量文本
//
// 返回：
//   - TextBlockDeltaEvent: 文本增量事件
func NewTextBlockDeltaEvent(replyID, blockID, delta string) TextBlockDeltaEvent {
	return TextBlockDeltaEvent{
		Event:   newEvent(replyID),
		BlockID: blockID,
		Delta:   delta,
	}
}

// TextBlockEndEvent 文本块完成。
//
// 字段：
//   - ReplyID: 回复 ID
//   - BlockID: 文本块 ID
type TextBlockEndEvent struct {
	Event
	BlockID string `json:"block_id"` // 文本块 ID
}

// NewTextBlockEndEvent 创建文本块结束事件。
//
// 参数：
//   - replyID: 回复 ID
//   - blockID: 文本块 ID
//
// 返回：
//   - TextBlockEndEvent: 文本块结束事件
func NewTextBlockEndEvent(replyID, blockID string) TextBlockEndEvent {
	return TextBlockEndEvent{
		Event:   newEvent(replyID),
		BlockID: blockID,
	}
}

// =============================================
// 思考流式事件（start → delta → end）
// =============================================

// ThinkingBlockStartEvent 新的思考块开始。
//
// 字段：
//   - ReplyID: 回复 ID
//   - BlockID: 思考块唯一标识符
type ThinkingBlockStartEvent struct {
	Event
	BlockID string `json:"block_id"` // 思考块唯一标识符
}

// NewThinkingBlockStartEvent 创建思考块开始事件。
//
// 参数：
//   - replyID: 回复 ID
//   - blockID: 思考块唯一标识符
//
// 返回：
//   - ThinkingBlockStartEvent: 思考块开始事件
func NewThinkingBlockStartEvent(replyID, blockID string) ThinkingBlockStartEvent {
	return ThinkingBlockStartEvent{
		Event:   newEvent(replyID),
		BlockID: blockID,
	}
}

// ThinkingBlockDeltaEvent 增量思考内容到达。
//
// 字段：
//   - ReplyID: 回复 ID
//   - BlockID: 关联的思考块 ID
//   - Delta: 增量思考文本
type ThinkingBlockDeltaEvent struct {
	Event
	BlockID string `json:"block_id"` // 关联的思考块 ID
	Delta   string `json:"delta"`    // 增量思考文本
}

// NewThinkingBlockDeltaEvent 创建思考增量事件。
//
// 参数：
//   - replyID: 回复 ID
//   - blockID: 思考块 ID
//   - delta: 增量思考文本
//
// 返回：
//   - ThinkingBlockDeltaEvent: 思考增量事件
func NewThinkingBlockDeltaEvent(replyID, blockID, delta string) ThinkingBlockDeltaEvent {
	return ThinkingBlockDeltaEvent{
		Event:   newEvent(replyID),
		BlockID: blockID,
		Delta:   delta,
	}
}

// ThinkingBlockEndEvent 思考块完成。
//
// 字段：
//   - ReplyID: 回复 ID
//   - BlockID: 思考块 ID
type ThinkingBlockEndEvent struct {
	Event
	BlockID string `json:"block_id"` // 思考块 ID
}

// NewThinkingBlockEndEvent 创建思考块结束事件。
//
// 参数：
//   - replyID: 回复 ID
//   - blockID: 思考块 ID
//
// 返回：
//   - ThinkingBlockEndEvent: 思考块结束事件
func NewThinkingBlockEndEvent(replyID, blockID string) ThinkingBlockEndEvent {
	return ThinkingBlockEndEvent{
		Event:   newEvent(replyID),
		BlockID: blockID,
	}
}

// =============================================
// 数据流式事件（start → delta → end）
// =============================================

// DataBlockStartEvent 新的数据块开始（图片、音频等）。
//
// 字段：
//   - ReplyID: 回复 ID
//   - BlockID: 数据块唯一标识符
//   - MediaType: MIME 类型（如 "image/png"）
type DataBlockStartEvent struct {
	Event
	BlockID   string `json:"block_id"`   // 数据块唯一标识符
	MediaType string `json:"media_type"` // MIME 类型
}

// NewDataBlockStartEvent 创建数据块开始事件。
//
// 参数：
//   - replyID: 回复 ID
//   - blockID: 数据块 ID
//   - mediaType: MIME 类型
//
// 返回：
//   - DataBlockStartEvent: 数据块开始事件
func NewDataBlockStartEvent(replyID, blockID, mediaType string) DataBlockStartEvent {
	return DataBlockStartEvent{
		Event:     newEvent(replyID),
		BlockID:   blockID,
		MediaType: mediaType,
	}
}

// DataBlockDeltaEvent 增量二进制数据到达。
//
// 字段：
//   - ReplyID: 回复 ID
//   - BlockID: 关联的数据块 ID
//   - Data: 增量 base64 编码数据
//   - MediaType: MIME 类型
type DataBlockDeltaEvent struct {
	Event
	BlockID   string `json:"block_id"`   // 关联的数据块 ID
	Data      string `json:"data"`       // 增量 base64 编码数据
	MediaType string `json:"media_type"` // MIME 类型
}

// NewDataBlockDeltaEvent 创建数据增量事件。
//
// 参数：
//   - replyID: 回复 ID
//   - blockID: 数据块 ID
//   - data: 增量 base64 数据
//   - mediaType: MIME 类型
//
// 返回：
//   - DataBlockDeltaEvent: 数据增量事件
func NewDataBlockDeltaEvent(replyID, blockID, data, mediaType string) DataBlockDeltaEvent {
	return DataBlockDeltaEvent{
		Event:     newEvent(replyID),
		BlockID:   blockID,
		Data:      data,
		MediaType: mediaType,
	}
}

// DataBlockEndEvent 数据块完成。
//
// 字段：
//   - ReplyID: 回复 ID
//   - BlockID: 数据块 ID
type DataBlockEndEvent struct {
	Event
	BlockID string `json:"block_id"` // 数据块 ID
}

// NewDataBlockEndEvent 创建数据块结束事件。
//
// 参数：
//   - replyID: 回复 ID
//   - blockID: 数据块 ID
//
// 返回：
//   - DataBlockEndEvent: 数据块结束事件
func NewDataBlockEndEvent(replyID, blockID string) DataBlockEndEvent {
	return DataBlockEndEvent{
		Event:   newEvent(replyID),
		BlockID: blockID,
	}
}

// =============================================
// 工具调用流式事件（start → delta → end）
// =============================================

// ToolCallStartEvent Agent 开始一次工具调用。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ToolCallID: 工具调用唯一标识符
//   - ToolCallName: 被调用的工具名称
type ToolCallStartEvent struct {
	Event
	ToolCallID   string `json:"tool_call_id"`   // 工具调用唯一标识符
	ToolCallName string `json:"tool_call_name"` // 被调用的工具名称
}

// NewToolCallStartEvent 创建工具调用开始事件。
//
// 参数：
//   - replyID: 回复 ID
//   - toolCallID: 工具调用 ID
//   - toolCallName: 工具名称
//
// 返回：
//   - ToolCallStartEvent: 工具调用开始事件
func NewToolCallStartEvent(replyID, toolCallID, toolCallName string) ToolCallStartEvent {
	return ToolCallStartEvent{
		Event:        newEvent(replyID),
		ToolCallID:   toolCallID,
		ToolCallName: toolCallName,
	}
}

// ToolCallDeltaEvent 增量工具调用参数到达。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ToolCallID: 关联的工具调用 ID
//   - Delta: 增量 JSON 参数片段
type ToolCallDeltaEvent struct {
	Event
	ToolCallID string `json:"tool_call_id"` // 关联的工具调用 ID
	Delta      string `json:"delta"`        // 增量 JSON 参数片段
}

// NewToolCallDeltaEvent 创建工具调用增量事件。
//
// 参数：
//   - replyID: 回复 ID
//   - toolCallID: 工具调用 ID
//   - delta: 增量参数片段（JSON 字符串）
//
// 返回：
//   - ToolCallDeltaEvent: 工具调用增量事件
func NewToolCallDeltaEvent(replyID, toolCallID, delta string) ToolCallDeltaEvent {
	return ToolCallDeltaEvent{
		Event:      newEvent(replyID),
		ToolCallID: toolCallID,
		Delta:      delta,
	}
}

// ToolCallEndEvent 工具调用参数完成。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ToolCallID: 工具调用 ID
type ToolCallEndEvent struct {
	Event
	ToolCallID string `json:"tool_call_id"` // 工具调用 ID
}

// NewToolCallEndEvent 创建工具调用结束事件。
//
// 参数：
//   - replyID: 回复 ID
//   - toolCallID: 工具调用 ID
//
// 返回：
//   - ToolCallEndEvent: 工具调用结束事件
func NewToolCallEndEvent(replyID, toolCallID string) ToolCallEndEvent {
	return ToolCallEndEvent{
		Event:      newEvent(replyID),
		ToolCallID: toolCallID,
	}
}

// =============================================
// 工具结果流式事件（start → delta → end）
// =============================================

// ToolResultStartEvent 工具开始执行。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ToolCallID: 对应工具调用的 ID
//   - ToolCallName: 工具名称
type ToolResultStartEvent struct {
	Event
	ToolCallID   string `json:"tool_call_id"`   // 对应工具调用的 ID
	ToolCallName string `json:"tool_call_name"` // 工具名称
}

// NewToolResultStartEvent 创建工具结果开始事件。
//
// 参数：
//   - replyID: 回复 ID
//   - toolCallID: 对应的工具调用 ID
//   - toolCallName: 工具名称
//
// 返回：
//   - ToolResultStartEvent: 工具结果开始事件
func NewToolResultStartEvent(replyID, toolCallID, toolCallName string) ToolResultStartEvent {
	return ToolResultStartEvent{
		Event:        newEvent(replyID),
		ToolCallID:   toolCallID,
		ToolCallName: toolCallName,
	}
}

// ToolResultTextDeltaEvent 工具的增量文本输出到达。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ToolCallID: 关联的工具调用 ID
//   - Delta: 增量文本内容
type ToolResultTextDeltaEvent struct {
	Event
	ToolCallID string `json:"tool_call_id"` // 关联的工具调用 ID
	Delta      string `json:"delta"`        // 增量文本内容
}

// NewToolResultTextDeltaEvent 创建工具结果文本增量事件。
//
// 参数：
//   - replyID: 回复 ID
//   - toolCallID: 工具调用 ID
//   - delta: 增量文本
//
// 返回：
//   - ToolResultTextDeltaEvent: 工具结果文本增量事件
func NewToolResultTextDeltaEvent(replyID, toolCallID, delta string) ToolResultTextDeltaEvent {
	return ToolResultTextDeltaEvent{
		Event:      newEvent(replyID),
		ToolCallID: toolCallID,
		Delta:      delta,
	}
}

// ToolResultDataDeltaEvent 工具的二进制数据输出到达。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ToolCallID: 关联的工具调用 ID
//   - BlockID: 数据块唯一标识符
//   - MediaType: MIME 类型
//   - Data: base64 编码数据（与 URL 互斥）
//   - URL: 资源 URL（与 Data 互斥）
type ToolResultDataDeltaEvent struct {
	Event
	ToolCallID string `json:"tool_call_id"` // 关联的工具调用 ID
	BlockID    string `json:"block_id"`     // 数据块唯一标识符
	MediaType  string `json:"media_type"`   // MIME 类型
	Data       string `json:"data"`         // base64 编码数据（与 url 互斥）
	URL        string `json:"url"`          // URL（与 data 互斥）
}

// NewToolResultDataDeltaEvent 创建工具结果数据增量事件。
//
// 参数：
//   - replyID: 回复 ID
//   - toolCallID: 工具调用 ID
//   - blockID: 数据块 ID
//   - mediaType: MIME 类型
//   - dataOrURL: 数据内容（base64 或 URL）
//   - isURL: true 表示 dataOrURL 是 URL，false 表示是 base64 数据
//
// 返回：
//   - ToolResultDataDeltaEvent: 工具结果数据增量事件
func NewToolResultDataDeltaEvent(replyID, toolCallID, blockID, mediaType string, dataOrURL string, isURL bool) ToolResultDataDeltaEvent {
	evt := ToolResultDataDeltaEvent{
		Event:      newEvent(replyID),
		ToolCallID: toolCallID,
		BlockID:    blockID,
		MediaType:  mediaType,
	}
	if isURL {
		evt.URL = dataOrURL
	} else {
		evt.Data = dataOrURL
	}
	return evt
}

// ToolResultEndEvent 工具执行完成。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ToolCallID: 关联的工具调用 ID
//   - State: 最终状态：SUCCESS / ERROR / INTERRUPTED / DENIED / RUNNING
type ToolResultEndEvent struct {
	Event
	ToolCallID string          `json:"tool_call_id"` // 关联的工具调用 ID
	State      ToolResultState `json:"state"`        // 最终状态
}

// NewToolResultEndEvent 创建工具结果结束事件。
//
// 参数：
//   - replyID: 回复 ID
//   - toolCallID: 工具调用 ID
//   - state: 最终状态（如 ToolResultStateSuccess）
//
// 返回：
//   - ToolResultEndEvent: 工具结果结束事件
func NewToolResultEndEvent(replyID, toolCallID string, state ToolResultState) ToolResultEndEvent {
	return ToolResultEndEvent{
		Event:      newEvent(replyID),
		ToolCallID: toolCallID,
		State:      state,
	}
}

// =============================================
// 模型调用事件
// =============================================

// ModelCallStartEvent 模型 API 调用开始。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ModelName: 被调用的模型名称
type ModelCallStartEvent struct {
	Event
	ModelName string `json:"model_name"` // 被调用的模型名称
}

// NewModelCallStartEvent 创建模型调用开始事件。
//
// 参数：
//   - replyID: 回复 ID
//   - modelName: 模型名称
//
// 返回：
//   - ModelCallStartEvent: 模型调用开始事件
func NewModelCallStartEvent(replyID, modelName string) ModelCallStartEvent {
	return ModelCallStartEvent{
		Event:     newEvent(replyID),
		ModelName: modelName,
	}
}

// ModelCallEndEvent 模型 API 调用完成。
//
// 字段：
//   - ReplyID: 回复 ID
//   - InputTokens: 输入 token 数量
//   - OutputTokens: 输出 token 数量
type ModelCallEndEvent struct {
	Event
	InputTokens  int `json:"input_tokens"`  // 输入 token 数量
	OutputTokens int `json:"output_tokens"` // 输出 token 数量
}

// NewModelCallEndEvent 创建模型调用结束事件。
//
// 参数：
//   - replyID: 回复 ID
//   - inputTokens: 输入 token 数
//   - outputTokens: 输出 token 数
//
// 返回：
//   - ModelCallEndEvent: 模型调用结束事件
func NewModelCallEndEvent(replyID string, inputTokens, outputTokens int) ModelCallEndEvent {
	return ModelCallEndEvent{
		Event:        newEvent(replyID),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
}

// =============================================
// 人工介入事件
// =============================================

// RequireUserConfirmEvent Agent 暂停等待用户确认。
//
// 当权限系统判断某个工具调用需要用户批准时发出。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ToolCalls: 待用户确认的工具调用列表（ToolCallBlock）
type RequireUserConfirmEvent struct {
	Event
	ToolCalls []ContentBlock `json:"tool_calls"` // 待用户确认的工具调用列表
}

// NewRequireUserConfirmEvent 创建用户确认请求事件。
//
// 参数：
//   - replyID: 回复 ID
//   - toolCalls: 待确认的工具调用块列表
//
// 返回：
//   - RequireUserConfirmEvent: 用户确认请求事件
func NewRequireUserConfirmEvent(replyID string, toolCalls []ContentBlock) RequireUserConfirmEvent {
	return RequireUserConfirmEvent{
		Event:     newEvent(replyID),
		ToolCalls: toolCalls,
	}
}

// RequireExternalExecutionEvent Agent 暂停等待外部执行。
//
// 当工具标记为 is_external_tool = true 时发出。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ToolCalls: 待外部执行的工具调用列表
type RequireExternalExecutionEvent struct {
	Event
	ToolCalls []ContentBlock `json:"tool_calls"` // 待外部执行的工具调用列表
}

// NewRequireExternalExecutionEvent 创建外部执行请求事件。
//
// 参数：
//   - replyID: 回复 ID
//   - toolCalls: 待外部执行的工具调用块列表
//
// 返回：
//   - RequireExternalExecutionEvent: 外部执行请求事件
func NewRequireExternalExecutionEvent(replyID string, toolCalls []ContentBlock) RequireExternalExecutionEvent {
	return RequireExternalExecutionEvent{
		Event:     newEvent(replyID),
		ToolCalls: toolCalls,
	}
}

// ConfirmResult 单个确认结果。
//
// 字段：
//   - ToolCallID: 工具调用 ID
//   - Approved: 是否批准
//   - Reason: 批准/拒绝理由（可选）
type ConfirmResult struct {
	ToolCallID string `json:"tool_call_id"`     // 工具调用 ID
	Approved   bool   `json:"approved"`         // 是否批准
	Reason     string `json:"reason,omitempty"` // 批准/拒绝理由
}

// UserConfirmResultEvent 用户提供确认结果（输入事件）。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ConfirmResults: 每个待确认工具调用的确认结果
type UserConfirmResultEvent struct {
	Event
	ConfirmResults []ConfirmResult `json:"confirm_results"` // 确认结果列表
}

// NewUserConfirmResultEvent 创建用户确认结果事件。
//
// 参数：
//   - replyID: 回复 ID
//   - results: 确认结果列表
//
// 返回：
//   - UserConfirmResultEvent: 用户确认结果事件
func NewUserConfirmResultEvent(replyID string, results []ConfirmResult) UserConfirmResultEvent {
	return UserConfirmResultEvent{
		Event:          newEvent(replyID),
		ConfirmResults: results,
	}
}

// ExternalExecutionResultEvent 外部系统提供执行结果（输入事件）。
//
// 字段：
//   - ReplyID: 回复 ID
//   - ExecutionResults: 外部执行器返回的 ToolResultBlock 列表
type ExternalExecutionResultEvent struct {
	Event
	ExecutionResults []ContentBlock `json:"execution_results"` // 外部执行结果列表
}

// NewExternalExecutionResultEvent 创建外部执行结果事件。
//
// 参数：
//   - replyID: 回复 ID
//   - results: ToolResultBlock 列表
//
// 返回：
//   - ExternalExecutionResultEvent: 外部执行结果事件
func NewExternalExecutionResultEvent(replyID string, results []ContentBlock) ExternalExecutionResultEvent {
	return ExternalExecutionResultEvent{
		Event:            newEvent(replyID),
		ExecutionResults: results,
	}
}

// =============================================
// 一次性事件
// =============================================

// HintBlockEvent 提示块注入到 Agent 上下文。
//
// 与文本/思考/数据块不同，不遵循 start → delta → end 模式。
// 完整载荷在单个事件中到达。
//
// 字段：
//   - ReplyID: 回复 ID
//   - BlockID: 提示块唯一标识符
//   - Hint: 提示载荷（string 或 []ContentBlock）
//   - Source: 发送方/来源标签
type HintBlockEvent struct {
	Event
	BlockID string      `json:"block_id"` // 提示块唯一标识符
	Hint    interface{} `json:"hint"`     // 提示载荷
	Source  string      `json:"source"`   // 来源标签
}

// NewHintBlockEvent 创建提示块事件。
//
// 参数：
//   - replyID: 回复 ID
//   - blockID: 提示块 ID
//   - hint: 提示载荷
//   - source: 来源标签
//
// 返回：
//   - HintBlockEvent: 提示块事件
func NewHintBlockEvent(replyID, blockID string, hint interface{}, source string) HintBlockEvent {
	return HintBlockEvent{
		Event:   newEvent(replyID),
		BlockID: blockID,
		Hint:    hint,
		Source:  source,
	}
}

// CustomEvent 通用可扩展事件。
//
// 用于通知订阅者状态变更（任务进度、团队成员、权限更新…），
// 无需污染核心 Agent 事件枚举。
//
// 字段：
//   - ReplyID: 回复 ID
//   - Name: 信号名称
//   - Value: 可 JSON 序列化的载荷
type CustomEvent struct {
	Event
	Name  string                 `json:"name"`  // 信号名称
	Value map[string]interface{} `json:"value"` // 载荷
}

// NewCustomEvent 创建自定义事件。
//
// 参数：
//   - replyID: 回复 ID
//   - name: 信号名称
//   - value: 载荷（可 JSON 序列化）
//
// 返回：
//   - CustomEvent: 自定义事件
func NewCustomEvent(replyID, name string, value map[string]interface{}) CustomEvent {
	return CustomEvent{
		Event: newEvent(replyID),
		Name:  name,
		Value: value,
	}
}
