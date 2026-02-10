import os
import tempfile
from datetime import datetime, timezone


class TaskFiles:
    def __init__(self, task_dir: str = "/bowie/task"):
        self.task_dir = task_dir

    def _path(self, name: str) -> str:
        return os.path.join(self.task_dir, name)

    def read(self, name: str) -> str:
        path = self._path(name)
        if not os.path.exists(path):
            return ""
        with open(path) as f:
            return f.read()

    def write(self, name: str, content: str):
        path = self._path(name)
        dir_name = os.path.dirname(path)
        fd, tmp = tempfile.mkstemp(dir=dir_name, suffix=".tmp")
        try:
            with os.fdopen(fd, "w") as f:
                f.write(content)
            os.replace(tmp, path)
        except Exception:
            try:
                os.unlink(tmp)
            except OSError:
                pass
            raise

    def append(self, name: str, content: str):
        existing = self.read(name)
        self.write(name, existing + content)

    def context(self, memory_max_chars: int = 0) -> str:
        parts = []
        for name, label in [
            ("status.md", "Current Status"),
            ("memory.md", "Memory / Notes"),
            ("roadmap.md", "Roadmap"),
        ]:
            text = self.read(name)
            if not text.strip():
                continue
            # Cap memory.md to avoid unbounded context growth
            if name == "memory.md" and memory_max_chars > 0 and len(text) > memory_max_chars:
                # Keep the end (most recent notes) rather than the beginning
                text = "[earlier notes omitted]\n\n" + text[-memory_max_chars:]
            parts.append(f"## {label}\n{text.strip()}")
        return "\n\n".join(parts)

    def log(self, entry: str):
        ts = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC")
        self.append("logs.md", f"[{ts}] {entry}\n")
