package agent

// =============================================
// 上下文管理（ 的 Context 管理）
//
// 通过三种机制让 agent 持续运行：
//  1. 上下文压缩 — 当 token 用量逼近模型上限时，把较早消息汇总为摘要
//  2. 工具结果截断 — 在过大的工具输出进入上下文之前先截断
//  3. 上下文 Offload — 把已移除上下文的内容持久化到外部存储
//
// 每次模型调用前，agent 把三层内容拼成单次 API 输入：
//  1. System Prompt（含 skill 指令 + middleware 转换）
//  2. Summary（已压缩历史，若存在）
//  3. Context（最近未压缩消息）
// =============================================

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// =============================================
// ContextConfig — 上下文配置
// =============================================

// ContextConfig 上下文配置（ 的 ContextConfig）。
//
// 字段：
//   - TriggerRatio: 当 token 用量超过该比例 × 模型上下文长度时触发压缩（默认 0.8，上限 0.9）
//   - ReserveRatio: 压缩后作为最近消息保留的上下文 token 比例（默认 0.1）
//   - ToolResultLimit: 单条工具结果的最大 token 数，超出则截断（默认 100000，即不限制）
//   - ToolResultExemptTools: 豁免截断的工具名列表（如 ["browser_use", "read_file"]）
//   - ToolResultExemptExts: 豁免截断的文件扩展名列表（如 [".png", ".jpg", ".pdf"]）
//   - CompressionPrompt: 引导模型生成摘要的 prompt
//   - SummaryTemplate: 把摘要拼回上下文时的模板（支持 {{field}} 占位符）
//   - SummarySchema: 约束模型结构化摘要输出的 JSON Schema
type ContextConfig struct {
	TriggerRatio      float64                `json:"trigger_ratio"`      // 触发压缩比例（默认 0.8）
	ReserveRatio      float64                `json:"reserve_ratio"`      // 保留比例（默认 0.1）
	ToolResultLimit   int                    `json:"tool_result_limit"`  // 工具结果截断阈值（默认 100000）
	ToolResultExemptTools []string            `json:"tool_result_exempt_tools"`  // 豁免截断的工具名
	ToolResultExemptExts  []string            `json:"tool_result_exempt_exts"`   // 豁免截断的文件扩展名
	CompressionPrompt string                 `json:"compression_prompt"` // 压缩提示词
	SummaryTemplate   string                 `json:"summary_template"`   // 摘要模板
	SummarySchema     map[string]interface{} `json:"summary_schema"`     // 摘要 JSON Schema
}

// DefaultContextConfig 返回默认上下文配置。
//
// 默认值：
//   - TriggerRatio: 0.8（使用 80% 上下文时触发压缩）
//   - ReserveRatio: 0.1（压缩后保留最近 10% 的内容）
//   - ToolResultLimit: 100000（工具结果不截断，足够容纳完整的 SKILL.md）
//   - ToolResultExemptTools: nil（不过滤任何工具）
//   - ToolResultExemptExts: nil（不过滤任何扩展名）
func DefaultContextConfig() *ContextConfig {
	return &ContextConfig{
		TriggerRatio:    0.8,
		ReserveRatio:    0.1,
		ToolResultLimit: 100000,
		CompressionPrompt: `请对以下对话历史进行摘要，保留关键信息。输出 JSON 格式：
{"task_overview":"...","current_state":"...","important_discoveries":[...],"next_steps":[...],"context_to_preserve":"..."}`,
		SummaryTemplate: `<summary>
任务概览: {{task_overview}}
当前状态: {{current_state}}
重要发现: {{important_discoveries}}
下一步: {{next_steps}}
保留上下文: {{context_to_preserve}}
</summary>`,
		SummarySchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_overview":         map[string]interface{}{"type": "string"},
				"current_state":         map[string]interface{}{"type": "string"},
				"important_discoveries": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"next_steps":            map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"context_to_preserve":   map[string]interface{}{"type": "string"},
			},
			"required": []string{"task_overview", "current_state", "important_discoveries", "next_steps", "context_to_preserve"},
		},
	}
}

