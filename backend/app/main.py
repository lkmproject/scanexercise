import asyncio
import threading
from contextlib import asynccontextmanager

from fastapi import FastAPI, File, HTTPException, UploadFile
from fastapi.responses import JSONResponse, Response

from app.config import Settings, load_settings
from app.editor import ImageEditor, QwenImageEditor
from app.jobs import JobError, JobStore


def create_app(
    settings: Settings | None = None,
    editor: ImageEditor | None = None,
) -> FastAPI:
    settings = settings or load_settings()
    editor = editor or QwenImageEditor(
        api_key=settings.dashscope_api_key,
        workspace_id=settings.qwen_workspace_id,
        model=settings.qwen_model,
        timeout_seconds=settings.qwen_timeout_seconds,
    )
    store = JobStore(settings, editor)

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        app.state.store.cleanup()
        stop = asyncio.Event()

        async def sweep() -> None:
            while not stop.is_set():
                try:
                    await asyncio.wait_for(stop.wait(), timeout=600)
                except TimeoutError:
                    await asyncio.to_thread(app.state.store.cleanup)

        task = asyncio.create_task(sweep())
        yield
        stop.set()
        task.cancel()

    app = FastAPI(title="試卷筆跡清除", lifespan=lifespan)
    app.state.store = store

    @app.post("/jobs", status_code=202)
    async def create_job(pages: list[UploadFile] = File(...)) -> dict[str, str]:
        uploads: list[bytes] = []
        for page in pages:
            uploads.append(await page.read())
        try:
            job = store.create(uploads)
        except JobError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc
        threading.Thread(target=store.run, args=(job.id,), name=f"job-{job.id}").start()
        return {"id": job.id, "status": job.status}

    @app.get("/jobs/{job_id}")
    def get_job(job_id: str) -> JSONResponse:
        job = store.get(job_id)
        if job is None:
            raise HTTPException(status_code=404, detail="工作不存在或已過期")
        body: dict[str, object] = {
            "id": job.id,
            "status": job.status,
            "page_count": job.page_count,
            "error": job.error,
        }
        if job.status == "succeeded":
            body["previews"] = [f"/jobs/{job.id}/preview/{index}" for index in range(job.page_count)]
            body["pdf"] = f"/jobs/{job.id}/pdf"
        return JSONResponse(body)

    @app.get("/jobs/{job_id}/preview")
    def list_previews(job_id: str) -> dict[str, object]:
        job = _ready_job(store, job_id)
        return {
            "pages": [
                {"index": index, "url": f"/jobs/{job.id}/preview/{index}"}
                for index in range(job.page_count)
            ]
        }

    @app.get("/jobs/{job_id}/preview/{index}")
    def get_preview(job_id: str, index: int) -> Response:
        try:
            data = store.preview(job_id, index)
        except JobError as exc:
            raise _job_http_error(exc) from exc
        return Response(content=data, media_type="image/jpeg")

    @app.get("/jobs/{job_id}/pdf")
    def get_pdf(job_id: str) -> Response:
        try:
            data = store.pdf(job_id)
        except JobError as exc:
            raise _job_http_error(exc) from exc
        store.delete(job_id)
        return Response(
            content=data,
            media_type="application/pdf",
            headers={"Content-Disposition": 'attachment; filename="exam.pdf"'},
        )

    return app


def _ready_job(store: JobStore, job_id: str):
    job = store.get(job_id)
    if job is None:
        raise HTTPException(status_code=404, detail="工作不存在或已過期")
    if job.status == "failed":
        raise HTTPException(status_code=409, detail=job.error or "模型呼叫失敗")
    if job.status != "succeeded":
        raise HTTPException(status_code=409, detail="結果尚未完成")
    return job


def _job_http_error(exc: JobError) -> HTTPException:
    message = str(exc)
    if message == "工作不存在或已過期":
        return HTTPException(status_code=404, detail=message)
    if message == "找不到這一頁":
        return HTTPException(status_code=404, detail=message)
    return HTTPException(status_code=409, detail=message)


app = create_app()
