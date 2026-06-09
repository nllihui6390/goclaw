# go-claw

Go 语言 AI Agent 框架，实现 Gateway-Agent-Session 三层解耦架构。支持多模型供应商、多渠道接入、工具调用、流式输出、记忆持久化、多 Agent 协作。

## 特性一览

| 分类 | 能力 |
|------|------|
| **架构** | Gateway-Agent-Session 三层解耦、手动 Agent 路由 |
| **模型** | OpenAI/DeepSeek 等云 API + 本地 Ollama，多供应商配置 |
| **渠道** | Console、Web 管理后台、飞书、钉钉、企业微信、微信个人 (iLink) |
| **前端** | Vue 3 + Element Plus，14 个管理页面，深色/亮色主题 |
| **桌面** | Wails3 编译独立 EXE，Go 函数直接调用，无 HTTP 端口 |
| **运行时** | Auto-continue、Summarizing、API 重试、智能终止、推理标签剥离 |
| **上下文** | 自动压缩、工具结果裁剪、Token 预算管理 |
| **记忆** | 短期/长期记忆、关键词/向量检索、自动提取、每日记忆、Dream 优化 |
| **工具** | 24 个内置工具、插件模式、动态注册、安全守卫 |
| **技能** | Prompt-based 技能系统、全局 + Agent 专属两层、热加载 |
| **协作** | 多 Agent 交互、监督者模式、Mission 自主任务、Plan 任务规划 |
| **扩展** | MCP 客户端、ACP 外部 Agent 协议、定时任务 Cron |
| **运维** | Docker 部署、配置热加载、Prometheus 指标、日志按日分割 |

## 快速开始

```bash
# 1. 构建前端
cd frontend && npm install && npm run build && cd ..

# 2. 配置 API Key（编辑 config.json 或设置环境变量）

# 3. 构建并运行
go build -tags server -o go-claw-server.exe .
./go-claw-server.exe    # → http://localhost:8080
```

### 开发模式

```bash
# 终端 1: Go 后端
go run .

# 终端 2: 前端热重载
cd frontend && npm run dev    # → http://localhost:5173
```

### 构建模式

| 命令 | 产物 | 说明 |
|------|------|------|
| `go build -tags server` | `go-claw-server.exe` (~20MB) | Web 服务，HTTP API + 前端 SPA |
| `wails3 build` | `bin/go-claw.exe` (~34MB) | 桌面应用，WebView2 窗口 |

  ┌─────────────────────────┬────────┬─────────────────────────────────────────┐
  │        编译命令         │  前端  │                  说明                   │
  ├─────────────────────────┼────────┼─────────────────────────────────────────┤
  │ go build -tags server . │ 不嵌入 │ 生产环境用，前端由 nginx/caddy 等 serve │
  ├─────────────────────────┼────────┼─────────────────────────────────────────┤
  │ go build .              │ 嵌入   │ 开发/桌面端用，前端打包进二进制         │
  └─────────────────────────┴────────┴─────────────────────────────────────────┘

  原理：
  - embed.go 有 //go:build !server — 带 -tags server 时被排除
  - embed_release.go 有 //go:build server — 带 -tags server 时启用（空的 initFrontend）


