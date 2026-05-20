package database

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

var (
	routeProfileSACRanks        = map[string]int{"T1": 1, "T2": 2, "T3": 3, "T4": 4, "T5": 5, "T6": 6}
	routeProfileMTBRanks        = map[string]int{"S0": 0, "S1": 1, "S2": 2, "S3": 3, "S4": 4, "S5": 5}
	routeProfileViaFerrataRanks = map[string]int{"K1": 1, "K2": 2, "K3": 3, "K4": 4, "K5": 5, "K6": 6}
)

const routeProfileNoVersionUUID = "00000000-0000-0000-0000-000000000000"

func (db *DB) CreateRouteProfileSnapshot(ctx context.Context, snapshotID string) (*model.RouteProfileSnapshot, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin route profile snapshot: %w", err)
	}
	defer tx.Rollback(ctx)

	profile, err := db.CreateRouteProfileSnapshotTx(ctx, tx, snapshotID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit route profile snapshot: %w", err)
	}
	return profile, nil
}

func (db *DB) CreateRouteProfileSnapshotTx(ctx context.Context, tx pgx.Tx, snapshotID string) (*model.RouteProfileSnapshot, error) {
	snapshot, err := loadRouteProfileSourceSnapshotTx(ctx, tx, snapshotID)
	if err != nil {
		return nil, err
	}
	builder, err := buildRouteProfileSnapshot(snapshot)
	if err != nil {
		return nil, err
	}

	var profileID string
	if err := tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO route_profile_snapshots (
				route_snapshot_id, source_kind, source_version, source_hash, confidence,
				distance_m, elevation_gain_m, elevation_loss_m, forest_pct, protected_pct,
				surface_distribution, way_distribution, difficulty_scales, context_counts, ranges
			)
			VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10,
				$11, $12, $13, $14, $15
			)
			ON CONFLICT (route_snapshot_id, profile_schema_key, profile_schema_version) DO NOTHING
			RETURNING id
		)
		SELECT id FROM inserted
		UNION ALL
		SELECT id
		FROM route_profile_snapshots
		WHERE route_snapshot_id = $1
		  AND profile_schema_key = 'route_profile_v1'
		  AND profile_schema_version = 1
		LIMIT 1
	`, snapshot.ID, builder.SourceKind, builder.SourceVersion, builder.SourceHash, builder.Confidence,
		snapshot.TotalDistanceM, snapshot.TotalElevationGain, snapshot.TotalElevationLoss, builder.ForestPct, builder.ProtectedPct,
		builder.SurfaceDistribution, builder.WayDistribution, builder.DifficultyScales, builder.ContextCounts, builder.Ranges,
	).Scan(&profileID); err != nil {
		return nil, fmt.Errorf("insert route profile snapshot: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO route_course_profile_projection (
			subject_kind, subject_id, subject_version_id, subject_version_key,
			route_snapshot_id, profile_snapshot_id, visibility, access_scope_id,
			sport_type, sub_type, is_electric, is_indoor, is_virtual, is_commute,
			distance_m, elevation_gain_m, elevation_loss_m,
			sac_max_rank, sac_dominant_rank, sac_weighted_mean_rank,
			mtb_max_rank, mtb_dominant_rank, mtb_weighted_mean_rank,
			via_ferrata_max_rank, via_ferrata_dominant_rank, via_ferrata_weighted_mean_rank,
			forest_pct, protected_pct, primary_surface, primary_way_type, source_hash
		)
		VALUES (
			'planned_route_snapshot', $1, NULL, $2,
			$1, $3, 'private', NULL,
			$4, $5, $6, $7, $8, $9,
			$10, $11, $12,
			$13, $14, $15,
			$16, $17, $18,
			$19, $20, $21,
			$22, $23, $24, $25, $26
		)
		ON CONFLICT (subject_kind, subject_id, subject_version_key) DO UPDATE SET
			profile_snapshot_id = EXCLUDED.profile_snapshot_id,
			route_snapshot_id = EXCLUDED.route_snapshot_id,
			visibility = EXCLUDED.visibility,
			access_scope_id = EXCLUDED.access_scope_id,
			sport_type = EXCLUDED.sport_type,
			sub_type = EXCLUDED.sub_type,
			is_electric = EXCLUDED.is_electric,
			is_indoor = EXCLUDED.is_indoor,
			is_virtual = EXCLUDED.is_virtual,
			is_commute = EXCLUDED.is_commute,
			distance_m = EXCLUDED.distance_m,
			elevation_gain_m = EXCLUDED.elevation_gain_m,
			elevation_loss_m = EXCLUDED.elevation_loss_m,
			sac_max_rank = EXCLUDED.sac_max_rank,
			sac_dominant_rank = EXCLUDED.sac_dominant_rank,
			sac_weighted_mean_rank = EXCLUDED.sac_weighted_mean_rank,
			mtb_max_rank = EXCLUDED.mtb_max_rank,
			mtb_dominant_rank = EXCLUDED.mtb_dominant_rank,
			mtb_weighted_mean_rank = EXCLUDED.mtb_weighted_mean_rank,
			via_ferrata_max_rank = EXCLUDED.via_ferrata_max_rank,
			via_ferrata_dominant_rank = EXCLUDED.via_ferrata_dominant_rank,
			via_ferrata_weighted_mean_rank = EXCLUDED.via_ferrata_weighted_mean_rank,
			forest_pct = EXCLUDED.forest_pct,
			protected_pct = EXCLUDED.protected_pct,
			primary_surface = EXCLUDED.primary_surface,
			primary_way_type = EXCLUDED.primary_way_type,
			source_hash = EXCLUDED.source_hash,
			generated_at = NOW()
	`, snapshot.ID, routeProfileNoVersionUUID, profileID,
		snapshot.SportType, snapshot.SubType, snapshot.IsElectric, snapshot.IsIndoor, snapshot.IsVirtual, snapshot.IsCommute,
		snapshot.TotalDistanceM, snapshot.TotalElevationGain, snapshot.TotalElevationLoss,
		routeProfileRankPtr(builder.SAC.MaxLevelRank), routeProfileRankPtr(builder.SAC.DominantLevelRank), builder.SAC.WeightedMeanRank,
		routeProfileRankPtr(builder.MTB.MaxLevelRank), routeProfileRankPtr(builder.MTB.DominantLevelRank), builder.MTB.WeightedMeanRank,
		routeProfileRankPtr(builder.ViaFerrata.MaxLevelRank), routeProfileRankPtr(builder.ViaFerrata.DominantLevelRank), builder.ViaFerrata.WeightedMeanRank,
		builder.ForestPct, builder.ProtectedPct, builder.PrimarySurface, builder.PrimaryWayType, builder.SourceHash,
	); err != nil {
		return nil, fmt.Errorf("insert route profile projection: %w", err)
	}

	return loadRouteProfileSnapshotTx(ctx, tx, profileID)
}

