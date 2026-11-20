import importlib

from fastapi.testclient import TestClient

import config
import main


def reload_main_with_env():
    importlib.reload(config)
    return importlib.reload(main)


def test_status_returns_build_metadata(monkeypatch):
    monkeypatch.setenv("ML_CALLBACK_SECRET", "status-secret")
    monkeypatch.setenv("SKADIA_GIT_COMMIT", "abc123")
    monkeypatch.setenv("SKADIA_GIT_COMMIT_TIME", "2026-05-17T09:58:00Z")
    monkeypatch.setenv("SKADIA_BUILD_TIME", "2026-05-17T10:00:00Z")
    app_main = reload_main_with_env()

    async def fake_llm_broker_readiness():
        return {"status": "ready", "roles": {"document_extract": "configured"}}

    monkeypatch.setattr(app_main, "get_llm_broker_readiness", fake_llm_broker_readiness)
    client = TestClient(app_main.app)

    resp = client.get("/status", headers={"X-ML-Callback-Secret": "status-secret"})

    assert resp.status_code == 200
    body = resp.json()
    assert body["service"] == "ml"
    assert body["git_commit"] == "abc123"
    assert body["commit_time"] == "2026-05-17T09:58:00Z"
    assert body["build_time"] == "2026-05-17T10:00:00Z"
    assert body["llm_broker"]["status"] == "ready"


def test_status_requires_internal_token(monkeypatch):
    monkeypatch.setenv("ML_CALLBACK_SECRET", "status-secret")
    app_main = reload_main_with_env()
    client = TestClient(app_main.app)

    assert client.get("/status").status_code == 401


def test_status_does_not_leak_exception_details(monkeypatch):
    monkeypatch.setenv("ML_CALLBACK_SECRET", "status-secret")
    app_main = reload_main_with_env()

    async def boom():
        raise RuntimeError("secret-internal-dsn://user:pw@db:5432")

    monkeypatch.setattr(app_main, "get_llm_broker_readiness", boom)
    client = TestClient(app_main.app)

    resp = client.get("/status", headers={"X-ML-Callback-Secret": "status-secret"})
    assert resp.status_code == 200
    body = resp.json()
    assert body["llm_broker"]["status"] == "unavailable"
    assert body["llm_broker"]["error"] == "readiness_check_failed"
    assert "secret-internal-dsn" not in resp.text


def test_readyz_does_not_leak_exception_details(monkeypatch):
    app_main = reload_main_with_env()

    async def boom_pool():
        raise RuntimeError("secret-db-host.internal:5432")

    async def boom_broker():
        raise RuntimeError("secret-broker-token-xyz")

    async def ok_storage():
        return {"status": "ready"}

    monkeypatch.setattr(app_main, "get_pool", boom_pool)
    monkeypatch.setattr(app_main, "get_llm_broker_readiness", boom_broker)
    monkeypatch.setattr(app_main, "get_local_shadowfs_readiness", lambda: {"status": "ready"})
    monkeypatch.setattr(app_main, "get_storage_readiness_async", ok_storage)
    monkeypatch.setattr(app_main, "storage_readiness_degraded", lambda _: False)
    client = TestClient(app_main.app)

    resp = client.get("/readyz")
    assert resp.status_code == 503
    body = resp.json()
    assert body["db"] == "unavailable"
    assert body["llm_broker"]["error"] == "readiness_check_failed"
    assert "secret-db-host" not in resp.text
    assert "secret-broker-token" not in resp.text
