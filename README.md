# go-claw

Go 语言仿照 OpenClaw 架构思想实现的 AI Agent 框架。核心保留 Gateway-Agent-Session 三层解耦设计，并实现多渠道接入、工具调用、流式输出、记忆持久化、多 Agent 协作等能力。

**Agent 路由为手动指定模式 — 不自动根据消息内容切换 Agent。** 用户需通过 `/agent <name>` 命令（控制台）或 `agent` 字段（API）显式指定目标 Agent，未指定时走默认 Agent。

## 特性

- **三层架构**: Gateway（路由协调）→ Agent（LLM 交互）→ Session（会话管理）
- **多模型供应商**: 支持 OpenAI/DeepSeek 等云 API + 本地 Ollama，一个配置多个供应商
- **手动 Agent 路由**: `/agent <name>` 切换、API `agent` 字段指定，无自动关键词匹配
- **多渠道接入**: 控制台、REST API (HTTP)、WebSocket、飞书、钉钉、企业微信（均为 WebSocket 客户端模式）
- **流式输出**: SSE (Server-Sent Events) 流式响应，WebSocket 逐块推送
- **记忆系统**: 短期/长期记忆、关键词检索、向量语义检索、JSON 文件持久化
- **工具系统**: 插件模式、动态注册、Skill 分组、内置天气查询、命令执行、文件读写编辑追加、浏览器自动化、时间/时区、文件发送
- **智能运行时**: Auto-continue、Summarizing、API 重试（429/5xx 自动重试+指数退避）、Token 预算管理、智能终止、推理标签剥离
- **上下文压缩**: 对话接近 token 阈值时自动压缩旧消息为摘要
- **工具结果裁剪**: 超长结果截断并缓存到文件，支持豁免规则
- **工作空间人设**: 每个 Agent 有独立工作空间，AGENTS.md + SOUL.md + PROFILE.md 自动注入 system prompt
- **Skill 技能系统**: 全局技能 + Agent 专属技能两层架构，fsnotify 热加载
- **Skill 动态创建**: 对话中自动注入 SKILL.md 标准模板
- **浏览器自动化**: rod 驱动，CSS/XPath/:has-text() 选择器，panic 安全 + 超时保护
- **日志按日分割**: 日志文件自动按天分割，跨天无缝切换
- **自动记忆提取**: 对话后异步提取关键信息存入长期记忆
- **每日记忆**: 工作空间 `memory/YYYY-MM-DD.md` 每日记忆文件
- **主动模式**: 空闲后主动发送提醒或建议
- **多模态能力提示**: 不支持图片/视频时自动添加提示
- **多 Agent 协作**: 事件总线、监督者模式、Agent 间对话 (chat_with_agent/submit_to_agent)
- **任务规划**: Plan Mode 分解任务为子任务，跟踪进度
- **定时任务**: Cron 系统，支持 @every/HH:MM 调度、活跃时段控制
- **工具安全守卫**: Shell 注入检测、文件访问保护、规则引擎 (deny/guard/approve)
- **Dream 优化**: 空闲时记忆整理，去重合并清理 MEMORY.md
- **Inbox 系统**: 事件通知存储，心跳/定时任务结果记录
- **对话持久化**: 按日 JSONL 文件保存对话历史
- **每模型限流器**: QPM 滑动窗口、429 冷却、并发控制
- **Mission 模式**: 自主任务执行，Master/Worker/Verifier 三角色协作
- **ACP 协议**: 外部 Agent 集成，启动管理 Claude Code/Codex 等外部进程
- **MCP 集成**: Model Context Protocol 客户端，stdio/SSE 连接外部工具服务
- **安全**: Bearer Token 鉴权、令牌桶限流、工具安全守卫、命令执行过滤
- **运维**: Docker 化部署、配置热加载、Skill 热加载、Prometheus 指标、结构化日志

## 快速开始

```bash
# 1. 克隆并进入项目
git clone <repo> && cd go-claw

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env 填入 OPENAI_API_KEY 等

# 3. 构建
go build -o go-claw .

# 4. 运行
./go-claw
```

### Docker 部署

```bash
docker compose up -d
```

## 控制台命令

| 命令 | 说明 |
|------|------|
| `/agent <name>` | 切换到指定 Agent |
| `/agent` | 显示当前 Agent |
| `/agents` | 列出可用 Agent |
| `/help` | 显示帮助 |
| `/exit` | 退出 |

