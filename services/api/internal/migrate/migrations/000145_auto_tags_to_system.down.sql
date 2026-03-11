UPDATE tags
SET is_system = false
WHERE is_auto = true
  AND name IN ('Nacht', 'Frost', 'Rundtour')
  AND is_system = true;
