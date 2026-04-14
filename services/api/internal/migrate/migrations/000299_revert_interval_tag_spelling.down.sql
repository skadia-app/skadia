-- WARNUNG: Diese Down-Migration ist nur für Test-/Dev-Umgebungen gedacht.
-- In Produktion mit aktiven User-Daten NICHT ausführen — ein Down-Pingpong
-- zwischen 298/299 kann Daten korrumpieren, weil Tag-Merges nicht reversibel
-- sind: einmal gelöschte Tag-IDs bleiben verloren, und der ursprüngliche
-- Bestand (wer hatte vor 298 welchen Tag?) ist nach dem ersten Up-Run
-- nicht mehr rekonstruierbar.
-- Zusätzlich: Diese Down-Migration ist sinnlos allein — sie erzeugt wieder
-- die inkonsistente Mischung, die 299 gerade erst aufgeräumt hat. Nur
-- kombiniert mit einem anschließenden Down von 298 verwenden.
--
-- Reverse: rename "Intervalltraining" → "Intervall-Training" for users
-- who do not already have the hyphenated tag. Cannot distinguish users who
-- already had "Intervalltraining" prior to 000298 from those who were
-- auto-created/renamed by 298/299.
DO $$
DECLARE
    u_id TEXT;
    tag_id TEXT;
BEGIN
    FOR u_id, tag_id IN
        SELECT user_id::text, id::text FROM tags WHERE name = 'Intervalltraining'
    LOOP
        IF NOT EXISTS (
            SELECT 1 FROM tags
            WHERE user_id::text = u_id AND name = 'Intervall-Training'
        ) THEN
            UPDATE tags SET name = 'Intervall-Training' WHERE id::text = tag_id;
            UPDATE activities
            SET tags = array_replace(tags, 'Intervalltraining', 'Intervall-Training')
            WHERE user_id::text = u_id
              AND tags @> ARRAY['Intervalltraining'];
        END IF;
    END LOOP;

    UPDATE automation_rules
    SET name = 'Intervall-Training'
    WHERE name = 'Intervalltraining';
END $$;
