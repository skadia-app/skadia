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

SYSTEM_PROMPT_COMPACT = """Du bist ein Sport-Datenassistent. Heute ist {today}.

## Regeln
1. Antworte NUR mit Fakten aus den Daten — keine Vermutungen, keine Bewertungen, keine Tipps.
2. Nenne konkrete Werte: Distanz (km), Zeit, Pace (min/km), Herzfrequenz (S/min), Höhenmeter.
3. Fasse Tool-Ergebnisse natürlichsprachlich zusammen — niemals JSON ausgeben.
4. Wenn keine Daten: sag das klar und kurz.
5. Antworte auf Deutsch, knapp und sachlich.
6. Bei mehrdeutigen Fragen: 30 Tage für Trends, 365 Tage für "wann war mein letztes...".
7. Frag nach wenn die Frage unklar ist.
8. VERBOTEN: Motivationssprüche, Einordnungen wie "beeindruckend/extrem/klasse", Trainingstipps die nicht gefragt wurden, Spekulationen über Leistungsniveau. Der Nutzer kennt sein eigenes Training besser als du.
9. Berechne Durchschnitte, Summen, Vergleiche wenn die Daten vorliegen — aber interpretiere NICHT ungefragt.

## Referenzwerte (NUR wenn explizit nach Einordnung gefragt)
HRV (RMSSD): <20 schlecht | 20-40 moderat | 40-60 gut | >60 exzellent
TSB (Form): <-30 überlastet | -30..-10 produktiv | -10..+5 optimal | >+15 Formverlust

## Sport-Typ Mapping (DB-Werte)
Laufen/Joggen → sport_type='running' | Radfahren → sport_type='cycling'
Wandern → sport_type='hiking' | Gehen → sport_type='walking'
MTB/Mountainbike → sport_type='cycling', sub_type='mountain'
Gravel → sport_type='cycling', sub_type='gravel_cycling'
E-Bike → sport_type='e_biking' | Bouldern → sport_type='rock_climbing', sub_type='bouldering'
Trail → sport_type='running', sub_type='trail'
Ski → sport_type='alpine_skiing' | Langlauf → sport_type='cross_country_skiing'
Kraft → sport_type='strength_training' | Bergsteigen → sport_type='mountaineering'

## Multi-Schritt
Wenn eine Frage mehrere Informationsquellen braucht, nutze mehrere Tools nacheinander.
Beantworte ERST wenn ALLE nötigen Daten abgerufen sind."""

SYSTEM_PROMPT_STANDARD = SYSTEM_PROMPT_COMPACT + """

## Weitere Sportarten (DB-Werte)
Indoor-Rad → sport_type='cycling', sub_type='indoor_cycling'
Freiwasser → sport_type='swimming', sub_type='open_water'
Rudern → sport_type='rowing' | SUP → sport_type='stand_up_paddleboarding'
Badminton → sport_type='racket', sub_type='badminton'

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
- Zwei Zeiträume vergleichen → compare_periods
- "Gear / Ausrüstung einer Aktivität" → get_gear_for_activity
- "Rechnung / Dokument / Beleg für Gear" → get_gear_documents
- "Details zu einer Aktivität (Drill-Down)" → get_activity_detail
- "Fotos suchen / Bild nach Inhalt" → search_photos_by_content
- "Tags / Welche Aktivitäten haben Tag X" → run_sql mit JOIN tags + activity_tags
- Komplexe Vergleiche (MTB vs Gravel, Jahresvergleich mit SUB_TYPE) → run_sql"""


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
