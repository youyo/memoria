import pytest
from pydantic import ValidationError

from app.schemas import EmbedRequest, EmbedResponse, HealthResponse


def test_embed_request_valid():
    req = EmbedRequest(texts=["hello", "world"], normalize=True)
    assert len(req.texts) == 2


def test_embed_request_default_normalize():
    req = EmbedRequest(texts=["hello"])
    assert req.normalize is True


def test_embed_request_empty_texts():
    with pytest.raises(ValidationError):
        EmbedRequest(texts=[], normalize=True)


def test_embed_request_too_many_texts():
    with pytest.raises(ValidationError):
        EmbedRequest(texts=["x"] * 65, normalize=True)


def test_embed_request_exactly_64_texts():
    req = EmbedRequest(texts=["x"] * 64, normalize=True)
    assert len(req.texts) == 64


def test_embed_response():
    resp = EmbedResponse(
        embeddings=[[0.1, 0.2]],
        dim=2,
        model="cl-nagoya/ruri-v3-30m",
        count=1,
    )
    assert resp.count == 1
    assert resp.dim == 2
    assert resp.model == "cl-nagoya/ruri-v3-30m"
    assert len(resp.embeddings) == 1


def test_health_response_ok():
    resp = HealthResponse(
        status="ok",
        model="cl-nagoya/ruri-v3-30m",
        model_loaded=True,
        dim=256,
        uptime_seconds=10.0,
    )
    assert resp.status == "ok"
    assert resp.model_loaded is True
    assert resp.dim == 256


def test_health_response_loading():
    resp = HealthResponse(
        status="loading",
        model="cl-nagoya/ruri-v3-30m",
        model_loaded=False,
        dim=None,
        uptime_seconds=3.1,
    )
    assert resp.status == "loading"
    assert resp.model_loaded is False
    assert resp.dim is None
