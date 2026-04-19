-- 000302_fusion_memberships.up.sql
-- Introduces activity_fusion_memberships as the new Source-of-Truth for fusions.
-- Runs in parallel to existing is_fusion_source / fusion_sources during Phase 1 (Dual-Write).

-- 1. Table.
-- user_id is denormalised (same as activities.user_id for both merged and source — they are always same-user).
-- Rationale: AFTER DELETE trigger during CASCADE cannot resolve activities.user_id because the parent
-- row is already gone at that point. Storing user_id directly makes the trigger self-contained and
-- robust against CASCADE ordering.
CREATE TABLE activity_fusion_memberships (
    merged_id  uuid NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    source_id  uuid NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    field_key  text NOT NULL,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (merged_id, source_id, field_key)
);
CREATE INDEX activity_fusion_memberships_source_idx
    ON activity_fusion_memberships (source_id);
CREATE INDEX activity_fusion_memberships_user_idx
    ON activity_fusion_memberships (user_id);

-- 2. New trigger function + trigger (ADDITIVE — alongside trg_fusion_cache_upd from migration 000257).
-- Uses OLD.user_id / NEW.user_id directly (no sub-select on activities — that would return NULL during CASCADE).
CREATE OR REPLACE FUNCTION fn_bump_fusion_cache_seq() RETURNS trigger AS $$
DECLARE
    affected_user UUID;
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected_user := OLD.user_id;
    ELSE
        affected_user := NEW.user_id;
    END IF;
    UPDATE users SET fusion_invalidation_seq = fusion_invalidation_seq + 1
    WHERE id = affected_user;
    IF TG_OP = 'DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_fusion_cache_memberships
    AFTER INSERT OR UPDATE OR DELETE ON activity_fusion_memberships
    FOR EACH ROW EXECUTE FUNCTION fn_bump_fusion_cache_seq();

-- 3. Backfill from existing fusion_sources JSONB (populates user_id from the merged activity).
-- For each activity with fusion_sources != NULL, insert one membership per (field_key, source_id).
-- Today prod DB has 0 rows with fusion_sources — this block is a no-op now,
-- but idempotent and robust for any future state or other installations.
INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
SELECT
    a.id AS merged_id,
    (kv.value #>> '{}')::uuid AS source_id,
    kv.key AS field_key,
    a.user_id AS user_id
FROM activities a,
     LATERAL jsonb_each(a.fusion_sources) AS kv
WHERE a.fusion_sources IS NOT NULL
  AND kv.value::text != '"fuse"'
  AND kv.value #>> '{}' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
ON CONFLICT DO NOTHING;

-- "fuse"-literal expansion: one row per source from any other field_key present in the same JSONB.
INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
SELECT DISTINCT
    fuse_a.id AS merged_id,
    other_src::uuid AS source_id,
    'gps' AS field_key,
    fuse_a.user_id AS user_id
FROM activities fuse_a,
     LATERAL jsonb_each(fuse_a.fusion_sources) AS fuse_kv,
     LATERAL (
         SELECT kv2.value #>> '{}' AS other_src
         FROM jsonb_each(fuse_a.fusion_sources) AS kv2
         WHERE kv2.value::text != '"fuse"'
           AND kv2.value #>> '{}' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
     ) others
WHERE fuse_a.fusion_sources IS NOT NULL
  AND fuse_kv.key = 'gps'
  AND fuse_kv.value::text = '"fuse"'
ON CONFLICT DO NOTHING;

-- Additional fallback: parent_id-based V1 chain (children of merged).
INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
SELECT
    merged.id AS merged_id,
    child.id AS source_id,
    'gps' AS field_key,
    merged.user_id AS user_id
FROM activities merged
JOIN activities child ON child.parent_id = merged.id
WHERE merged.fusion_sources IS NOT NULL
  AND (merged.fusion_sources->>'gps') = 'fuse'
ON CONFLICT DO NOTHING;
