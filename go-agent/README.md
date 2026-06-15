# go-agent

Go 语言 AI Agent 独立库， 工程的工程化实践。零外部依赖，直接 `import` 使用。

## 安装

```bash
go get github.com/nllihui6390/go-agent
```

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "github.com/nllihui6390/go-agent"
    "github.com/nllihui6390/go-agent/model"
    "github.com/nllihui6390/go-agent/tool"
)

func main() {
    llm := model.NewOpenAIModel(model.ModelConfig{
        APIKey: "sk-xxx", Model: "gpt-4",
        BaseURL: "https://api.openai.com/v1",
    })
    tools := tool.NewRegistry()
    tools.Register(tool.NewBasicTool("echo", "Echo input", nil,
        func(ctx context.Context, params map[string]any) (string, error) {
            return params["text"].(string), nil
        }))

    ag := agent.NewAgent(*agent.DefaultConfig("assistant", llm, "You are helpful.").
        WithTools(tools).WithMaxIters(10))

    reply, _ := ag.Reply(ctx, agent.UserMsg("user", "Hello!"))
    fmt.Println(reply.GetTextContent())
}
```

---

## 架构总览

```
go-agent/
├── agent.go           # Agent 核心：Reply / ReplyStream / Observe / CompressContext
├── config.go          # Config / ModelConfig / ReActConfig + Builder 模式
├── message.go         # Msg / ContentBlock(6种) / 工厂函数 / 深拷贝 / 消息标记
├── event.go           # 完整事件系统（20+ 种事件类型，start→delta→end 模式）
├── append_event.go    # AppendEvent — 从事件流逐步重建完整消息
├── session.go         # Session / SessionStore / 按标记过滤 / 压缩标记管理
├── context.go         # ContextConfig / Summary / TokenCounter / ContextManager
├── middleware.go       # Middleware 接口 / MiddlewareChain / 8 种内置中间件 / 9 个阶段
├── permission.go      # 完整权限引擎（5 种 Mode + Rules + Built-in Checks）
├── offloader.go       # Offloader 协议 / LocalWorkspace / NoOpOffloader
├── state.go           # AgentState / SessionRecord / StateStorage / InMemory
├── model/
│   ├── model.go       # ChatModel / ToolSetter / Formatter 接口 / 响应类型
│   ├── openai.go      # OpenAI / DeepSeek / Ollama 实现（SSE流式、工具调用）
│   ├── formatter.go   # Formatter 层：OpenAI/Anthropic 格式化器
│   └── rate_limiter.go # RateLimiter：QPM+并发限制
├── tool/
│   ├── tool.go        # Tool 接口 / ToolPermissionProvider / ToolProperties / HandlerTool
│   ├── permission_types.go # PermissionAction/Mode/Context/Rule（共享基础类型）
│   └── registry.go    # 合并/克隆/分组
├── memory/
│   ├── memory.go      # Memory 接口 / MemoryItem
│   └── simple.go      # SimpleMemory 实现
├── skill/
│   ├── skill.go       # Skill 定义 / Registry / 匹配
│   └── registry.go    # Executor / 变量替换
└── examples/          # 使用示例
```

---

## 模块详解

### 1. Agent 核心 (`agent.go`, `config.go`)

Agent 是**无状态**的推理-行动循环引擎，将模型、工具、权限、中间件、上下文管理整合到统一接口。

#### 核心接口

| 方法 | 签名 | 说明 |
|-----|------|------|
| `Reply` | `(ctx, inputs) → (*Msg, error)` | 同步推理-行动循环，返回最终 Msg |
| `ReplyStream` | `(ctx, inputs) → (<-chan interface{}, error)` | 流式版本，逐一产出 Event |
| `Observe` | `(msgs ...Msg)` | 将消息注入上下文，不触发推理 |
| `CompressContext` | `(ctx, config) → error` | 手动/自定义配置触发上下文压缩 |
| `Process` | `(ctx, msg) → (*Msg, error)` | 向后兼容，等同于 Reply |
| `GetSession` | `() → *Session` | 获取当前会话 |
| `ClearSession` | `()` | 清除会话和摘要 |

#### ReAct 推理循环

 的推理-行动循环，每次迭代经过以下步骤：

```
1. PhasePreReasoning → 检查上下文健康、触发自动压缩
2. PhaseReasoning   → 构建模型输入、注入工具定义 → 调用模型
3. 模型返回文本     → 加入 session → 返回最终结果
4. 模型返回工具调用 → PhaseActing → 权限检查 → PhasePostActing
5. 继续循环（最多 MaxIters 次）
```

#### inputs 支持的类型

```go
// 开始新的 reply
ag.Reply(ctx, agent.UserMsg("user", "Hello"))

