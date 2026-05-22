package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

var ErrTargetNotFound = errors.New("target not found")

func (db *DB) CreateTargetPlace(ctx context.Context, userID string, input model.TargetPlaceCreate) (*model.TargetPlace, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: owner user is required", ErrTargetValidation)
	}
	visibility := targetVisibilityOrDefault(input.Visibility)
	if err := db.validateTargetAccessForUserOwner(ctx, visibility, input.AccessScopeID, userID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.PlaceKind) == "" || strings.TrimSpace(input.GeometryGeoJSON) == "" {
		return nil, fmt.Errorf("%w: name, place kind, and geometry are required", ErrTargetValidation)
	}
	if err := validateTargetPlaceInput(input.PlaceKind, input.GeometryGeoJSON); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.ExternalRefVisibility) != "" {
		if err := validateTargetEnum(input.ExternalRefVisibility, "external_ref_visibility", allowedTargetExternalRefVisibility); err != nil {
			return nil, err
		}
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create target place: %w", err)
	}
	defer tx.Rollback(ctx)

	place, err := insertTargetPlaceTx(ctx, tx, userID, input, visibility)
	if err != nil {
		return nil, err
	}
	if err := rebuildTargetPlaceProjectionTx(ctx, tx, place.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create target place: %w", err)
	}
	return place, nil
}

func (db *DB) UpdateTargetPlace(ctx context.Context, userID, id string, input model.TargetPlaceUpdate) (*model.TargetPlace, error) {
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	if userID == "" || id == "" {
		return nil, fmt.Errorf("%w: owner user and place id are required", ErrTargetValidation)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update target place: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentVisibility string
	var currentScope *string
	var ownerUserID *string
	if err := tx.QueryRow(ctx, `
		SELECT visibility, access_scope_id, owner_user_id
		FROM target_places
		WHERE id = $1 AND archived_at IS NULL
		FOR UPDATE
	`, id).Scan(&currentVisibility, &currentScope, &ownerUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
		return nil, fmt.Errorf("load target place for update: %w", err)
	}
	if input.GeometryGeoJSON != nil {
		if err := validateTargetPlaceInput(targetPlaceKindForValidation(input.PlaceKind), *input.GeometryGeoJSON); err != nil {
			return nil, err
		}
	}
	if input.PlaceKind != nil {
		if err := validateTargetEnum(*input.PlaceKind, "place_kind", allowedTargetPlaceKinds); err != nil {
			return nil, err
		}
	}
	if input.ExternalRefVisibility != nil {
		if err := validateTargetEnum(*input.ExternalRefVisibility, "external_ref_visibility", allowedTargetExternalRefVisibility); err != nil {
			return nil, err
		}
	}
	if err := db.requireTargetObjectUpdatePermissions(ctx, userID, "place", id, ownerUserID, targetPlaceUpdatePermissions(input)); err != nil {
		return nil, err
	}
	nextVisibility := currentVisibility
	if input.Visibility != nil {
		nextVisibility = targetVisibilityOrDefault(*input.Visibility)
	}
	nextScope := currentScope
	if input.AccessScopeID != nil {
		nextScope = input.AccessScopeID
	}
	if nextVisibility != "shared" {
		nextScope = nil
	}
	if err := db.validateTargetAccessForExistingOwner(ctx, nextVisibility, nextScope, ownerUserID); err != nil {
		return nil, err
	}

	place, err := updateTargetPlaceTx(ctx, tx, userID, id, input, nextVisibility, nextScope)
	if err != nil {
		return nil, err
	}
	if err := rebuildTargetPlaceProjectionTx(ctx, tx, id); err != nil {
		return nil, err
	}
	if err := rebuildCollectionsForMemberSourceTx(ctx, tx, "place", id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update target place: %w", err)
	}
	return place, nil
}

func (db *DB) ArchiveTargetPlace(ctx context.Context, userID, id string) error {
	return db.archiveTargetObject(ctx, "target_places", "place", userID, id, rebuildTargetPlaceProjectionTx)
}

func (db *DB) GetTargetPlace(ctx context.Context, userID, id string, visibleScopes []string) (*model.TargetPlace, error) {
	return db.GetTargetPlaceWithAccess(ctx, userID, id, visibleScopes, false)
}

func (db *DB) GetTargetPlaceWithAccess(ctx context.Context, userID, id string, visibleScopes []string, isAdmin bool) (*model.TargetPlace, error) {
	params := model.ListTargetObjectsParams{UserID: userID, IsAdmin: isAdmin, VisibleAccessScopeIDs: visibleScopes, IncludeGeometry: true, Limit: 1}
	args, where := targetObjectVisibilityWhere("p", "owner_user_id", params)
	args = append(args, id)
	query := targetPlaceSelectSQLWithEffectivePermissions("$1::uuid") + `
		JOIN target_object_search_projection p ON p.object_kind = 'place' AND p.object_id = t.id
		WHERE p.object_kind = 'place'
		  AND p.archived_at IS NULL
		  AND t.id = $` + fmt.Sprint(len(args)) + `
		  AND ` + where + `
		LIMIT 1`
	place, err := scanTargetPlaceWithEffectivePermissions(db.Pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if place != nil {
		redactTargetPlaceForViewer(place, params)
	}
	return place, err
}

func (db *DB) ListTargetPlaces(ctx context.Context, params model.ListTargetObjectsParams) ([]model.TargetPlace, error) {
	args, where := targetObjectVisibilityWhere("p", "owner_user_id", params)
	query := targetPlaceSelectSQLWithEffectivePermissions("$1::uuid") + `
		JOIN target_object_search_projection p ON p.object_kind = 'place' AND p.object_id = t.id
		WHERE p.object_kind = 'place'
		  AND p.archived_at IS NULL
		  AND ` + where
	query = appendTargetObjectFilters(query, &args, params, "p")
	query += fmt.Sprintf(" ORDER BY p.updated_at DESC, p.object_id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, targetLimit(params.Limit), targetOffset(params.Offset))

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list target places: %w", err)
	}
	defer rows.Close()

	places := []model.TargetPlace{}
	for rows.Next() {
		place, err := scanTargetPlaceWithEffectivePermissions(rows)
		if err != nil {
			return nil, err
		}
		redactTargetPlaceForViewer(place, params)
		if !params.IncludeGeometry {
			place.GeometryGeoJSON = ""
		}
		places = append(places, *place)
	}
	return places, rows.Err()
}

func (db *DB) CreateTargetRegion(ctx context.Context, userID string, input model.TargetRegionCreate) (*model.TargetRegion, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: owner user is required", ErrTargetValidation)
	}
	visibility := targetVisibilityOrDefault(input.Visibility)
	if err := db.validateTargetAccessForUserOwner(ctx, visibility, input.AccessScopeID, userID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.RegionKind) == "" || strings.TrimSpace(input.GeometryGeoJSON) == "" {
		return nil, fmt.Errorf("%w: name, region kind, and geometry are required", ErrTargetValidation)
	}
	if err := validateTargetRegionInput(input.RegionKind, input.GeometryGeoJSON); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.ExternalRefVisibility) != "" {
		if err := validateTargetEnum(input.ExternalRefVisibility, "external_ref_visibility", allowedTargetExternalRefVisibility); err != nil {
			return nil, err
		}
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create target region: %w", err)
	}
	defer tx.Rollback(ctx)

	region, err := insertTargetRegionTx(ctx, tx, userID, input, visibility)
	if err != nil {
		return nil, err
	}
	if err := rebuildTargetRegionProjectionTx(ctx, tx, region.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create target region: %w", err)
	}
	return region, nil
}

