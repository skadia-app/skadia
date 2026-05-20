CREATE TABLE object_access_scopes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_kind TEXT NOT NULL DEFAULT 'user',
    owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    owner_ref TEXT,
    scope_kind TEXT NOT NULL,
    label TEXT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CHECK (scope_kind IN (
        'manual',
        'relationship_rule',
        'link',
        'mixed',
        'system'
    )),
    CHECK (owner_kind IN ('user', 'group', 'organization', 'system')),
    CHECK (
        (owner_kind = 'user' AND owner_user_id IS NOT NULL AND owner_ref IS NULL)
        OR (owner_kind <> 'user' AND owner_user_id IS NULL AND owner_ref IS NOT NULL)
    ),
    CHECK (revoked_at IS NULL OR revoked_at > created_at)
);

CREATE TABLE object_access_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    access_scope_id UUID NOT NULL REFERENCES object_access_scopes(id) ON DELETE CASCADE,
    principal_kind TEXT NOT NULL,
    principal_user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    principal_ref TEXT,
    principal_selector JSONB NOT NULL DEFAULT '{}'::jsonb,
    principal_selector_hash TEXT NOT NULL DEFAULT '',
    link_token_hash TEXT,
    permission TEXT NOT NULL DEFAULT 'view',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CHECK (principal_kind IN (
        'user',
        'group',
        'relationship_rule',
        'link_token',
        'team',
        'organization',
        'role',
        'server',
        'delegated_audience'
    )),
    CHECK (permission = 'view'),
    CHECK (
        (principal_kind = 'user' AND principal_user_id IS NOT NULL AND principal_ref IS NULL AND link_token_hash IS NULL)
        OR (principal_kind = 'link_token' AND link_token_hash IS NOT NULL AND principal_user_id IS NULL AND principal_ref IS NULL)
        OR (principal_kind <> 'user' AND principal_kind <> 'link_token' AND principal_ref IS NOT NULL AND principal_user_id IS NULL AND link_token_hash IS NULL)
    ),
    CHECK (revoked_at IS NULL OR revoked_at > created_at)
);

CREATE UNIQUE INDEX uq_object_access_grants_active_user
    ON object_access_grants (access_scope_id, principal_user_id, permission)
    WHERE principal_kind = 'user' AND revoked_at IS NULL;

CREATE UNIQUE INDEX uq_object_access_grants_active_ref
    ON object_access_grants (access_scope_id, principal_kind, principal_ref, principal_selector_hash, permission)
    WHERE principal_kind <> 'user' AND principal_kind <> 'link_token' AND revoked_at IS NULL;

CREATE UNIQUE INDEX uq_object_access_grants_active_link
    ON object_access_grants (access_scope_id, link_token_hash, permission)
    WHERE principal_kind = 'link_token' AND revoked_at IS NULL;

CREATE INDEX idx_object_access_grants_user_lookup
    ON object_access_grants (principal_user_id, permission, access_scope_id)
    WHERE principal_kind = 'user' AND revoked_at IS NULL;

CREATE INDEX idx_object_access_grants_ref_lookup
    ON object_access_grants (principal_kind, principal_ref, principal_selector_hash, permission, access_scope_id)
    WHERE principal_kind <> 'user' AND principal_kind <> 'link_token' AND revoked_at IS NULL;

ALTER TABLE segments
    ADD COLUMN IF NOT EXISTS access_scope_id UUID REFERENCES object_access_scopes(id) ON DELETE SET NULL;

ALTER TABLE segments
    DROP CONSTRAINT IF EXISTS segments_visibility_check;

ALTER TABLE segments
    ADD CONSTRAINT segments_visibility_check
    CHECK (visibility IN ('private', 'shared', 'public'));

ALTER TABLE segments
    ADD CONSTRAINT segments_access_scope_visibility_check
    CHECK (
        (visibility = 'shared' AND access_scope_id IS NOT NULL)
        OR (visibility IN ('private', 'public') AND access_scope_id IS NULL)
    );

