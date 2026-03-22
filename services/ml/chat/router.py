"""Semantic query router — classifies user queries into domains.

Layer 1: Keyword fast-path (handles ~50% of queries, <1ms)
Layer 2: Embedding similarity via BGE-M3 (future, ~25ms)
Layer 3: Fallback to default domains
"""
from __future__ import annotations

from dataclasses import dataclass

from chat.profiles import ModelProfile


@dataclass
class RouteResult:
    domains: list[str]
    confidence: float


# Keyword sets per domain (German, lowercase)
DOMAIN_KEYWORDS: dict[str, set[str]] = {
    "activities": {
        "aktivität", "aktivitäten", "training", "trainings",
        "lauf", "laufen", "joggen", "gelaufen",
        "rad", "radfahren", "radtour", "biken", "gefahren",
        "wandern", "wanderung", "gewandert",
        "schwimmen", "geschwommen",
        "wann war", "wie oft", "letzte", "letzten", "letztes",
        "schnellste", "längste", "kürzeste", "weiteste",
    },
    "health": {
        "hrv", "herzratenvariabilität", "herzfrequenzvariabilität",
        "schlaf", "geschlafen", "schlafen",
        "erholung", "erholt", "erholungszeit",
        "puls", "ruhepuls", "herzfrequenz",
        "readiness", "bereitschaft",
        "müde", "müdigkeit", "erschöpft",
        "krank", "erkältung", "infekt",
        "stress", "gestresst",
        "body battery", "spo2", "sauerstoff",
        "gewicht", "körperfett",
    },
    "training": {
        "ctl", "atl", "tsb", "tss",
        "form", "fitness", "ermüdung",
        "belastung", "trainingsbelastung",
        "intervall", "tempo", "tempolauf",
        "zone", "zonen", "herzfrequenzzone",
        "acwr", "workload",
        "trainingsplan", "periodisierung",
        "übertraining", "überlastet",
    },
    "gear": {
        "schuh", "schuhe", "laufschuh", "laufschuhe",
        "rad", "fahrrad", "bike",
        "ausrüstung", "equipment", "gear",
        "verschleiß", "abnutzung",
        "km-stand", "kilometerstand",
        "helm", "brille", "uhr", "garmin",
        "batterie", "akku",
        "material", "reparatur", "wartung",
        "kette", "reifen", "kassette", "bremsbelag",
    },
    "geo": {
        "gipfel", "berg", "berge", "summit",
        "summit", "summits",
        "pass", "pässe", "passüberquerung",
        "bestiegen", "besteigung", "besteigungen",
        "see", "seen", "fluss", "bach", "gewässer",
        "wald", "wälder", "forst",
        "hütte", "hütten", "alm", "alpen",
        "einkehr", "restaurant", "café", "gasthaus",
        "höhe", "höhenmeter", "aufstieg",
        "route", "routen", "strecke",
        "ort", "region", "stadt", "karte",
    },
    "segments": {
        "segment", "segmente",
        "bestzeit", "bestzeiten",
        "pr", "personal record",
        "rekord", "rekorde",
        "runde", "runden", "lap", "laps",
        "leaderboard", "rangliste",
        "effort", "efforts",
    },
    "photos": {
        "foto", "fotos", "bild", "bilder", "photo", "tier", "tiere",
        "landschaft", "gesehen", "steinbock", "murmeltier", "panorama",
    },
    "routing": {
        "route", "routenplanung", "navigation", "wegpunkt",
        "wanderung planen", "radtour planen", "tour planen",
        "strecke", "weg", "pfad", "isochrone", "erreichbar",
        "wie komme ich", "kürzester weg", "schönste route", "rundtour",
        "schwierigkeit", "sac", "mtb skala", "steigung",
        "asphalt", "schotter", "profil", "gelände",
    },
}

# Which domains include which tools (for tool filtering)
DOMAIN_TOOLS: dict[str, list[str]] = {
    "activities": ["query_activities", "aggregate_stats", "compare_periods", "get_activity_detail", "get_gear_for_activity"],
    "health": ["get_health_status", "get_analytics"],
    "training": ["get_training_load", "compare_periods", "get_analytics"],
    "gear": ["get_gear_stats", "get_gear_for_activity", "get_gear_documents"],
    "geo": ["query_geo_features", "search_routes"],
    "segments": ["query_segments"],
    "photos": ["search_photos_by_content"],
    "routing": [
        "search_pois_nearby", "search_trails_by_difficulty",
        "plan_route", "search_pois_on_route", "get_isochrone",
        "query_geo_features", "search_routes",
    ],
}

# Always-available tools (not domain-filtered)
GLOBAL_TOOLS = ["run_sql"]

ALL_DOMAINS = list(DOMAIN_KEYWORDS.keys())


def route_query(query: str, profile: ModelProfile) -> RouteResult:
    """Route a user query to 1-2 relevant domains.

    For large model profiles (use_router=False), returns all domains.
    """
    if not profile.use_router:
        return RouteResult(domains=ALL_DOMAINS, confidence=1.0)

    query_lower = query.lower()

    # Layer 1: Keyword fast-path
    scores: dict[str, int] = {}
    for domain, keywords in DOMAIN_KEYWORDS.items():
        hits = sum(1 for kw in keywords if kw in query_lower)
        if hits > 0:
            scores[domain] = hits

    if scores:
        sorted_domains = sorted(scores, key=scores.get, reverse=True)
        return RouteResult(
            domains=sorted_domains[:2],
            confidence=0.9 if scores[sorted_domains[0]] >= 2 else 0.7,
        )

    # Layer 2: Embedding routing (TODO — BGE-M3 integration)
    # For now, fall through to default

    # Layer 3: Fallback — general domains
    return RouteResult(
        domains=["activities", "health"],
        confidence=0.3,
    )


def get_tools_for_domains(domains: list[str], profile: ModelProfile) -> list[str]:
    """Get the set of tool names available for the given domains."""
    if not profile.use_router:
        # Large model: all tools
        tools = set()
        for tool_list in DOMAIN_TOOLS.values():
            tools.update(tool_list)
        tools.update(GLOBAL_TOOLS)
        return sorted(tools)

    tools = set()
    for domain in domains:
        tools.update(DOMAIN_TOOLS.get(domain, []))

    # Add global tools if enabled
    if profile.enable_sql_agent:
        tools.update(GLOBAL_TOOLS)

    return sorted(tools)
