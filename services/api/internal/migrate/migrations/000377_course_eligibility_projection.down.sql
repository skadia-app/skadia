ALTER TABLE segment_efforts
    DROP COLUMN IF EXISTS eligibility_source_hash,
    DROP COLUMN IF EXISTS eligibility_policy_version,
    DROP COLUMN IF EXISTS eligibility_policy_key,
    DROP COLUMN IF EXISTS activity_trait_bucket_key;

DROP INDEX IF EXISTS idx_tcep_filter_preset;
DROP INDEX IF EXISTS idx_tcep_target_lookup;
DROP INDEX IF EXISTS idx_tcep_candidate_lookup;

DROP TABLE IF EXISTS target_course_eligibility_projection;
DROP TABLE IF EXISTS activity_trait_projection;
