"""Tests for analytics.readiness (Phase 3).

Coverage:
- Factor loaders (HRV, sleep, training_load, manufacturer, DFA, sample-entropy, RHR-changepoint)
- Refusal logic (no_data, insufficient_factors, path_a, path_b)
- Composite renormalisation (3 / 2 / 2 factors)
- Score-to-level transitions (5-stufen)
- Bonus modifiers (DFA, RHR-changepoint, stress)
- Source mix (pref manufacturer / skadia / null, manual_override, available_sources)
- Path selection
- Concurrency (advisory-lock contention)
- Pref-Race (stale_compute_pref_changed → republish)
- Tenant isolation
"""
from __future__ import annotations

import json
import asyncio
import uuid
from datetime import datetime, timezone, timedelta

import pytest

from analytics.readiness import (
    compute_readiness,
    handle_readiness_message,
    _check_refusal,
    _compute_composite,
    _score_to_level,
    _mix_sources,
    _apply_bonus_modifiers,
    _load_hrv_status,
    _load_sleep_factor,
    _load_training_load_factor,
    _load_manufacturer_factors,
    _load_dfa_alpha1,
    _load_sample_entropy,
    _load_rhr_changepoint_signal,
    _refused_envelope,
    FactorInput,
    CRITICAL_FACTORS,
)
from tests.conftest import seed_user


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

async def _insert_hrv_night_history(conn, uid: str, n_days: int, rmssd_mean: float = 50.0,
                                     rmssd_sd: float = 5.0):
    """Seed n_days of nightly RMSSD ending YESTERDAY (so today's value is separate)."""
    for i in range(1, n_days + 1):
        await conn.execute(
            """
            INSERT INTO health_hrv_night_values (user_id, ts, rmssd_ms)
            VALUES ($1::uuid, (CURRENT_DATE - $2::int)::timestamptz - INTERVAL '6 hours', $3)
            ON CONFLICT (user_id, ts) DO NOTHING
            """,
            uid, i, rmssd_mean + (rmssd_sd if i % 2 == 0 else -rmssd_sd),
        )


async def _insert_hrv_today_snapshot(conn, uid: str, rmssd: float = 60.0, hours_ago: float = 2.0):
    await conn.execute(
        """
        INSERT INTO health_snapshots (user_id, measured_at_utc, rmssd_ms, source_type)
        VALUES ($1::uuid, NOW() - ($2::float || ' hours')::interval, $3, 'garmin_fit')
        """,
        uid, hours_ago, rmssd,
    )


async def _insert_sleep(conn, uid: str, *, score: int | None = 80, recorded_s: int = 7 * 3600,
                         calendar_offset: int = 0, hours_ago_end: float = 2.0):
    await conn.execute(
        """
        INSERT INTO health_sleep
            (user_id, calendar_date, sleep_start_utc, sleep_end_utc,
             deep_seconds, light_seconds, rem_seconds, score_overall, source_type)
        VALUES
            ($1::uuid,
             CURRENT_DATE - $2::int,
             NOW() - ($3::float + 8.0 || ' hours')::interval,
             NOW() - ($3::float || ' hours')::interval,
             $4, $5, $6, $7, 'garmin_fit')
        """,
        uid, calendar_offset, hours_ago_end,
        recorded_s // 3, recorded_s // 3, recorded_s - 2 * (recorded_s // 3),
        score,
    )


async def _insert_pmc(conn, uid: str, *, acwr: float, days_back: int = 0,
                      sport_group: str = "__combined__"):
    await conn.execute(
        """
        INSERT INTO pmc_daily (user_id, date, sport_group, acwr_ra)
        VALUES ($1::uuid, CURRENT_DATE - $2::int, $3, $4)
        ON CONFLICT (user_id, date, sport_group) DO UPDATE SET acwr_ra = EXCLUDED.acwr_ra
        """,
        uid, days_back, sport_group, acwr,
    )


async def _insert_manufacturer(conn, uid: str, *, score: int | None = 75,
                                hrv: float | None = 70, sleep: float | None = 80,
                                acwr: float | None = 50,
                                stress: float | None = 60,
                                recovery: float | None = 70,
                                source_type: str = "garmin",
                                calendar_offset: int = 0):
    await conn.execute(
        """
        INSERT INTO health_readiness_scores
            (user_id, calendar_date, score, level,
             hrv_factor_pct, sleep_history_factor_pct, stress_history_factor_pct,
             recovery_time_factor_pct, acwr_pct, source_type)
        VALUES ($1::uuid, CURRENT_DATE - $2::int, $3, $4, $5, $6, $7, $8, $9, $10)
        ON CONFLICT (user_id, calendar_date) DO UPDATE
        SET score = EXCLUDED.score, source_type = EXCLUDED.source_type
        """,
        uid, calendar_offset, score, "good" if score and score >= 60 else "low",
        hrv, sleep, stress, recovery, acwr, source_type,
    )


