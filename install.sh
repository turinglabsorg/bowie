#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
SKILL_DIR="$HOME/.claude/skills/bowie"

echo "=== Bowie Installer ==="
echo ""

# ── Dependency checks ──────────────────────────────────────────────

check_cmd() {
    if ! command -v "$1" &>/dev/null; then
        echo "ERROR: $1 is not installed."
        echo "  $2"
        exit 1
    fi
    echo "  $1 ... $(command -v "$1")"
}

echo "Checking dependencies..."
check_cmd go      "Install Go 1.25+: https://go.dev/dl/"
check_cmd docker  "Install Docker: https://docs.docker.com/get-docker/"
check_cmd make    "Install make via Xcode CLI tools: xcode-select --install"

# Check Go version (need 1.25+)
GO_VERSION=$(go version | grep -oE '[0-9]+\.[0-9]+' | head -1)
GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)
if [ "$GO_MAJOR" -lt 1 ] || ([ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 25 ]); then
    echo "ERROR: Go $GO_VERSION found, but 1.25+ is required."
    echo "  Update Go: https://go.dev/dl/"
    exit 1
fi
echo "  go version ... $GO_VERSION"

# Check Docker is running
if ! docker info &>/dev/null; then
    echo "ERROR: Docker is not running."
    echo "  Start Docker Desktop or the Docker daemon and try again."
    exit 1
fi
echo "  docker daemon ... running"
echo ""

# ── Build ──────────────────────────────────────────────────────────

echo "Building bowie binary..."
cd "$REPO_DIR"
make bowie
echo ""

# ── Install binary ─────────────────────────────────────────────────

INSTALL_DIR="$HOME/.local/bin"
if [ -w /usr/local/bin ]; then
    INSTALL_DIR="/usr/local/bin"
fi
mkdir -p "$INSTALL_DIR"
cp "$REPO_DIR/bowie" "$INSTALL_DIR/bowie"
echo "Installed binary at $INSTALL_DIR/bowie"

if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    echo ""
    echo "  WARNING: $INSTALL_DIR is not in your PATH."
    echo "  Add to your shell profile: export PATH=\"$INSTALL_DIR:\$PATH\""
    echo ""
fi

# ── Build Docker image ─────────────────────────────────────────────

echo "Building agent Docker image..."
make agent-image
echo ""

# ── Install Claude Code skill ──────────────────────────────────────

if [ -d "$HOME/.claude" ]; then
    echo "Installing Claude Code skill..."
    mkdir -p "$SKILL_DIR"
    cat > "$SKILL_DIR/SKILL.md" << 'SKILLEOF'
---
name: bowie
description: Launch an autonomous AI agent in a Docker container to perform research, DeFi operations, or any task using MCP servers. Use when the user wants to delegate a task to an autonomous agent, or mentions bowie.
allowed-tools: Bash, Read
argument-hint: <task description>
---

# Bowie — Autonomous Agent Skill

Bowie runs autonomous AI agents inside Docker containers. Each agent gets its own isolated environment with Python, Node.js, Rust, and Foundry. Agents connect to LLM providers and optionally MCP servers to execute tasks.

## Available LLM configs

Check `~/.bowie/configs/` for available configs. Run `bowie onboard` if none exist.

## Available MCP configs

Check `~/.bowie/mcps/` for available MCP servers.

## Commands

### Create a new task (headless — required for programmatic use)

```bash
TASK_ID=$(bowie new --headless --config <config_name> [--mcp <mcp_name>] [--soul <soul_name>] "task description")
```

- `--config`: LLM config name (required). Check `ls ~/.bowie/configs/` for options.
- `--mcp`: Optional MCP server (e.g., `duckduckgo`, `factor-mcp`)
- `--soul`: Optional persona (default: `default`). Check `ls ~/.bowie/souls/`
- Always use `--headless` when calling from Claude Code

### Send a follow-up message

```bash
bowie send <task_id> "your message"
```

Blocks until the agent responds (up to 5 min). Response on stdout, tool calls on stderr.

### Read task files

```bash
bowie read <task_id> status    # current progress
bowie read <task_id> roadmap   # task plan with checkboxes
bowie read <task_id> memory    # conversation transcript
bowie read <task_id> logs      # full activity log with tool results
bowie read <task_id>           # all files
```

### Other commands

```bash
bowie list                     # list all tasks
bowie stop <task_id>           # stop a running task
bowie rm <task_id>             # remove task and data
bowie clean                    # remove all containers and image
```

## Typical workflow

1. Check available configs: `ls ~/.bowie/configs/`
2. Launch: `TASK_ID=$(bowie new --headless --config <cfg> [--mcp <mcp>] "task")`
3. Wait ~15-30 seconds for the agent to start working
4. Poll: `bowie read "$TASK_ID" status`
5. Follow up: `bowie send "$TASK_ID" "additional instructions"`
6. Read results: `bowie read "$TASK_ID" memory`
7. Clean up: `bowie stop "$TASK_ID" && bowie rm "$TASK_ID"`

## Important notes

- The agent starts working immediately — no need to send a first message
- Always capture the task ID from `bowie new --headless`
- Use `bowie read <id> status` to check progress before sending follow-ups
- Task data persists at `~/.bowie/tasks/task_<id>/`
SKILLEOF
    echo "  Skill installed at $SKILL_DIR/SKILL.md"
else
    echo "Claude Code not detected (~/.claude/ not found). Skipping skill install."
    echo "  Run this installer again after installing Claude Code to add the skill."
fi

echo ""
echo "=== Installation complete ==="
echo ""
echo "  Binary:       $INSTALL_DIR/bowie"
echo "  Docker image: bowie-agent:latest"
echo "  Configs:      ~/.bowie/"
if [ -d "$SKILL_DIR" ]; then
echo "  Claude skill: $SKILL_DIR/SKILL.md"
fi
echo ""
echo "Next steps:"
echo "  1. Run 'bowie onboard' to configure LLM providers and MCP servers"
echo "  2. Run 'bowie new --config <name> \"your task\"' to start your first agent"
echo ""