func (db *DB) UpdateTargetRegion(ctx context.Context, userID, id string, input model.TargetRegionUpdate) (*model.TargetRegion, error) {
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	if userID == "" || id == "" {
		return nil, fmt.Errorf("%w: owner user and region id are required", ErrTargetValidation)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update target region: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentVisibility string
	var currentScope *string
	var ownerUserID *string
	if err := tx.QueryRow(ctx, `
		SELECT visibility, access_scope_id, owner_user_id
		FROM target_regions
		WHERE id = $1 AND archived_at IS NULL
		FOR UPDATE
	`, id).Scan(&currentVisibility, &currentScope, &ownerUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
		return nil, fmt.Errorf("load target region for update: %w", err)
	}
	if input.GeometryGeoJSON != nil {
		if err := validateTargetRegionInput(targetRegionKindForValidation(input.RegionKind), *input.GeometryGeoJSON); err != nil {
			return nil, err
		}
	}
	if input.RegionKind != nil {
		if err := validateTargetEnum(*input.RegionKind, "region_kind", allowedTargetRegionKinds); err != nil {
			return nil, err
		}
	}
	if input.ExternalRefVisibility != nil {
		if err := validateTargetEnum(*input.ExternalRefVisibility, "external_ref_visibility", allowedTargetExternalRefVisibility); err != nil {
			return nil, err
		}
	}
	if err := db.requireTargetObjectUpdatePermissions(ctx, userID, "region", id, ownerUserID, targetRegionUpdatePermissions(input)); err != nil {
		return nil, err
	}
	nextVisibility := currentVisibility
	if input.Visibility != nil {
		nextVisibility = targetVisibilityOrDefault(*input.Visibility)
	}
	nextScope := currentScope
	if input.AccessScopeID != nil {
		nextScope = input.AccessScopeID
	}
	if nextVisibility != "shared" {
		nextScope = nil
	}
	if err := db.validateTargetAccessForExistingOwner(ctx, nextVisibility, nextScope, ownerUserID); err != nil {
		return nil, err
	}

	region, err := updateTargetRegionTx(ctx, tx, userID, id, input, nextVisibility, nextScope)
	if err != nil {
		return nil, err
	}
	if err := rebuildTargetRegionProjectionTx(ctx, tx, id); err != nil {
		return nil, err
	}
	if err := rebuildCollectionsForMemberSourceTx(ctx, tx, "region", id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update target region: %w", err)
	}
	return region, nil
}

func (db *DB) ArchiveTargetRegion(ctx context.Context, userID, id string) error {
	return db.archiveTargetObject(ctx, "target_regions", "region", userID, id, rebuildTargetRegionProjectionTx)
}

func (db *DB) GetTargetRegion(ctx context.Context, userID, id string, visibleScopes []string) (*model.TargetRegion, error) {
	return db.GetTargetRegionWithAccess(ctx, userID, id, visibleScopes, false)
}

func (db *DB) GetTargetRegionWithAccess(ctx context.Context, userID, id string, visibleScopes []string, isAdmin bool) (*model.TargetRegion, error) {
	params := model.ListTargetObjectsParams{UserID: userID, IsAdmin: isAdmin, VisibleAccessScopeIDs: visibleScopes, IncludeGeometry: true, Limit: 1}
	args, where := targetObjectVisibilityWhere("p", "owner_user_id", params)
	args = append(args, id)
	query := targetRegionSelectSQLWithEffectivePermissions("$1::uuid") + `
		JOIN target_object_search_projection p ON p.object_kind = 'region' AND p.object_id = t.id
		WHERE p.object_kind = 'region'
		  AND p.archived_at IS NULL
		  AND t.id = $` + fmt.Sprint(len(args)) + `
		  AND ` + where + `
		LIMIT 1`
	region, err := scanTargetRegionWithEffectivePermissions(db.Pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if region != nil {
		redactTargetRegionForViewer(region, params)
	}
	return region, err
}

func (db *DB) ListTargetRegions(ctx context.Context, params model.ListTargetObjectsParams) ([]model.TargetRegion, error) {
	args, where := targetObjectVisibilityWhere("p", "owner_user_id", params)
	query := targetRegionSelectSQLWithEffectivePermissions("$1::uuid") + `
		JOIN target_object_search_projection p ON p.object_kind = 'region' AND p.object_id = t.id
		WHERE p.object_kind = 'region'
		  AND p.archived_at IS NULL
		  AND ` + where
	query = appendTargetObjectFilters(query, &args, params, "p")
	query += fmt.Sprintf(" ORDER BY p.updated_at DESC, p.object_id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, targetLimit(params.Limit), targetOffset(params.Offset))

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list target regions: %w", err)
	}
	defer rows.Close()

	regions := []model.TargetRegion{}
	for rows.Next() {
		region, err := scanTargetRegionWithEffectivePermissions(rows)
		if err != nil {
			return nil, err
		}
		redactTargetRegionForViewer(region, params)
		if !params.IncludeGeometry {
			region.GeometryGeoJSON = ""
		}
		regions = append(regions, *region)
	}
	return regions, rows.Err()
}

func (db *DB) validateTargetAccessForUserOwner(ctx context.Context, visibility string, accessScopeID *string, userID string) error {
	if err := validateTargetVisibilityAccessPair(visibility, accessScopeID); err != nil {
		return err
	}
	if visibility == "shared" {
		if err := db.ValidateObjectAccessScopeForTargetOwner(ctx, *accessScopeID, "user", userID, ""); err != nil {
			return fmt.Errorf("%w: %v", ErrTargetValidation, err)
		}
	}
	return nil
}

func insertTargetPlaceTx(ctx context.Context, tx pgx.Tx, userID string, input model.TargetPlaceCreate, visibility string) (*model.TargetPlace, error) {
	return scanTargetPlace(tx.QueryRow(ctx, `
		INSERT INTO target_places (
			owner_user_id, name, description, place_kind, geom, visibility,
			access_scope_id, origin_key, external_ref_kind, external_ref_id,
			external_ref, external_ref_visibility, metadata, created_by
		)
		VALUES (
			$1, $2, $3, $4, ST_SetSRID(ST_GeomFromGeoJSON($5), 4326),
			$6, $7, $8, $9, $10, $11, $12, $13, $1
		)
		RETURNING `+targetPlaceColumnsSQL(""),
		userID, strings.TrimSpace(input.Name), input.Description, strings.TrimSpace(input.PlaceKind),
		input.GeometryGeoJSON, visibility, input.AccessScopeID, targetOriginKey(input.OriginKey),
		input.ExternalRefKind, input.ExternalRefID, input.ExternalRef,
		targetExternalRefVisibility(input.ExternalRefVisibility), targetMetadata(input.Metadata),
	))
}

func updateTargetPlaceTx(ctx context.Context, tx pgx.Tx, userID, id string, input model.TargetPlaceUpdate, visibility string, accessScopeID *string) (*model.TargetPlace, error) {
	current, err := scanTargetPlace(tx.QueryRow(ctx, `SELECT `+targetPlaceColumnsSQL("t")+` FROM target_places t WHERE id = $1 AND archived_at IS NULL`, id))
	if err != nil {
		return nil, err
	}
	name := current.Name
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	placeKind := current.PlaceKind
	if input.PlaceKind != nil {
		placeKind = strings.TrimSpace(*input.PlaceKind)
	}
	externalRefVisibility := current.ExternalRefVisibility
	if input.ExternalRefVisibility != nil {
		externalRefVisibility = targetExternalRefVisibility(*input.ExternalRefVisibility)
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = targetMetadata(*input.Metadata)
	}
	geometryExpr := "geom"
	args := []any{name, input.Description, placeKind, visibility, accessScopeID, input.ExternalRefKind, input.ExternalRefID, input.ExternalRef, externalRefVisibility, metadata, id}
	if input.GeometryGeoJSON != nil {
		geometryExpr = fmt.Sprintf("ST_SetSRID(ST_GeomFromGeoJSON($%d), 4326)", len(args)+1)
		args = append(args, *input.GeometryGeoJSON)
	}
	return scanTargetPlace(tx.QueryRow(ctx, `
		UPDATE target_places
		SET name = $1,
		    description = COALESCE($2, description),
		    place_kind = $3,
		    geom = `+geometryExpr+`,
		    visibility = $4,
		    access_scope_id = $5,
		    external_ref_kind = COALESCE($6, external_ref_kind),
		    external_ref_id = COALESCE($7, external_ref_id),
		    external_ref = COALESCE($8, external_ref),
		    external_ref_visibility = $9,
		    metadata = $10,
		    updated_at = NOW()
		WHERE id = $11 AND archived_at IS NULL
		RETURNING `+targetPlaceColumnsSQL(""), args...))
}

func insertTargetRegionTx(ctx context.Context, tx pgx.Tx, userID string, input model.TargetRegionCreate, visibility string) (*model.TargetRegion, error) {
	return scanTargetRegion(tx.QueryRow(ctx, `
		INSERT INTO target_regions (
			owner_user_id, name, description, region_kind, geom, visibility,
			access_scope_id, origin_key, external_ref_kind, external_ref_id,
			external_ref, external_ref_visibility, metadata, created_by
		)
		VALUES (
			$1, $2, $3, $4, ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON($5), 4326)),
			$6, $7, $8, $9, $10, $11, $12, $13, $1
		)
		RETURNING `+targetRegionColumnsSQL(""),
		userID, strings.TrimSpace(input.Name), input.Description, strings.TrimSpace(input.RegionKind),
		input.GeometryGeoJSON, visibility, input.AccessScopeID, targetOriginKey(input.OriginKey),
		input.ExternalRefKind, input.ExternalRefID, input.ExternalRef,
		targetExternalRefVisibility(input.ExternalRefVisibility), targetMetadata(input.Metadata),
	))
}

func updateTargetRegionTx(ctx context.Context, tx pgx.Tx, userID, id string, input model.TargetRegionUpdate, visibility string, accessScopeID *string) (*model.TargetRegion, error) {
	current, err := scanTargetRegion(tx.QueryRow(ctx, `SELECT `+targetRegionColumnsSQL("t")+` FROM target_regions t WHERE id = $1 AND archived_at IS NULL`, id))
	if err != nil {
		return nil, err
	}
	name := current.Name
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	regionKind := current.RegionKind
	if input.RegionKind != nil {
		regionKind = strings.TrimSpace(*input.RegionKind)
	}
	externalRefVisibility := current.ExternalRefVisibility
	if input.ExternalRefVisibility != nil {
		externalRefVisibility = targetExternalRefVisibility(*input.ExternalRefVisibility)
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = targetMetadata(*input.Metadata)
	}
	geometryExpr := "geom"
	args := []any{name, input.Description, regionKind, visibility, accessScopeID, input.ExternalRefKind, input.ExternalRefID, input.ExternalRef, externalRefVisibility, metadata, id}
	if input.GeometryGeoJSON != nil {
		geometryExpr = fmt.Sprintf("ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON($%d), 4326))", len(args)+1)
		args = append(args, *input.GeometryGeoJSON)
	}
	return scanTargetRegion(tx.QueryRow(ctx, `
		UPDATE target_regions
		SET name = $1,
		    description = COALESCE($2, description),
		    region_kind = $3,
		    geom = `+geometryExpr+`,
		    visibility = $4,
		    access_scope_id = $5,
		    external_ref_kind = COALESCE($6, external_ref_kind),
		    external_ref_id = COALESCE($7, external_ref_id),
		    external_ref = COALESCE($8, external_ref),
		    external_ref_visibility = $9,
		    metadata = $10,
		    updated_at = NOW()
		WHERE id = $11 AND archived_at IS NULL
		RETURNING `+targetRegionColumnsSQL(""), args...))
}

func (db *DB) archiveTargetObject(ctx context.Context, table, projectionKind, userID, id string, rebuild func(context.Context, pgx.Tx, string) error) error {
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	if userID == "" || id == "" {
		return fmt.Errorf("%w: owner user and object id are required", ErrTargetValidation)
	}
	canEdit, err := db.CanEditTarget(ctx, userID, projectionKind, id, "hide_or_archive")
	if err != nil {
		return err
	}
	if !canEdit {
		return ErrTargetNotFound
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin archive target object: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE `+table+`
		SET archived_at = COALESCE(archived_at, NOW()),
		    updated_at = NOW()
		WHERE id = $1 AND archived_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("archive %s: %w", projectionKind, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTargetNotFound
	}
	if err := rebuild(ctx, tx, id); err != nil {
		return err
	}
	if err := rebuildCollectionsForMemberSourceTx(ctx, tx, projectionKind, id); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit archive target object: %w", err)
	}
	return nil
}

func rebuildTargetPlaceProjectionTx(ctx context.Context, tx pgx.Tx, placeID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM target_object_search_projection WHERE object_kind = 'place' AND object_id = $1`, placeID); err != nil {
		return fmt.Errorf("delete target place projection: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO target_object_search_projection (
			object_kind, object_id, owner_user_id, lifecycle_owner_kind, lifecycle_owner_ref,
			name, object_subkind, visibility, access_scope_id, geom, bounds, centroid,
			external_ref_kind, updated_at, archived_at
		)
		SELECT
			'place', id, owner_user_id, lifecycle_owner_kind, lifecycle_owner_ref,
			name, place_kind, visibility, access_scope_id, geom, geom, geom,
			CASE WHEN external_ref_visibility IN ('shared_safe', 'public_safe') THEN external_ref_kind ELSE NULL END,
			updated_at, archived_at
		FROM target_places
		WHERE id = $1
		  AND archived_at IS NULL
	`, placeID); err != nil {
		return fmt.Errorf("rebuild target place projection: %w", err)
	}
	return nil
}

func rebuildTargetRegionProjectionTx(ctx context.Context, tx pgx.Tx, regionID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM target_object_search_projection WHERE object_kind = 'region' AND object_id = $1`, regionID); err != nil {
		return fmt.Errorf("delete target region projection: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO target_object_search_projection (
			object_kind, object_id, owner_user_id, lifecycle_owner_kind, lifecycle_owner_ref,
			name, object_subkind, visibility, access_scope_id, geom, bounds, centroid,
			external_ref_kind, updated_at, archived_at
		)
		SELECT
			'region', id, owner_user_id, lifecycle_owner_kind, lifecycle_owner_ref,
			name, region_kind, visibility, access_scope_id, geom, ST_Envelope(geom), ST_PointOnSurface(geom),
			CASE WHEN external_ref_visibility IN ('shared_safe', 'public_safe') THEN external_ref_kind ELSE NULL END,
			updated_at, archived_at
		FROM target_regions
		WHERE id = $1
		  AND archived_at IS NULL
	`, regionID); err != nil {
		return fmt.Errorf("rebuild target region projection: %w", err)
	}
	return nil
}

