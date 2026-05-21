package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

func (db *DB) CreateTargetCollection(ctx context.Context, userID string, input model.TargetCollectionCreate) (*model.TargetCollection, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: owner user is required", ErrTargetValidation)
	}
	visibility := targetVisibilityOrDefault(input.Visibility)
	if err := db.validateTargetAccessForUserOwner(ctx, visibility, input.AccessScopeID, userID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.CollectionKind) == "" {
		return nil, fmt.Errorf("%w: name and collection kind are required", ErrTargetValidation)
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create target collection: %w", err)
	}
	defer tx.Rollback(ctx)

	collection, err := scanTargetCollection(tx.QueryRow(ctx, `
		INSERT INTO target_collections (
			owner_user_id, name, description, collection_kind, visibility,
			access_scope_id, display_style, default_sort, origin_key,
			external_ref_kind, external_ref_id, external_ref,
			external_ref_visibility, metadata, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $1)
		RETURNING `+targetCollectionColumnsSQL(""),
		userID, strings.TrimSpace(input.Name), input.Description, strings.TrimSpace(input.CollectionKind),
		visibility, input.AccessScopeID, targetDisplayStyle(input.DisplayStyle),
		targetDefaultSort(input.DefaultSort), targetOriginKey(input.OriginKey),
		input.ExternalRefKind, input.ExternalRefID, input.ExternalRef,
		targetExternalRefVisibility(input.ExternalRefVisibility), targetMetadata(input.Metadata),
	))
	if err != nil {
		return nil, fmt.Errorf("insert target collection: %w", err)
	}
	if err := rebuildTargetCollectionSummaryTx(ctx, tx, collection.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create target collection: %w", err)
	}
	return collection, nil
}

