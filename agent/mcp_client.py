import asyncio
import json
import os
import subprocess

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

from agent import protocol


class MCPManager:
    def __init__(self, mcp_config: dict):
        self.config = mcp_config
        self.name = mcp_config.get("name", "mcp")
        self.session: ClientSession | None = None
        self._stdio_context = None
        self._session_context = None
        self._tools: list = []

    async def connect(self):
        # Run install command if specified (e.g. "pip install duckduckgo-mcp-server")
        install = self.config.get("install")
        if install:
            protocol.send("status", state="thinking", detail=f"Installing MCP dependencies: {install}")
            try:
                subprocess.run(install, shell=True, check=True, capture_output=True, text=True)
            except subprocess.CalledProcessError as e:
                protocol.send("error", content=f"MCP install failed: {e.stderr}")

        command = self.config["command"]
        args = self.config.get("args", [])
        env_vars = {**os.environ, **self.config.get("env", {})}

        server_params = StdioServerParameters(
            command=command,
            args=args,
            env=env_vars,
        )

        self._stdio_context = stdio_client(server_params)
        read_stream, write_stream = await self._stdio_context.__aenter__()

        self._session_context = ClientSession(read_stream, write_stream)
        self.session = await self._session_context.__aenter__()

        await self.session.initialize()

        result = await self.session.list_tools()
        self._tools = result.tools
        protocol.send("status", state="idle", detail=f"MCP '{self.name}' connected, {len(self._tools)} tools")

    async def disconnect(self):
        if self._session_context:
            try:
                await self._session_context.__aexit__(None, None, None)
            except Exception:
                pass
        if self._stdio_context:
            try:
                await self._stdio_context.__aexit__(None, None, None)
            except Exception:
                pass
        self.session = None

    async def reconnect(self):
        protocol.send("status", state="thinking", detail=f"Reconnecting MCP '{self.name}'...")
        await self.disconnect()
        try:
            await self.connect()
        except Exception as e:
            protocol.send("error", content=f"MCP reconnect failed: {e}")

    def get_tools_for_llm(self) -> list[dict]:
        tools = []
        for tool in self._tools:
            schema = tool.inputSchema if tool.inputSchema else {"type": "object", "properties": {}}
            tools.append({
                "type": "function",
                "function": {
                    "name": tool.name,
                    "description": tool.description or "",
                    "parameters": schema,
                },
            })
        return tools

    def has_tool(self, name: str) -> bool:
        return any(t.name == name for t in self._tools)

    async def call_tool(self, name: str, args: dict) -> str:
        if not self.session:
            return "Error: MCP not connected"
        try:
            result = await self.session.call_tool(name, args)
            parts = []
            for item in result.content:
                if hasattr(item, "text"):
                    parts.append(item.text)
                else:
                    parts.append(str(item))
            return "\n".join(parts)
        except Exception as e:
            protocol.send("error", content=f"MCP tool '{name}' failed: {e}")
            await self.reconnect()
            try:
                result = await self.session.call_tool(name, args)
                parts = []
                for item in result.content:
                    if hasattr(item, "text"):
                        parts.append(item.text)
                    else:
                        parts.append(str(item))
                return "\n".join(parts)
            except Exception as e2:
                return f"Error calling tool '{name}' after reconnect: {e2}"
