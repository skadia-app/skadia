-- 000324_segments_sport_group_cleanup.up.sql
-- Fix legacy sport_group-Werte wo sport_type statt segmentGroup gespeichert wurde
-- Whitelist-Ansatz: Nur bekannte falsche Werte werden gemappt
UPDATE segments SET sport_group = 'foot'    WHERE sport_group IN ('running', 'hiking', 'walking', 'trail_running', 'snowshoeing');
UPDATE segments SET sport_group = 'bike'    WHERE sport_group IN ('cycling', 'mountain_biking', 'gravel_biking', 'bikepacking', 'indoor_cycling');
UPDATE segments SET sport_group = 'swim'    WHERE sport_group IN ('swimming', 'lap_swimming', 'open_water_swimming');
UPDATE segments SET sport_group = 'ski'     WHERE sport_group IN ('alpine_skiing', 'cross_country_skiing', 'backcountry_skiing', 'snowboarding');
UPDATE segments SET sport_group = 'paddle'  WHERE sport_group IN ('kayaking', 'stand_up_paddleboarding', 'canoeing', 'rowing', 'rafting', 'paddling');
UPDATE segments SET sport_group = 'generic' WHERE sport_group IN ('surfing', 'kitesurfing', 'windsurfing', 'wakeboarding', 'sailing', 'boating');
-- KEIN NOT IN-Catchall: unbekannte Werte bleiben — müssen manuell geprüft werden
