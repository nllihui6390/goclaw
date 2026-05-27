# go-claw

Go 语言仿照 OpenClaw 架构思想实现的 AI Agent 框架。核心保留 Gateway-Agent-Session 三层解耦设计，并实现多渠道接入、工具调用、流式输出、记忆持久化、多 Agent 协作等能力。

**Agent 路由为手动指定模式 — 不自动根据消息内容切换 Agent。** 用户需通过 `/agent <name>` 命令（控制台）或 `agent` 字段（API）显式指定目标 Agent，未指定时走默认 Agent。

## 特性

- **三层架构**: Gateway（路由协调）→ Agent（LLM 交互）→ Session（会话管理）
- **多模型供应商**: 支持 OpenAI/DeepSeek 等云 API + 本地 Ollama，一个配置多个供应商
- **手动 Agent 路由**: `/agent <name>` 切换、API `agent` 字段指定，无自动关键词匹配
- **多渠道接入**: 控制台 (stdin/stdout)、REST API (HTTP)、WebSocket (实时双向)、飞书机器人、钉钉机器人、企业微信机器人（均为 WebSocket 客户端模式，无需开端口）
- **流式输出**: SSE (Server-Sent Events) 流式响应，WebSocket 逐块推送
- **记忆系统**: 短期/长期记忆、关键词检索、向量语义检索 (Embedding + 余弦相似度)、JSON 文件持久化
- **工具系统**: 插件模式、动态注册、Skill 分组、内置天气查询、命令执行、文件读写编辑、浏览器自动化、技能调用；控制台实时输出工具调用过程
- **智能运行时**: Auto-continue（模型暗示工具时自动注入提示继续）、Summarizing（迭代上限时优雅总结而非硬中断）、API 重试（429/5xx 自动重试+指数退避）、Token 预算管理、智能终止
- **工作空间人设**: 每个 Agent 有独立工作空间，AGENTS.md + SOUL.md + PROFILE.md 自动加载注入 system prompt
- **Skill 技能系统**: 全局技能 + Agent 专属技能两层架构，fsnotify 热加载，无需重启即可生效
- **Skill 动态创建**: 用户对话中要求创建技能时，自动注入 SKILL.md 标准模板，AI 使用 write_file 工具按规范创建
- **浏览器自动化**: rod 驱动，支持 CSS 和 XPath 选择器，自动转换 Playwright :has-text() 伪选择器，全操作 panic 安全 + 超时保护
- **日志按日分割**: 日志文件自动按天分割（如 `app-2026-05-27.log`），跨天无缝切换
- **多 Agent 协作**: 事件总线 (AgentBus)、监督者模式 (SupervisorAgent)
- **安全**: Bearer Token 鉴权、令牌桶限流、命令执行安全过滤
- **运维**: Docker 化部署、配置热加载 (fsnotify)、Skill 热加载、Prometheus 指标、结构化日志

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

go-claw 支持兼容 OpenClaw 规范的 Skill 系统，通过 `SKILL.md` 文件定义技能，AI 可通过 `skill_use` 工具主动调用。

### 两层技能架构

每个 Agent 同时加载 **全局技能** 和 **Agent 专属技能**：

| 层级 | 目录 | 说明 |
|------|------|------|
| 全局技能 | `goclaw-data/skills/` | 所有 Agent 共享 |
| Agent 专属 | `goclaw-data/workspaces/<agent-name>/skills/` | 仅当前 Agent 可用 |

同名技能 Agent 专属版优先（不覆盖全局版）。

### 热加载

Skill 目录通过 `fsnotify` 监控，文件增删改后自动重载，**无需重启程序**：

- 全局目录变化 → 重载所有 Agent 的技能
- Agent 目录变化 → 仅重载该 Agent 的技能

### SKILL.md 格式

每个 Skill 位于独立子目录，包含 `SKILL.md`（YAML frontmatter + Markdown 正文）和可选的 `scripts/` 脚本目录：

```
goclaw-data/skills/
└── weather-query/
    ├── SKILL.md          # 技能定义
    └── scripts/          # 可选脚本 (.sh/.py/.js)
        └── query.sh
```

SKILL.md 示例：

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
- 获取未来几天的天气预报

