package agent

// =============================================
// 消息系统（ 的 Message 设计）
//
// 消息（Msg）是对话的基本单元，代表一个完整的对话轮次。
// content 以有序的类型化内容块列表表示，不再使用纯字符串。
//
// 内容块类型：
//   - TextBlock: 纯文本
//   - DataBlock: 二进制数据（base64 或 URL）
//   - ThinkingBlock: 模型推理过程（思维链）
//   - ToolCallBlock: 工具调用
//   - ToolResultBlock: 工具执行结果
//   - HintBlock: 注入的带外提示
//
// 工厂函数：
//   - UserMsg: 用户消息（仅允许 TextBlock 和 DataBlock）
//   - AssistantMsg: 助手消息（允许所有块类型）
//   - SystemMsg: 系统消息（仅允许 TextBlock）
// =============================================

import (
	"encoding/json"
	"fmt"
	"time"
)

// =============================================
// 角色类型
// =============================================

// Role 消息角色类型。
//
// 三种角色：
//   - RoleUser: 用户消息（仅允许 TextBlock 和 DataBlock）
//   - RoleAssistant: 助手消息（允许所有块类型）
//   - RoleSystem: 系统消息（仅允许 TextBlock）
type Role string

const (
	RoleUser      Role = "user"      // 用户角色
	RoleAssistant Role = "assistant" // 助手角色
	RoleSystem    Role = "system"    // 系统角色
)

// =============================================
// 消息标记（ _MemoryMark）
// =============================================

// MsgMark 消息标记类型。
//
// /AgentScope 的 _MemoryMark 系统：
// 消息可以携带标记，用于压缩管理、上下文过滤等。
// 例如，标记为 MsgMarkCompressed 的消息在构建模型输入时会被排除，
// 只保留摘要 + 未压缩的近期消息。
type MsgMark string

const (
	MsgMarkCompressed MsgMark = "compressed" // 已压缩标记（消息内容已被压缩为摘要）
	MsgMarkHint       MsgMark = "hint"       // 提示标记（上下文注入的提示信息）
)

// =============================================
// ContentBlock — 类型化内容块（AgentScope 风格）
// =============================================

// BlockType 内容块类型标识。
//
// 六种类型对应 AgentScope 的 ContentBlock 类型：
//   - BlockTypeText: 纯文本
//   - BlockTypeData: 二进制数据
//   - BlockTypeThinking: 思维链
//   - BlockTypeToolCall: 工具调用
//   - BlockTypeToolResult: 工具结果
//   - BlockTypeHint: 带外提示
type BlockType string

const (
	BlockTypeText       BlockType = "text"        // 文本块
	BlockTypeData       BlockType = "data"        // 数据块（图片/音频）
	BlockTypeThinking   BlockType = "thinking"    // 思考块（思维链）
	BlockTypeToolCall   BlockType = "tool_call"   // 工具调用块
	BlockTypeToolResult BlockType = "tool_result" // 工具结果块
	BlockTypeHint       BlockType = "hint"        // 提示块
)

