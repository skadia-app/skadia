UPDATE activity_tracks
SET bbox = ST_Envelope(ST_Expand(track_full, 0.0000001))
WHERE track_full IS NOT NULL
  AND (
    bbox IS NULL
    OR ST_Area(bbox) = 0
  );