CREATE INDEX idx_segments_shared_access_scope
    ON segments (access_scope_id, created_at DESC)
    WHERE visibility = 'shared';

CREATE TABLE planned_route_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id UUID REFERENCES planned_routes(id) ON DELETE SET NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    source_route_version BIGINT NOT NULL,
    snapshot_schema_key TEXT NOT NULL DEFAULT 'planned_route_snapshot',
    snapshot_schema_version INTEGER NOT NULL DEFAULT 1,
    route_name TEXT NOT NULL,
    route_description TEXT,
    sport_type TEXT NOT NULL,
    sub_type TEXT,
    is_electric BOOLEAN NOT NULL DEFAULT false,
    is_indoor BOOLEAN NOT NULL DEFAULT false,
    is_virtual BOOLEAN NOT NULL DEFAULT false,
    is_commute BOOLEAN NOT NULL DEFAULT false,
    profile TEXT NOT NULL,
    routing_engine TEXT,
    route_schema_version SMALLINT NOT NULL,
    geometry GEOMETRY(LineString, 4326) NOT NULL,
    waypoints JSONB NOT NULL,
    segments JSONB NOT NULL,
    elevation_profile JSONB,
    enrichment JSONB,
    total_distance_m DOUBLE PRECISION,
    total_elevation_gain DOUBLE PRECISION,
    total_elevation_loss DOUBLE PRECISION,
    estimated_time_min INTEGER,
    geometry_hash TEXT NOT NULL,
    snapshot_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_route_deleted_at TIMESTAMPTZ,
    UNIQUE (route_id, source_route_version, snapshot_hash),
    CHECK (snapshot_schema_key = 'planned_route_snapshot'),
    CHECK (snapshot_schema_version = 1),
    CHECK (routing_engine IS NULL OR routing_engine IN ('badrouter', 'brouter'))
);

CREATE INDEX idx_planned_route_snapshots_route
    ON planned_route_snapshots (route_id, created_at DESC);

CREATE INDEX idx_planned_route_snapshots_user
    ON planned_route_snapshots (user_id, created_at DESC);

CREATE INDEX idx_planned_route_snapshots_geom
    ON planned_route_snapshots USING GIST (geometry);

CREATE TABLE route_publication_privacy_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_snapshot_id UUID NOT NULL REFERENCES planned_route_snapshots(id) ON DELETE CASCADE,
    snapshot_hash TEXT NOT NULL,
    audience_class TEXT NOT NULL,
    route_name_state TEXT NOT NULL DEFAULT 'unknown',
    description_state TEXT NOT NULL DEFAULT 'unknown',
    waypoints_state TEXT NOT NULL DEFAULT 'unknown',
    source_metadata_state TEXT NOT NULL DEFAULT 'unknown',
    geometry_state TEXT NOT NULL DEFAULT 'unknown',
    enrichment_state TEXT NOT NULL DEFAULT 'unknown',
    media_pointer_state TEXT NOT NULL DEFAULT 'unknown',
    overall_state TEXT NOT NULL DEFAULT 'unknown',
    owner_approved_at TIMESTAMPTZ,
    reviewer_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    review_hash TEXT NOT NULL,
    redaction_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (route_snapshot_id, snapshot_hash, audience_class),
    CHECK (audience_class IN ('public', 'shared')),
    CHECK (route_name_state IN ('unknown', 'safe', 'redacted', 'blocked', 'needs_owner_review')),
    CHECK (description_state IN ('unknown', 'safe', 'redacted', 'blocked', 'needs_owner_review')),
    CHECK (waypoints_state IN ('unknown', 'safe', 'redacted', 'blocked', 'needs_owner_review')),
    CHECK (source_metadata_state IN ('unknown', 'safe', 'redacted', 'blocked', 'needs_owner_review')),
    CHECK (geometry_state IN ('unknown', 'safe', 'redacted', 'blocked', 'needs_owner_review')),
    CHECK (enrichment_state IN ('unknown', 'safe', 'redacted', 'blocked', 'needs_owner_review')),
    CHECK (media_pointer_state IN ('unknown', 'safe', 'redacted', 'blocked', 'needs_owner_review')),
    CHECK (overall_state IN ('unknown', 'safe', 'redacted', 'blocked', 'needs_owner_review'))
);

