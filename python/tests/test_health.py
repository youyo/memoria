import pytest
from asgi_lifespan import LifespanManager
from httpx import AsyncClient, ASGITransport

from app.main import create_app


@pytest.mark.asyncio
async def test_health_model_not_loaded():
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=False)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.get("/health")
    assert response.status_code == 503
    data = response.json()
    assert data["model_loaded"] is False
    assert data["status"] == "loading"


@pytest.mark.asyncio
async def test_health_model_loaded(mock_sentence_transformer):
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=True)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.get("/health")
    assert response.status_code == 200
    data = response.json()
    assert data["model_loaded"] is True
    assert data["dim"] == 256
    assert data["status"] == "ok"


@pytest.mark.asyncio
async def test_health_response_has_uptime(mock_sentence_transformer):
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=True)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.get("/health")
    data = response.json()
    assert "uptime_seconds" in data
    assert data["uptime_seconds"] >= 0


@pytest.mark.asyncio
async def test_health_response_has_model_name(mock_sentence_transformer):
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=True)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            response = await client.get("/health")
    data = response.json()
    assert data["model"] == "cl-nagoya/ruri-v3-30m"
