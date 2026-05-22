CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE segments
    ADD COLUMN IF NOT EXISTS elevation_kind TEXT;

ALTER TABLE segment_versions
    ADD COLUMN IF NOT EXISTS matching_contract_key TEXT NOT NULL DEFAULT 'course_gps_v1',
    ADD COLUMN IF NOT EXISTS matching_contract_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS valid_from_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS valid_until_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS validity_basis TEXT NOT NULL DEFAULT 'unknown_start',
    ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS eligibility_segment_type TEXT NOT NULL DEFAULT 'standard',
    ADD COLUMN IF NOT EXISTS eligibility_sport_group TEXT NOT NULL DEFAULT 'generic',
    ADD COLUMN IF NOT EXISTS eligibility_allowed_predicates JSONB,
    ADD COLUMN IF NOT EXISTS eligibility_allowed_predicates_set BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS eligibility_bidirectional BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS eligibility_visibility TEXT NOT NULL DEFAULT 'public',
    ADD COLUMN IF NOT EXISTS eligibility_elevation_kind TEXT NOT NULL DEFAULT 'flat';

ALTER TABLE segment_efforts
    ADD COLUMN IF NOT EXISTS matching_contract_key TEXT NOT NULL DEFAULT 'course_gps_v1',
    ADD COLUMN IF NOT EXISTS matching_contract_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS match_source_type TEXT NOT NULL DEFAULT 'platform_activity',
    ADD COLUMN IF NOT EXISTS match_source_id TEXT,
    ADD COLUMN IF NOT EXISTS match_identity_key TEXT,
    ADD COLUMN IF NOT EXISTS occurrence_index INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS occurrence_hash TEXT,
    ADD COLUMN IF NOT EXISTS superseded_by_effort_id UUID,
    ADD COLUMN IF NOT EXISTS leaderboard_sport_type TEXT NOT NULL DEFAULT 'generic',
    ADD COLUMN IF NOT EXISTS leaderboard_sub_type TEXT NOT NULL DEFAULT 'generic',
    ADD COLUMN IF NOT EXISTS leaderboard_equipment_class TEXT NOT NULL DEFAULT 'unspecified',
    ADD COLUMN IF NOT EXISTS leaderboard_assist_class TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS leaderboard_indoor_policy_state TEXT NOT NULL DEFAULT 'outdoor',
    ADD COLUMN IF NOT EXISTS leaderboard_virtual_policy_state TEXT NOT NULL DEFAULT 'real',
    ADD COLUMN IF NOT EXISTS leaderboard_electric_policy_state TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS leaderboard_direction TEXT NOT NULL DEFAULT 'forward',
    ADD COLUMN IF NOT EXISTS leaderboard_version_policy_key TEXT NOT NULL DEFAULT 'legacy_segment_wide',
    ADD COLUMN IF NOT EXISTS leaderboard_contract_key TEXT NOT NULL DEFAULT 'course_gps_v1',
    ADD COLUMN IF NOT EXISTS leaderboard_contract_version INTEGER NOT NULL DEFAULT 1;

UPDATE segments
SET terrain_type = 'lift',
    climb_type = 'bergfahrt'
WHERE segment_type = 'lift'
  AND (
      terrain_type IS DISTINCT FROM 'lift'
      OR climb_type IS DISTINCT FROM 'bergfahrt'
  );

UPDATE segments
SET elevation_kind = CASE
    WHEN segment_type = 'lap' THEN 'flat'
    WHEN segment_type = 'lift' THEN 'uphill'
    WHEN climb_type IN ('bergauf', 'bergfahrt') THEN 'uphill'
    WHEN climb_type IN ('bergab', 'talfahrt') THEN 'downhill'
    ELSE 'flat'
