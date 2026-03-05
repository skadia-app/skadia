-- 000079_timescaledb_compression.down.sql
-- Entfernt die Compression Policy und deaktiviert Komprimierung.
-- Bereits komprimierte Chunks werden NICHT automatisch dekomprimiert.
-- Falls noetig: SELECT decompress_chunk(c) FROM show_chunks('activity_streams') c;

SELECT remove_compression_policy('activity_streams', if_exists => true);

ALTER TABLE activity_streams SET (timescaledb.compress = false);
