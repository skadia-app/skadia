"""LLM-Pipeline fuer Gear-Dokumente (Rechnungen, Handbuecher).

Ablauf:
1. Text-PDF (is_text_pdf=True): extracted_text -> document_extract role -> JSON
2. Bild-PDF (is_text_pdf=False): S3-Download -> vision role -> JSON
Fallback: Bild-Extraktion fehlgeschlagen + extracted_text vorhanden -> Text-Extraktion
"""
import asyncio
import base64
import json
import logging
import os
import subprocess
import tempfile
from pathlib import Path
from typing import Optional

import aioboto3

from config import (
    LLMRoleNotConfigured,
    ML_DOCUMENT_ROLE,
    ML_MARKDOWN_ROLE,
    ML_VISION_ROLE,
    call_llm_broker,
)

logger = logging.getLogger(__name__)

MAX_VISION_IMAGE_BYTES = int(os.getenv("ML_DOCUMENT_VISION_MAX_IMAGE_BYTES", "700000"))
MAX_DOCUMENT_DOWNLOAD_BYTES = int(os.getenv("ML_DOCUMENT_MAX_DOWNLOAD_BYTES", str(30 << 20)))
MAX_SCANNED_PDF_PAGES = int(os.getenv("ML_DOCUMENT_SCANNED_PDF_MAX_PAGES", "3"))
SCANNED_PDF_RENDER_DPI = int(os.getenv("ML_DOCUMENT_SCANNED_PDF_RENDER_DPI", "120"))
SCANNED_PDF_RENDER_TIMEOUT_SECONDS = float(os.getenv("ML_DOCUMENT_SCANNED_PDF_RENDER_TIMEOUT_SECONDS", "30"))


class RetryableDocumentError(RuntimeError):
    """Raised for transient document-processing failures that should be redelivered."""

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
        raise RetryableDocumentError("text extraction broker call failed") from e
    return None


async def download_s3_doc(s3_bucket: str, s3_key: str) -> Optional[bytes]:
    """Lade Dokument aus Garage S3."""
    try:
        session = aioboto3.Session()
        async with session.client(
            "s3",
            endpoint_url=os.getenv("S3_ENDPOINT", os.getenv("GARAGE_ENDPOINT", "http://garage:3900")),
            aws_access_key_id=os.getenv("S3_ACCESS_KEY", os.getenv("GARAGE_ACCESS_KEY")),
            aws_secret_access_key=os.getenv("S3_SECRET_KEY", os.getenv("GARAGE_SECRET_KEY")),
            region_name=os.getenv("S3_REGION", "garage"),
        ) as s3:
            response = await s3.get_object(Bucket=s3_bucket, Key=s3_key)
            content_length = response.get("ContentLength")
            if content_length is not None and int(content_length) > MAX_DOCUMENT_DOWNLOAD_BYTES:
                logger.warning(
                    "S3 document exceeds %d bytes: %s/%s",
                    MAX_DOCUMENT_DOWNLOAD_BYTES,
                    s3_bucket,
                    s3_key,
                )
                body = response.get("Body")
                if body and hasattr(body, "close"):
                    body.close()
                return None
            body = response["Body"]
            chunks: list[bytes] = []
            total = 0
            while True:
                chunk = await body.read(64 * 1024)
                if not chunk:
                    break
                total += len(chunk)
                if total > MAX_DOCUMENT_DOWNLOAD_BYTES:
                    logger.warning(
                        "S3 document exceeds %d bytes: %s/%s",
                        MAX_DOCUMENT_DOWNLOAD_BYTES,
                        s3_bucket,
                        s3_key,
                    )
                    if hasattr(body, "close"):
                        body.close()
                    return None
                chunks.append(chunk)
            if hasattr(body, "close"):
                body.close()
            return b"".join(chunks)
    except Exception as e:
        logger.warning("S3 download failed for %s/%s: %s", s3_bucket, s3_key, e)
        raise RetryableDocumentError("S3 document download failed") from e
    return None


