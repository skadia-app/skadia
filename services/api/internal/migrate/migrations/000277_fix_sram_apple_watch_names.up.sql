-- 000277_fix_sram_apple_watch_names.up.sql
-- Fixes two device naming issues:
-- 1. SRAM (manufacturer_id=268): product_name "Sram" → "SRAM"
-- 2. Apple Watch (manufacturer_id=255): raw iOS model IDs (e.g. "Watch5,10") → readable names

-- SRAM: FormatManufacturer now returns "SRAM" (was "Sram")
UPDATE devices
SET product_name = 'SRAM'
WHERE manufacturer_id = 268 AND product_name = 'Sram';

-- Apple Watch: map raw iOS model identifiers to human-readable names.
-- Verified identifiers from Apple public docs + local fixture coverage.
UPDATE devices
SET product_name = CASE product_name
    WHEN 'Watch1,1'  THEN 'Apple Watch 38mm'
    WHEN 'Watch1,2'  THEN 'Apple Watch 42mm'
    WHEN 'Watch2,3'  THEN 'Apple Watch Series 2 38mm'
    WHEN 'Watch2,4'  THEN 'Apple Watch Series 2 42mm'
    WHEN 'Watch2,6'  THEN 'Apple Watch Series 1 38mm'
    WHEN 'Watch2,7'  THEN 'Apple Watch Series 1 42mm'
    WHEN 'Watch3,1'  THEN 'Apple Watch Series 3 38mm Cellular'
    WHEN 'Watch3,2'  THEN 'Apple Watch Series 3 42mm Cellular'
    WHEN 'Watch3,3'  THEN 'Apple Watch Series 3 38mm'
    WHEN 'Watch3,4'  THEN 'Apple Watch Series 3 42mm'
    WHEN 'Watch4,1'  THEN 'Apple Watch Series 4 40mm'
    WHEN 'Watch4,2'  THEN 'Apple Watch Series 4 44mm'
    WHEN 'Watch4,3'  THEN 'Apple Watch Series 4 40mm Cellular'
    WHEN 'Watch4,4'  THEN 'Apple Watch Series 4 44mm Cellular'
    WHEN 'Watch5,1'  THEN 'Apple Watch Series 5 40mm'
    WHEN 'Watch5,2'  THEN 'Apple Watch Series 5 44mm'
    WHEN 'Watch5,3'  THEN 'Apple Watch Series 5 40mm Cellular'
    WHEN 'Watch5,4'  THEN 'Apple Watch Series 5 44mm Cellular'
    WHEN 'Watch5,9'  THEN 'Apple Watch SE 40mm'
    WHEN 'Watch5,10' THEN 'Apple Watch SE 44mm'
    WHEN 'Watch5,11' THEN 'Apple Watch SE 40mm Cellular'
    WHEN 'Watch5,12' THEN 'Apple Watch SE 44mm Cellular'
    WHEN 'Watch6,1'  THEN 'Apple Watch Series 6 40mm'
    WHEN 'Watch6,2'  THEN 'Apple Watch Series 6 44mm'
    WHEN 'Watch6,3'  THEN 'Apple Watch Series 6 40mm Cellular'
    WHEN 'Watch6,4'  THEN 'Apple Watch Series 6 44mm Cellular'
    WHEN 'Watch6,6'  THEN 'Apple Watch Series 7 41mm'
    WHEN 'Watch6,7'  THEN 'Apple Watch Series 7 45mm'
    WHEN 'Watch6,8'  THEN 'Apple Watch Series 7 41mm Cellular'
    WHEN 'Watch6,9'  THEN 'Apple Watch Series 7 45mm Cellular'
    WHEN 'Watch6,10' THEN 'Apple Watch SE 2 40mm'
    WHEN 'Watch6,11' THEN 'Apple Watch SE 2 44mm'
    WHEN 'Watch6,12' THEN 'Apple Watch SE 2 40mm Cellular'
    WHEN 'Watch6,13' THEN 'Apple Watch SE 2 44mm Cellular'
    WHEN 'Watch6,14' THEN 'Apple Watch Series 8 41mm'
    WHEN 'Watch6,15' THEN 'Apple Watch Series 8 45mm'
    WHEN 'Watch6,16' THEN 'Apple Watch Series 8 41mm Cellular'
    WHEN 'Watch6,17' THEN 'Apple Watch Series 8 45mm Cellular'
    WHEN 'Watch6,18' THEN 'Apple Watch Ultra'
    WHEN 'Watch7,1'  THEN 'Apple Watch Series 9 41mm'
    WHEN 'Watch7,2'  THEN 'Apple Watch Series 9 45mm'
    WHEN 'Watch7,3'  THEN 'Apple Watch Series 9 41mm Cellular'
    WHEN 'Watch7,4'  THEN 'Apple Watch Series 9 45mm Cellular'
    WHEN 'Watch7,5'  THEN 'Apple Watch Ultra 2'
    WHEN 'Watch7,8'  THEN 'Apple Watch Series 10 42mm'
    WHEN 'Watch7,9'  THEN 'Apple Watch Series 10 46mm'
    WHEN 'Watch7,10' THEN 'Apple Watch Series 10 42mm Cellular'
    WHEN 'Watch7,11' THEN 'Apple Watch Series 10 46mm Cellular'
    WHEN 'Watch7,12' THEN 'Apple Watch Ultra 2'
    WHEN 'Watch7,13' THEN 'Apple Watch SE 3 42mm'
    WHEN 'Watch7,14' THEN 'Apple Watch SE 3 46mm'
    WHEN 'Watch7,15' THEN 'Apple Watch SE 3 42mm Cellular'
    WHEN 'Watch7,16' THEN 'Apple Watch SE 3 46mm Cellular'
    WHEN 'Watch7,17' THEN 'Apple Watch Series 11 42mm'
    WHEN 'Watch7,18' THEN 'Apple Watch Series 11 46mm'
    WHEN 'Watch7,19' THEN 'Apple Watch Series 11 42mm Cellular'
    WHEN 'Watch7,20' THEN 'Apple Watch Series 11 46mm Cellular'
    ELSE product_name
END
WHERE manufacturer_id = 255
  AND product_name LIKE 'Watch%';
