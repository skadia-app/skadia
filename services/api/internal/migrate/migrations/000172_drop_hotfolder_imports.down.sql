-- Recreated from 000009 original schema + 000169 fixes
CREATE TABLE hotfolder_imports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    filename    TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    error_msg   TEXT,
    activity_id UUID REFERENCES activities(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX ON hotfolder_imports (user_id, created_at DESC);
CREATE INDEX ON hotfolder_imports (status) WHERE status IN ('pending', 'processing');
