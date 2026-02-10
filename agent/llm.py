import os

import litellm


SUMMARIZE_PROMPT = """You are a tool result compressor for an AI agent. Compress the tool result below into a SHORT version.

RULES:
1. If the result contains "simulationHint" or "scriptContent", preserve that JSON block EXACTLY — copy it character-for-character
2. Preserve error codes, error types, and messages exactly
3. Preserve all Ethereum addresses and amounts exactly
4. For lists with more than 3 items, show only the first 2 and note the count
5. Remove verbose descriptions and duplicate explanations
6. Keep your output under {target} characters
7. Output raw content (no markdown wrapping, no ```json blocks)

Tool: {tool_name}
Result:
{result}"""


class LLM:
    def __init__(self, config: dict):
        self.provider = config["provider"]
        self.model_name = config["model"]
        self.base_url = config.get("base_url")
        self._setup_env(config)
        self.model = self._build_model_string()

    def _setup_env(self, config: dict):
        if self.provider == "anthropic":
            os.environ["ANTHROPIC_API_KEY"] = config["api_key"]
        elif self.provider == "openai":
            os.environ["OPENAI_API_KEY"] = config["api_key"]
        elif self.provider == "openrouter":
            os.environ["OPENROUTER_API_KEY"] = config["api_key"]
        elif self.provider == "ollama":
            endpoint = config.get("endpoint", "http://localhost:11434")
            os.environ["OLLAMA_API_BASE"] = endpoint

        if self.base_url:
            os.environ["OPENAI_API_KEY"] = config.get("api_key", "")
            os.environ["OPENAI_API_BASE"] = self.base_url

    def _build_model_string(self) -> str:
        if self.base_url:
            return f"openai/{self.model_name}"
        prefixes = {
            "anthropic": "anthropic/",
            "openai": "openai/",
            "openrouter": "openrouter/",
            "ollama": "ollama_chat/",
        }
        prefix = prefixes.get(self.provider, "")
        return f"{prefix}{self.model_name}"

    async def completion(self, messages: list[dict], tools: list[dict] | None = None) -> dict:
        kwargs = {
            "model": self.model,
            "messages": messages,
        }
        if tools:
            kwargs["tools"] = tools
        response = await litellm.acompletion(**kwargs)
        return response

    async def summarize_tool_result(self, tool_name: str, result: str, target_chars: int = 2000, timeout: float = 30.0) -> str:
        """Use a quick LLM call to summarize a large tool result.

        Acts as a subagent: reads the full result and returns a concise
        version that preserves actionable data (hints, errors, addresses)
        while compressing verbose content (long lists, descriptions).

        Has a timeout to prevent hanging if the API is slow.
        """
        import asyncio

        prompt = SUMMARIZE_PROMPT.format(
            target=target_chars,
            tool_name=tool_name,
            result=result,
        )
        messages = [{"role": "user", "content": prompt}]
        response = await asyncio.wait_for(
            litellm.acompletion(model=self.model, messages=messages),
            timeout=timeout,
        )
        return response.choices[0].message.content or result
