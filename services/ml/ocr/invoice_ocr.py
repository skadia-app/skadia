"""
Invoice OCR Pipeline
Converts PDF invoices to structured Markdown using the local VLM (llama-server).

Usage:
    python -m services.ml.ocr.invoice_ocr <pdf_path> [output_path]
    python -m services.ml.ocr.invoice_ocr invoice.pdf invoice.md

    Or import and call process_invoice() directly.
"""

import argparse
import base64
import json
import re
import subprocess
import tempfile
import urllib.request
from pathlib import Path

DEFAULT_API = "http://localhost:8181/v1/chat/completions"

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


def _call_vlm(messages: list, api_url: str, timeout: int = 300) -> str:
    payload = {
        "model": "qwen3.5",
        "messages": messages,
        "temperature": 0.3,
        "top_p": 0.9,
        "top_k": 20,
        "frequency_penalty": 0.15,
        "presence_penalty": 0.1,
        "stream": False,
    }
    req = urllib.request.Request(
        api_url,
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        result = json.loads(resp.read())
    content = result["choices"][0]["message"]["content"]
    return re.sub(r"<think>.*?</think>", "", content, flags=re.DOTALL).strip()


def process_invoice(
    pdf_path: str,
    output_path: str | None = None,
    api_url: str = DEFAULT_API,
    dpi: int = 150,
) -> str:
    """
    Convert a PDF invoice to Markdown with an Extracted Parts section.

    Args:
        pdf_path:    Path to the input PDF.
        output_path: Where to write the .md file. Defaults to same dir as PDF.
        api_url:     llama-server API endpoint.
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
            text = _call_vlm(
                [{"role": "user", "content": [
                    {"type": "image_url", "image_url": {"url": f"data:image/jpeg;base64,{b64}"}},
                    {"type": "text", "text": PROMPT_PAGE},
                ]}],
                api_url=api_url,
            )
            page_results.append(text)
            print(f"    {len(text)} Zeichen")

    combined = "\n\n---\n\n".join(page_results)

    print("  Teile extrahieren...")
    parts_section = _call_vlm(
        [{"role": "user", "content": PROMPT_PARTS + combined}],
        api_url=api_url,
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
    parser.add_argument("--api", default=DEFAULT_API, help="llama-server API URL")
    parser.add_argument("--dpi", type=int, default=150, help="PDF render DPI (default: 150)")
    args = parser.parse_args()

    result = process_invoice(args.pdf, args.output, api_url=args.api, dpi=args.dpi)
    print(f"\nFertig ({len(result)} Zeichen)")