// ContentBlock 类型化内容块。
//
// 使用 Type 字段区分块类型，不同类型使用不同的字段组。
// 这种设计在 JSON 序列化时自动省略无关字段（omitempty），
// 比多态接口更简洁且对 JSON 友好。
//
// 字段（按类型）：
//   - TextBlock: Type + Text
//   - DataBlock: Type + Source + MediaType
//   - ThinkingBlock: Type + Thinking
//   - ToolCallBlock: Type + ToolCallID + ToolCallName + ToolCallInput + ToolCallState
//   - ToolResultBlock: Type + ToolCallID + ToolResultOutput + ToolResultState
//   - HintBlock: Type + BlockID + Hint + HintSource
type ContentBlock struct {
	Type BlockType `json:"type"` // 块类型标识（必填）

	// --- TextBlock 字段 ---
	// Text 文本内容（仅 TextBlock）
	Text string `json:"text,omitempty"`

	// --- DataBlock 字段 ---
	// Source 数据来源（base64 或 URL，仅 DataBlock）
	Source *DataSource `json:"source,omitempty"`
	// MediaType MIME 类型（仅 DataBlock 和 ToolResultDataDeltaEvent）
	MediaType string `json:"media_type,omitempty"`

	// --- ThinkingBlock 字段 ---
	// Thinking 思考内容（仅 ThinkingBlock）
	Thinking string `json:"thinking,omitempty"`

	// --- ToolCallBlock 字段 ---
	// ToolCallID 工具调用唯一标识符（仅 ToolCallBlock 和 ToolResultBlock）
	ToolCallID string `json:"tool_call_id,omitempty"`
	// ToolCallName 被调用的工具名称（仅 ToolCallBlock）
	ToolCallName string `json:"tool_call_name,omitempty"`
	// ToolCallInput 工具调用输入（JSON 字符串，仅 ToolCallBlock）
	ToolCallInput string `json:"tool_call_input,omitempty"`
	// ToolCallState 工具调用状态（仅 ToolCallBlock）
	ToolCallState ToolCallState `json:"tool_call_state,omitempty"`

	// --- ToolResultBlock 字段 ---
	// ToolResultOutput 工具执行结果的输出块列表（仅 ToolResultBlock）
	ToolResultOutput []ContentBlock `json:"tool_result_output,omitempty"`
	// ToolResultState 工具执行最终状态（仅 ToolResultBlock）
	ToolResultState ToolResultState `json:"tool_result_state,omitempty"`

	// --- HintBlock 字段 ---
	// BlockID 块唯一标识符（用于流式事件的 block_id 关联）
	BlockID string `json:"block_id,omitempty"`
	// Hint 提示载荷：string 或 []ContentBlock（仅 HintBlock）
	Hint interface{} `json:"hint,omitempty"`
	// HintSource 提示来源标签（仅 HintBlock）
	HintSource string `json:"hint_source,omitempty"`
}

// =============================================
// 数据源（base64 或 URL）
// =============================================

// DataSource 数据来源结构。
//
// 支持两种数据源类型：
//   - DataSourceBase64: base64 编码的内联数据
//   - DataSourceURL: 指向外部资源的 URL
type DataSource struct {
	Type      DataSourceType `json:"type"`                 // 数据源类型："base64" 或 "url"
	Data      string         `json:"data,omitempty"`       // base64 编码数据（与 URL 互斥）
	URL       string         `json:"url,omitempty"`        // URL 引用（与 Data 互斥）
	MediaType string         `json:"media_type,omitempty"` // MIME 类型
}

// DataSourceType 数据源类型标识。
type DataSourceType string

const (
	DataSourceBase64 DataSourceType = "base64" // base64 内联数据
	DataSourceURL    DataSourceType = "url"    // URL 引用
)

// =============================================
// 工具调用/结果状态
// =============================================

// ToolCallState 工具调用状态。
//
// 状态流转：
//
//	PENDING → RUNNING → COMPLETE
//	                   → ERROR
//	PENDING → ASKING（等待用户确认）
type ToolCallState string

const (
	ToolCallStatePending  ToolCallState = "PENDING"  // 等待执行
	ToolCallStateAsking   ToolCallState = "ASKING"   // 等待用户确认
	ToolCallStateRunning  ToolCallState = "RUNNING"  // 执行中
	ToolCallStateComplete ToolCallState = "COMPLETE" // 执行完成
	ToolCallStateError    ToolCallState = "ERROR"    // 执行错误
)

// ToolResultState 工具结果状态。
//
// 五种结果状态：
//   - ToolResultStateSuccess: 执行成功
//   - ToolResultStateError: 执行失败
//   - ToolResultStateInterrupted: 被中断
//   - ToolResultStateDenied: 被权限系统拒绝
//   - ToolResultStateRunning: 仍在执行中（流式结果）
type ToolResultState string

const (
	ToolResultStateSuccess     ToolResultState = "SUCCESS"     // 执行成功
	ToolResultStateError       ToolResultState = "ERROR"       // 执行失败
	ToolResultStateInterrupted ToolResultState = "INTERRUPTED" // 被中断
	ToolResultStateDenied      ToolResultState = "DENIED"      // 被拒绝
	ToolResultStateRunning     ToolResultState = "RUNNING"     // 执行中
)

// =============================================
// ContentBlock 工厂函数
// =============================================

// NewTextBlock 创建文本块。
//
// 参数：
//   - text: 文本内容
//
// 返回：
//   - ContentBlock: TextBlock 类型的 ContentBlock
//
// 示例：
//
//	block := agent.NewTextBlock("Hello, World!")
func NewTextBlock(text string) ContentBlock {
	return ContentBlock{Type: BlockTypeText, Text: text}
}