示例：

```
> 你好
[Assistant] 你好！

> /agent default
[系统] 已切换到Agent: default

> 今天天气怎么样
[Assistant] 北京 晴 25°C ...

> /help
[系统] 可用命令:
  /agent [name]   - 切换Agent
  /agents         - 列出可用Agent
  /exit           - 退出
  /help           - 显示帮助
```

## 机器人渠道

go-claw 支持三种 IM 机器人，均为 **WebSocket 客户端模式** — 主动连接对方服务器，无需本地开端口，无需消息加解密：

| 渠道 | 连接地址 | 配置字段 |
|------|---------|----------|
| 飞书 | `wss://open.feishu.cn/open-apis/event/v2/stream/` | `app_id`, `app_secret` |
| 钉钉 | `wss://stream.dingtalk.com` | `client_id`, `client_secret` |
| 企业微信 | `wss://openws.work.weixin.qq.com` | `bot_id`, `secret` |

配置示例：

```json
"channels": {
  "lark":      { "enabled": true, "app_id": "cli_xxx", "app_secret": "xxx" },
  "dingtalk":  { "enabled": true, "client_id": "xxx", "client_secret": "xxx" },
  "wecom":     { "enabled": true, "bot_id": "xxx", "secret": "xxx" }
}
```

所有 Bot 渠道自动心跳保活（30秒 ping）、断线自动重连。

## Skill 技能系统

go-claw 采用 **Prompt-based** 技能系统，技能描述注入系统提示词，AI 读取完整 SKILL.md 后用 `exec` 等工具执行脚本。

### 两层技能架构

| 层级 | 目录 | 说明 |
|------|------|------|
| 全局技能 | `goclaw-data/skills/` | 所有 Agent 共享 |
| Agent 专属 | `goclaw-data/workspaces/<agent-name>/skills/` | 仅当前 Agent 可用 |

同名技能 Agent 专属版优先。Skill 目录通过 `fsnotify` 热加载，无需重启。

### 使用流程

```
1. 系统提示词注入技能信息（名称、描述、SKILL.md 路径、脚本路径）
2. AI 用 read_file 读取完整 SKILL.md（包含参数表、示例、注意事项）
3. AI 理解如何使用技能，构造正确的命令参数
4. AI 用 exec 工具执行脚本
```

**优势**：AI 拥有完整 SKILL.md 上下文，无参数 Schema 刚性，更灵活。

### SKILL.md 格式

```markdown
---
name: weather-query
description: 查询指定城市的实时天气信息和天气预报
metadata:
  openclaw:
    emoji: "🌤️"
    requires:
      bins:
        - curl
---

## 核心能力
- 查询任意城市的当前天气状况

## 执行步骤
1. 确认用户要查询的城市名称 {{city}}
2. 如果有天气 API 工具可用，直接调用 weather 工具查询

## 输出格式
返回城市的天气信息，包括温度、体感温度、风速、湿度
```

### 动态创建技能

对话中发送包含"创建技能""做成技能""封装成skill"等关键词时，自动注入 SKILL.md 标准模板。

## Agent 运行时优化

### Auto-continue

模型暗示要使用工具但没实际调用时，自动注入提示继续循环，最多 3 次。

### Summarizing

达到 `max_iterations` 上限时，调用模型不带 tools 优雅总结而非硬中断。

### API 重试

`callLLMWithRetry` 处理 429/500-504 错误，自动重试 3 次，指数退避（1s → 2s → 4s，上限 30s）。

### 智能终止

- 安全上限 100 次迭代
- 总失败次数 ≥ 8 且 > 3× 成功次数
- 模型返回无 tool_calls 的最终响应

### 推理标签剥离

自动剥离 DeepSeek 等模型的内部推理标签，用户只看到实际回答内容。

### 上下文压缩

对话接近 `max_tokens` 阈值（默认 80%）时，自动调用 LLM 压缩旧消息为摘要。压缩后保留 system prompt + 压缩摘要 + 最近消息。

```json
{
  "compact_threshold_ratio": 0.8,
  "reserve_threshold_ratio": 0.15
}
```

### 工具结果裁剪

超长结果自动截断，完整内容保存到 `cache/` 目录。支持按工具名和文件扩展名豁免裁剪。

