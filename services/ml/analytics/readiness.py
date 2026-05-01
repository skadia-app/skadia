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
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

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

# Canonical set of reason strings used in missing_factors entries.
# "no_data"      — factor was never available (no historical data at all)
# "stale"        — factor data exists but is older than the allowed max age
# "no_baseline"  — raw measurement exists but baseline (60d HRV / 28d ACWR) is insufficient
# "no_data_today"— factor data from prior days exists, but nothing recorded today
MISSING_REASONS = ("no_data", "stale", "no_baseline", "no_data_today")

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
    missing_reason: str | None = None  # one of MISSING_REASONS when missing=True
    available_sources: tuple[str, ...] = ()

    def _replace(self, **kw):  # mimic NamedTuple._replace
        return replace(self, **kw)


# --- Loaders -----------------------------------------------------------------

async def _load_user_timezone(conn, user_id: str) -> str:
    """Return the user's IANA timezone name (e.g. 'Europe/Berlin').

    Falls back to 'UTC' if the row is missing, the timezone column is NULL,
    or the stored value is not a valid IANA zone — preventing a single bad
    row from breaking the entire readiness computation.
    """
    row = await conn.fetchrow(
        "SELECT timezone FROM users WHERE id = $1::uuid",
        user_id,
    )
    tz_name = (row["timezone"] if row and row["timezone"] else None) or "UTC"
    try:
        ZoneInfo(tz_name)  # validate — raises ZoneInfoNotFoundError if bogus
    except (ZoneInfoNotFoundError, KeyError):
        logger.warning(
            "readiness: invalid timezone %r for user %s — falling back to UTC",
            tz_name, user_id,
        )
        tz_name = "UTC"
    return tz_name


async def _load_hrv_status(conn, user_id: str, user_tz: str) -> FactorInput:
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
           AND measured_at_utc::date = (NOW() AT TIME ZONE $2)::date
         ORDER BY measured_at_utc DESC
         LIMIT 1
        """,
        user_id, user_tz,
    )
    if today_row is None:
        today_row = await conn.fetchrow(
            """
            SELECT rmssd_ms,
                   EXTRACT(EPOCH FROM (NOW() - ts)) / 3600.0 AS age_h
              FROM health_hrv_night_values
             WHERE user_id = $1::uuid
               AND ts::date = (NOW() AT TIME ZONE $2)::date
             ORDER BY ts DESC
             LIMIT 1
            """,
            user_id, user_tz,
        )

    if today_row is None or today_row["age_h"] is None:
        return FactorInput(
            key="hrv_status", value=None, missing=True, missing_reason="no_data_today",
            source="skadia",
        )
    if float(today_row["age_h"]) > HRV_TODAY_MAX_AGE_H:
        return FactorInput(
            key="hrv_status", value=None, missing=True, missing_reason="stale",
            source="skadia", stale_hours=float(today_row["age_h"]),
        )

    rmssd_today = float(today_row["rmssd_ms"])
    stale_h = float(today_row["age_h"])

    # 60-day baseline — union health_hrv_night_values + health_snapshots so that
    # mobile-import users (snapshots-only) get a baseline too. Day+source dedup:
    # one row per (local-date, source_type) — prevents source-double-counting
    # when both tables ingest the same night via different importers.
    baseline = await conn.fetchrow(
        """
        WITH src AS (
            SELECT (ts AT TIME ZONE $2)::date AS day,
                   source_type,
                   AVG(rmssd_ms) AS rmssd
              FROM health_hrv_night_values
             WHERE user_id = $1::uuid
               AND ts >= ((NOW() AT TIME ZONE $2)::date - INTERVAL '60 days')
               AND ts::date < (NOW() AT TIME ZONE $2)::date
             GROUP BY day, source_type
            UNION ALL
            SELECT (measured_at_utc AT TIME ZONE $2)::date AS day,
                   source_type,
                   AVG(rmssd_ms) AS rmssd
              FROM health_snapshots
             WHERE user_id = $1::uuid
               AND rmssd_ms IS NOT NULL
               AND measured_at_utc >= ((NOW() AT TIME ZONE $2)::date - INTERVAL '60 days')
               AND (measured_at_utc AT TIME ZONE $2)::date < (NOW() AT TIME ZONE $2)::date
             GROUP BY day, source_type
        )
        SELECT AVG(rmssd) AS mean,
               STDDEV_SAMP(rmssd) AS sd,
               COUNT(*) AS n
          FROM src
        """,
        user_id, user_tz,
    )
    if baseline is None or baseline["n"] is None or int(baseline["n"]) < 7 \
            or baseline["mean"] is None or baseline["sd"] is None or float(baseline["sd"]) <= 0:
        return FactorInput(
            key="hrv_status", value=None, missing=True, missing_reason="no_baseline",
            source="skadia", raw_value=rmssd_today, raw_unit="ms", stale_hours=stale_h,
        )

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


async def _load_sleep_factor(conn, user_id: str, user_tz: str) -> FactorInput:
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
           AND calendar_date <= (NOW() AT TIME ZONE $2)::date
         ORDER BY calendar_date DESC, sleep_end_utc DESC
         LIMIT 1
        """,
        user_id, user_tz,
    )
    if row is None:
        return FactorInput(
            key="sleep", value=None, missing=True, missing_reason="no_data", source="skadia",
        )

    recorded_s = int(row["recorded_s"] or 0)
    age_h = float(row["age_h"]) if row["age_h"] is not None else None

    if recorded_s < SLEEP_MIN_RECORDED_SECONDS:
        # Recording exists but is below the minimum quality threshold → treat as stale.
        return FactorInput(
            key="sleep", value=None, missing=True, missing_reason="stale",
            source="skadia", stale_hours=age_h,
        )
    if age_h is None or age_h > SLEEP_MAX_AGE_H:
        return FactorInput(
            key="sleep", value=None, missing=True, missing_reason="stale",
            source="skadia", stale_hours=age_h,
        )

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