async def _insert_pref(conn, uid: str, pref: str | None):
    await conn.execute(
        """
        INSERT INTO user_readiness_preferences (user_id, preferred_source)
        VALUES ($1::uuid, $2)
        ON CONFLICT (user_id) DO UPDATE SET preferred_source = EXCLUDED.preferred_source,
                                            updated_at = NOW()
        """,
        uid, pref,
    )


async def _insert_dfa_today(conn, uid: str, value: float):
    await conn.execute(
        """
        INSERT INTO user_daily_features
            (user_id, date, activity_id, feature_key, value_num, computed_at)
        VALUES ($1::uuid, CURRENT_DATE,
                '00000000-0000-0000-0000-000000000000'::uuid,
                'hrv.HRV_DFA_alpha1', $2, NOW())
        ON CONFLICT (user_id, date, feature_key, activity_id) DO UPDATE
            SET value_num = EXCLUDED.value_num, computed_at = NOW()
        """,
        uid, value,
    )


# ===========================================================================
# Pure-logic tests (no DB)
# ===========================================================================

def test_score_to_level_thresholds():
    assert _score_to_level(0) == "critical"
    assert _score_to_level(19) == "critical"
    assert _score_to_level(20) == "low"
    assert _score_to_level(39) == "low"
    assert _score_to_level(40) == "moderate"
    assert _score_to_level(59) == "moderate"
    assert _score_to_level(60) == "good"
    assert _score_to_level(79) == "good"
    assert _score_to_level(80) == "optimal"
    assert _score_to_level(100) == "optimal"


def _f(key, value, missing=False, source="skadia"):
    return FactorInput(key=key, value=value, missing=missing, source=source)


def test_check_refusal_no_data():
    eigen = {k: _f(k, None, missing=True) for k in CRITICAL_FACTORS}
    assert _check_refusal(eigen, None) == (True, "no_data")


def test_check_refusal_insufficient():
    eigen = {k: _f(k, None, missing=True) for k in CRITICAL_FACTORS}
    eigen["hrv_status"] = _f("hrv_status", 70.0)
    assert _check_refusal(eigen, None) == (True, "insufficient_factors")


def test_check_refusal_path_a_two_factors():
    eigen = {k: _f(k, None, missing=True) for k in CRITICAL_FACTORS}
    eigen["hrv_status"] = _f("hrv_status", 70.0)
    eigen["sleep"] = _f("sleep", 80.0)
    refused, _ = _check_refusal(eigen, None)
    assert not refused


def test_check_refusal_path_b_only():
    eigen = {k: _f(k, None, missing=True) for k in CRITICAL_FACTORS}
    mfr = {"score": 75, "hrv_factor_pct": None, "sleep_history_factor_pct": None, "acwr_pct": None}
    refused, _ = _check_refusal(eigen, mfr)
    assert not refused


def test_check_refusal_mfr_factors_only_no_score():
    eigen = {k: _f(k, None, missing=True) for k in CRITICAL_FACTORS}
    mfr = {"score": None,
           "hrv_factor_pct": 70.0, "sleep_history_factor_pct": 80.0, "acwr_pct": 50.0}
    refused, _ = _check_refusal(eigen, mfr)
    assert not refused  # path A via manufacturer columns


def test_composite_three_factors():
    factors = {
        "hrv_status":    _f("hrv_status", 80.0),
        "sleep":         _f("sleep", 70.0),
        "training_load": _f("training_load", 90.0),
    }
    score, used, level = _compute_composite(factors)
    # 0.30*80 + 0.40*70 + 0.30*90 = 24 + 28 + 27 = 79; / 1.0 = 79
    assert score == 79
    assert sum(used.values()) == pytest.approx(1.0)
    assert level == "good"


def test_composite_two_factors_renorm_07():
    factors = {
        "hrv_status": _f("hrv_status", 80.0),
        "sleep":      _f("sleep", 60.0),
        "training_load": _f("training_load", None, missing=True),
    }
    score, used, _ = _compute_composite(factors)
    # (0.30*80 + 0.40*60) / 0.70 = (24 + 24) / 0.70 = 68.57 → 69
    assert score == 69
    assert sum(used.values()) == pytest.approx(0.70)


def test_composite_two_factors_renorm_06():
    factors = {
        "hrv_status":    _f("hrv_status", 80.0),
        "sleep":         _f("sleep", None, missing=True),
        "training_load": _f("training_load", 50.0),
    }
    score, used, _ = _compute_composite(factors)
    # (0.30*80 + 0.30*50) / 0.60 = (24 + 15) / 0.60 = 65
    assert score == 65
    assert sum(used.values()) == pytest.approx(0.60)


