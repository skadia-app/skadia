## docs/03_FRONTEND.md — Bug-Fixes Klettern/Bouldern-Aktivitätsanzeige
**Suche nach:** SplitTable, ClimbingSplits, rock_climbing
**Delta:**
- `SplitTable.tsx`: Neuer optionaler `subType`-Prop; `splitTypeLabel()` berücksichtigt jetzt `subType === 'bouldering'` → zeigt "Bouldern" statt "Klettern"; `ClimbingSplits` zeigt "Boulder-Routen" / "geboulbert" wenn subType=bouldering
- `ActivityDetailPage.tsx`: `subType={activity.sub_type ?? undefined}` wird an `SplitTable` für rock_climbing übergeben
- `ActivityDetailPage.tsx`: `ClimbsSection` nur bei `activity.has_gps` (Bug 2c)
- `ActivityDetailPage.tsx`: `ActivitySegments` nur bei `activity.has_gps` (Bug 2c)

## docs/09_FEATURE_CATALOG.md — Bug 2b (Schwierigkeitsgrad) als bekannte Einschränkung
**Suche nach:** rock_climbing, Klettern, Schwierigkeit
**Delta:**
- Garmin FIT-SDK (`muktihari/fit`) hat kein standardisiertes `climbing_grade`-Feld im Split/Lap-Modell. Das Schwierigkeitsgrad-Feature erfordert proprietäre Garmin-Felder (developer data fields oder undokumentierte Felddefinitionen). Nicht implementiert, kein DB-Feld vorhanden. P4/Backlog.

## CHANGELOG.md
**Delta:**
### Fixed
- Klettern/Bouldern-Aktivitäten: Routen-Label zeigt jetzt korrekt "Bouldern" (statt "Klettern") wenn sub_type=bouldering
- Indoor-Aktivitäten ohne GPS: Segmente-Sektion und Anstiege-Sektion werden ausgeblendet (has_gps-Check)