func targetObjectVisibilityWhere(alias, ownerColumn string, params model.ListTargetObjectsParams) ([]any, string) {
	var userArg any
	if strings.TrimSpace(params.UserID) != "" {
		userArg = strings.TrimSpace(params.UserID)
	}
	scopes := params.VisibleAccessScopeIDs
	args := []any{userArg, params.IsAdmin, scopes}
	base := fmt.Sprintf(`(
		%[1]s.visibility = 'public'
		OR $2::boolean = true
		OR (%[1]s.%[2]s = $1::uuid)
		OR (%[1]s.visibility = 'shared' AND %[1]s.access_scope_id = ANY($3::uuid[]))
		OR `+governanceTargetVisibleSQL(alias+".object_kind", alias+".object_id", "$1::uuid")+`
	)`, alias, ownerColumn)
	switch strings.TrimSpace(params.VisibilityMode) {
	case "", "visible", "all":
		return args, base
	case "public", "shared", "private":
		return args, fmt.Sprintf("(%s.visibility = '%s' AND %s)", alias, strings.TrimSpace(params.VisibilityMode), base)
	default:
		return args, "FALSE"
	}
}

func appendTargetObjectFilters(query string, args *[]any, params model.ListTargetObjectsParams, alias string) string {
	if len(params.Kinds) > 0 {
		*args = append(*args, params.Kinds)
		query += fmt.Sprintf(" AND %s.object_subkind = ANY($%d::text[])", alias, len(*args))
	}
	if params.Search != "" {
		*args = append(*args, "%"+strings.TrimSpace(params.Search)+"%")
		query += fmt.Sprintf(" AND %s.name ILIKE $%d", alias, len(*args))
	}
	if params.BBox != nil {
		*args = append(*args, params.BBox.MinLon, params.BBox.MinLat, params.BBox.MaxLon, params.BBox.MaxLat)
		query += fmt.Sprintf(" AND %s.bounds && ST_MakeEnvelope($%d, $%d, $%d, $%d, 4326)", alias, len(*args)-3, len(*args)-2, len(*args)-1, len(*args))
	}
	return query
}

