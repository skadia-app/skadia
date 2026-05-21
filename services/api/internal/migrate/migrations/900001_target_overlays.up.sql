CREATE TABLE IF NOT EXISTS target_activity_observations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    overlay_subject_kind TEXT NOT NULL,
    subject_id UUID NOT NULL,
    target_version_id UUID NULL,
    activity_id UUID NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    observation_type TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_occurrence_key TEXT NOT NULL,
    source_match_id UUID NULL,
    source_effort_id UUID NULL,
    source_effort_started_at TIMESTAMPTZ NULL,
    source_effort_segment_version_id UUID NULL,
    activity_tuple JSONB NOT NULL DEFAULT '{}'::jsonb,
    stats JSONB NOT NULL DEFAULT '{}'::jsonb,
    visibility TEXT NOT NULL DEFAULT 'private',
    access_scope_id UUID NULL REFERENCES object_access_scopes(id) ON DELETE SET NULL,
    visibility_snapshot TEXT NOT NULL DEFAULT 'private',
    privacy_policy_key TEXT NOT NULL DEFAULT 'live_source_activity_v1',
    authorization_mode TEXT NOT NULL DEFAULT 'live_source_activity',
    redaction_policy_key TEXT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    superseded_at TIMESTAMPTZ NULL,
    CONSTRAINT target_activity_observations_subject_kind_check
        CHECK (overlay_subject_kind IN ('course', 'route_publication', 'place', 'region', 'collection')),
    CONSTRAINT target_activity_observations_observation_type_check
        CHECK (observation_type IN ('matched_traversal', 'visit', 'dwell', 'check_in', 'completion')),
    CONSTRAINT target_activity_observations_visibility_check
        CHECK (visibility IN ('public', 'private', 'shared')),
    CONSTRAINT target_activity_observations_visibility_scope_check
        CHECK ((visibility = 'shared' AND access_scope_id IS NOT NULL) OR (visibility IN ('public', 'private') AND access_scope_id IS NULL)),
    CONSTRAINT target_activity_observations_visibility_snapshot_check
        CHECK (visibility_snapshot IN ('public', 'private', 'shared')),
    CONSTRAINT target_activity_observations_authorization_mode_check
        CHECK (authorization_mode IN ('live_source_activity', 'owner', 'system'))
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_target_activity_observations_active_source
    ON target_activity_observations (
        overlay_subject_kind,
        subject_id,
        target_version_id,
        source_type,
        source_id,
        source_occurrence_key,
        observation_type
    ) NULLS NOT DISTINCT
    WHERE superseded_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_target_activity_observations_subject
    ON target_activity_observations (overlay_subject_kind, subject_id, target_version_id, occurred_at DESC)
    WHERE superseded_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_target_activity_observations_user
    ON target_activity_observations (user_id, occurred_at DESC)
    WHERE user_id IS NOT NULL AND superseded_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_target_activity_observations_activity
    ON target_activity_observations (activity_id, occurred_at DESC)
    WHERE activity_id IS NOT NULL AND superseded_at IS NULL;

CREATE TABLE IF NOT EXISTS target_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    overlay_subject_kind TEXT NOT NULL,
    subject_id UUID NOT NULL,
    target_version_id UUID NULL,
    author_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    report_type TEXT NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'private',
    access_scope_id UUID NULL REFERENCES object_access_scopes(id) ON DELETE SET NULL,
    attribution_mode TEXT NOT NULL DEFAULT 'named',
    applies_to JSONB NOT NULL DEFAULT '{}'::jsonb,
    condition_state TEXT NULL,
    condition_severity INTEGER NULL,
    description TEXT NOT NULL DEFAULT '',
    source_activity_id UUID NULL REFERENCES activities(id) ON DELETE SET NULL,
    source_policy_key TEXT NOT NULL DEFAULT 'none',
    media_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    observed_at TIMESTAMPTZ NOT NULL,
    valid_from TIMESTAMPTZ NULL,
    valid_until TIMESTAMPTZ NULL,
    moderation_state TEXT NOT NULL DEFAULT 'visible',
    resolver_effect TEXT NOT NULL DEFAULT 'none',
    redaction_policy_key TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT target_reports_subject_kind_check
        CHECK (overlay_subject_kind IN ('course', 'route_publication', 'place', 'region', 'collection')),
    CONSTRAINT target_reports_report_type_check
        CHECK (report_type IN ('condition', 'issue', 'warning', 'trail_work', 'closure', 'metadata')),
    CONSTRAINT target_reports_visibility_check
        CHECK (visibility IN ('public', 'private', 'shared')),
    CONSTRAINT target_reports_visibility_scope_check
        CHECK ((visibility = 'shared' AND access_scope_id IS NOT NULL) OR (visibility IN ('public', 'private') AND access_scope_id IS NULL)),
    CONSTRAINT target_reports_attribution_mode_check
        CHECK (attribution_mode IN ('named', 'anonymous_public', 'moderator_only', 'hidden')),
    CONSTRAINT target_reports_moderation_state_check
        CHECK (moderation_state IN ('visible', 'hidden', 'redacted', 'rejected')),
    CONSTRAINT target_reports_valid_range_check
        CHECK (valid_from IS NULL OR valid_until IS NULL OR valid_from <= valid_until)
);

