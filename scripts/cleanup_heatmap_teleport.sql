-- cleanup_heatmap_teleport.sql
-- Bereinigt heatmap_lines und heatmap_points für Aktivitäten, die
-- "Teleportationslinien" enthalten (Segmente > 2000 m zwischen zwei
-- aufeinanderfolgenden GPS-Punkten).
--
-- Ausführen:
--   distrobox-host-exec podman exec -i skadia-postgres-1 \
--     psql -U skadia -d skadia -f /pfad/zu/cleanup_heatmap_teleport.sql
--
-- Das Skript ist idempotent: es kann mehrfach ausgeführt werden.

BEGIN;

-- ────────────────────────────────────────────────────────────────────────────
-- Schritt 0: Vorschau – betroffene Einträge anzeigen
-- ────────────────────────────────────────────────────────────────────────────
\echo '=== Betroffene heatmap_lines ==='
SELECT hl.id,
       hl.activity_id,
       a.started_at::date AS activity_date,
       ROUND(MAX(ST_Length(seg.geom::geography))::numeric, 1) AS max_segment_m
FROM heatmap_lines hl
JOIN activities a ON a.id = hl.activity_id
JOIN LATERAL (SELECT (ST_DumpSegments(hl.geom)).geom) seg(geom) ON true
GROUP BY hl.id, hl.activity_id, a.started_at
HAVING MAX(ST_Length(seg.geom::geography)) > 2000
ORDER BY max_segment_m DESC;

-- ────────────────────────────────────────────────────────────────────────────
-- Schritt 1: Alte heatmap_lines für betroffene Aktivitäten löschen
-- ────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '=== Lösche betroffene heatmap_lines ==='

-- Sammle die activity_ids aller betroffenen Einträge
CREATE TEMP TABLE teleport_activities AS
SELECT DISTINCT hl.activity_id
FROM heatmap_lines hl
WHERE EXISTS (
  SELECT 1
  FROM (SELECT (ST_DumpSegments(hl.geom)).geom AS seg) s
  WHERE ST_Length(seg::geography) > 2000
);

-- Zeige wie viele Aktivitäten betroffen sind
SELECT COUNT(*) AS betroffene_aktivitaeten FROM teleport_activities;

-- Lösche alle heatmap_lines für diese Aktivitäten
-- (eine Aktivität kann mehrere Zeilen haben, wenn sie bereits
--  partiell bereinigt wurde – wir ersetzen sie alle)
DELETE FROM heatmap_lines
WHERE activity_id IN (SELECT activity_id FROM teleport_activities);

\echo 'heatmap_lines gelöscht.'

-- ────────────────────────────────────────────────────────────────────────────
-- Schritt 2: heatmap_lines neu befüllen (mit Teleportationsfilter)
-- ────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '=== Befülle heatmap_lines neu (mit Teleportationsfilter) ==='

WITH pts AS (
  SELECT a.id AS activity_id, a.user_id, a.sport_type, a.sub_type, a.started_at,
         (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).path[1] AS idx,
         (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).geom  AS pt
  FROM activities a
  JOIN activity_tracks t ON t.activity_id = a.id
  WHERE a.id IN (SELECT activity_id FROM teleport_activities)
    AND t.track_full IS NOT NULL
),
with_dist AS (
  SELECT activity_id, user_id, sport_type, sub_type, started_at, idx, pt,
    COALESCE(
      ST_Distance(pt::geography,
        LAG(pt) OVER (PARTITION BY activity_id ORDER BY idx)::geography),
      0
    ) AS prev_dist
  FROM pts
),
grouped AS (
  SELECT activity_id, user_id, sport_type, sub_type, started_at, idx, pt,
    SUM(CASE WHEN prev_dist > 2000 THEN 1 ELSE 0 END)
      OVER (PARTITION BY activity_id ORDER BY idx) AS grp
  FROM with_dist
),
line_groups AS (
  SELECT activity_id, user_id, sport_type, sub_type, started_at, grp,
         ST_MakeLine(pt ORDER BY idx) AS geom
  FROM grouped
  GROUP BY activity_id, user_id, sport_type, sub_type, started_at, grp
  HAVING COUNT(*) >= 2
)
INSERT INTO heatmap_lines (user_id, activity_id, sport_type, sub_type, geom, activity_date)
SELECT user_id, activity_id, sport_type, sub_type, geom, started_at::date
FROM line_groups
WHERE geom IS NOT NULL;

