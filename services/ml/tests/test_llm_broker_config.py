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
    assert cfg.ML_LLM_BROKER_URL == "http://api:8080/internal/ai-broker"
    assert not hasattr(cfg, "LLM_BASE_URL")
    assert not hasattr(cfg, "LLM_API_KEY")
    assert not hasattr(cfg, "ML_ANALYSIS_ROLE")
    assert "analysis" not in cfg.configured_llm_roles()


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


def test_llm_broker_readiness_degrades_for_configured_roles_without_token(monkeypatch):
    monkeypatch.delenv("ML_LLM_BROKER_TOKEN", raising=False)
    monkeypatch.delenv("ML_CALLBACK_SECRET", raising=False)

    import config

    cfg = importlib.reload(config)

    assert cfg.llm_broker_readiness_from_statuses(cfg.get_llm_role_statuses_sync()) == {
        "status": "degraded",
        "roles": {
            "document_extract": "not_configured",
            "markdown_extract": "not_configured",
            "vision": "not_configured",
        },
        "error": "broker_token_missing",
    }


def test_llm_broker_readiness_degrades_when_configured_role_unavailable(monkeypatch):
    monkeypatch.setenv("ML_LLM_BROKER_TOKEN", "token")

    import config

    cfg = importlib.reload(config)
    monkeypatch.setattr(cfg, "ML_LLM_ROLES", ("document_extract", "markdown_extract", "vision"))

    assert cfg.llm_broker_readiness_from_statuses(
        {
            "document_extract": "not_configured",
            "markdown_extract": "configured",
            "vision": "configured",
        }
    ) == {
        "status": "degraded",
        "roles": {
            "document_extract": "not_configured",
            "markdown_extract": "configured",
            "vision": "configured",
        },
    }


def test_llm_broker_readiness_allows_explicitly_disabled_roles(monkeypatch):
    import config

    cfg = importlib.reload(config)
    monkeypatch.setattr(cfg, "ML_LLM_ROLES", ("", "", ""))

    assert cfg.llm_broker_readiness_from_statuses({}) == {"status": "disabled", "roles": {}}


def test_local_shadowfs_readiness_reports_missing_configured_path(monkeypatch, tmp_path):
    monkeypatch.setenv("ML_SHADOWFS_BASE_PATH", str(tmp_path / "missing"))
    monkeypatch.delenv("LIBRARY_BASE_PATH", raising=False)
    monkeypatch.delenv("SHADOW_FS_BASE_PATH", raising=False)

    import config

    cfg = importlib.reload(config)

    assert cfg.get_local_shadowfs_readiness() == {
        "status": "degraded",
        "base_path": str(tmp_path / "missing"),
        "error": "missing",
    }


def test_local_shadowfs_readiness_allows_unconfigured_mount(monkeypatch):
    monkeypatch.delenv("ML_SHADOWFS_BASE_PATH", raising=False)
    monkeypatch.delenv("LIBRARY_BASE_PATH", raising=False)
    monkeypatch.delenv("SHADOW_FS_BASE_PATH", raising=False)

    import config

    cfg = importlib.reload(config)

    assert cfg.get_local_shadowfs_readiness() == {"status": "disabled"}
