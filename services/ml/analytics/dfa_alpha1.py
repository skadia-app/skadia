"""DFA alpha1 Berechnung aus R-R-Intervall-Streams."""
import logging

import numpy as np

logger = logging.getLogger(__name__)

_MIN_RR_COUNT = 60  # Mindest-Anzahl RR-Intervalle für valide DFA
_SCALES = list(range(4, 17))  # n=4..16 (Kurzzeit-Fluktuationen)


def compute_dfa_alpha1(
    rr_intervals: list,
    window_size: int = 180,
) -> list:
    """Berechnet DFA-alpha1 für gleitende 3-Minuten-Fenster.

    Args:
        rr_intervals: Liste von R-R-Intervallen in Millisekunden.
        window_size:  Anzahl der RR-Intervalle pro Analysefenster (≈ 3 min
                      bei ~60 bpm entspricht ~180 Schlägen).

    Rückgabe:
        Liste von Dicts: [{segment, alpha1, mean_rr}]
        Leere Liste bei Fehler oder zu wenig Daten.
    """
    if not rr_intervals or len(rr_intervals) < _MIN_RR_COUNT:
        logger.debug(
            "compute_dfa_alpha1: insufficient RR data (%d intervals)", len(rr_intervals)
        )
        return []

    try:
        rr = np.array(rr_intervals, dtype=np.float64)
    except Exception as e:
        logger.error("compute_dfa_alpha1: array conversion failed: %s", e)
        return []

    results = []
    step = window_size // 2  # 50 % Überlappung
    n_segments = max(1, (len(rr) - window_size) // step + 1)

    for seg_idx in range(n_segments):
        start = seg_idx * step
        end = start + window_size
        if end > len(rr):
            break

        window = rr[start:end]
        alpha = _dfa(window)
        if alpha is None:
            continue

        results.append(
            {
                "segment": seg_idx,
                "alpha1": round(float(alpha), 4),
                "mean_rr": round(float(np.mean(window)), 1),
            }
        )

    return results


def _dfa(rr: np.ndarray) -> float | None:
    """Detrended Fluctuation Analysis (DFA) alpha-Exponent.

    Berechnet den DFA-alpha-Exponenten für einen RR-Intervall-Segment.

    Algorithmus:
      1. Kumulierte Summe (Profil) der demeaned RR-Reihe
      2. Für jede Skalenlänge n: teile Profil in non-overlapping Fenster,
         entferne linearen Trend (polyfit Grad 1), berechne RMSE
      3. Log-Log-Regression von F(n) auf n → Steigung = alpha
    """
    n = len(rr)
    if n < max(_SCALES) * 2:
        return None

    # 1. Profil: kumulierte Summe der demeaned Reihe
    profile = np.cumsum(rr - np.mean(rr))

    log_scales = []
    log_fluct = []

    for scale in _SCALES:
        if scale < 4 or scale > n // 4:
            continue

        n_windows = n // scale
        if n_windows < 2:
            continue

        f_sq = []
        for w in range(n_windows):
            segment = profile[w * scale : (w + 1) * scale]
            x = np.arange(scale, dtype=np.float64)
            try:
                coeffs = np.polyfit(x, segment, 1)
                trend = np.polyval(coeffs, x)
                residuals = segment - trend
                f_sq.append(np.mean(residuals ** 2))
            except Exception:
                continue

        if not f_sq:
            continue

        fluctuation = np.sqrt(np.mean(f_sq))
        if fluctuation <= 0:
            continue

        log_scales.append(np.log(float(scale)))
        log_fluct.append(np.log(fluctuation))

    if len(log_scales) < 4:
        return None

    try:
        coeffs = np.polyfit(log_scales, log_fluct, 1)
        alpha = float(coeffs[0])
    except Exception as e:
        logger.debug("_dfa: polyfit failed: %s", e)
        return None

    # Plausibilitätsprüfung: DFA alpha1 liegt typisch zwischen 0.5 und 2.0
    if not (0.0 < alpha < 3.0):
        return None

    return alpha
