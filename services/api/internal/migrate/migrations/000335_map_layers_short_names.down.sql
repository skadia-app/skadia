UPDATE map_layers SET name = 'OpenStreetMap'        WHERE key = 'osm'       AND is_system;
UPDATE map_layers SET name = 'OpenTopoMap'          WHERE key = 'topo'      AND is_system;
UPDATE map_layers SET name = 'Esri World Imagery'   WHERE key = 'satellite' AND is_system;
UPDATE map_layers SET name = 'Carto Dark'           WHERE key = 'dark'      AND is_system;
UPDATE map_layers SET name = 'Lokal (Vector)'       WHERE key = 'local'     AND is_system;
UPDATE map_layers SET name = 'Offline (Protomaps)'  WHERE key = 'offline'   AND is_system;