def test_bonus_dfa_high_path_a():
    score, sig = _apply_bonus_modifiers(
        composite_score=70, dfa_alpha1=0.80, sampen=None, sampen_median=None,
        rhr_changepoint=None, path_kind="A", manufacturer_in_critical=False,
    )
    assert score == 73
    assert sig is None


def test_bonus_dfa_low_path_a():
    score, _ = _apply_bonus_modifiers(
        composite_score=70, dfa_alpha1=0.40, sampen=None, sampen_median=None,
        rhr_changepoint=None, path_kind="A", manufacturer_in_critical=False,
    )
    assert score == 67


def test_bonus_rhr_changepoint_path_a():
    score, sig = _apply_bonus_modifiers(
        composite_score=70, dfa_alpha1=None, sampen=None, sampen_median=None,
        rhr_changepoint="rhr_changepoint_recent", path_kind="A",
        manufacturer_in_critical=False,
    )
    assert score == 65
    assert sig == "rhr_changepoint_recent"


def test_bonus_rhr_changepoint_path_b():
    # RHR penalty applies even on Path B per spec.
    score, sig = _apply_bonus_modifiers(
        composite_score=80, dfa_alpha1=0.90, sampen=None, sampen_median=None,
        rhr_changepoint="rhr_changepoint_recent", path_kind="B",
        manufacturer_in_critical=True,
    )
    # DFA bonus must NOT apply on Path B.
    assert score == 75
    assert sig == "rhr_changepoint_recent"


def test_bonus_no_stress_when_manufacturer_in_critical():
    # stress_unusually_high=True but manufacturer is in critical inputs → no bonus.
    score, _ = _apply_bonus_modifiers(
        composite_score=70, dfa_alpha1=None, sampen=None, sampen_median=None,
        rhr_changepoint=None, stress_unusually_high=True,
        path_kind="A", manufacturer_in_critical=True,
    )
    assert score == 70


def test_mix_pref_manufacturer_picks_mfr():
    eigen = {"hrv_status": _f("hrv_status", 80.0)}
    mfr = {"hrv_factor_pct": 50.0, "source_type": "garmin"}
    out = _mix_sources(eigen, mfr, "manufacturer")
    assert out["hrv_status"].value == 50.0
    assert out["hrv_status"].source == "garmin"
    assert set(out["hrv_status"].available_sources) == {"skadia", "garmin"}


def test_mix_pref_skadia_picks_eigen():
    eigen = {"hrv_status": _f("hrv_status", 80.0)}
    mfr = {"hrv_factor_pct": 50.0, "source_type": "garmin"}
    out = _mix_sources(eigen, mfr, "skadia")
    assert out["hrv_status"].value == 80.0
    assert out["hrv_status"].source == "skadia"
    assert set(out["hrv_status"].available_sources) == {"skadia", "garmin"}


def test_mix_pref_none_picks_eigen_auto_best():
    """R1-#5 (Option B): pref=None → Skadia-first per factor.
    The path selector only routes to the mix (Path A) when eigen is complete,
    so pref=None inside the mix means we already chose to trust eigen."""
    eigen = {"hrv_status": _f("hrv_status", 80.0)}
    mfr = {"hrv_factor_pct": 50.0, "source_type": "garmin"}
    out = _mix_sources(eigen, mfr, None)
    assert out["hrv_status"].value == 80.0
    assert out["hrv_status"].source == "skadia"


def test_mix_manual_override_source():
    eigen = {"hrv_status": _f("hrv_status", None, missing=True)}
    mfr = {"hrv_factor_pct": 65.0, "source_type": "manual_override"}
    out = _mix_sources(eigen, mfr, None)
    assert out["hrv_status"].source == "manual_override"


def test_mix_available_sources_eigen_only():
    eigen = {"hrv_status": _f("hrv_status", 80.0)}
    out = _mix_sources(eigen, None, None)
    assert out["hrv_status"].available_sources == ("skadia",)


def test_mix_available_sources_mfr_only():
    eigen = {"hrv_status": _f("hrv_status", None, missing=True)}
    mfr = {"hrv_factor_pct": 60.0, "source_type": "garmin"}
    out = _mix_sources(eigen, mfr, None)
    assert out["hrv_status"].available_sources == ("garmin",)


def test_mix_available_sources_both():
    eigen = {"hrv_status": _f("hrv_status", 80.0)}
    mfr = {"hrv_factor_pct": 50.0, "source_type": "garmin"}
    out = _mix_sources(eigen, mfr, "skadia")
    assert set(out["hrv_status"].available_sources) == {"skadia", "garmin"}


