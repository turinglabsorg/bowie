import asyncio
import json
import sys


def send(msg_type: str, **kwargs):
    msg = {"type": msg_type, **kwargs}
    line = json.dumps(msg, ensure_ascii=False)
    sys.stdout.write(line + "\n")
    sys.stdout.flush()


async def recv() -> dict | None:
    loop = asyncio.get_event_loop()
    line = await loop.run_in_executor(None, sys.stdin.readline)
    if not line:
        return None
    line = line.strip()
    if not line:
        return None
    return json.loads(line)
