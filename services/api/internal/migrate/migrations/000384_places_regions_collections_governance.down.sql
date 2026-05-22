DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM target_places LIMIT 1)
       OR EXISTS (SELECT 1 FROM target_regions LIMIT 1)
       OR EXISTS (SELECT 1 FROM target_collections LIMIT 1)
       OR EXISTS (SELECT 1 FROM target_collection_members LIMIT 1)
       OR EXISTS (SELECT 1 FROM governance_scopes LIMIT 1)
       OR EXISTS (SELECT 1 FROM governance_actor_roles LIMIT 1)
       OR EXISTS (SELECT 1 FROM governance_scope_target_bindings LIMIT 1)
       OR EXISTS (SELECT 1 FROM target_change_events LIMIT 1)
       OR EXISTS (SELECT 1 FROM target_revision_snapshots LIMIT 1)
    THEN
        RAISE EXCEPTION 'refusing destructive target object/governance rollback while target data exists';
    END IF;
END $$;

DROP TABLE IF EXISTS governance_actor_target_projection;
ALTER TABLE IF EXISTS target_change_events DROP CONSTRAINT IF EXISTS target_change_events_before_snapshot_fk;
ALTER TABLE IF EXISTS target_change_events DROP CONSTRAINT IF EXISTS target_change_events_after_snapshot_fk;
DROP TABLE IF EXISTS target_revision_snapshots;
DROP TABLE IF EXISTS target_change_events;
DROP TABLE IF EXISTS governance_scope_target_bindings;
DROP TABLE IF EXISTS governance_actor_roles;
DROP TABLE IF EXISTS governance_scopes;
DROP TABLE IF EXISTS target_collection_summary_projection;
DROP TABLE IF EXISTS target_object_search_projection;
DROP TABLE IF EXISTS target_collection_member_reverse_index;
DROP TABLE IF EXISTS target_collection_members;
DROP TABLE IF EXISTS target_collections;
DROP TABLE IF EXISTS target_regions;
DROP TABLE IF EXISTS target_places;
