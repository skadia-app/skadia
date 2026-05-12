from __future__ import annotations

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
    calls: list[tuple[str, str, list]] = []

    async def fake_broker(role: str, task_type: str, messages: list, timeout: float = 300.0) -> str:
        calls.append((role, task_type, messages))
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
