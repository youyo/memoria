import asyncio
import threading
from functools import partial

from sentence_transformers import SentenceTransformer


class ModelNotReadyError(Exception):
    """モデルがロードされていない場合に発生する例外"""


class ModelManager:
    def __init__(self, model_name: str):
        self._model_name = model_name
        self._model: SentenceTransformer | None = None
        self._loaded = False
        self._dim: int | None = None
        self._load_error: str | None = None
        self._lock = threading.Lock()

    def preload(self) -> None:
        """同期的にモデルをロードする。起動時の --preload オプションで呼ばれる。"""
        try:
            # device 選択順: mps (Apple Silicon) -> cuda (GPU) -> cpu
            device = _select_device()
            model = SentenceTransformer(self._model_name, device=device)
            dim = model.get_sentence_embedding_dimension()
            with self._lock:
                self._model = model
                self._dim = dim
                self._loaded = True
                self._load_error = None
        except Exception as e:
            with self._lock:
                self._loaded = False
                self._load_error = str(e)
            raise RuntimeError(f"model load failed: {e}") from e

    def embed(self, texts: list[str], normalize: bool = True) -> list[list[float]]:
        """テキストリストを embedding ベクトルに変換する（同期）。"""
        with self._lock:
            if not self._loaded or self._model is None:
                raise ModelNotReadyError("model is not loaded")
            model = self._model

        embeddings = model.encode(
            texts,
            normalize_embeddings=normalize,
            batch_size=32,
            show_progress_bar=False,
        )
        return embeddings.tolist()

    async def embed_async(self, texts: list[str], normalize: bool = True) -> list[list[float]]:
        """スレッドプール経由で embed() を非同期実行する。asyncio ブロッキングを防ぐ。"""
        loop = asyncio.get_event_loop()
        fn = partial(self.embed, texts, normalize)
        return await loop.run_in_executor(None, fn)

    def status(self) -> dict:
        """health エンドポイント向けのステータス情報を返す。"""
        with self._lock:
            loaded = self._loaded
            dim = self._dim
        return {
            "status": "ok" if loaded else "loading",
            "model": self._model_name,
            "model_loaded": loaded,
            "dim": dim,
        }


def _select_device() -> str:
    """利用可能なデバイスを選択する。mps -> cuda -> cpu の順。"""
    try:
        import torch
        if torch.backends.mps.is_available():
            return "mps"
        if torch.cuda.is_available():
            return "cuda"
    except Exception:
        pass
    return "cpu"
