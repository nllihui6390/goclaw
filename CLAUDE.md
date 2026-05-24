# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

go-claw is a Go-language AI Agent framework inspired by OpenClaw architecture. It implements a Gateway-Agent-Session three-layer decoupled design with multi-channel access and tool calling capabilities.

## Architecture

The codebase follows a clean layered architecture:

```
main.go              → Entry point: loads config, creates gateway, registers agents/channels/routes
config/config.go     → JSON-based configuration (agents, channels, gateway settings)
internal/gateway/    → Gateway core lifecycle management and message routing
internal/agent/      → Agent implementation with runtime execution and session context
internal/channel/    → Channel abstractions: console (stdin/stdout) and webhook (HTTP API)
internal/tool/       → Tool interface and implementations (weather, exec)
```

**Key architectural patterns:**

- **Gateway** acts as the central coordinator — manages agent registration, channel registration, and message routing via `gateway.go` and `router.go`
- **Agent** encapsulates the LLM interaction loop — `agent.go` defines the agent, `runtime.go` handles the execution loop, `context.go` manages session context
- **Channel** is an interface for message ingress — `channel.go` defines the contract, `console.go` implements stdin/stdout, `webhook.go` implements HTTP webhook
- **Tool** follows a plugin pattern — `tool.go` defines the interface, individual tools (`weather.go`, `exec.go`) implement it

## Commands

```bash
# Build
go build -o go-claw .

# Run (requires OPENAI_API_KEY env var or config.json)
go run .

# With specific config
OPENAI_API_KEY=your-key go run .
```

There are no tests or linting configured yet.

## Configuration

- Primary config file: `config.json` (in project root)
- Fallback: `getDefaultConfig()` in `main.go` when config file fails to load
- API keys should come from environment variables (`OPENAI_API_KEY`), not hardcoded

## Important Notes

- The project uses OpenAI-compatible API format (configurable `base_url` and `model`), not provider-locked
- Route matching supports channel-pattern matching and keyword-based routing
- Tools are registered by name string in config, resolved via switch statement in `main.go`'s `loadTools()`
- The webhook channel runs an HTTP server on the configured port (default 8080)
