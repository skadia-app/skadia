package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) ForkRoute(ctx context.Context, sourceRouteID, userID, name, operationID, operationInputHash string, sourceRouteVersion int64) (*model.PlannedRoute, string, error) {
	if sourceRouteID == "" || userID == "" || operationID == "" || operationInputHash == "" {
		return nil, "", fmt.Errorf("%w: source route, user, operation id, and input hash are required", ErrRoutePublicationValidation)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("begin fork route: %w", err)
	}
	defer tx.Rollback(ctx)

	existing, err := loadRouteRelationshipOperationTx(ctx, tx, userID, operationID)
	if err != nil {
		return nil, "", err
	}
	if existing != nil {
		if existing.OperationInputHash != operationInputHash {
			return nil, "", ErrRoutePublicationConflict
		}
		if existing.RelationType != "fork_of" || existing.TargetRouteID != sourceRouteID {
			return nil, "", ErrRoutePublicationConflict
		}
		route, err := db.GetRoute(ctx, existing.SourceRouteID, userID)
		if err != nil {
			return nil, "", err
		}
		return route, existing.ID, nil
	}

	source, err := db.GetRoute(ctx, sourceRouteID, userID)
	if err != nil {
		return nil, "", err
	}
	if source.Version != sourceRouteVersion {
		return nil, "", ErrVersionMismatch
	}
	fork := *source
	fork.ID = ""
	fork.Name = name
	fork.Photos = nil
	fork.Tags = nil
	newID, err := db.CreateRouteTx(ctx, tx, &fork)
	if err != nil {
		return nil, "", err
	}
	relationshipID, err := insertRouteRelationshipTx(ctx, tx, newID, sourceRouteID, userID, "fork_of", operationID, operationInputHash)
	if err != nil {
		return nil, "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO route_publication_events (
			actor_user_id, operation, operation_id, operation_input_hash,
			subject_kind, subject_id, after_snapshot_hash, metadata
		)
		VALUES ($1, 'fork_route', $2, $3, 'route_relationship', $4, $5, jsonb_build_object('source_route_id', $6::text, 'target_route_id', $7::text))
		ON CONFLICT (actor_user_id, operation_id, operation) DO NOTHING
	`, userID, operationID, operationInputHash, relationshipID, routeRelationshipEventHash("fork_of", newID, sourceRouteID), newID, sourceRouteID); err != nil {
		return nil, "", fmt.Errorf("insert fork route event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, "", fmt.Errorf("commit fork route: %w", err)
	}
	created, err := db.GetRoute(ctx, newID, userID)
	if err != nil {
		return nil, "", err
	}
	return created, relationshipID, nil
}

func (db *DB) CreateRouteRelationship(ctx context.Context, sourceRouteID, targetRouteID, userID, relationType, operationID, operationInputHash string) (string, error) {
	if sourceRouteID == targetRouteID {
		return "", fmt.Errorf("%w: route relationship source and target must differ", ErrRoutePublicationValidation)
	}
	if relationType != "variant_of" && relationType != "derived_from" {
		return "", fmt.Errorf("%w: unsupported route relationship %q", ErrRoutePublicationValidation, relationType)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin route relationship: %w", err)
	}
	defer tx.Rollback(ctx)

	existing, err := loadRouteRelationshipOperationTx(ctx, tx, userID, operationID)
	if err != nil {
		return "", err
	}
	if existing != nil {
		if existing.OperationInputHash != operationInputHash {
			return "", ErrRoutePublicationConflict
		}
		if existing.SourceRouteID != sourceRouteID || existing.TargetRouteID != targetRouteID || existing.RelationType != relationType {
			return "", ErrRoutePublicationConflict
		}
		return existing.ID, nil
	}
	if err := requireOwnedRoutesTx(ctx, tx, userID, sourceRouteID, targetRouteID); err != nil {
		return "", err
	}
	relationshipID, err := insertRouteRelationshipTx(ctx, tx, sourceRouteID, targetRouteID, userID, relationType, operationID, operationInputHash)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO route_publication_events (
			actor_user_id, operation, operation_id, operation_input_hash,
			subject_kind, subject_id, after_snapshot_hash, metadata
		)
		VALUES ($1, 'relate_route', $2, $3, 'route_relationship', $4, $5, jsonb_build_object('relation_type', $6::text))
		ON CONFLICT (actor_user_id, operation_id, operation) DO NOTHING
	`, userID, operationID, operationInputHash, relationshipID, routeRelationshipEventHash(relationType, sourceRouteID, targetRouteID), relationType); err != nil {
		return "", fmt.Errorf("insert route relationship event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit route relationship: %w", err)
	}
	return relationshipID, nil
}