// 从暂停状态恢复（用户确认结果）
ag.Reply(ctx, confirmEvent)

// 从暂停状态恢复（外部执行结果）
ag.Reply(ctx, externalEvent)

// 多条消息
ag.Reply(ctx, []agent.Msg{msg1, msg2})
```

#### Config 配置

```go
cfg := agent.DefaultConfig("name", model, "system prompt").
    WithTools(tool.NewRegistry()).        // 工具注册表
    WithSkills(skill.NewRegistry()).      // 技能注册表
    WithMemory(memory.NewSimpleMemory()). // 长期记忆
    WithContextConfig(contextCfg).        // 上下文压缩配置
    WithOffloader(offloader).             // 卸载器
    WithPermission(checker).              // 权限检查
    WithMaxIters(10).                     // 最大迭代次数
    WithMiddlewares(mw1, mw2).            // 中间件
    WithStorage(storage, "user1", "agent1") // 状态持久化

ag := agent.NewAgent(*cfg)
```

#### Config 字段一览

| 字段 | 类型 | 默认值 | 说明 |
|-----|------|--------|------|
| `Name` | `string` | 必填 | Agent 标识符 |
| `Model` | `model.ChatModel` | 必填 | 主模型 |
| `SystemPrompt` | `string` | 必填 | 系统提示词 |
| `Tools` | `*tool.Registry` | 空注册表 | 工具集（Agent 通过 ToolSetter 注入到模型） |
| `Skills` | `*skill.Registry` | nil | 技能集 |
| `Memory` | `memory.Memory` | SimpleMemory | 长期记忆 |
| `SessionStore` | `SessionStore` | InMemory | 会话存储 |
| `ContextConfig` | `*ContextConfig` | 默认值 | 压缩阈值 |
| `Offloader` | `Offloader` | NoOp | 压缩内容持久化 |
| `ModelConfig` | `*ModelConfig` | 重试3次/延迟1s | 重试/备用模型 |
| `ReActConfig` | `*ReActConfig` | MaxIters=10 | 循环控制 |
| `Permission` | `PermissionChecker` | Default | 权限检查 |
| `Middlewares` | `[]Middleware` | 空 | 中间件列表 |
| `Storage` | `StateStorage` | nil | 状态持久化 |

---

### 2. 消息系统 (`message.go`)

消息以**有序类型化内容块列表**表示， 的 Msg / ContentBlock 设计。

#### 内容块类型

| 块类型 | 常量 | 说明 | 允许的角色 |
|-------|------|------|-----------|
| TextBlock | `BlockTypeText` | 纯文本 | user / assistant / system |
| DataBlock | `BlockTypeData` | 二进制数据（图片/音频），base64 或 URL | user / assistant |
| ThinkingBlock | `BlockTypeThinking` | 模型推理过程（思维链/推理内容） | assistant |
| ToolCallBlock | `BlockTypeToolCall` | 工具调用，含名称/输入/状态 | assistant |
| ToolResultBlock | `BlockTypeToolResult` | 工具执行结果 | assistant |
| HintBlock | `BlockTypeHint` | 注入的带外提示 | assistant |

#### 消息标记（ _MemoryMark）

```go
// 消息可携带标记，用于压缩管理和上下文过滤
const (
    MsgMarkCompressed MsgMark = "compressed" // 已压缩标记
    MsgMarkHint       MsgMark = "hint"       // 提示标记
)