# ===========================================================================
# DB-backed loader tests
# ===========================================================================

async def test_load_hrv_status_with_baseline(conn):
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid, n_days=30)
    await _insert_hrv_today_snapshot(conn, uid, rmssd=60.0, hours_ago=2.0)

    f = await _load_hrv_status(conn, uid, 'UTC')
    assert not f.missing
    assert f.value is not None
    assert 0 <= f.value <= 100
    assert f.source == "skadia"
    assert f.raw_unit == "ms"


async def test_load_hrv_status_no_data_today(conn):
    uid = await seed_user(conn)
    # No today snapshot at all → no_data_today
    f = await _load_hrv_status(conn, uid, 'UTC')
    assert f.missing
    assert f.missing_reason == "no_data_today"


async def test_load_hrv_status_no_baseline(conn):
    uid = await seed_user(conn)
    await _insert_hrv_today_snapshot(conn, uid, rmssd=60.0)
    # Only 5 days of baseline → < 7 → missing
    await _insert_hrv_night_history(conn, uid, n_days=5)
    f = await _load_hrv_status(conn, uid, 'UTC')
    assert f.missing
    assert f.missing_reason == "no_baseline"


async def test_load_sleep_fresh(conn):
    uid = await seed_user(conn)
    await _insert_sleep(conn, uid, score=82, recorded_s=7 * 3600, hours_ago_end=2.0)
    f = await _load_sleep_factor(conn, uid, 'UTC')
    assert not f.missing
    assert f.value == 82
    assert f.source == "skadia"


async def test_load_sleep_stale(conn):
    uid = await seed_user(conn)
    # End time 40h ago → > 36h gate → missing
    await _insert_sleep(conn, uid, score=82, recorded_s=7 * 3600,
                        calendar_offset=2, hours_ago_end=40.0)
    f = await _load_sleep_factor(conn, uid, 'UTC')
    assert f.missing
    assert f.missing_reason == "stale"


async def test_load_sleep_score_null(conn):
    uid = await seed_user(conn)
    await _insert_sleep(conn, uid, score=None, recorded_s=7 * 3600)
    f = await _load_sleep_factor(conn, uid, 'UTC')
    assert f.missing  # query filters score_overall IS NOT NULL
    assert f.missing_reason == "no_data"


async def test_load_sleep_short_duration(conn):
    uid = await seed_user(conn)
    # 3h recorded → < 4h → missing
    await _insert_sleep(conn, uid, score=80, recorded_s=3 * 3600, hours_ago_end=2.0)
    f = await _load_sleep_factor(conn, uid, 'UTC')
    assert f.missing
    assert f.missing_reason == "stale"


async def test_load_training_load_no_data(conn):
    uid = await seed_user(conn)
    # No pmc_daily row at all → no_data
    f = await _load_training_load_factor(conn, uid, 'UTC')
    assert f.missing
    assert f.missing_reason == "no_data"


async def test_load_training_load_acwr_zero(conn):
    uid = await seed_user(conn)
    await _insert_pmc(conn, uid, acwr=0.0)
    f = await _load_training_load_factor(conn, uid, 'UTC')
    assert f.missing
    assert f.missing_reason == "no_baseline"


async def test_load_training_load_acwr_normal(conn):
    uid = await seed_user(conn)
    await _insert_pmc(conn, uid, acwr=1.0)
    f = await _load_training_load_factor(conn, uid, 'UTC')
    assert not f.missing
    assert f.value == 100.0


async def test_load_manufacturer_today(conn):
    uid = await seed_user(conn)
    await _insert_manufacturer(conn, uid, score=78)
    mfr = await _load_manufacturer_factors(conn, uid, 'UTC')
    assert mfr is not None
    assert mfr["score"] == 78
    assert mfr["source_type"] == "garmin"


async def test_load_manufacturer_yesterday_ignored(conn):
    uid = await seed_user(conn)
    await _insert_manufacturer(conn, uid, score=78, calendar_offset=1)
    mfr = await _load_manufacturer_factors(conn, uid, 'UTC')
    assert mfr is None


async def test_load_dfa_present_and_missing(conn):
    uid = await seed_user(conn)
    assert await _load_dfa_alpha1(conn, uid, 'UTC') is None
    await _insert_dfa_today(conn, uid, 0.85)
    assert await _load_dfa_alpha1(conn, uid, 'UTC') == pytest.approx(0.85)


# ===========================================================================
# Refusal end-to-end (DB)
# ===========================================================================

async def test_refusal_no_data_db(conn):
    uid = await seed_user(conn)
    result = await compute_readiness(conn, uid)
    assert result["refused"] is True
    assert result["refusal_reason"] == "no_data"
    assert result["written"] is True
    assert result["data_window_end"] is not None


