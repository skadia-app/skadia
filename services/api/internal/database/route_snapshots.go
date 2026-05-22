package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) CreatePlannedRouteSnapshot(ctx context.Context, input model.CreatePlannedRouteSnapshotInput) (*model.PlannedRouteSnapshot, error) {
	route, err := db.GetRoute(ctx, input.RouteID, input.UserID)
	if err != nil {
		return nil, err
	}
	if route.Version != input.SourceRouteVersion {
		return nil, ErrVersionMismatch
	}

	waypointsJSON, err := json.Marshal(route.Waypoints)
	if err != nil {
		return nil, fmt.Errorf("marshal route snapshot waypoints: %w", err)
	}
	segmentsJSON, err := json.Marshal(route.Segments)
	if err != nil {
		return nil, fmt.Errorf("marshal route snapshot segments: %w", err)
	}
	geometryHash := stableHashString(route.GeometryGeoJSON)
	snapshotHash, err := plannedRouteSnapshotHash(route, waypointsJSON, segmentsJSON, geometryHash)
	if err != nil {
		return nil, err
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin planned route snapshot: %w", err)
	}
	defer tx.Rollback(ctx)

	var snapshotID string
	if err := tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO planned_route_snapshots (
				route_id, user_id, source_route_version, route_name, route_description,
				sport_type, sub_type, is_electric, is_indoor, is_virtual, is_commute,
				profile, routing_engine, route_schema_version, geometry,
				waypoints, segments, elevation_profile, enrichment,
				total_distance_m, total_elevation_gain, total_elevation_loss,
				estimated_time_min, geometry_hash, snapshot_hash
			)
			VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10, $11,
				$12, $13, $14, ST_Force2D(ST_GeomFromGeoJSON($15)),
				$16, $17, $18, $19,
				$20, $21, $22,
				$23, $24, $25
			)
			ON CONFLICT (route_id, source_route_version, snapshot_hash) DO NOTHING
			RETURNING id
		)
		SELECT id FROM inserted
		UNION ALL
		SELECT id
		FROM planned_route_snapshots
		WHERE route_id = $1
		  AND source_route_version = $3
		  AND snapshot_hash = $25
		LIMIT 1
	`, route.ID, route.UserID, route.Version, route.Name, route.Description,
		route.SportType, route.SubType, route.IsElectric, route.IsIndoor, route.IsVirtual, route.IsCommute,
		route.Profile, route.RoutingEngine, route.RouteSchemaVersion, route.GeometryGeoJSON,
		waypointsJSON, segmentsJSON, nullableRawSQL(route.ElevationProfile), nullableRawSQL(route.Enrichment),
		route.TotalDistanceM, route.TotalElevationGain, route.TotalElevationLoss,
		route.EstimatedTimeMin, geometryHash, snapshotHash,
	).Scan(&snapshotID); err != nil {
		return nil, fmt.Errorf("insert planned route snapshot: %w", err)
	}

	operationID := input.OperationID
	if operationID == "" {
		operationID = "snapshot:" + snapshotID
	}
	operationInputHash := stableHashString(fmt.Sprintf("%s:%d:%s", route.ID, route.Version, snapshotHash))
	var existingInputHash string
	err = tx.QueryRow(ctx, `
		SELECT operation_input_hash
		FROM route_publication_events
		WHERE actor_user_id = $1
		  AND operation = 'snapshot'
		  AND operation_id = $2
	`, route.UserID, operationID).Scan(&existingInputHash)
	if err == nil {
		if existingInputHash != operationInputHash {
			return nil, ErrRoutePublicationConflict
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("load planned route snapshot event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO route_publication_events (
			actor_user_id, operation, operation_id, operation_input_hash,
			subject_kind, subject_id, after_snapshot_hash, metadata
		)
		VALUES ($1, 'snapshot', $2, $3, 'planned_route_snapshot', $4, $5, jsonb_build_object('reason', NULLIF($6::text, '')))
		ON CONFLICT (actor_user_id, operation_id, operation) DO NOTHING
	`, route.UserID, operationID, operationInputHash, snapshotID, snapshotHash, input.Reason); err != nil {
		return nil, fmt.Errorf("insert planned route snapshot event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit planned route snapshot: %w", err)
	}
	return db.GetPlannedRouteSnapshot(ctx, snapshotID, input.UserID)
}

