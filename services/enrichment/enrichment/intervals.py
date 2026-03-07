"""Intervalltraining-Erkennung via Binary Segmentation."""

import logging
import numpy as np
from typing import Optional

logger = logging.getLogger(__name__)

MIN_INTERVAL_S = 30
MIN_ACTIVITY_S = 600


async def enrich_intervals(pool, activity_id: str, user_id: str) -> None:
    """Erkennt Intervall-Segmente in einer Aktivität."""
    async with pool.acquire() as conn:
        rows = await conn.fetch(
            """SELECT time, power_w, heart_rate, speed_ms, cadence
               FROM activity_streams
               WHERE activity_id = $1
               ORDER BY time""",
            activity_id,
        )

        if len(rows) < MIN_ACTIVITY_S:
            return

        signal = _extract_signal(rows)
        if signal is None:
            return

        times = [r["time"] for r in rows]
        segments = _segment_signal(signal, times)
        if not segments:
            return

        classified = _classify_segments(segments, signal, rows)

        hard_count = sum(1 for s in classified if s["intensity"] == "hard")
        if hard_count < 2:
            return

        await conn.execute(
            "DELETE FROM activity_intervals WHERE activity_id = $1",
            activity_id,
        )

        for i, seg in enumerate(classified):
            await conn.execute(
                """INSERT INTO activity_intervals
                   (activity_id, segment_num, start_time, end_time,
                    duration_s, avg_power_w, avg_heart_rate,
                    avg_speed_ms, avg_cadence, intensity)
                   VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)""",
                activity_id, i + 1,
                seg["start"], seg["end"], seg["duration_s"],
                seg["avg_power"], seg["avg_hr"],
                seg["avg_speed"], seg["avg_cadence"],
                seg["intensity"],
            )

        await conn.execute(
            "UPDATE activities SET training_type = 'interval' WHERE id = $1",
            activity_id,
        )

        logger.info(
            "detected %d intervals (%d hard) for %s",
            len(classified), hard_count, activity_id,
        )


def _extract_signal(rows) -> Optional[np.ndarray]:
    """Wählt das beste Signal: Power > HR > Speed."""
    # Versuche Power
    power = [r["power_w"] for r in rows]
    if sum(1 for p in power if p is not None) > len(rows) * 0.5:
        arr = np.array([p if p is not None else 0.0 for p in power], dtype=float)
        return _smooth(arr)

    # Fallback HR
    hr = [r["heart_rate"] for r in rows]
    if sum(1 for h in hr if h is not None) > len(rows) * 0.5:
        arr = np.array([h if h is not None else 0.0 for h in hr], dtype=float)
        return _smooth(arr)

    # Fallback Speed
    speed = [r["speed_ms"] for r in rows]
    if sum(1 for s in speed if s is not None) > len(rows) * 0.5:
        arr = np.array([s if s is not None else 0.0 for s in speed], dtype=float)
        return _smooth(arr)

    return None


def _smooth(arr: np.ndarray, window: int = 10) -> np.ndarray:
    """Rolling-Mean Glättung."""
    kernel = np.ones(window) / window
    return np.convolve(arr, kernel, mode="same")


def _segment_signal(signal: np.ndarray, times) -> list:
    """Binary Segmentation: teilt Signal rekursiv an Punkten maximaler Varianz-Reduktion."""
    segments = []
    _binary_split(signal, times, 0, len(signal) - 1, segments)
    segments.sort(key=lambda s: s["start_idx"])
    return segments


def _binary_split(signal, times, left, right, segments, depth=0):
    if right - left < MIN_INTERVAL_S or depth > 10:
        segments.append({"start_idx": left, "end_idx": right,
                         "start": times[left], "end": times[right]})
        return

    best_idx = -1
    best_score = -1.0

    total_var = signal[left:right+1].var() * (right - left + 1)

    for i in range(left + MIN_INTERVAL_S, right - MIN_INTERVAL_S):
        left_seg = signal[left:i]
        right_seg = signal[i:right+1]
        if len(left_seg) < 2 or len(right_seg) < 2:
            continue
        score = total_var - (left_seg.var() * len(left_seg) + right_seg.var() * len(right_seg))
        if score > best_score:
            best_score = score
            best_idx = i

    if best_idx < 0 or best_score < signal[left:right+1].std() * 0.5:
        segments.append({"start_idx": left, "end_idx": right,
                         "start": times[left], "end": times[right]})
        return

    _binary_split(signal, times, left, best_idx, segments, depth + 1)
    _binary_split(signal, times, best_idx, right, segments, depth + 1)


def _classify_segments(segments, signal, rows) -> list:
    """Klassifiziert Segmente anhand des Signalwerts relativ zum Gesamtmittelwert."""
    global_mean = signal.mean()
    result = []

    for seg in segments:
        sl = signal[seg["start_idx"]:seg["end_idx"]+1]
        seg_mean = sl.mean() if len(sl) > 0 else global_mean

        ratio = seg_mean / global_mean if global_mean > 0 else 1.0
        if ratio > 1.10:
            intensity = "hard"
        elif ratio >= 0.90:
            intensity = "moderate"
        elif ratio >= 0.70:
            intensity = "easy"
        else:
            intensity = "recovery"

        # Aggregiere Stream-Werte für dieses Segment
        seg_rows = rows[seg["start_idx"]:seg["end_idx"]+1]
        powers = [r["power_w"] for r in seg_rows if r["power_w"] is not None]
        hrs = [r["heart_rate"] for r in seg_rows if r["heart_rate"] is not None]
        speeds = [r["speed_ms"] for r in seg_rows if r["speed_ms"] is not None]
        cadences = [r["cadence"] for r in seg_rows if r["cadence"] is not None]

        duration_s = int((seg["end"] - seg["start"]).total_seconds())

        result.append({
            "start": seg["start"],
            "end": seg["end"],
            "duration_s": max(duration_s, 1),
            "avg_power": sum(powers) / len(powers) if powers else None,
            "avg_hr": int(sum(hrs) / len(hrs)) if hrs else None,
            "avg_speed": sum(speeds) / len(speeds) if speeds else None,
            "avg_cadence": sum(cadences) / len(cadences) if cadences else None,
            "intensity": intensity,
        })

    return result
