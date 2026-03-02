-- Phase 2: Shadow File System + Hotfolder

-- Dateiablage-Schema für Shadow FS (Template-Pfad)
ALTER TABLE users ADD COLUMN IF NOT EXISTS file_naming_schema TEXT DEFAULT '/{year}/{month}/{day}_{sport}_{city}';

-- Lokaler Dateipfad im Shadow FS
ALTER TABLE activities ADD COLUMN IF NOT EXISTS local_file_path TEXT;

-- Import-Log für Hotfolder
CREATE TABLE hotfolder_imports (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    filename             TEXT NOT NULL,
    status               TEXT NOT NULL DEFAULT 'pending',  -- pending/processing/done/failed
    error_msg            TEXT,
    imported_activity_id UUID REFERENCES activities(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX ON hotfolder_imports (user_id, created_at DESC);
CREATE INDEX ON hotfolder_imports (status) WHERE status IN ('pending', 'processing');
