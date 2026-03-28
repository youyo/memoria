import numpy as np
import pytest
from unittest.mock import MagicMock, patch

from app.model import ModelManager, ModelNotReadyError


def test_model_manager_initial_state():
    mgr = ModelManager("cl-nagoya/ruri-v3-30m")
    assert not mgr._loaded
    assert mgr._dim is None


def test_preload_success(mock_sentence_transformer):
    mgr = ModelManager("cl-nagoya/ruri-v3-30m")
    mgr.preload()
    assert mgr._loaded
    assert mgr._dim == 256


def test_preload_failure():
    with patch("app.model.SentenceTransformer", side_effect=OSError("no model")):
        mgr = ModelManager("cl-nagoya/ruri-v3-30m")
        with pytest.raises(RuntimeError, match="model load failed"):
            mgr.preload()


def test_preload_failure_sets_not_loaded():
    with patch("app.model.SentenceTransformer", side_effect=OSError("no model")):
        mgr = ModelManager("cl-nagoya/ruri-v3-30m")
        try:
            mgr.preload()
        except RuntimeError:
            pass
        assert not mgr._loaded


def test_embed_not_loaded():
    mgr = ModelManager("cl-nagoya/ruri-v3-30m")
    with pytest.raises(ModelNotReadyError):
        mgr.embed(["hello"])


def test_embed_returns_vectors(mock_sentence_transformer):
    mock_sentence_transformer.encode.return_value = np.random.rand(2, 256).astype("float32")
    mgr = ModelManager("cl-nagoya/ruri-v3-30m")
    mgr.preload()
    result = mgr.embed(["hello", "world"])
    assert len(result) == 2
    assert len(result[0]) == 256


def test_embed_single_text(mock_sentence_transformer):
    mock_sentence_transformer.encode.return_value = np.random.rand(1, 256).astype("float32")
    mgr = ModelManager("cl-nagoya/ruri-v3-30m")
    mgr.preload()
    result = mgr.embed(["hello"])
    assert len(result) == 1
    assert len(result[0]) == 256


def test_embed_normalize_passed(mock_sentence_transformer):
    mock_sentence_transformer.encode.return_value = np.random.rand(1, 256).astype("float32")
    mgr = ModelManager("cl-nagoya/ruri-v3-30m")
    mgr.preload()
    mgr.embed(["hello"], normalize=False)
    call_kwargs = mock_sentence_transformer.encode.call_args.kwargs
    assert call_kwargs.get("normalize_embeddings") is False


def test_status_loaded(mock_sentence_transformer):
    mgr = ModelManager("cl-nagoya/ruri-v3-30m")
    mgr.preload()
    s = mgr.status()
    assert s["model_loaded"] is True
    assert s["dim"] == 256
    assert s["status"] == "ok"


def test_status_not_loaded():
    mgr = ModelManager("cl-nagoya/ruri-v3-30m")
    s = mgr.status()
    assert s["model_loaded"] is False
    assert s["dim"] is None
    assert s["status"] == "loading"


@pytest.mark.asyncio
async def test_embed_async_returns_vectors(mock_sentence_transformer):
    mock_sentence_transformer.encode.return_value = np.random.rand(2, 256).astype("float32")
    mgr = ModelManager("cl-nagoya/ruri-v3-30m")
    mgr.preload()
    result = await mgr.embed_async(["hello", "world"])
    assert len(result) == 2
    assert len(result[0]) == 256


@pytest.mark.asyncio
async def test_embed_async_not_loaded():
    mgr = ModelManager("cl-nagoya/ruri-v3-30m")
    with pytest.raises(ModelNotReadyError):
        await mgr.embed_async(["hello"])
