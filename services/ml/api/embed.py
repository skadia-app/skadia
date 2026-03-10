"""Text-Embedding-Endpoint für semantische Fotosuche."""
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from models.clip import embed_text

router = APIRouter()


class TextEmbedRequest(BaseModel):
    text: str


class TextEmbedResponse(BaseModel):
    embedding: list[float]
    dim: int


@router.post("/embed/text", response_model=TextEmbedResponse)
async def embed_text_endpoint(req: TextEmbedRequest) -> TextEmbedResponse:
    """Gibt SigLIP2-Text-Embedding (1152 Dims) zurück.

    Wird von Go-API aufgerufen um CLIP-Vektorsuche zu ermöglichen.
    Erfordert SigLIP2-Modell (lazy-loaded, ~1.6 GB).
    """
    if not req.text.strip():
        raise HTTPException(status_code=400, detail="text must not be empty")
    try:
        vec = await embed_text(req.text)
    except Exception as exc:
        raise HTTPException(status_code=503, detail=f"embedding failed: {exc}") from exc
    return TextEmbedResponse(embedding=vec, dim=len(vec))