// 标记相关方法
msg.IsCompressed()       // 检查是否已标记为压缩
msg.SetMark(MsgMarkXxx)  // 设置标记
msg.HasMark(MsgMarkXxx)  // 检查是否携带指定标记
```

#### 消息深拷贝（ normalize_messages）

```go
// 每次 LLM 调用前深拷贝消息，确保 session 原始历史不被意外修改
clone := msg.Clone()           // 深拷贝单条消息（含嵌套 ContentBlock/Metadata/Marks）
clones := CloneMessages(msgs)  // 批量深拷贝
```

#### 消息工厂函数

```go
// AgentScope 风格（name + content）
userMsg := agent.UserMsg("user", "你好！")
assistantMsg := agent.AssistantMsg("assistant", "回复内容")
systemMsg := agent.SystemMsg("system", "你是一个有帮助的助手。")

// 向后兼容（简单字符串）
msg := agent.NewUserMsg("Hello")

// 多模态消息
msg := agent.UserMsg("user", []agent.ContentBlock{
    agent.NewTextBlock("描述这张图片："),
    agent.NewDataBlockURL("https://example.com/img.png", "image/png"),
})

// 助手消息含工具调用
msg := agent.AssistantMsg("assistant", []agent.ContentBlock{
    agent.NewTextBlock("让我查一下。"),
    agent.NewThinkingBlock("用户要求计算，我需要调用 calculator 工具…"),
    agent.NewToolCallBlock("call_1", "search", `{"q": "golang"}`),
    agent.NewToolResultTextBlock("call_1", "搜索结果..."),
})
```

#### 消息辅助方法

```go
msg.GetTextContent()                          // 拼接所有 TextBlock 文本
msg.GetContentBlocks(agent.BlockTypeToolCall) // 按类型过滤
msg.HasContentBlocks(agent.BlockTypeThinking) // 是否含指定类型
msg.HasToolCalls()                            // 是否有工具调用
msg.HasToolResults()                          // 是否有工具结果
msg.GetToolCallBlocks()                       // 获取所有 ToolCallBlock
msg.GetToolResultBlocks()                     // 获取所有 ToolResultBlock
msg.Clone()                                   // 深克隆
msg.SetFinished()                             // 设置完成时间
msg.AppendEvent(event)                        // 从事件重建消息
```

#### ContentBlock 工厂函数

```go
agent.NewTextBlock(text)
agent.NewDataBlockBase64(data, mimeType)
agent.NewDataBlockURL(url, mimeType)
agent.NewThinkingBlock(thinking)
agent.NewToolCallBlock(id, name, inputJSON)
agent.NewToolResultBlock(toolCallID, outputBlocks)
agent.NewToolResultTextBlock(toolCallID, text)
agent.NewHintBlock(blockID, hint, source)
```

---

### 3. 事件系统 (`event.go`, `append_event.go`)

事件是消息的流式对应物，遵循 **start → delta → end** 模式。同一回复中的所有事件共享 `reply_id`。

#### 事件类型一览

| 类别 | 事件类型 | 模式 |
|-----|---------|------|
| **生命周期** | `ReplyStartEvent`, `ReplyEndEvent`, `ExceedMaxItersEvent` | 单次 |
| **文本流式** | `TextBlockStartEvent → TextBlockDeltaEvent → TextBlockEndEvent` | start→delta→end |
| **思考流式** | `ThinkingBlockStartEvent → ThinkingBlockDeltaEvent → ThinkingBlockEndEvent` | start→delta→end |
| **数据流式** | `DataBlockStartEvent → DataBlockDeltaEvent → DataBlockEndEvent` | start→delta→end |
| **工具调用** | `ToolCallStartEvent → ToolCallDeltaEvent → ToolCallEndEvent` | start→delta→end |
| **工具结果** | `ToolResultStartEvent → ToolResultTextDeltaEvent/DataDeltaEvent → ToolResultEndEvent` | start→delta→end |
| **模型调用** | `ModelCallStartEvent → ModelCallEndEvent` | 配对 |
| **人工介入** | `RequireUserConfirmEvent`, `RequireExternalExecutionEvent`, `UserConfirmResultEvent`, `ExternalExecutionResultEvent` | 单次 |
| **一次性** | `HintBlockEvent`, `CustomEvent` | 单次 |

#### 流式使用

```go
eventStream, _ := ag.ReplyStream(ctx, agent.UserMsg("user", "Hello"))

