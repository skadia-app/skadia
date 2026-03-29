-- Data correction only — segments.visibility column exists since migration 000032.
-- Recalculates effort_count and athlete_count for all segments,
-- excluding efforts from non-public activities (whitelist: only 'public').
UPDATE segments s SET
    effort_count = (
        SELECT COUNT(*)
        FROM segment_efforts se
        JOIN activities a ON a.id = se.activity_id AND a.visibility = 'public'
        WHERE se.segment_id = s.id
    ),
    athlete_count = (
        SELECT COUNT(DISTINCT se.user_id)
        FROM segment_efforts se
        JOIN activities a ON a.id = se.activity_id AND a.visibility = 'public'
        WHERE se.segment_id = s.id
    );

-- Enforce valid visibility values at DB level.
ALTER TABLE segments
    ADD CONSTRAINT segments_visibility_check
    CHECK (visibility IN ('public', 'private'));
