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
    ):
        monkeypatch.delenv(key, raising=False)


def configure_s3(monkeypatch):
    clear_storage_env(monkeypatch)
    monkeypatch.setenv("SKADIA_STORAGE_MODE", "s3-local-garage")
    monkeypatch.setenv("S3_ENDPOINT", "http://garage:3900")
    monkeypatch.setenv("S3_ACCESS_KEY", "access")
    monkeypatch.setenv("S3_SECRET_KEY", "secret")
    monkeypatch.setenv("S3_REGION", "us-east-1")
    monkeypatch.setenv("S3_BUCKET", "skadia-originals")


def test_s3_storage_readiness_probes_bucket_before_ready(monkeypatch):
    configure_s3(monkeypatch)
    calls: list[tuple[str, dict]] = []

    class _Client:
        def list_objects_v2(self, **kwargs):
            calls.append(("list_objects_v2", kwargs))
            return {"KeyCount": 0}

    boto3 = types.ModuleType("boto3")
    boto3.client = lambda service, **kwargs: _Client()
    monkeypatch.setitem(sys.modules, "boto3", boto3)

    result = config.get_s3_storage_readiness()

    assert result["status"] == "ready"
    assert calls == [("list_objects_v2", {"Bucket": "skadia-originals", "MaxKeys": 1})]


def test_s3_storage_readiness_degrades_when_probe_fails(monkeypatch):
    configure_s3(monkeypatch)

    class _Client:
        def list_objects_v2(self, **kwargs):
            raise TimeoutError("probe timed out")

    boto3 = types.ModuleType("boto3")
    boto3.client = lambda service, **kwargs: _Client()
    monkeypatch.setitem(sys.modules, "boto3", boto3)

    result = config.get_s3_storage_readiness()

    assert result["status"] == "degraded"
    assert result["error"] == "s3_read_probe_failed"
    assert "probe timed out" in result["detail"]


def test_storage_readiness_degraded_ignores_disabled_backends():
    assert not config.storage_readiness_degraded({
        "local": {"status": "disabled"},
        "s3": {"status": "disabled"},
    })
    assert config.storage_readiness_degraded({
        "local": {"status": "disabled"},
        "s3": {"status": "degraded", "error": "probe_failed"},
    })