for event := range eventStream {
    switch e := event.(type) {
    case agent.ReplyStartEvent:
        fmt.Println("回复开始:", e.ReplyID)
    case agent.ThinkingBlockStartEvent:
        fmt.Print("[思考] ")
    case agent.ThinkingBlockDeltaEvent:
        fmt.Print(e.Delta) // 实时打印推理过程
    case agent.TextBlockDeltaEvent:
        fmt.Print(e.Delta) // 实时打印
    case agent.ToolCallStartEvent:
        fmt.Printf("\n[调用工具: %s]", e.ToolCallName)
    case agent.ToolResultEndEvent:
        fmt.Printf(" [状态: %s]", e.State)
    case agent.ReplyEndEvent:
        fmt.Println("\n[完成]")
    }
}
```

#### 从事件流重建消息

```go
msg := &agent.Msg{ID: replyID, Role: agent.RoleAssistant, Content: []agent.ContentBlock{}}
for event := range eventStream {
    msg.AppendEvent(event) // 增量重建
}
// msg 现在包含完整的回复内容
```

---

### 4. 上下文管理 (`context.go`)

 的三层模型输入结构：**System Prompt + Summary + Context**。

#### ContextConfig

```go
cfg := &agent.ContextConfig{
    TriggerRatio:      0.8,   // token 用量超过 80% 时触发压缩
    ReserveRatio:      0.1,   // 压缩后保留最近 10% 的内容
    ToolResultLimit:   3000,  // 单条工具结果最大 token
    CompressionPrompt: "...", // 压缩提示词
    SummarySchema:     {...}, // 结构化摘要 JSON Schema
}
```

#### 压缩流程（ mark_messages_compressed）

```
1. 计算 token 数（SystemPrompt + Summary + Context）
2. 超过 trigger_ratio × context_size → 触发压缩
3. 切分消息：较旧 → 待压缩，recent → 保留
4. Offload 待压缩的消息到外部存储
5. 标记被压缩消息为 MsgMarkCompressed
6. 生成结构化摘要替换被压缩的消息
7. buildModelMessages 时排除 MsgMarkCompressed 消息，注入摘要
```

#### TokenCounter

```go
type TokenCounter interface {
    CountTokens(text string) int
    CountMessageTokens(msg Msg) int
    CountMessagesTokens(messages []Msg) int
}

// 内置简单实现（基于字符数估算，3字符/token）
counter := agent.NewSimpleTokenCounter()
```

---

### 5. 中间件系统 (`middleware.go`)

 Context Manager 钩子系统，在 9 个关键生命周期阶段提供钩子。

#### 中间件接口

```go
type Middleware interface {
    Name() string
    Phase() MiddlewarePhase
    Execute(ctx context.Context, agent *Agent, data interface{}) error
}
```

#### 中间件阶段

| 阶段 | 常量 | data 类型 | 触发时机 | 参考 |
|-----|------|----------|---------|------|
| reply 开始前 | `PhaseReply` | `interface{}` (inputs) | Reply 入口 | AgentScope |
| 最终回复前 | `PhasePreReply` | `Msg` | 用户消息添加后 |  pre_reply |
| 最终回复后 | `PhasePostReply` | `*Msg` (reply) | 回复完成后 |  post_reply |
| 推理前 | `PhaseReasoning` | `[]Msg` (messages) | 每次模型调用前 | AgentScope |
| 每次推理前 | `PhasePreReasoning` | `[]Msg` (messages) | 检查上下文健康 |  pre_reasoning |
| 行动前 | `PhaseActing` | `ToolCall` | 每个工具调用前 | AgentScope |
| 工具执行后 | `PhasePostActing` | `ContentBlock` | 工具执行完成后 |  post_acting |
| 模型调用 | `PhaseModelCall` | `error` (调用错误时) | 模型调用后 | AgentScope |
| System Prompt | `PhaseSystemPrompt` | `*string` (prompt指针) | 组装 system prompt | AgentScope |

#### 内置中间件（8 种）

```go
// 1. System Prompt 中间件 — 注入动态上下文
agent.NewSystemPromptMiddleware("workdir", func(ctx, ag, prompt *string) error {
    *prompt += "\n当前工作目录: /home/user"
    return nil
})

