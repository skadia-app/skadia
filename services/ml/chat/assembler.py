"""Context assembler — builds the system prompt dynamically.

Combines base prompt + skill modules + tool guide + few-shot examples
based on the active model profile and router results.
"""
from __future__ import annotations

import json
from datetime import date
from pathlib import Path

from chat.profiles import ModelProfile
from chat.skill_loader import Skill


FEW_SHOT_FILE = Path(__file__).parent / "config" / "few_shot_examples.json"

SYSTEM_PROMPT_COMPACT = """Du bist ein Trainingsanalyst für Ausdauersport. Heute ist {today}.

## Regeln
1. Sprich direkt und knapp — Zahlen vor Prosa.
2. Nenne konkrete Werte: Distanz (km), Zeit, Pace (min/km), Herzfrequenz (S/min), Höhenmeter.
3. Fasse Tool-Ergebnisse natürlichsprachlich zusammen — niemals JSON ausgeben.
4. Wenn keine Daten: sag das klar und kurz.
5. Antworte auf Deutsch.
6. Bei mehrdeutigen Fragen: 30 Tage für Trends, 365 Tage für "wann war mein letztes...".
7. Frag nach wenn die Frage unklar ist.
8. Ordne Werte ein: "überdurchschnittlich", "im Vergleich zum Vormonat +15%".

## Referenzwerte
HRV (RMSSD): <20 schlecht | 20-40 moderat | 40-60 gut | >60 exzellent
TSB (Form): <-30 überlastet | -30..-10 produktiv | -10..+5 optimal | >+15 Formverlust
ACWR: <0.8 unterbelastet | 0.8-1.3 optimal | >1.5 Verletzungsrisiko

## Sport-Typ Mapping
Laufen/Joggen → running | Radfahren → cycling | Wandern → hiking
Gehen → walking | Schwimmen → swimming | Trail → trail_running
MTB → mountain_biking | Ski → alpine_skiing | Langlauf → cross_country_skiing
Kraft → strength_training | Bergsteigen → mountaineering"""

SYSTEM_PROMPT_STANDARD = SYSTEM_PROMPT_COMPACT + """

## Weitere Sportarten
Gravel → gravel_cycling | Indoor-Rad → indoor_cycling | Freiwasser → open_water_swimming
E-Bike → e_bike | Yoga → yoga | Rudern → rowing | SUP → stand_up_paddleboarding

## Einheiten-Umrechnung
- Pace (min/km) = (1000 / speed_ms) / 60
- Speed (km/h) = speed_ms × 3.6
- Distanz (km) = distance_m / 1000
- Dauer (h) = duration_s / 3600"""

SYSTEM_PROMPT_RICH = SYSTEM_PROMPT_STANDARD + """

## Trainingstheorie
- Grundlagenausdauer (GA1): 60-75% HFmax, Fettverbrennung, aerobe Basis
- Entwicklungsbereich (GA2): 75-85% HFmax, Laktattoleranz
- Schwellenbereich (EB): 85-92% HFmax, anaerobe Schwelle
- Spitzenbereich (SB): >92% HFmax, VO2max, kurze Intervalle
- Superkompensation: Training → Ermüdung → Erholung → Anpassung
- Periodisierung: Makro (Saison) → Meso (4-6 Wochen) → Mikro (1 Woche)

## Physiologische Marker
- Maximale Herzfrequenz (HFmax): 220 - Alter (Faustformel), individuell variabel
- FTP (Functional Threshold Power): Leistung an der anaeroben Schwelle (W)
- LTHR (Lactate Threshold Heart Rate): HF an der anaeroben Schwelle (S/min)
- VO2max: maximale Sauerstoffaufnahme (ml/kg/min)"""

_PROMPT_MODES = {
    "compact": SYSTEM_PROMPT_COMPACT,
    "standard": SYSTEM_PROMPT_STANDARD,
    "rich": SYSTEM_PROMPT_RICH,
}

TOOL_GUIDE = """## Werkzeug-Auswahl
- "Wie viele / Wie weit insgesamt" → aggregate_stats (NICHT query_activities)
- "Zeig mir / Welche Aktivitäten" → query_activities
- "Gipfel / Pass / See / Hütte" → query_geo_features
- "HRV / Schlaf / Erholung" → get_health_status
- "CTL / ATL / Form / Belastung" → get_training_load
- "Segment / Bestzeit / PR" → query_segments
- "Ausrüstung / Schuhe / km-Stand" → get_gear_stats
- "Anomalie / Korrelation / Trend" → get_analytics
- Zwei Zeiträume vergleichen → compare_periods"""


def _load_few_shot_library() -> list[dict]:
    """Load the few-shot example library from JSON."""
    if not FEW_SHOT_FILE.exists():
        return []
    with open(FEW_SHOT_FILE) as f:
        return json.load(f)


def assemble_system_prompt(
    profile: ModelProfile,
    skills: list[Skill],
    today: date | None = None,
) -> str:
    """Assemble the complete system prompt from components.

    Args:
        profile: Active model capability profile
        skills: Selected skill modules (from router)
        today: Current date (default: today)
    """
    today_str = (today or date.today()).strftime("%d.%m.%Y")
    base = _PROMPT_MODES.get(profile.system_prompt_mode, SYSTEM_PROMPT_COMPACT)
    parts = [base.format(today=today_str)]

    # Inject skill contexts
    for skill in skills:
        parts.append("")
        parts.append(skill.to_context_string())

    # Add tool guide for small/medium models
    if profile.use_router:
        parts.append("")
        parts.append(TOOL_GUIDE)

    # Add few-shot examples from skills
    if profile.few_shot_count > 0:
        few_shots = []
        for skill in skills:
            few_shots.extend(skill.few_shot_examples[:2])
        if few_shots:
            parts.append("")
            parts.append("## Beispiele")
            for ex in few_shots[:profile.few_shot_count]:
                parts.append(f'Frage: "{ex["q"]}"')
                parts.append(f"→ {ex['tool']}({ex.get('args', '')})")
                parts.append(f"→ {ex['answer']}")
                parts.append("")

    return "\n".join(parts)
