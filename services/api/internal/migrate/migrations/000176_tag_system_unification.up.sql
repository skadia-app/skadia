-- 000176_tag_system_unification.up.sql
--
-- Tag-System Unification: Konsolidiert drei Auto-Tag-Mechanismen auf die Rules Engine.
-- Reihenfolge ist kritisch: Adjektive VOR Kategorie-Updates (WHERE category='weather').

-- ============================================================
-- 1. Adjektiv-Tags kleinschreiben (ZUERST — nutzt category='weather')
-- ============================================================
UPDATE tags t SET name = 'sonnig'
WHERE t.name = 'Sonnig' AND t.is_system = true AND t.category = 'weather'
  AND NOT EXISTS (SELECT 1 FROM tags t2 WHERE t2.user_id = t.user_id AND t2.name = 'sonnig');

UPDATE tags t SET name = 'bewölkt'
WHERE t.name = 'Bewölkt' AND t.is_system = true AND t.category = 'weather'
  AND NOT EXISTS (SELECT 1 FROM tags t2 WHERE t2.user_id = t.user_id AND t2.name = 'bewölkt');

UPDATE activities SET tags = array_replace(tags, 'Sonnig', 'sonnig') WHERE 'Sonnig' = ANY(tags);
UPDATE activities SET tags = array_replace(tags, 'Bewölkt', 'bewölkt') WHERE 'Bewölkt' = ANY(tags);

-- ============================================================
-- 2. automation_rules erweitern
-- ============================================================
ALTER TABLE automation_rules
  ADD COLUMN is_system BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN system_type TEXT CHECK (system_type IN ('ebike_sync', 'round_trip', 'frost', 'nacht'));

CREATE UNIQUE INDEX uq_automation_rules_system_type
  ON automation_rules (user_id, system_type) WHERE is_system = true;

-- has_automation materialisierte Spalte auf tags
ALTER TABLE tags ADD COLUMN has_automation BOOLEAN NOT NULL DEFAULT false;

-- ============================================================
-- 3. System-Rules für alle bestehenden User anlegen
-- ============================================================

-- 3a. E-Bike (UI-Anker, keine Conditions/Actions)
INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
SELECT u.id, 'E-Bike', true, 'ebike_sync', true, -100, '[]'::jsonb, 'and', '[]'::jsonb
FROM users u
ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING;

-- 3b. Rundtour
INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
SELECT u.id, 'Rundtour', true, 'round_trip', true, -99,
  '[{"field":"start_end_distance_m","op":"lte","value":500}]'::jsonb,
  'and',
  json_build_array(
    json_build_object('action', 'add_tag', 'params', json_build_object('tag_id', t.id::text)),
    json_build_object('action', 'set_field', 'params', json_build_object('field', 'is_round_trip', 'value', true))
  )::jsonb
FROM users u
JOIN tags t ON t.user_id = u.id AND t.name = 'Rundtour' AND t.is_system = true
ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING;

-- 3c. Frost
-- Bewusste Vereinfachung: nur min_temperature_c <= 0 (vorher zusätzlich avg_temperature_c <= 0).
-- User kann die Condition im UI editieren falls gewünscht.
INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
SELECT u.id, 'Frost', true, 'frost', true, -98,
  '[{"field":"min_temperature_c","op":"lte","value":0}]'::jsonb,
  'and',
  json_build_array(
    json_build_object('action', 'add_tag', 'params', json_build_object('tag_id', t.id::text))
  )::jsonb
FROM users u
JOIN tags t ON t.user_id = u.id AND t.name = 'Frost' AND t.is_system = true
ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING;

-- 3d. Nacht
-- Default -6° = bürgerliche Dämmerung (nicht -18° astronomische Nacht, die im Sommer in
-- Mitteleuropa nie erreicht wird). User kann die Gradzahl im UI editieren.
INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
SELECT u.id, 'Nacht', true, 'nacht', true, -97,
  '[{"field":"sun_elevation","op":"lte","value":-6}]'::jsonb,
  'and',
  json_build_array(
    json_build_object('action', 'add_tag', 'params', json_build_object('tag_id', t.id::text))
  )::jsonb
FROM users u
JOIN tags t ON t.user_id = u.id AND t.name = 'Nacht' AND t.is_system = true
ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING;

-- Initialen has_automation Wert setzen für Tags die in System-Rules referenziert werden
UPDATE tags t SET has_automation = true
WHERE EXISTS (
  SELECT 1 FROM automation_rules ar
  WHERE ar.user_id = t.user_id AND ar.is_active = true
    AND ar.actions::text LIKE '%"tag_id":"' || t.id::text || '"%'
);

-- ============================================================
-- 4. Tag-Kategorien fixen
-- ============================================================
UPDATE tags SET category = 'system'
WHERE is_system = true AND (category IS NULL OR category = 'custom' OR category = '');

-- Location-Tags
UPDATE tags SET category = 'standort'
WHERE is_system = false AND lat IS NOT NULL
  AND (category IS NULL OR category = 'custom' OR category = '');

-- ============================================================
-- 5. Rundtour-Trigger entfernen (System-Rules sind jetzt angelegt)
-- ============================================================
DROP TRIGGER IF EXISTS trg_sync_rundtour_to_tags ON activities;
DROP TRIGGER IF EXISTS trg_sync_rundtour_from_tags ON activity_tags;
DROP FUNCTION IF EXISTS sync_rundtour_to_tags();
DROP FUNCTION IF EXISTS sync_rundtour_from_tags();