type routeProfileSourceSnapshot struct {
	ID                 string
	SportType          string
	SubType            *string
	IsElectric         bool
	IsIndoor           bool
	IsVirtual          bool
	IsCommute          bool
	Segments           json.RawMessage
	Enrichment         json.RawMessage
	TotalDistanceM     *float64
	TotalElevationGain *float64
	TotalElevationLoss *float64
}

func loadRouteProfileSourceSnapshotTx(ctx context.Context, tx pgx.Tx, snapshotID string) (*routeProfileSourceSnapshot, error) {
	var snapshot routeProfileSourceSnapshot
	if err := tx.QueryRow(ctx, `
		SELECT id, sport_type, sub_type, is_electric, is_indoor, is_virtual, is_commute,
		       segments, enrichment, total_distance_m, total_elevation_gain, total_elevation_loss
		FROM planned_route_snapshots
		WHERE id = $1
	`, snapshotID).Scan(
		&snapshot.ID, &snapshot.SportType, &snapshot.SubType,
		&snapshot.IsElectric, &snapshot.IsIndoor, &snapshot.IsVirtual, &snapshot.IsCommute,
		&snapshot.Segments, &snapshot.Enrichment,
		&snapshot.TotalDistanceM, &snapshot.TotalElevationGain, &snapshot.TotalElevationLoss,
	); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func loadRouteProfileSnapshotTx(ctx context.Context, tx pgx.Tx, profileID string) (*model.RouteProfileSnapshot, error) {
	var profile model.RouteProfileSnapshot
	if err := tx.QueryRow(ctx, `
		SELECT id, route_snapshot_id, profile_schema_key, profile_schema_version,
		       source_kind, source_version, source_hash, confidence, computed_at,
		       distance_m, elevation_gain_m, elevation_loss_m,
		       forest_pct, protected_pct, surface_distribution, way_distribution,
		       difficulty_scales, context_counts, ranges
		FROM route_profile_snapshots
		WHERE id = $1
	`, profileID).Scan(
		&profile.ID, &profile.RouteSnapshotID, &profile.ProfileSchemaKey, &profile.ProfileSchemaVersion,
		&profile.SourceKind, &profile.SourceVersion, &profile.SourceHash, &profile.Confidence, &profile.ComputedAt,
		&profile.DistanceM, &profile.ElevationGainM, &profile.ElevationLossM,
		&profile.ForestPct, &profile.ProtectedPct, &profile.SurfaceDistribution, &profile.WayDistribution,
		&profile.DifficultyScales, &profile.ContextCounts, &profile.Ranges,
	); err != nil {
		return nil, err
	}
	return &profile, nil
}

type routeProfileBuilder struct {
	SourceKind          string
	SourceVersion       string
	SourceHash          string
	Confidence          string
	ForestPct           *float64
	ProtectedPct        *float64
	SurfaceDistribution json.RawMessage
	WayDistribution     json.RawMessage
	DifficultyScales    json.RawMessage
	ContextCounts       json.RawMessage
	Ranges              json.RawMessage
	PrimarySurface      *string
	PrimaryWayType      *string
	SAC                 routeProfileScaleSummary
	MTB                 routeProfileScaleSummary
	ViaFerrata          routeProfileScaleSummary
}

type routeProfileEnrichment struct {
	EnrichmentVersion int                                 `json:"enrichment_version"`
	ForestPct         *float64                            `json:"forest_pct"`
	ProtectedPct      *float64                            `json:"protected_pct"`
	ForestRanges      []routeProfileCoordRange            `json:"forest_ranges"`
	ProtectedRanges   []routeProfileCoordRange            `json:"protected_ranges"`
	SACRanges         map[string][]routeProfileCoordRange `json:"sac_ranges"`
	MTBRanges         map[string][]routeProfileCoordRange `json:"mtb_ranges"`
	VFRanges          map[string][]routeProfileCoordRange `json:"vf_ranges"`
	VFUnknownRanges   []routeProfileCoordRange            `json:"vf_unknown_ranges"`
	SummitsPassed     []json.RawMessage                   `json:"summits_passed"`
}

type routeProfileCoordRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type routeProfileScaleSummary struct {
	State             string                                   `json:"state,omitempty"`
	MaxLevelKey       string                                   `json:"max_level_key,omitempty"`
	MaxLevelRank      *int                                     `json:"max_level_rank,omitempty"`
	DominantLevelKey  string                                   `json:"dominant_level_key,omitempty"`
	DominantLevelRank *int                                     `json:"dominant_level_rank,omitempty"`
	WeightedMeanRank  *float64                                 `json:"weighted_mean_rank,omitempty"`
	KnownDistanceM    float64                                  `json:"known_distance_m"`
	UnknownDistanceM  float64                                  `json:"unknown_distance_m"`
	Distribution      map[string]routeProfileDistributionEntry `json:"distribution"`
	Ranges            map[string][]routeProfileCoordRange      `json:"ranges"`
}

type routeProfileDistributionEntry struct {
	DistanceM float64 `json:"distance_m"`
	Rank      *int    `json:"rank,omitempty"`
}

type routeProfileSubSegment struct {
	FromIdx int    `json:"from_idx"`
	ToIdx   int    `json:"to_idx"`
	Highway string `json:"highway"`
	Surface string `json:"surface"`
}

func buildRouteProfileSnapshot(snapshot *routeProfileSourceSnapshot) (*routeProfileBuilder, error) {
	var segments []model.RouteSegment
	if len(snapshot.Segments) > 0 {
		if err := json.Unmarshal(snapshot.Segments, &segments); err != nil {
			return nil, fmt.Errorf("unmarshal route snapshot segments: %w", err)
		}
	}
	coords := routeProfileCoordinateBasis(segments)
	coordHash, err := routeProfileJSONHash(coords)
	if err != nil {
		return nil, err
	}

	builder := &routeProfileBuilder{
		SourceKind:    "planner_enrichment",
		SourceVersion: "missing",
		SourceHash:    stableHashBytes(snapshot.Enrichment),
		Confidence:    "low",
	}
	enrichment, hasEnrichment, err := routeProfileDecodeEnrichment(snapshot.Enrichment)
	if err != nil {
		return nil, err
	}
	if hasEnrichment {
		builder.SourceVersion = strconv.Itoa(enrichment.EnrichmentVersion)
		builder.Confidence = "medium"
		builder.ForestPct = routeProfileClampPct(enrichment.ForestPct)
		builder.ProtectedPct = routeProfileClampPct(enrichment.ProtectedPct)
	}

	builder.SAC = routeProfileBuildScale(enrichment.SACRanges, nil, routeProfileSACRanks, coords)
	builder.MTB = routeProfileBuildScale(enrichment.MTBRanges, nil, routeProfileMTBRanks, coords)
	builder.ViaFerrata = routeProfileBuildScale(enrichment.VFRanges, enrichment.VFUnknownRanges, routeProfileViaFerrataRanks, coords)

	surfaceDistribution, wayDistribution, primarySurface, primaryWay := routeProfileSurfaceWayDistribution(segments)
	builder.PrimarySurface = primarySurface
	builder.PrimaryWayType = primaryWay

	difficultyScales := map[string]any{
		"sac":         builder.SAC,
		"mtb":         builder.MTB,
		"via_ferrata": builder.ViaFerrata,
		"mtb_uphill":  map[string]any{"state": "reserved", "reason": "source_mtb_uphill_ranges_missing"},
	}
	ranges := map[string]any{
		"coordinate_basis_hash": coordHash,
		"sac_ranges":            enrichment.SACRanges,
		"mtb_ranges":            enrichment.MTBRanges,
		"vf_ranges":             enrichment.VFRanges,
		"vf_unknown_ranges":     enrichment.VFUnknownRanges,
		"forest_ranges":         enrichment.ForestRanges,
		"protected_ranges":      enrichment.ProtectedRanges,
	}
	contextCounts := map[string]int{
		"summits_passed": len(enrichment.SummitsPassed),
	}

	builder.SurfaceDistribution, err = json.Marshal(surfaceDistribution)
	if err != nil {
		return nil, err
	}
	builder.WayDistribution, err = json.Marshal(wayDistribution)
	if err != nil {
		return nil, err
	}
	builder.DifficultyScales, err = json.Marshal(difficultyScales)
	if err != nil {
		return nil, err
	}
	builder.ContextCounts, err = json.Marshal(contextCounts)
	if err != nil {
		return nil, err
	}
	builder.Ranges, err = json.Marshal(ranges)
	if err != nil {
		return nil, err
	}
	return builder, nil
}

func routeProfileDecodeEnrichment(raw json.RawMessage) (routeProfileEnrichment, bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return routeProfileEnrichment{}, false, nil
	}
	var enrichment routeProfileEnrichment
	if err := json.Unmarshal(raw, &enrichment); err != nil {
		return routeProfileEnrichment{}, false, fmt.Errorf("unmarshal route enrichment: %w", err)
	}
	return enrichment, enrichment.EnrichmentVersion == 1, nil
}

