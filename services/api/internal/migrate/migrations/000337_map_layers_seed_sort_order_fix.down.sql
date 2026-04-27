-- 000337_map_layers_seed_sort_order_fix.down.sql
-- 000337 was idempotent (000333 seeded these sort_order values exactly the
-- same; the up migration only re-affirms them). Down is a no-op — there is
-- no prior state to restore. Reverting would not change any row.
SELECT 1;
