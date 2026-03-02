# Sonnenstandserkennung (Sunrise/Sunset)

**Feature-ID:** enrichment-sunrise-sunset
**Kategorie:** Stage 1 Enrichment
**Status:** Implementiert (2026-03-02)
**Migration:** 000016_sunrise_sunset
**Betroffene Schichten:** Datenbank, Go Backend, React Frontend

---

## 1. Uebersicht

Skadia berechnet fuer jede Aktivitaet mit GPS-Daten die exakten Zeiten fuer Sonnenaufgang und Sonnenuntergang. Im Gegensatz zu einfachen Implementierungen, die nur den Startpunkt verwenden, nutzt Skadia den **GPS-Track**, um die Position zum Zeitpunkt des Sonnenereignisses zu bestimmen. Bei langen Aktivitaeten (>100 km) kann die Sonnenposition entlang des Tracks um mehrere Minuten variieren.

### Anwendungsfaelle

- **Aktivitaets-Detail:** Anzeige der Sonnenauf-/-untergangszeiten mit Icons
- **Hoehenprofil:** Vertikale Marker im ECharts-Diagramm zeigen, wo auf der Strecke das Ereignis stattfand
- **Karteninteraktion:** Hover ueber den Marker zeigt den GPS-Standort auf der Karte
- **Filterung/Suche:** (zukuenftig) Aktivitaeten nach Tageslichtstunden filtern

---

## 2. Algorithmus

### 2.1 Berechnungsablauf

Die Funktion `CalculateSunTimes()` in `services/api/internal/enrichment/stage1.go` implementiert folgenden Algorithmus:

```
Eingabe: startLat, startLng, endLat, endLng, startedAt, endedAt, streams[]

1. Vorlaeufe Berechnung am Startpunkt
   times = suncalc.GetTimes(startedAt, startLat, startLng)
   prelimSunrise = times[Sunrise]
   prelimSunset  = times[Sunset]

2. Sonnenaufgang bestimmen
   WENN sunrise INNERHALB [startedAt, endedAt]:
     → GPS-Punkt zum Zeitpunkt des Sonnenaufgangs finden
     → suncalc.GetTimes() mit diesen Koordinaten erneut aufrufen
     → Verfeinerte Sunrise-Zeit zurueckgeben
   WENN sunrise VOR startedAt:
     → Startpunkt-Wert verwenden (Sonne war bereits aufgegangen)
   WENN sunrise NACH endedAt:
     → nil (irrelevant, z.B. Nachtaktivitaet)

3. Sonnenuntergang bestimmen
   WENN sunset INNERHALB [startedAt, endedAt]:
     → GPS-Punkt zum Zeitpunkt des Sonnenuntergangs finden
     → suncalc.GetTimes() mit diesen Koordinaten erneut aufrufen
     → Verfeinerte Sunset-Zeit zurueckgeben
   WENN sunset NACH endedAt:
     → Endpunkt-Koordinaten verwenden (Sonne geht erst spaeter unter)
   WENN sunset VOR startedAt:
     → nil (irrelevant)
```

### 2.2 GPS-Punkt-Suche (findCoordsAtTime)

Die Hilfsfunktion `findCoordsAtTime()` findet den GPS-Punkt, der zeitlich dem gesuchten Moment am naechsten liegt:

1. **Binary Search** auf dem chronologisch sortierten Stream-Array → O(log n)
2. Vergleich von `streams[lo]` und `streams[lo-1]` fuer den absolut naechsten Punkt
3. **Outward Walk** vom Kandidaten, falls der naechste Punkt keine GPS-Daten hat (z.B. Indoor-Abschnitt mit Tunnel)
4. **Fallback** auf die uebergebenen Default-Koordinaten, wenn kein Stream GPS-Daten hat

### 2.3 Bibliothek