func routeProfileCoordinateBasis(segments []model.RouteSegment) [][]float64 {
	var coords [][]float64
	for i, segment := range segments {
		if segment.Geometry == nil {
			continue
		}
		start := 0
		if i > 0 {
			start = 1
		}
		for j := start; j < len(segment.Geometry.Coordinates); j++ {
			coord := segment.Geometry.Coordinates[j]
			coords = append(coords, coord)
		}
	}
	return coords
}

func routeProfileBuildScale(ranges map[string][]routeProfileCoordRange, unknownRanges []routeProfileCoordRange, ranks map[string]int, coords [][]float64) routeProfileScaleSummary {
	summary := routeProfileScaleSummary{
		Distribution: map[string]routeProfileDistributionEntry{},
		Ranges:       map[string][]routeProfileCoordRange{},
	}
	var weightedSum float64
	for level, levelRanges := range ranges {
		rank, ok := ranks[level]
		if !ok {
			continue
		}
		var distance float64
		for _, r := range levelRanges {
			distance += routeProfileRangeDistanceM(coords, r)
		}
		if distance <= 0 {
			continue
		}
		rankCopy := rank
		summary.Distribution[level] = routeProfileDistributionEntry{DistanceM: distance, Rank: &rankCopy}
		summary.Ranges[level] = append([]routeProfileCoordRange(nil), levelRanges...)
		summary.KnownDistanceM += distance
		weightedSum += float64(rank) * distance
		if summary.MaxLevelRank == nil || rank > *summary.MaxLevelRank {
			summary.MaxLevelKey = level
			summary.MaxLevelRank = &rankCopy
		}
		if summary.DominantLevelRank == nil || distance > summary.Distribution[summary.DominantLevelKey].DistanceM {
			dominantRank := rank
			summary.DominantLevelKey = level
			summary.DominantLevelRank = &dominantRank
		}
	}
	for _, r := range unknownRanges {
		summary.UnknownDistanceM += routeProfileRangeDistanceM(coords, r)
	}
	if summary.KnownDistanceM > 0 {
		mean := weightedSum / summary.KnownDistanceM
		summary.WeightedMeanRank = &mean
	}
	return summary
}

