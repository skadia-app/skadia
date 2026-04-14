-- WARNUNG: Diese Down-Migration ist nur für Test-/Dev-Umgebungen gedacht.
-- In Produktion mit aktiven User-Daten NICHT ausführen — ein Down-Pingpong
-- (298 down → 298 up → 299 down → 299 up …) kann Daten korrumpieren, weil
-- Tag-Merges nicht reversibel sind: einmal gelöschte Tag-IDs kommen nicht
-- zurück, und gemischte Bestände ("beide Tags existierten vor der Migration")
-- sind nach dem ersten Up-Run nicht mehr unterscheidbar von reinen
-- Migrationsartefakten.
-- Zusätzlich: Wenn 299 bereits appliziert wurde, darf 298 NICHT einzeln
-- heruntergerollt werden — immer beide oder keine.
--
-- Down-migration is a best-effort rollback. It cannot distinguish auto-tagged
-- activities that already had "Intervall-Training" before the unification from
-- those that were unified. We rename the hyphenated tag back for users who
-- do not have a (non-hyphenated) counterpart; users who had both will not
-- have the separate tag re-created.
DO $$
DECLARE
    u_id TEXT;
    tag_id TEXT;
BEGIN
    FOR u_id, tag_id IN
        SELECT user_id::text, id::text FROM tags WHERE name = 'Intervall-Training'
    LOOP
        IF NOT EXISTS (
            SELECT 1 FROM tags
            WHERE user_id::text = u_id AND name = 'Intervalltraining'
        ) THEN
            UPDATE tags SET name = 'Intervalltraining' WHERE id::text = tag_id;
            UPDATE activities
            SET tags = array_replace(tags, 'Intervall-Training', 'Intervalltraining')
            WHERE user_id::text = u_id
              AND tags @> ARRAY['Intervall-Training'];
        END IF;
    END LOOP;
END $$;
