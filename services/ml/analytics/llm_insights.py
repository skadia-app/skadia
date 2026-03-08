"""LLM-gestützte Erklärungen für Analytics-Ergebnisse."""
import logging
import os

import httpx

logger = logging.getLogger(__name__)

LLM_BASE_URL = os.environ.get("LLM_BASE_URL", "http://litellm:4000/v1")
LLM_MODEL = os.environ.get("LLM_MODEL", "skadia-default")
LLM_TIMEOUT = 30.0  # Sekunden

TEMPLATES: dict[str, str] = {
    "readiness": (
        "Du bist ein Trainingsberater. Der Athlet hat heute einen Bereitschafts-Score von "
        "{score}/100 (Level: {level}). "
        "Faktoren: HRV-Trend {hrv_value}/100 ({hrv_status}), "
        "Ermüdung {fatigue_value}/100 ({fatigue_status}), "
        "Schlaf {sleep_value}/100 ({sleep_status}). "
        "Gib eine kurze, direkte Empfehlung (max. 3 Sätze) auf Deutsch, "
        "was der Athlet heute trainieren oder unterlassen sollte."
    ),
    "warnings": (
        "Du bist ein Trainingsberater. Es wurden {count} HRV-Anomalien in den letzten "
        "90 Tagen erkannt. Die stärkste Anomalie hatte einen Z-Score von {worst_zscore:.2f} "
        "am {worst_date}. "
        "Erkläre kurz (max. 2 Sätze) auf Deutsch, was das bedeutet und was der Athlet "
        "beachten sollte."
    ),
    "clusters": (
        "Du bist ein Trainingsberater. Die Aktivitäten des Athleten wurden in "
        "{k} Gruppen eingeteilt: {cluster_summary}. "
        "Gib eine kurze Einschätzung (max. 3 Sätze) auf Deutsch zur Trainingsvielfalt "
        "und ob die Verteilung sinnvoll erscheint."
    ),
    "projection": (
        "Du bist ein Trainingsberater. Der aktuelle CTL-Wert des Athleten beträgt "
        "{current_ctl:.1f} und wird in 28 Tagen voraussichtlich {projected_ctl_28d:.1f} "
        "betragen (Trend: {trend}, Steigung: {slope:.3f} CTL/Tag). "
        "Gib eine kurze Einschätzung (max. 2 Sätze) auf Deutsch und eine konkrete "
        "Trainingsempfehlung."
    ),
}


async def generate_explanation(insight_type: str, data: dict) -> str | None:
    """Generiert eine LLM-Erklärung für einen Analytics-Insight.

    Args:
        insight_type: Schlüssel in TEMPLATES (z.B. "readiness", "warnings").
        data:         Dict mit Template-Variablen.

    Rückgabe:
        Erklärungstext auf Deutsch oder None bei Fehler/fehlendem Template.
    """
    template = TEMPLATES.get(insight_type)
    if template is None:
        logger.warning(
            "generate_explanation: unknown insight_type %r", insight_type
        )
        return None

    try:
        prompt = template.format(**data)
    except KeyError as e:
        logger.warning(
            "generate_explanation: missing template key for %r: %s", insight_type, e
        )
        return None

    try:
        async with httpx.AsyncClient(timeout=LLM_TIMEOUT) as client:
            response = await client.post(
                f"{LLM_BASE_URL}/chat/completions",
                json={
                    "model": LLM_MODEL,
                    "messages": [
                        {"role": "system", "content": "/no_think"},
                        {"role": "user", "content": prompt},
                    ],
                    "max_tokens": 200,
                    "temperature": 0.5,
                },
            )
            response.raise_for_status()
            payload = response.json()
            text = (
                payload.get("choices", [{}])[0]
                .get("message", {})
                .get("content", "")
                .strip()
            )
            return text if text else None
    except httpx.TimeoutException:
        logger.warning(
            "generate_explanation: LLM request timed out for %r", insight_type
        )
        return None
    except Exception as e:
        logger.warning(
            "generate_explanation: LLM request failed for %r: %s", insight_type, e
        )
        return None
