-- 000134_route_photos_unique_hash.up.sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_route_photos_unique_hash ON route_photos(route_id, file_hash);
