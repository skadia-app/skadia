package model

import (
	"encoding/json"
	"time"
)

type TargetPlace struct {
	ID                    string          `json:"id"`
	OwnerUserID           *string         `json:"owner_user_id,omitempty"`
	LifecycleOwnerKind    string          `json:"lifecycle_owner_kind"`
	LifecycleOwnerRef     *string         `json:"lifecycle_owner_ref,omitempty"`
	Name                  string          `json:"name"`
	Description           *string         `json:"description,omitempty"`
	PlaceKind             string          `json:"place_kind"`
	GeometryGeoJSON       string          `json:"geometry,omitempty"`
	Visibility            string          `json:"visibility"`
	AccessScopeID         *string         `json:"access_scope_id,omitempty"`
	OriginKey             string          `json:"origin_key"`
	ExternalRefKind       *string         `json:"external_ref_kind,omitempty"`
	ExternalRefID         *string         `json:"external_ref_id,omitempty"`
	ExternalRef           *string         `json:"external_ref,omitempty"`
	ExternalRefVisibility string          `json:"external_ref_visibility"`
	Metadata              json.RawMessage `json:"metadata,omitempty"`
	CreatedBy             *string         `json:"created_by,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	ArchivedAt            *time.Time      `json:"archived_at,omitempty"`
}

type TargetPlaceCreate struct {
	Name                  string          `json:"name"`
	Description           *string         `json:"description,omitempty"`
	PlaceKind             string          `json:"place_kind"`
	GeometryGeoJSON       string          `json:"geometry"`
	Visibility            string          `json:"visibility"`
	AccessScopeID         *string         `json:"access_scope_id,omitempty"`
	OriginKey             string          `json:"origin_key"`
	ExternalRefKind       *string         `json:"external_ref_kind,omitempty"`
	ExternalRefID         *string         `json:"external_ref_id,omitempty"`
	ExternalRef           *string         `json:"external_ref,omitempty"`
	ExternalRefVisibility string          `json:"external_ref_visibility"`
	Metadata              json.RawMessage `json:"metadata,omitempty"`
}

type TargetPlaceUpdate struct {
	Name                  *string          `json:"name,omitempty"`
	Description           *string          `json:"description,omitempty"`
	PlaceKind             *string          `json:"place_kind,omitempty"`
	GeometryGeoJSON       *string          `json:"geometry,omitempty"`
	Visibility            *string          `json:"visibility,omitempty"`
	AccessScopeID         *string          `json:"access_scope_id,omitempty"`
	ExternalRefKind       *string          `json:"external_ref_kind,omitempty"`
	ExternalRefID         *string          `json:"external_ref_id,omitempty"`
	ExternalRef           *string          `json:"external_ref,omitempty"`
	ExternalRefVisibility *string          `json:"external_ref_visibility,omitempty"`
	Metadata              *json.RawMessage `json:"metadata,omitempty"`
}

type TargetRegion struct {
	ID                    string          `json:"id"`
	OwnerUserID           *string         `json:"owner_user_id,omitempty"`
	LifecycleOwnerKind    string          `json:"lifecycle_owner_kind"`
	LifecycleOwnerRef     *string         `json:"lifecycle_owner_ref,omitempty"`
	Name                  string          `json:"name"`
	Description           *string         `json:"description,omitempty"`
	RegionKind            string          `json:"region_kind"`
	GeometryGeoJSON       string          `json:"geometry,omitempty"`
	Visibility            string          `json:"visibility"`
	AccessScopeID         *string         `json:"access_scope_id,omitempty"`
	OriginKey             string          `json:"origin_key"`
	ExternalRefKind       *string         `json:"external_ref_kind,omitempty"`
	ExternalRefID         *string         `json:"external_ref_id,omitempty"`
	ExternalRef           *string         `json:"external_ref,omitempty"`
	ExternalRefVisibility string          `json:"external_ref_visibility"`
	Metadata              json.RawMessage `json:"metadata,omitempty"`
	CreatedBy             *string         `json:"created_by,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	ArchivedAt            *time.Time      `json:"archived_at,omitempty"`
}

