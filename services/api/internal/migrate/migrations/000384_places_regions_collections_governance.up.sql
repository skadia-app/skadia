CREATE TABLE target_places (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    lifecycle_owner_kind TEXT NOT NULL DEFAULT 'user',
    lifecycle_owner_ref TEXT,
    name TEXT NOT NULL,
    description TEXT,
    place_kind TEXT NOT NULL,
    geom GEOMETRY(Point, 4326) NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'private',
    access_scope_id UUID REFERENCES object_access_scopes(id) ON DELETE SET NULL,
    origin_key TEXT NOT NULL,
    external_ref_kind TEXT,
    external_ref_id UUID,
    external_ref TEXT,
    external_ref_visibility TEXT NOT NULL DEFAULT 'owner_only',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at TIMESTAMPTZ,
    CHECK (lifecycle_owner_kind IN ('user', 'organization', 'system')),
    CHECK (
        (lifecycle_owner_kind = 'user' AND owner_user_id IS NOT NULL AND lifecycle_owner_ref IS NULL)
        OR (lifecycle_owner_kind <> 'user' AND owner_user_id IS NULL AND lifecycle_owner_ref IS NOT NULL)
    ),
    CHECK (place_kind IN ('custom', 'private_place', 'poi', 'entrance', 'parking', 'summit', 'pass', 'water', 'settlement', 'amenity')),
    CHECK (visibility IN ('private', 'shared', 'public')),
    CHECK (
        (visibility = 'shared' AND access_scope_id IS NOT NULL)
        OR (visibility IN ('private', 'public') AND access_scope_id IS NULL)
    ),
    CHECK (external_ref_visibility IN ('owner_only', 'shared_safe', 'public_safe')),
    CHECK (archived_at IS NULL OR archived_at >= created_at)
);

