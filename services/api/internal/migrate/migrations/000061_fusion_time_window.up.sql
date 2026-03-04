ALTER TABLE users
  ADD COLUMN IF NOT EXISTS fusion_time_window_min INT NOT NULL DEFAULT 30;
  -- Zeitfenster für Tier-2 Fusionserkennung in Minuten (1–120)