def _s3_bucket_for_file_type(file_type: str) -> str:
    default_bucket = os.getenv("S3_BUCKET", "skadia-originals")
    if file_type in {"activity", "health"}:
        return os.getenv("S3_BUCKET_ACTIVITY") or default_bucket
    if file_type in {"media", "document", "thumb", "preview"}:
        return os.getenv("S3_BUCKET_MEDIA") or default_bucket
    return default_bucket


def _local_storage_base_path() -> str:
    return (
        os.getenv("ML_SHADOWFS_BASE_PATH")
        or os.getenv("LIBRARY_BASE_PATH")
        or os.getenv("SHADOW_FS_BASE_PATH")
        or ""
    )


async def download_user_file(storage_backend: str, storage_path: str, file_type: str, size_bytes: int | None = None) -> Optional[bytes]:
    """Load bytes for a user_files row without relying on legacy event S3 fields."""
    storage_backend = (storage_backend or "").strip()
    storage_path = (storage_path or "").strip()
    if not storage_backend or not storage_path:
        return None
    if size_bytes is not None and size_bytes > MAX_DOCUMENT_DOWNLOAD_BYTES:
        logger.warning("document metadata exceeds %d bytes: %s", MAX_DOCUMENT_DOWNLOAD_BYTES, storage_path)
        return None
    if storage_backend == "s3":
        return await download_s3_doc(_s3_bucket_for_file_type(file_type), storage_path)
    if storage_backend == "local":
        base = _local_storage_base_path()
        if not base:
            logger.info("local ShadowFS document storage is not mounted in ML service")
            return None
        base_path = Path(base).resolve()
        full_path = (base_path / storage_path).resolve()
        try:
            full_path.relative_to(base_path)
        except ValueError:
            logger.warning("blocked document storage path traversal: %s", storage_path)
            return None
        try:
            return await asyncio.to_thread(_read_limited_file, full_path, MAX_DOCUMENT_DOWNLOAD_BYTES)
        except RetryableDocumentError:
            raise
        except Exception as e:
            logger.warning("local document read failed for %s: %s", storage_path, e)
            return None
    logger.info("unsupported document storage backend %s", storage_backend)
    return None


def _read_limited_file(path: Path, max_bytes: int) -> bytes:
    with path.open("rb") as f:
        data = f.read(max_bytes + 1)
    if len(data) > max_bytes:
        raise ValueError(f"document exceeds {max_bytes} bytes")
    return data


def _is_pdf_bytes(data: bytes) -> bool:
    return data.lstrip().startswith(b"%PDF-")


def _is_image_payload_allowed(image_bytes: bytes, content_type: str) -> bool:
    if not image_bytes:
        return False
    if _is_pdf_bytes(image_bytes) or content_type == "application/pdf":
        logger.info("refusing raw PDF bytes for vision payload; render pages first")
        return False
    if len(image_bytes) > MAX_VISION_IMAGE_BYTES:
        logger.warning("vision image payload too large: %d bytes", len(image_bytes))
        return False
    return True


def _render_pdf_pages_sync(pdf_bytes: bytes) -> list[bytes]:
    if not pdf_bytes or len(pdf_bytes) > MAX_DOCUMENT_DOWNLOAD_BYTES:
        return []
    with tempfile.TemporaryDirectory() as tmpdir:
        tmp_path = Path(tmpdir)
        pdf_path = tmp_path / "document.pdf"
        pdf_path.write_bytes(pdf_bytes)
        try:
            subprocess.run(
                [
                    "pdftoppm",
                    "-jpeg",
                    "-f",
                    "1",
                    "-l",
                    str(MAX_SCANNED_PDF_PAGES),
                    "-r",
                    str(SCANNED_PDF_RENDER_DPI),
                    str(pdf_path),
                    str(tmp_path / "page"),
                ],
                check=True,
                capture_output=True,
                timeout=SCANNED_PDF_RENDER_TIMEOUT_SECONDS,
            )
        except (FileNotFoundError, subprocess.CalledProcessError, subprocess.TimeoutExpired) as e:
            logger.warning("scanned PDF render unavailable or failed: %s", e)
            return []
        rendered: list[bytes] = []
        for page in sorted(tmp_path.glob("page-*.jpg"))[:MAX_SCANNED_PDF_PAGES]:
            data = page.read_bytes()
            if len(data) <= MAX_VISION_IMAGE_BYTES:
                rendered.append(data)
            else:
                logger.warning("rendered PDF page too large: %d bytes", len(data))
        return rendered