**sixdouglas/suncalc** (Go-Port von mourner/suncalc.js):
- Berechnet Sonnenposition basierend auf astronomischen Formeln
- Liefert alle relevanten Zeiten: Night, NauticalDawn, Dawn, Sunrise, GoldenHourEnd, GoldenHour, Sunset, Dusk, NauticalDusk, Night
- Genauigkeit: ~1 Minute (ausreichend fuer Aktivitaetstracking)

### 2.4 Sonderfaelle

| Szenario | Ergebnis |
|---|---|
| Aktivitaet komplett bei Tag | sunrise_at = Startpunkt-Wert, sunset_at = Endpunkt-Wert |
| Aktivitaet komplett bei Nacht | sunrise_at = nil, sunset_at = nil |
| Sunrise waehrend Aktivitaet | sunrise_at = verfeinert am Track-Punkt |
| Sunset waehrend Aktivitaet | sunset_at = verfeinert am Track-Punkt |
| Polarregion (kein Sunrise/Sunset) | sunrise_at = nil, sunset_at = nil (suncalc gibt Zero-Time zurueck) |
| Keine GPS-Daten | Keine Berechnung (Vorbedingung: StartLat != 0) |
| Indoor-Aktivitaet mit GPS | Wird berechnet (is_indoor filtert nicht) |

---

## 3. Datenmodell

### 3.1 Migration (000016)

```sql
-- up
ALTER TABLE activities ADD COLUMN IF NOT EXISTS sunrise_at TIMESTAMPTZ;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS sunset_at TIMESTAMPTZ;

-- down
ALTER TABLE activities DROP COLUMN IF EXISTS sunrise_at;
ALTER TABLE activities DROP COLUMN IF EXISTS sunset_at;
```

### 3.2 Spalten

| Spalte | Typ | Nullable | Beschreibung |
|---|---|---|---|
| `sunrise_at` | TIMESTAMPTZ | ja | Sonnenaufgangszeit fuer diese Aktivitaet (am relevanten GPS-Punkt) |
| `sunset_at` | TIMESTAMPTZ | ja | Sonnenuntergangszeit fuer diese Aktivitaet (am relevanten GPS-Punkt) |

Beide Spalten sind `NULL` wenn:
- Keine GPS-Daten vorhanden
- Das Ereignis fuer die Aktivitaet irrelevant ist (z.B. Nachtaktivitaet → kein Sunset im Zeitfenster)
- Polarregion ohne Sonnenauf-/-untergang am Aktivitaetstag

### 3.3 Go Model

```go
// In model.Activity (nach TimeOfDay, vor OriginalFilename)
SunriseAt  *time.Time `json:"sunrise_at,omitempty"`
SunsetAt   *time.Time `json:"sunset_at,omitempty"`
```

### 3.4 TypeScript Interface

```typescript
// In Activity (nach time_of_day, vor gear_id)
sunrise_at?: string | null
sunset_at?: string | null
```

---

## 4. API

### 4.1 Berechnung bei Upload/Recompute

Die Berechnung erfolgt automatisch als Teil von `RunStage1()` in `services/api/internal/enrichment/stage1.go`. Bei Upload und Recompute sind die vollstaendigen Stream-Daten im Speicher verfuegbar, sodass `findCoordsAtTime()` direkt auf dem Stream-Array arbeitet.

Vorbedingung:
```go
if data.StartLat != 0 && !data.StartedAt.IsZero() && !data.EndedAt.IsZero() {
    sun := CalculateSunTimes(...)
}
```

### 4.2 Backfill-Endpoint

```
POST /api/v1/admin/backfill-sunrise-sunset
Authorization: Bearer <token>
```

**Beschreibung:** Berechnet Sunrise/Sunset nachtraeglich fuer alle Aktivitaeten des authentifizierten Nutzers.

