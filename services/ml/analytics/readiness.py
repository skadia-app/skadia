"""Readiness Score Worker — composite + factor loaders + cache write.

Spec: docs/internal/specs/2026-05-01-readiness-design.md (§5–§6, §10.4).
Plan: docs/internal/2026-05-01-readiness-plan.md (Phase 3).

The worker computes a daily readiness score (0–100) from up to three
critical factors (HRV-Status, Sleep, Training-Load) plus optional bonus
factors (DFA-alpha1, Sample Entropy, RHR-Changepoint). Manufacturer-
provided readiness scores (`health_readiness_scores`) act as a parallel
input path — user preference (`user_readiness_preferences`) decides which
side wins when both are available.

Concurrency model:
- `pg_try_advisory_xact_lock(hashtextextended('readiness:'||user_id, 0))`
  inside the same transaction that performs the cache UPSERT — prevents
  k8s-style write races where two pods compute concurrently and the older
  result clobbers the newer (LWW).
- Pref-Race detection: read `user_readiness_preferences.updated_at` at
  start AND end. Mismatch → refuse, ack, re-publish the invalidate so a
  fresh worker re-runs against the new pref.
"""
from __future__ import annotations

import json
import logging
from dataclasses import dataclass, replace
from datetime import date, datetime, timezone
from typing import Any

from .changepoints import detect_changepoints

logger = logging.getLogger(__name__)

# --- Constants ---------------------------------------------------------------

CACHE_TTL_HOURS = 6
WEIGHT_SLEEP, WEIGHT_HRV, WEIGHT_LOAD = 0.40, 0.30, 0.30
HRV_TODAY_MAX_AGE_H = 48
SLEEP_MAX_AGE_H = 36
DFA_BONUS_HIGH, DFA_BONUS_LOW = +3, -3   # >0.75 / <0.5
SAMPEN_BONUS = -2
RHR_CHANGEPOINT_PENALTY = -5
STRESS_BONUS = -3

CRITICAL_FACTORS = ("hrv_status", "sleep", "training_load")
MFR_FACTOR_COL = {
    "hrv_status":    "hrv_factor_pct",
    "sleep":         "sleep_history_factor_pct",
    "training_load": "acwr_pct",
}

# Sleep minimum recorded duration (deep+light+rem ≥ 4h)
SLEEP_MIN_RECORDED_SECONDS = 4 * 3600

# Baseline / history window thresholds for confidence
HRV_BASELINE_MIN_DAYS = 60
ACWR_HISTORY_MIN_DAYS = 28


# --- Data class --------------------------------------------------------------

@dataclass(frozen=True)
class FactorInput:
    key: str
    value: float | None              # 0..100 sub-score, None if missing
    raw_value: float | None = None   # Original measurement (for UI detail)
    raw_unit: str | None = None
    source: str | None = None        # 'skadia', 'garmin', 'manual_override', ...
    stale_hours: float | None = None
    missing: bool = False
    available_sources: tuple[str, ...] = ()

    def _replace(self, **kw):  # mimic NamedTuple._replace
        return replace(self, **kw)


# --- Loaders -----------------------------------------------------------------

