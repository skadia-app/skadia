-- M30: Fix check_file_attachment_ownership trigger function.
-- Move SET search_path from PL/pgSQL body to function-level option.
-- A SET inside a PL/pgSQL body requires SECURITY DEFINER to take effect;
-- as a function option it is applied before any user code runs.
CREATE OR REPLACE FUNCTION check_file_attachment_ownership() RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM user_files WHERE id = NEW.file_id AND user_id = NEW.user_id) THEN
        RAISE EXCEPTION 'file_id does not belong to user_id';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql SET search_path = public;
