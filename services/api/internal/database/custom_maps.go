package database

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) ListCustomMaps(ctx context.Context, userID string) ([]model.CustomMap, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT cm.id, cm.user_id, cm.name, cm.description, cm.color, cm.is_visible, cm.sort_order,
		       cm.created_at, cm.updated_at,
		       COUNT(cmm.id) AS marker_count
		FROM custom_maps cm
		LEFT JOIN custom_map_markers cmm ON cmm.map_id = cm.id
		WHERE cm.user_id = $1
		GROUP BY cm.id
		ORDER BY cm.sort_order, cm.created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	maps := make([]model.CustomMap, 0)
	for rows.Next() {
		var m model.CustomMap
		if err := rows.Scan(&m.ID, &m.UserID, &m.Name, &m.Description, &m.Color, &m.IsVisible,
			&m.SortOrder, &m.CreatedAt, &m.UpdatedAt, &m.MarkerCount); err != nil {
			return nil, err
		}
		maps = append(maps, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return maps, nil
}

func (db *DB) CreateCustomMap(ctx context.Context, userID, name, color string, description *string) (model.CustomMap, error) {
	var m model.CustomMap
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO custom_maps (user_id, name, description, color)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, description, color, is_visible, sort_order, created_at, updated_at
	`, userID, name, description, color).Scan(
		&m.ID, &m.UserID, &m.Name, &m.Description, &m.Color, &m.IsVisible,
		&m.SortOrder, &m.CreatedAt, &m.UpdatedAt,
	)
	return m, err
}

func (db *DB) UpdateCustomMap(ctx context.Context, id, userID string, req model.UpdateCustomMapRequest) (model.CustomMap, error) {
	var m model.CustomMap
	err := db.Pool.QueryRow(ctx, `
		UPDATE custom_maps
		SET name        = COALESCE($3, name),
		    description = COALESCE($4, description),
		    color       = COALESCE($5, color),
		    is_visible  = COALESCE($6, is_visible),
		    updated_at  = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, name, description, color, is_visible, sort_order, created_at, updated_at
	`, id, userID, req.Name, req.Description, req.Color, req.IsVisible).Scan(
		&m.ID, &m.UserID, &m.Name, &m.Description, &m.Color, &m.IsVisible,
		&m.SortOrder, &m.CreatedAt, &m.UpdatedAt,
	)
	return m, err
}

func (db *DB) DeleteCustomMap(ctx context.Context, id, userID string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM custom_maps WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (db *DB) ListCustomMapMarkers(ctx context.Context, mapID, userID string) ([]model.CustomMapMarker, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, map_id, user_id, name, description, lat, lng, color, image_url, created_at, updated_at
		FROM custom_map_markers
		WHERE map_id = $1 AND user_id = $2
		ORDER BY created_at
	`, mapID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	markers := make([]model.CustomMapMarker, 0)
	for rows.Next() {
		var mk model.CustomMapMarker
		if err := rows.Scan(&mk.ID, &mk.MapID, &mk.UserID, &mk.Name, &mk.Description,
			&mk.Lat, &mk.Lng, &mk.Color, &mk.ImageURL, &mk.CreatedAt, &mk.UpdatedAt); err != nil {
			return nil, err
		}
		markers = append(markers, mk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return markers, nil
}

func (db *DB) CreateCustomMapMarker(ctx context.Context, mapID, userID string, req model.CreateMarkerRequest) (model.CustomMapMarker, error) {
	var mk model.CustomMapMarker
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO custom_map_markers (map_id, user_id, name, description, lat, lng, color)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, map_id, user_id, name, description, lat, lng, color, image_url, created_at, updated_at
	`, mapID, userID, req.Name, req.Description, req.Lat, req.Lng, req.Color).Scan(
		&mk.ID, &mk.MapID, &mk.UserID, &mk.Name, &mk.Description,
		&mk.Lat, &mk.Lng, &mk.Color, &mk.ImageURL, &mk.CreatedAt, &mk.UpdatedAt,
	)
	return mk, err
}

func (db *DB) UpdateCustomMapMarker(ctx context.Context, id, mapID, userID string, req model.UpdateMarkerRequest) (model.CustomMapMarker, error) {
	var mk model.CustomMapMarker
	err := db.Pool.QueryRow(ctx, `
		UPDATE custom_map_markers
		SET name        = COALESCE($4, name),
		    description = COALESCE($5, description),
		    lat         = COALESCE($6, lat),
		    lng         = COALESCE($7, lng),
		    color       = COALESCE($8, color),
		    image_url   = COALESCE($9, image_url),
		    updated_at  = NOW()
		WHERE id = $1 AND map_id = $2 AND user_id = $3
		RETURNING id, map_id, user_id, name, description, lat, lng, color, image_url, created_at, updated_at
	`, id, mapID, userID, req.Name, req.Description, req.Lat, req.Lng, req.Color, req.ImageURL).Scan(
		&mk.ID, &mk.MapID, &mk.UserID, &mk.Name, &mk.Description,
		&mk.Lat, &mk.Lng, &mk.Color, &mk.ImageURL, &mk.CreatedAt, &mk.UpdatedAt,
	)
	return mk, err
}

func (db *DB) DeleteCustomMapMarker(ctx context.Context, id, mapID, userID string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM custom_map_markers WHERE id = $1 AND map_id = $2 AND user_id = $3`, id, mapID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
