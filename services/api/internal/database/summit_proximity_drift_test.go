package database

import (
	"context"
	"errors"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type summitDriftFixture struct {
	activityID string
	geomWKB    []byte
	radiusM    float64
}

type summitStableKey struct {
	osmID int64
	name  string
	eleM  string
}

func TestFindSummitsAlongGeom_vs_ActivityTrack_DriftGuard(t *testing.T) {
	pool := summitDriftTestPool(t)
	ctx := context.Background()

	fixture := summitDriftFixtureForExistingActivity(t, ctx, pool)
	t.Logf("comparing summit wrappers for activity_id=%s radius_m=%.0f", fixture.activityID, fixture.radiusM)

	routeSummits, err := FindSummitsAlongGeom(ctx, pool, fixture.geomWKB, fixture.radiusM)
	if err != nil {
		t.Fatalf("FindSummitsAlongGeom: %v", err)
	}

	activitySummits, err := FindSummitsAlongActivityTrack(ctx, pool, fixture.activityID, fixture.radiusM)
	if err != nil {
		t.Fatalf("FindSummitsAlongActivityTrack: %v", err)
	}

	routeKeys := sortedSummitStableKeys(routeSummits)
	activityKeys := sortedSummitStableKeys(activitySummits)
	if !reflect.DeepEqual(routeKeys, activityKeys) {
		t.Fatalf("route/activity summit drift for activity_id=%s radius_m=%.0f\nroute=%v\nactivity=%v",
			fixture.activityID, fixture.radiusM, routeKeys, activityKeys)
	}
}

func summitDriftFixtureForExistingActivity(t *testing.T, ctx context.Context, pool *pgxpool.Pool) summitDriftFixture {
	t.Helper()

	const q = `
		SELECT candidate.activity_id,
			       ST_AsBinary(ST_Simplify(track.track_full, 0.00005)) AS geom_wkb,
		       GREATEST(10::double precision, candidate.max_dist_m::double precision + 1) AS radius_m
		FROM (
			SELECT g.activity_id, COALESCE(MAX(g.dist_m), 79) AS max_dist_m
			FROM activity_geo_tags g
			JOIN activity_tracks t ON t.activity_id = g.activity_id
			WHERE g.category IN ('summit', 'pass')
			  AND t.track_full IS NOT NULL
			GROUP BY g.activity_id
			ORDER BY g.activity_id
			LIMIT 1
		) AS candidate
		JOIN activity_tracks track ON track.activity_id = candidate.activity_id
	`

	var fixture summitDriftFixture
	err := pool.QueryRow(ctx, q).Scan(&fixture.activityID, &fixture.geomWKB, &fixture.radiusM)
	if errors.Is(err, pgx.ErrNoRows) {
		t.Skip("no activity with summit/pass geo tags and track_full found")
	}
	if err != nil {
		t.Fatalf("summit drift fixture query: %v", err)
	}
	if len(fixture.geomWKB) == 0 {
		t.Skipf("activity %s has empty simplified track geometry", fixture.activityID)
	}
	return fixture
}

func summitDriftTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := testdb.RequireSafeDSN(t)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("summitDriftTestPool: connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func sortedSummitStableKeys(summits []PassedSummit) []summitStableKey {
	keys := make([]summitStableKey, 0, len(summits))
	for _, summit := range summits {
		key := summitStableKey{
			osmID: summit.OsmID,
			name:  summit.Name,
			eleM:  "<nil>",
		}
		if summit.EleM != nil {
			key.eleM = strconv.FormatFloat(*summit.EleM, 'f', -1, 64)
		}
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].osmID != keys[j].osmID {
			return keys[i].osmID < keys[j].osmID
		}
		if keys[i].name != keys[j].name {
			return keys[i].name < keys[j].name
		}
		return keys[i].eleM < keys[j].eleM
	})
	return keys
}