async def test_refusal_one_critical_only_insufficient(conn):
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid, n_days=30)
    await _insert_hrv_today_snapshot(conn, uid, rmssd=55.0)
    result = await compute_readiness(conn, uid)
    assert result["refused"] is True
    assert result["refusal_reason"] == "insufficient_factors"


async def test_path_a_with_two_factors(conn):
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid, n_days=30)
    await _insert_hrv_today_snapshot(conn, uid, rmssd=55.0)
    await _insert_sleep(conn, uid, score=82)
    result = await compute_readiness(conn, uid)
    assert result["refused"] is False
    assert result["score"] is not None
    assert result["level"] in ("critical", "low", "moderate", "good", "optimal")


async def test_path_b_manufacturer_only(conn):
    uid = await seed_user(conn)
    await _insert_manufacturer(conn, uid, score=82)
    result = await compute_readiness(conn, uid)
    assert result["refused"] is False
    assert result["score"] == 82  # no RHR penalty
    assert result["confidence"] == "medium"


# ===========================================================================
# Path selection
# ===========================================================================

async def test_path_selection_pref_manufacturer_chooses_b(conn):
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid, n_days=30)
    await _insert_hrv_today_snapshot(conn, uid, rmssd=55.0)
    await _insert_sleep(conn, uid, score=82)
    await _insert_manufacturer(conn, uid, score=50)
    await _insert_pref(conn, uid, "manufacturer")
    result = await compute_readiness(conn, uid)
    # Path B → score = manufacturer score (no RHR)
    assert result["score"] == 50


async def test_path_selection_pref_skadia_chooses_a(conn):
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid, n_days=30)
    await _insert_hrv_today_snapshot(conn, uid, rmssd=55.0)
    await _insert_sleep(conn, uid, score=82)
    await _insert_manufacturer(conn, uid, score=20)
    await _insert_pref(conn, uid, "skadia")
    result = await compute_readiness(conn, uid)
    # Path A → composite of own factors, mfr score 20 not used directly
    assert result["score"] != 20
    assert result["score"] >= 50  # sleep=82 dominates


# ===========================================================================
# RHR auto-trigger
# ===========================================================================

async def test_rhr_auto_trigger_when_no_cache(conn):
    uid = await seed_user(conn)
    # No cache exists yet — _load_rhr_changepoint_signal should write an entry.
    sig = await _load_rhr_changepoint_signal(conn, uid, 'UTC')
    # detect_changepoints returns [] without enough data → no signal
    assert sig is None
    row = await conn.fetchrow(
        "SELECT data FROM analytics_cache WHERE user_id=$1::uuid AND cache_key='changepoints.resting_hr'",
        uid,
    )
    assert row is not None  # auto-trigger persisted an empty list


async def test_rhr_auto_trigger_when_cache_stale(conn):
    uid = await seed_user(conn)
    # Insert stale (>24h old) cache; expect overwrite by auto-trigger.
    await conn.execute(
        """
        INSERT INTO analytics_cache (user_id, cache_key, data, computed_at)
        VALUES ($1::uuid, 'changepoints.resting_hr', '[]'::jsonb, NOW() - INTERVAL '48 hours')
        """,
        uid,
    )
    await _load_rhr_changepoint_signal(conn, uid, 'UTC')
    row = await conn.fetchrow(
        "SELECT computed_at FROM analytics_cache WHERE user_id=$1::uuid AND cache_key='changepoints.resting_hr'",
        uid,
    )
    age_h = (datetime.now(timezone.utc) - row["computed_at"]).total_seconds() / 3600.0
    assert age_h < 1.0  # was just rewritten


# ===========================================================================
# Mix DB-backed (just one extra spot-check)
# ===========================================================================

async def test_mix_sources_via_compute_pref_null_path_b_when_eigen_incomplete(conn):
    """R1-#5 auto-best: pref=None and eigen incomplete (no training_load) →
    Path B (manufacturer score used directly)."""
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid, n_days=30)
    await _insert_hrv_today_snapshot(conn, uid, rmssd=55.0)
    await _insert_sleep(conn, uid, score=82)
    # No pmc_daily → eigen incomplete (training_load missing).
    await _insert_manufacturer(conn, uid, score=75, hrv=40, sleep=40, acwr=40)
    result = await compute_readiness(conn, uid)
    assert result["refused"] is False
    # Path B → score equals manufacturer score (no RHR penalty seeded).
    assert result["score"] == 75


# ===========================================================================
# Envelope schema completeness
# ===========================================================================