func targetPlaceSelectSQL() string {
	return `SELECT ` + targetPlaceColumnsSQL("t") + ` FROM target_places t `
}

func targetPlaceSelectSQLWithEffectivePermissions(actorUserExpr string) string {
	return `SELECT ` + targetPlaceColumnsSQL("t") + `, ` + governanceEffectivePermissionsSQL("place", "t.id", actorUserExpr) + ` FROM target_places t `
}

func targetRegionSelectSQL() string {
	return `SELECT ` + targetRegionColumnsSQL("t") + ` FROM target_regions t `
}

func targetRegionSelectSQLWithEffectivePermissions(actorUserExpr string) string {
	return `SELECT ` + targetRegionColumnsSQL("t") + `, ` + governanceEffectivePermissionsSQL("region", "t.id", actorUserExpr) + ` FROM target_regions t `
}

func targetPlaceColumnsSQL(alias string) string {
	prefix := targetColumnPrefix(alias)
	return prefix + `id, ` + prefix + `owner_user_id, ` + prefix + `lifecycle_owner_kind, ` + prefix + `lifecycle_owner_ref,
		` + prefix + `name, ` + prefix + `description, ` + prefix + `place_kind, ST_AsGeoJSON(` + prefix + `geom)::text,
		` + prefix + `visibility, ` + prefix + `access_scope_id, ` + prefix + `origin_key, ` + prefix + `external_ref_kind,
		` + prefix + `external_ref_id, ` + prefix + `external_ref, ` + prefix + `external_ref_visibility, ` + prefix + `metadata,
		` + prefix + `created_by, ` + prefix + `created_at, ` + prefix + `updated_at, ` + prefix + `archived_at`
}