// 2. 日志中间件 — 记录每次 reply
agent.NewLoggingMiddleware("logger", func(phase, name string, data interface{}) {
    log.Printf("[%s] %s: %v", phase, name, data)
})

// 3. 限流中间件 — 限制调用频率
agent.NewRateLimitMiddleware("ratelimit", 60) // 每分钟最多60次

// 4. 错误处理中间件 — 模型调用错误处理
agent.NewErrorHandlingMiddleware("errhandler", func(ctx, ag, err error) error {
    log.Printf("Model error: %v", err)
    return nil // 返回 nil 忽略错误继续
})

// 5. PreReply 中间件 — 回复前注入记忆检索结果
agent.NewPreReplyMiddleware("memory_inject", func(ctx, ag, data interface{}) error {
    // 在最终回复前检索记忆并注入上下文
    return nil
})

// 6. PostReply 中间件 — 回复后后台任务
agent.NewPostReplyMiddleware("summarize", func(ctx, ag, data interface{}) error {
    // 在最终回复后调度后台摘要任务
    return nil
})

// 7. PreReasoning 中间件 — 推理前上下文检查
agent.NewPreReasoningMiddleware("context_check", func(ctx, ag, data interface{}) error {
    // 每次模型调用前检查上下文大小
    return nil
})

// 8. PostActing 中间件 — 工具执行后审计
agent.NewPostActingMiddleware("tool_audit", func(ctx, ag, data interface{}) error {
    // 工具执行后记录审计日志
    return nil
})
```

---

### 6. 权限系统 (`permission.go`, `tool/permission_types.go`)

完整学习 AgentScope 的权限引擎设计：**Mode + Rules + Built-in Checks** 三层机制。

#### 5 种权限模式

| 模式 | 常量 | 行为 | 适用场景 |
|-----|------|------|---------|
| DEFAULT | `tool.ModeDefault` | 需要显式规则或用户确认；只读命令自动放行 | 最安全，推荐默认 |
| EXPLORE | `tool.ModeExplore` | 只读工具+只读命令放行；任何修改拒绝 | 代码探索、规划 |
| ACCEPT_EDITS | `tool.ModeAcceptEdits` | 工作目录内文件操作自动放行 | 活跃开发 |
| BYPASS | `tool.ModeBypass` | 跳过所有检查（除 deny/ask 规则外） | 沙箱/可信环境 |
| DONT_ASK | `tool.ModeDontAsk` | 所有 ASK 转 DENY（无需用户回应） | CI/无人值守 |

#### 各 Mode 决策流程

```
DEFAULT:    DenyRules→AskRules→已接受规则→tool.CheckPermissions→AllowRules→ASK
EXPLORE:    DenyRules→AskRules→CheckReadOnly→true=ALLOW, false=DENY
ACCEPT_EDITS: DenyRules→AskRules→CheckReadOnly(true→ALLOW)→CheckPermissions→AllowRules→工作目录判断
BYPASS:     DenyRules→AskRules→CheckPermissions(非DENY=allow)→AllowRules→ALLOW
DONT_ASK:   DenyRules→AskRules(→DENY)→CheckPermissions(ASK→DENY, safety→DENY)→AllowRules→DENY
```

---

### 7. Model 层 (`model/`)

#### ChatModel 接口

```go
type ChatModel interface {
    Call(ctx, messages) (*Response, error)     // 同步调用
    Stream(ctx, messages) (<-chan StreamChunk, error) // 流式调用
    GetName() string                            // 模型名称
    GetProvider() string                        // 提供商名称
}
```

#### ToolSetter 接口（Agent 注入工具）

```go
// 模型实现此接口后，Agent 在每次调用前自动注入工具定义
//  FormatSystem：formatter 负责格式化，Agent 负责注入
type ToolSetter interface {
    SetTools(tools []ToolDefinition)
}
```

#### Formatter 层（ FormatterBase）

```go
// 将统一的 Msg/ToolDefinition 格式转换为特定提供商的 API 请求格式
type Formatter interface {
    FormatMessages(messages []Msg, tools []ToolDefinition) []map[string]interface{}
}

type FormatterType string
const (
    FormatterOpenAI    FormatterType = "openai"
    FormatterAnthropic FormatterType = "anthropic"
    FormatterGemini    FormatterType = "gemini"
    FormatterDeepSeek  FormatterType = "deepseek"
)

