from pydantic import BaseModel, field_validator


class EmbedRequest(BaseModel):
    texts: list[str]
    normalize: bool = True

    @field_validator("texts")
    @classmethod
    def texts_must_be_non_empty_and_bounded(cls, v: list[str]) -> list[str]:
        if len(v) == 0:
            raise ValueError("texts must contain at least 1 item")
        if len(v) > 64:
            raise ValueError("texts must contain at most 64 items")
        return v


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]
    dim: int
    model: str
    count: int


class HealthResponse(BaseModel):
    status: str
    model: str
    model_loaded: bool
    dim: int | None
    uptime_seconds: float
