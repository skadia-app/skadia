ALTER TABLE activities ADD COLUMN parent_id UUID REFERENCES activities(id) ON DELETE SET NULL;
CREATE INDEX idx_activities_parent ON activities(parent_id) WHERE parent_id IS NOT NULL;
