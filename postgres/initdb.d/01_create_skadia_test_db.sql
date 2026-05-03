-- Runs once at first Postgres container init.
-- Creates the dedicated skadia_test database so migration integration tests
-- (services/api/internal/migrate/migrations/migration_*_test.go) have a target
-- that is NOT the production skadia database.
--
-- The test helper requireSafeTestDSN() refuses to run if TEST_DATABASE_URL
-- points at 'skadia' — see services/api/internal/migrate/migrations/
-- testdb_safety_test.go.
SELECT 'CREATE DATABASE skadia_test OWNER skadia'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'skadia_test')\gexec
