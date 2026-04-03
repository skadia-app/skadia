-- 000275_fix_garmin_product_names.up.sql
-- Corrects Garmin device product_name formatting inconsistencies caused by the old
-- formatDeviceNameFromEnum logic (title-case only, no acronym/digit-split handling).
--
-- Patterns fixed:
--   "Garmin Fr955"        → "Garmin FR955"          (Forerunner series)
--   "Garmin Fr945 Lte"    → "Garmin FR945 LTE"
--   "Garmin Marq Gen2"    → "Garmin MARQ Gen 2"
--   "Garmin Fenix6x"      → "Garmin Fenix 6X"       (alpha+digit split + variant suffix)
--   "Garmin Enduro2"      → "Garmin Enduro 2"
--   "Garmin Venu3"        → "Garmin Venu 3"
--   "Garmin Epix Gen2"    → "Garmin Epix Gen 2"
--   "Garmin Instinct3 Amoled 50mm" → "Garmin Instinct 3 AMOLED 50mm"
-- Also fixes specific models that stored only the manufacturer fallback "Garmin":
--   manufacturer_id=1, product_id=4565 → "Garmin FR970"
--   manufacturer_id=1, product_id=4574 → "Garmin FR570"

DO $$
DECLARE
    r   RECORD;
    n   TEXT;
BEGIN
    FOR r IN
        SELECT DISTINCT product_name
        FROM devices
        WHERE product_name LIKE 'Garmin %'
    LOOP
        n := r.product_name;

        -- 1. Forerunner "Fr" prefix → "FR"
        n := regexp_replace(n, '^Garmin Fr(\d)', 'Garmin FR\1');

        -- 2. "Marq " → "MARQ "
        n := replace(n, 'Garmin Marq ', 'Garmin MARQ ');

        -- 3. Known acronyms as standalone space-delimited words
        n := replace(n, ' Lte',    ' LTE');
        n := replace(n, ' Hrm',    ' HRM');
        n := replace(n, ' Gps',    ' GPS');
        n := replace(n, ' Amoled', ' AMOLED');

        -- 4. Insert space between a capitalized word and an immediately-following digit
        --    e.g. "Fenix6" → "Fenix 6",  "Gen2" → "Gen 2",  "Instinct3" → "Instinct 3"
        --    [A-Z][a-z]+ ensures we only match title-cased words, not "FR" or "MARQ".
        n := regexp_replace(n, '([A-Z][a-z]+)(\d)', '\1 \2', 'g');

        -- 5. Uppercase known single-letter variant suffixes attached to a digit
        --    (after step 4, "Fenix6x" became "Fenix 6x" — now fix the trailing 'x'/'s')
        n := replace(n, ' 6x', ' 6X');
        n := replace(n, ' 7x', ' 7X');
        n := replace(n, ' 8x', ' 8X');
        n := replace(n, ' 6s', ' 6S');
        n := replace(n, ' 7s', ' 7S');
        n := replace(n, ' 8s', ' 8S');
        n := replace(n, ' 3s', ' 3S');
        n := replace(n, ' 6sl', ' 6SL');
        n := replace(n, ' 7sl', ' 7SL');

        IF n <> r.product_name THEN
            UPDATE devices SET product_name = n WHERE product_name = r.product_name;
            RAISE NOTICE 'devices: renamed "%" → "%"', r.product_name, n;
        END IF;
    END LOOP;
END $$;

-- Fix models that were stored as bare "Garmin" fallback because their product_id
-- was not yet in the GarminProduct SDK enum at import time.
UPDATE devices
SET product_name = 'Garmin FR970'
WHERE manufacturer_id = 1
  AND product_id = 4565
  AND product_name = 'Garmin';

UPDATE devices
SET product_name = 'Garmin FR570'
WHERE manufacturer_id = 1
  AND product_id = 4574
  AND product_name = 'Garmin';