func (db *DB) UpdateTargetCollection(ctx context.Context, userID, id string, input model.TargetCollectionUpdate) (*model.TargetCollection, error) {
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	if userID == "" || id == "" {
		return nil, fmt.Errorf("%w: owner user and collection id are required", ErrTargetValidation)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update target collection: %w", err)
	}
	defer tx.Rollback(ctx)

	current, err := scanTargetCollection(tx.QueryRow(ctx, `
		SELECT `+targetCollectionColumnsSQL("t")+`
		FROM target_collections t
		WHERE id = $1 AND owner_user_id = $2 AND archived_at IS NULL
		FOR UPDATE
	`, id, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
		return nil, fmt.Errorf("load target collection: %w", err)
	}
	visibility := current.Visibility
	if input.Visibility != nil {
		visibility = targetVisibilityOrDefault(*input.Visibility)
	}
	accessScopeID := current.AccessScopeID
	if input.AccessScopeID != nil {
		accessScopeID = input.AccessScopeID
	}
	if visibility != "shared" {
		accessScopeID = nil
	}
	if err := db.validateTargetAccessForUserOwner(ctx, visibility, accessScopeID, userID); err != nil {
		return nil, err
	}
	name := current.Name
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	kind := current.CollectionKind
	if input.CollectionKind != nil {
		kind = strings.TrimSpace(*input.CollectionKind)
	}
	displayStyle := current.DisplayStyle
	if input.DisplayStyle != nil {
		displayStyle = targetDisplayStyle(*input.DisplayStyle)
	}
	defaultSort := current.DefaultSort
	if input.DefaultSort != nil {
		defaultSort = targetDefaultSort(*input.DefaultSort)
	}
	externalRefVisibility := current.ExternalRefVisibility
	if input.ExternalRefVisibility != nil {
		externalRefVisibility = targetExternalRefVisibility(*input.ExternalRefVisibility)
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = targetMetadata(*input.Metadata)
	}

	collection, err := scanTargetCollection(tx.QueryRow(ctx, `
		UPDATE target_collections
		SET name = $1,
		    description = COALESCE($2, description),
		    collection_kind = $3,
		    visibility = $4,
		    access_scope_id = $5,
		    display_style = $6,
		    default_sort = $7,
		    external_ref_kind = COALESCE($8, external_ref_kind),
		    external_ref_id = COALESCE($9, external_ref_id),
		    external_ref = COALESCE($10, external_ref),
		    external_ref_visibility = $11,
		    metadata = $12,
		    updated_at = NOW()
		WHERE id = $13 AND owner_user_id = $14 AND archived_at IS NULL
		RETURNING `+targetCollectionColumnsSQL(""),
		name, input.Description, kind, visibility, accessScopeID, displayStyle,
		defaultSort, input.ExternalRefKind, input.ExternalRefID, input.ExternalRef,
		externalRefVisibility, metadata, id, userID,
	))
	if err != nil {
		return nil, fmt.Errorf("update target collection: %w", err)
	}
	if err := rebuildTargetCollectionSummaryTx(ctx, tx, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update target collection: %w", err)
	}
	return collection, nil
}

func (db *DB) ArchiveTargetCollection(ctx context.Context, userID, id string) error {
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	if userID == "" || id == "" {
		return fmt.Errorf("%w: owner user and collection id are required", ErrTargetValidation)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin archive target collection: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE target_collections
		SET archived_at = COALESCE(archived_at, NOW()),
		    updated_at = NOW()
		WHERE id = $1 AND owner_user_id = $2 AND archived_at IS NULL
	`, id, userID)
	if err != nil {
		return fmt.Errorf("archive target collection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTargetNotFound
	}
	if err := rebuildTargetCollectionSummaryTx(ctx, tx, id); err != nil {
		return err
	}
	if err := rebuildCollectionsForMemberSourceTx(ctx, tx, "collection", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (db *DB) GetTargetCollection(ctx context.Context, userID, id string, visibleScopes []string, includeMembers bool) (*model.TargetCollectionDetail, error) {
	params := model.ListTargetCollectionsParams{UserID: userID, VisibleAccessScopeIDs: visibleScopes, Limit: 1}
	args, where := targetCollectionVisibilityWhere("s", params)
	args = append(args, id)
	collection, err := scanTargetCollection(db.Pool.QueryRow(ctx, `
		SELECT `+targetCollectionColumnsSQL("c")+`
		FROM target_collections c
		JOIN target_collection_summary_projection s ON s.collection_id = c.id
		WHERE c.id = $`+fmt.Sprint(len(args))+`
		  AND s.archived_at IS NULL
		  AND `+where+`
		LIMIT 1
	`, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
		return nil, fmt.Errorf("get target collection: %w", err)
	}
	detail := &model.TargetCollectionDetail{TargetCollection: *collection}
	if !includeMembers {
		return detail, nil
	}
	ownerView := params.IsAdmin || strings.TrimSpace(userID) != "" && collection.OwnerUserID != nil && *collection.OwnerUserID == strings.TrimSpace(userID)
	members, hidden, err := db.listTargetCollectionMembers(ctx, collection.ID, userID, visibleScopes, ownerView)
	if err != nil {
		return nil, err
	}
	detail.Members = members
	if ownerView {
		detail.HiddenMemberCount = hidden
	}
	return detail, nil
}

func (db *DB) ListTargetCollections(ctx context.Context, params model.ListTargetCollectionsParams) ([]model.TargetCollectionSummary, error) {
	args, where := targetCollectionVisibilityWhere("s", params)
	query := `
		SELECT collection_id, owner_user_id, lifecycle_owner_kind, lifecycle_owner_ref,
		       name, collection_kind, visibility, access_scope_id,
		       CASE WHEN bounds IS NULL THEN NULL ELSE ST_AsGeoJSON(bounds)::text END,
		       owner_member_count, public_member_count,
		       owner_counts_by_kind, owner_counts_by_role,
		       public_counts_by_kind, public_counts_by_role,
		       updated_at, archived_at
		FROM target_collection_summary_projection s
		WHERE s.archived_at IS NULL
		  AND ` + where
	if len(params.CollectionKinds) > 0 {
		args = append(args, params.CollectionKinds)
		query += fmt.Sprintf(" AND s.collection_kind = ANY($%d::text[])", len(args))
	}
	if strings.TrimSpace(params.Search) != "" {
		args = append(args, "%"+strings.TrimSpace(params.Search)+"%")
		query += fmt.Sprintf(" AND s.name ILIKE $%d", len(args))
	}
	if params.BBox != nil {
		args = append(args, params.BBox.MinLon, params.BBox.MinLat, params.BBox.MaxLon, params.BBox.MaxLat)
		query += fmt.Sprintf(" AND s.bounds && ST_MakeEnvelope($%d, $%d, $%d, $%d, 4326)", len(args)-3, len(args)-2, len(args)-1, len(args))
	}
	query += fmt.Sprintf(" ORDER BY s.updated_at DESC, s.collection_id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, targetLimit(params.Limit), targetOffset(params.Offset))

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list target collections: %w", err)
	}
	defer rows.Close()
	var out []model.TargetCollectionSummary
	for rows.Next() {
		var summary model.TargetCollectionSummary
		if err := rows.Scan(
			&summary.ID, &summary.OwnerUserID, &summary.LifecycleOwnerKind, &summary.LifecycleOwnerRef,
			&summary.Name, &summary.CollectionKind, &summary.Visibility, &summary.AccessScopeID,
			&summary.BoundsGeoJSON, &summary.OwnerMemberCount, &summary.PublicMemberCount,
			&summary.OwnerCountsByKind, &summary.OwnerCountsByRole,
			&summary.PublicCountsByKind, &summary.PublicCountsByRole,
			&summary.UpdatedAt, &summary.ArchivedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, rows.Err()
}

func (db *DB) AddTargetCollectionMember(ctx context.Context, userID, collectionID string, input model.TargetCollectionMemberCreate) (*model.TargetCollectionMember, error) {
	userID = strings.TrimSpace(userID)
	collectionID = strings.TrimSpace(collectionID)
	if userID == "" || collectionID == "" {
		return nil, fmt.Errorf("%w: owner user and collection id are required", ErrTargetValidation)
	}
	memberKind := strings.TrimSpace(input.MemberKind)
	if err := validateTargetMemberInput(memberKind, input.MemberID, input.MemberRef); err != nil {
		return nil, err
	}
	bindingMode := "live_current"
	if input.BindingMode != nil && strings.TrimSpace(*input.BindingMode) != "" {
		bindingMode = strings.TrimSpace(*input.BindingMode)
	}
	if memberKind == "external_ref" && bindingMode != "external_ref" {
		return nil, fmt.Errorf("%w: external_ref members require external_ref binding", ErrTargetValidation)
	}
	if bindingMode == "external_ref" && (input.MemberRef == nil || !strings.HasPrefix(strings.TrimSpace(*input.MemberRef), "external:")) {
		return nil, fmt.Errorf("%w: external members require an external: ref", ErrTargetValidation)
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin add target collection member: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := assertTargetCollectionOwnerTx(ctx, tx, collectionID, userID); err != nil {
		return nil, err
	}
	if err := db.validateTargetCollectionMemberVisibleTx(ctx, tx, userID, memberKind, input.MemberID); err != nil {
		return nil, err
	}
	if err := rejectRecursiveCollectionMemberTx(ctx, tx, collectionID, memberKind, input.MemberID); err != nil {
		return nil, err
	}
	member, err := scanTargetCollectionMember(tx.QueryRow(ctx, `
		INSERT INTO target_collection_members (
			collection_id, member_kind, member_id, member_ref, binding_mode,
			pinned_target_version_id, pinned_snapshot_id, member_owner_scope,
			member_visibility_snapshot, membership_visibility, origin_key,
			role, sort_order, display_name_override, metadata,
			valid_from, valid_until, added_by
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)
		RETURNING `+targetCollectionMemberColumnsSQL(""),
		collectionID, memberKind, input.MemberID, input.MemberRef, bindingMode,
		input.PinnedTargetVersionID, input.PinnedSnapshotID, targetDefault(input.MemberOwnerScope, "unknown"),
		targetDefault(input.MemberVisibilitySnapshot, "unknown"), targetDefault(input.MembershipVisibility, "inherit_collection"),
		targetOriginKey(input.OriginKey), targetDefault(input.Role, "member"), input.SortOrder,
		input.DisplayNameOverride, targetMetadata(input.Metadata), input.ValidFrom, input.ValidUntil, userID,
	))
	if err != nil {
		return nil, fmt.Errorf("insert target collection member: %w", err)
	}
	if err := rebuildTargetCollectionMemberReverseIndexTx(ctx, tx, member.ID); err != nil {
		return nil, err
	}
	if err := rebuildTargetCollectionSummaryTx(ctx, tx, collectionID); err != nil {
		return nil, err
	}
	if err := rebuildCollectionsForMemberSourceTx(ctx, tx, "collection", collectionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit add target collection member: %w", err)
	}
	return member, nil
}

func (db *DB) UpdateTargetCollectionMember(ctx context.Context, userID, collectionID, memberID string, input model.TargetCollectionMemberUpdate) (*model.TargetCollectionMember, error) {
	userID = strings.TrimSpace(userID)
	collectionID = strings.TrimSpace(collectionID)
	memberID = strings.TrimSpace(memberID)
	if userID == "" || collectionID == "" || memberID == "" {
		return nil, fmt.Errorf("%w: owner user, collection id, and member id are required", ErrTargetValidation)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update target collection member: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := assertTargetCollectionOwnerTx(ctx, tx, collectionID, userID); err != nil {
		return nil, err
	}
	current, err := scanTargetCollectionMember(tx.QueryRow(ctx, `
		SELECT `+targetCollectionMemberColumnsSQL("m")+`
		FROM target_collection_members m
		WHERE m.id = $1 AND m.collection_id = $2 AND m.removed_at IS NULL
		FOR UPDATE
	`, memberID, collectionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
		return nil, err
	}
	bindingMode := current.BindingMode
	if input.BindingMode != nil {
		bindingMode = strings.TrimSpace(*input.BindingMode)
	}
	role := current.Role
	if input.Role != nil {
		role = strings.TrimSpace(*input.Role)
	}
	sortOrder := current.SortOrder
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}
	membershipVisibility := current.MembershipVisibility
	if input.MembershipVisibility != nil {
		membershipVisibility = strings.TrimSpace(*input.MembershipVisibility)
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = targetMetadata(*input.Metadata)
	}
	member, err := scanTargetCollectionMember(tx.QueryRow(ctx, `
		UPDATE target_collection_members
		SET binding_mode = $1,
		    pinned_target_version_id = COALESCE($2, pinned_target_version_id),
		    pinned_snapshot_id = COALESCE($3, pinned_snapshot_id),
		    membership_visibility = $4,
		    role = $5,
		    sort_order = $6,
		    display_name_override = COALESCE($7, display_name_override),
		    metadata = $8,
		    valid_from = COALESCE($9, valid_from),
		    valid_until = COALESCE($10, valid_until)
		WHERE id = $11 AND collection_id = $12 AND removed_at IS NULL
		RETURNING `+targetCollectionMemberColumnsSQL(""),
		bindingMode, input.PinnedTargetVersionID, input.PinnedSnapshotID,
		membershipVisibility, role, sortOrder, input.DisplayNameOverride, metadata,
		input.ValidFrom, input.ValidUntil, memberID, collectionID,
	))
	if err != nil {
		return nil, fmt.Errorf("update target collection member: %w", err)
	}
	if err := rebuildTargetCollectionMemberReverseIndexTx(ctx, tx, member.ID); err != nil {
		return nil, err
	}
	if err := rebuildTargetCollectionSummaryTx(ctx, tx, collectionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update target collection member: %w", err)
	}
	return member, nil
}

func (db *DB) RemoveTargetCollectionMember(ctx context.Context, userID, collectionID, memberID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin remove target collection member: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := assertTargetCollectionOwnerTx(ctx, tx, collectionID, userID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE target_collection_members
		SET removed_at = COALESCE(removed_at, NOW())
		WHERE id = $1 AND collection_id = $2 AND removed_at IS NULL
	`, memberID, collectionID)
	if err != nil {
		return fmt.Errorf("remove target collection member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTargetNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM target_collection_member_reverse_index WHERE collection_member_id = $1`, memberID); err != nil {
		return err
	}
	if err := rebuildTargetCollectionSummaryTx(ctx, tx, collectionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func rebuildTargetCollectionMemberReverseIndexTx(ctx context.Context, tx pgx.Tx, collectionMemberID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM target_collection_member_reverse_index WHERE collection_member_id = $1`, collectionMemberID); err != nil {
		return fmt.Errorf("delete collection member reverse index: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO target_collection_member_reverse_index (
			member_kind, member_id, collection_id, collection_member_id, contributes_bounds
		)
		SELECT member_kind, member_id, collection_id, id,
		       member_kind IN ('course', 'place', 'region', 'collection')
		FROM target_collection_members
		WHERE id = $1
		  AND removed_at IS NULL
		  AND member_id IS NOT NULL
		  AND member_kind <> 'external_ref'
	`, collectionMemberID); err != nil {
		return fmt.Errorf("rebuild collection member reverse index: %w", err)
	}
	return nil
}

func rebuildCollectionsForMemberSourceTx(ctx context.Context, tx pgx.Tx, memberKind, memberID string) error {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT collection_id
		FROM target_collection_member_reverse_index
		WHERE member_kind = $1
		  AND member_id = $2
	`, memberKind, memberID)
	if err != nil {
		return fmt.Errorf("load affected collections: %w", err)
	}
	defer rows.Close()
	var collectionIDs []string
	for rows.Next() {
		var collectionID string
		if err := rows.Scan(&collectionID); err != nil {
			return err
		}
		collectionIDs = append(collectionIDs, collectionID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, collectionID := range collectionIDs {
		if err := rebuildTargetCollectionSummaryTx(ctx, tx, collectionID); err != nil {
			return err
		}
	}
	return nil
}

func rebuildTargetCollectionSummaryTx(ctx context.Context, tx pgx.Tx, collectionID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM target_collection_summary_projection WHERE collection_id = $1`, collectionID); err != nil {
		return fmt.Errorf("delete collection summary projection: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		WITH active_members AS (
			SELECT *
			FROM target_collection_members
			WHERE collection_id = $1
			  AND removed_at IS NULL
		),
		member_geoms AS (
			SELECT p.geom
			FROM active_members m
			JOIN target_places p ON m.member_kind = 'place' AND p.id = m.member_id AND p.archived_at IS NULL
			UNION ALL
			SELECT r.geom
			FROM active_members m
			JOIN target_regions r ON m.member_kind = 'region' AND r.id = m.member_id AND r.archived_at IS NULL
		),
		member_counts AS (
			SELECT
				COUNT(*)::int AS owner_member_count,
				COUNT(*) FILTER (
					WHERE membership_visibility <> 'owner_only'
					  AND (
						(member_kind = 'place' AND EXISTS (SELECT 1 FROM target_places p WHERE p.id = member_id AND p.visibility = 'public' AND p.archived_at IS NULL))
						OR (member_kind = 'region' AND EXISTS (SELECT 1 FROM target_regions r WHERE r.id = member_id AND r.visibility = 'public' AND r.archived_at IS NULL))
						OR (member_kind = 'collection' AND EXISTS (SELECT 1 FROM target_collections c WHERE c.id = member_id AND c.visibility = 'public' AND c.archived_at IS NULL))
					  )
				)::int AS public_member_count
			FROM active_members
		),
		bounds AS (
			SELECT CASE WHEN COUNT(*) = 0 THEN NULL ELSE ST_Envelope(ST_Collect(geom)) END AS geom
			FROM member_geoms
		)
		INSERT INTO target_collection_summary_projection (
			collection_id, owner_user_id, lifecycle_owner_kind, lifecycle_owner_ref,
			name, collection_kind, visibility, access_scope_id, bounds,
			owner_member_count, public_member_count, updated_at, archived_at
		)
		SELECT
			c.id, c.owner_user_id, c.lifecycle_owner_kind, c.lifecycle_owner_ref,
			c.name, c.collection_kind, c.visibility, c.access_scope_id, b.geom,
			COALESCE(mc.owner_member_count, 0), COALESCE(mc.public_member_count, 0),
			c.updated_at, c.archived_at
		FROM target_collections c
		CROSS JOIN bounds b
		CROSS JOIN member_counts mc
		WHERE c.id = $1
		  AND c.archived_at IS NULL
	`, collectionID); err != nil {
		return fmt.Errorf("rebuild collection summary projection: %w", err)
	}
	return nil
}

func (db *DB) CanEditTarget(ctx context.Context, userID, targetKind, targetID, permission string) (bool, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(targetKind) == "" || strings.TrimSpace(targetID) == "" {
		return false, nil
	}
	if err := validateGovernancePermissions("user", []string{permission}); err != nil {
		return false, err
	}
	table := ""
	switch strings.TrimSpace(targetKind) {
	case "place":
		table = "target_places"
	case "region":
		table = "target_regions"
	case "collection":
		table = "target_collections"
	default:
		return false, nil
	}
	var exists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM `+table+`
			WHERE id = $1
			  AND owner_user_id = $2
			  AND archived_at IS NULL
		)
	`, targetID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check target owner edit permission: %w", err)
	}
	if exists {
		return true, nil
	}
	targets, err := db.ListGovernanceEffectiveTargets(ctx, model.GovernanceActor{
		ActorType:   "user",
		ActorUserID: &userID,
	}, model.GovernanceEffectiveTargetFilter{
		TargetKind: targetKind,
		TargetID:   targetID,
		Permission: permission,
		Limit:      1,
	})
	if err != nil {
		return false, err
	}
	return len(targets) > 0, nil
}

func (db *DB) listTargetCollectionMembers(ctx context.Context, collectionID, userID string, visibleScopes []string, ownerView bool) ([]model.TargetCollectionMember, int, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+targetCollectionMemberColumnsSQL("m")+`
		FROM target_collection_members m
		WHERE m.collection_id = $1
		  AND m.removed_at IS NULL
		ORDER BY m.sort_order, m.added_at, m.id
	`, collectionID)
	if err != nil {
		return nil, 0, fmt.Errorf("list target collection members: %w", err)
	}
	defer rows.Close()
	var out []model.TargetCollectionMember
	hidden := 0
	for rows.Next() {
		member, err := scanTargetCollectionMember(rows)
		if err != nil {
			return nil, 0, err
		}
		visible := ownerView
		if !visible {
			visible, err = db.collectionMemberVisible(ctx, *member, userID, visibleScopes)
			if err != nil {
				return nil, 0, err
			}
		} else if member.MemberKind != "external_ref" {
			publicVisible, err := db.collectionMemberVisible(ctx, *member, "", nil)
			if err != nil {
				return nil, 0, err
			}
			if !publicVisible {
				hidden++
			}
		}
		if visible {
			out = append(out, *member)
		}
	}
	return out, hidden, rows.Err()
}

func (db *DB) collectionMemberVisible(ctx context.Context, member model.TargetCollectionMember, userID string, visibleScopes []string) (bool, error) {
	if member.MembershipVisibility == "owner_only" {
		return false, nil
	}
	if member.MemberID == nil {
		return member.MembershipVisibility == "public_safe", nil
	}
	switch member.MemberKind {
	case "place":
		return targetObjectVisibleByProjection(ctx, db, "place", *member.MemberID, userID, visibleScopes)
	case "region":
		return targetObjectVisibleByProjection(ctx, db, "region", *member.MemberID, userID, visibleScopes)
	case "collection":
		return targetCollectionVisibleBySummary(ctx, db, *member.MemberID, userID, visibleScopes)
	case "external_ref":
		return member.MembershipVisibility == "public_safe", nil
	default:
		return false, nil
	}
}

func targetObjectVisibleByProjection(ctx context.Context, db *DB, kind, id, userID string, visibleScopes []string) (bool, error) {
	var userArg any
	if strings.TrimSpace(userID) != "" {
		userArg = strings.TrimSpace(userID)
	}
	var exists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM target_object_search_projection
			WHERE object_kind = $1
			  AND object_id = $2
			  AND archived_at IS NULL
			  AND (
				visibility = 'public'
				OR owner_user_id = $3::uuid
				OR (visibility = 'shared' AND access_scope_id = ANY($4::uuid[]))
			  )
		)
	`, kind, id, userArg, visibleScopes).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func targetCollectionVisibleBySummary(ctx context.Context, db *DB, id, userID string, visibleScopes []string) (bool, error) {
	var userArg any
	if strings.TrimSpace(userID) != "" {
		userArg = strings.TrimSpace(userID)
	}
	var exists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM target_collection_summary_projection
			WHERE collection_id = $1
			  AND archived_at IS NULL
			  AND (
				visibility = 'public'
				OR owner_user_id = $2::uuid
				OR (visibility = 'shared' AND access_scope_id = ANY($3::uuid[]))
			  )
		)
	`, id, userArg, visibleScopes).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (db *DB) validateTargetCollectionMemberVisibleTx(ctx context.Context, tx pgx.Tx, userID, memberKind string, memberID *string) error {
	if memberKind == "external_ref" {
		return nil
	}
	if memberID == nil {
		return fmt.Errorf("%w: member id is required", ErrTargetValidation)
	}
	var exists bool
	switch memberKind {
	case "place":
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM target_places WHERE id = $1 AND archived_at IS NULL AND (visibility = 'public' OR owner_user_id = $2))`, *memberID, userID).Scan(&exists); err != nil {
			return err
		}
	case "region":
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM target_regions WHERE id = $1 AND archived_at IS NULL AND (visibility = 'public' OR owner_user_id = $2))`, *memberID, userID).Scan(&exists); err != nil {
			return err
		}
	case "collection":
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM target_collections WHERE id = $1 AND archived_at IS NULL AND (visibility = 'public' OR owner_user_id = $2))`, *memberID, userID).Scan(&exists); err != nil {
			return err
		}
	default:
		exists = true
	}
	if !exists {
		return ErrTargetNotFound
	}
	return nil
}

func rejectRecursiveCollectionMemberTx(ctx context.Context, tx pgx.Tx, collectionID, memberKind string, memberID *string) error {
	if memberKind != "collection" || memberID == nil {
		return nil
	}
	if *memberID == collectionID {
		return fmt.Errorf("%w: collection cannot contain itself", ErrTargetValidation)
	}
	var cycle bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM target_collection_members
			WHERE collection_id = $1
			  AND member_kind = 'collection'
			  AND member_id = $2
			  AND removed_at IS NULL
		)
	`, *memberID, collectionID).Scan(&cycle); err != nil {
		return err
	}
	if cycle {
		return fmt.Errorf("%w: recursive collection membership is not allowed", ErrTargetValidation)
	}
	return nil
}