type TargetRegionCreate struct {
	Name                  string          `json:"name"`
	Description           *string         `json:"description,omitempty"`
	RegionKind            string          `json:"region_kind"`
	GeometryGeoJSON       string          `json:"geometry"`
	Visibility            string          `json:"visibility"`
	AccessScopeID         *string         `json:"access_scope_id,omitempty"`
	OriginKey             string          `json:"origin_key"`
	ExternalRefKind       *string         `json:"external_ref_kind,omitempty"`
	ExternalRefID         *string         `json:"external_ref_id,omitempty"`
	ExternalRef           *string         `json:"external_ref,omitempty"`
	ExternalRefVisibility string          `json:"external_ref_visibility"`
	Metadata              json.RawMessage `json:"metadata,omitempty"`
}

type TargetRegionUpdate struct {
	Name                  *string          `json:"name,omitempty"`
	Description           *string          `json:"description,omitempty"`
	RegionKind            *string          `json:"region_kind,omitempty"`
	GeometryGeoJSON       *string          `json:"geometry,omitempty"`
	Visibility            *string          `json:"visibility,omitempty"`
	AccessScopeID         *string          `json:"access_scope_id,omitempty"`
	ExternalRefKind       *string          `json:"external_ref_kind,omitempty"`
	ExternalRefID         *string          `json:"external_ref_id,omitempty"`
	ExternalRef           *string          `json:"external_ref,omitempty"`
	ExternalRefVisibility *string          `json:"external_ref_visibility,omitempty"`
	Metadata              *json.RawMessage `json:"metadata,omitempty"`
}

