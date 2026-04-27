-- Shorten user-facing names for built-in base map layers to their concise
-- historical labels (matched the prior hardcoded mapStyles.ts mapping).
UPDATE map_layers SET name = 'OSM'       WHERE key = 'osm'       AND is_system;
UPDATE map_layers SET name = 'Topo'      WHERE key = 'topo'      AND is_system;
UPDATE map_layers SET name = 'Satellit'  WHERE key = 'satellite' AND is_system;
UPDATE map_layers SET name = 'Dunkel'    WHERE key = 'dark'      AND is_system;
UPDATE map_layers SET name = 'Lokal'     WHERE key = 'local'     AND is_system;
UPDATE map_layers SET name = 'Offline'   WHERE key = 'offline'   AND is_system;
