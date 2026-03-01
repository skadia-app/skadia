-- Wird beim ersten Start des Postgres-Containers ausgeführt.
-- Erstellt die OIDC-Datenbank (NUR Entwicklung).
SELECT 'CREATE DATABASE oidc OWNER skadia'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'oidc')\gexec