**Ablauf:**
1. Alle Aktivitaeten via `ListActivitiesForBackfill()` laden (leichtgewichtiges Struct)
2. Pro Aktivitaet: vorlaeufige Zeiten am Startpunkt berechnen
3. Fuer Ereignisse innerhalb der Aktivitaetszeit: `GetStreamPointNearTime()` — gezielte DB-Query pro Ereignis (max. 2 Queries/Aktivitaet statt voller Stream-Ladung)
4. Verfeinerte Zeiten via `UpdateEnrichmentFields()` speichern

**DB-Query fuer naechtsten Stream-Punkt:**
```sql
SELECT time, lat, lng FROM activity_streams
WHERE activity_id = $1 AND lat IS NOT NULL
ORDER BY ABS(EXTRACT(EPOCH FROM (time - $2::timestamptz)))
LIMIT 1
```

**Response:**
```json
{
  "total": 791,
  "updated": 743
}
```

### 4.3 Activity-Detail-Response

Die Felder `sunrise_at` und `sunset_at` sind Teil der `GET /api/v1/activities/{id}`-Response:

```json
{
  "id": "550e8400-...",
  "started_at": "2025-08-14T05:30:00+02:00",
  "ended_at": "2025-08-14T12:15:00+02:00",
  "time_of_day": ["buergerliche-daemmerung", "goldene-stunde", "tag"],
  "sunrise_at": "2025-08-14T06:12:00+02:00",
  "sunset_at": null,
  ...
}
```

---

## 5. Frontend-Integration

### 5.1 SunInfo-Komponente

Kompakte Inline-Anzeige in der Aktivitaets-Detailseite (`ActivityDetailPage.tsx`), positioniert zwischen EnrichmentTags und WeatherBadge:

```
☀ 06:12    ☾ 20:47
```