### 桌面应用 (Wails3)

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
wails3 dev      # 开发模式（前端热重载 + WebView2 窗口）
wails3 build   # 构建 → bin/go-claw.exe
```

前端自动检测环境，桌面模式通过 Wails Bridge 直接调用 Go 函数，Web 模式使用 HTTP + SSE。

## 控制台命令

| 命令 | 说明 |
|------|------|
| `/agent <name>` | 切换到指定 Agent |
| `/agent` | 显示当前 Agent |
| `/agents` | 列出可用 Agent |
| `/help` | 显示帮助 |
| `/exit` | 退出 |

## 机器人渠道

| 渠道 | 连接方式 | 配置字段 |
|------|----------|----------|
| 飞书 | WebSocket 长连接 | `app_id`, `app_secret` |
| 钉钉 | WebSocket Stream | `client_id`, `client_secret` |
| 企业微信 | WebSocket 长连接 | `bot_id`, `secret` |
| 微信个人 | HTTP 长轮询 (iLink Bot) | `bot_token`（留空扫码登录） |

飞书、钉钉、企业微信为 **WebSocket 客户端模式** — 主动连接对方服务器，无需本地开端口。

### 显示控制

每个渠道支持 3 个开关：

| 配置项 | 说明 |
|-------|------|
| `show_tool_messages` | 显示工具调用和输出 |
| `show_thinking` | 显示模型思考/推理内容 |
| `stream_output` | 流式输出 |

Console 默认全开，Bot 渠道默认关闭工具和思考显示。

### 文件发送

`send_file` 工具支持直接发送文件：

| 渠道 | 方式 | 限制 |
|------|------|------|
| 企业微信 | WebSocket 分片上传 | 单片 ≤512KB，最大 20MB |
| 钉钉 | HTTP API 上传 | - |
| 飞书 | HTTP API 上传 | - |
| 微信 iLink | CDN 加密上传 | AES-ECB 加密 |

## 内置工具

### 默认工具（自动加载）

| 工具 | 说明 |
|------|------|
| `cron_status` | 定时任务管理 |
| `get_current_time` | 当前日期时间 |
| `system_info` | 系统信息 |
| `http_request` | HTTP 请求 |
| `web_search` | 网页搜索 |
| `url_summary` | 网页正文提取 |
| `calculate` | 数学计算 |
| `run_code` | 运行 Python/JS 代码 |
| `list_files` | 目录文件列表 |
| `read_pdf` | PDF 读取 |
| `ocr_image` | 图片 OCR |
| `generate_image` | AI 图片生成 |
| `network_check` | 网络检测 |
| `database_query` | SQLite 查询 |
| `manage_config` | 配置读写 |

### 可选工具（需声明）

| 工具 | 说明 |
|------|------|
| `weather` | 天气查询 |
| `execute_command` | Shell 命令执行 |
| `read_file` | 读文件 |
| `write_file` | 写文件 |
| `edit_file` | 编辑文件 |
| `append_file` | 追加文件 |
| `send_file` | 发送文件给用户 |
| `browser_use` | 浏览器自动化 (rod) |
| `set_user_timezone` | 设置时区 |
| `list_agents` | 列出 Agent |
| `chat_with_agent` | 与 Agent 对话 |
| `submit_to_agent` | 提交后台任务 |

## Agent 运行时

### Auto-continue
模型暗示要使用工具但没实际调用时，自动注入提示继续，最多 3 次。

### Summarizing
达到 `max_iterations` 上限时，调用模型不带工具优雅总结。

### API 重试
429/500-504 错误自动重试 3 次，指数退避（1s → 2s → 4s，上限 30s）。

### 上下文压缩
接近 `max_tokens` 阈值（默认 80%）时，自动压缩旧消息为摘要。

### 工具结果裁剪
超长结果自动截断，完整内容保存到 `cache/` 目录。

### 自动记忆提取
对话后异步提取关键信息，存入长期记忆和每日记忆文件。

### 主动模式
空闲超过 `idle_minutes` 后，分析记忆主动发送提醒或建议。

### Dream 优化
空闲时整理记忆：去重、合并、清理过期信息。

## 工作空间

每个 Agent 有独立工作空间 `goclaw-data/workspaces/<agent-name>/`：

| 文件 | 用途 | 注入 |
|------|------|------|
| AGENTS.md | 行为规则、安全指南 | ✅ |
| SOUL.md | 核心人格、价值观 | ✅ |
| PROFILE.md | 用户身份、偏好 | ✅ |
| MEMORY.md | 长期记忆 | ❌ 工具访问 |
| HEARTBEAT.md | 周期任务提示 | ❌ |
| BOOTSTRAP.md | 首次引导 | ✅ 首次对话 |

## Skill 技能系统

Prompt-based 技能系统，技能描述注入系统提示词，AI 读取 SKILL.md 后用工具执行。

### 两层架构

| 层级 | 目录 |
|------|------|
| 全局技能 | `goclaw-data/skills/` |
| Agent 专属 | `goclaw-data/workspaces/<agent>/skills/` |

同名技能 Agent 专属版优先，目录通过 fsnotify 热加载。

### SKILL.md 格式

```markdown
---
name: weather-query
description: 查询城市天气
metadata:
  openclaw:
    emoji: "🌤️"
    requires:
      bins: [curl]
---

## 核心能力
- 查询任意城市天气

## 执行步骤
1. 确认城市名称 {{city}}
2. 调用 weather 工具查询
```

## 多 Agent 协作

| 工具 | 说明 |
|------|------|
| `list_agents` | 列出所有 Agent |
| `chat_with_agent` | 与指定 Agent 对话 |
| `submit_to_agent` | 提交后台任务 |
| `check_agent_task` | 查询任务结果 |

## 定时任务

支持三种调度格式：
- `@every 5m` — 每 5 分钟
- `HH:MM` — 每天 HH:MM
- 标准 cron 表达式

支持活跃时段控制（如仅 09:00-18:00 执行）。

## 安全守卫

三层机制：
- Shell 注入检测（`$(cmd)`、`|cmd`、`;cmd`、`rm -rf /`）
- 文件访问保护（`.env`、`credentials`、`/etc/passwd`）
- 规则引擎（deny/guard/approve）

## 扩展协议

### MCP (Model Context Protocol)
连接外部工具服务，支持 stdio 和 SSE 模式。

### ACP (Agent Communication Protocol)
启动和管理 Claude Code、Codex 等外部 Agent 进程。

## Web 管理后台

Vue 3 + Element Plus，14 个页面：

| 路由 | 页面 | 状态 |
|------|------|------|
| `/` | 对话 | ✅ |
| `/channels` | 渠道管理 | ✅ |
| `/sessions` | 会话管理 | ✅ |
| `/cron-jobs` | 定时任务 | ✅ |
| `/agent-config` | Agent 配置 | ✅ |
| `/skills` | 技能管理 | ✅ |
| `/files` | 文件管理 | ✅ |
| `/tools` | 工具管理 | ✅ |
| `/models` | 模型配置 | ✅ |
| `/debug` | 调试日志 | ✅ |
| `/inbox` | 事件通知 | 开发中 |
| `/workspace` | 工作空间 | 开发中 |
| `/mcp` | MCP 配置 | 开发中 |
| `/security` | 安全规则 | 开发中 |

支持深色/亮色主题切换。

## API 端点

### 对话
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/chat` | 发送消息（支持 SSE） |

