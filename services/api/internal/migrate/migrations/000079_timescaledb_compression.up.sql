-- 000079_timescaledb_compression.up.sql
-- Aktiviert TimescaleDB Columnar Compression auf activity_streams.
--
-- Segmentierung nach activity_id: alle Datenpunkte einer Aktivitaet
-- landen im selben komprimierten Block.
-- Sortierung nach time DESC: neueste Punkte zuerst (gut fuer
-- "letzte N Punkte einer Aktivitaet"-Queries).
--
-- Compression Policy: Chunks aelter als 7 Tage werden automatisch
-- komprimiert. Neue Chunks bleiben unkomprimiert (wichtig fuer
-- INSERT-Performance beim FIT/GPX-Import).
--
-- Erwartete Einsparung: 1,8 GB -> ~100-180 MB (10-18x Faktor).

ALTER TABLE activity_streams SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'activity_id',
    timescaledb.compress_orderby = 'time DESC'
);

SELECT add_compression_policy('activity_streams', INTERVAL '7 days');
