import os
import tomllib
from pathlib import Path
import tomli_w

CONFIG_DIR = Path(os.getenv("SHADOWCAST_CONFIG_DIR", Path.home() / ".config" / "shadowcast"))
CONFIG_FILE = CONFIG_DIR / "config.toml"

DEFAULT_CONFIG = {
    "server_url": "http://localhost:8000",
    "api_token": "",
    "default_workspace": "default",
}

def load_config() -> dict:
    if not CONFIG_FILE.exists():
        return dict(DEFAULT_CONFIG)
    try:
        data = tomllib.loads(CONFIG_FILE.read_text())
        merged = dict(DEFAULT_CONFIG)
        merged.update(data)
        return merged
    except Exception:
        return dict(DEFAULT_CONFIG)

def save_config(server_url: str, api_token: str, default_workspace: str = "default") -> Path:
    CONFIG_DIR.mkdir(parents=True, exist_ok=True)
    payload = {
        "server_url": server_url.rstrip("/"),
        "api_token": api_token.strip(),
        "default_workspace": default_workspace.strip() or "default",
    }
    CONFIG_FILE.write_text(tomli_w.dumps(payload))
    os.chmod(CONFIG_FILE, 0o600)
    return CONFIG_FILE
