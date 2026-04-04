-- Add sport_group column with default for existing rows
ALTER TABLE pmc_daily ADD COLUMN sport_group TEXT NOT NULL DEFAULT '__combined__';

-- Drop existing PK and recreate with sport_group
ALTER TABLE pmc_daily DROP CONSTRAINT pmc_daily_pkey;
ALTER TABLE pmc_daily ADD PRIMARY KEY (user_id, date, sport_group);

-- Index for sport-specific queries
CREATE INDEX idx_pmc_daily_sport ON pmc_daily (user_id, sport_group, date);
