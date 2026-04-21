-- 000322_segments_wassersport.down.sql
-- Rollback auf 6 segTypes ohne wassersport
-- VORSICHT: schlägt fehl wenn wassersport-Segmente existieren
ALTER TABLE segments DROP CONSTRAINT IF EXISTS segments_segment_type_check;
ALTER TABLE segments ADD CONSTRAINT segments_segment_type_check
  CHECK (segment_type IN ('standard', 'trail', 'lap', 'lift', 'wintersport'));