// NewDataBlockBase64 创建 base64 编码的数据块（图片、音频等）。
//
// 参数：
//   - data: base64 编码的二进制数据
//   - mediaType: MIME 类型（如 "image/png", "audio/mp3"）
//
// 返回：
//   - ContentBlock: DataBlock 类型的 ContentBlock
func NewDataBlockBase64(data, mediaType string) ContentBlock {
	return ContentBlock{
		Type:      BlockTypeData,
		Source:    &DataSource{Type: DataSourceBase64, Data: data, MediaType: mediaType},
		MediaType: mediaType,
	}
}

// NewDataBlockURL 创建 URL 引用的数据块。
//
// 参数：
//   - url: 资源 URL
//   - mediaType: MIME 类型
//
// 返回：
//   - ContentBlock: DataBlock 类型的 ContentBlock
func NewDataBlockURL(url, mediaType string) ContentBlock {
	return ContentBlock{
		Type:      BlockTypeData,
		Source:    &DataSource{Type: DataSourceURL, URL: url, MediaType: mediaType},
		MediaType: mediaType,
	}
}

// NewThinkingBlock 创建思考块（模型推理过程/思维链）。
//
// 参数：
//   - thinking: 思考文本
//
// 返回：
//   - ContentBlock: ThinkingBlock 类型的 ContentBlock
func NewThinkingBlock(thinking string) ContentBlock {
	return ContentBlock{Type: BlockTypeThinking, Thinking: thinking}
}

// NewToolCallBlock 创建工具调用块。
//
// 参数：
//   - id: 工具调用唯一标识符
//   - name: 被调用的工具名称
//   - input: 工具调用输入（JSON 字符串格式）
//
// 返回：
//   - ContentBlock: ToolCallBlock 类型的 ContentBlock（状态为 PENDING）
//
// 示例：
//
//	block := agent.NewToolCallBlock("call_123", "search", `{"query": "golang"}`)
func NewToolCallBlock(id, name, input string) ContentBlock {
	return ContentBlock{
		Type:          BlockTypeToolCall,
		ToolCallID:    id,
		ToolCallName:  name,
		ToolCallInput: input,
		ToolCallState: ToolCallStatePending,
	}
}

// NewToolResultBlock 创建工具结果块。
//
// 参数：
//   - toolCallID: 对应的工具调用 ID
//   - output: 工具输出的内容块列表
//
// 返回：
//   - ContentBlock: ToolResultBlock 类型的 ContentBlock（状态为 SUCCESS）
func NewToolResultBlock(toolCallID string, output []ContentBlock) ContentBlock {
	return ContentBlock{
		Type:             BlockTypeToolResult,
		ToolCallID:       toolCallID,
		ToolResultOutput: output,
		ToolResultState:  ToolResultStateSuccess,
	}
}

// NewToolResultTextBlock 创建纯文本工具结果块（便捷函数）。
//
// 参数：
//   - toolCallID: 对应的工具调用 ID
//   - text: 工具输出的文本内容
//
// 返回：
//   - ContentBlock: ToolResultBlock 类型的 ContentBlock（内部含 TextBlock）
func NewToolResultTextBlock(toolCallID, text string) ContentBlock {
	return ContentBlock{
		Type:             BlockTypeToolResult,
		ToolCallID:       toolCallID,
		ToolResultOutput: []ContentBlock{NewTextBlock(text)},
		ToolResultState:  ToolResultStateSuccess,
	}
}

// NewHintBlock 创建提示块（注入到对话中的带外信息）。
//
// 参数：
//   - blockID: 提示块唯一标识符
//   - hint: 提示载荷（string 或 []ContentBlock）
//   - source: 来源标签（前端用其标识提示来源）
//
// 返回：
//   - ContentBlock: HintBlock 类型的 ContentBlock
func NewHintBlock(blockID string, hint interface{}, source string) ContentBlock {
	return ContentBlock{
		Type:       BlockTypeHint,
		BlockID:    blockID,
		Hint:       hint,
		HintSource: source,
	}
}

// =============================================
// ContentBlock 类型判断方法
// =============================================

// IsText 判断是否是文本块。
func (b ContentBlock) IsText() bool { return b.Type == BlockTypeText }

// IsData 判断是否是数据块。
func (b ContentBlock) IsData() bool { return b.Type == BlockTypeData }

// IsThinking 判断是否是思考块。
func (b ContentBlock) IsThinking() bool { return b.Type == BlockTypeThinking }