```json
{
  "tool_result_max_bytes": 20000,
  "tool_result_exempt_tools": ["browser_use"],
  "tool_result_exempt_extensions": [".png", ".jpg"]
}
```

### 多模态能力提示

```json
{
  "supports_image": false,
  "supports_video": false
}
```

### 自动记忆提取

每次对话结束后，异步调用 LLM 提取关键信息（用户偏好、重要决策、待办事项），存入长期记忆和每日记忆文件。

### 主动模式

后台监控会话空闲时间，超过 `idle_minutes` 后分析记忆，主动发送提醒或建议消息。

```json
"proactive": { "enabled": true, "idle_minutes": 30, "agent_name": "default" }
```

### Dream 优化

空闲时自动整理记忆：去重、合并相似条目、清理过期信息、覆盖旧状态。基于极简主义、状态覆盖、归纳合并、过期清理四大原则。

### 任务规划 (Plan Mode)

将复杂任务分解为子任务，跟踪进度（todo/in_progress/done/abandoned），支持任务状态更新和进度摘要。

### 定时任务 (Cron System)

支持三种调度格式：
- `@every 5m` — 每 5 分钟
- `HH:MM` — 每天 HH:MM
- 标准 cron 表达式

支持活跃时段控制（如仅在工作时间 09:00-18:00 执行）和两种任务类型：文本消息和 Agent 任务。

### 工具安全守卫 (Tool Guard)

三层安全机制：
- **Shell 注入检测**: 检测 `$(cmd)`、`|cmd`、`;cmd`、`rm -rf /` 等危险命令模式
- **文件访问保护**: 禁止访问 `.env`、`credentials`、`/etc/passwd` 等敏感路径
- **规则引擎**: 可配置 deny/guard/approve 规则，浏览器自动化等高风险操作需确认

### 多 Agent 协作

| 工具 | 说明 |
|------|------|
| `list_agents` | 列出所有可用 Agent |
| `chat_with_agent` | 与指定 Agent 对话，等待回复 |
| `submit_to_agent` | 向 Agent 提交后台任务，返回任务 ID |
| `check_agent_task` | 查询后台任务结果 |

Agent 间消息自动添加来源标识 `[Agent {caller_id} requesting]`。

### Inbox 系统

事件通知存储，记录心跳、定时任务、主动消息等事件，支持已读/未读跟踪、严重程度分级。

### 对话持久化

对话历史按日保存到 JSONL 文件（`dialogs/YYYY-MM-DD.jsonl`），支持按日期范围读取。

### 每模型限流器

独立限流器：QPM 滑动窗口、429 冷却退避、并发信号量控制、请求间抖动避免突发。

### Mission 模式

自主任务执行引擎：
- Phase 1: LLM 生成 PRD（产品需求文档）
- Phase 2: 迭代执行，每轮生成具体指令
- Phase 3: 可选验证（Verify），判断任务是否完成

配置 `max_iterations` 和 `verify` 参数。

### ACP 协议

外部 Agent 集成协议，启动和管理 Claude Code、Codex 等外部进程：
- stdio 管道通信
- JSON 消息格式
- 异步任务提交和结果查询

### MCP 集成

Model Context Protocol 客户端，连接外部工具服务：
- stdio 模式（启动子进程）
- SSE 模式（连接 HTTP 服务器）
- JSON-RPC 2.0 协议
- 自动初始化和工具发现

## 内置工具

| 工具名 | 说明 |
|--------|------|
| `weather` | 天气查询（和风/OpenWeather/Seniverse） |
| `exec` | Shell 命令执行（安全过滤） |
| `write_file` | 写文件（自动创建目录） |
| `read_file` | 读文件（支持行号/偏移） |
| `edit_file` | 编辑文件（精确字符串替换） |
| `append_file` | 追加文件（增量写入） |
| `send_file` | 发送文件给用户 |
| `browser_use` | 浏览器自动化 (rod) |
| `get_current_time` | 获取当前时间 |
| `set_user_timezone` | 设置用户时区 |
| `list_agents` | 列出所有 Agent |
| `chat_with_agent` | 与其他 Agent 对话 |
| `submit_to_agent` | 提交后台任务 |
| `cron_status` | 查询内部定时任务状态 |

## 工作空间人设系统

每个 Agent 有独立工作空间目录 `goclaw-data/workspaces/<agent-name>/`：