CREATE INDEX idx_route_publication_privacy_reviews_snapshot
    ON route_publication_privacy_reviews (route_snapshot_id, audience_class, overall_state);

CREATE TABLE route_course_publications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id UUID REFERENCES planned_routes(id) ON DELETE SET NULL,
    route_snapshot_id UUID NOT NULL REFERENCES planned_route_snapshots(id) ON DELETE RESTRICT,
    course_segment_id UUID NOT NULL REFERENCES segments(id) ON DELETE RESTRICT,
    course_segment_version_id UUID NOT NULL REFERENCES segment_versions(id) ON DELETE RESTRICT,
    publication_state TEXT NOT NULL DEFAULT 'published',
    publish_policy TEXT NOT NULL,
    equivalence_decision TEXT NOT NULL,
    visibility_snapshot TEXT NOT NULL DEFAULT 'private',
    access_scope_id_snapshot UUID REFERENCES object_access_scopes(id) ON DELETE SET NULL,
    target_publication_id UUID REFERENCES route_course_publications(id) ON DELETE SET NULL,
    target_course_segment_id UUID REFERENCES segments(id) ON DELETE SET NULL,
    target_course_segment_version_id UUID REFERENCES segment_versions(id) ON DELETE SET NULL,
    compatibility_evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    operation_id TEXT NOT NULL,
    operation_input_hash TEXT NOT NULL,
    observed_route_version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retired_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (route_snapshot_id, course_segment_id, course_segment_version_id),
    UNIQUE (actor_user_id, operation_id),
    CHECK (publication_state IN ('draft_publication', 'published', 'retired')),
    CHECK (publish_policy IN (
        'new_version_same_course',
        'continue_equivalence_group',
        'new_equivalence_group',
        'new_course'
    )),
    CHECK (equivalence_decision IN ('continue_group', 'new_group', 'version_specific')),
    CHECK (visibility_snapshot IN ('private', 'shared', 'public')),
    CHECK (
        (visibility_snapshot = 'shared' AND access_scope_id_snapshot IS NOT NULL)
        OR (visibility_snapshot IN ('private', 'public') AND access_scope_id_snapshot IS NULL)
    ),
    CHECK (
        (publish_policy IN ('new_version_same_course', 'continue_equivalence_group', 'new_equivalence_group')
            AND target_course_segment_id IS NOT NULL)
        OR (publish_policy = 'new_course'
            AND target_publication_id IS NULL
            AND target_course_segment_id IS NULL
            AND target_course_segment_version_id IS NULL)
    ),
    CHECK (
        publish_policy <> 'continue_equivalence_group'
        OR (compatibility_evidence <> '{}'::jsonb AND equivalence_decision = 'continue_group')
    ),
    CHECK (retired_at IS NULL OR retired_at > created_at)
);

CREATE INDEX idx_route_course_publications_route
    ON route_course_publications (route_id, created_at DESC);

CREATE INDEX idx_route_course_publications_course
    ON route_course_publications (course_segment_id, course_segment_version_id);

CREATE INDEX idx_route_course_publications_public
    ON route_course_publications (visibility_snapshot, publication_state, created_at DESC)
    WHERE visibility_snapshot = 'public' AND publication_state = 'published';

CREATE INDEX idx_route_course_publications_access_scope
    ON route_course_publications (access_scope_id_snapshot, publication_state)
    WHERE visibility_snapshot = 'shared' AND publication_state = 'published';

CREATE TABLE route_relationships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_route_id UUID NOT NULL REFERENCES planned_routes(id) ON DELETE CASCADE,
    target_route_id UUID NOT NULL REFERENCES planned_routes(id) ON DELETE CASCADE,
    relation_type TEXT NOT NULL,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    source_snapshot_id UUID REFERENCES planned_route_snapshots(id) ON DELETE SET NULL,
    target_snapshot_id UUID REFERENCES planned_route_snapshots(id) ON DELETE SET NULL,
    operation_id TEXT NOT NULL,
    operation_input_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CHECK (source_route_id <> target_route_id),
    CHECK (relation_type IN ('fork_of', 'variant_of', 'derived_from'))
);