// IsToolCall 判断是否是工具调用块。
func (b ContentBlock) IsToolCall() bool { return b.Type == BlockTypeToolCall }

// IsToolResult 判断是否是工具结果块。
func (b ContentBlock) IsToolResult() bool { return b.Type == BlockTypeToolResult }

// IsHint 判断是否是提示块。
func (b ContentBlock) IsHint() bool { return b.Type == BlockTypeHint }

// AsText 获取文本内容（仅 TextBlock 有效）。
//
// 返回：
//   - string: 文本内容
//   - bool: 是否成功获取（false 表示不是 TextBlock）
func (b ContentBlock) AsText() (string, bool) {
	if b.Type == BlockTypeText {
		return b.Text, true
	}
	return "", false
}

// AsToolCall 获取工具调用信息（仅 ToolCallBlock 有效）。
//
// 返回：
//   - id: 工具调用 ID
//   - name: 工具名称
//   - input: 工具输入参数（JSON 字符串）
//   - ok: 是否成功获取（false 表示不是 ToolCallBlock）
func (b ContentBlock) AsToolCall() (id, name, input string, ok bool) {
	if b.Type == BlockTypeToolCall {
		return b.ToolCallID, b.ToolCallName, b.ToolCallInput, true
	}
	return "", "", "", false
}

// =============================================
// Msg — 消息结构（AgentScope 风格）
// =============================================

// Usage Token 用量统计。
//
// 仅 assistant 消息包含 Usage（记录模型调用的 token 消耗）。
type Usage struct {
	InputTokens  int `json:"input_tokens"`  // 输入 token 数量
	OutputTokens int `json:"output_tokens"` // 输出 token 数量
	TotalTokens  int `json:"total_tokens"`  // 总 token 数量
}

// Msg 对话消息（ 的 Msg 类）。
//
// 每个 Msg 代表一个完整的对话轮次——用户输入、助手回复或系统指令。
// content 以有序的类型化块列表表示，不再使用纯字符串。
//
// 核心字段：
//   - ID: 唯一消息标识符
//   - Name: 发送方名称
//   - Role: 发送方角色（user / assistant / system）
//   - Content: 有序内容块列表（[]ContentBlock）
//   - Metadata: 任意键值元数据
//   - CreatedAt: 创建时间（ISO 8601 格式）
//   - FinishedAt: 完成时间（仅 assistant 消息设置，ISO 8601 格式）
//   - Usage: Token 用量统计（仅 assistant 消息）
type Msg struct {
	ID         string                 `json:"id"`                    // 唯一消息标识符
	Name       string                 `json:"name,omitempty"`        // 发送方名称
	Role       Role                   `json:"role"`                  // 发送方角色
	Content    []ContentBlock         `json:"content"`               // 有序内容块列表
	Metadata   map[string]interface{} `json:"metadata,omitempty"`    // 任意键值元数据
	Marks      map[MsgMark]bool       `json:"marks,omitempty"`       // 消息标记（ _MemoryMark）
	CreatedAt  string                 `json:"created_at"`            // 创建时间（ISO 8601）
	FinishedAt *string                `json:"finished_at,omitempty"` // 完成时间（ISO 8601）
	Usage      *Usage                 `json:"usage,omitempty"`       // Token 用量统计
}

// IsCompressed 检查消息是否已被标记为压缩。
//
//	_MemoryMark.COMPRESSED：
//
// 在构建模型输入时，已压缩的消息会被排除，
// 只保留摘要 + 未压缩的近期消息。
func (m *Msg) IsCompressed() bool {
	return m.Marks[MsgMarkCompressed]
}

// SetMark 设置消息标记。
func (m *Msg) SetMark(mark MsgMark) {
	if m.Marks == nil {
		m.Marks = make(map[MsgMark]bool)
	}
	m.Marks[mark] = true
}

// HasMark 检查消息是否携带指定标记。
func (m *Msg) HasMark(mark MsgMark) bool {
	return m.Marks[mark]
}

// =============================================
// 内部辅助
// =============================================

// =============================================
// 消息标准化（深拷贝， 的 normalize_messages）
// =============================================