func assertTargetCollectionOwnerTx(ctx context.Context, tx pgx.Tx, collectionID, userID string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM target_collections
			WHERE id = $1
			  AND owner_user_id = $2
			  AND archived_at IS NULL
		)
	`, collectionID, strings.TrimSpace(userID)).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrTargetNotFound
	}
	return nil
}

func targetCollectionVisibilityWhere(alias string, params model.ListTargetCollectionsParams) ([]any, string) {
	var userArg any
	if strings.TrimSpace(params.UserID) != "" {
		userArg = strings.TrimSpace(params.UserID)
	}
	args := []any{userArg, params.IsAdmin, params.VisibleAccessScopeIDs}
	return args, fmt.Sprintf(`(
		%[1]s.visibility = 'public'
		OR $2::boolean = true
		OR (%[1]s.owner_user_id = $1::uuid)
		OR (%[1]s.visibility = 'shared' AND %[1]s.access_scope_id = ANY($3::uuid[]))
	)`, alias)
}

func targetCollectionColumnsSQL(alias string) string {
	prefix := targetColumnPrefix(alias)
	return prefix + `id, ` + prefix + `owner_user_id, ` + prefix + `lifecycle_owner_kind, ` + prefix + `lifecycle_owner_ref,
		` + prefix + `name, ` + prefix + `description, ` + prefix + `collection_kind,
		` + prefix + `visibility, ` + prefix + `access_scope_id, ` + prefix + `display_style,
		` + prefix + `default_sort, ` + prefix + `origin_key, ` + prefix + `external_ref_kind,
		` + prefix + `external_ref_id, ` + prefix + `external_ref, ` + prefix + `external_ref_visibility,
		` + prefix + `metadata, ` + prefix + `created_by, ` + prefix + `created_at,
		` + prefix + `updated_at, ` + prefix + `archived_at`
}

func targetCollectionMemberColumnsSQL(alias string) string {
	prefix := targetColumnPrefix(alias)
	return prefix + `id, ` + prefix + `collection_id, ` + prefix + `member_kind,
		` + prefix + `member_id, ` + prefix + `member_ref, ` + prefix + `binding_mode,
		` + prefix + `pinned_target_version_id, ` + prefix + `pinned_snapshot_id,
		` + prefix + `member_owner_scope, ` + prefix + `member_visibility_snapshot,
		` + prefix + `membership_visibility, ` + prefix + `origin_key, ` + prefix + `role,
		` + prefix + `sort_order, ` + prefix + `display_name_override, ` + prefix + `metadata,
		` + prefix + `valid_from, ` + prefix + `valid_until, ` + prefix + `added_by,
		` + prefix + `added_at, ` + prefix + `removed_at`
}

func scanTargetCollection(row pgx.Row) (*model.TargetCollection, error) {
	var collection model.TargetCollection
	if err := row.Scan(
		&collection.ID, &collection.OwnerUserID, &collection.LifecycleOwnerKind, &collection.LifecycleOwnerRef,
		&collection.Name, &collection.Description, &collection.CollectionKind,
		&collection.Visibility, &collection.AccessScopeID, &collection.DisplayStyle,
		&collection.DefaultSort, &collection.OriginKey, &collection.ExternalRefKind,
		&collection.ExternalRefID, &collection.ExternalRef, &collection.ExternalRefVisibility,
		&collection.Metadata, &collection.CreatedBy, &collection.CreatedAt,
		&collection.UpdatedAt, &collection.ArchivedAt,
	); err != nil {
		return nil, err
	}
	return &collection, nil
}

func scanTargetCollectionMember(row pgx.Row) (*model.TargetCollectionMember, error) {
	var member model.TargetCollectionMember
	if err := row.Scan(
		&member.ID, &member.CollectionID, &member.MemberKind,
		&member.MemberID, &member.MemberRef, &member.BindingMode,
		&member.PinnedTargetVersionID, &member.PinnedSnapshotID,
		&member.MemberOwnerScope, &member.MemberVisibilitySnapshot,
		&member.MembershipVisibility, &member.OriginKey, &member.Role,
		&member.SortOrder, &member.DisplayNameOverride, &member.Metadata,
		&member.ValidFrom, &member.ValidUntil, &member.AddedBy,
		&member.AddedAt, &member.RemovedAt,
	); err != nil {
		return nil, err
	}
	return &member, nil
}

func targetDisplayStyle(value string) string {
	return targetDefault(value, "map_list")
}

func targetDefaultSort(value string) string {
	return targetDefault(value, "manual")
}

func targetDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func targetJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}
