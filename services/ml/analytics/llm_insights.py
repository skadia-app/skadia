"""LLM-gestützte Erklärungen für Analytics-Ergebnisse."""
import logging

logger = logging.getLogger(__name__)

LLM_TIMEOUT = 30.0  # Sekunden
SUPPORTED_LOCALES = {"de", "en", "fr", "es", "it"}
DEFAULT_LOCALE = "de"
PROTECTED_TERMS = (
    "Skadia",
    "Garmin",
    "Strava",
    "Komoot",
    "OSM",
    "BRouter",
    "MapLibre",
    "Power Phase",
    "Body Battery",
    "Mountainbike",
    "MTB",
    "TSS",
    "FTP",
    "PMC",
    "HRV",
    "FIT",
    "GPX",
    "CSV",
    "ZIP",
    "TCX",
)

LANGUAGE_INSTRUCTIONS: dict[str, str] = {
    "de": "Answer in German.",
    "en": "Answer in English.",
    "fr": "Answer in French.",
    "es": "Answer in Spanish.",
    "it": "Rispondi in italiano.",
}

TEMPLATES: dict[str, str] = {
    "readiness": (
        "You are a training advisor. The athlete has a readiness score of "
        "{score}/100 today (level: {level}). "
        "Factors: HRV trend {hrv_value}/100 ({hrv_status}), "
        "fatigue {fatigue_value}/100 ({fatigue_status}), "
        "sleep {sleep_value}/100 ({sleep_status}). "
        "Give a short, direct recommendation (max. 3 sentences) about what "
        "the athlete should train or avoid today."
    ),
    "warnings": (
        "You are a training advisor. {count} HRV anomalies were detected in the last "
        "90 days. The strongest anomaly had a z-score of {worst_zscore:.2f} "
        "on {worst_date}. "
        "Briefly explain (max. 2 sentences) what this means and what the athlete "
        "should watch."
    ),
    "clusters": (
        "You are a training advisor. The athlete's activities were grouped into "
        "{k} clusters: {cluster_summary}. "
        "Give a short assessment (max. 3 sentences) of training variety and whether "
        "the distribution looks sensible."
    ),
    "projection": (
        "You are a training advisor. The athlete's current CTL is {current_ctl:.1f} "
        "and is projected to be {projected_ctl_28d:.1f} in 28 days "
        "(trend: {trend}, slope: {slope:.3f} CTL/day). "
        "Give a short assessment (max. 2 sentences) and one concrete training "
        "recommendation."
    ),
}


def normalize_locale(locale: str | None) -> str:
    if not locale:
        return DEFAULT_LOCALE
    normalized = (
        locale.strip().lower().split("-", maxsplit=1)[0].split("_", maxsplit=1)[0]
    )
    if normalized in SUPPORTED_LOCALES:
        return normalized
    return DEFAULT_LOCALE


def build_prompt(
    insight_type: str,
    data: dict,
    locale: str | None = None,
    protected_terms: list[str] | tuple[str, ...] | None = None,
) -> str | None:
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

    language = LANGUAGE_INSTRUCTIONS[normalize_locale(locale)]
    protected = ", ".join(dict.fromkeys((*PROTECTED_TERMS, *(protected_terms or ()))))
    return (
        f"{prompt} {language} Preserve these terms exactly when used: "
        f"{protected}."
    )


async def generate_explanation(
    insight_type: str,
    data: dict,
    locale: str | None = None,
    protected_terms: list[str] | tuple[str, ...] | None = None,
) -> str | None:
    """Generiert eine LLM-Erklärung für einen Analytics-Insight.

    Args:
        insight_type: Schlüssel in TEMPLATES (z.B. "readiness", "warnings").
        data:         Dict mit Template-Variablen.
        locale:       Optionaler BCP-47-Sprachcode; Fallback ist Deutsch.
        protected_terms: Zusätzliche Begriffe, die unverändert bleiben sollen.

    Rückgabe:
        Erklärungstext in der gewünschten Sprache oder None bei Fehler/fehlendem Template.
    """
    prompt = build_prompt(
        insight_type,
        data,
        locale=locale,
        protected_terms=protected_terms,
    )
    if prompt is None:
        return None

    logger.info("generate_explanation: analysis LLM role is deferred in this branch")
    _ = prompt
    return None
