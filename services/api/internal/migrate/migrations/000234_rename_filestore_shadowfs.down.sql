ALTER TABLE migration_shadowfs_journal RENAME TO migration_filestore_journal;
UPDATE background_jobs SET job_type = REPLACE(job_type, 'shadowfs_', 'filestore_')
  WHERE job_type LIKE 'shadowfs_%';
