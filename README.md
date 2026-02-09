# OpenBowie

> *"I don't know where I'm going from here, but I promise it won't be boring."*

An autonomous agent that tests MCP servers against LLM providers. You give it a task, an LLM config, and optionally an MCP server — it spins up a dockerized agent, connects to everything, and gets to work. The agent analyzes the task, builds a roadmap, and starts executing without waiting for instructions.

Think of it as a scratchpad for poking at MCP tools with different models. It turns and faces the strange.

## Prerequisites

- **Go 1.25+**
- **Docker** (running)
- An API key for at least one LLM provider (Anthropic, OpenAI, OpenRouter, etc.)

## Quick Start

> *"Tomorrow belongs to those who can hear it coming."*

```bash
# Build the CLI
make openbowie

# Build the agent Docker image (includes Python, Node.js, Rust, Foundry)
make agent-image

# Run the setup wizard
./openbowie onboard
```

The onboard wizard walks you through configuring your LLM provider(s), souls, and optionally any MCP servers. It writes config files to `~/.openbowie/`.

Once configured:

```bash
# Start a task with an MCP
./openbowie new --config anthropic --mcp duckduckgo "search for the latest AI agent frameworks"

# Start a task without an MCP (just the LLM + internal tools)
./openbowie new --config anthropic "help me plan a project"

# Start with a custom soul
./openbowie new --config anthropic --soul researcher "deep dive into quantum computing"
```

The agent starts immediately — it analyzes the task, creates a roadmap, and begins executing autonomously.

## Headless Mode

> *"I'm just an individual who doesn't feel that I need to have somebody qualify my work in any particular way."*

For programmatic use (e.g., from Claude Code or scripts):

```bash
# Start a task headless — prints task ID and exits
TASK_ID=$(./openbowie new --headless --config anthropic --mcp factor-mcp "deploy a vault on Arbitrum")

# Send a follow-up message and get the response
./openbowie send "$TASK_ID" "now add an Aave adapter to the vault"

# Read task files
./openbowie read "$TASK_ID" status    # current status
./openbowie read "$TASK_ID" roadmap   # task plan
./openbowie read "$TASK_ID" memory    # conversation transcript
./openbowie read "$TASK_ID" logs      # full activity log with tool results
```

## TUI Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `ctrl+o` | Toggle tool result details (collapsed by default) |
| `ctrl+c` | Quit |
| `esc` | Back to task list |
| `enter` | Send message / select task |
| `enter enter` | Interrupt the agent |

## Configuration

### LLM Config (`~/.openbowie/configs/<name>.json`)

```json
{
  "provider": "anthropic",
  "api_key": "sk-ant-...",
  "model": "claude-sonnet-4-5-20250929"
}
```

Supported providers: `anthropic`, `openai`, `openrouter`, `ollama`, or anything OpenAI-compatible via `base_url`:

```json
{
  "provider": "minimax",
  "api_key": "your-key",
  "base_url": "https://api.minimax.io/anthropic/v1",
  "model": "your-model"
}
```

For Ollama, use `endpoint` instead:

```json
{
  "provider": "ollama",
  "endpoint": "http://host.docker.internal:11434",
  "model": "llama3.1"
}
```

### MCP Config (`~/.openbowie/mcps/<name>.json`)

Basic MCP (npm package, auto-installed via npx):

```json
{
  "name": "my-mcp",
  "command": "npx",
  "args": ["-y", "some-mcp-package"]
}
```

Python MCP with install step:

```json
{
  "name": "duckduckgo",
  "command": "python",
  "args": ["-m", "duckduckgo_mcp_server.server"],
  "install": "pip install duckduckgo-mcp-server"
}
```

Git-based MCP with build step:

