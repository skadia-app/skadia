-- Make auto-tags (Nacht, Frost, Rundtour) system tags so they match weather tags behavior.
-- Previously they were seeded with is_system=false, category='custom' which was inconsistent.
UPDATE tags
SET is_system = true
WHERE is_auto = true
  AND name IN ('Nacht', 'Frost', 'Rundtour')
  AND is_system = false;
