-- Phase 6: Gear + Automation Rules

CREATE TABLE gear (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    sub_type TEXT,
    brand TEXT,
    model TEXT,
    total_distance_m DOUBLE PRECISION DEFAULT 0,
    total_time_s BIGINT DEFAULT 0,
    total_activities INTEGER DEFAULT 0,
    purchased_at DATE,
    retired_at DATE,
    is_retired BOOLEAN DEFAULT false,
    default_for_sports TEXT[],
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX ON gear (user_id, is_retired);

CREATE TABLE gear_components (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gear_id UUID NOT NULL REFERENCES gear(id) ON DELETE CASCADE,
    parent_component_id UUID REFERENCES gear_components(id),
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    brand TEXT,
    model TEXT,
    total_distance_m DOUBLE PRECISION DEFAULT 0,
    installed_at TIMESTAMPTZ,
    removed_at TIMESTAMPTZ,
    is_retired BOOLEAN DEFAULT false,
    notes TEXT
);

CREATE TABLE gear_service_intervals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    component_id UUID NOT NULL REFERENCES gear_components(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    interval_type TEXT NOT NULL, -- distance_m|time_s|elapsed_days
    interval_value DOUBLE PRECISION NOT NULL,
    is_recurring BOOLEAN DEFAULT true,
    last_service_at TIMESTAMPTZ,
    last_service_distance_m DOUBLE PRECISION,
    next_due_at TIMESTAMPTZ
);

-- gear_id FK auf activities
ALTER TABLE activities ADD COLUMN IF NOT EXISTS gear_id UUID REFERENCES gear(id) ON DELETE SET NULL;

CREATE TABLE automation_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    is_active BOOLEAN DEFAULT true,
    priority INTEGER DEFAULT 0,
    conditions JSONB NOT NULL,
    actions JSONB NOT NULL,
    times_applied INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX ON automation_rules (user_id, is_active, priority);
