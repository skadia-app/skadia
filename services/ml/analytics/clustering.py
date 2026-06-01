"""Activity clustering via KMeans."""
import logging

import numpy as np
from sklearn.cluster import KMeans
from sklearn.preprocessing import StandardScaler

logger = logging.getLogger(__name__)

_MIN_ACTIVITIES = 10
_DEFAULT_LOCALE = "de"
_SUPPORTED_LOCALES = {"de", "en", "fr", "es", "it"}

_LABELS: dict[str, dict[str, str]] = {
    "mountain": {
        "de": "Berg",
        "en": "Climb",
        "fr": "Montée",
        "es": "Montaña",
        "it": "Salita",
    },
    "recovery": {
        "de": "Erholung",
        "en": "Recovery",
        "fr": "Récupération",
        "es": "Recuperación",
        "it": "Recupero",
    },
    "endurance": {
        "de": "Ausdauer",
        "en": "Endurance",
        "fr": "Endurance",
        "es": "Resistencia",
        "it": "Resistenza",
    },
    "interval": {
        "de": "Intervall",
        "en": "Interval",
        "fr": "Intervalle",
        "es": "Intervalo",
        "it": "Intervallo",
    },
    "race": {
        "de": "Wettkampf",
        "en": "Race",
        "fr": "Course",
        "es": "Competición",
        "it": "Gara",
    },
    "mixed": {
        "de": "Gemischt",
        "en": "Mixed",
        "fr": "Mixte",
        "es": "Mixto",
        "it": "Misto",
    },
}


def _normalize_locale(locale: str | None) -> str:
    if not locale:
        return _DEFAULT_LOCALE
    normalized = (
        locale.strip().lower().split("-", maxsplit=1)[0].split("_", maxsplit=1)[0]
    )
    if normalized in _SUPPORTED_LOCALES:
        return normalized
    return _DEFAULT_LOCALE


async def _fetch_user_locale(conn, user_id: str) -> str:
    try:
        row = await conn.fetchrow(
            "SELECT locale FROM users WHERE id = $1::uuid",
            user_id,
        )
    except Exception as e:
        logger.warning(
            "cluster_activities: locale query failed for user %s: %s", user_id, e
        )
        return _DEFAULT_LOCALE
    if row is None:
        return _DEFAULT_LOCALE
    return _normalize_locale(row["locale"])


async def cluster_activities(conn, user_id: str) -> list:
    """Gruppiert Aktivitäten in 3–6 Cluster via KMeans.

    Features (alle normalisiert):
      - duration_h:  Dauer in Stunden
      - hr_pct:      Durchschnittliche HF als Prozent der Max-HF
      - elevation_m: Höhenmeter
      - power_w:     Durchschnittliche Leistung (Watt)

    Rückgabe: Liste von Cluster-Dicts mit id, label, center, activity_ids.
    Graceful: [] bei weniger als 10 Aktivitäten.
    """
    try:
        rows = await conn.fetch(
            """
            SELECT
                a.id,
                COALESCE(a.elapsed_time_s, 0)                   AS elapsed_s,
                COALESCE(a.avg_heart_rate, 0)                   AS avg_hr,
                COALESCE(u.effective_max_hr, 190)               AS max_hr,
                COALESCE(a.elevation_gain_m, 0)                 AS elevation_m,
                COALESCE(a.avg_power_w, 0)                      AS power_w
            FROM activities a
            LEFT JOIN user_profiles u ON u.user_id = a.user_id
            WHERE a.user_id = $1
              AND a.elapsed_time_s IS NOT NULL
              AND a.elapsed_time_s > 0
            ORDER BY a.started_at DESC
            LIMIT 2000
            """,
            user_id,
        )
    except Exception as e:
        logger.error("cluster_activities: DB query failed for user %s: %s", user_id, e)
        return []

    if len(rows) < _MIN_ACTIVITIES:
        logger.debug(
            "cluster_activities: insufficient activities for user %s (%d)",
            user_id,
            len(rows),
        )
        return []

    locale = await _fetch_user_locale(conn, user_id)
    ids = [str(r["id"]) for r in rows]

    def safe_float(v) -> float:
        try:
            return float(v) if v is not None else 0.0
        except (TypeError, ValueError):
            return 0.0

    features = np.array(
        [
            [
                safe_float(r["elapsed_s"]) / 3600.0,  # duration_h
                (
                    safe_float(r["avg_hr"]) / safe_float(r["max_hr"]) * 100.0
                    if safe_float(r["max_hr"]) > 0
                    else 0.0
                ),  # hr_pct
                safe_float(r["elevation_m"]),  # elevation_m
                safe_float(r["power_w"]),  # power_w
            ]
            for r in rows
        ],
        dtype=np.float64,
    )

    # Normalisieren
    scaler = StandardScaler()
    features_scaled = scaler.fit_transform(features)

    k = min(6, max(3, len(rows) // 50))

    try:
        kmeans = KMeans(n_clusters=k, random_state=42, n_init=10)
        labels = kmeans.fit_predict(features_scaled)
    except Exception as e:
        logger.error("cluster_activities: KMeans failed for user %s: %s", user_id, e)
        return []

    # Cluster-Center in Original-Skala zurückrechnen
    centers_original = scaler.inverse_transform(kmeans.cluster_centers_)

    clusters = []
    for cluster_id in range(k):
        mask = labels == cluster_id
        member_ids = [ids[i] for i, m in enumerate(mask) if m]
        if not member_ids:
            continue

        center = centers_original[cluster_id]
        center_dict = {
            "duration_h": round(float(center[0]), 2),
            "hr_pct": round(float(center[1]), 1),
            "elevation_m": round(float(center[2]), 0),
            "power_w": round(float(center[3]), 1),
        }

        clusters.append(
            {
                "id": cluster_id,
                "label_key": _auto_label_key(center_dict),
                "label": _auto_label(center_dict, locale=locale),
                "center": center_dict,
                "count": len(member_ids),
                "activity_ids": member_ids,
            }
        )

    # Absteigend nach Größe sortieren
    clusters.sort(key=lambda c: c["count"], reverse=True)
    return clusters


def _auto_label_key(center: dict) -> str:
    """Stable cluster label code derived from the cluster center."""
    dur = center["duration_h"]
    hr = center["hr_pct"]
    elev = center["elevation_m"]
    power = center["power_w"]

    short = dur < 1.0
    long_ = dur >= 3.0
    easy = hr < 65.0 and power < 150.0
    intense = hr >= 75.0 or power >= 250.0
    hilly = elev >= 500.0

    if hilly:
        return "mountain"
    if short and easy:
        return "recovery"
    if long_ and easy:
        return "endurance"
    if short and intense:
        return "interval"
    if long_ and intense:
        return "race"
    return "mixed"


def _auto_label(center: dict, locale: str | None = None) -> str:
    """Heuristische Cluster-Benennung anhand des Cluster-Zentrums."""
    return _LABELS[_auto_label_key(center)][_normalize_locale(locale)]