func routeProfileRangeDistanceM(coords [][]float64, r routeProfileCoordRange) float64 {
	if len(coords) < 2 || r.To <= r.From {
		return 0
	}
	from := max(0, r.From)
	to := min(r.To, len(coords)-1)
	if from >= to {
		return 0
	}
	var distance float64
	for i := from; i < to; i++ {
		a := coords[i]
		b := coords[i+1]
		if len(a) < 2 || len(b) < 2 {
			continue
		}
		distance += geo.DistanceHaversine(a[1], a[0], b[1], b[0])
	}
	return distance
}

func routeProfileSurfaceWayDistribution(segments []model.RouteSegment) (map[string]routeProfileDistributionEntry, map[string]routeProfileDistributionEntry, *string, *string) {
	surfaceDistances := map[string]float64{}
	wayDistances := map[string]float64{}
	for _, segment := range segments {
		if segment.Geometry == nil || len(segment.SubSegments) == 0 {
			continue
		}
		var subSegments []routeProfileSubSegment
		if err := json.Unmarshal(segment.SubSegments, &subSegments); err != nil {
			continue
		}
		for _, sub := range subSegments {
			distance := routeProfileRangeDistanceM(segment.Geometry.Coordinates, routeProfileCoordRange{From: sub.FromIdx, To: sub.ToIdx})
			if distance <= 0 {
				continue
			}
			surface := sub.Surface
			if surface == "" {
				surface = "unknown"
			}
			highway := sub.Highway
			if highway == "" {
				highway = "unknown"
			}
			surfaceDistances[surface] += distance
			wayDistances[highway] += distance
		}
	}
	return routeProfileDistribution(surfaceDistances), routeProfileDistribution(wayDistances), routeProfilePrimaryKey(surfaceDistances), routeProfilePrimaryKey(wayDistances)
}

func routeProfileDistribution(distances map[string]float64) map[string]routeProfileDistributionEntry {
	out := make(map[string]routeProfileDistributionEntry, len(distances))
	for key, distance := range distances {
		out[key] = routeProfileDistributionEntry{DistanceM: distance}
	}
	return out
}

func routeProfilePrimaryKey(distances map[string]float64) *string {
	var keys []string
	for key := range distances {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var bestKey string
	var bestDistance float64
	for _, key := range keys {
		if distances[key] > bestDistance {
			bestKey = key
			bestDistance = distances[key]
		}
	}
	if bestKey == "" || bestDistance <= 0 {
		return nil
	}
	return &bestKey
}

func routeProfileClampPct(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clamped := math.Max(0, math.Min(1, *value))
	return &clamped
}

func routeProfileRankPtr(rank *int) *int {
	if rank == nil {
		return nil
	}
	value := *rank
	return &value
}

func routeProfileJSONHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal route profile hash input: %w", err)
	}
	return stableHashBytes(raw), nil
}
