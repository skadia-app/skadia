## Idee: GPS-Track-Stitching bei Fusion

**Kontext:** Beim DuplicateReviewPanel soll nicht nur "GPS von A" oder "GPS von B" wählbar sein,
sondern auch "GPS aus beiden Tracks zusammensetzen".

**Szenario:**
- Gerät 1 (länger, weniger genau): startet t=0, endet t=60min
- Gerät 2 (kürzer, präziser): startet t=5, endet t=60min
- Gewünscht: t=0..5 von Gerät 1 + t=5..60 von Gerät 2

**Ansatz (virtuell, kein neues FIT-File):**
- `fusion_sources.gps = "stitch:uuid-A:uuid-B"` (neues spezielles Format)
- Beim Stream-Serving (`applyFusionSources`): wenn gps-Quelle mit "stitch:" beginnt →
  beide Tracks laden, Überlappungspunkt finden (nächster Zeitpunkt), Track A bis Überlappung + Track B ab da
- Overlap-Erkennung: letzter Punkt von Track B vor Beginn von Track A → Schnittstelle

**UI im DuplicateReviewPanel:**
- Dritte Option bei GPS: "Tracks zusammenführen (längsten + genauesten)"
- Zeigt Vorschau: "X km von Gerät 1 (Anfang) + Y km von Gerät 2 (Rest)"

**Scope:** Eigene Brainstorming-Session empfohlen, da Track-Stitching Logik nicht trivial.
Plan-Datei: noch nicht erstellt.