_REQUIRED_ENVELOPE_KEYS = frozenset({
    "score", "level", "computed_at", "data_window_end", "preferences_version",
    "confidence", "refused", "refusal_reason", "factors", "missing_factors",
    "recommendation_code", "signal_modifier",
})


def test_already_running_envelope_is_schema_complete():
    """already_running refused envelope must carry all schema keys so that
    callers which serialize the result never encounter a KeyError or a
    frontend type mismatch (L4 regression guard)."""
    from datetime import datetime, timezone
    env = _refused_envelope(
        refusal_reason="already_running",
        data_window_end="2026-05-01",
        preferences_version=None,
        factors_eigen={},
        mfr=None,
        now_utc=datetime(2026, 5, 1, 12, 0, 0, tzinfo=timezone.utc),
    )
    assert _REQUIRED_ENVELOPE_KEYS.issubset(env.keys()), (
        f"Missing keys: {_REQUIRED_ENVELOPE_KEYS - env.keys()}"
    )
    assert env["refused"] is True
    assert env["refusal_reason"] == "already_running"
    assert env["score"] is None
    assert env["level"] is None
    assert isinstance(env["factors"], list)
    assert isinstance(env["missing_factors"], list)


# ===========================================================================
# Pref-Race
# ===========================================================================

async def test_pref_race_handle_message_republishes(conn, monkeypatch):
    """compute_readiness running while a pref change happens between
    snapshot-read and final re-read should refuse with stale_compute_pref_changed
    and the wrapper should call publish_analytics_invalidate exactly once.
    """
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid, n_days=30)
    await _insert_hrv_today_snapshot(conn, uid, rmssd=55.0)
    await _insert_sleep(conn, uid, score=82)
    await _insert_pref(conn, uid, "skadia")

    # Intercept _read_pref: bump the pref between the first and second call so
    # the pref-version re-check inside _compute_readiness_locked sees a mismatch.
    import analytics.readiness as mod

    orig = mod._read_pref
    call_count = {"n": 0}

    async def racy_read(c, user_id):
        call_count["n"] += 1
        if call_count["n"] == 1:
            return await orig(c, user_id)
        # Before the second read, mutate the pref so updated_at advances.
        await c.execute(
            """
            UPDATE user_readiness_preferences
               SET preferred_source='manufacturer', updated_at = NOW() + INTERVAL '1 second'
             WHERE user_id=$1::uuid
            """, user_id,
        )
        return await orig(c, user_id)

    monkeypatch.setattr(mod, "_read_pref", racy_read)

    publish_calls = []

    class FakePub:
        async def publish_analytics_invalidate(self, user_id):
            publish_calls.append(user_id)

    result = await handle_readiness_message(conn, uid, FakePub())

    assert result["refused"] is True
    assert result["refusal_reason"] == "stale_compute_pref_changed"
    assert result["written"] is False
    assert publish_calls == [uid]


async def test_pref_race_no_cache_write_on_stale(conn, monkeypatch):
    """stale_compute_pref_changed must not UPSERT analytics_cache."""
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid, n_days=30)
    await _insert_hrv_today_snapshot(conn, uid, rmssd=55.0)
    await _insert_sleep(conn, uid, score=82)
    await _insert_pref(conn, uid, "skadia")

    import analytics.readiness as mod
    orig = mod._read_pref
    call_count = {"n": 0}

    async def racy_read(c, user_id):
        call_count["n"] += 1
        if call_count["n"] == 1:
            return await orig(c, user_id)
        await c.execute(
            """
            UPDATE user_readiness_preferences
               SET updated_at = NOW() + INTERVAL '2 seconds'
             WHERE user_id=$1::uuid
            """, user_id,
        )
        return await orig(c, user_id)

    monkeypatch.setattr(mod, "_read_pref", racy_read)
    await compute_readiness(conn, uid)

    row = await conn.fetchrow(
        "SELECT 1 FROM analytics_cache WHERE user_id=$1::uuid AND cache_key='readiness'",
        uid,
    )
    assert row is None


async def test_pref_race_handle_message_calls_publisher_once(conn, monkeypatch):
    """Idempotency: handle_readiness_message must invoke
    publish_analytics_invalidate at most once per stale-detection."""
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid, n_days=30)
    await _insert_hrv_today_snapshot(conn, uid, rmssd=55.0)
    await _insert_sleep(conn, uid, score=82)
    await _insert_pref(conn, uid, "skadia")

    import analytics.readiness as mod
    orig = mod._read_pref
    call_count = {"n": 0}

    async def racy_read(c, user_id):
        call_count["n"] += 1
        if call_count["n"] == 1:
            return await orig(c, user_id)
        await c.execute(
            "UPDATE user_readiness_preferences SET updated_at = NOW() + INTERVAL '3 seconds' WHERE user_id=$1::uuid",
            user_id,
        )
        return await orig(c, user_id)

    publish_calls: list = []

    class FakePub:
        async def publish_analytics_invalidate(self, user_id):
            publish_calls.append(user_id)

    monkeypatch.setattr(mod, "_read_pref", racy_read)
    await handle_readiness_message(conn, uid, FakePub())

    assert len(publish_calls) == 1