END
WHERE elevation_kind IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segments_elevation_kind_chk') THEN
        ALTER TABLE segments
            ADD CONSTRAINT segments_elevation_kind_chk
            CHECK (elevation_kind IN ('flat', 'hilly', 'uphill', 'downhill'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segments_lap_elevation_kind_chk') THEN
        ALTER TABLE segments
            ADD CONSTRAINT segments_lap_elevation_kind_chk
            CHECK (segment_type <> 'lap' OR elevation_kind = 'flat');
    END IF;
END
$$;

CREATE OR REPLACE FUNCTION normalize_segment_elevation_kind()
RETURNS trigger
LANGUAGE plpgsql
AS $$
	BEGIN
	    IF NEW.segment_type = 'lift' THEN
	        NEW.terrain_type := 'lift';
	        IF NEW.climb_type IS NULL OR NEW.climb_type = '' OR NEW.climb_type = 'standard' THEN
	            NEW.climb_type := 'bergfahrt';
	        END IF;
	        IF NEW.elevation_kind IS NULL THEN
	            NEW.elevation_kind := CASE
	                WHEN NEW.climb_type IN ('bergab', 'talfahrt') THEN 'downhill'
	                ELSE 'uphill'
	            END;
	        END IF;
	        RETURN NEW;
	    END IF;

	    IF NEW.segment_type = 'lap' THEN
	        NEW.elevation_kind := 'flat';
	        NEW.climb_type := 'standard';
        RETURN NEW;
    END IF;

    IF TG_OP = 'INSERT' THEN
        IF NEW.elevation_kind IS NULL THEN
            NEW.elevation_kind := CASE
                WHEN NEW.climb_type IN ('bergauf', 'bergfahrt') THEN 'uphill'
                WHEN NEW.climb_type IN ('bergab', 'talfahrt') THEN 'downhill'
                ELSE 'flat'
            END;
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.elevation_kind IS NULL THEN
        NEW.elevation_kind := COALESCE(OLD.elevation_kind, CASE
            WHEN NEW.climb_type IN ('bergauf', 'bergfahrt') THEN 'uphill'
            WHEN NEW.climb_type IN ('bergab', 'talfahrt') THEN 'downhill'
            ELSE 'flat'
        END);
    ELSIF NEW.climb_type IS DISTINCT FROM OLD.climb_type
       AND NEW.elevation_kind IS NOT DISTINCT FROM OLD.elevation_kind THEN
        NEW.elevation_kind := CASE
            WHEN NEW.climb_type IN ('bergauf', 'bergfahrt') THEN 'uphill'
            WHEN NEW.climb_type IN ('bergab', 'talfahrt') THEN 'downhill'
            ELSE 'flat'
        END;
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_segments_normalize_elevation_kind ON segments;
CREATE TRIGGER trg_segments_normalize_elevation_kind
    BEFORE INSERT OR UPDATE OF segment_type, climb_type, elevation_kind
    ON segments
    FOR EACH ROW
    EXECUTE FUNCTION normalize_segment_elevation_kind();

ALTER TABLE segments
    ALTER COLUMN elevation_kind SET NOT NULL;

UPDATE segment_versions sv
SET eligibility_segment_type = s.segment_type,
    eligibility_sport_group = s.sport_group,
    eligibility_allowed_predicates = s.allowed_predicates,
    eligibility_allowed_predicates_set = (s.allowed_predicates IS NOT NULL),
    eligibility_bidirectional = s.bidirectional,
    eligibility_visibility = COALESCE(s.visibility, 'public'),
    eligibility_elevation_kind = s.elevation_kind
FROM segments s
WHERE s.id = sv.segment_id
  AND (
      sv.eligibility_segment_type IS DISTINCT FROM s.segment_type
      OR sv.eligibility_sport_group IS DISTINCT FROM s.sport_group
      OR sv.eligibility_allowed_predicates IS DISTINCT FROM s.allowed_predicates
      OR sv.eligibility_allowed_predicates_set IS DISTINCT FROM (s.allowed_predicates IS NOT NULL)
      OR sv.eligibility_bidirectional IS DISTINCT FROM s.bidirectional
      OR sv.eligibility_visibility IS DISTINCT FROM COALESCE(s.visibility, 'public')
      OR sv.eligibility_elevation_kind IS DISTINCT FROM s.elevation_kind
  );

CREATE INDEX IF NOT EXISTS idx_segments_elevation_kind
    ON segments (elevation_kind);

WITH ordered AS (
    SELECT
        id,
        segment_id,
        version_number,
        CASE
            WHEN version_number = 1 THEN NULL::timestamptz
            ELSE (valid_from::timestamp AT TIME ZONE 'UTC')
        END AS base_from_at
    FROM segment_versions
),
deduped AS (
    SELECT
        id,
        segment_id,
        version_number,
        CASE
            WHEN base_from_at IS NULL THEN NULL::timestamptz
            ELSE base_from_at + (
                (ROW_NUMBER() OVER (
                    PARTITION BY segment_id, base_from_at
                    ORDER BY version_number
                ) - 1) * INTERVAL '1 microsecond'
            )
        END AS computed_from_at
    FROM ordered
),
with_next AS (
    SELECT
        id,
        computed_from_at,
        LEAD(computed_from_at) OVER (
            PARTITION BY segment_id
            ORDER BY version_number
        ) AS computed_until_at
    FROM deduped
)
UPDATE segment_versions sv
SET valid_from_at = wn.computed_from_at,
    valid_until_at = wn.computed_until_at,
    validity_basis = CASE
        WHEN wn.computed_from_at IS NULL THEN 'unknown_start'
        ELSE 'legacy_date_utc'
    END
FROM with_next wn
WHERE sv.id = wn.id
  AND (
      sv.valid_from_at IS DISTINCT FROM wn.computed_from_at
      OR sv.valid_until_at IS DISTINCT FROM wn.computed_until_at
      OR sv.validity_basis IS DISTINCT FROM CASE
          WHEN wn.computed_from_at IS NULL THEN 'unknown_start'
          ELSE 'legacy_date_utc'
      END
  );

WITH exact_duplicates AS (
    SELECT
        id,
        started_at,
        FIRST_VALUE(id) OVER (
            PARTITION BY activity_id, segment_id, segment_version_id, started_at, is_reverse,
                         elapsed_time_s, COALESCE(distance_m, -1), COALESCE(elevation_gain_m, -1)
            ORDER BY created_at, id
        ) AS keep_id,
        ROW_NUMBER() OVER (
            PARTITION BY activity_id, segment_id, segment_version_id, started_at, is_reverse,
                         elapsed_time_s, COALESCE(distance_m, -1), COALESCE(elevation_gain_m, -1)
            ORDER BY created_at, id
        ) AS duplicate_rank
    FROM segment_efforts
),
marked AS (
    UPDATE segment_efforts se
    SET superseded_by_effort_id = d.keep_id,
        in_leaderboard = false
    FROM exact_duplicates d
    WHERE se.id = d.id
      AND se.started_at = d.started_at
      AND d.duplicate_rank > 1
    RETURNING se.id
),
ranked AS (
    SELECT
        se.id,
        se.started_at,
        ROW_NUMBER() OVER (
            PARTITION BY se.activity_id, se.segment_id, se.segment_version_id, se.started_at, se.is_reverse
            ORDER BY se.created_at, se.id
        ) - 1 AS computed_occurrence_index
    FROM segment_efforts se
    WHERE se.superseded_by_effort_id IS NULL
)
UPDATE segment_efforts se
SET occurrence_index = ranked.computed_occurrence_index,
    match_source_id = se.activity_id::text,
    match_identity_key = concat_ws(':',
        'platform_activity',
        se.activity_id::text,
        'segment',
        se.segment_id::text,
        'version',
        se.segment_version_id::text,
        'contract',
        'course_gps_v1',
        '1',
        'direction',
        CASE WHEN se.is_reverse THEN 'reverse' ELSE 'forward' END,
        'start',
        to_char(date_trunc('microseconds', se.started_at) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
        'occurrence',
        ranked.computed_occurrence_index::text
    ),
    leaderboard_sport_type = COALESCE(NULLIF(se.sport_type, ''), 'generic'),
    leaderboard_sub_type = COALESCE(NULLIF(se.sub_type, ''), 'generic'),
    leaderboard_equipment_class = 'unspecified',
    leaderboard_assist_class = CASE WHEN COALESCE(a.is_electric, false) THEN 'electric' ELSE 'none' END,
    leaderboard_indoor_policy_state = CASE WHEN COALESCE(a.is_indoor, false) THEN 'indoor' ELSE 'outdoor' END,
    leaderboard_virtual_policy_state = CASE WHEN COALESCE(a.is_virtual, false) THEN 'virtual' ELSE 'real' END,
    leaderboard_electric_policy_state = CASE WHEN COALESCE(a.is_electric, false) THEN 'electric' ELSE 'none' END,
    leaderboard_direction = CASE WHEN se.is_reverse THEN 'reverse' ELSE 'forward' END,
    leaderboard_version_policy_key = 'legacy_segment_wide',
    leaderboard_contract_key = 'course_gps_v1',
    leaderboard_contract_version = 1
FROM activities a, ranked
WHERE a.id = se.activity_id
  AND ranked.id = se.id
  AND ranked.started_at = se.started_at;

UPDATE segment_efforts se
SET match_source_id = se.activity_id::text,
    match_identity_key = concat_ws(':',
        'platform_activity',
        se.activity_id::text,
        'segment',
        se.segment_id::text,
        'version',
        se.segment_version_id::text,
        'contract',
        'course_gps_v1',
        '1',
        'direction',
        CASE WHEN se.is_reverse THEN 'reverse' ELSE 'forward' END,
        'start',
        to_char(date_trunc('microseconds', se.started_at) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
        'occurrence',
        se.occurrence_index::text
    ),
    leaderboard_sport_type = COALESCE(NULLIF(se.sport_type, ''), 'generic'),
    leaderboard_sub_type = COALESCE(NULLIF(se.sub_type, ''), 'generic'),
    leaderboard_equipment_class = 'unspecified',
    leaderboard_assist_class = CASE WHEN COALESCE(a.is_electric, false) THEN 'electric' ELSE 'none' END,
    leaderboard_indoor_policy_state = CASE WHEN COALESCE(a.is_indoor, false) THEN 'indoor' ELSE 'outdoor' END,
    leaderboard_virtual_policy_state = CASE WHEN COALESCE(a.is_virtual, false) THEN 'virtual' ELSE 'real' END,
    leaderboard_electric_policy_state = CASE WHEN COALESCE(a.is_electric, false) THEN 'electric' ELSE 'none' END,
    leaderboard_direction = CASE WHEN se.is_reverse THEN 'reverse' ELSE 'forward' END,
    leaderboard_version_policy_key = 'legacy_segment_wide',
    leaderboard_contract_key = 'course_gps_v1',
    leaderboard_contract_version = 1
FROM activities a
WHERE a.id = se.activity_id
  AND (se.match_source_id IS NULL OR se.match_identity_key IS NULL);

CREATE OR REPLACE FUNCTION fn_segment_efforts_match_metadata_defaults()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    activity_is_indoor boolean := false;
    activity_is_virtual boolean := false;
    activity_is_electric boolean := false;
BEGIN
    SELECT COALESCE(is_indoor, false),
           COALESCE(is_virtual, false),
           COALESCE(is_electric, false)
    INTO activity_is_indoor,
         activity_is_virtual,
         activity_is_electric
    FROM activities
    WHERE id = NEW.activity_id;

    NEW.matching_contract_key := COALESCE(NULLIF(NEW.matching_contract_key, ''), 'course_gps_v1');
    NEW.matching_contract_version := COALESCE(NEW.matching_contract_version, 1);
    NEW.match_source_type := COALESCE(NULLIF(NEW.match_source_type, ''), 'platform_activity');
    NEW.match_source_id := COALESCE(NULLIF(NEW.match_source_id, ''), NEW.activity_id::text);
    NEW.occurrence_index := COALESCE(NEW.occurrence_index, 0);

    IF NEW.leaderboard_sport_type IS NULL OR NEW.leaderboard_sport_type IN ('', 'generic') THEN
        NEW.leaderboard_sport_type := COALESCE(NULLIF(NEW.sport_type, ''), 'generic');
    END IF;
    IF NEW.leaderboard_sub_type IS NULL OR NEW.leaderboard_sub_type IN ('', 'generic') THEN
        NEW.leaderboard_sub_type := COALESCE(NULLIF(NEW.sub_type, ''), 'generic');
    END IF;
    NEW.leaderboard_equipment_class := COALESCE(NULLIF(NEW.leaderboard_equipment_class, ''), 'unspecified');
    IF NEW.leaderboard_assist_class IS NULL OR NEW.leaderboard_assist_class = '' OR (NEW.leaderboard_assist_class = 'none' AND activity_is_electric) THEN
        NEW.leaderboard_assist_class := CASE WHEN activity_is_electric THEN 'electric' ELSE 'none' END;
    END IF;
    IF NEW.leaderboard_indoor_policy_state IS NULL OR NEW.leaderboard_indoor_policy_state = '' OR (NEW.leaderboard_indoor_policy_state = 'outdoor' AND activity_is_indoor) THEN
        NEW.leaderboard_indoor_policy_state := CASE WHEN activity_is_indoor THEN 'indoor' ELSE 'outdoor' END;
    END IF;
    IF NEW.leaderboard_virtual_policy_state IS NULL OR NEW.leaderboard_virtual_policy_state = '' OR (NEW.leaderboard_virtual_policy_state = 'real' AND activity_is_virtual) THEN
        NEW.leaderboard_virtual_policy_state := CASE WHEN activity_is_virtual THEN 'virtual' ELSE 'real' END;
    END IF;
    IF NEW.leaderboard_electric_policy_state IS NULL OR NEW.leaderboard_electric_policy_state = '' OR (NEW.leaderboard_electric_policy_state = 'none' AND activity_is_electric) THEN
        NEW.leaderboard_electric_policy_state := CASE WHEN activity_is_electric THEN 'electric' ELSE 'none' END;
    END IF;
    IF NEW.leaderboard_direction IS NULL OR NEW.leaderboard_direction = '' OR (NEW.leaderboard_direction = 'forward' AND NEW.is_reverse) THEN
        NEW.leaderboard_direction := CASE WHEN NEW.is_reverse THEN 'reverse' ELSE 'forward' END;
    END IF;
    NEW.leaderboard_version_policy_key := COALESCE(NULLIF(NEW.leaderboard_version_policy_key, ''), 'legacy_segment_wide');
    NEW.leaderboard_contract_key := COALESCE(NULLIF(NEW.leaderboard_contract_key, ''), 'course_gps_v1');
    NEW.leaderboard_contract_version := COALESCE(NEW.leaderboard_contract_version, 1);

    IF NEW.match_identity_key IS NULL OR NEW.match_identity_key = '' THEN
        NEW.match_identity_key := concat_ws(':',
            NEW.match_source_type,
            NEW.match_source_id,
            'segment',
            NEW.segment_id::text,
            'version',
            NEW.segment_version_id::text,
            'contract',
            NEW.matching_contract_key,
            NEW.matching_contract_version::text,
            'direction',
            NEW.leaderboard_direction,
            'start',
            to_char(date_trunc('microseconds', NEW.started_at) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
            'occurrence',
            NEW.occurrence_index::text
        );
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_segment_efforts_match_metadata_defaults ON segment_efforts;
CREATE TRIGGER trg_segment_efforts_match_metadata_defaults
    BEFORE INSERT OR UPDATE OF matching_contract_key, matching_contract_version,
        match_source_type, match_source_id, match_identity_key, occurrence_index,
        leaderboard_sport_type, leaderboard_sub_type, leaderboard_equipment_class,
        leaderboard_assist_class, leaderboard_indoor_policy_state,
        leaderboard_virtual_policy_state, leaderboard_electric_policy_state,
        leaderboard_direction, leaderboard_version_policy_key,
        leaderboard_contract_key, leaderboard_contract_version,
        sport_type, sub_type, is_reverse
    ON segment_efforts
    FOR EACH ROW
    EXECUTE FUNCTION fn_segment_efforts_match_metadata_defaults();

CREATE OR REPLACE FUNCTION fn_segment_versions_validity_defaults()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    boundary timestamptz;
    snapshot_segment_type text;
    snapshot_sport_group text;
    snapshot_allowed_predicates jsonb;
    snapshot_bidirectional boolean;
    snapshot_visibility text;
    snapshot_elevation_kind text;
    snapshot_defaults_missing boolean;
BEGIN
    NEW.matching_contract_key := COALESCE(NULLIF(NEW.matching_contract_key, ''), 'course_gps_v1');
    NEW.matching_contract_version := COALESCE(NEW.matching_contract_version, 1);
    NEW.published_at := COALESCE(NEW.published_at, NOW());

    snapshot_defaults_missing := (
        NEW.eligibility_segment_type = 'standard'
        AND NEW.eligibility_sport_group = 'generic'
        AND NEW.eligibility_allowed_predicates IS NULL
        AND NEW.eligibility_allowed_predicates_set = false
        AND NEW.eligibility_bidirectional = true
        AND NEW.eligibility_visibility = 'public'
        AND NEW.eligibility_elevation_kind = 'flat'
    );

    IF snapshot_defaults_missing
       OR NEW.eligibility_segment_type IS NULL
       OR NEW.eligibility_sport_group IS NULL
       OR NEW.eligibility_bidirectional IS NULL
       OR NEW.eligibility_visibility IS NULL
       OR NEW.eligibility_elevation_kind IS NULL
       OR (NEW.eligibility_allowed_predicates IS NULL AND NEW.eligibility_allowed_predicates_set = false) THEN
        SELECT s.segment_type,
               s.sport_group,
               s.allowed_predicates,
               s.bidirectional,
               COALESCE(s.visibility, 'public'),
               s.elevation_kind
        INTO snapshot_segment_type,
             snapshot_sport_group,
             snapshot_allowed_predicates,
             snapshot_bidirectional,
             snapshot_visibility,
             snapshot_elevation_kind
        FROM segments s
        WHERE s.id = NEW.segment_id;

        IF snapshot_defaults_missing THEN
            NEW.eligibility_segment_type := COALESCE(snapshot_segment_type, 'standard');
            NEW.eligibility_sport_group := COALESCE(snapshot_sport_group, 'generic');
            NEW.eligibility_bidirectional := COALESCE(snapshot_bidirectional, true);
            NEW.eligibility_visibility := COALESCE(snapshot_visibility, 'public');
            NEW.eligibility_elevation_kind := COALESCE(snapshot_elevation_kind, 'flat');
            NEW.eligibility_allowed_predicates := snapshot_allowed_predicates;
            NEW.eligibility_allowed_predicates_set := (snapshot_allowed_predicates IS NOT NULL);
        ELSE
            NEW.eligibility_segment_type := COALESCE(NEW.eligibility_segment_type, snapshot_segment_type, 'standard');
            NEW.eligibility_sport_group := COALESCE(NEW.eligibility_sport_group, snapshot_sport_group, 'generic');
            NEW.eligibility_bidirectional := COALESCE(NEW.eligibility_bidirectional, snapshot_bidirectional, true);
            NEW.eligibility_visibility := COALESCE(NEW.eligibility_visibility, snapshot_visibility, 'public');
            NEW.eligibility_elevation_kind := COALESCE(NEW.eligibility_elevation_kind, snapshot_elevation_kind, 'flat');
            IF NEW.eligibility_allowed_predicates IS NULL AND NEW.eligibility_allowed_predicates_set = false THEN
                NEW.eligibility_allowed_predicates := snapshot_allowed_predicates;
                NEW.eligibility_allowed_predicates_set := (snapshot_allowed_predicates IS NOT NULL);
            END IF;
        END IF;
    END IF;

    IF NEW.version_number = 1 AND NEW.valid_from_at IS NULL THEN
        NEW.validity_basis := COALESCE(NULLIF(NEW.validity_basis, ''), 'unknown_start');
    ELSIF NEW.valid_from_at IS NULL THEN
        NEW.valid_from_at := NEW.published_at;
        IF NEW.validity_basis IS NULL OR NEW.validity_basis = 'unknown_start' THEN
            NEW.validity_basis := 'editor_asserted';
        END IF;
    END IF;

    IF NEW.valid_from_at IS NOT NULL THEN
        boundary := NEW.valid_from_at;

        UPDATE segment_versions
        SET valid_until_at = boundary
        WHERE segment_id = NEW.segment_id
          AND matching_contract_key = NEW.matching_contract_key
          AND matching_contract_version = NEW.matching_contract_version
          AND version_number < NEW.version_number
          AND (valid_from_at IS NULL OR valid_from_at < boundary)
          AND (valid_until_at IS NULL OR valid_until_at > boundary);
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_segment_versions_validity_defaults ON segment_versions;
CREATE TRIGGER trg_segment_versions_validity_defaults
    BEFORE INSERT
    ON segment_versions
    FOR EACH ROW
    EXECUTE FUNCTION fn_segment_versions_validity_defaults();

ALTER TABLE segment_efforts
    ALTER COLUMN match_source_id SET NOT NULL,
    ALTER COLUMN match_identity_key SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_versions_matching_contract_chk') THEN
        ALTER TABLE segment_versions
            ADD CONSTRAINT segment_versions_matching_contract_chk
            CHECK (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_versions_validity_basis_chk') THEN
        ALTER TABLE segment_versions
            ADD CONSTRAINT segment_versions_validity_basis_chk
            CHECK (validity_basis IN ('unknown_start', 'legacy_date_utc', 'editor_asserted', 'import_asserted'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_versions_validity_order_chk') THEN
        ALTER TABLE segment_versions
            ADD CONSTRAINT segment_versions_validity_order_chk
            CHECK (valid_until_at IS NULL OR valid_from_at IS NULL OR valid_until_at > valid_from_at);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_versions_eligibility_segment_type_chk') THEN
        ALTER TABLE segment_versions
            ADD CONSTRAINT segment_versions_eligibility_segment_type_chk
            CHECK (eligibility_segment_type IN ('standard', 'trail', 'lap', 'wintersport', 'wassersport', 'lift'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_versions_eligibility_elevation_kind_chk') THEN
        ALTER TABLE segment_versions
            ADD CONSTRAINT segment_versions_eligibility_elevation_kind_chk
            CHECK (eligibility_elevation_kind IN ('flat', 'hilly', 'uphill', 'downhill'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_efforts_matching_contract_chk') THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_matching_contract_chk
            CHECK (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_efforts_leaderboard_contract_chk') THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_leaderboard_contract_chk
            CHECK (leaderboard_contract_key = 'course_gps_v1' AND leaderboard_contract_version = 1);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_efforts_match_source_type_chk') THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_match_source_type_chk
            CHECK (match_source_type IN ('platform_activity', 'manual_result', 'imported_result'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_efforts_leaderboard_direction_chk') THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_leaderboard_direction_chk
            CHECK (leaderboard_direction IN ('forward', 'reverse'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_efforts_leaderboard_policy_chk') THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_leaderboard_policy_chk
            CHECK (leaderboard_version_policy_key IN ('legacy_segment_wide'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_efforts_policy_state_chk') THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_policy_state_chk
            CHECK (
                leaderboard_assist_class IN ('none', 'electric')
                AND leaderboard_indoor_policy_state IN ('outdoor', 'indoor')
                AND leaderboard_virtual_policy_state IN ('real', 'virtual')
                AND leaderboard_electric_policy_state IN ('none', 'electric')
            );
    END IF;
END
$$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_segment_efforts_active_match_identity
    ON segment_efforts (match_identity_key, started_at)
    WHERE superseded_by_effort_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_segment_versions_course_gps_v1_validity
    ON segment_versions (
        segment_id,
        matching_contract_key,
        matching_contract_version,
        valid_from_at DESC,
        version_number DESC
    );

CREATE INDEX IF NOT EXISTS idx_segment_efforts_leaderboard_partition
    ON segment_efforts (
        segment_id,
        leaderboard_sport_type,
        leaderboard_direction,
        elapsed_time_s ASC
    )
    WHERE in_leaderboard = true AND superseded_by_effort_id IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'segment_versions_course_gps_v1_no_overlap') THEN
        ALTER TABLE segment_versions
            ADD CONSTRAINT segment_versions_course_gps_v1_no_overlap
            EXCLUDE USING gist (
                segment_id WITH =,
                matching_contract_key WITH =,
                matching_contract_version WITH =,
                tstzrange(
                    COALESCE(valid_from_at, '-infinity'::timestamptz),
                    COALESCE(valid_until_at, 'infinity'::timestamptz),
                    '[)'
                ) WITH &&
            );
    END IF;
END
$$;
