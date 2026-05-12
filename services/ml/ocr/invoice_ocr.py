"""
Invoice OCR Pipeline
Converts PDF invoices to structured Markdown using the API LLM broker.

Usage:
    python -m services.ml.ocr.invoice_ocr <pdf_path> [output_path]
    python -m services.ml.ocr.invoice_ocr invoice.pdf invoice.md

    Or import and call process_invoice() directly.
"""

import argparse
import asyncio
import base64
import json
import re
import subprocess
import tempfile
from pathlib import Path

from config import ML_VISION_ROLE, call_llm_broker

PROMPT_PAGE = (
    "/no_think\n"
    "Convert this invoice page to markdown. Return only the markdown, no explanation, no delimiters.\n\n"
    "RULES:\n"
    "  - Include ALL content: company header (left: address, right: opening hours), invoice metadata, line items, footer.\n"
    "  - Invoice metadata (Datum, Rech-Nr., Fahrzeug, Bearbeiter, Monteur, Kunde) as key-value pairs.\n"
    "  - Line items as HTML table, columns: Menge | Bezeichnung | MWSt. | Einzelpreis | Gesamtpreis\n"
    "    * Labor rows: Menge has unit AW (e.g. '1.5 AW')\n"
    "    * Material rows: plain number; ArtNr. on new line within Bezeichnung cell if present\n"
    "  - Subtotals (Summe Material, Summe Arbeit, Gesamt/Total) as a separate HTML table if present.\n"
    "  - Preserve currency exactly (Fr., CHF, EUR, etc.).\n"
    "  - Do not add commentary outside the document content."
)

PROMPT_PARTS = (
    "/no_think\n"
    "From the invoice markdown below, extract ONLY the physical parts/components "
    "(material rows — NOT labor/service/AW rows).\n"
    "Return a single section '## Extracted Parts' with one HTML table, "
    "columns: ArtNr | Bezeichnung | Menge | Einzelpreis | Gesamtpreis. "
    "Leave ArtNr empty if not in the document. Preserve currency exactly.\n"
    "Return only this section, nothing else.\n\n"
)


def _broker_payload(messages: list, role: str, task_type: str) -> str:
    return json.dumps(
        {
            "role": role,
            "task_type": task_type,
            "messages": messages,
        },
        separators=(",", ":"),
    )


async def _chat_via_broker(
    role: str,
    task_type: str,
    messages: list,
    timeout: float = 300.0,
) -> str:
    return await call_llm_broker(role, task_type, messages, timeout=timeout)


async def _call_vlm(
    messages: list,
    role: str = ML_VISION_ROLE,
    task_type: str = "invoice_page",
    timeout: float = 300.0,
) -> str:
    _broker_payload(messages, role=role, task_type=task_type)
    content = await _chat_via_broker(role, task_type, messages, timeout=timeout)
    return re.sub(r"<think>.*?</think>", "", content, flags=re.DOTALL).strip()


def _call_vlm_sync(
    messages: list,
    role: str = ML_VISION_ROLE,
    task_type: str = "invoice_page",
    timeout: float = 300.0,
) -> str:
    return asyncio.run(_call_vlm(messages, role=role, task_type=task_type, timeout=timeout))


def process_invoice(
    pdf_path: str,
    output_path: str | None = None,
    role: str = ML_VISION_ROLE,
    dpi: int = 150,
) -> str:
    """
    Convert a PDF invoice to Markdown with an Extracted Parts section.

    Args:
        pdf_path:    Path to the input PDF.
        output_path: Where to write the .md file. Defaults to same dir as PDF.
        role:        Broker role to use for the VLM calls.
        dpi:         Resolution for PDF-to-image conversion.

    Returns:
        The generated Markdown string.
    """
    pdf_path = Path(pdf_path)
    if output_path is None:
        output_path = pdf_path.with_suffix(".md")

    with tempfile.TemporaryDirectory() as tmpdir:
        subprocess.run(
            ["pdftoppm", "-jpeg", "-r", str(dpi), str(pdf_path), f"{tmpdir}/page"],
            check=True,
            capture_output=True,
        )
        pages = sorted(Path(tmpdir).glob("page-*.jpg"))
        print(f"  {len(pages)} Seite(n) bei {dpi} DPI")

        page_results = []
        for i, page in enumerate(pages):
            print(f"  Seite {i + 1}...")
            with open(page, "rb") as f:
                b64 = base64.b64encode(f.read()).decode()
            text = _call_vlm_sync(
                [{"role": "user", "content": [
                    {"type": "image_url", "image_url": {"url": f"data:image/jpeg;base64,{b64}"}},
                    {"type": "text", "text": PROMPT_PAGE},
                ]}],
                role=role,
                task_type="invoice_page",
            )
            page_results.append(text)
            print(f"    {len(text)} Zeichen")

    combined = "\n\n---\n\n".join(page_results)

    print("  Teile extrahieren...")
    parts_section = _call_vlm_sync(
        [{"role": "user", "content": PROMPT_PARTS + combined}],
        role=role,
        task_type="invoice_parts",
    )
    print(f"    {len(parts_section)} Zeichen")

    final = combined + "\n\n" + parts_section
    Path(output_path).write_text(final, encoding="utf-8")
    print(f"  Gespeichert: {output_path}")
    return final


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Convert PDF invoice to Markdown via VLM.")
    parser.add_argument("pdf", help="Path to input PDF")
    parser.add_argument("output", nargs="?", help="Output .md path (optional)")
    parser.add_argument("--role", default=ML_VISION_ROLE, help="Broker role for invoice vision tasks")
    parser.add_argument("--dpi", type=int, default=150, help="PDF render DPI (default: 150)")
    args = parser.parse_args()

    result = process_invoice(args.pdf, args.output, role=args.role, dpi=args.dpi)
    print(f"\nFertig ({len(result)} Zeichen)")
