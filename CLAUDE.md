# Bowie

Autonomous agent system for testing MCP servers against LLM providers.

## Architecture

- **Go CLI** (`cmd/bowie/main.go`): Cobra commands + Bubble Tea TUI
- **Python agent** (`agent/`): Runs inside Docker, uses litellm for LLM abstraction
- **Docker**: Each task runs in an isolated container with Python, Node.js, Rust, Foundry
- **MCP**: Servers launched via stdio inside the container
- **IPC**: JSON over stdin/stdout between Go CLI and Python agent

## Key paths

- `cmd/bowie/main.go` — CLI entry point, all commands
- `agent/agent.py` — Core agent loop, system prompt, tool handling
- `agent/protocol.py` — JSON IPC (send/recv over stdin/stdout)
- `agent/files.py` — Task file management (status, memory, roadmap, logs)
- `internal/docker/docker.go` — Container lifecycle management
- `internal/config/config.go` — Config loading/saving (~/.bowie/)
- `internal/tui/tui.go` — Terminal UI (Bubble Tea)
- `internal/onboard/onboard.go` — Setup wizard
- `internal/task/task.go` — Task CRUD

## Config layout

```
~/.bowie/
  configs/<name>.json    # LLM provider configs
  mcps/<name>.json       # MCP server configs
  souls/<name>.md        # Agent personality/directives
  tasks/task_<id>/       # Task data (status.md, memory.md, roadmap.md, logs.md)
  cache/                 # Shared cache across containers
```

## Build & test

```bash
make bowie       # Build CLI
make agent-image     # Build Docker image
make test            # Run all tests
```

## Docker labels & naming

- Container label: `bowie.task.id`
- Container name: `bowie-<task_id>`
- Image: `bowie-agent:latest`
- Mount paths: `/bowie/task`, `/bowie/config/`, `/bowie/cache`, `/bowie/soul/`, `/bowie/mcp/`

## Go module

Module path is `github.com/turinglabs/bobby` (historical, not renamed to avoid import churn).
