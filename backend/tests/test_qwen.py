import io
import json

import httpx
import pytest
from PIL import Image

from app.editor import EditorError, QwenImageEditor, output_size
from app.prompt import PROMPT


def _jpeg(size: tuple[int, int]) -> bytes:
    buffer = io.BytesIO()
    Image.new("RGB", size, "white").save(buffer, format="JPEG")
    return buffer.getvalue()


def _png() -> bytes:
    buffer = io.BytesIO()
    Image.new("RGB", (30, 40), "green").save(buffer, format="PNG")
    return buffer.getvalue()


def test_output_size_keeps_page_ratio_inside_model_limit() -> None:
    assert output_size(800, 1100) == "800x1100"
    width, height = output_size(4000, 5600).split("x")
    assert int(width) * int(height) <= 2048 * 2048
    assert abs((int(width) / int(height)) - (4000 / 5600)) < 0.02


def test_edit_posts_fixed_prompt_to_hong_kong_and_returns_jpeg() -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        if request.method == "POST":
            seen["url"] = str(request.url)
            seen["authorization"] = request.headers["authorization"]
            seen["body"] = json.loads(request.content.decode())
            return httpx.Response(200, json={"data": [{"url": "https://cdn.example/out.png"}]})
        return httpx.Response(200, content=_png())

    editor = QwenImageEditor(
        "sk-test",
        "ws-hongkong",
        "qwen-image-3.0-pro",
        30,
        httpx.Client(transport=httpx.MockTransport(handler)),
    )
    result = editor.edit(_jpeg((800, 1100)))
    body = seen["body"]
    assert isinstance(body, dict)
    assert seen["url"] == (
        "https://ws-hongkong.cn-hongkong.maas.aliyuncs.com/compatible-mode/v1/images/generations"
    )
    assert seen["authorization"] == "Bearer sk-test"
    assert body["model"] == "qwen-image-3.0-pro"
    assert body["prompt"] == PROMPT
    assert body["prompt_extend"] is False
    assert body["watermark"] is False
    assert body["size"] == "800x1100"
    assert str(body["image"]).startswith("data:image/jpeg;base64,")
    assert result[:2] == b"\xff\xd8"


def test_missing_credentials_fail_before_the_request() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise AssertionError(request.url)

    client = httpx.Client(transport=httpx.MockTransport(handler))
    with pytest.raises(EditorError, match="DASHSCOPE_API_KEY"):
        QwenImageEditor("", "ws-hongkong", "qwen-image-3.0-pro", 5, client).edit(_jpeg((20, 20)))
    with pytest.raises(EditorError, match="QWEN_WORKSPACE_ID"):
        QwenImageEditor("sk-test", "", "qwen-image-3.0-pro", 5, client).edit(_jpeg((20, 20)))


def test_model_error_and_missing_image_are_failures() -> None:
    def reject(request: httpx.Request) -> httpx.Response:
        return httpx.Response(400, json={"error": {"message": "bad", "code": "InvalidParameter"}})

    with pytest.raises(EditorError, match="模型呼叫失敗"):
        QwenImageEditor(
            "sk-test",
            "ws-hongkong",
            "qwen-image-3.0-pro",
            5,
            httpx.Client(transport=httpx.MockTransport(reject)),
        ).edit(_jpeg((20, 20)))

    def empty(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"data": []})

    with pytest.raises(EditorError, match="模型沒有回傳圖像"):
        QwenImageEditor(
            "sk-test",
            "ws-hongkong",
            "qwen-image-3.0-pro",
            5,
            httpx.Client(transport=httpx.MockTransport(empty)),
        ).edit(_jpeg((20, 20)))