```json
{
  "name": "factor-mcp",
  "command": "node",
  "args": ["/bowie/cache/factor-mcp/dist/index.js"],
  "install": "if [ ! -f /bowie/cache/factor-mcp/dist/index.js ]; then git clone https://github.com/factorDAO/factor-mcp.git /bowie/cache/factor-mcp && cd /bowie/cache/factor-mcp && npm install && npm run build; fi",
  "env": {
    "ALCHEMY_API_KEY": "your-key",
    "DEFAULT_CHAIN": "ARBITRUM_ONE"
  }
}
```

The `install` field runs once before the MCP server starts. Use `/bowie/cache/` for persistence across runs.

### Souls (`~/.openbowie/souls/<name>.md`)

> *"I always had a repulsive need to be something more than human."*

Souls define the agent's personality and directives. A `default` soul ships out of the box (autonomous, proactive). Create custom ones to change how the agent thinks:

```bash
# Use during onboard
./openbowie onboard

# Or drop a markdown file directly
echo "Be concise. Think step by step. Always verify before acting." > ~/.openbowie/souls/careful.md

# Then use it
./openbowie new --config anthropic --soul careful "audit this smart contract"
```

## Commands

| Command | What it does |
|---------|-------------|
| `openbowie onboard` | Interactive setup wizard |
| `openbowie new --config <cfg> [--mcp <mcp>] [--soul <soul>] "desc"` | Create a task and start the agent |
| `openbowie new --headless ...` | Start a task without TUI (prints task ID) |
| `openbowie list` | List all tasks and their status |
| `openbowie attach <task_id>` | Attach to a running (or stopped) task |
| `openbowie send <task_id> "msg"` | Send a message and print the response |
| `openbowie read <task_id> [file]` | Read task files (status/roadmap/memory/logs) |
| `openbowie stop <task_id>` | Stop a running task |
| `openbowie restart <task_id>` | Restart a task |
| `openbowie logs <task_id>` | Show task activity logs |
| `openbowie rm <task_id>` | Remove a task and its data |
| `openbowie clean` | Remove all containers and the agent image |

Running `openbowie` with no arguments opens an interactive task list.

## How It Works

> *"The truth is of course is that there is no journey. We are arriving and departing all at the same time."*

1. The Go CLI manages tasks, configs, and Docker containers
2. Each task runs in an isolated Docker container with Python, Node.js, Rust, and Foundry
3. The containerized agent uses [litellm](https://github.com/BerriAI/litellm) to talk to your LLM
4. MCP servers are launched inside the container via stdio
5. On startup, the agent autonomously analyzes the task, creates a roadmap, and starts executing
6. The agent manages its own state files (status, memory, roadmap, logs) and persists them across restarts
7. You interact through a terminal UI — tool results are collapsed by default (ctrl+o to expand)

## Agent Behavior

> *"I'm not a prophet or a stone aged man, just a mortal with potential of a superman."*

OpenBowie is designed to be autonomous. When you create a task:

- It immediately analyzes the task and creates a roadmap
- It executes steps without asking for permission
- It makes reasonable assumptions and documents them in memory
- It only asks you a question if completely blocked
- It recovers from errors on its own (retries, alternative approaches)
- It keeps you informed with progress updates, not questions

## Project Structure

```
cmd/bobby/         CLI entry point (Cobra commands)
internal/
  config/          Config loading/saving (~/.openbowie/)
  docker/          Docker container management
  onboard/         Interactive setup wizard
  task/            Task CRUD and file management
  tui/             Terminal UI (Bubble Tea)
agent/             Python agent (runs in Docker)
  agent.py         Main agent loop
  llm.py           LLM abstraction (litellm)
  mcp_client.py    MCP client
  protocol.py      JSON IPC with the CLI
  files.py         Task file management
```

## Development

```bash
make openbowie      # Build the CLI
make agent-image    # Build the Docker image
make test           # Run all tests
make test-go        # Go tests only
make test-agent     # Python tests only
```

---

> *"I don't know where I'm going from here, but I promise it won't be boring."*
> — David Bowie