### 管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET/PUT | `/api/v1/agents`, `/api/v1/agents/:name` | Agent 管理 |
| GET/PUT | `/api/v1/channels`, `/api/v1/channels/:name` | 渠道管理 |
| GET | `/api/v1/providers` | 供应商列表 |
| GET | `/api/v1/tools` | 工具列表 |
| GET | `/api/v1/skills/*` | 技能管理 |
| GET/POST/PUT/DELETE | `/api/v1/cron/jobs/*` | 定时任务 |
| GET/PUT | `/api/v1/config` | 配置管理 |
| GET | `/api/v1/logs`, `/api/v1/status` | 日志/状态 |

### 其他
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查 |
| GET | `/metrics` | Prometheus 指标 |

## 项目结构

```
go-claw/
├── main.go                    # Web 入口 (//go:build !production)
├── main_desktop.go            # 桌面入口 (//go:build production)
├── embed.go                   # 前端静态文件嵌入
├── config.json                # 配置文件
├── config/config.go           # 配置管理
├── server/                    # HTTP 服务
│   ├── server.go              # 路由 + 中间件
│   ├── frontend.go            # SPA 服务
│   ├── controllers/           # API 处理器
│   └── service/               # 业务服务
├── frontend/                  # Vue 3 前端
│   └── src/
│       ├── views/             # 14 个页面
│       ├── components/        # 组件
│       └── stores/            # Pinia 状态
├── internal/
│   ├── gateway/               # 网关层
│   ├── agent/                 # Agent 层
│   ├── channel/               # 渠道层
│   ├── tool/                  # 工具层
│   ├── skill/                 # 技能层
│   ├── memory/                # 记忆层
│   ├── store/                 # 持久化
│   ├── workspace/             # 工作空间
│   ├── proactive/             # 主动模式
│   ├── inbox/                 # 事件通知
│   ├── cron/                  # 定时任务
│   ├── plan/                  # 任务规划
│   ├── multiagent/            # 多 Agent
│   ├── security/              # 安全守卫
│   ├── mission/               # 自主任务
│   ├── acp/                   # ACP 协议
│   ├── mcp/                   # MCP 客户端
│   ├── ratelimiter/           # 限流器
│   └── middleware/            # HTTP 中间件
├── goclaw-data/               # 数据目录
│   ├── skills/                # 全局技能
│   └── workspaces/            # Agent 工作空间
├── logs/                      # 日志目录
├── Dockerfile
├── docker-compose.yml
└── go.mod
```

## 配置示例

```json
{
  "gateway": {
    "default_agent": "default",
    "session_ttl": 60,
    "data_dir": "goclaw-data"
  },
  "providers": {
    "deepseek": {
      "type": "openai",
      "base_url": "https://api.deepseek.com/v1",
      "api_key": "your-api-key",
      "default_model": "deepseek-chat"
    }
  },
  "agents": [{
    "name": "default",
    "provider": "deepseek",
    "model": "deepseek-v4-oc",
    "system_prompt": "你是一个有用的AI助手。",
    "tools": ["weather", "execute_command", "read_file", "write_file"],
    "max_iterations": 20,
    "max_tokens": 32000
  }],
  "channels": {
    "console": { "enabled": true, "stream_output": true },
    "wecom": { "enabled": false, "bot_id": "", "secret": "" }
  },
  "logging": { "level": "info", "file_path": "logs/app.log" },
  "skills": { "enabled": true },
  "proactive": { "enabled": false, "idle_minutes": 30 }
}
```

## 环境变量

| 变量 | 说明 |
|------|------|
| `PROVIDER_<name>_API_KEY` | 供应商 API Key |
| `PROVIDER_<name>_BASE_URL` | 供应商 API 地址 |
| `OPENAI_API_KEY` | 兼容旧配置 |
| `HEFENG_API_KEY` | 和风天气 API Key |
| `GOCLAW_HOT_RELOAD` | 启用配置热加载 |

## Docker

```bash
docker compose up -d
```

端口：8080 (HTTP)、8081 (WebSocket)