import asyncio
import json
import sys

from agent.agent import Agent


def load_json(path: str) -> dict:
    with open(path) as f:
        return json.load(f)


def load_text(path: str) -> str:
    with open(path) as f:
        return f.read()


def main():
    task_cfg = load_json("/bowie/task/task.json")
    llm_cfg = load_json("/bowie/config/config.json")

    mcp_cfg = None
    try:
        mcp_cfg = load_json("/bowie/mcp/mcp.json")
    except FileNotFoundError:
        pass

    soul = ""
    try:
        soul = load_text("/bowie/soul/soul.md")
    except FileNotFoundError:
        pass

    agent = Agent(task_cfg, llm_cfg, mcp_cfg, soul=soul)
    asyncio.run(agent.run())


if __name__ == "__main__":
    main()
