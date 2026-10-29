import io
import json
import time

from fastapi.testclient import TestClient
from PIL import Image

from app.config import Settings
from app.editor import EditorError
from app.main import create_app
from app.prompt import PROMPT


class FakeEditor:
    def __init__(self, results: list[bytes] | None = None, error: EditorError | None = None) -> None:
        self._results = list(results or [])
        self._error = error
        self.calls: list[bytes] = []

    def edit(self, jpeg: bytes) -> bytes:
        self.calls.append(jpeg)
        if self._error is not None:
            raise self._error
        if not self._results:
            raise EditorError("模型沒有回傳圖像")
        return self._results.pop(0)


def _jpeg(color: str, size: tuple[int, int] = (20, 10)) -> bytes:
    buffer = io.BytesIO()
    Image.new("RGB", size, color).save(buffer, format="JPEG", quality=95)
    return buffer.getvalue()


def _client(tmp_path, editor: FakeEditor) -> TestClient:
    settings = Settings(
        dashscope_api_key="",
        qwen_workspace_id="",
        qwen_model="qwen-image-3.0-pro",
        data_dir=tmp_path,
        max_pages=20,
        retention_hours=24,
        max_long_edge=100,
        qwen_timeout_seconds=5,
    )
    app = create_app(settings, editor)
    return TestClient(app)


def _wait(client: TestClient, job_id: str) -> dict:
    last = {}
    for _ in range(50):
        last = client.get(f"/jobs/{job_id}").json()
        if last["status"] in {"succeeded", "failed"}:
            return last
        time.sleep(0.05)
    raise AssertionError(last)


def test_prompt_keeps_printed_text_and_figures() -> None:
    assert "不要轉換" in PROMPT
    assert "留在圖上" in PROMPT
    assert "不要挖空印刷內容" in PROMPT
    assert "只輸出修改後的圖像" in PROMPT


def test_preprocess_applies_exif_and_limits_long_edge(tmp_path) -> None:
    image = Image.new("RGB", (300, 40), "blue")
    exif = Image.Exif()
    exif[0x0112] = 6
    raw = io.BytesIO()
    image.save(raw, format="JPEG", exif=exif)
    editor = FakeEditor([_jpeg("green")])
    client = _client(tmp_path, editor)
    with client:
        created = client.post("/jobs", files=[("pages", ("page.jpg", raw.getvalue(), "image/jpeg"))])
        assert created.status_code == 202
        job_id = created.json()["id"]
        assert _wait(client, job_id)["status"] == "succeeded"
    prepared = Image.open(io.BytesIO(editor.calls[0]))
    assert max(prepared.size) <= 100
    assert prepared.size[0] < prepared.size[1]


def test_success_returns_edited_pages_in_order_then_deletes_after_pdf(tmp_path) -> None:
    first = _jpeg("red", (30, 20))
    second = _jpeg("blue", (30, 20))
    edited = [_jpeg("green", (16, 16)), _jpeg("yellow", (18, 12))]
    client = _client(tmp_path, FakeEditor(edited.copy()))
    with client:
        created = client.post(
            "/jobs",
            data={"prompt": "把題目改成另一種語言"},
            files=[
                ("pages", ("a.jpg", first, "image/jpeg")),
                ("pages", ("b.jpg", second, "image/jpeg")),
            ],
        )
        job_id = created.json()["id"]
        body = _wait(client, job_id)
        assert body["status"] == "succeeded"
        assert body["previews"] == [f"/jobs/{job_id}/preview/0", f"/jobs/{job_id}/preview/1"]
        page0 = client.get(f"/jobs/{job_id}/preview/0").content
        page1 = client.get(f"/jobs/{job_id}/preview/1").content
        assert page0 == edited[0]
        assert page1 == edited[1]
        assert first not in page0
        pdf = client.get(f"/jobs/{job_id}/pdf")
        assert pdf.status_code == 200
        assert pdf.headers["content-type"] == "application/pdf"
        assert pdf.content.find(edited[0]) < pdf.content.find(edited[1])
        assert client.get(f"/jobs/{job_id}").status_code == 404


def test_model_failure_does_not_return_the_original(tmp_path) -> None:
    original = _jpeg("red")
    client = _client(tmp_path, FakeEditor(error=EditorError("模型沒有回傳圖像")))
    with client:
        created = client.post("/jobs", files=[("pages", ("a.jpg", original, "image/jpeg"))])
        job_id = created.json()["id"]
        body = _wait(client, job_id)
        assert body["status"] == "failed"
        assert body["error"] == "模型沒有回傳圖像"
        preview = client.get(f"/jobs/{job_id}/preview/0")
        assert preview.status_code == 409
        assert original not in preview.content
        assert client.get(f"/jobs/{job_id}/pdf").status_code == 409
        assert not (tmp_path / "jobs" / job_id / "output").exists()


def test_second_page_failure_discards_partial_output(tmp_path) -> None:
    class FailSecond:
        def __init__(self) -> None:
            self.calls = 0

        def edit(self, jpeg: bytes) -> bytes:
            self.calls += 1
            if self.calls == 1:
                return _jpeg("green")
            raise EditorError("模型逾時")

    client = _client(tmp_path, FailSecond())
    with client:
        created = client.post(
            "/jobs",
            files=[
                ("pages", ("a.jpg", _jpeg("red"), "image/jpeg")),
                ("pages", ("b.jpg", _jpeg("blue"), "image/jpeg")),
            ],
        )
        job_id = created.json()["id"]
        body = _wait(client, job_id)
        assert body["status"] == "failed"
        assert body["error"] == "模型逾時"
        assert client.get(f"/jobs/{job_id}/preview/0").status_code == 409


def test_rejects_too_many_pages_and_unreadable_images(tmp_path) -> None:
    client = _client(tmp_path, FakeEditor())
    with client:
        too_many = [("pages", (f"{index}.jpg", _jpeg("red"), "image/jpeg")) for index in range(21)]
        assert client.post("/jobs", files=too_many).status_code == 400
        invalid = client.post("/jobs", files=[("pages", ("a.jpg", b"not-an-image", "image/jpeg"))])
        assert invalid.status_code == 400
        assert invalid.json()["detail"] == "無法讀取圖片"


def test_expired_job_is_removed(tmp_path) -> None:
    client = _client(tmp_path, FakeEditor([_jpeg("green")]))
    with client:
        created = client.post("/jobs", files=[("pages", ("a.jpg", _jpeg("red"), "image/jpeg"))])
        job_id = created.json()["id"]
        assert _wait(client, job_id)["status"] == "succeeded"
        meta_path = tmp_path / "jobs" / job_id / "meta.json"
        meta = json.loads(meta_path.read_text(encoding="utf-8"))
        meta["expires_at"] = "2000-01-01T00:00:00+00:00"
        meta_path.write_text(json.dumps(meta), encoding="utf-8")
        assert client.get(f"/jobs/{job_id}").status_code == 404
        assert not meta_path.exists()
