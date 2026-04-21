-- 000322_segments_wassersport.up.sql
-- Erweitert segment_type-Check um 'wassersport' (§3 Spec)
ALTER TABLE segments DROP CONSTRAINT IF EXISTS segments_segment_type_check;
ALTER TABLE segments ADD CONSTRAINT segments_segment_type_check
  CHECK (segment_type IN ('standard', 'trail', 'lap', 'lift', 'wintersport', 'wassersport'));