func (db *DB) CreateRouteCourseRelationship(ctx context.Context, sourcePublicationID, targetPublicationID, userID, relationType, operationID, operationInputHash string) (string, error) {
	if sourcePublicationID == targetPublicationID {
		return "", fmt.Errorf("%w: course relationship source and target must differ", ErrRoutePublicationValidation)
	}
	if relationType != "variant_of" && relationType != "derived_from" {
		return "", fmt.Errorf("%w: unsupported course relationship %q", ErrRoutePublicationValidation, relationType)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin route course relationship: %w", err)
	}
	defer tx.Rollback(ctx)

	existing, err := loadRouteCourseRelationshipOperationTx(ctx, tx, userID, operationID)
	if err != nil {
		return "", err
	}
	if existing != nil {
		if existing.OperationInputHash != operationInputHash {
			return "", ErrRoutePublicationConflict
		}
		if existing.SourcePublicationID != sourcePublicationID || existing.TargetPublicationID != targetPublicationID || existing.RelationType != relationType {
			return "", ErrRoutePublicationConflict
		}
		return existing.ID, nil
	}
	source, err := loadOwnedPublicationForRelationshipTx(ctx, tx, sourcePublicationID, userID)
	if err != nil {
		return "", err
	}
	target, err := loadOwnedPublicationForRelationshipTx(ctx, tx, targetPublicationID, userID)
	if err != nil {
		return "", err
	}
	var relationshipID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO route_course_relationships (
			source_publication_id, target_publication_id,
			source_course_segment_id, source_course_segment_version_id,
			target_course_segment_id, target_course_segment_version_id,
			relation_type, actor_user_id, operation_id, operation_input_hash
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (
			source_course_segment_id,
			COALESCE(source_course_segment_version_id, '00000000-0000-0000-0000-000000000000'::uuid),
			target_course_segment_id,
			COALESCE(target_course_segment_version_id, '00000000-0000-0000-0000-000000000000'::uuid),
			relation_type
		) DO UPDATE SET operation_input_hash = route_course_relationships.operation_input_hash
		RETURNING id
	`, sourcePublicationID, targetPublicationID,
		source.CourseSegmentID, source.CourseSegmentVersionID,
		target.CourseSegmentID, target.CourseSegmentVersionID,
		relationType, userID, operationID, operationInputHash).Scan(&relationshipID); err != nil {
		return "", fmt.Errorf("insert route course relationship: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO route_publication_events (
			actor_user_id, operation, operation_id, operation_input_hash,
			subject_kind, subject_id, after_snapshot_hash, metadata
		)
		VALUES ($1, 'relate_course', $2, $3, 'route_course_relationship', $4, $5, jsonb_build_object('relation_type', $6::text))
		ON CONFLICT (actor_user_id, operation_id, operation) DO NOTHING
	`, userID, operationID, operationInputHash, relationshipID, routeRelationshipEventHash(relationType, source.CourseSegmentID, target.CourseSegmentID), relationType); err != nil {
		return "", fmt.Errorf("insert route course relationship event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit route course relationship: %w", err)
	}
	return relationshipID, nil
}

