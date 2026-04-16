# Migration Notes

Migrations live in `migrations/` and are applied in numerical order by
`migrate.go`. Rollbacks run in reverse numerical order unless noted below.

---

## Rollback Order

Postgres migrations are usually rolled back in reverse numerical order
(000N → 000N-1). For the fusion consistency refactor the order is
strict — violating it breaks the schema:

### Fusion refactor (000302–000308)

| # | Up | Down prerequisite |
|---|---|---|
| 000302 | Create activity_fusion_memberships + trigger | 000304 already rolled back (legacy columns restored) |
| 000303 | Allow edit_type='field_copy' | none |
| 000304 | Drop legacy is_fusion_source + fusion_sources | **LOSSY if new merges happened post-000304** — see RUNBOOK NOTICE in 000304_up.sql |
| 000305 | fusion_candidates.dismissed_at | none |
| 000306 | activity_photos.source_activity_id | none |
| 000307 | Consolidate cache-invalidation triggers | restores `fn_bump_fusion_cache_seq` + rebinds memberships trigger |
| 000308 | Tighten edit_type CHECK (drop 'merge') | none |

**Golden rule:** to roll back the entire fusion refactor:

1. `migrate down 000308` (widens CHECK).
2. `migrate down 000307` (restores duplicate trigger function).
3. `migrate down 000306` (drops source_activity_id).
4. `migrate down 000305` (drops dismissed_at).
5. `migrate down 000304` (restores legacy columns — LOSSY, see runbook notice in 000304_up.sql).
6. `migrate down 000303` (reverts CHECK to not allow field_copy).
7. `migrate down 000302` (drops memberships table + trigger — safe only AFTER step 5, because the recreated trigger in 000304_down.sql references the legacy columns that 000302_down.sql drops).

**Never roll back 000302 without first rolling back 000304.** The trigger
recreated by 000302_down.sql references columns (`is_fusion_source`,
`fusion_sources`) that were restored by 000304_down.sql. Rolling back
000302 first would attempt to drop a table while the trigger function still
references those columns — or leave orphaned trigger objects depending on
DROP ordering.