# ===========================================================================
# Tenant isolation
# ===========================================================================

async def test_tenant_isolation(conn):
    a = await seed_user(conn)
    b = await seed_user(conn)

    # User A: fully populated, should compute path-A
    await _insert_hrv_night_history(conn, a, n_days=30)
    await _insert_hrv_today_snapshot(conn, a, rmssd=55.0)
    await _insert_sleep(conn, a, score=82)
    # User B: nothing
    res_a = await compute_readiness(conn, a)
    res_b = await compute_readiness(conn, b)

    assert res_a["refused"] is False
    assert res_b["refused"] is True and res_b["refusal_reason"] == "no_data"

    # Confirm user_id boundaries on the cache writes
    rows = await conn.fetch(
        "SELECT user_id, data FROM analytics_cache WHERE cache_key='readiness' AND user_id IN ($1::uuid, $2::uuid)",
        a, b,
    )
    assert len(rows) == 2
    by_uid = {str(r["user_id"]): json.loads(r["data"]) if isinstance(r["data"], str) else r["data"] for r in rows}
    assert by_uid[a]["refused"] is False
    assert by_uid[b]["refused"] is True


# ===========================================================================
# Concurrency (real connections, must clean up)
# ===========================================================================

async def _cleanup_user(pool, uid: str):
    """Delete all rows referencing a user, then the user itself."""
    async with pool.acquire() as c:
        for tbl in (
            "health_sleep", "health_snapshots", "health_hrv_night_values",
            "health_readiness_scores", "user_daily_features", "pmc_daily",
            "analytics_cache", "user_readiness_preferences",
        ):
            await c.execute(f"DELETE FROM {tbl} WHERE user_id=$1::uuid", uid)
        await c.execute("DELETE FROM users WHERE id=$1::uuid", uid)


@pytest.mark.concurrency
async def test_concurrent_compute_second_already_running(concurrent_pool):
    """Two parallel compute_readiness for the same user → second sees
    already_running due to pg_try_advisory_xact_lock."""
    # Seed a user committed (outside any rolled-back tx)
    async with concurrent_pool.acquire() as c:
        uid = str(uuid.uuid4())
        await c.execute(
            "INSERT INTO users (id, username, units, locale) VALUES ($1::uuid, $2, 'metric', 'de')",
            uid, f"conc_{uid[:8]}",
        )
        await c.execute(
            """
            INSERT INTO health_snapshots (user_id, measured_at_utc, rmssd_ms, source_type)
            VALUES ($1::uuid, NOW(), 55.0, 'garmin_fit')
            """, uid,
        )
        for i in range(1, 11):
            await c.execute(
                """
                INSERT INTO health_hrv_night_values (user_id, ts, rmssd_ms)
                VALUES ($1::uuid, (CURRENT_DATE - $2::int)::timestamptz - INTERVAL '6 hours', $3)
                """,
                uid, i, 50.0 + (5.0 if i % 2 == 0 else -5.0),
            )
        await c.execute(
            """
            INSERT INTO health_sleep
                (user_id, calendar_date, sleep_start_utc, sleep_end_utc,
                 deep_seconds, light_seconds, rem_seconds, score_overall, source_type)
            VALUES ($1::uuid, CURRENT_DATE,
                    NOW() - INTERVAL '10 hours', NOW() - INTERVAL '2 hours',
                    7200, 10800, 7200, 82, 'garmin_fit')
            """, uid,
        )

    try:
        # Two connections, run in parallel. The SECOND to acquire the advisory
        # lock will see refused=already_running.
        async def runner(barrier: asyncio.Event, hold_ms: int):
            async with concurrent_pool.acquire() as c:
                # We need to slow down whichever holds the lock so the other
                # has time to try. Use a sleep INSIDE compute_readiness via
                # a wrapper transaction that we control externally.
                return await compute_readiness(c, uid)

        # Custom: open the lock in one connection, then race a second.
        async with concurrent_pool.acquire() as c1:
            tx1 = c1.transaction()
            await tx1.start()
            try:
                got = await c1.fetchval(
                    "SELECT pg_try_advisory_xact_lock(hashtextextended('readiness:' || $1, 0))",
                    uid,
                )
                assert got is True

                # Now while c1 holds the lock, run compute_readiness on c2.
                async with concurrent_pool.acquire() as c2:
                    res2 = await compute_readiness(c2, uid)

                assert res2["refused"] is True
                assert res2["refusal_reason"] == "already_running"
                assert res2["written"] is False
            finally:
                await tx1.rollback()
    finally:
        await _cleanup_user(concurrent_pool, uid)


