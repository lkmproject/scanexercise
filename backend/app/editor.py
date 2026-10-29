import base64
import io
import re
from typing import Protocol

import httpx
from PIL import Image

from app.prompt import PROMPT

_MAX_PIXELS = 2048 * 2048
_MIN_PIXELS = 512 * 512
_WORKSPACE_ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")


class EditorError(Exception):
    pass


class ImageEditor(Protocol):
    def edit(self, jpeg: bytes) -> bytes: ...


class QwenImageEditor:
    def __init__(
        self,
        api_key: str,
        workspace_id: str,
        model: str,
        timeout_seconds: int,
        client: httpx.Client | None = None,
    ) -> None:
        self._api_key = api_key
        self._workspace_id = workspace_id
        self._model = model
        self._timeout_seconds = timeout_seconds
        self._client = client

    def edit(self, jpeg: bytes) -> bytes:
        if not self._api_key:
            raise EditorError("尚未設定 DASHSCOPE_API_KEY")
        if _WORKSPACE_ID.fullmatch(self._workspace_id) is None:
            raise EditorError("尚未設定 QWEN_WORKSPACE_ID")
        with Image.open(io.BytesIO(jpeg)) as source:
            size = output_size(source.width, source.height)
        payload = {
            "model": self._model,
            "prompt": PROMPT,
            "image": "data:image/jpeg;base64," + base64.b64encode(jpeg).decode("ascii"),
            "size": size,
            "n": 1,
            "prompt_extend": False,
            "watermark": False,
        }
        url = (
            f"https://{self._workspace_id}.cn-hongkong.maas.aliyuncs.com"
            "/compatible-mode/v1/images/generations"
        )
        client = self._client or httpx.Client()
        try:
            try:
                response = client.post(
                    url,
                    headers={"Authorization": f"Bearer {self._api_key}"},
                    json=payload,
                    timeout=self._timeout_seconds,
                )
            except httpx.TimeoutException as exc:
                raise EditorError("模型逾時") from exc
            except httpx.HTTPError as exc:
                raise EditorError("模型呼叫失敗") from exc
            if response.status_code >= 400:
                raise EditorError("模型呼叫失敗")
            image_url = _image_url(response.json())
            try:
                downloaded = client.get(image_url, timeout=self._timeout_seconds)
            except httpx.TimeoutException as exc:
                raise EditorError("模型逾時") from exc
            except httpx.HTTPError as exc:
                raise EditorError("模型呼叫失敗") from exc
        finally:
            if self._client is None:
                client.close()
        if downloaded.status_code >= 400 or not downloaded.content:
            raise EditorError("模型沒有回傳圖像")
        try:
            with Image.open(io.BytesIO(downloaded.content)) as image:
                return _to_jpeg(image)
        except Exception as exc:
            raise EditorError("模型沒有回傳圖像") from exc


def output_size(width: int, height: int) -> str:
    if width < 1 or height < 1:
        raise EditorError("無法讀取圖片")
    area = width * height
    scale = 1.0
    if area > _MAX_PIXELS:
        scale = (_MAX_PIXELS / area) ** 0.5
    elif area < _MIN_PIXELS:
        scale = (_MIN_PIXELS / area) ** 0.5
    out_width = max(1, round(width * scale))
    out_height = max(1, round(height * scale))
    while out_width * out_height > _MAX_PIXELS and (out_width > 1 or out_height > 1):
        if out_width >= out_height and out_width > 1:
            out_width -= 1
        else:
            out_height -= 1
    return f"{out_width}x{out_height}"


def _image_url(payload: object) -> str:
    if not isinstance(payload, dict):
        raise EditorError("模型沒有回傳圖像")
    data = payload.get("data")
    if not isinstance(data, list) or not data or not isinstance(data[0], dict):
        raise EditorError("模型沒有回傳圖像")
    url = data[0].get("url")
    if not isinstance(url, str) or not url.startswith("https://"):
        raise EditorError("模型沒有回傳圖像")
    return url


def _to_jpeg(image: Image.Image) -> bytes:
    buffer = io.BytesIO()
    image.convert("RGB").save(buffer, format="JPEG", quality=90)
    return buffer.getvalue()
