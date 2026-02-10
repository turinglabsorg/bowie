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

---

## MCP Server Testing with OpenBowie

Use OpenBowie as a test harness to verify that different LLM models can correctly use MCP server tools. This is essential for validating tool descriptions, parameter schemas, and multi-step workflows across models.

### Why Test MCP Servers with Multiple Models?

- Different models interpret tool descriptions differently
- Parameter naming and schema clarity affects model success rates
- Multi-step workflows expose chaining issues
- Testing reveals where tool descriptions need improvement

### Test Structure

Create a test directory inside your MCP server project:

```
<your-mcp>/tests/openbowie/
├── checklist.md              # Master test list with pass/fail criteria
├── prompts/                  # Test case prompt files (one per test)
│   ├── 01-basic-query.txt
│   ├── 02-parameter-test.txt
│   ├── 03-multi-step.txt
│   └── ...
└── results/
    └── <model-name>/         # One folder per model tested
        ├── 01-basic-query/
        │   ├── memory.txt    # Agent's reasoning and output
        │   ├── logs.txt      # Full tool call logs
        │   └── verdict.md    # PASS/PARTIAL/FAIL + notes
        └── summary.md        # Aggregated results
```

### Writing Test Prompts

Prompts should be **natural language** — like a real human would type. Don't tell the model which tools to call or what parameters to use. The test is whether the model can figure that out from tool descriptions.

Principles:
1. **Write like a human** — no tool names, no parameter hints
2. **Start simple** — "What's the config?" before "Set up full lending from scratch"
3. **Include context when needed** — vault addresses, chain names, token names
4. **Gradually increase complexity** — single intent → multi-step workflows

Example prompt files:

```
What chain am I on right now? Is there a wallet configured?
```

```
Deploy a new vault on Arbitrum that uses USDC. Call it "My Vault" with symbol "MV".
```

```
I have a vault at 0x1234...abcd. Set it up for Aave lending with USDC and then supply some.
```

### Running Tests Interactively

Launch tests from the MCP server directory:

```bash
# Launch a single test
TASK_ID=$(openbowie new --headless --config <model> --mcp <your-mcp> "$(cat tests/openbowie/prompts/01-basic-query.txt)")

# Check status
openbowie list

# Read agent output when done
openbowie read $TASK_ID memory

# Read full tool call logs
openbowie logs $TASK_ID

# Save results
mkdir -p tests/openbowie/results/<model>/01-basic-query
openbowie read $TASK_ID memory > tests/openbowie/results/<model>/01-basic-query/memory.txt
openbowie logs $TASK_ID > tests/openbowie/results/<model>/01-basic-query/logs.txt

# Clean up
openbowie rm $TASK_ID
```

### Running Tests in Parallel

```bash
T1=$(openbowie new --headless --config minimax --mcp <your-mcp> "$(cat tests/openbowie/prompts/01-basic-query.txt)")
T2=$(openbowie new --headless --config minimax --mcp <your-mcp> "$(cat tests/openbowie/prompts/02-parameter-test.txt)")
T3=$(openbowie new --headless --config minimax --mcp <your-mcp> "$(cat tests/openbowie/prompts/03-multi-step.txt)")

# Monitor all
openbowie list
```

### Pass/Fail Criteria

- **PASS**: Correct tool(s) called with correct parameters, valid response, structured output matches expected values
- **PARTIAL**: Right tools but wrong parameters, or incomplete/garbled results
- **FAIL**: Wrong tools, hallucinated results, crashed, or timed out

### Iterating on Failures

When a model fails a test:
1. Read `logs.txt` to see exactly what tool calls were made
2. Determine root cause: model issue, tool description issue, or prompt issue
3. If tool descriptions need improvement — update your MCP server code, rebuild, and re-test
4. If the prompt was confusing — rewrite it to be more explicit
5. Re-run only the failing test

### Comparing Models

Run the same test suite against multiple LLM configs:

```bash
openbowie new --headless --config minimax --mcp <your-mcp> "$(cat prompt.txt)"
openbowie new --headless --config openai --mcp <your-mcp> "$(cat prompt.txt)"
openbowie new --headless --config anthropic --mcp <your-mcp> "$(cat prompt.txt)"
```

Build a comparison matrix in `summary.md`:

| Test | MiniMax | GPT-4o | Claude | Qwen |
|------|---------|--------|--------|------|
| 01 | PASS | PASS | PASS | PASS |
| 02 | PASS | PARTIAL | PASS | FAIL |

### Test Categories to Cover

1. **Single-tool read-only** — Can the model call one tool correctly?
2. **Single-tool with parameters** — Can it pass required/optional params?
3. **Multi-step sequential** — Can it chain 3-4 tools in order?
4. **Cross-context** — Can it use output from tool A as input to tool B?
5. **Write operations** — Can it call mutation tools with correct params? (may revert — that's fine, we're testing the call format)
6. **Error handling** — Does it handle tool errors gracefully?
