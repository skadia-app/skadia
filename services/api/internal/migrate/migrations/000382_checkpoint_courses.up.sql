ALTER TABLE segments
    ADD COLUMN IF NOT EXISTS course_kind TEXT NOT NULL DEFAULT 'line';

ALTER TABLE segments
    DROP CONSTRAINT IF EXISTS segments_course_kind_chk;
ALTER TABLE segments
    ADD CONSTRAINT segments_course_kind_chk
    CHECK (course_kind IN ('line', 'checkpoint_sequence'));

CREATE INDEX IF NOT EXISTS idx_segments_public_line_order
    ON segments (created_at DESC, id DESC)
    WHERE visibility = 'public' AND course_kind = 'line';

CREATE INDEX IF NOT EXISTS idx_segments_owner_course_kind_order
    ON segments (created_by, course_kind, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_segments_shared_scope_course_kind_order
    ON segments (access_scope_id, course_kind, created_at DESC, id DESC)
    WHERE visibility = 'shared';

CREATE INDEX IF NOT EXISTS idx_segment_efforts_active_user_segment_counts
    ON segment_efforts (
        user_id,
        segment_id,
        started_at,
        elapsed_time_s,
        is_reverse,
        activity_id,
        created_at,
        id
    )
    WHERE superseded_at IS NULL;

ALTER TABLE segment_versions
    DROP CONSTRAINT IF EXISTS segment_versions_id_segment_id_unique;
ALTER TABLE segment_versions
    ADD CONSTRAINT segment_versions_id_segment_id_unique
    UNIQUE (id, segment_id);

ALTER TABLE segment_versions
    DROP CONSTRAINT IF EXISTS segment_versions_matching_contract_chk;
ALTER TABLE segment_versions
    ADD CONSTRAINT segment_versions_matching_contract_chk
    CHECK (
        (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1)
        OR (matching_contract_key = 'checkpoint_sequence_v1' AND matching_contract_version = 1)
    );

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_matching_contract_chk;
ALTER TABLE segment_efforts
    ADD CONSTRAINT segment_efforts_matching_contract_chk
    CHECK (
        (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1)
        OR (matching_contract_key = 'checkpoint_sequence_v1' AND matching_contract_version = 1)
    );

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_leaderboard_contract_chk;
ALTER TABLE segment_efforts
    ADD CONSTRAINT segment_efforts_leaderboard_contract_chk
    CHECK (
        (leaderboard_contract_key = 'course_gps_v1' AND leaderboard_contract_version = 1)
        OR (leaderboard_contract_key = 'checkpoint_sequence_v1' AND leaderboard_contract_version = 1)
    );

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_leaderboard_policy_chk;
ALTER TABLE segment_efforts
    ADD CONSTRAINT segment_efforts_leaderboard_policy_chk
    CHECK (leaderboard_version_policy_key IN ('legacy_segment_wide', 'checkpoint_sequence_current_version'));

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_id_started_version_unique;
ALTER TABLE segment_efforts
    ADD CONSTRAINT segment_efforts_id_started_version_unique
    UNIQUE (id, started_at, segment_version_id);

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_contract_bundle_chk;
ALTER TABLE segment_efforts
    ADD CONSTRAINT segment_efforts_contract_bundle_chk
    CHECK (
        (
            matching_contract_key = 'course_gps_v1'
            AND matching_contract_version = 1
            AND leaderboard_contract_key = 'course_gps_v1'
            AND leaderboard_contract_version = 1
            AND leaderboard_version_policy_key = 'legacy_segment_wide'
            AND leaderboard_direction IN ('forward', 'reverse')
        )
        OR (
            matching_contract_key = 'checkpoint_sequence_v1'
            AND matching_contract_version = 1
            AND leaderboard_contract_key = 'checkpoint_sequence_v1'
            AND leaderboard_contract_version = 1
            AND leaderboard_version_policy_key = 'checkpoint_sequence_current_version'
            AND leaderboard_direction = 'forward'
            AND is_reverse = false
        )
    );

ALTER TABLE target_course_eligibility_projection
    DROP CONSTRAINT IF EXISTS target_course_eligibility_projection_matching_contract_chk;
ALTER TABLE target_course_eligibility_projection
    ADD CONSTRAINT target_course_eligibility_projection_matching_contract_chk
    CHECK (
        (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1)
        OR (matching_contract_key = 'checkpoint_sequence_v1' AND matching_contract_version = 1)
    );

CREATE TABLE IF NOT EXISTS course_version_controls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    segment_id UUID NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
    segment_version_id UUID NOT NULL,
    sequence_index INTEGER NOT NULL,
    role TEXT NOT NULL,
    primitive TEXT NOT NULL,
    geometry_shape TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    geom GEOMETRY(Geometry, 4326) NOT NULL,
    hit_radius_m DOUBLE PRECISION,
    area_radius_m DOUBLE PRECISION,
    required BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT course_version_controls_role_chk
        CHECK (role IN ('start', 'checkpoint', 'finish')),
    CONSTRAINT course_version_controls_primitive_chk
        CHECK (primitive IN ('point', 'area')),
    CONSTRAINT course_version_controls_geometry_shape_chk
        CHECK (geometry_shape IN ('point', 'circle', 'polygon', 'multipolygon')),
    CONSTRAINT course_version_controls_geometry_contract_chk
        CHECK (
            (
                primitive = 'point'
                AND geometry_shape = 'point'
                AND GeometryType(geom) = 'POINT'
                AND hit_radius_m > 0
                AND hit_radius_m <= 5000
                AND area_radius_m IS NULL
            )
            OR (
                primitive = 'area'
                AND geometry_shape = 'circle'
                AND GeometryType(geom) = 'POINT'
                AND hit_radius_m IS NULL
                AND area_radius_m > 0
            )
            OR (
                primitive = 'area'
                AND geometry_shape = 'polygon'
                AND GeometryType(geom) = 'POLYGON'
                AND hit_radius_m IS NULL
                AND area_radius_m IS NULL
            )
            OR (
                primitive = 'area'
                AND geometry_shape = 'multipolygon'
                AND GeometryType(geom) = 'MULTIPOLYGON'
                AND hit_radius_m IS NULL
                AND area_radius_m IS NULL
            )
        ),
    CONSTRAINT course_version_controls_required_chk
        CHECK (required = true),
    CONSTRAINT course_version_controls_segment_version_fk
        FOREIGN KEY (segment_version_id, segment_id)
        REFERENCES segment_versions(id, segment_id) ON DELETE CASCADE,
    CONSTRAINT course_version_controls_id_version_unique
        UNIQUE (id, segment_version_id),
    CONSTRAINT course_version_controls_version_sequence_unique
        UNIQUE (segment_version_id, sequence_index)
);

