import numpy as np
import pytest
from asgi_lifespan import LifespanManager
from httpx import AsyncClient, ASGITransport

from app.main import create_app


@pytest.mark.asyncio
async def test_embed_success(mock_sentence_transformer):
    mock_sentence_transformer.encode.return_value = np.random.rand(1, 256).astype("float32")
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=True)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.post("/embed", json={
                "texts": ["決定: SQLite を採用"],
                "normalize": True,
            })
    assert response.status_code == 200
    data = response.json()
    assert data["count"] == 1
    assert len(data["embeddings"]) == 1
    assert len(data["embeddings"][0]) == 256
    assert data["dim"] == 256


@pytest.mark.asyncio
async def test_embed_batch(mock_sentence_transformer):
    mock_sentence_transformer.encode.return_value = np.random.rand(10, 256).astype("float32")
    texts = [f"text_{i}" for i in range(10)]
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=True)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.post("/embed", json={"texts": texts})
    assert response.status_code == 200
    assert response.json()["count"] == 10


@pytest.mark.asyncio
async def test_embed_model_not_ready():
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=False)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.post("/embed", json={"texts": ["hello"]})
    assert response.status_code == 503
    assert response.json()["error"] == "model_not_ready"


@pytest.mark.asyncio
async def test_embed_empty_texts():
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=False)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.post("/embed", json={"texts": []})
    assert response.status_code == 422


@pytest.mark.asyncio
async def test_embed_too_many_texts():
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=False)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.post("/embed", json={"texts": ["x"] * 65})
    assert response.status_code == 422


@pytest.mark.asyncio
async def test_embed_normalize_false(mock_sentence_transformer):
    mock_sentence_transformer.encode.return_value = np.random.rand(1, 256).astype("float32")
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=True)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.post("/embed", json={"texts": ["hello"], "normalize": False})
    assert response.status_code == 200
    call_kwargs = mock_sentence_transformer.encode.call_args.kwargs
    assert call_kwargs.get("normalize_embeddings") is False


@pytest.mark.asyncio
async def test_embed_response_has_model_name(mock_sentence_transformer):
    mock_sentence_transformer.encode.return_value = np.random.rand(1, 256).astype("float32")
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=True)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.post("/embed", json={"texts": ["hello"]})
    data = response.json()
    assert data["model"] == "cl-nagoya/ruri-v3-30m"
