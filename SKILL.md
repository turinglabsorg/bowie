# OpenBowie — Claude Code Skill

OpenBowie is an autonomous agent that runs tasks inside Docker containers. You can spin up agents, send them messages, and read their output programmatically.

## Prerequisites

- `openbowie` binary must be built (`make openbowie` in the openbowie repo)
- Docker must be running
- Agent image must be built (`make agent-image`)
- At least one LLM config must exist in `~/.openbowie/configs/` (run `openbowie onboard` to set up)

## Commands

### Create a new task (headless)

```bash
openbowie new --headless --config <config_name> [--mcp <mcp_name>] [--soul <soul_name>] "task description"
```

Starts the agent in the background and prints the task ID to stdout. The agent immediately begins working on the task autonomously.

- `--config`: Name of the LLM config (e.g., `anthropic`, `openai`)
- `--mcp`: Optional MCP server config (e.g., `duckduckgo`, `factor-mcp`)
- `--soul`: Optional soul/persona (default: `default`). Souls live in `~/.openbowie/souls/<name>.md`
- `--headless`: Required for programmatic use — skips the TUI

Example:
```bash
TASK_ID=$(openbowie new --headless --config anthropic --mcp duckduckgo "research the latest AI agent frameworks and summarize findings")
```

### Send a message to a running agent

```bash
openbowie send <task_id> "your message here"
```

Sends a message to a running agent, waits for the response, and prints it to stdout. Tool calls are printed to stderr. Returns when the agent goes idle.

Example:
```bash
openbowie send "$TASK_ID" "now compare the top 3 frameworks by features"
```

### Read task files

```bash
openbowie read <task_id> [status|roadmap|memory|logs]
```

Reads a specific task file. If no file is specified, prints all files.

- `status`: Current task status (short summary)
- `roadmap`: Task plan with steps (checked off as completed)
- `memory`: Conversation transcript and agent notes
- `logs`: Activity log with timestamps and raw tool results

Examples:
```bash
# Read everything
openbowie read "$TASK_ID"

# Check current status
openbowie read "$TASK_ID" status

# See the roadmap
openbowie read "$TASK_ID" roadmap

# Read conversation transcript
openbowie read "$TASK_ID" memory
```

### Other useful commands

```bash
# List all tasks
openbowie list

# Stop a running task
openbowie stop <task_id>

# Remove a task and its data
openbowie rm <task_id>

# Remove all containers and agent image
openbowie clean
```

## Typical workflow

1. Create a task with `--headless` — capture the task ID
2. Wait a few seconds for the agent to start and begin working
3. Poll `openbowie read <id> status` to check progress
4. Send follow-up messages with `openbowie send <id> "message"` if needed
5. Read final results with `openbowie read <id> roadmap` or `openbowie read <id> memory`
6. Stop and clean up with `openbowie stop <id>` and `openbowie rm <id>`

## Notes

- The agent starts working immediately after creation — no need to send a first message
- The agent is autonomous: it creates a roadmap, executes steps, and recovers from errors on its own
- `openbowie send` blocks until the agent finishes responding (up to 5 min timeout)
- Task files are persisted on disk at `~/.openbowie/tasks/task_<id>/`
- If the agent's container stops, `openbowie send` will return an error — use `openbowie restart <id>` to restart it