| 文件 | 用途 | 是否注入 system prompt |
|------|------|----------------------|
| AGENTS.md | 行为规则、安全指南 | ✅ 是 |
| SOUL.md | 核心人格、价值观 | ✅ 是 |
| PROFILE.md | 用户身份、偏好 | ✅ 是 |
| MEMORY.md | 长期记忆 | ❌ 否（工具访问） |
| HEARTBEAT.md | 周期任务提示 | ❌ 否 |
| BOOTSTRAP.md | 首次引导（自动标记完成） | ✅ 首次对话时 |

AGENTS.md 支持 `<!-- heartbeat:start -->` 和 `<!-- memory:start -->` 条件区块。

## 浏览器自动化

基于 rod 的浏览器自动化，支持 navigate、click、type、extract、screenshot、scroll、wait 操作。

| 选择器 | 示例 | 说明 |
|--------|------|------|
| CSS | `#id`, `.class` | `querySelector` |
| XPath | `//a[contains(text(),'文本')]` | `MustElementX` |
| :has-text() | `a:has-text("文本")` | 自动转换为 XPath |

安全：panic recovery、10 秒元素超时、30 秒导航超时、选择器验证。

## 日志系统

按日自动分割，格式 `{prefix}-{YYYY-MM-DD}{ext}`，跨天无缝切换。

```json
"logging": { "level": "info", "json_mode": false, "file_path": "logs/app.log", "console": true }
```

## API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/chat` | 发送消息（`agent` 字段、`stream=true` SSE） |
| GET | `/api/v1/sessions` | 列出会话 |
| GET | `/api/v1/sessions/{id}` | 查看会话详情 |
| DELETE | `/api/v1/sessions/{id}` | 删除会话 |
| POST | `/webhook` | 旧版 webhook |
| GET | `/health` | 健康检查 |
| GET | `/metrics` | Prometheus 指标 |
| GET | `/ws` | WebSocket (`?session=xxx`) |

## 项目结构

```
go-claw/
├── main.go                              # 主入口
├── config.json                          # 配置文件
├── config/config.go                     # 配置管理
├── pkg/log/log.go                       # 结构化日志 + 按日分割
├── internal/
│   ├── gateway/
│   │   ├── gateway.go                   # 网关核心 + 主动消息发送
│   │   ├── router.go                    # 手动路由
│   │   ├── agent_bus.go                 # Agent 事件总线
│   │   └── config_watcher.go            # 配置热加载
│   ├── agent/
│   │   ├── agent.go                     # Agent 核心 + 记忆提取
│   │   ├── runtime.go                   # 运行时（压缩/裁剪/豁免/LLM 调用）
│   │   ├── context.go                   # 会话管理（持久化 + 压缩摘要）
│   │   ├── skill_template.go            # 技能创建模板
│   │   └── supervisor.go               # 监督者 Agent
│   ├── channel/
│   │   ├── channel.go                   # 渠道接口 + Message
│   │   ├── console.go                   # 控制台渠道
│   │   ├── webhook.go                   # REST API + SSE + 指标
│   │   ├── websocket.go                 # WebSocket 渠道
│   │   ├── bot_base.go                  # Bot 共享基础
│   │   ├── lark.go                      # 飞书机器人
│   │   ├── dingtalk.go                  # 钉钉机器人
│   │   └── wecom.go                     # 企业微信机器人
│   ├── tool/
│   │   ├── tool.go                      # 工具接口
│   │   ├── registry.go                  # 工具注册表
│   │   ├── weather.go                   # 天气工具
│   │   ├── exec.go                      # 命令执行
│   │   ├── file.go                      # 文件读写编辑追加发送
│   │   ├── browser.go                   # 浏览器自动化
│   │   └── time.go                      # 时间/时区工具
│   ├── skill/
│   │   ├── skill.go                     # SKILL.md 解析
│   │   └── registry.go                  # 注册中心 + 热重载 + Prompt 生成
│   ├── memory/
│   │   ├── memory.go                    # 记忆接口
│   │   ├── simple.go                    # 关键词检索
│   │   ├── vector.go                    # 向量检索
│   │   └── dream.go                     # Dream 优化整理
│   ├── store/
│   │   ├── store.go                     # 持久化接口
│   │   ├── file.go                      # JSON 文件实现
│   │   └── dialog.go                    # 对话 JSONL 持久化
│   ├── workspace/
│   │   └── loader.go                    # 人设/记忆/引导加载器
│   ├── proactive/
│   │   └── proactive.go                 # 主动模式管理器
│   ├── inbox/
│   │   └── inbox.go                     # 事件通知存储
│   ├── cron/
│   │   └── cron.go                      # 定时任务系统
│   ├── plan/
│   │   └── plan.go                      # 任务规划
│   ├── multiagent/
│   │   └── multiagent.go                # 多 Agent 交互工具
│   ├── security/
│   │   └── tool_guard.go                # 工具安全守卫
│   ├── mission/
│   │   └── mission.go                   # 自主任务执行
│   ├── acp/
│   │   └── acp.go                       # Agent 通信协议
│   ├── mcp/
│   │   └── mcp.go                       # MCP 客户端集成
│   ├── ratelimiter/
│   │   └── rate_limiter.go              # 每模型限流器
│   └── middleware/
│       ├── auth.go                      # Bearer Token 鉴权
│       └── rate_limit.go                # 限流中间件
├── goclaw-data/
│   ├── skills/                          # 全局技能
│   └── workspaces/
│       └── <agent-name>/
│           ├── AGENTS.md / SOUL.md / PROFILE.md
│           ├── MEMORY.md / HEARTBEAT.md / BOOTSTRAP.md
│           ├── memories.json
│           ├── sessions/
│           ├── memory/YYYY-MM-DD.md      # 每日记忆
│           ├── cache/*.txt               # 工具结果缓存
│           ├── plans/*.json              # 任务规划
│           ├── inbox.json                # 事件通知
│           └── skills/                   # Agent 专属技能
├── logs/
├── Dockerfile
├── docker-compose.yml
├── .env.example
├── go.mod
└── go.sum
```

