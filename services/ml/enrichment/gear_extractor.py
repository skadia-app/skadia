"""Extract gear/product information from document text/images using LLM/VLM."""
import json
import logging
import os

import httpx

logger = logging.getLogger(__name__)

LLM_BASE_URL = os.environ.get("LLM_BASE_URL", "http://litellm:4000/v1")
LLM_API_KEY = os.environ.get("LLM_API_KEY", "none")
LLM_MODEL = os.environ.get("ML_LLM_MODEL", "skadia-default")
VLM_MODEL = os.environ.get("ML_VLM_MODEL", "skadia-vlm")

EXTRACTION_PROMPT = """Du bist ein Experte fuer Fahrrad- und Sportausruestung.
Extrahiere alle Produkte/Artikel aus diesem Dokument.

Antworte NUR mit einem JSON-Array. Jedes Produkt hat folgende Felder:
- "name": Produktname (ohne Mengenangabe), Pflichtfeld
- "brand": Marke/Hersteller, null wenn nicht erkennbar
- "model": Modellbezeichnung, null wenn nicht erkennbar
- "price": Einzelpreis als Dezimalzahl (ohne Waehrungszeichen), null wenn nicht erkennbar
- "currency": Waehrung (EUR, USD, CHF etc.), null wenn nicht erkennbar
- "category": Eine von: bike_part, clothing, shoes, accessories, nutrition, electronics, maintenance, other
- "quantity": Anzahl als ganze Zahl (Standard: 1)

Ignoriere Versandkosten, Rabatte, Summenzeilen und Steuerangaben.
Wenn keine Produkte erkennbar sind, antworte mit: []

Beispiel-Antwort:
[
  {"name": "Shimano XT Schaltwerk", "brand": "Shimano", "model": "RD-M8100", "price": 89.99, "currency": "EUR", "category": "bike_part", "quantity": 1}
]"""


def _parse_llm_json(content: str) -> list[dict]:
    """Parse JSON from LLM response, stripping markdown fences if present."""
    content = content.strip()
    # Strip markdown code fences
    if "```json" in content:
        content = content.split("```json")[1].split("```")[0].strip()
    elif "```" in content:
        content = content.split("```")[1].split("```")[0].strip()

    parsed = json.loads(content)
    if isinstance(parsed, list):
        return parsed
    # Handle wrapped format: {"products": [...]}
    if isinstance(parsed, dict):
        for key in ("products", "items", "results"):
            if key in parsed and isinstance(parsed[key], list):
                return parsed[key]
    return []


async def extract_gear_from_text(text: str) -> list[dict]:
    """Extract gear products from document text using text LLM.

    Returns a list of product dicts with keys: name, brand, model, price,
    currency, category, quantity.
    """
    if not text or not text.strip():
        return []

    prompt = EXTRACTION_PROMPT + "\n\nDokument-Text:\n" + text[:4000]

    try:
        async with httpx.AsyncClient(timeout=60.0) as client:
            resp = await client.post(
                f"{LLM_BASE_URL}/chat/completions",
                headers={"Authorization": f"Bearer {LLM_API_KEY}"},
                json={
                    "model": LLM_MODEL,
                    "messages": [{"role": "user", "content": prompt}],
                    "temperature": 0.1,
                    "max_tokens": 1024,
                },
            )
            resp.raise_for_status()
            content = resp.json()["choices"][0]["message"]["content"] or ""
            return _parse_llm_json(content)
    except json.JSONDecodeError as e:
        logger.warning("gear extraction JSON parse error: %s", e)
        return []
    except Exception as e:
        logger.error("gear extraction from text failed: %s", e)
        return []


async def extract_gear_from_image(image_b64: str, mime_type: str = "image/jpeg") -> list[dict]:
    """Extract gear products from document image using VLM.

    Args:
        image_b64: Base64-encoded image/document bytes.
        mime_type: MIME type of the image (e.g. "image/jpeg", "application/pdf").

    Returns a list of product dicts with keys: name, brand, model, price,
    currency, category, quantity.
    """
    prompt = EXTRACTION_PROMPT + "\n\nBitte analysiere das beigefuegte Bild/Dokument und extrahiere alle Produkte."

    try:
        async with httpx.AsyncClient(timeout=120.0) as client:
            resp = await client.post(
                f"{LLM_BASE_URL}/chat/completions",
                headers={"Authorization": f"Bearer {LLM_API_KEY}"},
                json={
                    "model": VLM_MODEL,
                    "messages": [
                        {
                            "role": "user",
                            "content": [
                                {"type": "text", "text": prompt},
                                {
                                    "type": "image_url",
                                    "image_url": {
                                        "url": f"data:{mime_type};base64,{image_b64}"
                                    },
                                },
                            ],
                        }
                    ],
                    "temperature": 0.1,
                    "max_tokens": 1024,
                },
            )
            resp.raise_for_status()
            content = resp.json()["choices"][0]["message"]["content"] or ""
            return _parse_llm_json(content)
    except json.JSONDecodeError as e:
        logger.warning("VLM gear extraction JSON parse error: %s", e)
        return []
    except Exception as e:
        logger.error("VLM gear extraction from image failed: %s", e)
        return []
