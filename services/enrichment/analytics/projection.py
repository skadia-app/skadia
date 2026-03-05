"""CTL trend projection via linear regression."""
import logging

import numpy as np

logger = logging.getLogger(__name__)

_MIN_DAYS = 14
_PROJECTION_DAYS = 28
_PLATEAU_SLOPE_THRESHOLD = 0.05  # CTL/Tag


async def compute_projection(pool, user_id: str) -> dict:
    """Berechnet einen 28-Tage-CTL-Ausblick via linearer Regression.

    Lädt die letzten 90 PMC-Tage, fittet eine Gerade auf CTL
    und extrapoliert 28 Tage in die Zukunft.

    Rückgabe:
      slope:            CTL-Änderung pro Tag (Regressionssteigung)
      current_ctl:      Aktueller CTL-Wert (letzter Datenpunkt)
      projected_ctl_28d: Projizierter CTL-Wert in 28 Tagen
      trend:            "aufbau" | "abbau" | "plateau"
    """
    try:
        rows = await pool.fetch(
            """
            SELECT date, ctl FROM pmc_daily
            WHERE user_id = $1
              AND date >= CURRENT_DATE - interval '90 days'
            ORDER BY date ASC
            """,
            user_id,
        )
    except Exception as e:
        logger.error("compute_projection: DB query failed for user %s: %s", user_id, e)
        return _empty_projection()

    if len(rows) < _MIN_DAYS:
        logger.debug(
            "compute_projection: insufficient PMC data for user %s (%d days)",
            user_id,
            len(rows),
        )
        return _empty_projection()

    ctl_values = [float(r["ctl"]) for r in rows if r["ctl"] is not None]
    if len(ctl_values) < _MIN_DAYS:
        return _empty_projection()

    x = np.arange(len(ctl_values), dtype=np.float64)
    y = np.array(ctl_values, dtype=np.float64)

    try:
        coeffs = np.polyfit(x, y, 1)
    except Exception as e:
        logger.error("compute_projection: polyfit failed for user %s: %s", user_id, e)
        return _empty_projection()

    slope = float(coeffs[0])
    current_ctl = ctl_values[-1]

    # Extrapolation: aktuelle Position = len(ctl_values) - 1, + 28 Tage
    x_future = len(ctl_values) - 1 + _PROJECTION_DAYS
    projected_ctl_28d = float(np.polyval(coeffs, x_future))

    if slope >= _PLATEAU_SLOPE_THRESHOLD:
        trend = "aufbau"
    elif slope <= -_PLATEAU_SLOPE_THRESHOLD:
        trend = "abbau"
    else:
        trend = "plateau"

    return {
        "slope": round(slope, 4),
        "current_ctl": round(current_ctl, 1),
        "projected_ctl_28d": round(projected_ctl_28d, 1),
        "trend": trend,
        "data_points": len(ctl_values),
    }


def _empty_projection() -> dict:
    return {
        "slope": None,
        "current_ctl": None,
        "projected_ctl_28d": None,
        "trend": None,
        "data_points": 0,
    }