## 配置

```json
{
  "gateway": { "default_agent": "default", "session_ttl": 60, "data_dir": "goclaw-data" },
  "providers": {
    "deepseek": {
      "type": "openai",
      "base_url": "https://api.deepseek.com/v1",
      "api_key": "your-api-key",
      "default_model": "deepseek-chat"
    },
    "ollama": {
      "type": "ollama",
      "base_url": "http://localhost:11434",
      "default_model": "llama3"
    }
  },
  "agents": [
    {
      "name": "default",
      "provider": "deepseek",
      "model": "deepseek-v4-oc",
      "system_prompt": "你是一个有用的AI助手。",
      "tools": ["weather", "exec", "write_file", "read_file", "edit_file", "append_file", "browser_use", "get_current_time"],
      "max_iterations": 20,
      "max_tokens": 32000,
      "compact_threshold_ratio": 0.8,
      "reserve_threshold_ratio": 0.15,
      "tool_result_max_bytes": 20000,
      "tool_result_exempt_tools": ["browser_use"],
      "tool_result_exempt_extensions": [".png", ".jpg"],
      "supports_image": false,
      "supports_video": false
    }
  ],
  "channels": {
    "console":   { "enabled": true },
    "webhook":   { "enabled": true, "port": "8080" },
    "websocket": { "enabled": false, "port": "8081" },
    "lark":      { "enabled": false, "app_id": "", "app_secret": "" },
    "dingtalk":  { "enabled": false, "client_id": "", "client_secret": "" },
    "wecom":     { "enabled": false, "bot_id": "", "secret": "" }
  },
  "logging": { "level": "info", "json_mode": false, "file_path": "logs/app.log", "console": true },
  "auth": { "enabled": false, "token": "" },
  "skills": { "enabled": true },
  "proactive": { "enabled": false, "idle_minutes": 30, "agent_name": "default" }
}
```

### 供应商类型

| 类型 | API格式 | 认证 |
|------|---------|------|
| `openai` | `/chat/completions` | Bearer Token |
| `ollama` | `/api/chat` | 无需认证 |

### 环境变量

| 变量 | 说明 | 必需 |
|------|------|------|
| `PROVIDER_<name>_API_KEY` | 供应商 API Key | 按需 |
| `PROVIDER_<name>_BASE_URL` | 供应商 API 地址 | 否 |
| `OPENAI_API_KEY` | 兼容旧配置 | 否 |
| `OPENAI_BASE_URL` | 兼容旧配置 | 否 |
| `HEFENG_API_KEY` | 和风天气 API Key | 否 |
| `GOCLAW_HOT_RELOAD` | 启用配置热加载 | 否 |