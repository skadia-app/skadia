-- segment_efforts is a TimescaleDB hypertable: this index is automatically
-- propagated per chunk (no single global index, one index per chunk).
CREATE INDEX IF NOT EXISTS idx_segment_efforts_activity_id
ON segment_efforts (activity_id);