async def _load_hrv_status(conn, user_id: str) -> FactorInput:
    """Today's RMSSD, normalized against 60-day baseline (Z-score → 0..100)."""
    # Today's value: prefer health_snapshots.rmssd_ms (max measured_at_utc::date = today),
    # fallback health_hrv_night_values.
    today_row = await conn.fetchrow(
        """
        SELECT rmssd_ms,
               EXTRACT(EPOCH FROM (NOW() - measured_at_utc)) / 3600.0 AS age_h
          FROM health_snapshots
         WHERE user_id = $1::uuid
           AND rmssd_ms IS NOT NULL
           AND measured_at_utc::date = CURRENT_DATE
         ORDER BY measured_at_utc DESC
         LIMIT 1
        """,
        user_id,
    )
    if today_row is None:
        today_row = await conn.fetchrow(
            """
            SELECT rmssd_ms,
                   EXTRACT(EPOCH FROM (NOW() - ts)) / 3600.0 AS age_h
              FROM health_hrv_night_values
             WHERE user_id = $1::uuid
               AND ts::date = CURRENT_DATE
             ORDER BY ts DESC
             LIMIT 1
            """,
            user_id,
        )

    if today_row is None or today_row["age_h"] is None or float(today_row["age_h"]) > HRV_TODAY_MAX_AGE_H:
        return FactorInput(key="hrv_status", value=None, missing=True, source="skadia")

    rmssd_today = float(today_row["rmssd_ms"])
    stale_h = float(today_row["age_h"])

    # 60-day baseline from health_hrv_night_values (n>=7, AVG/STDDEV)
    baseline = await conn.fetchrow(
        """
        SELECT AVG(rmssd_ms) AS mean,
               STDDEV_SAMP(rmssd_ms) AS sd,
               COUNT(*) AS n
          FROM health_hrv_night_values
         WHERE user_id = $1::uuid
           AND ts >= (CURRENT_DATE - INTERVAL '60 days')
           AND ts::date < CURRENT_DATE
        """,
        user_id,
    )
    if baseline is None or baseline["n"] is None or int(baseline["n"]) < 7 \
            or baseline["mean"] is None or baseline["sd"] is None or float(baseline["sd"]) <= 0:
        return FactorInput(key="hrv_status", value=None, missing=True, source="skadia",
                           raw_value=rmssd_today, raw_unit="ms", stale_hours=stale_h)

    z = (rmssd_today - float(baseline["mean"])) / float(baseline["sd"])
    z = max(-2.0, min(2.0, z))
    # Map [-2, +2] → [0, 100]
    sub_score = (z + 2.0) / 4.0 * 100.0
    return FactorInput(
        key="hrv_status",
        value=float(sub_score),
        raw_value=rmssd_today,
        raw_unit="ms",
        source="skadia",
        stale_hours=stale_h,
        missing=False,
    )


async def _load_sleep_factor(conn, user_id: str) -> FactorInput:
    """Most-recent health_sleep with score_overall NOT NULL, ≤36h, ≥4h recorded."""
    row = await conn.fetchrow(
        """
        SELECT calendar_date,
               score_overall,
               COALESCE(deep_seconds, 0)
                 + COALESCE(light_seconds, 0)
                 + COALESCE(rem_seconds, 0) AS recorded_s,
               sleep_end_utc,
               EXTRACT(EPOCH FROM (NOW() - sleep_end_utc)) / 3600.0 AS age_h
          FROM health_sleep
         WHERE user_id = $1::uuid
           AND score_overall IS NOT NULL
           AND calendar_date <= CURRENT_DATE
         ORDER BY calendar_date DESC, sleep_end_utc DESC
         LIMIT 1
        """,
        user_id,
    )
    if row is None:
        return FactorInput(key="sleep", value=None, missing=True, source="skadia")

    recorded_s = int(row["recorded_s"] or 0)
    age_h = float(row["age_h"]) if row["age_h"] is not None else None

    if recorded_s < SLEEP_MIN_RECORDED_SECONDS:
        return FactorInput(key="sleep", value=None, missing=True, source="skadia",
                           stale_hours=age_h)
    if age_h is None or age_h > SLEEP_MAX_AGE_H:
        return FactorInput(key="sleep", value=None, missing=True, source="skadia",
                           stale_hours=age_h)

    score = float(row["score_overall"])
    return FactorInput(
        key="sleep",
        value=max(0.0, min(100.0, score)),
        raw_value=score,
        raw_unit="score",
        source="skadia",
        stale_hours=age_h,
        missing=False,
    )


