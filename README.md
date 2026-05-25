# go-claw

Go 语言仿照 OpenClaw 架构思想实现的 AI Agent 框架。核心保留 Gateway-Agent-Session 三层解耦设计，并实现多渠道接入、工具调用、流式输出、记忆持久化、多 Agent 协作等能力。

**Agent 路由为手动指定模式 — 不自动根据消息内容切换 Agent。** 用户需通过 `/agent <name>` 命令（控制台）或 `agent` 字段（API）显式指定目标 Agent，未指定时走默认 Agent。

## 特性

- **三层架构**: Gateway（路由协调）→ Agent（LLM 交互）→ Session（会话管理）
- **多模型供应商**: 支持 OpenAI/DeepSeek 等云 API + 本地 Ollama，一个配置多个供应商
- **手动 Agent 路由**: `/agent <name>` 切换、API `agent` 字段指定，无自动关键词匹配
- **多渠道接入**: 控制台 (stdin/stdout)、REST API (HTTP)、WebSocket (实时双向)、Webhook (旧版兼容)
- **流式输出**: SSE (Server-Sent Events) 流式响应，WebSocket 逐块推送
- **记忆系统**: 短期/长期记忆、关键词检索、向量语义检索 (Embedding + 余弦相似度)、JSON 文件持久化
- **工具系统**: 插件模式、动态注册、Skill 分组、内置天气查询、命令执行、文件读写编辑、浏览器自动化；控制台实时输出工具调用过程
- **多 Agent 协作**: 事件总线 (AgentBus)、监督者模式 (SupervisorAgent)
- **安全**: Bearer Token 鉴权、令牌桶限流、命令执行安全过滤
- **运维**: Docker 化部署、配置热加载 (fsnotify)、Prometheus 指标、结构化日志

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
├── pkg/log/log.go                   # 结构化日志 (slog)
├── internal/
│   ├── gateway/
│   │   ├── gateway.go               # 网关核心
│   │   ├── router.go                # 手动路由（msg.Agent → agent）
│   │   ├── agent_bus.go             # Agent 事件总线
│   │   └── config_watcher.go        # 配置热加载
│   ├── agent/
│   │   ├── agent.go                 # Agent 核心
│   │   ├── runtime.go               # 运行时（阻塞 + 流式）
│   │   ├── context.go               # 会话管理（持久化）
│   │   └── supervisor.go            # 监督者 Agent
│   ├── channel/
│   │   ├── channel.go               # 渠道接口 + Message 结构体
│   │   ├── console.go               # 控制台渠道（/agent 命令）
│   │   ├── webhook.go               # REST API + SSE + 指标
│   │   └── websocket.go             # WebSocket 渠道
│   ├── tool/
│   │   ├── tool.go                  # 工具接口
│   │   ├── registry.go              # 工具注册表 + Skill 分组
│   │   ├── weather.go               # 天气工具
│   │   └── exec.go                  # 命令执行
│   │   └── file.go                  # 文件读写编辑工具
│   │   └── browser.go               # 浏览器自动化工具 (chromedp)
│   ├── memory/
│   │   ├── memory.go                # 记忆接口
│   │   ├── simple.go                # 关键词检索 + 持久化
│   │   └── vector.go                # 向量语义检索
│   ├── store/
│   │   ├── store.go                 # 持久化接口
│   │   └── file.go                  # JSON 文件实现
│   └── middleware/
│       ├── auth.go                  # Bearer Token 鉴权
│       └── rate_limit.go            # 限流中间件
├── Dockerfile                       # Docker 多阶段构建
├── docker-compose.yml               # Docker Compose
├── .env.example                     # 环境变量模板
├── go.mod
└── go.sum
```

## 配置

```json
{
  "gateway": { "default_agent": "default", "session_ttl": 60 },
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
      "tools": ["weather", "exec", "write_file", "read_file", "edit_file"],
      "max_iterations": 5
    },
    {
      "name": "local",
      "provider": "ollama",
      "model": "qwen2.5:7b",
      "system_prompt": "你是一个本地AI助手。",
      "tools": [],
      "max_iterations": 3
    }
  ],
  "channels": {
    "console":   { "enabled": true },
    "webhook":   { "enabled": true, "port": "8080" },
    "websocket": { "enabled": false, "port": "8081" }
  },
  "logging": { "level": "info", "json_mode": false, "file_path": "logs/app.log", "console": false },
  "auth": { "enabled": false, "token": "" }
}
```

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