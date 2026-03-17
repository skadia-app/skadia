## frontend/src/pages/SettingsPage.tsx — Stage2EnrichmentSection.toggle() partial-write bug
**Suche nach:** Stage2EnrichmentSection
**Delta:** `toggle()` sendete bisher nur `{ [apiField]: !current }` (partial). Jetzt: `{ ...settings, [apiField]: !current }` — volle Settings werden gesendet, damit der Backend-UPSERT alle bool-Felder korrekt setzt.

## services/api/internal/handler/enrichment.go — UpdateEnrichmentSettings partial-write defense
**Suche nach:** UpdateEnrichmentSettings
**Delta:** Handler liest jetzt zuerst bestehende Settings aus DB (`GetEnrichmentSettings`), merged den eingehenden JSON-Patch darüber (raw map merge), und schreibt erst dann. Verhindert, dass fehlende bool-Felder im Patch als Go-Nullwert (false) in die DB geschrieben werden.

## Root Cause beider Bugs
- **Bug 1 ("global aus"):** `Stage2EnrichmentSection.toggle()` in SettingsPage.tsx sendete partial JSON. Go dekodierte fehlende bools als false → alle anderen Stage-2-Module wurden auf false gesetzt. User sah "alle ON" auf /settings/enrichment (alter EnrichmentSettingsPage, hat KEINE Stage-2-Toggles) statt /settings.
- **Bug 2 (Wälder gelb/pending):** forests hatte `status: pending` weil enrichment_status der Aktivität noch pending war UND forests=false (kein no_data → pending). Folgeschaden aus Bug 1.

## CHANGELOG.md
**Suche nach:** Unreleased
**Delta:** fix(settings): Enrichment-Einstellungen — Stage-2-Module wurden bei partiellem Save auf false zurückgesetzt (partial-write bug in Stage2EnrichmentSection.toggle + Backend-Defense in UpdateEnrichmentSettings)