async def _load_training_load_factor(conn, user_id: str) -> FactorInput:
    """pmc_daily.acwr_ra for today, sport_group='__combined__'."""
    row = await conn.fetchrow(
        """
        SELECT acwr_ra
          FROM pmc_daily
         WHERE user_id = $1::uuid
           AND date = CURRENT_DATE
           AND sport_group = '__combined__'
        """,
        user_id,
    )
    if row is None or row["acwr_ra"] is None:
        return FactorInput(key="training_load", value=None, missing=True, source="skadia")
    acwr = float(row["acwr_ra"])
    if acwr == 0.0:
        # PMC convention: acwr_ra=0 → insufficient training-day history
        return FactorInput(key="training_load", value=None, missing=True, source="skadia",
                           raw_value=acwr, raw_unit="ratio")

    # Mapping: 0.8–1.3 → 100; <0.8 → linear 60→100 (at 0.0..0.8); >1.3 → linear 100→0 (1.3..2.0)
    if 0.8 <= acwr <= 1.3:
        sub = 100.0
    elif acwr < 0.8:
        # 0.0 → 60, 0.8 → 100
        sub = 60.0 + (acwr / 0.8) * 40.0
    else:
        # 1.3 → 100, 2.0 → 0
        if acwr >= 2.0:
            sub = 0.0
        else:
            sub = 100.0 - ((acwr - 1.3) / 0.7) * 100.0

    sub = max(0.0, min(100.0, sub))
    return FactorInput(
        key="training_load",
        value=float(sub),
        raw_value=acwr,
        raw_unit="ratio",
        source="skadia",
        missing=False,
    )


async def _load_manufacturer_factors(conn, user_id: str) -> dict | None:
    """health_readiness_scores for today, score IS NOT NULL."""
    row = await conn.fetchrow(
        """
        SELECT score, level,
               hrv_factor_pct, sleep_history_factor_pct,
               stress_history_factor_pct, recovery_time_factor_pct,
               acwr_pct, source_type
          FROM health_readiness_scores
         WHERE user_id = $1::uuid
           AND calendar_date = CURRENT_DATE
           AND score IS NOT NULL
        """,
        user_id,
    )
    if row is None:
        return None
    return {
        "score": int(row["score"]) if row["score"] is not None else None,
        "level": row["level"],
        "hrv_factor_pct": float(row["hrv_factor_pct"]) if row["hrv_factor_pct"] is not None else None,
        "sleep_history_factor_pct": float(row["sleep_history_factor_pct"]) if row["sleep_history_factor_pct"] is not None else None,
        "stress_history_factor_pct": float(row["stress_history_factor_pct"]) if row["stress_history_factor_pct"] is not None else None,
        "recovery_time_factor_pct": float(row["recovery_time_factor_pct"]) if row["recovery_time_factor_pct"] is not None else None,
        "acwr_pct": float(row["acwr_pct"]) if row["acwr_pct"] is not None else None,
        "source_type": row["source_type"] or "garmin",
    }


async def _load_dfa_alpha1(conn, user_id: str) -> float | None:
    row = await conn.fetchrow(
        """
        SELECT value_num
          FROM user_daily_features
         WHERE user_id = $1::uuid
           AND feature_key = 'hrv.HRV_DFA_alpha1'
           AND date = CURRENT_DATE
           AND value_num IS NOT NULL
         ORDER BY computed_at DESC NULLS LAST
         LIMIT 1
        """,
        user_id,
    )
    return float(row["value_num"]) if row else None


