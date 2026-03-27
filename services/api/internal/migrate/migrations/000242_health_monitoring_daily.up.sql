-- Continuous Aggregate health_monitoring_daily is created by Go app startup code
-- (garmin.EnsureMonitoringDailyCA) because TimescaleDB CAs cannot be created
-- inside a transaction, and golang-migrate wraps every migration in one.
-- This migration is intentionally a no-op placeholder to keep the version sequence.
SELECT 1;
