import os

import litellm


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