async def _load_sample_entropy(conn, user_id: str) -> tuple[float | None, float | None]:
    """Returns (today_value, 30d_median)."""
    today = await conn.fetchrow(
        """
        SELECT value_num
          FROM user_daily_features
         WHERE user_id = $1::uuid
           AND feature_key = 'hrv.HRV_SampEn'
           AND date = CURRENT_DATE
           AND value_num IS NOT NULL
         ORDER BY computed_at DESC NULLS LAST
         LIMIT 1
        """,
        user_id,
    )
    today_v = float(today["value_num"]) if today else None

    median_row = await conn.fetchrow(
        """
        SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY value_num) AS med
          FROM user_daily_features
         WHERE user_id = $1::uuid
           AND feature_key = 'hrv.HRV_SampEn'
           AND date >= (CURRENT_DATE - INTERVAL '30 days')
           AND date < CURRENT_DATE
           AND value_num IS NOT NULL
        """,
        user_id,
    )
    med = float(median_row["med"]) if median_row and median_row["med"] is not None else None
    return today_v, med


async def _load_rhr_changepoint_signal(conn, user_id: str) -> str | None:
    """Read analytics_cache cache_key='changepoints.resting_hr'.

    Auto-trigger: cache missing or older than 24h → run detect_changepoints
    inside the same transaction (caller's tx) and re-read.
    """
    row = await conn.fetchrow(
        """
        SELECT data, computed_at,
               EXTRACT(EPOCH FROM (NOW() - computed_at)) / 3600.0 AS age_h
          FROM analytics_cache
         WHERE user_id = $1::uuid
           AND cache_key = 'changepoints.resting_hr'
        """,
        user_id,
    )

    needs_recompute = row is None or row["computed_at"] is None or \
        (row["age_h"] is not None and float(row["age_h"]) > 24.0)

    if needs_recompute:
        try:
            cps = await detect_changepoints(conn, user_id, "resting_hr")
            await conn.execute(
                """
                INSERT INTO analytics_cache (user_id, cache_key, data, computed_at)
                VALUES ($1::uuid, 'changepoints.resting_hr', $2::jsonb, NOW())
                ON CONFLICT (user_id, cache_key) DO UPDATE
                SET data = EXCLUDED.data, computed_at = EXCLUDED.computed_at
                """,
                user_id, json.dumps(cps),
            )
            row = await conn.fetchrow(
                """
                SELECT data, computed_at
                  FROM analytics_cache
                 WHERE user_id = $1::uuid
                   AND cache_key = 'changepoints.resting_hr'
                """,
                user_id,
            )
        except Exception as exc:  # pragma: no cover — defensive
            logger.warning("rhr changepoint auto-trigger failed for %s: %s", user_id, exc)
            return None

    if row is None or row["data"] is None:
        return None
    data = row["data"]
    if isinstance(data, str):
        try:
            data = json.loads(data)
        except json.JSONDecodeError:
            return None
    if not isinstance(data, list) or not data:
        return None

    # Newest changepoint date
    latest_iso = None
    for cp in data:
        if not isinstance(cp, dict):
            continue
        d = cp.get("date") or cp.get("changepoint_date")
        if d and (latest_iso is None or d > latest_iso):
            latest_iso = d
    if latest_iso is None:
        return None
    try:
        latest = date.fromisoformat(latest_iso)
    except ValueError:
        return None
    if (date.today() - latest).days < 7:
        return "rhr_changepoint_recent"
    return None


# --- Refusal -----------------------------------------------------------------

def _check_refusal(eigen: dict[str, FactorInput], mfr: dict | None) -> tuple[bool, str | None]:
    def has_via_any(key: str) -> bool:
        f = eigen.get(key)
        if f is not None and not f.missing and f.value is not None:
            return True
        if mfr is None:
            return False
        return mfr.get(MFR_FACTOR_COL[key]) is not None

    n = sum(1 for k in CRITICAL_FACTORS if has_via_any(k))
    has_path_a = n >= 2
    has_path_b = mfr is not None and mfr.get("score") is not None
    if has_path_a or has_path_b:
        return False, None
    if all(eigen[k].missing for k in CRITICAL_FACTORS) and mfr is None:
        return True, "no_data"
    return True, "insufficient_factors"


# --- Composite ---------------------------------------------------------------

def _score_to_level(score: int) -> str:
    if score < 20:
        return "critical"
    if score < 40:
        return "low"
    if score < 60:
        return "moderate"
    if score < 80:
        return "good"
    return "optimal"