func routeRelationshipEventHash(relationType, sourceID, targetID string) string {
	return stableHashBytes([]byte(relationType + "\x00" + sourceID + "\x00" + targetID))
}

func insertRouteRelationshipTx(ctx context.Context, tx pgx.Tx, sourceRouteID, targetRouteID, userID, relationType, operationID, operationInputHash string) (string, error) {
	var relationshipID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO route_relationships (
			source_route_id, target_route_id, relation_type,
			actor_user_id, operation_id, operation_input_hash
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (source_route_id, target_route_id, relation_type) DO UPDATE
			SET operation_input_hash = route_relationships.operation_input_hash
		RETURNING id
	`, sourceRouteID, targetRouteID, relationType, userID, operationID, operationInputHash).Scan(&relationshipID); err != nil {
		return "", fmt.Errorf("insert route relationship: %w", err)
	}
	return relationshipID, nil
}

type routeRelationshipOperationRow struct {
	ID                 string
	OperationInputHash string
	SourceRouteID      string
	TargetRouteID      string
	RelationType       string
}

func loadRouteRelationshipOperationTx(ctx context.Context, tx pgx.Tx, userID, operationID string) (*routeRelationshipOperationRow, error) {
	if operationID == "" {
		return nil, fmt.Errorf("%w: operation_id is required", ErrRoutePublicationValidation)
	}
	var row routeRelationshipOperationRow
	err := tx.QueryRow(ctx, `
		SELECT id, operation_input_hash, source_route_id, target_route_id, relation_type
		FROM route_relationships
		WHERE actor_user_id = $1
		  AND operation_id = $2
	`, userID, operationID).Scan(&row.ID, &row.OperationInputHash, &row.SourceRouteID, &row.TargetRouteID, &row.RelationType)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load route relationship operation: %w", err)
	}
	return &row, nil
}

type routeCourseRelationshipOperationRow struct {
	ID                  string
	OperationInputHash  string
	SourcePublicationID string
	TargetPublicationID string
	RelationType        string
}

func loadRouteCourseRelationshipOperationTx(ctx context.Context, tx pgx.Tx, userID, operationID string) (*routeCourseRelationshipOperationRow, error) {
	if operationID == "" {
		return nil, fmt.Errorf("%w: operation_id is required", ErrRoutePublicationValidation)
	}
	var row routeCourseRelationshipOperationRow
	err := tx.QueryRow(ctx, `
		SELECT id, operation_input_hash,
		       COALESCE(source_publication_id::text, ''),
		       COALESCE(target_publication_id::text, ''),
		       relation_type
		FROM route_course_relationships
		WHERE actor_user_id = $1
		  AND operation_id = $2
	`, userID, operationID).Scan(&row.ID, &row.OperationInputHash, &row.SourcePublicationID, &row.TargetPublicationID, &row.RelationType)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load route course relationship operation: %w", err)
	}
	return &row, nil
}

func requireOwnedRoutesTx(ctx context.Context, tx pgx.Tx, userID string, routeIDs ...string) error {
	var count int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM planned_routes
		WHERE user_id = $1
		  AND id = ANY($2::uuid[])
	`, userID, routeIDs).Scan(&count); err != nil {
		return fmt.Errorf("check route ownership: %w", err)
	}
	if count != len(routeIDs) {
		return fmt.Errorf("%w: route relationship requires ownership of all routes", ErrRoutePublicationValidation)
	}
	return nil
}

func loadOwnedPublicationForRelationshipTx(ctx context.Context, tx pgx.Tx, publicationID, userID string) (*model.RouteCoursePublication, error) {
	publication, err := loadRoutePublicationByIDTx(ctx, tx, publicationID)
	if err != nil {
		return nil, err
	}
	if publication.ActorUserID == nil || *publication.ActorUserID != userID {
		return nil, fmt.Errorf("%w: route course relationship requires ownership of both publications", ErrRoutePublicationValidation)
	}
	return publication, nil
}
