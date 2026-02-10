import asyncio
import json

from agent import protocol
from agent.files import TaskFiles
from agent.llm import LLM
from agent.mcp_client import MCPManager

MAX_TOOL_ROUNDS = 25
MAX_MESSAGES = 50
KEEP_FIRST = 2
KEEP_LAST = 40

# Defaults for context management (can be overridden per-model in llm config)
DEFAULT_SUMMARIZE_THRESHOLD = 3000   # Summarize results larger than this
DEFAULT_SUMMARIZE_TARGET = 2000      # Target summary size
DEFAULT_FALLBACK_MAX_CHARS = 8000    # Fallback truncation limit
DEFAULT_FALLBACK_KEEP_HEAD = 3000
DEFAULT_FALLBACK_KEEP_TAIL = 3000
DEFAULT_MEMORY_MAX_CHARS = 3000      # Memory.md cap in system prompt

BASE_PROMPT = """You are Bowie, an autonomous task-focused AI agent. You may have access to tools from a connected MCP server and internal tools for managing your task state.

{soul}

## Task files

- status.md: Current task status (update as you progress)
- memory.md: Notes, context, and assumptions to persist across sessions
- roadmap.md: Steps/plan for the task (check off steps as you complete them)
- logs.md: Activity log (auto-appended)

## Internal tools

- update_status: Update status.md with current state
- update_memory: Update memory.md with notes to persist
- update_roadmap: Update roadmap.md with task plan

Always update your task files as you work so you can resume if restarted."""

DEFAULT_SOUL = """## Directives

- Be maximally autonomous. Do not ask the user for clarification — make reasonable assumptions and proceed.
- If something is ambiguous, pick the most sensible option and move forward. Document your assumptions in memory.md.
- Only ask the user a question if you are completely blocked and cannot make progress any other way.
- Execute tools proactively. If you have the tools to accomplish a step, just do it.
- When you hit an error, try to recover on your own: retry with different parameters, try an alternative approach, or skip and move to the next step.
- Keep the user informed with short progress updates, not questions."""

