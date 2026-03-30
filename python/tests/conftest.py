import numpy as np
import pytest
from asgi_lifespan import LifespanManager
from httpx import AsyncClient, ASGITransport
from unittest.mock import MagicMock, patch


@pytest.fixture
def mock_sentence_transformer():
    mock_model = MagicMock()
    mock_model.encode.return_value = np.random.rand(2, 256).astype("float32")
    mock_model.get_sentence_embedding_dimension.return_value = 256
    with patch("app.model.SentenceTransformer", return_value=mock_model):
        yield mock_model


@pytest.fixture
async def http_client_no_preload():
    """preload=False のアプリ用 AsyncClient (lifespan 付き)"""
    from app.main import create_app
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=False)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            yield client


@pytest.fixture
async def http_client_with_preload(mock_sentence_transformer):
    """preload=True のアプリ用 AsyncClient (lifespan 付き、モック使用)"""
    from app.main import create_app
    app = create_app(model_name="cl-nagoya/ruri-v3-30m", preload=True)
    async with LifespanManager(app):
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            yield client, mock_sentence_transformer
