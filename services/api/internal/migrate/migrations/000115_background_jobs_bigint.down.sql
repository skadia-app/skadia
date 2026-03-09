ALTER TABLE background_jobs
    ALTER COLUMN progress_current TYPE INT,
    ALTER COLUMN progress_total   TYPE INT;
