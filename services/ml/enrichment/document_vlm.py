"""LLM-Pipeline fuer Gear-Dokumente (Rechnungen, Handbuecher).

Ablauf:
1. Text-PDF (is_text_pdf=True): extracted_text -> document_extract role -> JSON
2. Bild-PDF (is_text_pdf=False): S3-Download -> vision role -> JSON
Fallback: Bild-Extraktion fehlgeschlagen + extracted_text vorhanden -> Text-Extraktion
"""
import json
import logging
import os
from typing import Optional

import aioboto3
import httpx

from config import (
    LLMRoleNotConfigured,
    ML_DOCUMENT_ROLE,
    ML_MARKDOWN_ROLE,
    ML_VISION_ROLE,
    call_llm_broker,
)

logger = logging.getLogger(__name__)

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


def _gear_type_and_scope(raw_type: Optional[str]) -> tuple[str, str]:
    value = (raw_type or "").strip().lower()
    if value in {"clothing", "apparel"}:
        return "apparel", "apparel"
    if value in {"shoe", "shoes"}:
        return "shoes", "shoes"
    if value in {"bike", "bicycle", "frame"}:
        return "bike", "bike"
    return "other", "other"


def not_configured_payload(role: str) -> dict:
    return {
        "error": {
            "code": "llm_role_not_configured",
            "role": role,
            "status": "not_configured",
        }
    }


def _is_not_configured_payload(result: Optional[dict]) -> bool:
    if not isinstance(result, dict):
        return False
    error = result.get("error")
    return isinstance(error, dict) and error.get("code") == "llm_role_not_configured"


async def _chat_via_broker(
    role: str,
    task_type: str,
    messages: list,
    timeout: float = 120.0,
) -> str:
    return await call_llm_broker(role, task_type, messages, timeout=timeout)


async def _get_user_prompt(pool, user_id: str, prompt_type: str) -> str:
    """Lädt den benutzerdefinierten Prompt oder gibt den Default zurück."""
    col = "ocr_invoice_prompt" if prompt_type == "invoice" else "ocr_gear_prompt"
    async with pool.acquire() as conn:
        result = await conn.fetchval(
            f"SELECT {col} FROM user_enrichment_settings WHERE user_id = $1",
            user_id,
        )
    return result or EXTRACTION_PROMPT


async def extract_from_text(text: str, prompt: str = EXTRACTION_PROMPT) -> Optional[dict]:
    """Extrahiere Produktdaten aus Text-PDF via document_extract role."""
    try:
        raw = await _chat_via_broker(
            ML_DOCUMENT_ROLE,
            "document_extract",
            [
                {"role": "system", "content": prompt},
                {"role": "user", "content": f"Rechnung:\n\n{text[:8000]}"},
            ],
        )
        # JSON aus Antwort extrahieren
        start = raw.find("{")
        end = raw.rfind("}") + 1
        if start >= 0 and end > start:
            return json.loads(raw[start:end])
    except LLMRoleNotConfigured as e:
        logger.info("text extraction skipped: %s", e)
        return not_configured_payload(e.role)
    except Exception as e:
        logger.warning("text extraction failed: %s", e)
    return None