## 执行步骤
1. 确认用户要查询的城市名称 {{city}}
2. 如果有天气 API 工具可用，直接调用 weather 工具查询
3. 如果没有，使用 curl 调用 wttr.in 服务

## 输出格式
返回城市的天气信息，包括温度、体感温度、风速、湿度

## 异常处理
- 如果城市名无法识别，提示用户确认
- 如果 API 调用失败，返回错误信息并建议稍后重试
```

### 运行机制

- **有脚本时**: 执行 `scripts/` 目录下的脚本，参数通过环境变量 (`SKILL_*`) 或命令行参数 (`--key=value`) 传递
- **无脚本时**: 将 Skill 的核心能力、执行步骤、输出格式等生成结构化指导信息，交给 AI 自行完成
- **变量替换**: 正文中 `{{city}}` 等占位符会被调用参数替换
- **依赖检查**: `metadata.openclaw.requires.bins` 检查所需命令是否存在，缺失则跳过加载

### 动态创建技能

用户在对话中发送包含"创建技能""做成技能""封装成skill"等关键词的消息时，系统会自动注入 SKILL.md 标准模板到 system prompt，AI 将使用 `write_file` 工具按规范创建技能文件。

触发关键词：创建技能、新建skill、做成技能、保存为技能、封装成skill、create skill、make a skill 等。

创建完成后，技能持久保存在 Agent 的专属技能目录，热加载自动生效。

### 配置

```json
"skills": {
  "enabled": true
}
```

`skill_use` 工具自动加载（无需在 agent 的 tools 列表中配置）。当 `skills.enabled` 为 true 且技能目录中有可用技能时，`skill_use` 自动注入到 Agent 的工具列表。

## Agent 运行时优化

基于 CoPaw/QwenPaw 设计模式，Agent 运行时实现了以下智能优化：

### Auto-continue 机制

当模型返回纯文本（无 tool_calls）但内容暗示要使用工具时（如"我来查询天气"），自动注入提示"请直接调用工具来完成任务"并继续循环，最多 3 次。避免模型只描述意图而不实际执行。

DeepSeek 模型的内部推理标签 `flater`...`flater` 会先被剥离，再判断是否需要 auto-continue，避免思考内容误触发。

### Summarizing 机制

达到配置的 `max_iterations` 上限时，不硬性报错退出，而是调用模型不带 tools（等效 `tool_choice: "none"`），让模型优雅总结当前进度并返回。

### API 重试

`callLLMWithRetry` 包装器处理 429/500-504 错误，自动重试 3 次，指数退避（1s → 2s → 4s，上限 30s）。

### Token 预算管理

`buildMessages` 中根据 `max_tokens` 配置估算上下文长度，超限时截断旧消息保留 system prompt + 最近 N 条消息。

### 智能终止

循环不再硬性限制次数，而是根据以下条件智能退出：
- 安全上限 100 次迭代
- 同一工具连续失败 3 次
- 总失败次数 ≥ 8 且 > 3× 成功次数
- 模型返回无 tool_calls 的最终响应

### 推理标签剥离

自动剥离 DeepSeek 等模型的内部推理标签，用户只看到实际回答内容，不暴露思考过程。

## 工作空间人设系统

基于 CoPaw 设计，每个 Agent 有独立的工作空间目录 `goclaw-data/workspaces/<agent-name>/`，包含人设文件，启动时自动加载注入到 system prompt：

| 文件 | 用途 | 是否注入 system prompt |
|------|------|----------------------|
| AGENTS.md | 行为规则、安全指南、工具使用说明 | ✅ 是（第一个） |
| SOUL.md | 核心人格、价值观、行为准则 | ✅ 是（第二个） |
| PROFILE.md | 用户身份、偏好、上下文 | ✅ 是（第三个） |
| MEMORY.md | 长期记忆存储 | ❌ 否（可通过工具访问） |
| HEARTBEAT.md | 周期任务提示 | ❌ 否（heartbeat 功能使用） |

### 加载顺序

默认注入顺序：`AGENTS.md` → `SOUL.md` → `PROFILE.md`，拼接后添加到 system prompt 开头。

文件不存在则跳过，YAML frontmatter（`---`分隔的元数据块）自动剥离。

### 自定义人设

修改工作空间目录下的人设文件，重启生效。AI 行为会根据 SOUL.md 定义的人格调整。

```
goclaw-data/workspaces/<agent-name>/SOUL.md     ← 修改核心人格
goclaw-data/workspaces/<agent-name>/PROFILE.md   ← 修改用户偏好
```

## 浏览器自动化工具

基于 rod 的浏览器自动化工具，支持以下操作：navigate、click、type、extract、screenshot、scroll、wait。

### 选择器支持

| 格式 | 示例 | 说明 |
|------|------|------|
| CSS 选择器 | `#id`, `.class`, `div > a` | 标准 CSS，使用 `querySelector` |
| XPath | `//a[contains(text(),'文本')]` | 使用 `MustElementX` |
| Playwright :has-text() | `a:has-text("财务分析")` | 自动转换为 XPath |

