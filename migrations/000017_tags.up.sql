CREATE TABLE tags (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    color           TEXT NOT NULL DEFAULT '#6366F1',
    text_color      TEXT NOT NULL DEFAULT '#FFFFFF',
    category        TEXT NOT NULL DEFAULT 'custom',
    is_system       BOOLEAN DEFAULT false,
    activity_count  INTEGER DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, name)
);

CREATE TABLE activity_tags (
    activity_id  UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    tag_id       UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    assigned_by  TEXT NOT NULL DEFAULT 'manual',
    PRIMARY KEY (activity_id, tag_id)
);
