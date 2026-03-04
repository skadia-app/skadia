## frontend/src/pages/EnrichmentSettingsPage.tsx — Phase 4b Status-Update
**Suche nach:** enable_summit_detection
**Delta:**
- `enable_summit_detection`, `enable_stop_detection`, `enable_commute_detection` von `status: 'planned'` auf `status: 'available'` geändert
- `plannedPhase` Felder für diese drei Einträge entfernt
- Hintergrund: ML-Service-Module `peaks.py`, `stops.py`, `commute.py` sind vollständig implementiert; Backend-Felder in `EnrichmentSettings`-Struct und DB-Queries existieren bereits

## services/api/internal/handler/llm_settings.go — localhost → host.containers.internal
**Suche nach:** TestLLMConnection
**Delta:**
- In `TestLLMConnection`: Vor dem Verbindungstest wird `localhost` und `127.0.0.1` in `api_base` durch `host.containers.internal` ersetzt
- Nur für den Test, nicht für die gespeicherte Config
- Hintergrund: Test läuft server-seitig im API-Container; `localhost` zeigt auf den Container selbst, nicht auf den Host-Rechner

## CHANGELOG.md
- fix(enrichment-settings): Phase-4b-Module als verfügbar markieren (Gipfelerkennung, Stopperkennung, Pendelerkennung)
- fix(llm-settings): Verbindungstest übersetzt localhost/127.0.0.1 zu host.containers.internal für Container-zu-Host-Kommunikation
