-- 000081_backfill_elevation_loss.up.sql
-- Berechnet elevation_loss_m (und max/min_elevation_m) nach, sofern
-- activity_streams Höhendaten enthalten und der Wert noch NULL ist.
--
-- Hintergrund: Strava-importierte Aktivitäten hatten keine elevation_loss_m,
-- weil die Strava-API nur elevation_gain liefert und EnrichActivity den Wert
-- nicht aus der FIT/GPX-Datei übernommen hat (behoben in enrich.go).
-- Dieser Backfill berechnet die fehlenden Werte aus den gespeicherten GPS-Streams.
--
-- Methode: einfache kumulative Differenz (kein Smoothing).
-- Für neu-verarbeitete Aktivitäten liefert ascents.py via Savitzky-Golay
-- genauere Werte. Dieser Backfill ist nur ein sinnvoller Initialwert.

UPDATE activities
SET elevation_loss_m = subq.loss
FROM (
    SELECT
        activity_id,
        COALESCE(SUM(CASE WHEN diff < 0 THEN -diff ELSE 0 END), 0) AS loss
    FROM (
        SELECT
            activity_id,
            altitude_m - LAG(altitude_m) OVER (PARTITION BY activity_id ORDER BY time) AS diff
        FROM activity_streams
        WHERE altitude_m IS NOT NULL
    ) diffs
    WHERE diff IS NOT NULL
    GROUP BY activity_id
) subq
WHERE activities.id = subq.activity_id
  AND activities.elevation_loss_m IS NULL
  AND subq.loss > 0;

-- max_elevation_m nachfüllen
UPDATE activities
SET max_elevation_m = subq.max_alt
FROM (
    SELECT activity_id, MAX(altitude_m) AS max_alt
    FROM activity_streams
    WHERE altitude_m IS NOT NULL
    GROUP BY activity_id
) subq
WHERE activities.id = subq.activity_id
  AND activities.max_elevation_m IS NULL;

-- min_elevation_m nachfüllen
UPDATE activities
SET min_elevation_m = subq.min_alt
FROM (
    SELECT activity_id, MIN(altitude_m) AS min_alt
    FROM activity_streams
    WHERE altitude_m IS NOT NULL
    GROUP BY activity_id
) subq
WHERE activities.id = subq.activity_id
  AND activities.min_elevation_m IS NULL;
