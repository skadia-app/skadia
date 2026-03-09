-- OSM-Importe können mehrere GB groß sein (z.B. 8 Regionen = ~15 GB total).
-- INT (int4, max 2,1 GB) reicht nicht aus — auf BIGINT upgraden.
ALTER TABLE background_jobs
    ALTER COLUMN progress_current TYPE BIGINT,
    ALTER COLUMN progress_total   TYPE BIGINT;
