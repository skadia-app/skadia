package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type AutoTitleActivityInput struct {
	ActivityID string
	SportType  string
	SubType    string
	StartCity  string
	EndCity    string

	StartLat, StartLng *float64
	EndLat, EndLng     *float64

	StartElevationM *float64
	EndElevationM   *float64
	MinElevationM   *float64
	MaxElevationM   *float64
}

func (db *DB) GetAutoTitleActivityInput(ctx context.Context, userID, activityID string) (*AutoTitleActivityInput, error) {
	var out AutoTitleActivityInput
	var startLat, startLng, endLat, endLng sql.NullFloat64
	var startElevationM, endElevationM, minElevationM, maxElevationM sql.NullFloat64

	out.ActivityID = activityID
	err := db.Pool.QueryRow(ctx, `
		SELECT
			a.sport_type,
			COALESCE(a.sub_type, ''),
			COALESCE(a.start_city, ''),
			COALESCE(a.end_city, ''),
			ST_Y(a.start_location::geometry),
			ST_X(a.start_location::geometry),
			ST_Y(a.end_location::geometry),
			ST_X(a.end_location::geometry),
			start_stream.altitude_m,
			end_stream.altitude_m,
			a.min_elevation_m,
			a.max_elevation_m
		FROM activities a
		LEFT JOIN LATERAL (
			SELECT s.altitude_m
			FROM activity_streams s
			WHERE s.activity_id = a.id AND s.altitude_m IS NOT NULL
			ORDER BY s.time ASC
			LIMIT 1
		) start_stream ON true
		LEFT JOIN LATERAL (
			SELECT s.altitude_m
			FROM activity_streams s
			WHERE s.activity_id = a.id AND s.altitude_m IS NOT NULL
			ORDER BY s.time DESC
			LIMIT 1
		) end_stream ON true
		WHERE a.id = $1 AND a.user_id = $2`,
		activityID, userID,
	).Scan(
		&out.SportType,
		&out.SubType,
		&out.StartCity,
		&out.EndCity,
		&startLat,
		&startLng,
		&endLat,
		&endLng,
		&startElevationM,
		&endElevationM,
		&minElevationM,
		&maxElevationM,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("auto title activity not found or not owned: %s", activityID)
	}
	if err != nil {
		return nil, fmt.Errorf("get auto title activity input: %w", err)
	}

	out.StartLat = nullableFloat64Ptr(startLat)
	out.StartLng = nullableFloat64Ptr(startLng)
	out.EndLat = nullableFloat64Ptr(endLat)
	out.EndLng = nullableFloat64Ptr(endLng)
	out.StartElevationM = nullableFloat64Ptr(startElevationM)
	out.EndElevationM = nullableFloat64Ptr(endElevationM)
	out.MinElevationM = nullableFloat64Ptr(minElevationM)
	out.MaxElevationM = nullableFloat64Ptr(maxElevationM)

	return &out, nil
}

func (db *DB) UpdateActivityAutoTitle(ctx context.Context, userID, activityID, title string) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE activities
		SET title_auto = NULLIF($3, '')
		WHERE id = $1 AND user_id = $2`,
		activityID, userID, title,
	)
	if err != nil {
		return fmt.Errorf("update activity auto title: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("auto title activity not found or not owned: %s", activityID)
	}
	return nil
}

func nullableFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}
