# go-claw.exe 使用说明

## 基本信息

| 项目 | 说明 |
|------|------|
| 文件名 | go-claw.exe |
| 大小 | 约 16MB |
| 平台 | Windows 64位 |
| 语言 | Go 语言 AI Agent 框架 |

## 快速启动

```bash
# 1. 首次运行（无 config.json 时）
./go-claw.exe

# 程序会启动配置向导，引导你设置：
#   - LLM 供应商（OpenAI / DeepSeek / Ollama / Anthropic / 自定义）
#   - API Key
#   - 模型选择
#   - 可选渠道（HTTP API、飞书、钉钉、企业微信）

# 2. 后续运行（已有 config.json）
./go-claw.exe
```

## 配置文件

**config.json** (程序根目录，首次运行后自动生成)

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
    "model": "deepseek-chat",
    "tools": ["weather", "exec", "write_file", "read_file", "edit_file"]
  }],
  "logging": {
    "level": "info",
    "file_path": "logs/app.log",
    "console": false
  }
}
```

## 环境变量

| 变量 | 说明 |
|------|------|
| `PROVIDER_<name>_API_KEY` | 供应商 API Key（如 PROVIDER_DEEPSEEK_API_KEY） |
| `PROVIDER_<name>_BASE_URL` | 供应商 API 地址 |
| `HEFENG_API_KEY` | 和风天气 API Key |
| `GOCLAW_HOT_RELOAD` | 配置热加载（设为 true 启用） |

## 控制台命令

进入控制台对话后可用：

| 命令 | 说明 |
|------|------|
| `/agent <name>` | 切换 Agent |
| `/agent` | 显示当前 Agent |
| `/agents` | 列出可用 Agent |
| `/help` | 显示帮助 |
| `/exit` | 退出程序 |

## HTTP API 端点

启用 webhook 渠道后（默认端口 8080）：

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/chat` | 发送消息（支持 agent 字段、SSE 流式） |
| GET | `/api/v1/sessions` | 列出会话 |
| DELETE | `/api/v1/sessions/{id}` | 删除会话 |
| GET | `/health` | 健康检查 |
| GET | `/metrics` | Prometheus 指标 |

## 内置工具

| 工具 | 说明 |
|------|------|
| `weather` | 天气查询 |
| `exec` | Shell 命令执行 |
| `write_file` | 写文件 |
| `read_file` | 读文件 |
| `edit_file` | 编辑文件（精确替换） |
| `append_file` | 追加文件 |
| `send_file` | 发送文件给用户 |
| `browser_use` | 浏览器自动化 |
| `get_current_time` | 获取当前时间 |
| `set_user_timezone` | 设置时区 |
| `cron_status` | 查看定时任务状态 |
| `list_agents` | 列出 Agent |
| `chat_with_agent` | 与其他 Agent 对话 |

## 数据目录

运行后自动创建 `goclaw-data/` 目录：

```
goclaw-data/
├── skills/              # 全局技能
├── inbox.json           # 事件通知
└── workspaces/
    └── default/
        ├── AGENTS.md    # 行为规则
        ├── SOUL.md      # 人格
        ├── PROFILE.md   # 用户偏好
        ├── MEMORY.md    # 长期记忆
        ├── sessions/    # 会话数据
        ├── skills/      # Agent 专属技能
        └── cache/       # 工具结果缓存
```

## Skill 技能系统

Prompt-based 模式：
1. 系统提示词注入技能信息（名称、描述、SKILL.md 路径）
2. AI 用 read_file 读取完整 SKILL.md
3. AI 理解如何使用技能
4. AI 用 exec 工具执行脚本

技能目录结构：
```
goclaw-data/skills/
└── my-skill/
    ├── SKILL.md        # 技能描述
    └── scripts/
        └── run.sh      # 执行脚本
```

## 机器人渠道配置

| 渠道 | 配置字段 | 连接模式 |
|------|----------|----------|
| 飞书 | app_id, app_secret | WebSocket 客户端 |
| 钉钉 | client_id, client_secret | Stream 模式 |
| 企业微信 | bot_id, secret | WebSocket 长连接 |

## 定时任务

```json
"cron": {
  "enabled": true,
  "jobs": [{
    "name": "每日提醒",
    "schedule": "09:00",
    "type": "agent",
    "agent_name": "default",
    "agent_prompt": "提醒用户今日待办"
  }]
}
```

## 日志系统

- 默认关闭控制台输出，日志写入 `logs/app-YYYY-MM-DD.log`
- 在 config.json 中设置 `"console": true` 可同时输出到控制台
- 日志按日自动分割，跨天无缝切换

## 常见问题

**Q: API Key 如何设置？**
A: 两种方式：
1. 在 config.json 中填写 api_key 字段
2. 通过环境变量 PROVIDER_<name>_API_KEY

**Q: 如何查看日志？**
A: 查看 logs/app-YYYY-MM-DD.log 文件，或在 config.json 中设置 console: true

**Q: 如何添加新 Agent？**
A: 在 config.json 的 agents 数组中添加配置

**Q: 首次运行没有 config.json？**
A: 程序自动启动配置向导，完成后生成 config.json

**Q: 如何重新配置？**
A: 删除 config.json，再次运行即可进入配置向导