func (db *DB) GetPlannedRouteSnapshot(ctx context.Context, snapshotID, userID string) (*model.PlannedRouteSnapshot, error) {
	var snapshot model.PlannedRouteSnapshot
	if err := db.Pool.QueryRow(ctx, `
		SELECT id, route_id, user_id, source_route_version,
		       snapshot_schema_key, snapshot_schema_version,
		       route_name, route_description, sport_type, sub_type,
		       is_electric, is_indoor, is_virtual, is_commute,
		       profile, routing_engine, route_schema_version,
		       ST_AsGeoJSON(geometry), waypoints, segments,
		       elevation_profile, enrichment,
		       total_distance_m, total_elevation_gain, total_elevation_loss,
		       estimated_time_min, geometry_hash, snapshot_hash, created_at
		FROM planned_route_snapshots
		WHERE id = $1
		  AND user_id = $2
	`, snapshotID, userID).Scan(
		&snapshot.ID, &snapshot.RouteID, &snapshot.UserID, &snapshot.SourceRouteVersion,
		&snapshot.SnapshotSchemaKey, &snapshot.SnapshotSchemaVersion,
		&snapshot.RouteName, &snapshot.RouteDescription, &snapshot.SportType, &snapshot.SubType,
		&snapshot.IsElectric, &snapshot.IsIndoor, &snapshot.IsVirtual, &snapshot.IsCommute,
		&snapshot.Profile, &snapshot.RoutingEngine, &snapshot.RouteSchemaVersion,
		&snapshot.GeometryGeoJSON, &snapshot.Waypoints, &snapshot.Segments,
		&snapshot.ElevationProfile, &snapshot.Enrichment,
		&snapshot.TotalDistanceM, &snapshot.TotalElevationGain, &snapshot.TotalElevationLoss,
		&snapshot.EstimatedTimeMin, &snapshot.GeometryHash, &snapshot.SnapshotHash, &snapshot.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func plannedRouteSnapshotHash(route *model.PlannedRoute, waypointsJSON, segmentsJSON []byte, geometryHash string) (string, error) {
	payload := struct {
		Name               string          `json:"name"`
		Description        *string         `json:"description,omitempty"`
		SportType          string          `json:"sport_type"`
		SubType            *string         `json:"sub_type,omitempty"`
		IsElectric         bool            `json:"is_electric"`
		IsIndoor           bool            `json:"is_indoor"`
		IsVirtual          bool            `json:"is_virtual"`
		IsCommute          bool            `json:"is_commute"`
		Profile            string          `json:"profile"`
		RoutingEngine      *string         `json:"routing_engine,omitempty"`
		RouteSchemaVersion int16           `json:"route_schema_version"`
		GeometryHash       string          `json:"geometry_hash"`
		Waypoints          json.RawMessage `json:"waypoints"`
		Segments           json.RawMessage `json:"segments"`
		ElevationProfile   json.RawMessage `json:"elevation_profile,omitempty"`
		Enrichment         json.RawMessage `json:"enrichment,omitempty"`
		TotalDistanceM     *float64        `json:"total_distance_m,omitempty"`
		TotalElevationGain *float64        `json:"total_elevation_gain,omitempty"`
		TotalElevationLoss *float64        `json:"total_elevation_loss,omitempty"`
		EstimatedTimeMin   *int            `json:"estimated_time_min,omitempty"`
	}{
		Name:               route.Name,
		Description:        route.Description,
		SportType:          route.SportType,
		SubType:            route.SubType,
		IsElectric:         route.IsElectric,
		IsIndoor:           route.IsIndoor,
		IsVirtual:          route.IsVirtual,
		IsCommute:          route.IsCommute,
		Profile:            route.Profile,
		RoutingEngine:      route.RoutingEngine,
		RouteSchemaVersion: route.RouteSchemaVersion,
		GeometryHash:       geometryHash,
		Waypoints:          waypointsJSON,
		Segments:           segmentsJSON,
		ElevationProfile:   route.ElevationProfile,
		Enrichment:         route.Enrichment,
		TotalDistanceM:     route.TotalDistanceM,
		TotalElevationGain: route.TotalElevationGain,
		TotalElevationLoss: route.TotalElevationLoss,
		EstimatedTimeMin:   route.EstimatedTimeMin,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal planned route snapshot hash: %w", err)
	}
	return stableHashBytes(raw), nil
}

func nullableRawSQL(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func stableHashString(value string) string {
	return stableHashBytes([]byte(value))
}

func stableHashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