func targetRegionColumnsSQL(alias string) string {
	prefix := targetColumnPrefix(alias)
	return prefix + `id, ` + prefix + `owner_user_id, ` + prefix + `lifecycle_owner_kind, ` + prefix + `lifecycle_owner_ref,
		` + prefix + `name, ` + prefix + `description, ` + prefix + `region_kind, ST_AsGeoJSON(` + prefix + `geom)::text,
		` + prefix + `visibility, ` + prefix + `access_scope_id, ` + prefix + `origin_key, ` + prefix + `external_ref_kind,
		` + prefix + `external_ref_id, ` + prefix + `external_ref, ` + prefix + `external_ref_visibility, ` + prefix + `metadata,
		` + prefix + `created_by, ` + prefix + `created_at, ` + prefix + `updated_at, ` + prefix + `archived_at`
}

func redactTargetPlaceForViewer(place *model.TargetPlace, params model.ListTargetObjectsParams) {
	if targetObjectCanSeeExternalRef(place.OwnerUserID, place.Visibility, place.ExternalRefVisibility, params.UserID, params.IsAdmin, place.EffectivePermissions) {
		return
	}
	place.ExternalRefKind = nil
	place.ExternalRefID = nil
	place.ExternalRef = nil
}

func redactTargetRegionForViewer(region *model.TargetRegion, params model.ListTargetObjectsParams) {
	if targetObjectCanSeeExternalRef(region.OwnerUserID, region.Visibility, region.ExternalRefVisibility, params.UserID, params.IsAdmin, region.EffectivePermissions) {
		return
	}
	region.ExternalRefKind = nil
	region.ExternalRefID = nil
	region.ExternalRef = nil
}

