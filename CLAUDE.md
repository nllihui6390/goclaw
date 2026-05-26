# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

go-claw is a Go-language AI Agent framework inspired by OpenClaw architecture. It implements a Gateway-Agent-Session three-layer decoupled design with multi-channel access (Console, HTTP REST API, WebSocket), tool calling, streaming output, memory persistence, and multi-agent collaboration capabilities.

**Agent routing is manual-only — no automatic keyword or pattern-based routing.** Users must explicitly specify which agent to use via `/agent <name>` command (console) or `agent` field (API). Messages without a specified agent go to the default agent.

## Architecture

The codebase follows a clean layered architecture:

```
main.go                          → Entry point: loads config, creates gateway, registers agents/channels/tools
config/config.go                 → JSON-based configuration (providers, agents, channels, gateway, logging, auth)
internal/gateway/
  gateway.go                     → Gateway core: lifecycle, message routing (manual agent selection), session cleanup
  router.go                      → Manual routing: msg.Agent → agent, fallback to defaultAgent (no keyword auto-routing)
  agent_bus.go                   → Agent-to-agent event bus for inter-agent communication
  config_watcher.go              → fsnotify-based config.json hot-reload
internal/agent/
  agent.go                       → Agent definition: Process(), memory injection, session management
  runtime.go                     → LLM execution loop (blocking Execute + streaming ExecuteStream)
  context.go                     → Session/SessionManager with JSON file persistence
  supervisor.go                  → SupervisorAgent: LLM intent routing → sub-agent dispatch
internal/channel/
  channel.go                     → Channel interface + Message struct (Agent field for manual routing)
  console.go                     → stdin/stdout channel with /agent, /agents, /help commands
  webhook.go                     → HTTP REST API + SSE streaming + Prometheus metrics (agent field + X-Agent header)
  websocket.go                   → WebSocket real-time bidirectional channel
  bot_base.go                    → Shared HTTP server base for bot channels
  lark.go                        → 飞书机器人 (WebSocket client mode, no port needed)
  dingtalk.go                    → 钉钉机器人 (Stream mode, no port needed)
  wecom.go                       → 企业微信机器人 (WebSocket long connection mode, no port needed)
internal/tool/
  tool.go                        → Tool interface (Name/Description/Parameters/Execute)
  registry.go                    → ToolRegistry + Skill groups + GlobalRegistry (plugin pattern)
  weather.go                     → Weather tool (HeFeng, OpenWeather, Seniverse APIs)
  exec.go                        → Shell command execution with safety guards
  file.go                        → File operations (read_file, write_file, edit_file)
  browser.go                     → Browser automation (rod: navigate, click, type, extract, screenshot, scroll, wait)
internal/memory/
  memory.go                      → Memory interface (Store/Retrieve/Consolidate/Forget)
  simple.go                      → In-memory keyword retrieval + persistence backend
  vector.go                      → Embedding-based semantic retrieval (cosine similarity)
internal/store/
  store.go                       → Persistence interface (sessions + memories)
  file.go                        → JSON file-based persistence implementation
internal/middleware/
  auth.go                        → Bearer Token HTTP auth middleware
  rate_limit.go                  → Token bucket rate limiter
pkg/log/
  log.go                         → slog-based structured logging
```

## Commands

```bash
# Build
go build -o go-claw .

# Run (requires OPENAI_API_KEY env var or config.json)
go run .

# With specific config
OPENAI_API_KEY=your-key go run .

# Docker
docker compose up -d
```

## Console Commands

| Command | Description |
|---------|-------------|
| `/agent <name>` | Switch to a specific agent (manual routing) |
| `/agent` | Show current agent |
| `/agents` | List available agents |
| `/help` | Show help |
| `/exit` | Exit |

## API Agent Selection

REST API requests support two ways to specify an agent:
- JSON body: `{"content":"hello", "agent":"weather_agent"}`
- HTTP header: `X-Agent: weather_agent`

If neither is set, the message routes to the default agent.

## Configuration

- Primary config file: `config.json` (in project root)
- Fallback: `getDefaultConfig()` in `main.go` when config file fails to load
- **Multi-provider support**: `providers` section defines multiple LLM providers (OpenAI, DeepSeek, Ollama, etc.), agents reference providers via `provider` field
- Provider types: `openai` (OpenAI-compatible API with Bearer auth), `ollama` (local Ollama `/api/chat`, no auth)
- Per-provider env vars: `PROVIDER_<name>_API_KEY`, `PROVIDER_<name>_BASE_URL` override config.json
- Legacy env vars: `OPENAI_API_KEY`, `OPENAI_BASE_URL` still work (override agent-level api_key/base_url)
- Set `GOCLAW_HOT_RELOAD=true` to enable config.json hot-reload
- `.env.example` provides template for environment variables

## Key Architectural Patterns

- **Gateway**: Central coordinator — manages agent/channel registration, **manual agent routing** (msg.Agent field), session TTL cleanup, and inter-agent event bus. No automatic keyword/channel pattern routing.
- **Agent**: Encapsulates LLM interaction loop with two execution modes — blocking (`Execute`) and streaming (`ExecuteStream` with SSE parsing). Supports multiple provider types via `ProviderType` field: `openai` (OpenAI-compatible) and `ollama` (local Ollama)
- **Provider**: Decoupled model configuration — agents reference providers by name, providers define type/base_url/api_key/default_model. Enables mixing cloud APIs (DeepSeek, OpenAI) with local models (Ollama)
- **Channel**: Pluggable message interface — `channel.go` defines the contract, implemented by console (with `/agent` command), webhook (HTTP REST + SSE + agent field), websocket, and three bot channels: 飞书 (Lark WebSocket client), 钉钉 (DingTalk Stream), 企业微信 (WeCom WebSocket long connection). All bot channels use WebSocket client mode — actively connect to platform servers, no local port needed, no encryption required.
- **Tool**: Plugin pattern — `tool.go` defines the interface, `registry.go` provides dynamic registration + skill grouping. Built-in tools: weather, exec, write_file, read_file, edit_file, browser_use
- **Memory**: Cross-cutting concern with two implementations — `SimpleMemory` (keyword + importance scoring) and `VectorMemory` (embedding + cosine similarity), both backed by `store.FileStore` for JSON persistence
- **Supervisor**: Multi-agent orchestration — LLM-based intent classification routes user messages to specialized sub-agents