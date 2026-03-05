## services/api/internal/model/activity.go — Lift-Felder in Activity-Struct
**Suche nach:** LiftCount
**Delta:**
- `Activity` struct: 6 neue Felder — `LiftCount *int`, `LiftDistanceM *float64`, `LiftAscentM *float64`, `LiftDescentM *float64`, `LiftDurationS *int`, `ExcludeLiftSubtraction bool`
- `ActivityUpdate` struct: neues Feld `ExcludeLiftSubtraction *bool` für PATCH

## services/api/internal/database/database.go — GetActivity + UpdateActivity + Lift-Aggregation
**Suche nach:** GetLiftCandidates
**Delta:**
- `GetActivity()`: SELECT und Scan um 6 Lift-Spalten erweitert (`lift_count`, `lift_distance_m`, `lift_ascent_m`, `lift_descent_m`, `lift_duration_s`, `exclude_lift_subtraction`)
- `UpdateActivity()`: neues Handling für `ExcludeLiftSubtraction`
- `ConfirmLiftCandidate()`: gibt jetzt `activity_id` via RETURNING zurück + ruft `recomputeLiftStats` auf
- `RejectLiftCandidate()`: neue DB-Funktion (DELETE + recomputeLiftStats)
- `recomputeLiftStats()`: neue private Funktion — aggregiert `lift_count`, `lift_duration_s`, `lift_ascent_m` aus bestätigten Kandidaten in `activities`-Tabelle

## services/api/internal/handler/lift_candidates.go — RejectLiftCandidate-Handler
**Suche nach:** RejectLiftCandidate
**Delta:** Nutzt nun `h.DB.RejectLiftCandidate()` statt direktem Pool.Exec — Fehlerbehandlung + Stats-Recompute

## CHANGELOG.md
- feat(api): lift-Felder in Activity-Model + GetActivity-Query (#map-features)
- fix(api): ConfirmLiftCandidate/RejectLiftCandidate aktualisieren jetzt Activity-Aggregatspalten
- feat(api): PATCH activities/:id unterstützt `exclude_lift_subtraction`
