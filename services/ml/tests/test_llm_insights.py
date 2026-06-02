import importlib.util
from pathlib import Path


_MODULE_PATH = Path(__file__).resolve().parents[1] / "analytics" / "llm_insights.py"
_SPEC = importlib.util.spec_from_file_location("llm_insights_under_test", _MODULE_PATH)
assert _SPEC is not None
_MODULE = importlib.util.module_from_spec(_SPEC)
assert _SPEC.loader is not None
_SPEC.loader.exec_module(_MODULE)


def test_build_prompt_uses_requested_language_and_preserves_protected_terms():
    prompt = _MODULE.build_prompt(
        "projection",
        {
            "current_ctl": 42.0,
            "projected_ctl_28d": 45.0,
            "trend": "plateau",
            "slope": 0.012,
        },
        locale="fr",
        protected_terms=["VO2max"],
    )

    assert prompt is not None
    assert "Answer in French." in prompt
    assert "CTL" in prompt
    assert "FTP" in prompt
    assert "FIT" in prompt
    assert "GPX" in prompt
    assert "BRouter" in prompt
    assert "MTB" in prompt
    assert "TCX" in prompt
    assert "VO2max" in prompt


def test_build_prompt_falls_back_to_german_for_unsupported_locale():
    prompt = _MODULE.build_prompt(
        "warnings",
        {
            "count": 2,
            "worst_zscore": -2.4,
            "worst_date": "2026-06-01",
        },
        locale="pt-BR",
    )

    assert prompt is not None
    assert "Answer in German." in prompt
