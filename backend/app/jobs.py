import json
import re
import shutil
import threading
import uuid
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path

from app.config import Settings
from app.editor import EditorError, ImageEditor
from app.pdf import build_pdf
from app.preprocess import PreprocessError, prepare_page


class JobError(Exception):
    pass


@dataclass
class Job:
    id: str
    status: str
    page_count: int
    error: str | None
    created_at: str
    expires_at: str


class JobStore:
    def __init__(self, settings: Settings, editor: ImageEditor) -> None:
        self._settings = settings
        self._editor = editor
        self._lock = threading.Lock()
        self._root = settings.data_dir / "jobs"
        self._root.mkdir(parents=True, exist_ok=True)

    def create(self, uploads: list[bytes]) -> Job:
        self.cleanup()
        if not uploads:
            raise JobError("沒有上傳頁面")
        if len(uploads) > self._settings.max_pages:
            raise JobError(f"一次最多 {self._settings.max_pages} 頁")
        for upload in uploads:
            if len(upload) > self._settings.max_upload_bytes:
                raise JobError("圖片太大")

        prepared: list[bytes] = []
        for upload in uploads:
            try:
                prepared.append(prepare_page(upload, self._settings.max_long_edge))
            except PreprocessError as exc:
                raise JobError(str(exc)) from exc

        job_id = uuid.uuid4().hex
        directory = self._job_dir(job_id)
        (directory / "input").mkdir(parents=True)
        for index, page in enumerate(prepared):
            (directory / "input" / f"{index}.jpg").write_bytes(page)
        now = _now()
        job = Job(
            id=job_id,
            status="pending",
            page_count=len(prepared),
            error=None,
            created_at=now.isoformat(),
            expires_at=(now + timedelta(hours=self._settings.retention_hours)).isoformat(),
        )
        self._write(job)
        return job

    def run(self, job_id: str) -> None:
        job = self._read(job_id)
        if job is None or job.status != "pending":
            return
        job.status = "running"
        self._write(job)
        directory = self._job_dir(job_id)
        output_dir = directory / "output"
        output_dir.mkdir(exist_ok=True)
        pages: list[bytes] = []
        try:
            for index in range(job.page_count):
                source = (directory / "input" / f"{index}.jpg").read_bytes()
                pages.append(self._editor.edit(source))
            for index, page in enumerate(pages):
                (output_dir / f"{index}.jpg").write_bytes(page)
            (directory / "result.pdf").write_bytes(build_pdf(pages))
        except EditorError as exc:
            self._fail(job, str(exc))
            return
        except Exception:
            self._fail(job, "模型呼叫失敗")
            return
        job.status = "succeeded"
        job.error = None
        self._write(job)

    def get(self, job_id: str) -> Job | None:
        try:
            self._delete_if_expired(job_id)
            return self._read(job_id)
        except JobError:
            return None

    def preview(self, job_id: str, index: int) -> bytes:
        job = self.get(job_id)
        self._require_ready(job)
        assert job is not None
        path = self._job_dir(job_id) / "output" / f"{index}.jpg"
        if index < 0 or index >= job.page_count or not path.is_file():
            raise JobError("找不到這一頁")
        return path.read_bytes()

    def pdf(self, job_id: str) -> bytes:
        job = self.get(job_id)
        self._require_ready(job)
        path = self._job_dir(job_id) / "result.pdf"
        if not path.is_file():
            raise JobError("結果尚未完成")
        return path.read_bytes()

    def delete(self, job_id: str) -> None:
        directory = self._job_dir(job_id)
        if directory.exists():
            shutil.rmtree(directory)

    def cleanup(self) -> None:
        if not self._root.exists():
            return
        for directory in self._root.iterdir():
            if directory.is_dir():
                self._delete_if_expired(directory.name)

    def _fail(self, job: Job, message: str) -> None:
        output_dir = self._job_dir(job.id) / "output"
        if output_dir.exists():
            shutil.rmtree(output_dir)
        pdf_path = self._job_dir(job.id) / "result.pdf"
        if pdf_path.exists():
            pdf_path.unlink()
        job.status = "failed"
        job.error = message
        self._write(job)

    def _require_ready(self, job: Job | None) -> None:
        if job is None:
            raise JobError("工作不存在或已過期")
        if job.status == "failed":
            raise JobError(job.error or "模型呼叫失敗")
        if job.status != "succeeded":
            raise JobError("結果尚未完成")

    def _delete_if_expired(self, job_id: str) -> None:
        job = self._read(job_id)
        if job is None:
            return
        expires_at = datetime.fromisoformat(job.expires_at)
        if expires_at <= _now():
            self.delete(job_id)

    def _job_dir(self, job_id: str) -> Path:
        if _JOB_ID.fullmatch(job_id) is None:
            raise JobError("工作不存在或已過期")
        return self._root / job_id

    def _write(self, job: Job) -> None:
        path = self._job_dir(job.id) / "meta.json"
        payload = {
            "id": job.id,
            "status": job.status,
            "page_count": job.page_count,
            "error": job.error,
            "created_at": job.created_at,
            "expires_at": job.expires_at,
        }
        with self._lock:
            path.write_text(json.dumps(payload), encoding="utf-8")

    def _read(self, job_id: str) -> Job | None:
        path = self._job_dir(job_id) / "meta.json"
        if not path.is_file():
            return None
        with self._lock:
            payload = json.loads(path.read_text(encoding="utf-8"))
        return Job(
            id=payload["id"],
            status=payload["status"],
            page_count=payload["page_count"],
            error=payload["error"],
            created_at=payload["created_at"],
            expires_at=payload["expires_at"],
        )


_JOB_ID = re.compile(r"^[0-9a-f]{32}$")


def _now() -> datetime:
    return datetime.now(timezone.utc)