def _compute_composite(factors: dict[str, FactorInput]) -> tuple[int, dict, str]:
    weights = {"hrv_status": WEIGHT_HRV, "sleep": WEIGHT_SLEEP, "training_load": WEIGHT_LOAD}
    weighted_sum = 0.0
    weight_sum = 0.0
    used: dict[str, float] = {}
    for k, w in weights.items():
        f = factors.get(k)
        if f is None or f.missing or f.value is None:
            continue
        weighted_sum += w * float(f.value)
        weight_sum += w
        used[k] = w
    assert weight_sum >= 0.60, "_check_refusal sollte das verhindern"
    score = max(0, min(100, round(weighted_sum / weight_sum)))
    return score, used, _score_to_level(score)


# --- Bonus modifiers ---------------------------------------------------------

def _apply_bonus_modifiers(
    *,
    composite_score: int,
    dfa_alpha1: float | None,
    sampen: float | None,
    sampen_median: float | None,
    rhr_changepoint: str | None,
    stress_unusually_high: bool = False,
    path_kind: str,
    manufacturer_in_critical: bool,
) -> tuple[int, str | None]:
    score, sig = composite_score, None
    if path_kind == "A":
        if dfa_alpha1 is not None:
            if dfa_alpha1 > 0.75:
                score += DFA_BONUS_HIGH
            elif dfa_alpha1 < 0.5:
                score += DFA_BONUS_LOW
        if sampen is not None and sampen_median is not None and sampen < sampen_median:
            score += SAMPEN_BONUS
        if stress_unusually_high and not manufacturer_in_critical:
            score += STRESS_BONUS
    if rhr_changepoint == "rhr_changepoint_recent":
        score += RHR_CHANGEPOINT_PENALTY
        sig = "rhr_changepoint_recent"
    return max(0, min(100, score)), sig


# --- Mix ---------------------------------------------------------------------

def _mix_sources(
    eigen: dict[str, FactorInput],
    mfr: dict | None,
    pref: str | None,
) -> dict[str, FactorInput]:
    mfr_map = MFR_FACTOR_COL
    src = (mfr.get("source_type") if mfr else None) or "garmin"
    out: dict[str, FactorInput] = dict(eigen)

    for key, col in mfr_map.items():
        eigen_avail = (key in eigen and not eigen[key].missing and eigen[key].value is not None)
        mfr_avail = (mfr is not None) and (mfr.get(col) is not None)

        avail: list[str] = []
        if eigen_avail:
            avail.append("skadia")
        if mfr_avail:
            avail.append(src)

        if not eigen_avail and not mfr_avail:
            continue

        # Picking rule (matches spec §6.3):
        # - mfr_only            → use mfr
        # - eigen_only          → use eigen
        # - both, pref=manufacturer → mfr
        # - both, pref=skadia   → eigen
        # - both, pref=None      → mfr (manufacturer fresh today wins per spec)
        use_mfr = mfr_avail and (
            not eigen_avail
            or pref == "manufacturer"
            or pref is None
        )
        if use_mfr:
            v = float(mfr[col])
            out[key] = FactorInput(
                key=key,
                value=v,
                raw_value=v,
                source=src,
                available_sources=tuple(avail),
                raw_unit=None,
                stale_hours=None,
                missing=False,
            )
        elif eigen_avail:
            out[key] = eigen[key]._replace(available_sources=tuple(avail))

    return out


def _mfr_to_factors(mfr: dict) -> list[dict]:
    """Path-B factor envelope: surface manufacturer sub-factors directly."""
    src = mfr.get("source_type") or "garmin"
    factors: list[dict] = []
    weights = {
        "hrv_status": ("hrv_factor_pct", WEIGHT_HRV),
        "sleep": ("sleep_history_factor_pct", WEIGHT_SLEEP),
        "training_load": ("acwr_pct", WEIGHT_LOAD),
    }
    for key, (col, w) in weights.items():
        v = mfr.get(col)
        factors.append({
            "key": key,
            "value": float(v) if v is not None else None,
            "raw_value": float(v) if v is not None else None,
            "raw_unit": "pct",
            "weight": w,
            "source": src,
            "stale_hours": None,
            "missing": v is None,
        })
    return factors