// 工厂方法
formatter := model.NewFormatter(model.FormatterAnthropic)
```

#### Rate Limiting（ LLMRateLimiter）

```go
// 每个 model key 独立的速率限制器：并发信号量 + QPM 滑动窗口
type RateLimitConfig struct {
    MaxConcurrent  int // 最大并发数（默认 10）
    MaxQPM         int // 每分钟最大请求数（默认 60）
    AcquireTimeout int // 获取超时秒数（默认 30）
}

// ModelConfig 中配置
llm := model.NewOpenAIModel(model.ModelConfig{
    RateLimitConfig: model.RateLimitConfig{
        MaxConcurrent: 5,
        MaxQPM:        30,
    },
})
```

#### StreamChunk 类型

```go
type StreamChunk struct {
    Type     string    // "content" / "thinking" / "tool_call" / "done" / "error"
    Content  string    // 增量文本
    Thinking string    // 思考内容增量（DeepSeek reasoning_content）
    ToolCall *ToolCall // 工具调用
    Error    error     // 错误
}
```

#### 内置实现

```go
// OpenAI / DeepSeek / Ollama
llm := model.NewOpenAIModel(model.ModelConfig{
    Model: "gpt-4", APIKey: "sk-xxx",
    BaseURL: "https://api.openai.com/v1",
    FormatterType: model.FormatterOpenAI,  // 可选，默认 OpenAI
})
llm := model.NewDeepSeekModel(model.ModelConfig{...})
llm := model.NewOllamaModel(model.ModelConfig{...})

// 自定义模型
type MyModel struct{}
func (m *MyModel) Call(ctx, msgs) (*model.Response, error) { ... }
```

#### 支持的模型特性

- **流式工具调用**：累积增量参数片段，完成后发送完整 ToolCall
- **SSE 格式兼容**：同时支持 `data: {...}` 和 `data:{...}` 两种格式
- **推理内容**：支持 DeepSeek `reasoning_content` → StreamChunk.Thinking → ThinkingBlock
- **多轮工具调用**：正确序列化 `tool_calls` 和 `role:"tool"` 消息到请求体
- **格式化器解耦**：通过 Formatter 接口支持 OpenAI/Anthropic/Gemini 格式

---

### 8. Tool 层 (`tool/`)

 的 ToolBase 设计，工具不仅是可执行函数，还内嵌完整的权限检查能力。

#### Tool 接口

```go
type Tool interface {
    Name() string                                   // 工具名称（暴露给 LLM）
    Description() string                            // 工具描述
    Parameters() map[string]interface{}             // JSON Schema 参数定义
    Execute(ctx, params) (string, error)            // 执行逻辑
}
```

#### ToolPermissionProvider（工具自定义权限逻辑）

```go
type ToolPermissionProvider interface {
    CheckPermissions(ctx, toolInput, context) (*ToolPermissionDecision, error)
    CheckReadOnly(toolInput) bool
    MatchRule(ruleContent, toolInput) bool
    GenerateSuggestions(toolInput) []PermissionRule
}
```

#### Registry（依赖注入，非全局）

```go
tools := tool.NewRegistry()
tools.Register(myTool)
tools.RegisterGroup("file_ops", []string{"read", "write"})
tools.Execute(ctx, "echo", params)
```

---

### 9. Session 管理 (`session.go`)

```go
type Session struct {
    id      string
    store   SessionStore
    history []Msg
}

// 基本操作
session.AddMessage(msg)                       // 添加消息
history := session.GetHistory()               // 获取全部历史
session.Clear()                               // 清除会话

// 标记过滤（ get_memory + filter by _MemoryMark）
history := session.GetHistoryExcludeMarks(MsgMarkCompressed) // 排除已压缩消息
count := session.MarkMessagesCompressed(messageIDs)          // 批量标记为已压缩
```

---

### 10. Memory 层 (`memory/`)

```go
type Memory interface {
    Store(ctx, key, content, memType) error            // "short_term" / "long_term"
    Retrieve(ctx, query, limit) ([]MemoryItem, error)   // 关键词检索
    GetRecent(ctx, limit) ([]MemoryItem, error)         // 最近记忆
    Consolidate(ctx, threshold) error                   // 短期→长期
    Forget(ctx, id) error
    Clear(ctx) error
}
```

### 11. Skill 层 (`skill/`)

```go
type Skill struct {
    Name        string   // 技能名称
    Description string   // 描述
    Prompt      string   // 核心能力描述
    Workflow    string   // 执行步骤（支持 {{variable}} 替换）
    Input       string   // 输入要求
    Output      string   // 输出格式
    Requires    []string // 依赖的二进制文件
}
```

---

## 完整使用示例

```go
package main