func targetObjectCanSeeExternalRef(ownerUserID *string, visibility, refVisibility, userID string, isAdmin bool, effectivePermissions []string) bool {
	if isAdmin {
		return true
	}
	if ownerUserID != nil && strings.TrimSpace(userID) != "" && *ownerUserID == strings.TrimSpace(userID) {
		return true
	}
	if slices.Contains(effectivePermissions, "link_external_refs") {
		return true
	}
	switch strings.TrimSpace(refVisibility) {
	case "public_safe":
		return true
	case "shared_safe":
		return strings.TrimSpace(visibility) == "shared" && strings.TrimSpace(userID) != ""
	default:
		return false
	}
}

func targetColumnPrefix(alias string) string {
	if strings.TrimSpace(alias) == "" {
		return ""
	}
	return strings.TrimSpace(alias) + "."
}

func scanTargetPlace(row pgx.Row) (*model.TargetPlace, error) {
	var place model.TargetPlace
	if err := row.Scan(
		&place.ID, &place.OwnerUserID, &place.LifecycleOwnerKind, &place.LifecycleOwnerRef,
		&place.Name, &place.Description, &place.PlaceKind, &place.GeometryGeoJSON,
		&place.Visibility, &place.AccessScopeID, &place.OriginKey, &place.ExternalRefKind,
		&place.ExternalRefID, &place.ExternalRef, &place.ExternalRefVisibility, &place.Metadata,
		&place.CreatedBy, &place.CreatedAt, &place.UpdatedAt, &place.ArchivedAt,
	); err != nil {
		return nil, err
	}
	return &place, nil
}

