from __future__ import annotations

import pytest

import sys
import types

sys.modules.setdefault("aioboto3", types.ModuleType("aioboto3"))

from enrichment import document_vlm


@pytest.mark.asyncio
async def test_extract_from_text_returns_structured_not_configured(monkeypatch):
    async def fake_broker(*args, **kwargs):
        raise document_vlm.LLMRoleNotConfigured("document_extract")

    monkeypatch.setattr(document_vlm, "_chat_via_broker", fake_broker)

    result = await document_vlm.extract_from_text("Rechnungstext " * 20)

    assert result == {
        "error": {
            "code": "llm_role_not_configured",
            "role": "document_extract",
            "status": "not_configured",
        }
    }


@pytest.mark.asyncio
async def test_process_document_marks_not_configured_as_failed(monkeypatch):
    class _Conn:
        async def execute(self, *args, **kwargs):  # pragma: no cover - must not run
            raise AssertionError("process_document must not write gear rows")

    class _Acquire:
        async def __aenter__(self):
            return _Conn()

        async def __aexit__(self, exc_type, exc, tb):
            return False

    class _Pool:
        def acquire(self):
            return _Acquire()

    async def fake_prompt(*args, **kwargs):
        return document_vlm.EXTRACTION_PROMPT

    async def fake_extract(*args, **kwargs):
        return document_vlm.not_configured_payload("document_extract")

    monkeypatch.setattr(document_vlm, "_get_user_prompt", fake_prompt)
    monkeypatch.setattr(document_vlm, "extract_from_text", fake_extract)

    ok = await document_vlm.process_document(
        pool=_Pool(),
        doc_id="doc-1",
        gear_id=None,
        component_id=None,
        user_id="user-1",
        is_text_pdf=True,
        extracted_text="Rechnungstext " * 20,
        s3_bucket="",
        s3_key="",
    )

    assert ok is False


@pytest.mark.asyncio
async def test_convert_to_markdown_returns_none_when_role_not_configured(monkeypatch):
    async def fake_broker(*args, **kwargs):
        raise document_vlm.LLMRoleNotConfigured("markdown_extract")

    monkeypatch.setattr(document_vlm, "_chat_via_broker", fake_broker)

    assert await document_vlm.convert_to_markdown("Text " * 80) is None
