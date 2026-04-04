CREATE TABLE routing_profiles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    base_profile TEXT NOT NULL,
    sport_types  TEXT[] NOT NULL DEFAULT '{}',
    config       JSONB NOT NULL,
    source       TEXT NOT NULL DEFAULT 'user',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX routing_profiles_user_name
    ON routing_profiles (user_id, name) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX routing_profiles_global_name
    ON routing_profiles (name) WHERE user_id IS NULL;