// =============================================
// Summary — 结构化摘要
// =============================================

// Summary 结构化摘要（压缩后的历史， 的 Summary）。
//
// 模型基于较早消息生成，包含五个字段：
//   - TaskOverview: 任务整体描述
//   - CurrentState: 当前进展状态
//   - ImportantDiscoveries: 重要发现列表
//   - NextSteps: 下一步行动
//   - ContextToPreserve: 需要保留的上下文细节
type Summary struct {
	TaskOverview         string   `json:"task_overview"`         // 任务整体描述
	CurrentState         string   `json:"current_state"`         // 当前进展状态
	ImportantDiscoveries []string `json:"important_discoveries"` // 重要发现列表
	NextSteps            []string `json:"next_steps"`            // 下一步行动
	ContextToPreserve    string   `json:"context_to_preserve"`   // 需保留的上下文细节
	CreatedAt            string   `json:"created_at"`            // 创建时间
	TokenCount           int      `json:"token_count"`           // token 数量
}

// Format 将摘要格式化为文本，用于注入上下文。
//
// 输出格式：
//
//	<summary>
//	任务概览: ...
//	当前状态: ...
//	重要发现: ...
//	下一步: ...
//	保留上下文: ...
//	</summary>
func (s *Summary) Format() string {
	var sb strings.Builder
	sb.WriteString("<summary>\n")
	sb.WriteString(fmt.Sprintf("任务概览: %s\n", s.TaskOverview))
	sb.WriteString(fmt.Sprintf("当前状态: %s\n", s.CurrentState))
	sb.WriteString("重要发现: ")
	for i, d := range s.ImportantDiscoveries {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(d)
	}
	sb.WriteString("\n下一步: ")
	for i, step := range s.NextSteps {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(step)
	}
	sb.WriteString(fmt.Sprintf("\n保留上下文: %s\n", s.ContextToPreserve))
	sb.WriteString("</summary>")
	return sb.String()
}

// ToJSON 将摘要序列化为 JSON 字符串。
func (s *Summary) ToJSON() string {
	bytes, _ := json.Marshal(s)
	return string(bytes)
}

