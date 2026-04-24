-- 000326_library_settings_admin_model.up.sql
-- Reset UUID-förmige library_dir_name-Werte (Altlast aus alter API-Semantik)
-- und schreibe Audit-Log-Eintrag. Kein Seed von server_settings — der läuft
-- via ApplySeedsIfMissing beim API-Startup.

-- 1. UUID-artige Werte auf NULL.
-- Regex matcht letzten Block mit 8-12 Hex-Chars (deckt 35/36-Char + kürzere
-- Kürzungsvarianten ab). User-gesetzte Labels wie "my-library" werden nicht
-- getroffen (sie haben kein 8-4-4-4-Präfix).
UPDATE users
SET library_dir_name = NULL
WHERE library_dir_name IS NOT NULL
  AND library_dir_name ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{8,12}$';

-- 2. Audit-Log
INSERT INTO auth_audit_log (user_id, event_type, details)
VALUES (NULL, 'migration', jsonb_build_object(
  'migration', '000326',
  'name', 'library_settings_admin_model',
  'reset_null_count', (SELECT count(*) FROM users WHERE library_dir_name IS NULL)
));
