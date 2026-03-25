ALTER TABLE migration_filestore_journal RENAME TO migration_shadowfs_journal;
UPDATE background_jobs SET job_type = REPLACE(job_type, 'filestore_', 'shadowfs_')
  WHERE job_type LIKE 'filestore_%';
