-- When FusionMerge runs in Group-Mode (req.GroupID is non-nil), the fused
-- sources should stay in their recording_group (they will be marked
-- is_group_secondary by trg_group_primary_sync after the post-commit
-- AddRecordingGroupMemberFused + SetRecordingGroupPrimary calls).
--
-- Without this gate, trigger 000313 (fn_fusion_source_removes_group_member)
-- fires on every activity_fusion_memberships INSERT — including the group-mode
-- case where all sources are group-members.  For 2-member groups this removes
-- all members from recording_group_members before the deferred sync can run,
-- causing the sync to dissolve the group.  The post-commit
-- AddRecordingGroupMemberFused then fails with FK violation (non-fatal),
-- leaving the merged activity without group context.
--
-- The FusionMerge handler signals Group-Mode by executing
--   SET LOCAL skadia.skip_rgm_cleanup = 'true'
-- inside the merge transaction before the activity_fusion_memberships INSERTs.
-- Ad-hoc merges (no group_id) leave the flag unset — the trigger fires as
-- before and cleans up zombie state.

CREATE OR REPLACE FUNCTION fn_fusion_source_removes_group_member()
RETURNS TRIGGER AS $$
BEGIN
    -- Group-Mode-Merge: sources must remain in the recording_group so that
    -- the post-commit AddRecordingGroupMemberFused + SetRecordingGroupPrimary
    -- can mark the merged activity as primary with the sources as secondaries.
    IF current_setting('skadia.skip_rgm_cleanup', true) = 'true' THEN
        RETURN NEW;
    END IF;
    IF NEW.source_id IS NOT NULL THEN
        DELETE FROM recording_group_members
        WHERE activity_id = NEW.source_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