async def download_s3_doc(s3_bucket: str, s3_key: str) -> Optional[bytes]:
    """Lade Dokument aus Garage S3."""
    garage_endpoint = os.getenv("S3_ENDPOINT", os.getenv("GARAGE_ENDPOINT", "http://garage:3900"))

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
    image_bytes: bytes, prompt: str = EXTRACTION_PROMPT
) -> Optional[dict]:
    """Extrahiere Produktdaten aus Bild-PDF via vision role."""
    import base64

    b64 = base64.b64encode(image_bytes).decode()
    try:
        raw = await _chat_via_broker(
            ML_VISION_ROLE,
            "vision",
            [
                {
                    "role": "user",
                    "content": [
                        {
                            "type": "text",
                            "text": prompt
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
        )
        start = raw.find("{")
        end = raw.rfind("}") + 1
        if start >= 0 and end > start:
            return json.loads(raw[start:end])
    except LLMRoleNotConfigured as e:
        logger.info("image extraction skipped: %s", e)
        return not_configured_payload(e.role)
    except Exception as e:
        logger.warning("image extraction failed: %s", e)
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

    prompt = await _get_user_prompt(pool, user_id, "invoice")
    result = None

    if is_text_pdf and extracted_text:
        result = await extract_from_text(extracted_text, prompt)
    else:
        # Bild-PDF: Download aus S3
        image_bytes = await download_s3_doc(s3_bucket, s3_key)
        if image_bytes:
            result = await extract_from_image(image_bytes, prompt=prompt)

        # Fallback: Falls Bild-Extraktion fehlschlaegt aber extracted_text vorhanden
        if (not result or _is_not_configured_payload(result)) and extracted_text:
            logger.info("image extraction failed, falling back to text extraction for %s", doc_id)
            result = await extract_from_text(extracted_text, prompt)

    if _is_not_configured_payload(result):
        logger.info("document extraction skipped for %s: %s", doc_id, result["error"])
        return False
    if not result:
        logger.info("document extraction failed for %s", doc_id)
        return False
    if not result.get("products"):
        logger.info("no products extracted from document %s", doc_id)
        return True

    # Gear-Vorschlaege in DB schreiben
    async with pool.acquire() as conn:
        for product in result["products"]:
            if not product.get("name"):
                continue
            type_key, scope = _gear_type_and_scope(product.get("type"))
            await conn.execute(
                """
                INSERT INTO gear (id, user_id, name, type, scope, brand, model,
                                  purchase_price, currency, review_status,
                                  source_document_id, created_at)
                VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6,
                        $7, $8, 'pending_review', $9, NOW())
                ON CONFLICT DO NOTHING
                """,
                user_id,
                product.get("name", "Unbekanntes Produkt"),
                type_key,
                scope,
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


MARKDOWN_PROMPT = """Konvertiere den folgenden extrahierten Text aus einem PDF-Dokument in sauberes Markdown.
Behalte die Struktur bei: Überschriften, Tabellen, Listen.
Wenn es eine Rechnung ist, formatiere Positionen als Markdown-Tabelle.
Gib NUR das Markdown zurück, keine Erklärungen."""


async def convert_to_markdown(text: str) -> Optional[str]:
    """Konvertiert Rohtext in strukturiertes Markdown via LLM."""
    if not text or len(text.strip()) < 50:
        return None

    try:
        result = await _chat_via_broker(
            ML_MARKDOWN_ROLE,
            "markdown_extract",
            [
                {
                    "role": "user",
                    "content": MARKDOWN_PROMPT + "\n\n" + text[:8000],
                }
            ],
        )
        if len(result.strip()) > 20:
            return result.strip()
    except LLMRoleNotConfigured as e:
        logger.info("markdown conversion skipped: %s", e)
    except Exception as e:
        logger.warning("markdown conversion failed: %s", e)

    return None


async def upload_to_s3(bucket: str, key: str, data: bytes, content_type: str) -> None:
    """Lädt Daten in S3/Garage hoch."""
    session = aioboto3.Session()
    async with session.client(
        "s3",
        endpoint_url=os.getenv("S3_ENDPOINT", os.getenv("GARAGE_ENDPOINT", "http://garage:3900")),
        aws_access_key_id=os.getenv("S3_ACCESS_KEY", os.getenv("GARAGE_ACCESS_KEY")),
        aws_secret_access_key=os.getenv("S3_SECRET_KEY", os.getenv("GARAGE_SECRET_KEY")),
        region_name="garage",
    ) as s3:
        await s3.put_object(Bucket=bucket, Key=key, Body=data, ContentType=content_type)