Playwright 伪选择器自动转换规则：
- `tag:has-text("文本")` → `//tag[contains(text(), "文本")]`
- `tag.class:has-text("文本")` → `//tag[contains(@class, "class") and contains(text(), "文本")]`
- `tag#id:has-text("文本")` → `//tag[@id="id" and contains(text(), "文本")]`

不支持 `:visible`、`:nth-match()`、`text=` 等 Playwright 伪选择器。

### 安全机制

- **Panic 安全**: 所有 rod 操作通过 `safeCall` 包装 panic recovery，工具失败返回错误而非崩溃退出
- **超时保护**: 元素查找 10 秒超时，页面导航 30 秒超时
- **选择器验证**: 无效选择器在执行前被拦截并返回明确错误信息

## 日志系统

日志支持按日自动分割，文件名格式：`{prefix}-{YYYY-MM-DD}{ext}`

配置 `logs/app.log` → 实际生成 `logs/app-2026-05-27.log`、`logs/app-2026-05-28.log` 等。

跨天写入时自动切换新文件，旧文件关闭，无缝衔接。

```json
"logging": { "level": "info", "json_mode": false, "file_path": "logs/app.log", "console": true }
```

## API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/chat` | 发送消息（支持 `agent` 字段指定目标 Agent，`stream=true` SSE 流式） |
| GET | `/api/v1/sessions` | 列出会话 |
| GET | `/api/v1/sessions/{id}` | 查看会话详情 |
| DELETE | `/api/v1/sessions/{id}` | 删除会话 |
| POST | `/webhook` | 旧版 webhook（兼容） |
| GET | `/health` | 健康检查 |
| GET | `/metrics` | Prometheus 格式指标 |
| GET | `/ws` | WebSocket 连接 (`?session=xxx`) |

### 示例请求

```bash
# 阻塞模式（使用默认 Agent）
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"session":"user1","content":"你好"}'

# 指定 Agent
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"session":"user1","content":"今天天气怎么样","agent":"default"}'

# 通过请求头指定 Agent
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -H "X-Agent: default" \
  -d '{"session":"user1","content":"你好"}'

# 流式模式 (SSE)
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"session":"user1","content":"你好","stream":true}'

# WebSocket
wscat -c ws://localhost:8081/ws?session=user1
```

## 项目结构

