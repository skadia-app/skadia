"""VLM-Pipeline fuer Gear-Dokumente (Rechnungen, Handbuecher).

Ablauf:
1. Text-PDF (is_text_pdf=True): extracted_text -> Qwen3-8B -> JSON
2. Bild-PDF (is_text_pdf=False): S3-Download -> Qwen3-VL-8B -> JSON
Graceful Degradation: VLM-8B -> VLM-4B -> Text-only (wenn kein VLM verfuegbar)
"""
import json
import logging
import os
from typing import Optional

import httpx
from openai import AsyncOpenAI

logger = logging.getLogger(__name__)

LLM_BASE_URL = os.getenv("LLM_BASE_URL", "http://litellm:4000/v1")
LLM_API_KEY = os.getenv("LLM_API_KEY", "none")
LLM_MODEL = os.getenv("ML_LLM_MODEL", "skadia-default")
VLM_MODEL = os.getenv("ML_VLM_MODEL", "skadia-vlm")
VLM_FALLBACK = os.getenv("ML_VLM_MODEL_FALLBACK", "skadia-vlm-fallback")

EXTRACTION_PROMPT = """Du bist ein Assistent zum Extrahieren von Produktinformationen aus Rechnungen.
Extrahiere alle Produkte aus dem Text und gib das Ergebnis als JSON zurueck.

Format:
{
  "products": [
    {
      "name": "Produktname",
      "type": "fork|frame|component|clothing|other",
      "brand": "Marke falls bekannt",
      "model": "Modellname falls bekannt",
      "price": 123.45,
      "currency": "EUR",
      "quantity": 1
    }
  ],
  "invoice_date": "YYYY-MM-DD oder null",
  "vendor": "Haendlername oder null",
  "total": 123.45
}

Wenn ein Feld unbekannt ist, setze es auf null. Gib NUR valides JSON zurueck, kein Markdown."""


async def extract_from_text(text: str) -> Optional[dict]:
    """Extrahiere Produktdaten aus Text-PDF via Qwen3-8B."""
    client = AsyncOpenAI(base_url=LLM_BASE_URL, api_key=LLM_API_KEY)
    try:
        response = await client.chat.completions.create(
            model=LLM_MODEL,
            messages=[
                {"role": "system", "content": EXTRACTION_PROMPT},
                {"role": "user", "content": f"Rechnung:\n\n{text[:8000]}"},
            ],
            temperature=0.1,
            max_tokens=1024,
        )
        raw = response.choices[0].message.content or ""
        # JSON aus Antwort extrahieren
        start = raw.find("{")
        end = raw.rfind("}") + 1
        if start >= 0 and end > start:
            return json.loads(raw[start:end])
    except Exception as e:
        logger.warning("text extraction failed: %s", e)
    return None


async def download_s3_doc(s3_bucket: str, s3_key: str) -> Optional[bytes]:
    """Lade Dokument aus Garage S3."""
    garage_endpoint = os.getenv("GARAGE_ENDPOINT", "http://garage:3900")

    url = f"{garage_endpoint}/{s3_bucket}/{s3_key}"
    try:
        async with httpx.AsyncClient() as client:
            resp = await client.get(url, timeout=30.0)
            if resp.status_code == 200:
                return resp.content
    except Exception as e:
        logger.warning("S3 download failed for %s/%s: %s", s3_bucket, s3_key, e)
    return None


async def extract_from_image(
    image_bytes: bytes, model: str = VLM_MODEL
) -> Optional[dict]:
    """Extrahiere Produktdaten aus Bild-PDF via Qwen3-VL."""
    import base64

    client = AsyncOpenAI(base_url=LLM_BASE_URL, api_key=LLM_API_KEY)
    b64 = base64.b64encode(image_bytes).decode()
    try:
        response = await client.chat.completions.create(
            model=model,
            messages=[
                {
                    "role": "user",
                    "content": [
                        {
                            "type": "text",
                            "text": EXTRACTION_PROMPT
                            + "\n\nExtrahiere aus diesem Rechnungsbild:",
                        },
                        {
                            "type": "image_url",
                            "image_url": {
                                "url": f"data:application/pdf;base64,{b64}"
                            },
                        },
                    ],
                }
            ],
            temperature=0.1,
            max_tokens=1024,
        )
        raw = response.choices[0].message.content or ""
        start = raw.find("{")
        end = raw.rfind("}") + 1
        if start >= 0 and end > start:
            return json.loads(raw[start:end])
    except Exception as e:
        logger.warning("VLM extraction with %s failed: %s", model, e)
        if model == VLM_MODEL:
            # Fallback auf kleineres VLM
            return await extract_from_image(image_bytes, VLM_FALLBACK)
    return None


async def process_document(
    pool,
    doc_id: str,
    gear_id: Optional[str],
    component_id: Optional[str],
    user_id: str,
    is_text_pdf: bool,
    extracted_text: Optional[str],
    s3_bucket: str,
    s3_key: str,
) -> bool:
    """Verarbeite ein hochgeladenes Dokument und erstelle Gear-Vorschlaege.

    Returns True bei Erfolg, False bei Fehler.
    """
    logger.info("processing document %s (is_text_pdf=%s)", doc_id, is_text_pdf)

    result = None

    if is_text_pdf and extracted_text:
        result = await extract_from_text(extracted_text)
    else:
        # Bild-PDF: Download aus S3
        image_bytes = await download_s3_doc(s3_bucket, s3_key)
        if image_bytes:
            result = await extract_from_image(image_bytes)

    if not result or not result.get("products"):
        logger.info("no products extracted from document %s", doc_id)
        return False

    # Gear-Vorschlaege in DB schreiben
    async with pool.acquire() as conn:
        for product in result["products"]:
            if not product.get("name"):
                continue
            await conn.execute(
                """
                INSERT INTO gear (id, user_id, name, type, brand, model,
                                  purchase_price, currency, review_status,
                                  source_document_id, created_at)
                VALUES (gen_random_uuid(), $1, $2, $3, $4, $5,
                        $6, $7, 'pending_review', $8, NOW())
                ON CONFLICT DO NOTHING
                """,
                user_id,
                product.get("name", "Unbekanntes Produkt"),
                product.get("type", "other"),
                product.get("brand"),
                product.get("model"),
                product.get("price"),
                product.get("currency", "EUR"),
                doc_id,
            )

        logger.info(
            "created %d gear suggestions for document %s",
            len(result["products"]),
            doc_id,
        )

    return True