CREATE UNIQUE INDEX uq_route_relationships_active
    ON route_relationships (source_route_id, target_route_id, relation_type);

CREATE INDEX idx_route_relationships_target
    ON route_relationships (target_route_id, relation_type);

CREATE UNIQUE INDEX uq_route_relationships_operation
    ON route_relationships (actor_user_id, operation_id);

CREATE TABLE route_course_relationships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_publication_id UUID REFERENCES route_course_publications(id) ON DELETE SET NULL,
    target_publication_id UUID REFERENCES route_course_publications(id) ON DELETE SET NULL,
    source_course_segment_id UUID NOT NULL REFERENCES segments(id) ON DELETE RESTRICT,
    source_course_segment_version_id UUID REFERENCES segment_versions(id) ON DELETE SET NULL,
    target_course_segment_id UUID NOT NULL REFERENCES segments(id) ON DELETE RESTRICT,
    target_course_segment_version_id UUID REFERENCES segment_versions(id) ON DELETE SET NULL,
    relation_type TEXT NOT NULL,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    operation_id TEXT NOT NULL,
    operation_input_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CHECK (source_course_segment_id <> target_course_segment_id OR source_course_segment_version_id IS DISTINCT FROM target_course_segment_version_id),
    CHECK (relation_type IN ('variant_of', 'derived_from'))
);

CREATE UNIQUE INDEX uq_route_course_relationships_active
    ON route_course_relationships (
        source_course_segment_id,
        COALESCE(source_course_segment_version_id, '00000000-0000-0000-0000-000000000000'::uuid),
        target_course_segment_id,
        COALESCE(target_course_segment_version_id, '00000000-0000-0000-0000-000000000000'::uuid),
        relation_type
    );

CREATE UNIQUE INDEX uq_route_course_relationships_operation
    ON route_course_relationships (actor_user_id, operation_id);

CREATE INDEX idx_route_course_relationships_target
    ON route_course_relationships (target_course_segment_id, relation_type);

CREATE TABLE route_profile_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_snapshot_id UUID NOT NULL REFERENCES planned_route_snapshots(id) ON DELETE CASCADE,
    profile_schema_key TEXT NOT NULL DEFAULT 'route_profile_v1',
    profile_schema_version INTEGER NOT NULL DEFAULT 1,
    source_kind TEXT NOT NULL,
    source_version TEXT NOT NULL,
    source_hash TEXT NOT NULL,
    confidence TEXT NOT NULL DEFAULT 'medium',
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    distance_m DOUBLE PRECISION,
    elevation_gain_m DOUBLE PRECISION,
    elevation_loss_m DOUBLE PRECISION,
    forest_pct DOUBLE PRECISION,
    protected_pct DOUBLE PRECISION,
    surface_distribution JSONB NOT NULL DEFAULT '{}'::jsonb,
    way_distribution JSONB NOT NULL DEFAULT '{}'::jsonb,
    difficulty_scales JSONB NOT NULL DEFAULT '{}'::jsonb,
    context_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    ranges JSONB NOT NULL DEFAULT '{}'::jsonb,
    CHECK (profile_schema_key = 'route_profile_v1'),
    CHECK (profile_schema_version = 1),
    CHECK (source_kind IN ('planner_enrichment', 'imported', 'manual_override', 'admin_override')),
    CHECK (confidence IN ('low', 'medium', 'high')),
    CHECK (forest_pct IS NULL OR (forest_pct >= 0 AND forest_pct <= 1)),
    CHECK (protected_pct IS NULL OR (protected_pct >= 0 AND protected_pct <= 1))
);

CREATE UNIQUE INDEX uq_route_profile_snapshot_route
    ON route_profile_snapshots (route_snapshot_id, profile_schema_key, profile_schema_version);

