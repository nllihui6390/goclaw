# go-claw 项目任务列表

> 最后更新: 2026-05-28

## ✅ 已完成功能

### 核心系统
- [x] Agent 系统 - 多 Agent 支持，独立工作空间
- [x] 工具注册表 - 全局工具加载与创建
- [x] 内存系统 - SimpleMemory, VectorMemory, Dream
- [x] 存储系统 - FileStore, DialogStore
- [x] 工作空间加载器 - Workspace Loader

### Cron 定时任务
- [x] Cron Manager - 定时任务管理器
- [x] Job 调度 - 支持 cron 表达式、@every 间隔、HH:MM 格式
- [x] 活跃时段控制 - ActiveStart/ActiveEnd
- [x] 两种任务类型 - text (发送消息) / agent (调用 Agent)
- [x] cron_status 工具 - AI 可查询任务状态

### 渠道集成
- [x] Console - 控制台交互
- [x] Webhook - HTTP 回调
- [x] WebSocket - WebSocket 服务
- [x] 飞书 (Lark) - WebSocket 长连接
- [x] 钉钉 (DingTalk) - Stream 模式
- [x] 企业微信 (WeCom) - WebSocket 长连接

### 工具集
- [x] weather - 天气查询 (和风 API)
- [x] file - 文件读写编辑
- [x] exec - 命令执行
- [x] browser_use - 浏览器自动化
- [x] cron_status - Cron 状态查询
- [x] time - 时间工具
- [x] 多 Agent 协作 - chat_with_agent, submit_to_agent, list_agents

### 安全与中间件
- [x] 认证中间件 - Token 验证
- [x] 限流中间件 - Rate Limiting
- [x] 工具安全守卫 - Shell 注入、敏感路径、浏览器规则

### 高级功能
- [x] MCP 集成 - Model Context Protocol
- [x] ACP 协议 - Agent Communication Protocol
- [x] Skill 系统 - Prompt-based 技能，热加载
- [x] 主动模式 - Proactive Manager
- [x] 会话清理 - TTL 过期管理

## 📋 当前 Cron 任务配置

| 任务名 | 调度 | 类型 | Agent | 活跃时段 |
|--------|------|------|-------|----------|
| 每日问候 | `0 9 * * *` | text | - | - |
| 每小时检查 | `0 * * * *` | agent | default | - |
| 晚间总结 | `0 21 * * *` | text | - | 21:00-23:59 |

## 🚧 待优化 / 待完善

### 高优先级
- [ ] `ExecuteText` 实现 - 当前 gatewayCronExecutor.ExecuteText 是空实现
- [ ] 任务持久化 - 重启后任务丢失，需保存到文件
- [ ] 任务 CRUD API - 运行时增删改查任务

### 中优先级
- [ ] 任务执行日志 - 记录每次执行的输入输出
- [ ] 失败重试机制 - 任务失败时自动重试
- [ ] 并发控制 - 限制同时执行的任务数
- [ ] 任务分组 - 按组管理任务

### 低优先级
- [ ] Web UI - 管理 Cron 任务
- [ ] 通知渠道配置 - 任务失败时发送通知
- [ ] 任务依赖 - 支持任务间依赖关系
- [ ] 执行超时配置 - 不同任务不同超时时间

## 🐛 已知问题

1. **ExecuteText 空实现** - `main.go` 第 438-441 行，文本任务不会真正发送消息
2. **SessionID 未传递** - Cron Job 的 SessionID 未在配置中支持
3. **Job ID 缺失** - 添加任务时未生成唯一 ID

## 📝 配置参考

```json
{
  "cron": {
    "enabled": true,
    "jobs": [
      {
        "name": "任务名称",
        "schedule": "0 9 * * *",
        "type": "agent|text",
        "agent_name": "default",
        "content": "任务内容",
        "active_start": "09:00",
        "active_end": "18:00"
      }
    ]
  }
}
```

### Schedule 格式支持
- `@every 5m` - 每 5 分钟
- `@every 1h` - 每 1 小时
- `09:00` - 每天 9:00
- `0 9 * * *` - 标准 cron (分 时 日 月 周)