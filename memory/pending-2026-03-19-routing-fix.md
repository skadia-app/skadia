## CHANGELOG.md
**Suche nach:** `## [Unreleased]`
**Delta:**
- fix(routing): Tile-Router Regression gelöst — zwei Root Causes: TurnCost-Explosion bei kurzen Edges nach TNE (skaliert mit min(1.0, distM/200m)), Cost-Cutting Over-Tightening (remaining × Pass0Coefficient als Safety-Margin). Ljubljana 2km Route funktioniert.
- docs: Phase 2 + Stufe 3a Design Spec — CoverageReader, GraphReader Interface, Unified A*, Douglas-Peucker, FormatVersion 4 (~1700 Zeilen, 2 Review-Runden)

## docs/internal/router/2026-03-19-next-session.md — Problem 10 GELÖST
**Suche nach:** TILE-ROUTER REGRESSION
**Delta:**
Problem 10 (Tile-Router Regression) ist GELÖST (Commit cabc4d0e). Zwei Root Causes: TurnCost-Explosion bei kurzen Edges (2-50m nach TNE) + Cost-Cutting Over-Tightening in pass1 trackIndex. Slowenien-Routing funktioniert. DACH-Tiles müssen noch rebuilt werden (199× v2).

## MEMORY.md
**Suche nach:** Tile-Router Regression
**Delta:**
- `routing-tile-router-regression.md` Beschreibung aktualisieren: "GELÖST (2026-03-19)" statt "KRITISCH: ... Muss debuggt werden"
- Project Status: Tile-Router Regression GELÖST ✅ (2026-03-19)
