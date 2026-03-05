-- 000081_backfill_elevation_loss.down.sql
-- Setzt die nachgefüllten Werte zurück auf NULL.
-- Achtung: Aktivitäten die den Wert bereits vor Migration 000081 hatten,
-- werden NICHT berührt (kein einfaches Rollback möglich).

-- Intentionally empty: kann nicht unterscheiden welche Werte
-- durch den Backfill gesetzt wurden vs. welche bereits vorher existierten.
-- Für sauberes Rollback: activities aus Backup wiederherstellen.
SELECT 'Migration 000081 ist nicht reversibel ohne Backup.' AS info;