func scanTargetPlaceWithEffectivePermissions(row pgx.Row) (*model.TargetPlace, error) {
	var place model.TargetPlace
	if err := row.Scan(
		&place.ID, &place.OwnerUserID, &place.LifecycleOwnerKind, &place.LifecycleOwnerRef,
		&place.Name, &place.Description, &place.PlaceKind, &place.GeometryGeoJSON,
		&place.Visibility, &place.AccessScopeID, &place.OriginKey, &place.ExternalRefKind,
		&place.ExternalRefID, &place.ExternalRef, &place.ExternalRefVisibility, &place.Metadata,
		&place.CreatedBy, &place.CreatedAt, &place.UpdatedAt, &place.ArchivedAt,
		&place.EffectivePermissions,
	); err != nil {
		return nil, err
	}
	return &place, nil
}

func scanTargetRegion(row pgx.Row) (*model.TargetRegion, error) {
	var region model.TargetRegion
	if err := row.Scan(
		&region.ID, &region.OwnerUserID, &region.LifecycleOwnerKind, &region.LifecycleOwnerRef,
		&region.Name, &region.Description, &region.RegionKind, &region.GeometryGeoJSON,
		&region.Visibility, &region.AccessScopeID, &region.OriginKey, &region.ExternalRefKind,
		&region.ExternalRefID, &region.ExternalRef, &region.ExternalRefVisibility, &region.Metadata,
		&region.CreatedBy, &region.CreatedAt, &region.UpdatedAt, &region.ArchivedAt,
	); err != nil {
		return nil, err
	}
	return &region, nil
}

func scanTargetRegionWithEffectivePermissions(row pgx.Row) (*model.TargetRegion, error) {
	var region model.TargetRegion
	if err := row.Scan(
		&region.ID, &region.OwnerUserID, &region.LifecycleOwnerKind, &region.LifecycleOwnerRef,
		&region.Name, &region.Description, &region.RegionKind, &region.GeometryGeoJSON,
		&region.Visibility, &region.AccessScopeID, &region.OriginKey, &region.ExternalRefKind,
		&region.ExternalRefID, &region.ExternalRef, &region.ExternalRefVisibility, &region.Metadata,
		&region.CreatedBy, &region.CreatedAt, &region.UpdatedAt, &region.ArchivedAt,
		&region.EffectivePermissions,
	); err != nil {
		return nil, err
	}
	return &region, nil
}

func (db *DB) validateTargetAccessForExistingOwner(ctx context.Context, visibility string, accessScopeID *string, ownerUserID *string) error {
	if ownerUserID == nil || strings.TrimSpace(*ownerUserID) == "" {
		if err := validateTargetVisibilityAccessPair(visibility, accessScopeID); err != nil {
			return err
		}
		if strings.TrimSpace(visibility) == "shared" {
			return fmt.Errorf("%w: shared targets require a user owner", ErrTargetValidation)
		}
		return nil
	}
	return db.validateTargetAccessForUserOwner(ctx, visibility, accessScopeID, *ownerUserID)
}

func (db *DB) requireTargetObjectUpdatePermissions(ctx context.Context, userID, targetKind, targetID string, ownerUserID *string, permissions []string) error {
	if ownerUserID != nil && strings.TrimSpace(userID) != "" && *ownerUserID == strings.TrimSpace(userID) {
		return nil
	}
	if len(permissions) == 0 {
		permissions = []string{"edit_metadata"}
	}
	for _, permission := range permissions {
		ok, err := db.CanEditTarget(ctx, userID, targetKind, targetID, permission)
		if err != nil {
			return err
		}
		if !ok {
			return ErrTargetNotFound
		}
	}
	return nil
}

func targetPlaceUpdatePermissions(input model.TargetPlaceUpdate) []string {
	var permissions []string
	if input.Name != nil || input.Description != nil || input.PlaceKind != nil || input.Metadata != nil {
		permissions = append(permissions, "edit_metadata")
	}
	if input.GeometryGeoJSON != nil {
		permissions = append(permissions, "edit_geometry")
	}
	if input.ExternalRefKind != nil || input.ExternalRefID != nil || input.ExternalRef != nil || input.ExternalRefVisibility != nil {
		permissions = append(permissions, "link_external_refs")
	}
	if input.Visibility != nil || input.AccessScopeID != nil {
		permissions = append(permissions, "publish")
	}
	return uniqueTargetPermissions(permissions)
}

