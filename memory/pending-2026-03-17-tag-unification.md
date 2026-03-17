## docs/09_FEATURE_CATALOG.md — Tags
**Suche nach:** Tag-System
**Delta:** Drei Auto-Tag-Mechanismen auf Rules Engine konsolidiert. System-Rules (E-Bike read-only, Rundtour/Frost/Nacht editierbar) in Workshop/Rules. EvaluateAutoTags entfernt. Rundtour-Trigger entfernt. Tags-migrieren-Button entfernt. Kategorien: System/Wetter/Standort/Eigene. Adjektive kleingeschrieben (sonnig, bewölkt). ApplyRulesToActivity gilt für ALLE Aktivitäten (nicht nur GPS).

## docs/07_DATABASE_SCHEMA.md — automation_rules
**Suche nach:** automation_rules
**Delta:** Neue Spalten is_system BOOLEAN, system_type TEXT (CHECK: ebike_sync/round_trip/frost/nacht). Partial unique index auf (user_id, system_type) WHERE is_system. Vier System-Rules pro User. tags.has_automation BOOLEAN materialisiert (refresh bei Rule-CRUD).

## docs/02_CORE_BACKEND.md — Rules Engine
**Suche nach:** Automation
**Delta:** Neue Conditions: sun_elevation, start_end_distance_m. Neue Action: set_field (Whitelist: is_round_trip, is_indoor). automation.ApplyRulesToActivity() shared helper mit Retraction-Semantik für System-Rules (no-match → remove tag + reset field). ApplyRuleChange add_tag/remove_tag schreiben korrekt in activity_tags Junction + TEXT[] Cache + activity_count. Import-Pipeline: 3 Call-Sites nach RunStage1().

## CHANGELOG.md
**Delta:**
- feat: Tag-System-Vereinheitlichung — drei Auto-Tag-Mechanismen auf Rules Engine konsolidiert
- feat: System-Rules (E-Bike, Rundtour, Frost, Nacht) in Workshop/Rules sichtbar + eingeschränkt editierbar
- feat: Rundtour-Schwellwert konfigurierbar (default 500m, per Rule editierbar)
- feat: Nacht-Tag per Sonnenstand-Gradzahl konfigurierbar (default -6° bürgerliche Dämmerung)
- feat: Tag-Kategorien: System, Wetter, Standort, Eigene (auto-set bei lat/lng)
- feat: has_automation Feld auf Tags (materialisiert, refresh bei Rule-CRUD)
- fix: ApplyRuleChange schreibt jetzt korrekt in activity_tags Junction-Table
- fix: Automation Rules gelten jetzt für ALLE Aktivitäten (nicht nur GPS)
- fix: Adjektiv-Tags kleingeschrieben (sonnig, bewölkt)
- refactor: EvaluateAutoTags, Rundtour-Trigger, Tags-migrieren-Button entfernt (~528 Zeilen Dead Code)