CREATE INDEX idx_target_places_owner ON target_places (owner_user_id, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_target_places_visibility ON target_places (visibility, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_target_places_shared_scope ON target_places (access_scope_id, updated_at DESC) WHERE visibility = 'shared' AND archived_at IS NULL;
CREATE INDEX idx_target_places_geom ON target_places USING GIST (geom) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX uq_target_places_external_ref_live
    ON target_places (external_ref_kind, external_ref_id)
    WHERE external_ref_kind IS NOT NULL AND external_ref_id IS NOT NULL AND archived_at IS NULL;

CREATE TABLE target_regions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    lifecycle_owner_kind TEXT NOT NULL DEFAULT 'user',
    lifecycle_owner_ref TEXT,
    name TEXT NOT NULL,
    description TEXT,
    region_kind TEXT NOT NULL,
    geom GEOMETRY(MultiPolygon, 4326) NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'private',
    access_scope_id UUID REFERENCES object_access_scopes(id) ON DELETE SET NULL,
    origin_key TEXT NOT NULL,
    external_ref_kind TEXT,
    external_ref_id UUID,
    external_ref TEXT,
    external_ref_visibility TEXT NOT NULL DEFAULT 'owner_only',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at TIMESTAMPTZ,
    CHECK (lifecycle_owner_kind IN ('user', 'organization', 'system')),
    CHECK (
        (lifecycle_owner_kind = 'user' AND owner_user_id IS NOT NULL AND lifecycle_owner_ref IS NULL)
        OR (lifecycle_owner_kind <> 'user' AND owner_user_id IS NULL AND lifecycle_owner_ref IS NOT NULL)
    ),
    CHECK (region_kind IN ('custom', 'private_area', 'public_pool', 'venue_area', 'privacy_zone_mirror', 'admin_area', 'protected_area')),
    CHECK (ST_IsValid(geom)),
    CHECK (visibility IN ('private', 'shared', 'public')),
    CHECK (
        (visibility = 'shared' AND access_scope_id IS NOT NULL)
        OR (visibility IN ('private', 'public') AND access_scope_id IS NULL)
    ),
    CHECK (external_ref_visibility IN ('owner_only', 'shared_safe', 'public_safe')),
    CHECK (archived_at IS NULL OR archived_at >= created_at)
);

CREATE INDEX idx_target_regions_owner ON target_regions (owner_user_id, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_target_regions_visibility ON target_regions (visibility, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_target_regions_shared_scope ON target_regions (access_scope_id, updated_at DESC) WHERE visibility = 'shared' AND archived_at IS NULL;
CREATE INDEX idx_target_regions_geom ON target_regions USING GIST (geom) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX uq_target_regions_external_ref_live
    ON target_regions (external_ref_kind, external_ref_id)
    WHERE external_ref_kind IS NOT NULL AND external_ref_id IS NOT NULL AND archived_at IS NULL;

CREATE TABLE target_collections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    lifecycle_owner_kind TEXT NOT NULL DEFAULT 'user',
    lifecycle_owner_ref TEXT,
    name TEXT NOT NULL,
    description TEXT,
    collection_kind TEXT NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'private',
    access_scope_id UUID REFERENCES object_access_scopes(id) ON DELETE SET NULL,
    display_style TEXT NOT NULL DEFAULT 'map_list',
    default_sort TEXT NOT NULL DEFAULT 'manual',
    origin_key TEXT NOT NULL,
    external_ref_kind TEXT,
    external_ref_id UUID,
    external_ref TEXT,
    external_ref_visibility TEXT NOT NULL DEFAULT 'owner_only',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at TIMESTAMPTZ,
    CHECK (lifecycle_owner_kind IN ('user', 'organization', 'system')),
    CHECK (
        (lifecycle_owner_kind = 'user' AND owner_user_id IS NOT NULL AND lifecycle_owner_ref IS NULL)
        OR (lifecycle_owner_kind <> 'user' AND owner_user_id IS NULL AND lifecycle_owner_ref IS NOT NULL)
    ),
    CHECK (collection_kind IN ('private_map', 'public_pool', 'venue_pool', 'route_set', 'course_set', 'rule_set', 'privacy_set', 'custom')),
    CHECK (visibility IN ('private', 'shared', 'public')),
    CHECK (
        (visibility = 'shared' AND access_scope_id IS NOT NULL)
        OR (visibility IN ('private', 'public') AND access_scope_id IS NULL)
    ),
    CHECK (display_style IN ('map_list', 'ordered_list', 'directory', 'compact')),
    CHECK (default_sort IN ('manual', 'name', 'distance', 'updated_at', 'role_then_manual')),
    CHECK (external_ref_visibility IN ('owner_only', 'shared_safe', 'public_safe')),
    CHECK (archived_at IS NULL OR archived_at >= created_at)
);

CREATE INDEX idx_target_collections_owner ON target_collections (owner_user_id, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_target_collections_visibility ON target_collections (visibility, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_target_collections_shared_scope ON target_collections (access_scope_id, updated_at DESC) WHERE visibility = 'shared' AND archived_at IS NULL;
CREATE UNIQUE INDEX uq_target_collections_external_ref_live
    ON target_collections (external_ref_kind, external_ref_id)
    WHERE external_ref_kind IS NOT NULL AND external_ref_id IS NOT NULL AND archived_at IS NULL;

CREATE TABLE target_collection_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID NOT NULL REFERENCES target_collections(id) ON DELETE CASCADE,
    member_kind TEXT NOT NULL,
    member_id UUID,
    member_ref TEXT,
    binding_mode TEXT NOT NULL DEFAULT 'live_current',
    pinned_target_version_id UUID,
    pinned_snapshot_id UUID,
    member_owner_scope TEXT NOT NULL DEFAULT 'unknown',
    member_visibility_snapshot TEXT NOT NULL DEFAULT 'unknown',
    membership_visibility TEXT NOT NULL DEFAULT 'inherit_collection',
    origin_key TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    sort_order INTEGER NOT NULL DEFAULT 0,
    display_name_override TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    added_by UUID REFERENCES users(id) ON DELETE SET NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    removed_at TIMESTAMPTZ,
    CHECK (member_kind IN ('course', 'route_publication', 'place', 'region', 'collection', 'privacy_zone', 'tag_zone', 'external_ref')),
    CHECK (
        (member_kind = 'external_ref' AND member_ref IS NOT NULL AND member_id IS NULL)
        OR (member_kind <> 'external_ref' AND member_id IS NOT NULL)
    ),
    CHECK (binding_mode IN ('live_current', 'pinned_version', 'pinned_snapshot', 'external_ref')),
    CHECK (
        (binding_mode = 'live_current' AND pinned_target_version_id IS NULL AND pinned_snapshot_id IS NULL)
        OR (binding_mode = 'pinned_version' AND pinned_target_version_id IS NOT NULL AND pinned_snapshot_id IS NULL)
        OR (binding_mode = 'pinned_snapshot' AND pinned_snapshot_id IS NOT NULL)
        OR (binding_mode = 'external_ref' AND member_kind = 'external_ref' AND member_ref IS NOT NULL)
    ),
    CHECK (member_owner_scope IN ('unknown', 'user', 'shared', 'global', 'system')),
    CHECK (member_visibility_snapshot IN ('unknown', 'private', 'shared', 'public', 'deleted', 'archived')),
    CHECK (membership_visibility IN ('inherit_collection', 'owner_only', 'public_safe')),
    CHECK (role IN ('member', 'primary', 'trail', 'lift', 'area', 'poi', 'entrance', 'parking', 'start', 'finish', 'boundary', 'variant', 'admin_context')),
    CHECK (valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from),
    CHECK (removed_at IS NULL OR removed_at >= added_at)
);

CREATE INDEX idx_target_collection_members_collection ON target_collection_members (collection_id, sort_order, added_at) WHERE removed_at IS NULL;
CREATE INDEX idx_target_collection_members_member ON target_collection_members (member_kind, member_id) WHERE removed_at IS NULL AND member_id IS NOT NULL;
CREATE INDEX idx_target_collection_members_pinned_version ON target_collection_members (pinned_target_version_id) WHERE pinned_target_version_id IS NOT NULL AND removed_at IS NULL;
CREATE INDEX idx_target_collection_members_pinned_snapshot ON target_collection_members (pinned_snapshot_id) WHERE pinned_snapshot_id IS NOT NULL AND removed_at IS NULL;
CREATE UNIQUE INDEX uq_target_collection_members_live_member_role
    ON target_collection_members (collection_id, member_kind, member_id, role)
    WHERE removed_at IS NULL AND member_id IS NOT NULL;

CREATE TABLE target_collection_member_reverse_index (
    member_kind TEXT NOT NULL,
    member_id UUID NOT NULL,
    collection_id UUID NOT NULL REFERENCES target_collections(id) ON DELETE CASCADE,
    collection_member_id UUID NOT NULL REFERENCES target_collection_members(id) ON DELETE CASCADE,
    contributes_bounds BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (member_kind, member_id, collection_member_id),
    CHECK (member_kind IN ('course', 'route_publication', 'place', 'region', 'collection', 'privacy_zone', 'tag_zone'))
);

CREATE INDEX idx_target_collection_member_reverse_index_collection ON target_collection_member_reverse_index (collection_id);

CREATE TABLE target_object_search_projection (
    object_kind TEXT NOT NULL,
    object_id UUID NOT NULL,
    owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    lifecycle_owner_kind TEXT NOT NULL,
    lifecycle_owner_ref TEXT,
    name TEXT NOT NULL,
    object_subkind TEXT NOT NULL,
    visibility TEXT NOT NULL,
    access_scope_id UUID,
    geom GEOMETRY(Geometry, 4326) NOT NULL,
    bounds GEOMETRY(Geometry, 4326) NOT NULL,
    centroid GEOMETRY(Point, 4326) NOT NULL,
    external_ref_kind TEXT,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,
    PRIMARY KEY (object_kind, object_id),
    CHECK (object_kind IN ('place', 'region')),
    CHECK (lifecycle_owner_kind IN ('user', 'organization', 'system')),
    CHECK (visibility IN ('private', 'shared', 'public')),
    CHECK (
        (visibility = 'shared' AND access_scope_id IS NOT NULL)
        OR (visibility IN ('private', 'public') AND access_scope_id IS NULL)
    )
);

CREATE INDEX idx_target_object_search_projection_public ON target_object_search_projection (object_kind, object_subkind, updated_at DESC) WHERE visibility = 'public' AND archived_at IS NULL;
CREATE INDEX idx_target_object_search_projection_owner ON target_object_search_projection (owner_user_id, object_kind, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_target_object_search_projection_shared ON target_object_search_projection (access_scope_id, object_kind, updated_at DESC) WHERE visibility = 'shared' AND archived_at IS NULL;
CREATE INDEX idx_target_object_search_projection_geom ON target_object_search_projection USING GIST (geom) WHERE archived_at IS NULL;
CREATE INDEX idx_target_object_search_projection_bounds ON target_object_search_projection USING GIST (bounds) WHERE archived_at IS NULL;
CREATE INDEX idx_target_object_search_projection_name_trgm ON target_object_search_projection USING GIN (name gin_trgm_ops) WHERE archived_at IS NULL;

CREATE TABLE target_collection_summary_projection (
    collection_id UUID PRIMARY KEY REFERENCES target_collections(id) ON DELETE CASCADE,
    owner_user_id UUID,
    lifecycle_owner_kind TEXT NOT NULL,
    lifecycle_owner_ref TEXT,
    name TEXT NOT NULL,
    collection_kind TEXT NOT NULL,
    visibility TEXT NOT NULL,
    access_scope_id UUID,
    bounds GEOMETRY(Geometry, 4326),
    owner_member_count INTEGER NOT NULL DEFAULT 0,
    public_member_count INTEGER NOT NULL DEFAULT 0,
    owner_counts_by_kind JSONB NOT NULL DEFAULT '{}'::jsonb,
    owner_counts_by_role JSONB NOT NULL DEFAULT '{}'::jsonb,
    public_counts_by_kind JSONB NOT NULL DEFAULT '{}'::jsonb,
    public_counts_by_role JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,
    CHECK (lifecycle_owner_kind IN ('user', 'organization', 'system')),
    CHECK (visibility IN ('private', 'shared', 'public')),
    CHECK (
        (visibility = 'shared' AND access_scope_id IS NOT NULL)
        OR (visibility IN ('private', 'public') AND access_scope_id IS NULL)
    ),
    CHECK (owner_member_count >= 0),
    CHECK (public_member_count >= 0)
);

CREATE INDEX idx_target_collection_summary_projection_public ON target_collection_summary_projection (collection_kind, updated_at DESC) WHERE visibility = 'public' AND archived_at IS NULL;
CREATE INDEX idx_target_collection_summary_projection_owner ON target_collection_summary_projection (owner_user_id, collection_kind, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_target_collection_summary_projection_shared ON target_collection_summary_projection (access_scope_id, collection_kind, updated_at DESC) WHERE visibility = 'shared' AND archived_at IS NULL;
CREATE INDEX idx_target_collection_summary_projection_bounds ON target_collection_summary_projection USING GIST (bounds) WHERE bounds IS NOT NULL AND archived_at IS NULL;
CREATE INDEX idx_target_collection_summary_projection_name_trgm ON target_collection_summary_projection USING GIN (name gin_trgm_ops) WHERE archived_at IS NULL;

CREATE TABLE governance_scopes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind TEXT NOT NULL,
    anchor_target_kind TEXT,
    anchor_target_id UUID,
    anchor_collection_id UUID REFERENCES target_collections(id) ON DELETE SET NULL,
    owner_scope TEXT NOT NULL,
    owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    owner_ref TEXT,
    visibility TEXT NOT NULL DEFAULT 'private',
    origin_key TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at TIMESTAMPTZ,
    CHECK (kind IN ('user_space', 'organization', 'region_scope', 'collection_scope', 'system_scope')),
    CHECK (anchor_target_kind IS NULL OR anchor_target_kind IN ('course', 'route_publication', 'place', 'region')),
    CHECK (owner_scope IN ('user', 'shared', 'global', 'system')),
    CHECK (
        (owner_scope = 'user' AND owner_user_id IS NOT NULL AND owner_ref IS NULL)
        OR (owner_scope <> 'user' AND owner_user_id IS NULL AND owner_ref IS NOT NULL)
    ),
    CHECK (visibility IN ('private', 'shared', 'public')),
    CHECK (valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from),
    CHECK (archived_at IS NULL OR archived_at >= created_at)
);

CREATE INDEX idx_governance_scopes_owner_user ON governance_scopes (owner_user_id, kind, created_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_governance_scopes_owner_ref ON governance_scopes (owner_scope, owner_ref, kind, created_at DESC) WHERE archived_at IS NULL AND owner_ref IS NOT NULL;
CREATE INDEX idx_governance_scopes_anchor_target ON governance_scopes (anchor_target_kind, anchor_target_id) WHERE anchor_target_id IS NOT NULL AND archived_at IS NULL;
CREATE INDEX idx_governance_scopes_anchor_collection ON governance_scopes (anchor_collection_id) WHERE anchor_collection_id IS NOT NULL AND archived_at IS NULL;

CREATE TABLE governance_actor_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope_id UUID NOT NULL REFERENCES governance_scopes(id) ON DELETE CASCADE,
    actor_type TEXT NOT NULL,
    actor_user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    actor_ref TEXT,
    role TEXT NOT NULL,
    permissions TEXT[] NOT NULL DEFAULT '{}'::text[],
    public_credit TEXT NOT NULL DEFAULT 'hidden',
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CHECK (actor_type IN ('user', 'group', 'system', 'external_party_credit')),
    CHECK (
        (actor_type = 'user' AND actor_user_id IS NOT NULL AND actor_ref IS NULL)
        OR (actor_type <> 'user' AND actor_user_id IS NULL AND actor_ref IS NOT NULL AND actor_ref <> '')
    ),
    CHECK (role IN ('owner', 'region_admin', 'moderator', 'maintainer', 'contributor', 'sponsor', 'land_manager', 'viewer')),
    CHECK (permissions <@ ARRAY[
        'edit_metadata',
        'edit_geometry',
        'publish',
        'hide_or_archive',
        'disable_leaderboard',
        'disable_gps_download',
        'manage_reports',
        'manage_events',
        'resolve_claims',
        'manage_members',
        'manage_roles',
        'link_external_refs'
    ]::text[]),
    CHECK (public_credit IN ('hidden', 'listed', 'badge')),
    CHECK (actor_type <> 'external_party_credit' OR cardinality(permissions) = 0),
    CHECK (valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX idx_governance_actor_roles_scope ON governance_actor_roles (scope_id, role) WHERE revoked_at IS NULL;
CREATE INDEX idx_governance_actor_roles_actor_user ON governance_actor_roles (actor_user_id, scope_id) WHERE actor_type = 'user' AND revoked_at IS NULL;
CREATE INDEX idx_governance_actor_roles_actor_ref ON governance_actor_roles (actor_type, actor_ref, scope_id) WHERE actor_ref IS NOT NULL AND revoked_at IS NULL;

CREATE TABLE governance_scope_target_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope_id UUID NOT NULL REFERENCES governance_scopes(id) ON DELETE CASCADE,
    target_kind TEXT NOT NULL,
    target_id UUID NOT NULL,
    target_version_id UUID,
    binding_reason TEXT NOT NULL,
    permissions_scope TEXT NOT NULL,
    source TEXT NOT NULL,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    audit_event_id UUID,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CHECK (target_kind IN ('course', 'route_publication', 'place', 'region', 'collection', 'privacy_zone', 'tag_zone')),
    CHECK (binding_reason IN ('explicit_member', 'collection_member', 'imported_admin_area', 'manual_delegation')),
    CHECK (permissions_scope IN ('metadata', 'geometry', 'reports', 'events', 'moderation', 'membership', 'full')),
    CHECK (source IN ('owner_assignment', 'admin_assignment', 'import', 'materialized_policy')),
    CHECK (valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX idx_governance_scope_target_bindings_scope ON governance_scope_target_bindings (scope_id, permissions_scope) WHERE revoked_at IS NULL;
CREATE INDEX idx_governance_scope_target_bindings_target ON governance_scope_target_bindings (target_kind, target_id, permissions_scope) WHERE revoked_at IS NULL;
CREATE INDEX idx_governance_scope_target_bindings_version ON governance_scope_target_bindings (target_version_id) WHERE target_version_id IS NOT NULL AND revoked_at IS NULL;

CREATE TABLE target_change_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_type TEXT NOT NULL,
    subject_id UUID NOT NULL,
    target_kind TEXT,
    target_id UUID,
    collection_id UUID REFERENCES target_collections(id) ON DELETE SET NULL,
    governance_scope_id UUID REFERENCES governance_scopes(id) ON DELETE SET NULL,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    before_snapshot_id UUID,
    after_snapshot_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (subject_type IN ('place', 'region', 'collection', 'collection_membership', 'governance_scope', 'governance_actor_role', 'governance_scope_target_binding')),
    CHECK (target_kind IS NULL OR target_kind IN ('course', 'route_publication', 'place', 'region', 'collection', 'privacy_zone', 'tag_zone'))
);

CREATE INDEX idx_target_change_events_subject ON target_change_events (subject_type, subject_id, created_at DESC);
CREATE INDEX idx_target_change_events_target ON target_change_events (target_kind, target_id, created_at DESC) WHERE target_id IS NOT NULL;
CREATE INDEX idx_target_change_events_collection ON target_change_events (collection_id, created_at DESC) WHERE collection_id IS NOT NULL;
CREATE INDEX idx_target_change_events_governance ON target_change_events (governance_scope_id, created_at DESC) WHERE governance_scope_id IS NOT NULL;

CREATE TABLE target_revision_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    change_event_id UUID REFERENCES target_change_events(id) ON DELETE CASCADE,
    snapshot_role TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id UUID NOT NULL,
    target_kind TEXT,
    target_id UUID,
    collection_id UUID REFERENCES target_collections(id) ON DELETE SET NULL,
    governance_scope_id UUID REFERENCES governance_scopes(id) ON DELETE SET NULL,
    snapshot_hash TEXT NOT NULL,
    redaction_policy TEXT NOT NULL,
    snapshot JSONB NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (snapshot_role IN ('before', 'after')),
    CHECK (subject_type IN ('place', 'region', 'collection', 'collection_membership', 'governance_scope', 'governance_actor_role', 'governance_scope_target_binding')),
    CHECK (target_kind IS NULL OR target_kind IN ('course', 'route_publication', 'place', 'region', 'collection', 'privacy_zone', 'tag_zone')),
    CHECK (redaction_policy IN ('owner_private', 'shared_safe', 'public_safe', 'non_personal_hashes_only'))
);

CREATE INDEX idx_target_revision_snapshots_event ON target_revision_snapshots (change_event_id, snapshot_role) WHERE change_event_id IS NOT NULL;
CREATE UNIQUE INDEX uq_target_revision_snapshots_event_role
    ON target_revision_snapshots (change_event_id, snapshot_role)
    WHERE change_event_id IS NOT NULL;

ALTER TABLE target_change_events
    ADD CONSTRAINT target_change_events_before_snapshot_fk
    FOREIGN KEY (before_snapshot_id) REFERENCES target_revision_snapshots(id)
    DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE target_change_events
    ADD CONSTRAINT target_change_events_after_snapshot_fk
    FOREIGN KEY (after_snapshot_id) REFERENCES target_revision_snapshots(id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE governance_actor_target_projection (
    actor_type TEXT NOT NULL,
    actor_user_id UUID,
    actor_ref TEXT,
    actor_key TEXT GENERATED ALWAYS AS (
        CASE
            WHEN actor_user_id IS NOT NULL THEN actor_user_id::text
            ELSE COALESCE(actor_ref, '')
        END
    ) STORED,
    scope_id UUID NOT NULL REFERENCES governance_scopes(id) ON DELETE CASCADE,
    target_kind TEXT NOT NULL,
    target_id UUID NOT NULL,
    permissions TEXT[] NOT NULL,
    permissions_scope TEXT NOT NULL,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (actor_type, actor_key, scope_id, target_kind, target_id, permissions_scope),
    CHECK (actor_type IN ('user', 'group', 'system')),
    CHECK (
        (actor_type = 'user' AND actor_user_id IS NOT NULL AND actor_ref IS NULL)
        OR (actor_type <> 'user' AND actor_user_id IS NULL AND actor_ref IS NOT NULL AND actor_ref <> '')
    ),
    CHECK (target_kind IN ('course', 'route_publication', 'place', 'region', 'collection', 'privacy_zone', 'tag_zone')),
    CHECK (permissions <@ ARRAY[
        'edit_metadata',
        'edit_geometry',
        'publish',
        'hide_or_archive',
        'disable_leaderboard',
        'disable_gps_download',
        'manage_reports',
        'manage_events',
        'resolve_claims',
        'manage_members',
        'manage_roles',
        'link_external_refs'
    ]::text[]),
    CHECK (permissions_scope IN ('metadata', 'geometry', 'reports', 'events', 'moderation', 'membership', 'full')),
    CHECK (valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from)
);

CREATE INDEX idx_governance_actor_target_projection_actor ON governance_actor_target_projection (actor_type, actor_key, permissions_scope);
CREATE INDEX idx_governance_actor_target_projection_target ON governance_actor_target_projection (target_kind, target_id, permissions_scope);