INTERNAL_TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "update_status",
            "description": "Update the status.md file with current task status",
            "parameters": {
                "type": "object",
                "properties": {"content": {"type": "string", "description": "New status content"}},
                "required": ["content"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "update_memory",
            "description": "Update memory.md with notes to persist across sessions",
            "parameters": {
                "type": "object",
                "properties": {"content": {"type": "string", "description": "New memory content"}},
                "required": ["content"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "update_roadmap",
            "description": "Update roadmap.md with task plan and steps",
            "parameters": {
                "type": "object",
                "properties": {"content": {"type": "string", "description": "New roadmap content"}},
                "required": ["content"],
            },
        },
    },
]


class Agent:
    def __init__(self, task_cfg: dict, llm_cfg: dict, mcp_cfg: dict | None, soul: str = ""):
        self.task_cfg = task_cfg
        self.soul = soul.strip() if soul else ""
        self.files = TaskFiles()
        self.llm = LLM(llm_cfg)
        self.mcp = MCPManager(mcp_cfg) if mcp_cfg else None
        self.messages: list[dict] = []
        self._interrupted = False
        self._turn_task: asyncio.Task | None = None

        # Context management settings (configurable per model via llm config)
        ctx = llm_cfg.get("context", {})
        self.summarize_threshold = ctx.get("summarize_threshold", DEFAULT_SUMMARIZE_THRESHOLD)
        self.summarize_target = ctx.get("summarize_target", DEFAULT_SUMMARIZE_TARGET)
        self.fallback_max_chars = ctx.get("fallback_max_chars", DEFAULT_FALLBACK_MAX_CHARS)
        self.fallback_keep_head = ctx.get("fallback_keep_head", DEFAULT_FALLBACK_KEEP_HEAD)
        self.fallback_keep_tail = ctx.get("fallback_keep_tail", DEFAULT_FALLBACK_KEEP_TAIL)
        self.memory_max_chars = ctx.get("memory_max_chars", DEFAULT_MEMORY_MAX_CHARS)

    def _build_system(self) -> str:
        ctx = self.files.context(memory_max_chars=self.memory_max_chars)
        task_desc = self.task_cfg.get("description", "")
        soul_content = self.soul if self.soul else DEFAULT_SOUL
        system_prompt = BASE_PROMPT.format(soul=soul_content)
        parts = [system_prompt, f"\n## Task Description\n{task_desc}"]
        if ctx:
            parts.append(f"\n## Task Context\n{ctx}")
        return "\n".join(parts)

    def _truncate_messages(self):
        if len(self.messages) > MAX_MESSAGES:
            self.messages = self.messages[:KEEP_FIRST] + self.messages[-KEEP_LAST:]

    def _compact_result(self, result: str) -> str:
        """Fallback truncation for when subagent summarization fails.

        Keeps the head and tail of the result so the model sees the structure
        (beginning) and the actionable info (end, where hints/errors usually are).
        """
        if len(result) <= self.fallback_max_chars:
            return result
        head = result[:self.fallback_keep_head]
        tail = result[-self.fallback_keep_tail:]
        omitted = len(result) - self.fallback_keep_head - self.fallback_keep_tail
        return f"{head}\n\n[... {omitted} characters omitted — see logs.md for full output ...]\n\n{tail}"

    async def _summarize_result(self, tool_name: str, result: str) -> str:
        """Compress a tool result using a subagent LLM call.

        For results exceeding SUMMARIZE_THRESHOLD, spawns a quick LLM call
        that reads the full result and returns a concise version preserving
        actionable data. Falls back to head/tail truncation on failure.

        Results containing structured hints (simulationHint, scriptContent)
        are never summarized — an LLM cannot reliably reproduce hex blobs
        and pre-built calldata. These pass through as-is.
        """
        if len(result) <= self.summarize_threshold:
            return result
        # Never summarize results with actionable hints that contain
        # pre-built data (hex calldata, scripts) — LLMs can't copy those exactly
        if "simulationHint" in result or "scriptContent" in result:
            self.files.log(f"Skipping summarization for {tool_name} (contains hints)")
            return result
        try:
            self.files.log(f"Summarizing {tool_name} result ({len(result)} chars)")
            summary = await self.llm.summarize_tool_result(
                tool_name, result, target_chars=self.summarize_target,
            )
            if summary and len(summary) < len(result):
                self.files.log(f"Summarized {tool_name}: {len(result)} → {len(summary)} chars")
                return summary
        except Exception as e:
            self.files.log(f"Summarization failed for {tool_name}: {e}")
        return self._compact_result(result)

    def _write_transcript(self):
        """Write the conversation transcript to memory.md."""
        lines = []
        for msg in self.messages:
            role = msg.get("role", "")
            content = msg.get("content", "")
            if role == "user":
                lines.append(f"**User:** {content}\n")
            elif role == "assistant":
                if content and content.strip():
                    lines.append(f"**Bowie:** {content.strip()}\n")
                tool_calls = msg.get("tool_calls")
                if tool_calls:
                    for tc in tool_calls:
                        fn = tc.get("function", {})
                        lines.append(f"  → tool: {fn.get('name', '?')}\n")
        self.files.write("memory.md", "\n".join(lines))

    def _get_tools(self) -> list[dict]:
        mcp_tools = self.mcp.get_tools_for_llm() if self.mcp else []
        return INTERNAL_TOOLS + mcp_tools

    async def _handle_internal_tool(self, name: str, args: dict) -> str:
        if name == "update_status":
            self.files.write("status.md", args["content"])
            self.files.log(f"Status updated")
            protocol.send("tool_result", tool="status", content=args["content"])
            return "Status updated."
        elif name == "update_memory":
            self.files.write("memory.md", args["content"])
            self.files.log(f"Memory updated")
            protocol.send("tool_result", tool="memory", content=args["content"])
            return "Memory updated."
        elif name == "update_roadmap":
            self.files.write("roadmap.md", args["content"])
            self.files.log(f"Roadmap updated")
            protocol.send("tool_result", tool="roadmap", content=args["content"])
            return "Roadmap updated."
        return f"Unknown internal tool: {name}"

    async def _handle_tool_calls(self, tool_calls: list) -> list[dict]:
        results = []
        for tc in tool_calls:
            tc_id = getattr(tc, "id", None) or (tc.get("id") if isinstance(tc, dict) else "unknown")

            if self._interrupted:
                results.append({
                    "role": "tool",
                    "tool_call_id": tc_id,
                    "content": "Interrupted by user.",
                })
                continue

            fn = getattr(tc, "function", None) or (tc.get("function") if isinstance(tc, dict) else None)
            if fn is None:
                self.files.log(f"Skipping tool call with missing function: {tc}")
                results.append({
                    "role": "tool",
                    "tool_call_id": tc_id,
                    "content": "Error: malformed tool call (missing function).",
                })
                continue

            name = getattr(fn, "name", None) or (fn.get("name") if isinstance(fn, dict) else "unknown")
            raw_args = getattr(fn, "arguments", None) or (fn.get("arguments") if isinstance(fn, dict) else "{}")
            try:
                args = json.loads(raw_args) if raw_args else {}
            except (json.JSONDecodeError, TypeError):
                args = {}

            protocol.send("tool_call", tool=name, args=args)
            self.files.log(f"Tool call: {name}")

            if name in ("update_status", "update_memory", "update_roadmap"):
                result = await self._handle_internal_tool(name, args)
            elif self.mcp and self.mcp.has_tool(name):
                result = await self.mcp.call_tool(name, args)
                protocol.send("tool_result", tool=name, content=result)
            else:
                result = f"Unknown tool: {name}"
                protocol.send("tool_result", tool=name, content=result)

            # Full result goes to logs; summarized version goes to LLM messages
            self.files.log(f"Tool result [{name}]: {result}")
            compacted = await self._summarize_result(name, result)

            results.append({
                "role": "tool",
                "tool_call_id": tc_id,
                "content": compacted,
            })
        self._write_transcript()
        return results

    def _parse_response(self, response):
        """Safely extract message from an LLM response.

        Returns (content, tool_calls) or raises ValueError with a
        descriptive message if the response is malformed.
        """
        choices = getattr(response, "choices", None)
        if not choices:
            raise ValueError(f"Empty or missing choices in response: {response}")
        msg = getattr(choices[0], "message", None)
        if msg is None:
            raise ValueError(f"Missing message in response choice: {choices[0]}")
        content = getattr(msg, "content", None) or ""
        tool_calls = getattr(msg, "tool_calls", None)
        # Normalise tool_calls: some providers return [] instead of None
        if tool_calls is not None and len(tool_calls) == 0:
            tool_calls = None
        return content, tool_calls

    async def _llm_turn(self):
        protocol.send("status", state="thinking")
        system = self._build_system()
        tools = self._get_tools()

        full_messages = [{"role": "system", "content": system}] + self.messages

        retries = 0
        max_retries = 2

        for _ in range(MAX_TOOL_ROUNDS):
            if self._interrupted:
                break

            response = await self.llm.completion(full_messages, tools=tools if tools else None)

            try:
                content, tool_calls = self._parse_response(response)
            except ValueError as e:
                retries += 1
                self.files.log(f"Malformed LLM response (attempt {retries}/{max_retries}): {e}")
                if retries >= max_retries:
                    self.files.log("Max retries for malformed responses reached, stopping turn")
                    protocol.send("error", content=f"LLM returned malformed responses after {max_retries} retries")
                    break
                await asyncio.sleep(1)
                continue

            retries = 0

            assistant_msg = {"role": "assistant", "content": content}
            if tool_calls:
                serialized_tcs = []
                for tc in tool_calls:
                    fn = getattr(tc, "function", None)
                    if fn is None:
                        continue
                    serialized_tcs.append({
                        "id": getattr(tc, "id", "unknown"),
                        "type": "function",
                        "function": {
                            "name": getattr(fn, "name", "unknown"),
                            "arguments": getattr(fn, "arguments", "{}"),
                        },
                    })
                if serialized_tcs:
                    assistant_msg["tool_calls"] = serialized_tcs
                else:
                    tool_calls = None

            full_messages.append(assistant_msg)
            self.messages.append(assistant_msg)

            if not tool_calls:
                if content:
                    protocol.send("agent_response", content=content)
                break

            # Send intermediate text if the LLM wrote something before calling tools
            if content:
                protocol.send("agent_response", content=content)

            if self._interrupted:
                break

            tool_results = await self._handle_tool_calls(tool_calls)
            full_messages.extend(tool_results)
            self.messages.extend(tool_results)

        self._truncate_messages()
        protocol.send("status", state="idle")

    async def _safe_llm_turn(self):
        try:
            await self._llm_turn()
        except asyncio.CancelledError:
            protocol.send("status", state="idle")
        except Exception as e:
            protocol.send("error", content=f"LLM error: {e}")
            self.files.log(f"Error: {e}")
            protocol.send("status", state="idle")
        finally:
            self._write_transcript()

    async def _cancel_turn(self):
        """Cancel the currently running LLM turn if any."""
        if self._turn_task and not self._turn_task.done():
            self._interrupted = True
            self._turn_task.cancel()
            try:
                await self._turn_task
            except asyncio.CancelledError:
                pass
            self._turn_task = None

    async def _start_turn(self):
        """Start a new LLM turn as a background task."""
        self._interrupted = False
        self._turn_task = asyncio.create_task(self._safe_llm_turn())

    async def run(self):
        if self.mcp:
            try:
                await self.mcp.connect()
            except Exception as e:
                protocol.send("error", content=f"Failed to connect MCP: {e}")

        self.files.log("Agent started")

        # Auto-start: analyze the task and execute autonomously
        desc = self.task_cfg.get("description", "")
        self.messages.append({
            "role": "user",
            "content": f"New task: {desc}\n\nAnalyze this task, create a roadmap using update_roadmap, set the status using update_status, and start executing immediately. Work through the steps autonomously — do not ask me questions, just make progress and report what you did.",
        })
        await self._start_turn()

        # Main loop: listen for user input concurrently with LLM turns
        # Keep recv_task alive across iterations — cancelling a
        # run_in_executor(readline) only cancels the Future, not the
        # underlying thread, so a new recv would race the old one for
        # stdin data and messages would be silently lost.
        recv_task = asyncio.ensure_future(protocol.recv())

        while True:
            wait_set = {recv_task}
            if self._turn_task and not self._turn_task.done():
                wait_set.add(self._turn_task)

            done, _ = await asyncio.wait(wait_set, return_when=asyncio.FIRST_COMPLETED)

            if recv_task in done:
                msg = recv_task.result()
                if msg is None:
                    await self._cancel_turn()
                    break
                if msg.get("type") == "shutdown":
                    self.files.log("Agent shutting down")
                    await self._cancel_turn()
                    break

                # Message consumed — start a fresh recv for next iteration
                recv_task = asyncio.ensure_future(protocol.recv())

                if msg.get("type") == "interrupt":
                    await self._cancel_turn()
                    self.files.log("Interrupted by user")
                    protocol.send("agent_response", content="Interrupted. What would you like me to do?")
                    protocol.send("status", state="idle")
                    continue
                if msg.get("type") == "user_message":
                    content = msg.get("content", "")
                    await self._cancel_turn()
                    self.messages.append({"role": "user", "content": content})
                    self._write_transcript()
                    self.files.log("User message received")
                    await self._start_turn()
                    continue

            # Clean up finished turn
            if self._turn_task and self._turn_task.done():
                self._turn_task = None

        if self.mcp:
            await self.mcp.disconnect()
        self.files.log("Agent stopped")
