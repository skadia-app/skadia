-- Erweitere activity_edits.edit_type CHECK-Constraint um 'fusion_merge'.
-- Ursache: FusionMerge (V1) und FusionMergeV2 fügen einen Audit-Record mit
-- edit_type='fusion_merge' ein. Der ursprüngliche Constraint aus Migration 000093
-- erlaubt nur ('trim', 'split', 'merge'), wodurch der INSERT fehlschlug.
-- In einer Transaktion vergiftete diese CHECK-Violation die gesamte Tx (25P02),
-- sodass der anschließende Commit als Rollback endete und die komplette Fusion
-- mit 500 "failed to create merged activity" abbrach.

ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;

ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
    CHECK (edit_type IN ('trim', 'split', 'merge', 'fusion_merge'));
