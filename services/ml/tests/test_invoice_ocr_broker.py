from __future__ import annotations

import subprocess

import pytest

from ocr import invoice_ocr


def test_invoice_ocr_has_no_default_provider_url_or_model_name():
    assert not hasattr(invoice_ocr, "DEFAULT_API")
    assert "qwen" not in invoice_ocr._broker_payload(
        [{"role": "user", "content": "x"}],
        role="vision",
        task_type="invoice_page",
    )


@pytest.mark.asyncio
async def test_invoice_ocr_call_uses_broker_role(monkeypatch):
    calls: list[tuple[str, str, list, float]] = []

    async def fake_broker(role: str, task_type: str, messages: list, timeout: float = 120.0) -> str:
        calls.append((role, task_type, messages, timeout))
        return "markdown"

    monkeypatch.setattr(invoice_ocr, "_chat_via_broker", fake_broker)

    text = await invoice_ocr._call_vlm(
        [{"role": "user", "content": "invoice"}],
        role="vision",
        task_type="invoice_page",
    )

    assert text == "markdown"
    assert calls[0][0] == "vision"
    assert calls[0][1] == "invoice_page"
    assert calls[0][3] == invoice_ocr.BROKER_TIMEOUT_SECONDS
    assert calls[0][3] <= 120.0


def test_invoice_ocr_rejects_page_images_larger_than_broker_body_limit(tmp_path):
    page = tmp_path / "page-1.jpg"
    page.write_bytes(b"x" * (invoice_ocr.MAX_PAGE_IMAGE_BYTES + 1))

    with pytest.raises(invoice_ocr.InvoiceOCRInputTooLarge):
        invoice_ocr._page_image_message(page)


def test_invoice_ocr_limits_rendered_pages(monkeypatch, tmp_path):
    pages = []
    for idx in range(invoice_ocr.MAX_INVOICE_PAGES + 2):
        page = tmp_path / f"page-{idx:02d}.jpg"
        page.write_bytes(b"\xff\xd8jpeg\xff\xd9")
        pages.append(page)

    render_calls: list[list[str]] = []

    def fake_render(args, **kwargs):
        render_calls.append(args)

    monkeypatch.setattr(invoice_ocr.subprocess, "run", fake_render)
    monkeypatch.setattr(invoice_ocr.tempfile, "TemporaryDirectory", lambda: _ExistingTempDir(tmp_path))

    calls: list[str] = []

    def fake_call(messages, role="vision", task_type="invoice_page", timeout=120.0):
        calls.append(task_type)
        return "markdown"

    monkeypatch.setattr(invoice_ocr, "_call_vlm_sync", fake_call)

    invoice_ocr.process_invoice(str(tmp_path / "invoice.pdf"), str(tmp_path / "invoice.md"))

    assert calls.count("invoice_page") == invoice_ocr.MAX_INVOICE_PAGES
    assert "-l" in render_calls[0]
    assert render_calls[0][render_calls[0].index("-l") + 1] == str(invoice_ocr.MAX_INVOICE_PAGES)


def test_invoice_ocr_bounds_combined_markdown_before_parts_call(monkeypatch, tmp_path):
    page = tmp_path / "page-01.jpg"
    page.write_bytes(b"\xff\xd8jpeg\xff\xd9")

    monkeypatch.setattr(invoice_ocr.subprocess, "run", lambda *args, **kwargs: None)
    monkeypatch.setattr(invoice_ocr.tempfile, "TemporaryDirectory", lambda: _ExistingTempDir(tmp_path))

    calls: list[tuple[str, str]] = []

    def fake_call(messages, role="vision", task_type="invoice_page", timeout=120.0):
        content = messages[0]["content"]
        if task_type == "invoice_page":
            calls.append((task_type, ""))
            return "x" * (invoice_ocr.MAX_COMBINED_MARKDOWN_CHARS + 1000)
        calls.append((task_type, content))
        return "## Extracted Parts\n"

    monkeypatch.setattr(invoice_ocr, "_call_vlm_sync", fake_call)

    invoice_ocr.process_invoice(str(tmp_path / "invoice.pdf"), str(tmp_path / "invoice.md"))

    parts_payload = [payload for task_type, payload in calls if task_type == "invoice_parts"][0]
    assert len(parts_payload) <= len(invoice_ocr.PROMPT_PARTS) + invoice_ocr.MAX_COMBINED_MARKDOWN_CHARS


def test_invoice_ocr_render_timeout_fails_cleanly(monkeypatch, tmp_path):
    def fake_render(*args, **kwargs):
        raise subprocess.TimeoutExpired(cmd="pdftoppm", timeout=1)

    monkeypatch.setattr(invoice_ocr.subprocess, "run", fake_render)

    with pytest.raises(invoice_ocr.InvoiceOCRInputTooLarge):
        invoice_ocr.process_invoice(str(tmp_path / "invoice.pdf"), str(tmp_path / "invoice.md"))


class _ExistingTempDir:
    def __init__(self, path):
        self.path = path

    def __enter__(self):
        return str(self.path)

    def __exit__(self, exc_type, exc, tb):
        return False
