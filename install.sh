#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
SKILL_DIR="$HOME/.claude/skills/openbowie"

echo "=== OpenBowie Installer ==="
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

echo "Building openbowie binary..."
cd "$REPO_DIR"
make openbowie
echo ""

# ── Install binary ─────────────────────────────────────────────────

INSTALL_DIR="$HOME/.local/bin"
if [ -w /usr/local/bin ]; then
    INSTALL_DIR="/usr/local/bin"
fi
mkdir -p "$INSTALL_DIR"
cp "$REPO_DIR/openbowie" "$INSTALL_DIR/openbowie"
echo "Installed binary at $INSTALL_DIR/openbowie"

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
name: openbowie
description: Launch an autonomous AI agent in a Docker container to perform research, DeFi operations, or any task using MCP servers. Use when the user wants to delegate a task to an autonomous agent, or mentions openbowie.
allowed-tools: Bash, Read
argument-hint: <task description>
---

# OpenBowie — Autonomous Agent Skill

OpenBowie runs autonomous AI agents inside Docker containers. Each agent gets its own isolated environment with Python, Node.js, Rust, and Foundry. Agents connect to LLM providers and optionally MCP servers to execute tasks.

## Available LLM configs

Check `~/.openbowie/configs/` for available configs. Run `openbowie onboard` if none exist.

## Available MCP configs

Check `~/.openbowie/mcps/` for available MCP servers.

## Commands

### Create a new task (headless — required for programmatic use)

```bash
TASK_ID=$(openbowie new --headless --config <config_name> [--mcp <mcp_name>] [--soul <soul_name>] "task description")
```

- `--config`: LLM config name (required). Check `ls ~/.openbowie/configs/` for options.
- `--mcp`: Optional MCP server (e.g., `duckduckgo`, `factor-mcp`)
- `--soul`: Optional persona (default: `default`). Check `ls ~/.openbowie/souls/`
- Always use `--headless` when calling from Claude Code

### Send a follow-up message

```bash
openbowie send <task_id> "your message"
```

Blocks until the agent responds (up to 5 min). Response on stdout, tool calls on stderr.

### Read task files

```bash
openbowie read <task_id> status    # current progress
openbowie read <task_id> roadmap   # task plan with checkboxes
openbowie read <task_id> memory    # conversation transcript
openbowie read <task_id> logs      # full activity log with tool results
openbowie read <task_id>           # all files
```

### Other commands

```bash
openbowie list                     # list all tasks
openbowie stop <task_id>           # stop a running task
openbowie rm <task_id>             # remove task and data
openbowie clean                    # remove all containers and image
```

## Typical workflow

1. Check available configs: `ls ~/.openbowie/configs/`
2. Launch: `TASK_ID=$(openbowie new --headless --config <cfg> [--mcp <mcp>] "task")`
3. Wait ~15-30 seconds for the agent to start working
4. Poll: `openbowie read "$TASK_ID" status`
5. Follow up: `openbowie send "$TASK_ID" "additional instructions"`
6. Read results: `openbowie read "$TASK_ID" memory`
7. Clean up: `openbowie stop "$TASK_ID" && openbowie rm "$TASK_ID"`

## Important notes

- The agent starts working immediately — no need to send a first message
- Always capture the task ID from `openbowie new --headless`
- Use `openbowie read <id> status` to check progress before sending follow-ups
- Task data persists at `~/.openbowie/tasks/task_<id>/`
SKILLEOF
    echo "  Skill installed at $SKILL_DIR/SKILL.md"
else
    echo "Claude Code not detected (~/.claude/ not found). Skipping skill install."
    echo "  Run this installer again after installing Claude Code to add the skill."
fi

echo ""
echo "=== Installation complete ==="
echo ""
echo "  Binary:       $INSTALL_DIR/openbowie"
echo "  Docker image: openbowie-agent:latest"
echo "  Configs:      ~/.openbowie/"
if [ -d "$SKILL_DIR" ]; then
echo "  Claude skill: $SKILL_DIR/SKILL.md"
fi
echo ""
echo "Next steps:"
echo "  1. Run 'openbowie onboard' to configure LLM providers and MCP servers"
echo "  2. Run 'openbowie new --config <name> \"your task\"' to start your first agent"
echo ""