// Clone 返回 Msg 的深拷贝。
//
// 在每次 LLM 调用前使用，确保存储的历史消息不被意外修改。
//
//	_clone_msg 模式：deepcopy → normalize → format。
//
// 返回：
//   - *Msg: 消息的完整深拷贝
func (m *Msg) Clone() *Msg {
	clone := &Msg{
		ID:        m.ID,
		Name:      m.Name,
		Role:      m.Role,
		Content:   cloneContentBlocks(m.Content),
		CreatedAt: m.CreatedAt,
	}

	if m.FinishedAt != nil {
		fa := *m.FinishedAt
		clone.FinishedAt = &fa
	}

	if m.Usage != nil {
		u := *m.Usage
		clone.Usage = &u
	}

	if m.Metadata != nil {
		clone.Metadata = cloneMap(m.Metadata)
	}

	if m.Marks != nil {
		clone.Marks = make(map[MsgMark]bool, len(m.Marks))
		for k, v := range m.Marks {
			clone.Marks[k] = v
		}
	}

	return clone
}

// Clone 返回 ContentBlock 的深拷贝。
func (cb *ContentBlock) Clone() ContentBlock {
	clone := ContentBlock{
		Type:            cb.Type,
		Text:            cb.Text,
		MediaType:       cb.MediaType,
		Thinking:        cb.Thinking,
		ToolCallID:      cb.ToolCallID,
		ToolCallName:    cb.ToolCallName,
		ToolCallInput:   cb.ToolCallInput,
		ToolCallState:   cb.ToolCallState,
		ToolResultState: cb.ToolResultState,
		BlockID:         cb.BlockID,
		HintSource:      cb.HintSource,
	}

	if cb.Source != nil {
		s := *cb.Source
		clone.Source = &s
	}

	if cb.ToolResultOutput != nil {
		clone.ToolResultOutput = cloneContentBlocks(cb.ToolResultOutput)
	}

	if cb.Hint != nil {
		clone.Hint = cloneValue(cb.Hint)
	}

	return clone
}

// cloneContentBlocks 深拷贝内容块列表。
func cloneContentBlocks(blocks []ContentBlock) []ContentBlock {
	if blocks == nil {
		return nil
	}
	result := make([]ContentBlock, len(blocks))
	for i, b := range blocks {
		result[i] = b.Clone()
	}
	return result
}

// cloneMap 深拷贝 map[string]interface{}。
func cloneMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = cloneValue(v)
	}
	return result
}

// cloneValue 深拷贝单个值。
func cloneValue(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return val
	case int, int64, float64, bool:
		return val
	case map[string]interface{}:
		return cloneMap(val)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = cloneValue(item)
		}
		return result
	default:
		// 无法深拷贝的类型直接返回原值
		return val
	}
}

// CloneMessages 批量深拷贝消息列表。
//
//	normalize_messages_for_model_request 模式：
//
// 在每次 LLM 调用前深拷贝消息，避免修改存储历史。
//
// 参数：
//   - messages: 原始消息列表
//
// 返回：
//   - []Msg: 消息列表的完整深拷贝
func CloneMessages(messages []Msg) []Msg {
	result := make([]Msg, len(messages))
	for i, m := range messages {
		c := m.Clone()
		result[i] = *c
	}
	return result
}

// nowISO 返回当前 UTC 时间的 ISO 8601 格式字符串。
func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// generateID 生成带前缀的唯一 ID。
//
// 格式：<prefix>_<YYYYMMDD_HHMMSS>
//
// 参数：
//   - prefix: ID 前缀（如 "msg", "evt", "reply"）
//
// 返回：
//   - string: 唯一 ID
func generateID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, time.Now().UTC().Format("20060102_150405"))
}

// =============================================
// Msg 工厂函数（ 的 UserMsg/AssistantMsg/SystemMsg）
// =============================================

// UserMsg 创建用户消息。
//
// 仅允许 TextBlock 和 DataBlock 作为内容块类型。
// 自动生成唯一 ID 和创建时间。
//
// 参数：
//   - name: 发送方名称
//   - content: 消息内容，支持：
//   - string → 自动包装为 [TextBlock]
//   - []ContentBlock → 直接使用
//   - nil → 空内容
//
// 返回：
//   - Msg: 用户消息
//
// 示例：
//
//	// 简单文本消息
//	msg := agent.UserMsg("user", "Hello!")
//
//	// 多模态消息
//	msg := agent.UserMsg("user", []agent.ContentBlock{
//	    agent.NewTextBlock("描述这张图片："),
//	    agent.NewDataBlockURL("https://example.com/img.png", "image/png"),
//	})
func UserMsg(name string, content interface{}) Msg {
	blocks := contentToBlocks(content, RoleUser)
	return Msg{
		ID:        generateID("msg"),
		Name:      name,
		Role:      RoleUser,
		Content:   blocks,
		CreatedAt: nowISO(),
	}
}