CREATE INDEX IF NOT EXISTS idx_target_reports_subject
    ON target_reports (overlay_subject_kind, subject_id, target_version_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_target_reports_open_subject
    ON target_reports (overlay_subject_kind, subject_id, report_type, observed_at DESC)
    WHERE moderation_state = 'visible';
CREATE INDEX IF NOT EXISTS idx_target_reports_author
    ON target_reports (author_user_id, created_at DESC)
    WHERE author_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_target_reports_source_activity
    ON target_reports (source_activity_id, observed_at DESC)
    WHERE source_activity_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS target_report_status_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id UUID NOT NULL REFERENCES target_reports(id) ON DELETE CASCADE,
    actor_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    status TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT target_report_status_events_status_check
        CHECK (status IN ('open', 'resolved', 'rejected', 'superseded', 'informational', 'redacted'))
);

CREATE INDEX IF NOT EXISTS idx_target_report_status_events_report
    ON target_report_status_events (report_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_target_report_status_events_actor
    ON target_report_status_events (actor_user_id, created_at DESC)
    WHERE actor_user_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS target_user_content (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    overlay_subject_kind TEXT NOT NULL,
    subject_id UUID NOT NULL,
    target_version_id UUID NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_kind TEXT NOT NULL,
    activity_id UUID NULL REFERENCES activities(id) ON DELETE SET NULL,
    activity_tuple JSONB NOT NULL DEFAULT '{}'::jsonb,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    visibility TEXT NOT NULL DEFAULT 'private',
    access_scope_id UUID NULL REFERENCES object_access_scopes(id) ON DELETE SET NULL,
    moderation_state TEXT NOT NULL DEFAULT 'visible',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT target_user_content_subject_kind_check
        CHECK (overlay_subject_kind IN ('course', 'route_publication', 'place', 'region', 'collection')),
    CONSTRAINT target_user_content_kind_check
        CHECK (content_kind IN ('comment', 'rating', 'save', 'completion', 'check_in')),
    CONSTRAINT target_user_content_visibility_check
        CHECK (visibility IN ('public', 'private', 'shared')),
    CONSTRAINT target_user_content_visibility_scope_check
        CHECK ((visibility = 'shared' AND access_scope_id IS NOT NULL) OR (visibility IN ('public', 'private') AND access_scope_id IS NULL)),
    CONSTRAINT target_user_content_moderation_state_check
        CHECK (moderation_state IN ('visible', 'hidden', 'redacted', 'rejected'))
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_target_user_content_active_save
    ON target_user_content (overlay_subject_kind, subject_id, target_version_id, user_id)
    NULLS NOT DISTINCT
    WHERE content_kind = 'save' AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_target_user_content_active_rating
    ON target_user_content (overlay_subject_kind, subject_id, target_version_id, user_id)
    NULLS NOT DISTINCT
    WHERE content_kind = 'rating' AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_target_user_content_subject
    ON target_user_content (overlay_subject_kind, subject_id, target_version_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_target_user_content_user
    ON target_user_content (user_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_target_user_content_activity
    ON target_user_content (activity_id, created_at DESC)
    WHERE activity_id IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS target_overlay_summary_projection (
    overlay_subject_kind TEXT NOT NULL,
    subject_id UUID NOT NULL,
    target_version_id UUID NULL,
    visibility_scope TEXT NOT NULL DEFAULT 'public',
    public_open_report_count INTEGER NOT NULL DEFAULT 0,
    latest_report_at TIMESTAMPTZ NULL,
    current_condition_state TEXT NULL,
    current_condition_severity INTEGER NULL,
    current_condition_observed_at TIMESTAMPTZ NULL,
    public_recent_observation_count_30d INTEGER NOT NULL DEFAULT 0,
    last_observation_at TIMESTAMPTZ NULL,
    public_comment_count INTEGER NOT NULL DEFAULT 0,
    public_rating_count INTEGER NOT NULL DEFAULT 0,
    public_rating_mean DOUBLE PRECISION NULL,
    public_completion_count INTEGER NOT NULL DEFAULT 0,
    public_check_in_count INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_hash TEXT NOT NULL DEFAULT '',
    CONSTRAINT target_overlay_summary_subject_kind_check
        CHECK (overlay_subject_kind IN ('course', 'route_publication', 'place', 'region', 'collection')),
    CONSTRAINT target_overlay_summary_visibility_scope_check
        CHECK (visibility_scope = 'public'),
    CONSTRAINT target_overlay_summary_counts_check
        CHECK (
            public_open_report_count >= 0
            AND public_recent_observation_count_30d >= 0
            AND public_comment_count >= 0
            AND public_rating_count >= 0
            AND public_completion_count >= 0
            AND public_check_in_count >= 0
        )
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_target_overlay_summary_subject
    ON target_overlay_summary_projection (overlay_subject_kind, subject_id, target_version_id, visibility_scope)
    NULLS NOT DISTINCT;
CREATE INDEX IF NOT EXISTS idx_target_overlay_summary_subject
    ON target_overlay_summary_projection (overlay_subject_kind, subject_id, target_version_id);
CREATE INDEX IF NOT EXISTS idx_target_overlay_summary_updated
    ON target_overlay_summary_projection (updated_at DESC);
