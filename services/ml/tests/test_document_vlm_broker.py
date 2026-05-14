from __future__ import annotations

import subprocess

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
        file_id="file-1",
        storage_backend="s3",
        storage_path="docs/invoice.pdf",
        file_type="document",
        content_type="application/pdf",
    )

    assert ok is False


@pytest.mark.asyncio
async def test_convert_to_markdown_returns_none_when_role_not_configured(monkeypatch):
    async def fake_broker(*args, **kwargs):
        raise document_vlm.LLMRoleNotConfigured("markdown_extract")

    monkeypatch.setattr(document_vlm, "_chat_via_broker", fake_broker)

    assert await document_vlm.convert_to_markdown("Text " * 80) is None


@pytest.mark.asyncio
async def test_process_document_uses_file_metadata_not_legacy_s3_payload(monkeypatch):
    calls: dict[str, object] = {}

    class _Conn:
        async def execute(self, *args):
            calls.setdefault("execute", []).append(args)

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

    async def fake_download(storage_backend: str, storage_path: str, file_type: str, size_bytes: int | None = None):
        calls["download"] = (storage_backend, storage_path, file_type)
        return b"\xff\xd8jpeg\xff\xd9"

    async def fake_extract(*args, **kwargs):
        return {"products": []}

    monkeypatch.setattr(document_vlm, "_get_user_prompt", fake_prompt)
    monkeypatch.setattr(document_vlm, "download_user_file", fake_download)
    monkeypatch.setattr(document_vlm, "extract_from_image", fake_extract)

    ok = await document_vlm.process_document(
        pool=_Pool(),
        doc_id="doc-2",
        gear_id=None,
        component_id=None,
        user_id="user-1",
        is_text_pdf=False,
        extracted_text=None,
        file_id="file-2",
        storage_backend="s3",
        storage_path="documents/invoice.jpg",
        file_type="document",
        content_type="image/jpeg",
    )

    assert ok is True
    assert calls["download"] == ("s3", "documents/invoice.jpg", "document")


@pytest.mark.asyncio
async def test_process_document_persists_extracted_data_and_product_count(monkeypatch):
    executed: list[tuple] = []

    class _Conn:
        async def execute(self, *args):
            executed.append(args)

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
        return {
            "products": [
                {"name": "Brake Pads", "type": "component", "currency": "EUR"},
                {"name": "Frame", "type": "frame", "brand": "Acme"},
            ],
            "vendor": "Bike Shop",
        }

    monkeypatch.setattr(document_vlm, "_get_user_prompt", fake_prompt)
    monkeypatch.setattr(document_vlm, "extract_from_text", fake_extract)

    ok = await document_vlm.process_document(
        pool=_Pool(),
        doc_id="doc-3",
        gear_id=None,
        component_id=None,
        user_id="user-1",
        is_text_pdf=True,
        extracted_text="Rechnungstext " * 20,
        file_id="file-3",
        storage_backend="s3",
        storage_path="documents/invoice.pdf",
        file_type="document",
        content_type="application/pdf",
    )

    assert ok is True
    update_calls = [args for args in executed if "extracted_data" in args[0] and "products_found" in args[0]]
    assert len(update_calls) == 1
    assert '"Bike Shop"' in update_calls[0][1]
    assert update_calls[0][2] == 2
    assert update_calls[0][3] == "doc-3"
    assert "user_id" in update_calls[0][0]
    assert "file_id" in update_calls[0][0]
    assert update_calls[0][4:] == ("user-1", "file-3")


@pytest.mark.asyncio
async def test_process_document_merges_scanned_pdf_page_results(monkeypatch):
    executed: list[tuple] = []

    class _Conn:
        async def execute(self, *args):
            executed.append(args)

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

    async def fake_download(*args, **kwargs):
        return b"%PDF-1.7 scanned"

    async def fake_render(_pdf):
        return [b"page-1", b"page-2"]

    async def fake_extract(page_bytes, **kwargs):
        if page_bytes == b"page-1":
            return {"products": [{"name": "Brake Pads"}], "vendor": "Shop"}
        return {"products": [{"name": "Chain"}], "invoice_date": "2026-05-01"}

    monkeypatch.setattr(document_vlm, "_get_user_prompt", fake_prompt)
    monkeypatch.setattr(document_vlm, "download_user_file", fake_download)
    monkeypatch.setattr(document_vlm, "render_pdf_pages", fake_render)
    monkeypatch.setattr(document_vlm, "extract_from_image", fake_extract)

    ok = await document_vlm.process_document(
        pool=_Pool(),
        doc_id="doc-merge",
        gear_id=None,
        component_id=None,
        user_id="user-1",
        is_text_pdf=False,
        extracted_text=None,
        file_id="file-merge",
        storage_backend="s3",
        storage_path="documents/invoice.pdf",
        file_type="document",
        content_type="application/pdf",
    )

    assert ok is True
    update_calls = [args for args in executed if "extracted_data" in args[0] and "products_found" in args[0]]
    assert len(update_calls) == 1
    assert update_calls[0][2] == 2
    assert '"Brake Pads"' in update_calls[0][1]
    assert '"Chain"' in update_calls[0][1]