CREATE INDEX idx_route_profile_snapshots_source_hash
    ON route_profile_snapshots (source_hash);

CREATE TABLE route_course_profile_projection (
    subject_kind TEXT NOT NULL,
    subject_id UUID NOT NULL,
    subject_version_id UUID,
    subject_version_key UUID NOT NULL,
    publication_id UUID REFERENCES route_course_publications(id) ON DELETE CASCADE,
    route_snapshot_id UUID REFERENCES planned_route_snapshots(id) ON DELETE CASCADE,
    profile_snapshot_id UUID NOT NULL REFERENCES route_profile_snapshots(id) ON DELETE CASCADE,
    visibility TEXT NOT NULL,
    access_scope_id UUID REFERENCES object_access_scopes(id) ON DELETE SET NULL,
    sport_type TEXT NOT NULL,
    sub_type TEXT,
    is_electric BOOLEAN NOT NULL DEFAULT false,
    is_indoor BOOLEAN NOT NULL DEFAULT false,
    is_virtual BOOLEAN NOT NULL DEFAULT false,
    is_commute BOOLEAN NOT NULL DEFAULT false,
    distance_m DOUBLE PRECISION,
    elevation_gain_m DOUBLE PRECISION,
    elevation_loss_m DOUBLE PRECISION,
    sac_max_rank INTEGER,
    sac_dominant_rank INTEGER,
    sac_weighted_mean_rank DOUBLE PRECISION,
    mtb_max_rank INTEGER,
    mtb_dominant_rank INTEGER,
    mtb_weighted_mean_rank DOUBLE PRECISION,
    via_ferrata_max_rank INTEGER,
    via_ferrata_dominant_rank INTEGER,
    via_ferrata_weighted_mean_rank DOUBLE PRECISION,
    forest_pct DOUBLE PRECISION,
    protected_pct DOUBLE PRECISION,
    primary_surface TEXT,
    primary_way_type TEXT,
    source_hash TEXT NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (subject_kind, subject_id, subject_version_key),
    CHECK (subject_kind IN ('planned_route_snapshot', 'published_course')),
    CHECK (subject_version_key = COALESCE(subject_version_id, '00000000-0000-0000-0000-000000000000'::uuid)),
    CHECK (visibility IN ('private', 'shared', 'public')),
    CHECK (
        (visibility = 'shared' AND access_scope_id IS NOT NULL)
        OR (visibility IN ('private', 'public') AND access_scope_id IS NULL)
    )
);

CREATE INDEX idx_rcpp_public_search
    ON route_course_profile_projection (
        visibility,
        sport_type,
        sub_type,
        distance_m,
        elevation_gain_m
    );

CREATE INDEX idx_rcpp_difficulty_search
    ON route_course_profile_projection (
        visibility,
        sac_max_rank,
        mtb_max_rank,
        via_ferrata_max_rank
    );

CREATE INDEX idx_rcpp_course_subject
    ON route_course_profile_projection (subject_kind, subject_id, subject_version_id);

CREATE INDEX idx_rcpp_shared_access_scope
    ON route_course_profile_projection (access_scope_id, sport_type, sub_type)
    WHERE visibility = 'shared';

CREATE TABLE route_publication_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    operation TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    operation_input_hash TEXT NOT NULL,
    subject_kind TEXT NOT NULL,
    subject_id UUID NOT NULL,
    before_snapshot_hash TEXT,
    after_snapshot_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CHECK (operation IN ('snapshot', 'publish_course', 'republish_course', 'fork_route', 'relate_route', 'relate_course')),
    CHECK (subject_kind IN ('planned_route', 'planned_route_snapshot', 'route_course_publication', 'route_relationship', 'route_course_relationship'))
);

CREATE UNIQUE INDEX uq_route_publication_events_operation
    ON route_publication_events (actor_user_id, operation_id, operation);

CREATE INDEX idx_route_publication_events_subject
    ON route_publication_events (subject_kind, subject_id, created_at DESC);
