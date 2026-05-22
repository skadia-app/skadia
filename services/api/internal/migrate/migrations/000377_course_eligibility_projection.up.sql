CREATE TABLE IF NOT EXISTS activity_trait_projection (
    activity_trait_bucket_key TEXT PRIMARY KEY,
    trait_policy_key TEXT NOT NULL,
    trait_policy_version INTEGER NOT NULL,
    source_hash TEXT NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sport_type TEXT NOT NULL,
    sub_type TEXT NOT NULL DEFAULT '',
    is_indoor BOOLEAN NOT NULL,
    is_virtual BOOLEAN NOT NULL,
    is_electric BOOLEAN NOT NULL,
    movement_families TEXT[] NOT NULL DEFAULT '{}',
    propulsion_modes TEXT[] NOT NULL DEFAULT '{}',
    equipment_families TEXT[] NOT NULL DEFAULT '{}',
    activity_form TEXT NOT NULL DEFAULT '',
    environment_state TEXT NOT NULL DEFAULT 'real_world',
    UNIQUE (trait_policy_key, trait_policy_version, sport_type, sub_type, is_indoor, is_virtual, is_electric)
);

CREATE TABLE IF NOT EXISTS target_course_eligibility_projection (
    target_kind TEXT NOT NULL,
    target_id UUID NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
    target_version_id UUID NOT NULL REFERENCES segment_versions(id) ON DELETE CASCADE,
    matching_contract_key TEXT NOT NULL,
    matching_contract_version INTEGER NOT NULL,
    activity_trait_bucket_key TEXT NOT NULL REFERENCES activity_trait_projection(activity_trait_bucket_key) ON DELETE RESTRICT,
    policy_key TEXT NOT NULL,
    policy_version INTEGER NOT NULL,
    source_hash TEXT NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    compatibility_preset TEXT NOT NULL,
    activity_group TEXT NOT NULL,
    match_allowed BOOLEAN NOT NULL,
    completion_allowed BOOLEAN NOT NULL,
    rank_allowed BOOLEAN NOT NULL,
    leaderboard_partition_template JSONB NOT NULL,
    deny_reason_code TEXT,
    PRIMARY KEY (target_kind, target_id, target_version_id, activity_trait_bucket_key, policy_key, policy_version),
    CONSTRAINT target_course_eligibility_projection_target_kind_chk CHECK (target_kind = 'segment'),
    CONSTRAINT target_course_eligibility_projection_matching_contract_chk CHECK (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1),
    CONSTRAINT target_course_eligibility_projection_policy_chk CHECK (policy_key = 'course_eligibility' AND policy_version = 1),
    CONSTRAINT target_course_eligibility_projection_allow_chk CHECK (match_allowed OR completion_allowed OR rank_allowed),
    CONSTRAINT target_course_eligibility_projection_preset_chk CHECK (compatibility_preset IN ('standard', 'trail', 'lap', 'wintersport', 'wassersport', 'lift')),
    CONSTRAINT target_course_eligibility_projection_group_chk CHECK (activity_group IN ('bike', 'foot', 'swim', 'ski', 'paddle', 'generic'))
);

CREATE INDEX IF NOT EXISTS idx_tcep_candidate_lookup
    ON target_course_eligibility_projection (
        activity_trait_bucket_key,
        target_kind,
        target_id,
        target_version_id
    )
    WHERE match_allowed;

CREATE INDEX IF NOT EXISTS idx_tcep_target_lookup
    ON target_course_eligibility_projection (target_kind, target_id, target_version_id);

CREATE INDEX IF NOT EXISTS idx_tcep_filter_preset
    ON target_course_eligibility_projection (compatibility_preset, activity_trait_bucket_key)
    WHERE match_allowed;

ALTER TABLE segment_efforts
    ADD COLUMN IF NOT EXISTS activity_trait_bucket_key TEXT,
    ADD COLUMN IF NOT EXISTS eligibility_policy_key TEXT NOT NULL DEFAULT 'course_eligibility',
    ADD COLUMN IF NOT EXISTS eligibility_policy_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS eligibility_source_hash TEXT;

UPDATE segment_efforts se
SET activity_trait_bucket_key =
    'atb:v1:sport=' || lower(trim(COALESCE(NULLIF(se.sport_type, ''), 'generic'))) ||
    ':sub=' || lower(trim(COALESCE(NULLIF(NULLIF(se.sub_type, ''), 'generic'), '_'))) ||
    ':indoor=' || CASE WHEN COALESCE(a.is_indoor, false) THEN '1' ELSE '0' END ||
    ':virtual=' || CASE WHEN COALESCE(a.is_virtual, false) THEN '1' ELSE '0' END ||
    ':electric=' || CASE WHEN COALESCE(a.is_electric, false) THEN '1' ELSE '0' END,
    eligibility_policy_key = COALESCE(NULLIF(se.eligibility_policy_key, ''), 'course_eligibility'),
    eligibility_policy_version = COALESCE(se.eligibility_policy_version, 1)
FROM activities a
WHERE a.id = se.activity_id
  AND se.activity_trait_bucket_key IS NULL;
