import sys
import types

import pytest

sys.modules.setdefault("aioboto3", types.ModuleType("aioboto3"))

from enrichment import user_files


@pytest.mark.asyncio
async def test_download_user_file_rejects_size_before_read(monkeypatch, tmp_path):
    monkeypatch.setenv("ML_SHADOWFS_BASE_PATH", str(tmp_path))
    (tmp_path / "user").mkdir()
    (tmp_path / "user" / "synthetic-photo.jpg").write_bytes(b"small")

    got = await user_files.download_user_file(
        "local",
        "user/synthetic-photo.jpg",
        "media",
        size_bytes=11,
        max_bytes=10,
        label="photo",
    )

    assert got is None


@pytest.mark.asyncio
async def test_download_user_file_blocks_local_path_traversal(monkeypatch, tmp_path):
    outside = tmp_path / "synthetic-outside.jpg"
    outside.write_bytes(b"secret")
    base = tmp_path / "library"
    base.mkdir()
    monkeypatch.setenv("ML_SHADOWFS_BASE_PATH", str(base))

    got = await user_files.download_user_file(
        "local",
        "../synthetic-outside.jpg",
        "media",
        size_bytes=6,
        max_bytes=100,
        label="photo",
    )

    assert got is None


@pytest.mark.asyncio
async def test_download_user_file_rejects_local_read_time_oversize_without_retry(monkeypatch, tmp_path):
    monkeypatch.setenv("ML_SHADOWFS_BASE_PATH", str(tmp_path))
    (tmp_path / "user").mkdir()
    (tmp_path / "user" / "synthetic-media.bin").write_bytes(b"01234567890")

    got = await user_files.download_user_file(
        "local",
        "user/synthetic-media.bin",
        "media",
        size_bytes=None,
        max_bytes=10,
        label="photo",
    )

    assert got is None
