from __future__ import annotations

import importlib


def test_llm_broker_config_uses_roles_and_callback_secret(monkeypatch):
    monkeypatch.delenv("LLM_BASE_URL", raising=False)
    monkeypatch.delenv("LLM_API_KEY", raising=False)
    monkeypatch.setenv("ML_CALLBACK_SECRET", "shared-secret")
    monkeypatch.delenv("ML_LLM_BROKER_TOKEN", raising=False)

    import config

    cfg = importlib.reload(config)

    assert cfg.ML_DOCUMENT_ROLE == "document_extract"
    assert cfg.ML_MARKDOWN_ROLE == "markdown_extract"
    assert cfg.ML_VISION_ROLE == "vision"
    assert cfg.ML_LLM_BROKER_TOKEN == "shared-secret"
    assert not hasattr(cfg, "LLM_BASE_URL")
    assert not hasattr(cfg, "LLM_API_KEY")


def test_role_statuses_do_not_contact_broker_without_token(monkeypatch):
    monkeypatch.delenv("ML_LLM_BROKER_TOKEN", raising=False)
    monkeypatch.delenv("ML_CALLBACK_SECRET", raising=False)

    import config

    cfg = importlib.reload(config)

    assert cfg.get_llm_role_statuses_sync() == {
        "document_extract": "not_configured",
        "markdown_extract": "not_configured",
        "vision": "not_configured",
    }
