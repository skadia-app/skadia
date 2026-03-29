ALTER TABLE segments DROP CONSTRAINT IF EXISTS segments_visibility_check;

-- Restore effort_count and athlete_count to total (all activities, including private).
UPDATE segments s SET
    effort_count = (
        SELECT COUNT(*) FROM segment_efforts WHERE segment_id = s.id
    ),
    athlete_count = (
        SELECT COUNT(DISTINCT user_id) FROM segment_efforts WHERE segment_id = s.id
    );