// ParseSummary 从 JSON 字符串解析摘要。
//
// 参数：
//   - jsonStr: JSON 格式的摘要
//
// 返回：
//   - *Summary: 解析后的摘要
//   - error: 解析错误
func ParseSummary(jsonStr string) (*Summary, error) {
	var s Summary
	if err := json.Unmarshal([]byte(jsonStr), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// =============================================
// TokenCounter — Token 计算接口
// =============================================

// TokenCounter Token 计算器接口。
//
// 上下文压缩的前提条件。不同模型有不同的 tokenizer，
// 通过实现此接口可适配不同模型。
type TokenCounter interface {
	// CountTokens 计算文本的 token 数量。
	//
	// 参数：
	//   - text: 文本内容
	//
	// 返回：
	//   - int: token 数量
	CountTokens(text string) int

	// CountMessageTokens 计算单条消息的 token 数量（含 role 和格式开销）。
	//
	// 参数：
	//   - msg: 消息
	//
	// 返回：
	//   - int: token 数量
	CountMessageTokens(msg Msg) int

	// CountMessagesTokens 计算消息列表的总 token 数量。
	//
	// 参数：
	//   - messages: 消息列表
	//
	// 返回：
	//   - int: 总 token 数量
	CountMessagesTokens(messages []Msg) int
}

// SimpleTokenCounter 简单 Token 计算器（基于字符数估算）。
//
// 英文约 4 字符/token，中文约 1.5 字符/token。
// 这里使用保守估计：3 字符/token。
// 实际生产环境应使用 tiktoken 或模型 API 返回的 usage。
type SimpleTokenCounter struct {
	charsPerToken float64 // 每 token 的字符数（默认 3.0）
}

// NewSimpleTokenCounter 创建简单 Token 计算器。
//
// 返回：
//   - *SimpleTokenCounter: 计算器指针
func NewSimpleTokenCounter() *SimpleTokenCounter {
	return &SimpleTokenCounter{charsPerToken: 3.0}
}

// CountTokens 计算文本的估算 token 数。
//
// 公式：len(text) / charsPerToken + 1
//
// 参数：
//   - text: 文本
//
// 返回：
//   - int: 估算 token 数
func (c *SimpleTokenCounter) CountTokens(text string) int {
	if text == "" {
		return 0
	}
	return int(float64(len(text))/c.charsPerToken) + 1
}

// CountMessageTokens 计算消息的估算 token 数（含各块类型的格式开销）。
//
// 参数：
//   - msg: 消息
//
// 返回：
//   - int: 估算 token 数
func (c *SimpleTokenCounter) CountMessageTokens(msg Msg) int {
	total := c.CountTokens(msg.GetTextContent())
	total += 1 // role token
	for _, block := range msg.Content {
		switch block.Type {
		case BlockTypeData:
			total += 10
		case BlockTypeToolCall:
			total += c.CountTokens(block.ToolCallName) + c.CountTokens(block.ToolCallInput) + 5
		case BlockTypeToolResult:
			for _, output := range block.ToolResultOutput {
				total += c.CountTokens(output.Text) + 2
			}
		case BlockTypeThinking:
			total += c.CountTokens(block.Thinking)
		case BlockTypeHint:
			if hintStr, ok := block.Hint.(string); ok {
				total += c.CountTokens(hintStr)
			}
		}
	}
	total += 4 // 消息格式开销
	return total
}

// CountMessagesTokens 计算消息列表的总估算 token 数。
//
// 参数：
//   - messages: 消息列表
//
// 返回：
//   - int: 总估算 token 数
func (c *SimpleTokenCounter) CountMessagesTokens(messages []Msg) int {
	total := 0
	for _, msg := range messages {
		total += c.CountMessageTokens(msg)
	}
	return total
}

// =============================================
// ContextManager — 上下文管理器
// =============================================

// ContextManager 上下文管理器，负责压缩、截断和卸载。
type ContextManager struct {
	config    *ContextConfig // 上下文配置
	counter   TokenCounter   // Token 计算器
	maxTokens int            // 模型最大上下文长度
	summary   *Summary       // 当前摘要（压缩后的历史）
	offloader Offloader      // 卸载器
}

// NewContextManager 创建上下文管理器。
//
// 参数：
//   - config: 上下文配置（nil 则使用默认值）
//   - maxTokens: 模型最大上下文长度
//   - offloader: 卸载器（如果提供了 SessionID，会尝试加载已持久化的摘要）
//
// 返回：
//   - *ContextManager: 上下文管理器
func NewContextManager(config *ContextConfig, maxTokens int, offloader Offloader, sessionID string) *ContextManager {
	if config == nil {
		config = DefaultContextConfig()
	}
	cm := &ContextManager{
		config: config, counter: NewSimpleTokenCounter(),
		maxTokens: maxTokens, offloader: offloader,
	}

	// 尝试从 offloader 恢复已持久化的摘要
	if sessionID != "" && offloader != nil {
		if lw, ok := offloader.(*LocalWorkspace); ok {
			if summary, err := lw.LoadSummary(sessionID); err == nil && summary != nil {
				cm.summary = summary
			}
		}
	}

	return cm
}

// SetTokenCounter 设置自定义 Token 计算器。
//
// 参数：
//   - counter: Token 计算器实现
func (m *ContextManager) SetTokenCounter(counter TokenCounter) { m.counter = counter }

// CheckAndCompress 检查并压缩上下文。
//
// 流程：
//  1. 计算总 token 数（system prompt + summary + context）
//  2. 与 trigger_ratio × maxTokens 比较
//  3. 超过阈值：切分消息 → offload 待压缩部分 → 标记需要压缩
//  4. 未超过：返回原消息
//
// 参数：
//   - ctx: 上下文
//   - sessionID: 会话 ID
//   - messages: 消息列表
//   - systemPrompt: 系统提示词
//
// 返回：
//   - []Msg: 保留的消息（reserve 部分）
//   - bool: 是否进行了压缩
//   - error: 错误
func (m *ContextManager) CheckAndCompress(ctx context.Context, sessionID string, messages []Msg, systemPrompt string) ([]Msg, bool, error) {
	totalTokens := m.counter.CountTokens(systemPrompt)
	if m.summary != nil {
		totalTokens += m.summary.TokenCount
	}
	totalTokens += m.counter.CountMessagesTokens(messages)

	triggerThreshold := int(float64(m.maxTokens) * m.config.TriggerRatio)
	if totalTokens < triggerThreshold {
		return messages, false, nil
	}

	reserveTokens := int(float64(m.maxTokens) * m.config.ReserveRatio)
	compressMsgs, reserveMsgs := m.splitMessages(messages, reserveTokens)

	if len(compressMsgs) == 0 {
		return messages, false, nil
	}

	if m.offloader != nil {
		_, err := m.offloader.OffloadContext(ctx, sessionID, compressMsgs)
		if err != nil {
			fmt.Printf("Offload context failed: %v\n", err)
		}
	}

	return reserveMsgs, true, nil
}

// splitMessages 切分消息为待压缩和保留两部分。
//
// 从后向前累加 token 数，直到达到 reserveTokens 上限。
// 保留部分包含最近的 message。
//
// 参数：
//   - messages: 完整消息列表
//   - reserveTokens: 保留的 token 上限
//
// 返回：
//   - []Msg: 待压缩的消息
//   - []Msg: 保留的消息
func (m *ContextManager) splitMessages(messages []Msg, reserveTokens int) ([]Msg, []Msg) {
	if len(messages) == 0 {
		return nil, nil
	}
	reserveMsgs := []Msg{}
	reserveCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := m.counter.CountMessageTokens(messages[i])
		if reserveCount+msgTokens > reserveTokens {
			break
		}
		reserveMsgs = append([]Msg{messages[i]}, reserveMsgs...)
		reserveCount += msgTokens
	}
	reserveMsgs = m.adjustSplitForToolPairs(messages, reserveMsgs)
	compressMsgs := messages[:len(messages)-len(reserveMsgs)]
	return compressMsgs, reserveMsgs
}

// adjustSplitForToolPairs 调整切分点，确保工具调用/结果对不被拆开。
//
// 参数：
//   - allMsgs: 完整消息列表
//   - reserveMsgs: 初步保留的消息
//
// 返回：
//   - []Msg: 调整后的保留消息
func (m *ContextManager) adjustSplitForToolPairs(allMsgs, reserveMsgs []Msg) []Msg {
	if len(reserveMsgs) == 0 {
		return reserveMsgs
	}
	firstReserve := reserveMsgs[0]
	toolCallID := ""
	if firstReserve.Role == "tool" {
		toolCallID = firstReserve.Name
	} else {
		for _, block := range firstReserve.GetToolResultBlocks() {
			toolCallID = block.ToolCallID
			break
		}
	}
	if toolCallID == "" {
		return reserveMsgs
	}
	compressCount := len(allMsgs) - len(reserveMsgs)
	for i := compressCount - 1; i >= 0; i-- {
		for _, block := range allMsgs[i].GetToolCallBlocks() {
			if block.ToolCallID == toolCallID {
				return append([]Msg{allMsgs[i]}, reserveMsgs...)
			}
		}
	}
	return reserveMsgs
}

// SetSummary 设置当前摘要。
//
// 参数：
//   - summary: 结构化摘要
func (m *ContextManager) SetSummary(summary *Summary) { m.summary = summary }

// GetSummary 获取当前摘要。
//
// 返回：
//   - *Summary: 摘要，nil 表示尚无摘要
func (m *ContextManager) GetSummary() *Summary { return m.summary }

// GetSummaryText 获取摘要的格式化文本。
//
// 返回：
//   - string: 格式化的摘要文本，无摘要时返回 ""
func (m *ContextManager) GetSummaryText() string {
	if m.summary == nil {
		return ""
	}
	return m.summary.Format()
}

// =============================================
// 工具结果截断
// =============================================

// TruncateToolResult 截断过大的工具结果。
//
// 当工具结果的 token 数超过 ToolResultLimit 时，
// 截断并追加标记。如果配置了 Offloader，截断部分持久化。
//
// 豁免规则：
//   - 如果 toolName 在 ToolResultExemptTools 列表中，不截断
//   - 如果结果文本中包含 ToolResultExemptExts 中的扩展名文件链接，不截断
//   - ToolResultLimit <= 0 时不截断（相当于无限）
//
// 截断标记格式：
//
//	<<<TRUNCATED>>>
//	<system-reminder>The remaining content has been omitted for limited context.</system-reminder>
//
// 参数：
//   - ctx: 上下文
//   - sessionID: 会话 ID
//   - toolName: 工具名称（用于豁免判断）
//   - result: 原始工具结果块
//   - exemptTools: 豁免截断的工具名列表
//   - exemptExts: 豁免截断的文件扩展名列表
//
// 返回：
//   - ContentBlock: 保留的结果块（含截断标记）
//   - string: offload 引用（无 offloader 或未截断时为空）
//   - bool: 是否进行了截断
func (m *ContextManager) TruncateToolResult(ctx context.Context, sessionID string, toolName string, result ContentBlock, exemptTools, exemptExts []string) (ContentBlock, string, bool) {
	// 1. 阈值 <= 0 → 不截断
	if m.config.ToolResultLimit <= 0 {
		return result, "", false
	}

	// 2. 豁免工具检查
	for _, name := range exemptTools {
		if name == toolName {
			return result, "", false
		}
	}

	// 3. 豁免扩展名检查 — 结果中包含豁免扩展名的文件链接，不截断
	for _, ext := range exemptExts {
		if strings.Contains(result.ToolResultOutput[0].Text, ext) {
			return result, "", false
		}
	}

	resultTokens := m.countToolResultTokens(result)
	if resultTokens <= m.config.ToolResultLimit {
		return result, "", false
	}

	var text string
	for _, block := range result.ToolResultOutput {
		if block.Type == BlockTypeText {
			text += block.Text
		}
	}

	charsPerToken := 3.0
	keepChars := int(float64(m.config.ToolResultLimit) * charsPerToken)
	if keepChars >= len(text) {
		return result, "", false
	}

	keepText := text[:keepChars]

	truncatedMarker := "\n<<<TRUNCATED>>>\n<system-reminder>The remaining content has been omitted for limited context.</system-reminder>"
	offloadRef := ""
	if m.offloader != nil {
		ref, err := m.offloader.OffloadToolResult(ctx, sessionID, result)
		if err == nil {
			offloadRef = ref
			truncatedMarker = fmt.Sprintf("\n<<<TRUNCATED>>>\n<system-reminder>The remaining content has been omitted for limited context. You can refer to the file in '%s' for the truncated content if needed.</system-reminder>", ref)
		}
	}

	keepResult := ContentBlock{
		Type: BlockTypeToolResult, ToolCallID: result.ToolCallID,
		ToolResultOutput: []ContentBlock{NewTextBlock(keepText + truncatedMarker)},
		ToolResultState:  result.ToolResultState,
	}
	return keepResult, offloadRef, true
}

// countToolResultTokens 计算工具结果的 token 数（内部使用）。
//
// 参数：
//   - result: 工具结果块
//
// 返回：
//   - int: 估算 token 数
func (m *ContextManager) countToolResultTokens(result ContentBlock) int {
	total := 0
	for _, block := range result.ToolResultOutput {
		if block.Type == BlockTypeText {
			total += m.counter.CountTokens(block.Text)
		}
	}
	return total
}