- **Sunrise:** Amber-farbenes Sonnensymbol (&#9728;) + Uhrzeit HH:MM (lokale Zeitzone)
- **Sunset:** Orange-farbenes Mondsymbol (&#9790;) + Uhrzeit HH:MM
- Nur sichtbar wenn mindestens ein Wert vorhanden
- Styling: `text-sm text-neutral-600 dark:text-neutral-400`

### 5.2 Vertikale Marker im Hoehenprofil

Die Funktion `computeSunMarkLines()` in `ActivityCharts.tsx` erzeugt ECharts-MarkLine-Daten fuer das Altitude-Panel:

**Berechnung:**
1. Pruefe ob `sunrise_at`/`sunset_at` innerhalb des Stream-Zeitraums liegt
2. Finde den naechsten Stream-Index via Zeitvergleich (linearer Scan)
3. Bestimme den x-Wert (`distanceKm` oder `elapsedMin` je nach xMode)
4. Erzeuge MarkLine-Eintrag mit Label, Farbe und gestricheltem Stil

**ECharts-Konfiguration pro Marker:**
```javascript
{
  xAxis: 42.3,          // km oder Minuten
  name: "☀ 06:12",
  label: {
    show: true,
    position: 'insideEndTop',
    backgroundColor: 'rgba(255,255,255,0.9)',
    padding: [2, 5],
    borderRadius: 3,
    color: '#f59e0b',    // amber fuer Sunrise
    fontSize: 11,
  },
  lineStyle: {
    type: 'dashed',
    color: '#f59e0b',
    width: 1.5,
  },
}
```

**Farbschema:**
| Ereignis | Farbe | CSS-Klasse | SVG-Symbol |
|---|---|---|---|
| Sunrise | `#f59e0b` (amber-500) | `text-amber-500` | Halbe Sonne aufgehend mit Strahlen |
| Sunset | `#e67e22` (warm orange) | `text-orange-600` | Halbe Sonne untergehend mit Strahlen |

**Darstellung:** Jeder Marker besteht aus einer gestrichelten vertikalen Linie (`markLine`) plus einer SVG-Grafik (`markPoint`, `symbolSize: 24`) am oberen Rand. Die SVGs sind als `image://data:image/svg+xml,...` Data-URIs eingebettet.

### 5.3 Karteninteraktion

Die Karteninteraktion funktioniert automatisch ueber den bestehenden Mechanismus:
1. Hover ueber den Sunrise/Sunset-Marker im Hoehenprofil
2. ECharts `showTip`-Event feuert mit `dataIndex` des naechsten Stream-Punkts
3. `onHover(dataIndex)` propagiert zum Parent
4. Der Hover-Marker auf der MapLibre-Karte zeigt `streams[dataIndex].lat/lng`
5. Die Karte scrollt **nicht** automatisch zum Punkt (nur Marker-Anzeige)

---

## 6. Dateien

| Datei | Aenderung |
|---|---|
| `migrations/000016_sunrise_sunset.up.sql` | Neue Spalten |
| `migrations/000016_sunrise_sunset.down.sql` | Rollback |
| `services/api/internal/enrichment/stage1.go` | `CalculateSunTimes()`, `findCoordsAtTime()`, `SunResult`, Integration in `RunStage1` |
| `services/api/internal/model/activity.go` | `SunriseAt`, `SunsetAt` Felder im Activity struct |
| `services/api/internal/database/database.go` | `GetActivity` SELECT/Scan erweitert, `GetStreamPointNearTime()` |
| `services/api/internal/handler/enrichment.go` | `BackfillSunriseSunset` Handler |
| `services/api/cmd/api/main.go` | Route `POST /admin/backfill-sunrise-sunset` |
| `frontend/src/api/types.ts` | `sunrise_at`, `sunset_at` im Activity interface |
| `frontend/src/pages/ActivityDetailPage.tsx` | `SunInfo` Komponente, Props an `ActivityCharts` |
| `frontend/src/components/ActivityCharts.tsx` | `SunMarkLine` interface, `computeSunMarkLines()`, Altitude-MarkLines |

---

## 7. Abhaengigkeiten

| Abhaengigkeit | Version | Zweck |
|---|---|---|
| `github.com/sixdouglas/suncalc` | Go | Astronomische Sonnenpositionsberechnung |
| ECharts `MarkLineComponent` | 6.x | Vertikale Linien im Chart |

Die suncalc-Bibliothek ist bereits als Abhaengigkeit vorhanden (wird auch von `TimeOfDayPeriods()` genutzt).

---

## 8. Verifikation

1. **Migration anwenden:** `000016_sunrise_sunset.up.sql`
2. **API kompilieren:** `cd services/api && go build ./...`
3. **TypeScript pruefen:** `cd frontend && npx tsc --noEmit`
4. **Backfill ausfuehren:** `POST /api/v1/admin/backfill-sunrise-sunset`
5. **Activity pruefen:** `GET /api/v1/activities/{id}` → `sunrise_at`/`sunset_at` in JSON vorhanden
6. **Frontend:** Aktivitaets-Detailseite → SunInfo mit Uhrzeiten sichtbar
7. **Hoehenprofil:** Gestrichelte Linien bei Sunrise/Sunset
8. **Hover:** Ueber Marker fahren → Karte zeigt GPS-Position

---

## 9. Verwandte Features

- **TimeOfDayPeriods** (`time_of_day` TEXT[]): Bestimmt welche Tageszeit-Perioden eine Aktivitaet durchlaeuft. Nutzt dieselbe suncalc-Bibliothek, berechnet aber nur am Startpunkt (ausreichend fuer Perioden-Klassifikation).
- **Wetter-Enrichment** (`weather_*`): Nutzt ebenfalls GPS-Koordinaten und Zeitstempel, aber ueber externe API (OpenMeteo).

---

*Querverweise: [02_CORE_BACKEND.md](../docs/02_CORE_BACKEND.md) (Stage 1 Enrichment) · [03_FRONTEND.md](../docs/03_FRONTEND.md) (ECharts) · [07_DATABASE_SCHEMA.md](../docs/07_DATABASE_SCHEMA.md) (Activities-Tabelle) · [08_API_REFERENCE.md](../docs/08_API_REFERENCE.md) (Endpoints)*