// AssistantMsg 创建助手消息。
//
// 允许所有内容块类型（TextBlock, DataBlock, ThinkingBlock,
// ToolCallBlock, ToolResultBlock, HintBlock）。
// 自动生成唯一 ID 和创建时间。
//
// 参数：
//   - name: 发送方名称
//   - content: 消息内容（string 或 []ContentBlock）
//
// 返回：
//   - Msg: 助手消息
//
// 示例：
//
//	// 包含工具调用的助手消息
//	msg := agent.AssistantMsg("assistant", []agent.ContentBlock{
//	    agent.NewTextBlock("Let me look that up."),
//	    agent.NewToolCallBlock("call_1", "search", `{"q": "golang"}`),
//	    agent.NewToolResultTextBlock("call_1", "Results..."),
//	})
func AssistantMsg(name string, content interface{}) Msg {
	blocks := contentToBlocks(content, RoleAssistant)
	return Msg{
		ID:        generateID("msg"),
		Name:      name,
		Role:      RoleAssistant,
		Content:   blocks,
		CreatedAt: nowISO(),
	}
}

// SystemMsg 创建系统消息。
//
// 仅允许 TextBlock 作为内容块类型。
// 自动生成唯一 ID 和创建时间。
//
// 参数：
//   - name: 发送方名称
//   - content: 消息内容（string 或 []ContentBlock，仅 TextBlock 有效）
//
// 返回：
//   - Msg: 系统消息
func SystemMsg(name string, content interface{}) Msg {
	blocks := contentToBlocks(content, RoleSystem)
	return Msg{
		ID:        generateID("msg"),
		Name:      name,
		Role:      RoleSystem,
		Content:   blocks,
		CreatedAt: nowISO(),
	}
}

// contentToBlocks 将 content 参数转换为 ContentBlock 列表。
//
// 支持：
//   - string → []ContentBlock{NewTextBlock(c)}
//   - []ContentBlock → 直接返回
//   - nil → 空列表
//
// 参数：
//   - content: 消息内容
//   - role: 消息角色（用于类型验证）
//
// 返回：
//   - []ContentBlock: 内容块列表
func contentToBlocks(content interface{}, role Role) []ContentBlock {
	switch c := content.(type) {
	case string:
		if c == "" {
			return []ContentBlock{}
		}
		return []ContentBlock{NewTextBlock(c)}
	case []ContentBlock:
		return c
	case nil:
		return []ContentBlock{}
	default:
		return []ContentBlock{NewTextBlock(fmt.Sprintf("%v", c))}
	}
}

// =============================================
// Msg 辅助方法（ 的 Msg 方法）
// =============================================

// GetTextContent 获取所有 TextBlock 的拼接文本。
//
//	的 msg.get_text_content() 方法。
//
// 将所有 TextBlock 的文本用换行符拼接。
//
// 返回：
//   - string: 拼接后的文本，无 TextBlock 时返回 ""
//
// 示例：
//
//	text := msg.GetTextContent()
func (m *Msg) GetTextContent() string {
	var texts []string
	for _, block := range m.Content {
		if block.Type == BlockTypeText {
			texts = append(texts, block.Text)
		}
	}
	if len(texts) == 0 {
		return ""
	}
	result := texts[0]
	for i := 1; i < len(texts); i++ {
		result += "\n" + texts[i]
	}
	return result
}

// GetContentBlocks 按类型过滤内容块。
//
//	的 msg.get_content_blocks() 方法。
//
// 参数：
//   - blockType: 内容块类型
//
// 返回：
//   - []ContentBlock: 匹配类型的内容块列表
//
// 示例：
//
//	toolCalls := msg.GetContentBlocks(agent.BlockTypeToolCall)
func (m *Msg) GetContentBlocks(blockType BlockType) []ContentBlock {
	var result []ContentBlock
	for _, block := range m.Content {
		if block.Type == blockType {
			result = append(result, block)
		}
	}
	return result
}

// HasContentBlocks 检查是否包含指定类型的内容块。
//
//	的 msg.has_content_blocks() 方法。
//
// 参数：
//   - blockType: 内容块类型
//
// 返回：
//   - bool: 是否包含
func (m *Msg) HasContentBlocks(blockType BlockType) bool {
	for _, block := range m.Content {
		if block.Type == blockType {
			return true
		}
	}
	return false
}

