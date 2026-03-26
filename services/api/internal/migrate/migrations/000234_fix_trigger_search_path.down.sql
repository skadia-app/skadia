-- Revert: restore original body-level SET search_path.
CREATE OR REPLACE FUNCTION check_file_attachment_ownership() RETURNS TRIGGER AS $$
BEGIN
    SET search_path = public;
    IF NOT EXISTS (SELECT 1 FROM user_files WHERE id = NEW.file_id AND user_id = NEW.user_id) THEN
        RAISE EXCEPTION 'file_id does not belong to user_id';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