```
go-claw/
├── main.go                          # 主入口
├── config.json                      # 配置文件
├── config/config.go                 # 配置管理
├── pkg/log/log.go                   # 结构化日志 (slog) + 按日分割
├── internal/
│   ├── gateway/
│   │   ├── gateway.go               # 网关核心
│   │   ├── router.go                # 手动路由（msg.Agent → agent）
│   │   ├── agent_bus.go             # Agent 事件总线
│   │   └── config_watcher.go        # 配置热加载
│   ├── agent/
│   │   ├── agent.go                 # Agent 核心
│   │   ├── runtime.go               # 运行时（阻塞 + 流式 + 智能循环）
│   │   ├── context.go               # 会话管理（持久化）
│   │   ├── skill_template.go        # 技能创建模板（动态注入）
│   │   └── supervisor.go            # 监督者 Agent
│   ├── channel/
│   │   ├── channel.go               # 渠道接口 + Message 结构体
│   │   ├── console.go               # 控制台渠道（/agent 命令）
│   │   ├── webhook.go               # REST API + SSE + 指标
│   │   ├── websocket.go             # WebSocket 渠道
│   │   ├── bot_base.go              # Bot 渠道共享基础
│   │   ├── lark.go                  # 飞书机器人 (WebSocket 客户端)
│   │   ├── dingtalk.go              # 钉钉机器人 (Stream 模式)
│   │   └── wecom.go                 # 企业微信机器人 (WebSocket 长连接)
│   ├── tool/
│   │   ├── tool.go                  # 工具接口
│   │   ├── registry.go              # 工具注册表 + Skill 分组
│   │   ├── weather.go               # 天气工具
│   │   ├── exec.go                  # 命令执行
│   │   ├── file.go                  # 文件读写编辑工具
│   │   └── browser.go               # 浏览器自动化 (rod, panic安全, XPath支持)
│   ├── skill/
│   │   ├── skill.go                 # Skill 结构体 + SKILL.md 解析
│   │   ├── registry.go              # Skill 注册中心（加载/热重载/匹配）
│   │   ├── executor.go              # Skill 执行器（脚本/AI指导）
│   │   └── tool.go                  # skill_use 工具（AI调用入口）
│   ├── memory/
│   │   ├── memory.go                # 记忆接口
│   │   ├── simple.go                # 关键词检索 + 持久化
│   │   └── vector.go                # 向量语义检索
│   ├── store/
│   │   ├── store.go                 # 持久化接口
│   │   └── file.go                  # 目录级 JSON 文件实现
│   ├── workspace/
│   │   └── loader.go                # 工作空间人设文件加载器
│   └── middleware/
│       ├── auth.go                  # Bearer Token 鉴权
│       └── rate_limit.go            # 限流中间件
├── goclaw-data/                     # 数据根目录（自动创建）
│   ├── skills/                      # 全局技能目录（所有 Agent 共享）
│   │   └── skill-name/
│   │       └── SKILL.md
│   └── workspaces/
│       └── <agent-name>/            # 每个 Agent 独立工作空间
│           ├── AGENTS.md            # 行为规则
│           ├── HEARTBEAT.md         # 心跳状态
│           ├── MEMORY.md            # 记忆说明
│           ├── PROFILE.md           # 用户偏好
│           ├── SOUL.md              # Agent 人格
│           ├── memories.json        # 记忆数据
│           ├── sessions/            # 会话文件目录（每个会话一个 JSON）
│           │   └── *.json
│           └── skills/              # Agent 专属技能目录
│               └── skill-name/
│                   └── SKILL.md
├── logs/                            # 日志目录（按日分割）
│   ├── app-2026-05-27.log
│   └── app-2026-05-28.log
├── Dockerfile                       # Docker 多阶段构建
├── docker-compose.yml               # Docker Compose
├── .env.example                     # 环境变量模板
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
      "tools": ["weather", "exec", "write_file", "read_file", "edit_file", "browser_use"],
      "max_iterations": 20,
      "max_tokens": 32000
    },
    {
      "name": "local",
      "provider": "ollama",
      "model": "qwen2.5:7b",
      "system_prompt": "你是一个本地AI助手。",
      "tools": [],
      "max_iterations": 20
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
  "skills": { "enabled": true }
}
```

注意：`skill_use` 不需要在 `tools` 列表中配置，当 `skills.enabled` 为 true 时自动加载。

### 供应商类型

| 类型 | 说明 | API格式 | 认证 |
|------|------|---------|------|
| `openai` | OpenAI兼容API (OpenAI/DeepSeek/Azure等) | `/chat/completions` | Bearer Token |
| `ollama` | 本地 Ollama | `/api/chat` | 无需认证 |

### 环境变量

| 变量 | 说明 | 必需 |
|------|------|------|
| `PROVIDER_<name>_API_KEY` | 供应商 API Key（如 `PROVIDER_DEEPSEEK_API_KEY`） | 按需 |
| `PROVIDER_<name>_BASE_URL` | 供应商 API 地址 | 否 |
| `OPENAI_API_KEY` | 兼容旧配置，覆盖 agent 的 api_key | 否 |
| `OPENAI_BASE_URL` | 兼容旧配置，覆盖 agent 的 base_url | 否 |
| `HEFENG_API_KEY` | 和风天气 API Key | 否 |
| `GOCLAW_HOT_RELOAD` | 启用配置热加载 (`true`) | 否 |