async def render_pdf_pages(pdf_bytes: bytes) -> list[bytes]:
    return await asyncio.to_thread(_render_pdf_pages_sync, pdf_bytes)


async def extract_from_image(
    image_bytes: bytes,
    prompt: str = EXTRACTION_PROMPT,
    content_type: str = "image/jpeg",
) -> Optional[dict]:
    """Extrahiere Produktdaten aus Bild-PDF via vision role."""
    if not _is_image_payload_allowed(image_bytes, content_type):
        return None

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
                                "url": f"data:{content_type};base64,{b64}"
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
        raise RetryableDocumentError("image extraction broker call failed") from e
    return None


def merge_extraction_results(results: list[dict]) -> Optional[dict]:
    """Merge bounded per-page extraction results into one document result."""
    merged: dict = {"products": []}
    for result in results:
        if not isinstance(result, dict) or _is_not_configured_payload(result):
            continue
        products = result.get("products") or []
        if isinstance(products, list):
            merged["products"].extend(products)
        for key in ("invoice_date", "vendor", "total"):
            if merged.get(key) in (None, "") and result.get(key) not in (None, ""):
                merged[key] = result.get(key)
    if merged["products"] or any(key in merged for key in ("invoice_date", "vendor", "total")):
        return merged
    return None


async def process_document(
    pool,
    doc_id: str,
    gear_id: Optional[str],
    component_id: Optional[str],
    user_id: str,
    is_text_pdf: bool,
    extracted_text: Optional[str],
    file_id: str,
    storage_backend: str,
    storage_path: str,
    file_type: str,
    content_type: str,
    size_bytes: int | None = None,
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
        file_bytes = await download_user_file(storage_backend, storage_path, file_type, size_bytes=size_bytes)
        if file_bytes:
            if content_type == "application/pdf" or _is_pdf_bytes(file_bytes):
                page_results: list[dict] = []
                for page_bytes in await render_pdf_pages(file_bytes):
                    page_result = await extract_from_image(page_bytes, prompt=prompt, content_type="image/jpeg")
                    if _is_not_configured_payload(page_result):
                        result = page_result
                        break
                    if page_result:
                        page_results.append(page_result)
                if page_results:
                    result = merge_extraction_results(page_results)
            else:
                result = await extract_from_image(file_bytes, prompt=prompt, content_type=content_type or "image/jpeg")

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

    products = result.get("products") or []
    async with pool.acquire() as conn:
        await conn.execute(
            """
            UPDATE gear_documents
            SET extracted_data = $1::jsonb, products_found = $2
            WHERE id = $3 AND user_id = $4 AND file_id = $5
            """,
            json.dumps(result, separators=(",", ":")),
            len(products),
            doc_id,
            user_id,
            file_id,
        )

    if not products:
        logger.info("no products extracted from document %s", doc_id)
        return True

    # Gear-Vorschlaege in DB schreiben
    async with pool.acquire() as conn:
        for product in products:
            if not product.get("name"):
                continue
            type_key, scope = _gear_type_and_scope(product.get("type"))
            await conn.execute(
                """
                INSERT INTO gear (id, user_id, name, type, scope, brand, model,
                                  purchase_price, currency, review_status,
                                  source_document_id, created_at)
                SELECT gen_random_uuid(), $1, $2, $3, $4, $5, $6,
                       $7, $8, 'pending_review', $9, NOW()
                WHERE NOT EXISTS (
                    SELECT 1
                    FROM gear
                    WHERE user_id = $1
                      AND source_document_id = $9
                      AND name = $2
                      AND type = $3
                      AND brand IS NOT DISTINCT FROM $5
                      AND model IS NOT DISTINCT FROM $6
                      AND purchase_price IS NOT DISTINCT FROM $7
                      AND currency IS NOT DISTINCT FROM $8
                )
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
            len(products),
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