# --- Confidence + recommendation --------------------------------------------

async def _derive_confidence(
    conn,
    user_id: str,
    *,
    factors: dict[str, FactorInput],
    path_kind: str,
) -> str:
    if path_kind == "B":
        return "medium"

    available = [
        k for k in CRITICAL_FACTORS
        if k in factors and not factors[k].missing and factors[k].value is not None
    ]
    n = len(available)
    if n == 0:
        return "low"

    # Stale check: any factor >24h fresh-but-below-48h hint → 'low'
    any_24_48h_stale = any(
        factors[k].stale_hours is not None and factors[k].stale_hours > 24.0
        for k in available
        if factors[k].stale_hours is not None
    )

    # History thresholds (only relevant for `high`)
    if n == 3 and not any_24_48h_stale:
        baseline_ok = await _hrv_baseline_days_geq(conn, user_id, HRV_BASELINE_MIN_DAYS)
        acwr_ok = await _acwr_history_days_geq(conn, user_id, ACWR_HISTORY_MIN_DAYS)
        if baseline_ok and acwr_ok:
            return "high"
        return "medium"

    if n >= 2:
        return "low" if any_24_48h_stale else "medium"
    return "low"


async def _hrv_baseline_days_geq(conn, user_id: str, days: int) -> bool:
    row = await conn.fetchrow(
        """
        SELECT COUNT(DISTINCT ts::date) AS n
          FROM health_hrv_night_values
         WHERE user_id = $1::uuid
           AND ts >= (CURRENT_DATE - ($2::int || ' days')::interval)
        """,
        user_id, days,
    )
    return row is not None and row["n"] is not None and int(row["n"]) >= days


async def _acwr_history_days_geq(conn, user_id: str, days: int) -> bool:
    row = await conn.fetchrow(
        """
        SELECT COUNT(*) AS n
          FROM pmc_daily
         WHERE user_id = $1::uuid
           AND sport_group = '__combined__'
           AND date >= (CURRENT_DATE - ($2::int || ' days')::interval)
           AND acwr_ra IS NOT NULL AND acwr_ra <> 0
        """,
        user_id, days,
    )
    return row is not None and row["n"] is not None and int(row["n"]) >= days


def _derive_recommendation_code(level: str, confidence: str, signal_modifier: str | None) -> str:
    return f"{level}_{confidence}_{signal_modifier or 'normal'}"


# --- Refused-envelope helper -------------------------------------------------

def _refused_envelope(
    *,
    refusal_reason: str,
    data_window_end: str,
    preferences_version: str | None,
    factors_eigen: dict[str, FactorInput],
    mfr: dict | None,
) -> dict:
    """Schema-stable refused envelope (Plan Phase 5)."""
    factor_list: list[dict] = []
    for k in CRITICAL_FACTORS:
        f = factors_eigen.get(k)
        factor_list.append({
            "key": k,
            "value": None if (f is None or f.missing) else f.value,
            "raw_value": None if f is None else f.raw_value,
            "raw_unit": None if f is None else f.raw_unit,
            "weight": None,
            "source": None if f is None else f.source,
            "stale_hours": None if f is None else f.stale_hours,
            "missing": True if f is None else f.missing,
        })

    return {
        "score": None,
        "level": None,
        "computed_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "data_window_end": data_window_end,
        "preferences_version": preferences_version,
        "confidence": None,
        "refused": True,
        "refusal_reason": refusal_reason,
        "factors": factor_list,
        "missing_factors": [
            {"key": k, "reason": "no_data", "last_seen_at": None}
            for k in CRITICAL_FACTORS
            if factors_eigen.get(k) is None or factors_eigen[k].missing
        ],
        "recommendation_code": None,
        "signal_modifier": None,
    }


