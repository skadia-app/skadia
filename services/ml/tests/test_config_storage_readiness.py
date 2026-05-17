from __future__ import annotations

import sys
import types

import config


def clear_storage_env(monkeypatch):
    for key in (
        "SKADIA_STORAGE_MODE",
        "SKADIA_S3_PROVIDER",
        "S3_ENDPOINT",
        "GARAGE_ENDPOINT",
        "S3_ACCESS_KEY",
        "S3_SECRET_KEY",
        "GARAGE_ACCESS_KEY",
        "GARAGE_SECRET_KEY",
        "S3_BUCKET",
        "S3_REGION",
        "S3_FORCE_PATH_STYLE",
        "STORAGE_BACKEND",
        "ML_SHADOWFS_BASE_PATH",
        "LIBRARY_BASE_PATH",
        "SHADOW_FS_BASE_PATH",
        "ML_S3_READ_PROBE_KEY",
    ):
        monkeypatch.delenv(key, raising=False)


def configure_s3(monkeypatch):
    clear_storage_env(monkeypatch)
    monkeypatch.setenv("SKADIA_STORAGE_MODE", "s3-local-garage")
    monkeypatch.setenv("STORAGE_BACKEND", "s3")
    monkeypatch.setenv("S3_ENDPOINT", "http://garage:3900")
    monkeypatch.setenv("S3_ACCESS_KEY", "access")
    monkeypatch.setenv("S3_SECRET_KEY", "secret")
    monkeypatch.setenv("S3_REGION", "us-east-1")
    monkeypatch.setenv("S3_BUCKET", "skadia-originals")


def test_s3_storage_readiness_probes_bucket_before_ready(monkeypatch):
    configure_s3(monkeypatch)
    calls: list[tuple[str, dict]] = []

    class _Client:
        def head_bucket(self, **kwargs):
            calls.append(("head_bucket", kwargs))
            return {"KeyCount": 0}

    boto3 = types.ModuleType("boto3")
    boto3.client = lambda service, **kwargs: _Client()
    monkeypatch.setitem(sys.modules, "boto3", boto3)

    result = config.get_s3_storage_readiness()

    assert result["status"] == "ready"
    assert calls == [("head_bucket", {"Bucket": "skadia-originals"})]


def test_s3_storage_readiness_probes_all_shadowfs_buckets(monkeypatch):
    configure_s3(monkeypatch)
    monkeypatch.setenv("S3_BUCKET", "default-bucket")
    monkeypatch.setenv("S3_BUCKET_ACTIVITY", "activity-bucket")
    monkeypatch.setenv("S3_BUCKET_MEDIA", "media-bucket")
    calls: list[str] = []

    class _Client:
        def head_bucket(self, **kwargs):
            calls.append(kwargs["Bucket"])
            return {}

    boto3 = types.ModuleType("boto3")
    boto3.client = lambda service, **kwargs: _Client()
    monkeypatch.setitem(sys.modules, "boto3", boto3)

    result = config.get_s3_storage_readiness()

    assert result["status"] == "ready"
    assert calls == ["default-bucket", "activity-bucket", "media-bucket"]


def test_s3_storage_readiness_uses_read_probe_key_without_list(monkeypatch):
    configure_s3(monkeypatch)
    monkeypatch.setenv("ML_S3_READ_PROBE_KEY", ".skadia-read-probe")
    calls: list[tuple[str, dict]] = []

    class _Body:
        def close(self):
            calls.append(("close", {}))

    class _Client:
        def get_object(self, **kwargs):
            calls.append(("get_object", kwargs))
            return {"Body": _Body()}

        def list_objects_v2(self, **kwargs):  # pragma: no cover - must not be used
            raise AssertionError("readiness must not require ListBucket when read probe key is configured")

    boto3 = types.ModuleType("boto3")
    boto3.client = lambda service, **kwargs: _Client()
    monkeypatch.setitem(sys.modules, "boto3", boto3)

    result = config.get_s3_storage_readiness()

    assert result["status"] == "ready"
    assert calls == [
        ("get_object", {"Bucket": "skadia-originals", "Key": ".skadia-read-probe", "Range": "bytes=0-0"}),
        ("close", {}),
    ]


def test_s3_storage_readiness_degrades_when_probe_fails(monkeypatch):
    configure_s3(monkeypatch)

    class _Client:
        def head_bucket(self, **kwargs):
            raise TimeoutError("probe timed out")

    boto3 = types.ModuleType("boto3")
    boto3.client = lambda service, **kwargs: _Client()
    monkeypatch.setitem(sys.modules, "boto3", boto3)

    result = config.get_s3_storage_readiness()

    assert result["status"] == "degraded"
    assert result["error"] == "s3_read_probe_failed"
    assert "probe timed out" in result["detail"]


def test_storage_readiness_ignores_stale_s3_when_active_backend_is_local(monkeypatch, tmp_path):
    clear_storage_env(monkeypatch)
    monkeypatch.setenv("SKADIA_STORAGE_MODE", "local")
    monkeypatch.setenv("STORAGE_BACKEND", "local")
    monkeypatch.setenv("ML_SHADOWFS_BASE_PATH", str(tmp_path))
    monkeypatch.setenv("S3_ENDPOINT", "https://stale.invalid")
    monkeypatch.setenv("S3_ACCESS_KEY", "stale")
    monkeypatch.setenv("S3_SECRET_KEY", "stale")

    result = config.get_storage_readiness()

    assert result["local"]["status"] == "ready"
    assert result["s3"]["status"] == "disabled"
    assert config.storage_readiness_ready(result)


def test_storage_readiness_degraded_ignores_disabled_backends():
    assert not config.storage_readiness_degraded({
        "local": {"status": "disabled"},
        "s3": {"status": "disabled"},
    })
    assert config.storage_readiness_degraded({
        "local": {"status": "disabled"},
        "s3": {"status": "degraded", "error": "probe_failed"},
    })
