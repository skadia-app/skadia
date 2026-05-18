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
    client = TestClient(app_main.app)

    resp = client.get("/status", headers={"X-ML-Callback-Secret": "status-secret"})

    assert resp.status_code == 200
    body = resp.json()
    assert body["service"] == "ml"
    assert body["git_commit"] == "abc123"
    assert body["commit_time"] == "2026-05-17T09:58:00Z"
    assert body["build_time"] == "2026-05-17T10:00:00Z"


def test_status_requires_internal_token(monkeypatch):
    monkeypatch.setenv("ML_CALLBACK_SECRET", "status-secret")
    app_main = reload_main_with_env()
    client = TestClient(app_main.app)

    assert client.get("/status").status_code == 401
