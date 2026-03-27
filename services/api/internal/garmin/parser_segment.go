package garmin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

type segmentPointData struct {
	Index typedef.MessageIndex
	Lat   float64
	Lng   float64
	Ele   float64
}

// processSegmentFIT parses a Garmin segment FIT file (FileType 34) and inserts
// a new segment into the database if it is not already present.
// If the SegmentId message has Enabled=false the file is silently skipped.
func processSegmentFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	if _, err := storeHealthFIT(ctx, db, fs, data, userID, "segment", "segment.fit", time.Now().UTC()); err != nil {
		return err
	}

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return err
	}

	var (
		segName  string
		segUUID  string
		segSport typedef.Sport
		points   []segmentPointData
	)

	for i := range fit.Messages {
		msg := &fit.Messages[i]
		switch msg.Num {
		case typedef.MesgNumSegmentId:
			seg := mesgdef.NewSegmentId(msg)
			// CRITICAL: segment disabled on device — skip entire file
			if seg.Enabled == typedef.BoolFalse {
				return nil
			}
			segName = seg.Name
			segUUID = seg.Uuid
			segSport = seg.Sport

		case typedef.MesgNumSegmentPoint:
			sp := mesgdef.NewSegmentPoint(msg)
			if sp.PositionLat == basetype.Sint32Invalid || sp.PositionLong == basetype.Sint32Invalid {
				continue
			}
			lat := float64(sp.PositionLat) * (180.0 / math.Pow(2, 31))
			lng := float64(sp.PositionLong) * (180.0 / math.Pow(2, 31))
			var ele float64
			if sp.EnhancedAltitude != basetype.Uint32Invalid {
				ele = float64(sp.EnhancedAltitude)/5.0 - 500.0
			}
			points = append(points, segmentPointData{
				Index: sp.MessageIndex,
				Lat:   lat,
				Lng:   lng,
				Ele:   ele,
			})
		}
	}

	if len(points) < 2 {
		slog.Debug("processSegmentFIT: not enough points, skipping", "uuid", segUUID)
		return nil
	}

	// Sort by MessageIndex
	sort.Slice(points, func(i, j int) bool {
		return points[i].Index < points[j].Index
	})

	// Build GeoJSON LineString
	coords := make([]string, len(points))
	hashParts := make([]string, len(points))
	for i, p := range points {
		coords[i] = fmt.Sprintf("[%.7f,%.7f,%.2f]", p.Lng, p.Lat, p.Ele)
		hashParts[i] = fmt.Sprintf("%.5f,%.5f", p.Lat, p.Lng)
	}
	trackGeoJSON := fmt.Sprintf(`{"type":"LineString","coordinates":[%s]}`, strings.Join(coords, ","))

	// Compute route_hash: SHA256 of coordinate string
	h := sha256.Sum256([]byte(strings.Join(hashParts, "|")))
	routeHash := hex.EncodeToString(h[:])

	// Exact duplicate check
	var existingID string
	err = db.Pool.QueryRow(ctx,
		`SELECT id FROM segments WHERE route_hash = $1 LIMIT 1`,
		routeHash,
	).Scan(&existingID)
	if err == nil {
		slog.Debug("processSegmentFIT: exact route_hash duplicate, skipping", "segment_id", existingID)
		return nil
	}

	// Spatial duplicate check: ST_DWithin + Hausdorff distance < 30m
	var spatialID string
	err = db.Pool.QueryRow(ctx, `
		SELECT s.id FROM segments s
		JOIN LATERAL (
			SELECT track FROM segment_versions
			WHERE segment_id = s.id
			ORDER BY version_number DESC
			LIMIT 1
		) sv ON true
		WHERE ST_DWithin(
			sv.track::geography,
			ST_SetSRID(ST_GeomFromGeoJSON($1), 4326)::geography,
			100
		)
		AND ST_HausdorffDistance(
			ST_Transform(sv.track, 3857),
			ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($1), 4326), 3857)
		) < 30
		LIMIT 1`,
		trackGeoJSON,
	).Scan(&spatialID)
	if err == nil {
		slog.Debug("processSegmentFIT: spatial duplicate found, skipping", "segment_id", spatialID)
		return nil
	}

	// Map FIT sport to sport_group
	sportGroup := fitSportToSegmentGroup(segSport)

	// Use UUID as name fallback
	name := segName
	if name == "" {
		if segUUID != "" {
			name = segUUID
		} else {
			name = "Imported Segment"
		}
	}

	// Insert segment
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("processSegmentFIT: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var segmentID string
	err = tx.QueryRow(ctx, `
		INSERT INTO segments (created_by, name, sport_group, segment_type, segment_type_meta, route_hash, visibility)
		VALUES ($1, $2, $3, 'route', '{}', $4, 'private')
		RETURNING id`,
		userID, name, sportGroup, routeHash,
	).Scan(&segmentID)
	if err != nil {
		return fmt.Errorf("processSegmentFIT: insert segment: %w", err)
	}

	// Insert first segment_version with track geometry
	_, err = tx.Exec(ctx, `
		WITH g AS (SELECT ST_SetSRID(ST_GeomFromGeoJSON($2), 4326) AS geom)
		INSERT INTO segment_versions (segment_id, version_number, track, start_point, end_point, valid_from, created_by)
		SELECT $1, 1, g.geom, ST_StartPoint(g.geom), ST_EndPoint(g.geom), CURRENT_DATE, $3
		FROM g`,
		segmentID, trackGeoJSON, userID,
	)
	if err != nil {
		return fmt.Errorf("processSegmentFIT: insert segment_version: %w", err)
	}

	// Update distance_m via PostGIS
	_, err = tx.Exec(ctx, `
		UPDATE segments SET distance_m = (
			SELECT ST_Length(track::geography)
			FROM segment_versions WHERE segment_id = $1 AND version_number = 1
		) WHERE id = $1`, segmentID)
	if err != nil {
		return fmt.Errorf("processSegmentFIT: update distance: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("processSegmentFIT: commit: %w", err)
	}

	slog.Info("processSegmentFIT: segment imported", "id", segmentID, "name", name, "sport_group", sportGroup)
	return nil
}

// fitSportToSegmentGroup maps a FIT typedef.Sport to the Skadia segment sport_group.
func fitSportToSegmentGroup(sport typedef.Sport) string {
	switch sport {
	case typedef.SportCycling:
		return "bike"
	case typedef.SportRunning, typedef.SportHiking, typedef.SportWalking:
		return "foot"
	case typedef.SportSwimming:
		return "swim"
	case typedef.SportAlpineSkiing, typedef.SportCrossCountrySkiing, typedef.SportSnowboarding,
		typedef.SportSnowshoeing:
		return "ski"
	case typedef.SportPaddling, typedef.SportRowing, typedef.SportKayaking, typedef.SportSailing:
		return "paddle"
	default:
		return "generic"
	}
}
