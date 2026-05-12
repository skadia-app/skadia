package agent

import (
	"fmt"
	"strings"
	"time"
)

const systemPromptCompact = `Du bist ein Sport-Datenassistent. Heute ist {today}.

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
E-Bike → sport_type='cycling', is_electric=true | Indoor → is_indoor=true
Virtuell → is_virtual=true | Pendeln → is_commute=true | Bouldern → sport_type='rock_climbing', sub_type='bouldering'
Trail → sport_type='running', sub_type='trail'
Ski allgemein → sport_type='skiing' | Alpin-Ski → sport_type='alpine_skiing' | Langlauf → sport_type='cross_country_skiing'
Krafttraining → sport_type='strength_training' | Bergsteigen → sport_type='mountaineering'

## Multi-Schritt
Wenn eine Frage mehrere Informationsquellen braucht, nutze mehrere Tools nacheinander.
Beantworte ERST wenn ALLE nötigen Daten abgerufen sind.`

const systemPromptStandardSuffix = `

## Weitere Sportarten (DB-Werte)
Indoor-Rad → sport_type='cycling', sub_type='indoor_cycling', is_indoor=true
Freiwasser → sport_type='swimming', sub_type='open_water'
Rudern → sport_type='rowing' | SUP → sport_type='stand_up_paddleboarding'
Badminton → sport_type='racket', sub_type='badminton'

## Einheiten-Umrechnung
- Pace (min/km) = (1000 / speed_ms) / 60
- Speed (km/h) = speed_ms × 3.6
- Distanz (km) = distance_m / 1000
- Dauer (h) = duration_s / 3600`

const systemPromptRichSuffix = `

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
- VO2max: maximale Sauerstoffaufnahme (ml/kg/min)`

const toolGuide = `## Werkzeug-Auswahl
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
- Subjektive Routing-Wünsche ("Gravel", "wenig Verkehr", "schoen/schön", "Trail", "Pendeln") → zuerst list_routing_profiles, dann passendes Profil wählen, dann plan_route als reine Vorschau ohne Speichern
- "Tags / Welche Aktivitäten haben Tag X" → run_sql mit JOIN tags + activity_tags
- Komplexe Vergleiche (MTB vs Gravel, Jahresvergleich mit SUB_TYPE) → run_sql`

func AssembleSystemPrompt(profile ModelProfile, skills []*Skill, today time.Time) string {
	if today.IsZero() {
		today = time.Now()
	}
	base := promptForMode(profile.SystemPromptMode)
	parts := []string{strings.ReplaceAll(base, "{today}", today.Format("02.01.2006"))}

	for _, skill := range skills {
		if skill == nil {
			continue
		}
		parts = append(parts, "", skill.ToContextString())
	}
	if profile.UseRouter {
		parts = append(parts, "", toolGuide)
	}
	if profile.FewShotCount > 0 {
		examples := collectFewShots(skills, profile.FewShotCount)
		if len(examples) > 0 {
			parts = append(parts, "", formatPromptFewShots(examples))
		}
	}
	return strings.Join(parts, "\n")
}

func promptForMode(mode string) string {
	standard := systemPromptCompact + systemPromptStandardSuffix
	switch mode {
	case "standard":
		return standard
	case "rich":
		return standard + systemPromptRichSuffix
	default:
		return systemPromptCompact
	}
}

func collectFewShots(skills []*Skill, maxCount int) []map[string]any {
	var out []map[string]any
	for _, skill := range skills {
		if skill == nil {
			continue
		}
		limit := 2
		if len(skill.FewShotExamples) < limit {
			limit = len(skill.FewShotExamples)
		}
		out = append(out, skill.FewShotExamples[:limit]...)
	}
	if len(out) > maxCount {
		out = out[:maxCount]
	}
	return out
}

func formatPromptFewShots(examples []map[string]any) string {
	lines := []string{"## Beispiele"}
	for _, ex := range examples {
		lines = append(lines, fmt.Sprintf("Frage: %q", stringValue(ex["q"])))
		if steps, ok := asStepSlice(ex["steps"]); ok {
			for idx, step := range steps {
				lines = append(lines, fmt.Sprintf("→ Schritt %d: %s(%s)", idx+1, stringValue(step["tool"]), pythonRepr(step["args"])))
			}
		} else {
			lines = append(lines, formatToolCall(ex["tool"], ex["args"]))
		}
		if answer, ok := ex["answer"]; ok && stringValue(answer) != "" {
			lines = append(lines, fmt.Sprintf("→ %s", stringValue(answer)))
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
