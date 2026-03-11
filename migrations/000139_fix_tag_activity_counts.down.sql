-- Downgrade: reset activity_count to 0
-- Note: This can't recover the original values, but 0 is the safe fallback
UPDATE tags SET activity_count = 0;