func targetRegionUpdatePermissions(input model.TargetRegionUpdate) []string {
	var permissions []string
	if input.Name != nil || input.Description != nil || input.RegionKind != nil || input.Metadata != nil {
		permissions = append(permissions, "edit_metadata")
	}
	if input.GeometryGeoJSON != nil {
		permissions = append(permissions, "edit_geometry")
	}
	if input.ExternalRefKind != nil || input.ExternalRefID != nil || input.ExternalRef != nil || input.ExternalRefVisibility != nil {
		permissions = append(permissions, "link_external_refs")
	}
	if input.Visibility != nil || input.AccessScopeID != nil {
		permissions = append(permissions, "publish")
	}
	return uniqueTargetPermissions(permissions)
}

func uniqueTargetPermissions(permissions []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		out = append(out, permission)
	}
	return out
}

func validateTargetPlaceInput(placeKind, geometryGeoJSON string) error {
	if strings.TrimSpace(placeKind) == "" {
		return fmt.Errorf("%w: place kind is required", ErrTargetValidation)
	}
	if strings.TrimSpace(placeKind) != "existing" {
		if err := validateTargetEnum(placeKind, "place_kind", allowedTargetPlaceKinds); err != nil {
			return err
		}
	}
	return validateTargetGeoJSONKind(geometryGeoJSON, map[string]struct{}{"Point": {}})
}

func validateTargetRegionInput(regionKind, geometryGeoJSON string) error {
	if strings.TrimSpace(regionKind) == "" {
		return fmt.Errorf("%w: region kind is required", ErrTargetValidation)
	}
	if strings.TrimSpace(regionKind) != "existing" {
		if err := validateTargetEnum(regionKind, "region_kind", allowedTargetRegionKinds); err != nil {
			return err
		}
	}
	return validateTargetGeoJSONKind(geometryGeoJSON, map[string]struct{}{"Polygon": {}, "MultiPolygon": {}})
}

func validateTargetGeoJSONKind(raw string, allowed map[string]struct{}) error {
	var body struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return fmt.Errorf("%w: invalid GeoJSON geometry", ErrTargetValidation)
	}
	if _, ok := allowed[body.Type]; !ok {
		return fmt.Errorf("%w: unsupported GeoJSON geometry type %q", ErrTargetValidation, body.Type)
	}
	if len(body.Coordinates) == 0 {
		return fmt.Errorf("%w: GeoJSON geometry requires coordinates", ErrTargetValidation)
	}
	switch body.Type {
	case "Point":
		var point []float64
		if err := json.Unmarshal(body.Coordinates, &point); err != nil || !validTargetPointCoordinates(point) {
			return fmt.Errorf("%w: invalid point coordinates", ErrTargetValidation)
		}
	case "Polygon":
		var polygon [][][]float64
		if err := json.Unmarshal(body.Coordinates, &polygon); err != nil || !validTargetPolygonCoordinates(polygon) {
			return fmt.Errorf("%w: invalid polygon coordinates", ErrTargetValidation)
		}
	case "MultiPolygon":
		var multipolygon [][][][]float64
		if err := json.Unmarshal(body.Coordinates, &multipolygon); err != nil || len(multipolygon) == 0 {
			return fmt.Errorf("%w: invalid multipolygon coordinates", ErrTargetValidation)
		}
		for _, polygon := range multipolygon {
			if !validTargetPolygonCoordinates(polygon) {
				return fmt.Errorf("%w: invalid multipolygon coordinates", ErrTargetValidation)
			}
		}
	}
	return nil
}

func validTargetPolygonCoordinates(polygon [][][]float64) bool {
	if len(polygon) == 0 {
		return false
	}
	for _, ring := range polygon {
		if len(ring) < 4 {
			return false
		}
		for _, point := range ring {
			if !validTargetPointCoordinates(point) {
				return false
			}
		}
	}
	return true
}

func validTargetPointCoordinates(point []float64) bool {
	if len(point) < 2 {
		return false
	}
	for i := range 2 {
		if math.IsNaN(point[i]) || math.IsInf(point[i], 0) {
			return false
		}
	}
	return true
}

func targetPlaceKindForValidation(kind *string) string {
	if kind == nil {
		return "existing"
	}
	return *kind
}

func targetRegionKindForValidation(kind *string) string {
	if kind == nil {
		return "existing"
	}
	return *kind
}

func targetLimit(limit int) int {
	if limit <= 0 || limit > 500 {
		return 100
	}
	return limit
}

func targetOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func targetVisibilityOrDefault(visibility string) string {
	if strings.TrimSpace(visibility) == "" {
		return "private"
	}
	return strings.TrimSpace(visibility)
}

func targetOriginKey(originKey string) string {
	if strings.TrimSpace(originKey) == "" {
		return "manual"
	}
	return strings.TrimSpace(originKey)
}

func targetExternalRefVisibility(value string) string {
	if strings.TrimSpace(value) == "" {
		return "owner_only"
	}
	return strings.TrimSpace(value)
}

func targetMetadata(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}
