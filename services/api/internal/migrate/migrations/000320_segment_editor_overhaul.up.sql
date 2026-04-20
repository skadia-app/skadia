-- Segment-Editor-Overhaul: four-step migration, single transaction.
--   1. Allow new segType 'wintersport' in segments.segment_type CHECK constraint.
--   2. Add segments.bidirectional BOOLEAN NOT NULL DEFAULT true.
--   3. Normalize historic German surface labels in segment_type_meta.surface.
--   4. Safety-net: convert any historic German difficulty_color keys to English.

-- Step 1 — segType constraint (neu, bisher existierte keiner — verifiziert per pg_constraint am 2026-04-20).
-- The DROP IF EXISTS is a no-op against current production but protects against DB-generations that somehow grew one.
ALTER TABLE segments DROP CONSTRAINT IF EXISTS segments_segment_type_check;
ALTER TABLE segments ADD CONSTRAINT segments_segment_type_check
  CHECK (segment_type IN ('standard', 'trail', 'lap', 'lift', 'wintersport'));

-- Step 2 — Add bidirectional column (default TRUE keeps legacy matcher behavior).
ALTER TABLE segments ADD COLUMN IF NOT EXISTS bidirectional BOOLEAN NOT NULL DEFAULT true;

-- Step 2b — Harden segment_type_meta default so future jsonb_set on NULL does not
-- silently no-op. Existing DEFAULT '{}' was set at table-create; for rows written
-- before that default existed the column may still be NULL. We first backfill,
-- then re-assert the default so step 3/4 below can rely on non-NULL.
UPDATE segments SET segment_type_meta = '{}'::jsonb WHERE segment_type_meta IS NULL;
ALTER TABLE segments ALTER COLUMN segment_type_meta SET DEFAULT '{}'::jsonb;
ALTER TABLE segments ALTER COLUMN segment_type_meta SET NOT NULL;

-- Step 3 — Surface-label normalization.
-- Maps old German editor strings from SegmentEditorModal.tsx
-- (SURFACES=['Asphalt','Schotter','Erde','Sand','Gras','Fels','Pflaster','Holz'])
-- to new parent-group keys from surfaceConstants.ts SURFACE_GROUPS.
-- Identity mappings (Asphalt, Sand, Gras, Fels, Pflaster, Holz) are no-ops but
-- ensure any value already matching a new key is passed through correctly.
-- ELSE-branch passes modern keys through so the step is idempotent.
-- NOTE (Phase-2-Followup): segment_efforts is a TimescaleDB hypertable — we do NOT
-- touch effort_rows here, but any future meta-rewrite that wants to mutate effort
-- JSONB snapshots must also run a chunk-pruning step to avoid locking every chunk
-- at once. No action this migration.
UPDATE segments SET segment_type_meta = jsonb_set(
  segment_type_meta,
  '{surface}',
  to_jsonb(CASE segment_type_meta->>'surface'
    WHEN 'Asphalt'   THEN 'Asphalt'
    WHEN 'Pflaster'  THEN 'Pflaster'
    WHEN 'Schotter'  THEN 'Schotter (grob)'
    WHEN 'Erde'      THEN 'Naturweg'
    WHEN 'Gras'      THEN 'Gras'
    WHEN 'Sand'      THEN 'Sand'
    WHEN 'Fels'      THEN 'Fels'
    WHEN 'Holz'      THEN 'Holz'
    ELSE segment_type_meta->>'surface'
  END)
) WHERE segment_type_meta ? 'surface';

-- Step 4 — difficulty_color safety-net. Both editors already write English keys
-- ('blue'/'red'/'black'); this catches any historic debug/test inserts with
-- German keys that would otherwise silent-no-render in DifficultyBadge.
UPDATE segments SET segment_type_meta = jsonb_set(
  segment_type_meta,
  '{difficulty_color}',
  to_jsonb(CASE segment_type_meta->>'difficulty_color'
    WHEN 'blau'    THEN 'blue'
    WHEN 'rot'     THEN 'red'
    WHEN 'schwarz' THEN 'black'
    ELSE segment_type_meta->>'difficulty_color'
  END)
) WHERE segment_type_meta ? 'difficulty_color'
  AND segment_type_meta->>'difficulty_color' IN ('blau', 'rot', 'schwarz');