# --- Top-level worker --------------------------------------------------------

async def _read_pref(conn, user_id: str) -> tuple[str | None, str | None]:
    """Returns (preferred_source, updated_at_iso). (None, None) if no row."""
    row = await conn.fetchrow(
        """
        SELECT preferred_source, updated_at
          FROM user_readiness_preferences
         WHERE user_id = $1::uuid
        """,
        user_id,
    )
    if row is None:
        return None, None
    pref = row["preferred_source"]
    upd = row["updated_at"]
    if isinstance(upd, datetime):
        if upd.tzinfo is None:
            upd = upd.replace(tzinfo=timezone.utc)
        upd_iso = upd.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")
    else:
        upd_iso = str(upd) if upd is not None else None
    return pref, upd_iso


async def _compute_readiness_locked(conn, user_id: str) -> dict:
    """Inside-tx computation. Lock has already been acquired by caller."""
    today_iso = date.today().isoformat()

    # 1. Pref + version snapshot
    pref, pref_version_at_start = await _read_pref(conn, user_id)

    # 2. Own factors
    eigen = {
        "hrv_status":    await _load_hrv_status(conn, user_id),
        "sleep":         await _load_sleep_factor(conn, user_id),
        "training_load": await _load_training_load_factor(conn, user_id),
    }

    # 3. Manufacturer factors
    mfr = await _load_manufacturer_factors(conn, user_id)

    # 4. Refusal check (early-exit)
    refused, reason = _check_refusal(eigen, mfr)
    if refused:
        return _refused_envelope(
            refusal_reason=reason or "insufficient_factors",
            data_window_end=today_iso,
            preferences_version=pref_version_at_start,
            factors_eigen=eigen,
            mfr=mfr,
        )

    # 5. Path determination
    n_eigen = sum(1 for f in eigen.values() if not f.missing and f.value is not None)
    path_a = n_eigen >= 2
    path_b = mfr is not None and mfr.get("score") is not None
    if path_a and path_b:
        path_kind = "B" if pref == "manufacturer" else "A"
    elif path_a:
        path_kind = "A"
    else:
        path_kind = "B"

    # Bonus inputs (only loaded once)
    rhr_signal = await _load_rhr_changepoint_signal(conn, user_id)

    if path_kind == "B":
        # 6. Path B: use manufacturer score directly
        score = int(mfr["score"])
        # Only RHR-changepoint bonus applies on Path B
        score, sig = _apply_bonus_modifiers(
            composite_score=score,
            dfa_alpha1=None,
            sampen=None,
            sampen_median=None,
            rhr_changepoint=rhr_signal,
            stress_unusually_high=False,
            path_kind="B",
            manufacturer_in_critical=True,
        )
        level = _score_to_level(score)
        factors_out = _mfr_to_factors(mfr)
        used_mix: dict[str, FactorInput] = {}  # for confidence path
    else:
        # 7. Path A
        mixed = _mix_sources(eigen, mfr, pref)
        composite, used_weights, level = _compute_composite(mixed)

        manufacturer_in_critical = any(
            mixed[k].source != "skadia"
            for k in CRITICAL_FACTORS
            if k in mixed and mixed[k] is not None and not mixed[k].missing
        )

        dfa = await _load_dfa_alpha1(conn, user_id)
        sampen, sampen_med = await _load_sample_entropy(conn, user_id)

        stress_unusually_high = bool(
            mfr is not None
            and mfr.get("stress_history_factor_pct") is not None
            and float(mfr["stress_history_factor_pct"]) < 30.0
        )

        score, sig = _apply_bonus_modifiers(
            composite_score=composite,
            dfa_alpha1=dfa,
            sampen=sampen,
            sampen_median=sampen_med,
            rhr_changepoint=rhr_signal,
            stress_unusually_high=stress_unusually_high,
            path_kind="A",
            manufacturer_in_critical=manufacturer_in_critical,
        )
        level = _score_to_level(score)
        used_mix = mixed

        factors_out = []
        for k in CRITICAL_FACTORS:
            f = mixed.get(k)
            if f is None or f.missing or f.value is None:
                factors_out.append({
                    "key": k, "value": None, "missing": True,
                    "weight": None, "source": None, "raw_value": None,
                    "raw_unit": None, "stale_hours": None,
                })
            else:
                factors_out.append({
                    "key": k,
                    "value": float(f.value),
                    "raw_value": f.raw_value,
                    "raw_unit": f.raw_unit,
                    "weight": used_weights.get(k),
                    "source": f.source,
                    "stale_hours": f.stale_hours,
                    "missing": False,
                    "available_sources": list(f.available_sources),
                })

    # 8. Confidence + recommendation
    confidence = await _derive_confidence(
        conn, user_id,
        factors=used_mix if path_kind == "A" else {},
        path_kind=path_kind,
    )
    recommendation_code = _derive_recommendation_code(level, confidence, sig)

    # 9. Pref-Race re-check
    _, pref_version_at_end = await _read_pref(conn, user_id)
    if pref_version_at_end != pref_version_at_start:
        return {
            "refused": True,
            "refusal_reason": "stale_compute_pref_changed",
        }

    # 10. Output
    missing_factors = [
        {"key": k, "reason": "no_data_today", "last_seen_at": None}
        for k in CRITICAL_FACTORS
        if (used_mix.get(k) is None or used_mix[k].missing) and path_kind == "A"
    ]

    return {
        "score": int(score),
        "level": level,
        "computed_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "data_window_end": today_iso,
        "preferences_version": pref_version_at_start,
        "confidence": confidence,
        "refused": False,
        "refusal_reason": None,
        "factors": factors_out,
        "missing_factors": missing_factors,
        "recommendation_code": recommendation_code,
        "signal_modifier": sig,
    }