CREATE INDEX IF NOT EXISTS idx_course_version_controls_segment_version
    ON course_version_controls(segment_version_id, sequence_index);

CREATE INDEX IF NOT EXISTS idx_course_version_controls_geom
    ON course_version_controls USING GIST(geom);

CREATE TABLE IF NOT EXISTS checkpoint_course_effort_controls (
    segment_effort_id UUID NOT NULL,
    segment_effort_started_at TIMESTAMPTZ NOT NULL,
    segment_version_id UUID NOT NULL,
    control_id UUID NOT NULL REFERENCES course_version_controls(id) ON DELETE CASCADE,
    sequence_index INTEGER NOT NULL,
    entered_at TIMESTAMPTZ NOT NULL,
    exited_at TIMESTAMPTZ,
    activity_stream_index INTEGER NOT NULL,
    nearest_distance_m DOUBLE PRECISION,
    evidence_hash TEXT NOT NULL,
    PRIMARY KEY (segment_effort_id, segment_effort_started_at, sequence_index),
    CONSTRAINT checkpoint_course_effort_controls_effort_fk
        FOREIGN KEY (segment_effort_id, segment_effort_started_at, segment_version_id)
        REFERENCES segment_efforts(id, started_at, segment_version_id) ON DELETE CASCADE,
    CONSTRAINT checkpoint_course_effort_controls_control_fk
        FOREIGN KEY (control_id, segment_version_id)
        REFERENCES course_version_controls(id, segment_version_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS checkpoint_course_projection (
    segment_id UUID PRIMARY KEY REFERENCES segments(id) ON DELETE CASCADE,
    active_segment_version_id UUID NOT NULL,
    control_count INTEGER NOT NULL,
    required_control_count INTEGER NOT NULL,
    max_hit_radius_m DOUBLE PRECISION NOT NULL DEFAULT 0,
    display_bounds GEOMETRY(Geometry, 4326) NOT NULL,
    control_summary JSONB NOT NULL,
    control_map_features JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT checkpoint_course_projection_active_version_fk
        FOREIGN KEY (active_segment_version_id, segment_id)
        REFERENCES segment_versions(id, segment_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_checkpoint_course_projection_bounds
    ON checkpoint_course_projection USING GIST(display_bounds);