@pytest.mark.asyncio
async def test_download_user_file_rejects_s3_size_before_download(monkeypatch):
    async def fake_s3(*args, **kwargs):  # pragma: no cover - must not run
        raise AssertionError("oversized S3 document must be rejected before download")

    monkeypatch.setattr(document_vlm, "download_s3_doc", fake_s3)

    assert await document_vlm.download_user_file(
        "s3",
        "documents/huge.pdf",
        "document",
        size_bytes=document_vlm.MAX_DOCUMENT_DOWNLOAD_BYTES + 1,
    ) is None


@pytest.mark.asyncio
async def test_download_s3_doc_uses_aioboto3_credentials_and_size_cap(monkeypatch):
    calls: dict[str, object] = {}

    class _Body:
        def __init__(self):
            self._chunks = [
                b"x" * document_vlm.MAX_DOCUMENT_DOWNLOAD_BYTES,
                b"y",
                b"",
            ]
            self.closed = False

        async def read(self, _size):
            return self._chunks.pop(0)

        def close(self):
            self.closed = True

    body = _Body()

    class _S3:
        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, tb):
            return False

        async def get_object(self, **kwargs):
            calls["get_object"] = kwargs
            return {"ContentLength": document_vlm.MAX_DOCUMENT_DOWNLOAD_BYTES, "Body": body}

    class _Session:
        def client(self, service, **kwargs):
            calls["client"] = (service, kwargs)
            return _S3()

    monkeypatch.setenv("S3_ENDPOINT", "http://garage.test")
    monkeypatch.setenv("S3_ACCESS_KEY", "access")
    monkeypatch.setenv("S3_SECRET_KEY", "secret")
    monkeypatch.setattr(document_vlm.aioboto3, "Session", lambda: _Session(), raising=False)

    assert await document_vlm.download_s3_doc("bucket", "huge.pdf") is None

    service, kwargs = calls["client"]
    assert service == "s3"
    assert kwargs["endpoint_url"] == "http://garage.test"
    assert kwargs["aws_access_key_id"] == "access"
    assert kwargs["aws_secret_access_key"] == "secret"
    assert calls["get_object"] == {"Bucket": "bucket", "Key": "huge.pdf"}
    assert body.closed is True


@pytest.mark.asyncio
async def test_download_s3_doc_reads_file_secrets_region_and_path_style(monkeypatch, tmp_path):
    calls: dict[str, object] = {}

    access_file = tmp_path / "s3_access_key"
    secret_file = tmp_path / "s3_secret_key"
    access_file.write_text("file-access\n", encoding="utf-8")
    secret_file.write_text("file-secret\n", encoding="utf-8")

    class _Body:
        def __init__(self):
            self._chunks = [b"ok", b""]

        async def read(self, _size):
            return self._chunks.pop(0)

        def close(self):
            return None

    class _S3:
        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, tb):
            return False

        async def get_object(self, **kwargs):
            calls["get_object"] = kwargs
            return {"ContentLength": 2, "Body": _Body()}

    class _Session:
        def client(self, service, **kwargs):
            calls["client"] = (service, kwargs)
            return _S3()

    monkeypatch.setenv("S3_ENDPOINT", "http://garage:3900")
    monkeypatch.delenv("S3_ACCESS_KEY", raising=False)
    monkeypatch.delenv("S3_SECRET_KEY", raising=False)
    monkeypatch.setenv("S3_ACCESS_KEY_FILE", str(access_file))
    monkeypatch.setenv("S3_SECRET_KEY_FILE", str(secret_file))
    monkeypatch.setenv("S3_REGION", "us-east-1")
    monkeypatch.setenv("S3_FORCE_PATH_STYLE", "true")
    monkeypatch.setattr(document_vlm.aioboto3, "Session", lambda: _Session(), raising=False)

    assert await document_vlm.download_s3_doc("bucket", "doc.pdf") == b"ok"

    service, kwargs = calls["client"]
    assert service == "s3"
    assert kwargs["aws_access_key_id"] == "file-access"
    assert kwargs["aws_secret_access_key"] == "file-secret"
    assert kwargs["region_name"] == "us-east-1"
    assert kwargs["endpoint_url"] == "http://garage:3900"
    assert kwargs["config"].s3 == {"addressing_style": "path"}


def test_render_pdf_pages_timeout_returns_empty(monkeypatch):
    def fake_run(*args, **kwargs):
        raise subprocess.TimeoutExpired(cmd="pdftoppm", timeout=1)

    monkeypatch.setattr(document_vlm.subprocess, "run", fake_run)

    assert document_vlm._render_pdf_pages_sync(b"%PDF-1.7") == []


@pytest.mark.asyncio
async def test_extract_from_image_rejects_pdf_bytes_without_render(monkeypatch):
    async def fake_broker(*args, **kwargs):  # pragma: no cover - must not run
        raise AssertionError("raw PDF bytes must not be sent as an image payload")

    monkeypatch.setattr(document_vlm, "_chat_via_broker", fake_broker)

    assert await document_vlm.extract_from_image(b"%PDF-1.7...", prompt="prompt") is None