async def compute_readiness(conn, user_id: str, publisher=None) -> dict:
    """Race-Verhalten:
    - already_running:        ack ohne Write. Kein Re-Trigger nötig.
    - stale_compute_pref_changed: ack ohne Write **und** anschließend einen
                              neuen Invalidate publishen (handled by
                              handle_readiness_message).
    - no_data / insufficient_factors: schreiben (Refused-Envelope).
    """
    async with conn.transaction():
        got_lock = await conn.fetchval(
            "SELECT pg_try_advisory_xact_lock(hashtextextended('readiness:' || $1, 0))",
            user_id,
        )
        if not got_lock:
            return {"refused": True, "refusal_reason": "already_running", "written": False}

        result = await _compute_readiness_locked(conn, user_id)

        if result.get("refusal_reason") == "stale_compute_pref_changed":
            return {**result, "written": False, "republish": True}

        await conn.execute(
            """
            INSERT INTO analytics_cache (user_id, cache_key, data, computed_at, expires_at)
            VALUES ($1::uuid, 'readiness', $2::jsonb, NOW(), NOW() + make_interval(secs => $3))
            ON CONFLICT (user_id, cache_key) DO UPDATE
            SET data        = EXCLUDED.data,
                computed_at = EXCLUDED.computed_at,
                expires_at  = EXCLUDED.expires_at
            """,
            user_id, json.dumps(result), int(CACHE_TTL_HOURS * 3600),
        )
        return {**result, "written": True}


async def handle_readiness_message(conn, user_id: str, publisher=None) -> dict:
    """NATS handler wrapper — pref-race triggers re-publish."""
    result = await compute_readiness(conn, user_id, publisher)
    if result.get("republish") and publisher is not None:
        await publisher.publish_analytics_invalidate(user_id)
    return result
