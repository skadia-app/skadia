DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM target_activity_observations LIMIT 1)
       OR EXISTS (SELECT 1 FROM target_reports LIMIT 1)
       OR EXISTS (SELECT 1 FROM target_report_status_events LIMIT 1)
       OR EXISTS (SELECT 1 FROM target_user_content LIMIT 1)
    THEN
        RAISE EXCEPTION 'refusing destructive target overlay rollback while overlay data exists';
    END IF;
END $$;

DROP TABLE IF EXISTS target_overlay_summary_projection;
DROP TABLE IF EXISTS target_user_content;
DROP TABLE IF EXISTS target_report_status_events;
DROP TABLE IF EXISTS target_reports;
DROP TABLE IF EXISTS target_activity_observations;