type TargetCollection struct {
	ID                    string          `json:"id"`
	OwnerUserID           *string         `json:"owner_user_id,omitempty"`
	LifecycleOwnerKind    string          `json:"lifecycle_owner_kind"`
	LifecycleOwnerRef     *string         `json:"lifecycle_owner_ref,omitempty"`
	Name                  string          `json:"name"`
	Description           *string         `json:"description,omitempty"`
	CollectionKind        string          `json:"collection_kind"`
	Visibility            string          `json:"visibility"`
	AccessScopeID         *string         `json:"access_scope_id,omitempty"`
	DisplayStyle          string          `json:"display_style"`
	DefaultSort           string          `json:"default_sort"`
	OriginKey             string          `json:"origin_key"`
	ExternalRefKind       *string         `json:"external_ref_kind,omitempty"`
	ExternalRefID         *string         `json:"external_ref_id,omitempty"`
	ExternalRef           *string         `json:"external_ref,omitempty"`
	ExternalRefVisibility string          `json:"external_ref_visibility"`
	Metadata              json.RawMessage `json:"metadata,omitempty"`
	CreatedBy             *string         `json:"created_by,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	ArchivedAt            *time.Time      `json:"archived_at,omitempty"`
}

type TargetCollectionMember struct {
	ID                       string          `json:"id"`
	CollectionID             string          `json:"collection_id"`
	MemberKind               string          `json:"member_kind"`
	MemberID                 *string         `json:"member_id,omitempty"`
	MemberRef                *string         `json:"member_ref,omitempty"`
	BindingMode              string          `json:"binding_mode"`
	PinnedTargetVersionID    *string         `json:"pinned_target_version_id,omitempty"`
	PinnedSnapshotID         *string         `json:"pinned_snapshot_id,omitempty"`
	MemberOwnerScope         string          `json:"member_owner_scope"`
	MemberVisibilitySnapshot string          `json:"member_visibility_snapshot"`
	MembershipVisibility     string          `json:"membership_visibility"`
	OriginKey                string          `json:"origin_key"`
	Role                     string          `json:"role"`
	SortOrder                int             `json:"sort_order"`
	DisplayNameOverride      *string         `json:"display_name_override,omitempty"`
	MemberVisibility         string          `json:"member_visibility_state,omitempty"`
	Metadata                 json.RawMessage `json:"metadata,omitempty"`
	ValidFrom                *time.Time      `json:"valid_from,omitempty"`
	ValidUntil               *time.Time      `json:"valid_until,omitempty"`
	AddedBy                  *string         `json:"added_by,omitempty"`
	AddedAt                  time.Time       `json:"added_at"`
	RemovedAt                *time.Time      `json:"removed_at,omitempty"`
}

type GovernanceScope struct {
	ID                 string          `json:"id"`
	Kind               string          `json:"kind"`
	AnchorTargetKind   *string         `json:"anchor_target_kind,omitempty"`
	AnchorTargetID     *string         `json:"anchor_target_id,omitempty"`
	AnchorCollectionID *string         `json:"anchor_collection_id,omitempty"`
	OwnerScope         string          `json:"owner_scope"`
	OwnerUserID        *string         `json:"owner_user_id,omitempty"`
	OwnerRef           *string         `json:"owner_ref,omitempty"`
	Visibility         string          `json:"visibility"`
	OriginKey          string          `json:"origin_key"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	ValidFrom          *time.Time      `json:"valid_from,omitempty"`
	ValidUntil         *time.Time      `json:"valid_until,omitempty"`
	CreatedBy          *string         `json:"created_by,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	ArchivedAt         *time.Time      `json:"archived_at,omitempty"`
}

type GovernanceActorRole struct {
	ID           string          `json:"id"`
	ScopeID      string          `json:"scope_id"`
	ActorType    string          `json:"actor_type"`
	ActorUserID  *string         `json:"actor_user_id,omitempty"`
	ActorRef     *string         `json:"actor_ref,omitempty"`
	Role         string          `json:"role"`
	Permissions  []string        `json:"permissions"`
	PublicCredit string          `json:"public_credit"`
	ValidFrom    *time.Time      `json:"valid_from,omitempty"`
	ValidUntil   *time.Time      `json:"valid_until,omitempty"`
	CreatedBy    *string         `json:"created_by,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	RevokedAt    *time.Time      `json:"revoked_at,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

type GovernanceScopeTargetBinding struct {
	ID               string          `json:"id"`
	ScopeID          string          `json:"scope_id"`
	TargetKind       string          `json:"target_kind"`
	TargetID         string          `json:"target_id"`
	TargetVersionID  *string         `json:"target_version_id,omitempty"`
	BindingReason    string          `json:"binding_reason"`
	PermissionsScope string          `json:"permissions_scope"`
	Source           string          `json:"source"`
	ValidFrom        *time.Time      `json:"valid_from,omitempty"`
	ValidUntil       *time.Time      `json:"valid_until,omitempty"`
	AuditEventID     *string         `json:"audit_event_id,omitempty"`
	CreatedBy        *string         `json:"created_by,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	RevokedAt        *time.Time      `json:"revoked_at,omitempty"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
}

type GovernanceActorTarget struct {
	ActorType        string     `json:"actor_type"`
	ActorUserID      *string    `json:"actor_user_id,omitempty"`
	ActorRef         *string    `json:"actor_ref,omitempty"`
	ScopeID          string     `json:"scope_id"`
	TargetKind       string     `json:"target_kind"`
	TargetID         string     `json:"target_id"`
	Permissions      []string   `json:"permissions"`
	PermissionsScope string     `json:"permissions_scope"`
	ValidFrom        *time.Time `json:"valid_from,omitempty"`
	ValidUntil       *time.Time `json:"valid_until,omitempty"`
}

type GeoBBox struct {
	MinLon float64
	MinLat float64
	MaxLon float64
	MaxLat float64
}

type ListTargetObjectsParams struct {
	UserID                string
	IsAdmin               bool
	VisibilityMode        string
	VisibleAccessScopeIDs []string
	Kinds                 []string
	BBox                  *GeoBBox
	Search                string
	IncludeGeometry       bool
	Limit                 int
	Offset                int
}

type ListTargetCollectionsParams struct {
	UserID                string
	IsAdmin               bool
	VisibilityMode        string
	VisibleAccessScopeIDs []string
	CollectionKinds       []string
	BBox                  *GeoBBox
	Search                string
	Limit                 int
	Offset                int
}
