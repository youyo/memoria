import time
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from app.lifecycle import IdleTimer
from app.model import ModelManager, ModelNotReadyError
from app.schemas import EmbedRequest, EmbedResponse, HealthResponse


def create_app(
    model_name: str = "cl-nagoya/ruri-v3-30m",
    preload: bool = False,
    idle_timeout: int = 600,
) -> FastAPI:
    model_mgr = ModelManager(model_name)
    idle_timer = IdleTimer(idle_timeout)
    start_time = time.monotonic()

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        if preload:
            model_mgr.preload()
        idle_timer.start()
        yield

    app = FastAPI(lifespan=lifespan)
    app.state.model = model_mgr
    app.state.idle_timer = idle_timer

    @app.middleware("http")
    async def touch_idle_timer(request: Request, call_next):
        idle_timer.touch()
        return await call_next(request)

    @app.get("/health", response_model=HealthResponse)
    async def health():
        s = model_mgr.status()
        uptime = time.monotonic() - start_time
        s["uptime_seconds"] = uptime
        code = 200 if s["model_loaded"] else 503
        return JSONResponse(s, status_code=code)

    @app.post("/embed", response_model=EmbedResponse)
    async def embed(req: EmbedRequest):
        try:
            vecs = await model_mgr.embed_async(req.texts, req.normalize)
        except ModelNotReadyError:
            return JSONResponse(
                {"error": "model_not_ready", "detail": "model is not loaded yet"},
                status_code=503,
            )
        except Exception as e:
            return JSONResponse(
                {"error": "embed_failed", "detail": str(e)},
                status_code=500,
            )
        return EmbedResponse(
            embeddings=vecs,
            dim=model_mgr._dim or 0,
            model=model_name,
            count=len(vecs),
        )

    return app