// GetToolCallBlocks 获取所有工具调用块。
//
// 返回：
//   - []ContentBlock: ToolCallBlock 类型的内容块列表
func (m *Msg) GetToolCallBlocks() []ContentBlock {
	return m.GetContentBlocks(BlockTypeToolCall)
}

// GetToolResultBlocks 获取所有工具结果块。
//
// 返回：
//   - []ContentBlock: ToolResultBlock 类型的内容块列表
func (m *Msg) GetToolResultBlocks() []ContentBlock {
	return m.GetContentBlocks(BlockTypeToolResult)
}

// HasToolCalls 是否包含工具调用。
//
// 返回：
//   - bool: 是否包含 ToolCallBlock
func (m *Msg) HasToolCalls() bool {
	return m.HasContentBlocks(BlockTypeToolCall)
}

// HasToolResults 是否包含工具结果。
//
// 返回：
//   - bool: 是否包含 ToolResultBlock
func (m *Msg) HasToolResults() bool {
	return m.HasContentBlocks(BlockTypeToolResult)
}

// SetFinished 设置消息完成时间为当前时间。
//
// 通常只在 assistant 消息上调用。
func (m *Msg) SetFinished() {
	now := nowISO()
	m.FinishedAt = &now
}

// =============================================
// 向后兼容的便捷函数
// =============================================

// NewUserMsg 创建用户消息（向后兼容，简单字符串）。
//
// 等同于 UserMsg("user", content)。
//
// 参数：
//   - content: 文本内容
//
// 返回：
//   - Msg: 用户消息
func NewUserMsg(content string) Msg {
	return UserMsg("user", content)
}

// NewAssistantMsg 创建助手消息（向后兼容，简单字符串）。
//
// 等同于 AssistantMsg("assistant", content)。
//
// 参数：
//   - content: 文本内容
//
// 返回：
//   - Msg: 助手消息
func NewAssistantMsg(content string) Msg {
	return AssistantMsg("assistant", content)
}

// NewSystemMsg 创建系统消息（向后兼容，简单字符串）。
//
// 等同于 SystemMsg("system", content)。
//
// 参数：
//   - content: 文本内容
//
// 返回：
//   - Msg: 系统消息
func NewSystemMsg(content string) Msg {
	return SystemMsg("system", content)
}

// NewToolResultMsg 创建工具结果消息（向后兼容，兼容 OpenAI API 的 role=tool 格式）。
//
// 根据 OpenAI API 要求，工具结果消息的 role 为 "tool"，
// 且需要 tool_call_id 字段关联到对应的工具调用。
//
// 参数：
//   - toolCallID: 对应的工具调用 ID
//   - content: 工具执行结果文本
//
// 返回：
//   - Msg: role=tool 的消息
func NewToolResultMsg(toolCallID, content string) Msg {
	return Msg{
		ID:        generateID("msg"),
		Role:      "tool",
		Content:   []ContentBlock{NewTextBlock(content)},
		Name:      toolCallID,
		CreatedAt: nowISO(),
	}
}

// =============================================
// JSON 序列化增强
// =============================================

// ToMap 将消息转换为 OpenAI API 兼容的 map。
//
// 处理：
//   - role + content（文本）
//   - tool_calls（OpenAI 格式）
//   - tool_call_id（role=tool 时）
//
// 返回：
//   - map[string]interface{}: OpenAI API 兼容格式
func (m *Msg) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"role":    string(m.Role),
		"content": m.GetTextContent(),
	}
	if m.Name != "" {
		result["name"] = m.Name
	}
	if m.HasToolCalls() {
		toolCalls := make([]map[string]interface{}, 0)
		for _, block := range m.GetToolCallBlocks() {
			tc := map[string]interface{}{
				"id":   block.ToolCallID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      block.ToolCallName,
					"arguments": block.ToolCallInput,
				},
			}
			toolCalls = append(toolCalls, tc)
		}
		result["tool_calls"] = toolCalls
	}
	if m.Role == "tool" {
		result["tool_call_id"] = m.Name
	}
	return result
}

// MarshalJSON 自定义 JSON 序列化。
//
// 使用标准结构体序列化，确保 JSON 输出格式正确。
func (m *Msg) MarshalJSON() ([]byte, error) {
	type MsgAlias Msg
	alias := MsgAlias(*m)
	return json.Marshal(alias)
}