async def _load_training_load_factor(conn, user_id: str, user_tz: str) -> FactorInput:
    """pmc_daily.acwr_ra for today, sport_group='__combined__'."""
    row = await conn.fetchrow(
        """
        SELECT acwr_ra
          FROM pmc_daily
         WHERE user_id = $1::uuid
           AND date = (NOW() AT TIME ZONE $2)::date
           AND sport_group = '__combined__'
        """,
        user_id, user_tz,
    )
    if row is None or row["acwr_ra"] is None:
        return FactorInput(
            key="training_load", value=None, missing=True, missing_reason="no_data",
            source="skadia",
        )
    acwr = float(row["acwr_ra"])
    if acwr == 0.0:
        # PMC convention: acwr_ra=0 → insufficient training-day history
        return FactorInput(
            key="training_load", value=None, missing=True, missing_reason="no_baseline",
            source="skadia", raw_value=acwr, raw_unit="ratio",
        )

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


async def _load_manufacturer_factors(conn, user_id: str, user_tz: str) -> dict | None:
    """health_readiness_scores for today.

    Returns the row when ANY relevant field is non-null (score, level, or any
    of the per-factor pct columns). Path-B eligibility is decided downstream
    via `mfr['score'] is not None` — this loader must not silently discard
    partial rows that still carry usable per-factor data for Path A's mix.
    """
    row = await conn.fetchrow(
        """
        SELECT score, level,
               hrv_factor_pct, sleep_history_factor_pct,
               stress_history_factor_pct, recovery_time_factor_pct,
               acwr_pct, source_type
          FROM health_readiness_scores
         WHERE user_id = $1::uuid
           AND calendar_date = (NOW() AT TIME ZONE $2)::date
           AND (
                score IS NOT NULL
             OR level IS NOT NULL
             OR hrv_factor_pct IS NOT NULL
             OR sleep_history_factor_pct IS NOT NULL
             OR stress_history_factor_pct IS NOT NULL
             OR recovery_time_factor_pct IS NOT NULL
             OR acwr_pct IS NOT NULL
           )
        """,
        user_id, user_tz,
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


async def _load_dfa_alpha1(conn, user_id: str, user_tz: str) -> float | None:
    row = await conn.fetchrow(
        """
        SELECT value_num
          FROM user_daily_features
         WHERE user_id = $1::uuid
           AND feature_key = 'hrv.HRV_DFA_alpha1'
           AND date = (NOW() AT TIME ZONE $2)::date
           AND value_num IS NOT NULL
         ORDER BY computed_at DESC NULLS LAST
         LIMIT 1
        """,
        user_id, user_tz,
    )
    return float(row["value_num"]) if row else None


async def _load_sample_entropy(conn, user_id: str, user_tz: str) -> tuple[float | None, float | None]:
    """Returns (today_value, 30d_median)."""
    today = await conn.fetchrow(
        """
        SELECT value_num
          FROM user_daily_features
         WHERE user_id = $1::uuid
           AND feature_key = 'hrv.HRV_SampEn'
           AND date = (NOW() AT TIME ZONE $2)::date
           AND value_num IS NOT NULL
         ORDER BY computed_at DESC NULLS LAST
         LIMIT 1
        """,
        user_id, user_tz,
    )
    today_v = float(today["value_num"]) if today else None

    median_row = await conn.fetchrow(
        """
        SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY value_num) AS med
          FROM user_daily_features
         WHERE user_id = $1::uuid
           AND feature_key = 'hrv.HRV_SampEn'
           AND date >= ((NOW() AT TIME ZONE $2)::date - INTERVAL '30 days')
           AND date < (NOW() AT TIME ZONE $2)::date
           AND value_num IS NOT NULL
        """,
        user_id, user_tz,
    )
    med = float(median_row["med"]) if median_row and median_row["med"] is not None else None
    return today_v, med


async def _ensure_rhr_changepoint_fresh(conn, user_id: str) -> None:
    """Refresh analytics_cache cache_key='changepoints.resting_hr' if stale.

    Per R1-#17: hoisted out of the readiness compute transaction so the
    detect_changepoints work — which can run for seconds on a long
    resting_hr series — does not extend the readiness write tx and the
    advisory lock window. Caller invokes this BEFORE opening the readiness
    transaction in its own short-lived tx.
    """
    row = await conn.fetchrow(
        """
        SELECT computed_at,
               EXTRACT(EPOCH FROM (NOW() - computed_at)) / 3600.0 AS age_h
          FROM analytics_cache
         WHERE user_id = $1::uuid
           AND cache_key = 'changepoints.resting_hr'
        """,
        user_id,
    )
    needs_recompute = row is None or row["computed_at"] is None or \
        (row["age_h"] is not None and float(row["age_h"]) > 24.0)
    if not needs_recompute:
        return

    try:
        cps = await detect_changepoints(conn, user_id, "resting_hr")
        async with conn.transaction():
            await conn.execute(
                """
                INSERT INTO analytics_cache (user_id, cache_key, data, computed_at)
                VALUES ($1::uuid, 'changepoints.resting_hr', $2::jsonb, NOW())
                ON CONFLICT (user_id, cache_key) DO UPDATE
                SET data = EXCLUDED.data, computed_at = EXCLUDED.computed_at
                """,
                user_id, json.dumps(cps),
            )
    except Exception as exc:  # pragma: no cover — defensive
        logger.warning("rhr changepoint auto-trigger failed for %s: %s", user_id, exc)


def _interpret_rhr_changepoint(data: Any, user_tz: str) -> str | None:
    """Pure interpretation of cached changepoint data → signal string."""
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
        # Accept both 'YYYY-MM-DD' and full datetime ISO strings — R1-#19.
        latest = datetime.fromisoformat(latest_iso).date()
    except (ValueError, TypeError) as exc:
        logger.warning("rhr changepoint date parse failed for %r: %s", latest_iso, exc)
        return None
    user_today = datetime.now(ZoneInfo(user_tz)).date()
    if (user_today - latest).days < 7:
        return "rhr_changepoint_recent"
    return None


async def _read_rhr_changepoint_signal(conn, user_id: str, user_tz: str) -> str | None:
    """Read-only signal lookup — assumes the cache has already been refreshed
    by `_ensure_rhr_changepoint_fresh`. Safe inside the readiness tx."""
    row = await conn.fetchrow(
        """
        SELECT data
          FROM analytics_cache
         WHERE user_id = $1::uuid
           AND cache_key = 'changepoints.resting_hr'
        """,
        user_id,
    )
    if row is None or row["data"] is None:
        return None
    return _interpret_rhr_changepoint(row["data"], user_tz)


async def _load_rhr_changepoint_signal(conn, user_id: str, user_tz: str) -> str | None:
    """Combined ensure-fresh + read. Kept for callers/tests that exercise
    both halves in one call. Production hot-path uses the split functions."""
    await _ensure_rhr_changepoint_fresh(conn, user_id)
    return await _read_rhr_changepoint_signal(conn, user_id, user_tz)


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


# --- Path-selection helper ---------------------------------------------------

def _eigen_is_complete(eigen: dict[str, FactorInput]) -> bool:
    """All critical eigen-factors present (per spec §6.1).

    Used by the auto-best path selector when the user has expressed no
    preference: only when every critical eigen-factor is available do we
    trust Skadia's own composite over the manufacturer's score.
    """
    for k in CRITICAL_FACTORS:
        f = eigen.get(k)
        if f is None or f.missing or f.value is None:
            return False
    return True


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

        # Picking rule (spec §6.4 — auto-best, R1-#5 user decision):
        # - mfr_only                → use mfr
        # - eigen_only              → use eigen
        # - both, pref=manufacturer → mfr
        # - both, pref=skadia      → eigen
        # - both, pref=None         → eigen (Skadia-first when both complete;
        #   path selector already routes to manufacturer-score Path B when
        #   eigen is incomplete, so reaching the mix with pref=None means we
        #   trust our own per-factor numbers).
        use_mfr = mfr_avail and (
            not eigen_avail
            or pref == "manufacturer"
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


def _mfr_to_factors(mfr: dict, eigen: dict[str, FactorInput] | None = None) -> list[dict]:
    """Path-B factor envelope: surface manufacturer sub-factors directly.

    When eigen factors are provided, available_sources is populated per-factor
    so the frontend toggle (f.available_sources?.length >= 2) is visible even
    when the user's preference routes through Path B. Without this, a user with
    full eigen factors + manufacturer score + pref='manufacturer' would see no
    toggle and could never switch back.
    """
    src = mfr.get("source_type") or "garmin"
    factors: list[dict] = []
    weights = {
        "hrv_status": ("hrv_factor_pct", WEIGHT_HRV),
        "sleep": ("sleep_history_factor_pct", WEIGHT_SLEEP),
        "training_load": ("acwr_pct", WEIGHT_LOAD),
    }
    for key, (col, w) in weights.items():
        v = mfr.get(col)
        # Compute available_sources: include 'skadia' when a non-missing eigen
        # factor for this key is present; include the mfr source when mfr provides it.
        avail: list[str] = []
        if eigen is not None:
            ef = eigen.get(key)
            if ef is not None and not ef.missing and ef.value is not None:
                avail.append("skadia")
        if v is not None:
            avail.append(src)
        factors.append({
            "key": key,
            "value": float(v) if v is not None else None,
            "raw_value": float(v) if v is not None else None,
            "raw_unit": "pct",
            "weight": w,
            "source": src,
            "stale_hours": None,
            "missing": v is None,
            "available_sources": avail,
        })
    return factors


# --- Confidence + recommendation --------------------------------------------

async def _derive_confidence(
    conn,
    user_id: str,
    *,
    factors: dict[str, FactorInput],
    path_kind: str,
    user_tz: str,
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
        baseline_ok = await _hrv_baseline_days_geq(conn, user_id, HRV_BASELINE_MIN_DAYS, user_tz)
        acwr_ok = await _acwr_history_days_geq(conn, user_id, ACWR_HISTORY_MIN_DAYS, user_tz)
        if baseline_ok and acwr_ok:
            return "high"
        return "medium"

    if n >= 2:
        return "low" if any_24_48h_stale else "medium"
    return "low"


async def _hrv_baseline_days_geq(conn, user_id: str, days: int, user_tz: str) -> bool:
    # Counts distinct local calendar days across the same union as the HRV
    # baseline mean/sd query so mobile-import users with snapshots-only data
    # qualify for `confidence='high'`.
    row = await conn.fetchrow(
        """
        SELECT COUNT(*) AS n FROM (
            SELECT (ts AT TIME ZONE $3)::date AS day
              FROM health_hrv_night_values
             WHERE user_id = $1::uuid
               AND ts >= ((NOW() AT TIME ZONE $3)::date - ($2::int || ' days')::interval)
            UNION
            SELECT (measured_at_utc AT TIME ZONE $3)::date AS day
              FROM health_snapshots
             WHERE user_id = $1::uuid
               AND rmssd_ms IS NOT NULL
               AND measured_at_utc >= ((NOW() AT TIME ZONE $3)::date - ($2::int || ' days')::interval)
        ) d
        """,
        user_id, days, user_tz,
    )
    return row is not None and row["n"] is not None and int(row["n"]) >= days


async def _acwr_history_days_geq(conn, user_id: str, days: int, user_tz: str) -> bool:
    row = await conn.fetchrow(
        """
        SELECT COUNT(*) AS n
          FROM pmc_daily
         WHERE user_id = $1::uuid
           AND sport_group = '__combined__'
           AND date >= ((NOW() AT TIME ZONE $3)::date - ($2::int || ' days')::interval)
           AND acwr_ra IS NOT NULL AND acwr_ra <> 0
        """,
        user_id, days, user_tz,
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
    now_utc: datetime | None = None,
) -> dict:
    """Schema-stable refused envelope (Plan Phase 5).

    `now_utc` lets callers thread the same UTC timestamp into the JSON envelope
    and the cache row's `computed_at` column (R1-#20). Defaults to a fresh
    `datetime.now(timezone.utc)` when omitted.
    """
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
        "computed_at": (now_utc or datetime.now(timezone.utc)).isoformat().replace("+00:00", "Z"),
        "data_window_end": data_window_end,
        "preferences_version": preferences_version,
        "confidence": None,
        "refused": True,
        "refusal_reason": refusal_reason,
        "factors": factor_list,
        "missing_factors": [
            {
                "key": k,
                "reason": (
                    factors_eigen[k].missing_reason
                    if factors_eigen.get(k) is not None and factors_eigen[k].missing_reason
                    else MISSING_REASONS[0]  # "no_data" fallback
                ),
                "last_seen_at": None,
            }
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


async def _compute_readiness_locked(conn, user_id: str, *, rhr_signal: str | None = None) -> dict:
    """Inside-tx computation. Lock has already been acquired by caller.

    All date anchors use the user's local timezone (users.timezone) so that
    a user in UTC+12 does not see empty "today" data because the server clock
    is still on the previous UTC date.
    """
    # 0. User timezone — anchor all date queries to local calendar day.
    user_tz = await _load_user_timezone(conn, user_id)
    # Single Python-side timestamp used for both the JSON envelope and the
    # cache-row's `computed_at` column (R1-#20). Avoids JSON/DB time drift.
    now_utc = datetime.now(timezone.utc)
    today_iso = now_utc.astimezone(ZoneInfo(user_tz)).date().isoformat()

    # 1. Pref + version snapshot
    pref, pref_version_at_start = await _read_pref(conn, user_id)

    # 2. Own factors
    eigen = {
        "hrv_status":    await _load_hrv_status(conn, user_id, user_tz),
        "sleep":         await _load_sleep_factor(conn, user_id, user_tz),
        "training_load": await _load_training_load_factor(conn, user_id, user_tz),
    }

    # 3. Manufacturer factors
    mfr = await _load_manufacturer_factors(conn, user_id, user_tz)

    # 4. Refusal check (early-exit)
    refused, reason = _check_refusal(eigen, mfr)
    if refused:
        return _refused_envelope(
            refusal_reason=reason or "insufficient_factors",
            data_window_end=today_iso,
            preferences_version=pref_version_at_start,
            factors_eigen=eigen,
            mfr=mfr,
            now_utc=now_utc,
        )

    # 5. Path determination — see spec §6.4 (Path A / Path B definitions) and
    # the user-decision recorded for R1-#5 (Option B "auto-best"): when the
    # user has no explicit preference (`pref is None`), we pick Path B
    # (Skadia / eigen-mix flow) only if all critical eigen-factors are
    # complete; otherwise we fall back to the manufacturer side (Path A here
    # uses the manufacturer's score directly when path_a is false).
    n_eigen = sum(1 for f in eigen.values() if not f.missing and f.value is not None)
    path_a = n_eigen >= 2
    path_b_available = mfr is not None and mfr.get("score") is not None
    if path_a and path_b_available:
        if pref == "skadia":
            path_kind = "A"
        elif pref == "manufacturer":
            path_kind = "B"
        else:  # pref is None — auto-best per R1-#5
            path_kind = "A" if _eigen_is_complete(eigen) else "B"
    elif path_a:
        path_kind = "A"
    else:
        path_kind = "B"

    # Bonus inputs — RHR changepoint cache was refreshed before the tx
    # opened (R1-#17). If the caller did not pass a pre-computed signal,
    # do a read-only lookup here (cache-only, no recompute).
    if rhr_signal is None:
        rhr_signal = await _read_rhr_changepoint_signal(conn, user_id, user_tz)

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
        # Pass eigen so _mfr_to_factors can populate available_sources when both
        # sources have data — required for the frontend source-toggle to appear.
        factors_out = _mfr_to_factors(mfr, eigen=eigen)
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

        dfa = await _load_dfa_alpha1(conn, user_id, user_tz)
        sampen, sampen_med = await _load_sample_entropy(conn, user_id, user_tz)

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
        user_tz=user_tz,
    )
    recommendation_code = _derive_recommendation_code(level, confidence, sig)

    # 9. Pref-Race re-check — return a full refused envelope so any caller sees a
    # consistent shape. The "republish" key is added by compute_readiness wrapper.
    _, pref_version_at_end = await _read_pref(conn, user_id)
    if pref_version_at_end != pref_version_at_start:
        return _refused_envelope(
            refusal_reason="stale_compute_pref_changed",
            data_window_end=today_iso,
            preferences_version=pref_version_at_start,
            factors_eigen=eigen,
            mfr=mfr,
            now_utc=now_utc,
        )

    # 10. Output
    missing_factors = [
        {
            "key": k,
            "reason": (
                used_mix[k].missing_reason
                if used_mix.get(k) is not None and used_mix[k].missing_reason
                else MISSING_REASONS[3]  # "no_data_today" default for Path-A missing
            ),
            "last_seen_at": None,
        }
        for k in CRITICAL_FACTORS
        if (used_mix.get(k) is None or used_mix[k].missing) and path_kind == "A"
    ]

    return {
        "score": int(score),
        "level": level,
        "computed_at": now_utc.isoformat().replace("+00:00", "Z"),
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


async def compute_readiness(conn, user_id: str) -> dict:
    """Race-Verhalten:
    - already_running:        ack ohne Write. Kein Re-Trigger nötig.
    - stale_compute_pref_changed: ack ohne Write **und** anschließend einen
                              neuen Invalidate publishen (handled by
                              handle_readiness_message).
    - no_data / insufficient_factors: schreiben (Refused-Envelope).
    - uncaught exception in _compute_readiness_locked: the original transaction
      is rolled back automatically; a fresh transaction writes a
      service_unavailable refused envelope so the cache does not stay stale
      forever. The original exception is logged and then suppressed (message ack).
    """
    try:
        # R1-#17: Refresh the RHR-changepoint cache BEFORE opening the
        # readiness transaction so detect_changepoints (which can run for
        # seconds on long series) does not extend the advisory-lock window
        # or block parallel readiness writers. Then read the signal from the
        # (now-fresh) cache for use inside the locked compute. Failures here
        # are non-fatal — the locked compute falls back to a read-only lookup
        # and tolerates a missing signal.
        try:
            await _ensure_rhr_changepoint_fresh(conn, user_id)
            user_tz_pre = await _load_user_timezone(conn, user_id)
            rhr_signal = await _read_rhr_changepoint_signal(conn, user_id, user_tz_pre)
        except Exception as pre_exc:  # pragma: no cover — defensive
            logger.warning(
                "compute_readiness: RHR pre-step failed for %s: %s — proceeding without",
                user_id, pre_exc,
            )
            rhr_signal = None

        async with conn.transaction():
            got_lock = await conn.fetchval(
                "SELECT pg_try_advisory_xact_lock(hashtextextended('readiness:' || $1, 0))",
                user_id,
            )
            if not got_lock:
                return {"refused": True, "refusal_reason": "already_running", "written": False}

            result = await _compute_readiness_locked(conn, user_id, rhr_signal=rhr_signal)

            if result.get("refusal_reason") == "stale_compute_pref_changed":
                return {**result, "written": False, "republish": True}

            # R1-#20: write the same UTC timestamp into the cache row that the
            # JSON envelope already carries — single source of truth.
            computed_at_db = datetime.fromisoformat(
                result["computed_at"].replace("Z", "+00:00")
            )
            await conn.execute(
                """
                INSERT INTO analytics_cache (user_id, cache_key, data, computed_at, expires_at)
                VALUES ($1::uuid, 'readiness', $2::jsonb, $4::timestamptz,
                        $4::timestamptz + make_interval(secs => $3))
                ON CONFLICT (user_id, cache_key) DO UPDATE
                SET data        = EXCLUDED.data,
                    computed_at = EXCLUDED.computed_at,
                    expires_at  = EXCLUDED.expires_at
                """,
                user_id, json.dumps(result), int(CACHE_TTL_HOURS * 3600), computed_at_db,
            )
            return {**result, "written": True}

    except Exception as exc:
        # Original transaction has been rolled back. Write a refused envelope in a
        # fresh transaction so the cache does not remain stale forever.
        logger.error(
            "compute_readiness: unhandled exception for %s — writing service_unavailable",
            user_id,
            exc_info=exc,
        )
        try:
            _exc_tz = await _load_user_timezone(conn, user_id)
        except Exception:
            _exc_tz = "UTC"
        exc_now_utc = datetime.now(timezone.utc)
        today_iso = exc_now_utc.astimezone(ZoneInfo(_exc_tz)).date().isoformat()
        refused = _refused_envelope(
            refusal_reason="service_unavailable",
            data_window_end=today_iso,
            preferences_version=None,
            factors_eigen={},
            mfr=None,
            now_utc=exc_now_utc,
        )
        try:
            async with conn.transaction():
                await conn.execute(
                    """
                    INSERT INTO analytics_cache (user_id, cache_key, data, computed_at, expires_at)
                    VALUES ($1::uuid, 'readiness', $2::jsonb, $4::timestamptz,
                            $4::timestamptz + make_interval(secs => $3))
                    ON CONFLICT (user_id, cache_key) DO UPDATE
                    SET data        = EXCLUDED.data,
                        computed_at = EXCLUDED.computed_at,
                        expires_at  = EXCLUDED.expires_at
                    """,
                    user_id, json.dumps(refused), int(CACHE_TTL_HOURS * 3600), exc_now_utc,
                )
        except Exception as write_exc:
            logger.error(
                "compute_readiness: fallback cache write also failed for %s: %s",
                user_id, write_exc,
            )
        return {**refused, "written": False}


async def handle_readiness_message(conn, user_id: str, publisher=None) -> dict:
    """NATS handler wrapper — pref-race triggers re-publish."""
    result = await compute_readiness(conn, user_id)
    if result.get("republish") and publisher is not None:
        await publisher.publish_analytics_invalidate(user_id)
    return result
