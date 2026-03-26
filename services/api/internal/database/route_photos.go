package database

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetRoutePhotos returns all photos for a route owned by the given user, ordered by sort_order.
func (db *DB) GetRoutePhotos(ctx context.Context, routeID, userID string) ([]model.RoutePhoto, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, route_id, user_id, file_hash,
			original_name, width, height, sort_order, created_at
		FROM route_photos
		WHERE route_id = $1 AND user_id = $2
		ORDER BY sort_order ASC, created_at ASC
	`, routeID, userID)
	if err != nil {
		return nil, fmt.Errorf("get route photos: %w", err)
	}
	defer rows.Close()

	var photos []model.RoutePhoto
	for rows.Next() {
		var p model.RoutePhoto
		if err := rows.Scan(
			&p.ID, &p.RouteID, &p.UserID, &p.FileHash,
			&p.OriginalName, &p.Width, &p.Height,
			&p.SortOrder, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan route photo: %w", err)
		}
		photos = append(photos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get route photos rows: %w", err)
	}
	if photos == nil {
		photos = []model.RoutePhoto{}
	}
	return photos, nil
}

// InsertRoutePhoto inserts a new route photo and returns the created record.
func (db *DB) InsertRoutePhoto(ctx context.Context, photo model.RoutePhoto) (model.RoutePhoto, error) {
	var saved model.RoutePhoto
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO route_photos (route_id, user_id, file_hash,
			original_name, width, height, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6,
			COALESCE((SELECT MAX(sort_order) + 1 FROM route_photos WHERE route_id = $1), 0))
		RETURNING id, route_id, user_id, file_hash,
			original_name, width, height, sort_order, created_at
	`,
		photo.RouteID, photo.UserID, photo.FileHash,
		photo.OriginalName, photo.Width, photo.Height,
	).Scan(
		&saved.ID, &saved.RouteID, &saved.UserID, &saved.FileHash,
		&saved.OriginalName, &saved.Width, &saved.Height,
		&saved.SortOrder, &saved.CreatedAt,
	)
	if err != nil {
		return model.RoutePhoto{}, fmt.Errorf("insert route photo: %w", err)
	}
	return saved, nil
}

// DeleteRoutePhoto deletes a route photo by ID. Only photos owned by userID and
// belonging to the specified routeID can be deleted.
// Returns the file_id so the caller can clean up file storage.
func (db *DB) DeleteRoutePhoto(ctx context.Context, photoID, userID, routeID string) (fileID string, err error) {
	err = db.Pool.QueryRow(ctx, `
		DELETE FROM route_photos
		WHERE id = $1 AND user_id = $2 AND route_id = $3
		RETURNING COALESCE(file_id::text, '')
	`, photoID, userID, routeID).Scan(&fileID)
	if err != nil {
		return "", fmt.Errorf("delete route photo: %w", err)
	}
	return fileID, nil
}

// ReorderRoutePhotos updates sort_order for the given photo IDs. The order of the
// slice determines the new sort_order (0-indexed). Only photos belonging to the
// given route are updated.
func (db *DB) ReorderRoutePhotos(ctx context.Context, routeID string, photoIDs []string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("reorder route photos begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for i, id := range photoIDs {
		tag, err := tx.Exec(ctx,
			`UPDATE route_photos SET sort_order = $1 WHERE id = $2 AND route_id = $3`,
			i, id, routeID,
		)
		if err != nil {
			return fmt.Errorf("reorder route photo %s: %w", id, err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("route photo %s not found in route %s", id, routeID)
		}
	}

	return tx.Commit(ctx)
}