import (
    "context"
    "fmt"
    "github.com/nllihui6390/go-agent"
    "github.com/nllihui6390/go-agent/memory"
    "github.com/nllihui6390/go-agent/model"
    "github.com/nllihui6390/go-agent/tool"
)

func main() {
    // 1. 创建模型（含速率限制配置）
    llm := model.NewOpenAIModel(model.ModelConfig{
        Model: "gpt-4", APIKey: "sk-xxx",
        BaseURL:   "https://api.openai.com/v1",
        FormatterType: model.FormatterOpenAI,
        RateLimitConfig: model.RateLimitConfig{
            MaxConcurrent: 5,
            MaxQPM:        30,
        },
    })

    // 2. 创建工具
    tools := tool.NewRegistry()
    tools.Register(tool.NewBasicTool("echo", "回显文本", map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "text": map[string]interface{}{"type": "string"},
        },
    }, func(ctx context.Context, p map[string]interface{}) (string, error) {
        return "回显: " + p["text"].(string), nil
    }))

    // 3. 创建 Agent（Builder 模式 + BYPASS 权限模式）
    ag := agent.NewAgent(*agent.DefaultConfig("assistant", llm, "你是一个有帮助的助手。").
        WithTools(tools).
        WithMemory(memory.NewSimpleMemory()).
        WithMaxIters(10).
        WithPermission(agent.NewPermissionChecker(&tool.PermissionContext{
            Mode: tool.ModeBypass,
        })))

    // 4. 同步调用
    ctx := context.Background()
    reply, err := ag.Reply(ctx, agent.UserMsg("user", "你好！"))
    if err != nil { panic(err) }
    fmt.Println(reply.GetTextContent())

    // 5. 流式调用（含思考/工具过程展示）
    eventStream, _ := ag.ReplyStream(ctx, agent.UserMsg("user", "用计算器算 2+2"))
    for event := range eventStream {
        switch e := event.(type) {
        case agent.ThinkingBlockDeltaEvent:
            fmt.Printf("[思考] %s", e.Delta)
        case agent.TextBlockDeltaEvent:
            fmt.Print(e.Delta)
        case agent.ToolCallStartEvent:
            fmt.Printf("\n[调用工具: %s]", e.ToolCallName)
        case agent.ToolResultEndEvent:
            fmt.Printf(" [状态: %s]", e.State)
        case agent.ModelCallStartEvent:
            fmt.Printf("\n[模型调用: %s]", e.ModelName)
        case agent.ReplyEndEvent:
            fmt.Println("\n[完成]")
        }
    }

    // 6. Observe（多 Agent 场景）
    ag.Observe(otherAgentMsg)

    // 7. 上下文压缩（自动标记已压缩消息）
    ag.CompressContext(ctx, nil)
}
```

---

## 设计原则

1. **依赖注入** — 所有组件通过 Config 注入，无全局变量
2. **接口隔离** — Model/Tool/Memory/Skill/Permission/Offloader/Formatter 均为接口
3. **零外部依赖** — 不依赖 gateway/channel/webhook，纯 Agent 核心
4. **事件驱动流式** — start→delta→end 模式驱动实时 UI，含思考过程
5. **消息可重建** — 完整消息状态可从事件流还原
6. **自动上下文管理** — 压缩+截断+卸载三位一体，标记管理
7. **Builder 模式** — Config 链式调用，灵活组合
8. **Formatter 解耦** — 消息格式化与模型调用分离，轻松扩展新提供商
9. **消息不可变** — 每次 LLM 调用前深拷贝消息，保护 session 历史
10. **速率保护** — 内置 QPM + 并发限制，防止 API 429 错误

## License

MIT
