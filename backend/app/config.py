import os
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Settings:
    dashscope_api_key: str
    qwen_workspace_id: str
    qwen_model: str
    data_dir: Path
    max_pages: int
    retention_hours: int
    max_long_edge: int
    qwen_timeout_seconds: int
    max_upload_bytes: int = 20 * 1024 * 1024


def load_settings() -> Settings:
    _load_dotenv(Path(".env"))
    data_dir = Path(os.environ.get("DATA_DIR", "data"))
    return Settings(
        dashscope_api_key=os.environ.get("DASHSCOPE_API_KEY", ""),
        qwen_workspace_id=os.environ.get("QWEN_WORKSPACE_ID", ""),
        qwen_model=os.environ.get("QWEN_MODEL", "qwen-image-3.0-pro"),
        data_dir=data_dir,
        max_pages=int(os.environ.get("MAX_PAGES", "20")),
        retention_hours=int(os.environ.get("RETENTION_HOURS", "24")),
        max_long_edge=int(os.environ.get("MAX_LONG_EDGE", "2048")),
        qwen_timeout_seconds=int(os.environ.get("QWEN_TIMEOUT_SECONDS", "180")),
    )


def _load_dotenv(path: Path) -> None:
    if not path.is_file():
        return
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or "=" not in stripped:
            continue
        key, value = stripped.split("=", 1)
        os.environ.setdefault(key.strip(), value.strip().strip("'\""))