\echo 'heatmap_lines neu befüllt.'

-- ────────────────────────────────────────────────────────────────────────────
-- Schritt 3: Alte heatmap_points für betroffene Aktivitäten löschen
-- ────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '=== Lösche betroffene heatmap_points ==='

DELETE FROM heatmap_points
WHERE activity_id IN (SELECT activity_id FROM teleport_activities);

\echo 'heatmap_points gelöscht.'

-- ────────────────────────────────────────────────────────────────────────────
-- Schritt 4: heatmap_points neu befüllen (mit Teleportationsfilter)
-- Standard-Sampling-Intervall: 20 m (kann angepasst werden)
-- ────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '=== Befülle heatmap_points neu (Intervall: 20 m) ==='

WITH pts AS (
  SELECT a.id AS activity_id, a.user_id, a.sport_type, a.sub_type, a.started_at,
         (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).path[1] AS idx,
         (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).geom  AS pt
  FROM activities a
  JOIN activity_tracks t ON t.activity_id = a.id
  WHERE a.id IN (SELECT activity_id FROM teleport_activities)
    AND t.track_full IS NOT NULL
),
with_dist AS (
  SELECT activity_id, user_id, sport_type, sub_type, started_at, idx, pt,
    COALESCE(
      ST_Distance(pt::geography,
        LAG(pt) OVER (PARTITION BY activity_id ORDER BY idx)::geography),
      0
    ) AS prev_dist
  FROM pts
),
grouped AS (
  SELECT activity_id, user_id, sport_type, sub_type, started_at, idx, pt,
    SUM(CASE WHEN prev_dist > 2000 THEN 1 ELSE 0 END)
      OVER (PARTITION BY activity_id ORDER BY idx) AS grp
  FROM with_dist
),
line_groups AS (
  SELECT activity_id, user_id, sport_type, sub_type, started_at, grp,
         ST_MakeLine(pt ORDER BY idx) AS geom
  FROM grouped
  GROUP BY activity_id, user_id, sport_type, sub_type, started_at, grp
  HAVING COUNT(*) >= 2
)
INSERT INTO heatmap_points (user_id, activity_id, sport_type, sub_type, activity_date, geom)
SELECT user_id, activity_id, sport_type, sub_type, started_at::date,
       (ST_DumpPoints(ST_Segmentize(geom::geography, 20)::geometry)).geom
FROM line_groups
WHERE geom IS NOT NULL;

\echo 'heatmap_points neu befüllt.'

-- ────────────────────────────────────────────────────────────────────────────
-- Schritt 5: Ergebnis prüfen – darf keine Zeilen mehr anzeigen
-- ────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '=== Verbleibende Teleportationslinien (soll leer sein) ==='
SELECT hl.id, hl.activity_id,
       ROUND(MAX(ST_Length(seg.geom::geography))::numeric, 1) AS max_segment_m
FROM heatmap_lines hl
JOIN LATERAL (SELECT (ST_DumpSegments(hl.geom)).geom) seg(geom) ON true
WHERE hl.activity_id IN (SELECT activity_id FROM teleport_activities)
GROUP BY hl.id, hl.activity_id
HAVING MAX(ST_Length(seg.geom::geography)) > 2000;

DROP TABLE teleport_activities;

COMMIT;

\echo ''
\echo 'Bereinigung abgeschlossen.'
