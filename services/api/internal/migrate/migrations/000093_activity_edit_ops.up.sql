-- activity_edits: audit log for trim/split/merge operations
CREATE TABLE IF NOT EXISTS activity_edits (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id UUID        NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id     UUID        NOT NULL,
    edit_type   TEXT        NOT NULL CHECK (edit_type IN ('trim', 'split', 'merge')),
    edit_data   JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_activity_edits_activity_id ON activity_edits(activity_id);
CREATE INDEX IF NOT EXISTS idx_activity_edits_user_id ON activity_edits(user_id);