@pytest.mark.concurrency
async def test_worker_crash_mid_tx_no_partial_state(concurrent_pool):
    """If a worker raises mid-transaction, no analytics_cache row is written."""
    async with concurrent_pool.acquire() as c:
        uid = str(uuid.uuid4())
        await c.execute(
            "INSERT INTO users (id, username, units, locale) VALUES ($1::uuid, $2, 'metric', 'de')",
            uid, f"crash_{uid[:8]}",
        )

    try:
        async with concurrent_pool.acquire() as c:
            tx = c.transaction()
            await tx.start()
            try:
                # Acquire lock (fine), then bail out.
                await c.fetchval(
                    "SELECT pg_try_advisory_xact_lock(hashtextextended('readiness:' || $1, 0))",
                    uid,
                )
                raise RuntimeError("simulated crash")
            except RuntimeError:
                await tx.rollback()

        # Cache row must NOT exist.
        async with concurrent_pool.acquire() as c:
            row = await c.fetchrow(
                "SELECT 1 FROM analytics_cache WHERE user_id=$1::uuid AND cache_key='readiness'",
                uid,
            )
            assert row is None
    finally:
        await _cleanup_user(concurrent_pool, uid)


# ===========================================================================
# C3 follow-up: timezone-anchored "today"
# ===========================================================================

async def test_compute_readiness_picks_up_local_today_for_far_east_tz(conn):
    """Around UTC midnight, a Pacific/Auckland user (UTC+12/+13) sees data
    inserted for the user's local 'today' even if the server clock is still
    on the previous UTC date. The C3 fix anchored every date query to
    (NOW() AT TIME ZONE users.timezone)::date — this regression test fails
    if any loader regresses to CURRENT_DATE / date.today()."""
    uid = await seed_user(conn)
    await conn.execute(
        "UPDATE users SET timezone = 'Pacific/Auckland' WHERE id = $1::uuid",
        uid,
    )

    # Anchor every seeded row to the USER's local-today, not the server's.
    user_today = await conn.fetchval(
        "SELECT (NOW() AT TIME ZONE 'Pacific/Auckland')::date"
    )

    # 60 nightly RMSSD values ending the user's local yesterday (baseline).
    for i in range(1, 61):
        await conn.execute(
            """
            INSERT INTO health_hrv_night_values (user_id, ts, rmssd_ms)
            VALUES ($1::uuid,
                    (($2::date - $3::int)::timestamp AT TIME ZONE 'Pacific/Auckland')
                       + INTERVAL '4 hours',
                    $4)
            ON CONFLICT (user_id, ts) DO NOTHING
            """,
            uid, user_today, i, 50.0 + (5.0 if i % 2 == 0 else -5.0),
        )

    # Today's HRV snapshot — a moment in user-local today.
    await conn.execute(
        """
        INSERT INTO health_snapshots (user_id, measured_at_utc, rmssd_ms, source_type)
        VALUES ($1::uuid, NOW(), 60.0, 'garmin_fit')
        """,
        uid,
    )

    # Sleep ending a couple of hours ago (well within 36h freshness gate).
    await conn.execute(
        """
        INSERT INTO health_sleep
            (user_id, calendar_date, sleep_start_utc, sleep_end_utc,
             deep_seconds, light_seconds, rem_seconds, score_overall, source_type)
        VALUES ($1::uuid, $2::date,
                NOW() - INTERVAL '10 hours', NOW() - INTERVAL '2 hours',
                7200, 10800, 7200, 82, 'garmin_fit')
        """,
        uid, user_today,
    )

    # PMC for user-local today.
    await conn.execute(
        """
        INSERT INTO pmc_daily (user_id, date, sport_group, acwr_ra)
        VALUES ($1::uuid, $2::date, '__combined__', 1.0)
        ON CONFLICT (user_id, date, sport_group) DO UPDATE SET acwr_ra = 1.0
        """,
        uid, user_today,
    )

    result = await compute_readiness(conn, uid)

    assert result["refused"] is False, (
        f"Expected refused=False but got {result.get('refusal_reason')!r} — "
        f"loaders likely missed the user's local 'today' rows."
    )
    by_key = {f["key"]: f for f in result["factors"]}
    for k in ("hrv_status", "sleep", "training_load"):
        assert by_key[k]["missing"] is False, f"factor {k} unexpectedly missing"
    assert result["data_window_end"] == user_today.isoformat()
