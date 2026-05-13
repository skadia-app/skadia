package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

func TestRegisterGeoToolsOwnershipSplit(t *testing.T) {
	reg := NewRegistry()
	RegisterGeoTools(reg)

	tests := map[string]bool{
		"query_geo_features":          true,
		"query_activity_geo_context":  true,
		"search_pois_nearby":          false,
		"search_trails_by_difficulty": false,
	}
	for name, wantUserOwned := range tests {
		tool := reg.tools[name]
		if tool == nil {
			t.Fatalf("tool %q was not registered", name)
		}
		if tool.Permission != PermissionRead {
			t.Fatalf("%s permission = %q, want read", name, tool.Permission)
		}
		if tool.IsUserOwned != wantUserOwned {
			t.Fatalf("%s IsUserOwned = %v, want %v", name, tool.IsUserOwned, wantUserOwned)
		}
		if err := validateSchema(name, tool.InputSchema); err != nil {
			t.Fatalf("%s schema invalid: %v", name, err)
		}
	}
}

func TestBuildQueryGeoFeaturesSQLKeepsGastronomyUserOwned(t *testing.T) {
	days := 30
	limitArg := 500
	args := queryGeoFeaturesArgs{FeatureType: "gastronomy", Days: &days, Limit: &limitArg}
	countSQL, dataSQL, params, limit, featureType, err := buildQueryGeoFeaturesSQL("00000000-0000-0000-0000-000000000001", args, fixedAgentToolNow())
	if err != nil {
		t.Fatalf("buildQueryGeoFeaturesSQL returned error: %v", err)
	}

	for _, sql := range []string{countSQL, dataSQL} {
		if !strings.Contains(sql, "JOIN activities a ON a.id = s.activity_id") {
			t.Fatalf("gastronomy SQL must join activities for ownership, got:\n%s", sql)
		}
		if !strings.Contains(sql, "a.user_id = $1") {
			t.Fatalf("gastronomy SQL must filter by user_id, got:\n%s", sql)
		}
	}
	if featureType != "gastronomy" {
		t.Fatalf("featureType = %q, want gastronomy", featureType)
	}
	if limit != 100 {
		t.Fatalf("limit = %d, want clamp to 100", limit)
	}
	if len(params) != 2 {
		t.Fatalf("params len = %d, want user and since params", len(params))
	}
}

func TestBuildQueryGeoFeaturesSQLMapsWaterAndFiltersActivities(t *testing.T) {
	subType := "trail"
	virtual := false
	args := queryGeoFeaturesArgs{
		FeatureType: "lake",
		SubType:     &subType,
		IsVirtual:   &virtual,
		ActivityIDs: []string{
			"00000000-0000-0000-0000-0000000000a1",
			"00000000-0000-0000-0000-0000000000a2",
		},
	}
	_, dataSQL, params, _, featureType, err := buildQueryGeoFeaturesSQL("00000000-0000-0000-0000-000000000001", args, fixedAgentToolNow())
	if err != nil {
		t.Fatalf("buildQueryGeoFeaturesSQL returned error: %v", err)
	}

	if featureType != "still_water" {
		t.Fatalf("featureType = %q, want still_water", featureType)
	}
	for _, want := range []string{
		"JOIN activities a ON a.id = gt.activity_id",
		"a.user_id = $1",
		"gt.category = $3",
		"a.sub_type = $4",
		"a.is_virtual = $5",
		"a.id = ANY($6::uuid[])",
	} {
		if !strings.Contains(dataSQL, want) {
			t.Fatalf("SQL missing %q:\n%s", want, dataSQL)
		}
	}
	if len(params) != 6 {
		t.Fatalf("params len = %d, want 6", len(params))
	}
}

func TestBuildQueryGeoFeatureGroupsUsesFullFilteredSet(t *testing.T) {
	limitArg := 1
	args := queryGeoFeaturesArgs{
		FeatureType: "summit",
		GroupBy:     "year",
		Limit:       &limitArg,
	}
	sql, params, err := buildQueryGeoFeatureGroupsSQL("00000000-0000-0000-0000-000000000001", args, fixedAgentToolNow())
	if err != nil {
		t.Fatalf("buildQueryGeoFeatureGroupsSQL returned error: %v", err)
	}
	for _, want := range []string{
		"COUNT(*) OVER (PARTITION BY group_key)",
		"ROW_NUMBER() OVER (PARTITION BY group_key",
		"FROM activity_geo_tags gt JOIN activities a ON a.id = gt.activity_id",
		"a.user_id = $1",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("group SQL missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "features") {
		t.Fatalf("group SQL must not group over the already limited feature result:\n%s", sql)
	}
	if len(params) != 3 {
		t.Fatalf("params len = %d, want user, since, feature_type", len(params))
	}
}

func TestBuildQueryGeoFeatureGroupsByFeatureUsesSourceExpressions(t *testing.T) {
	args := queryGeoFeaturesArgs{FeatureType: "gastronomy", GroupBy: "feature"}
	sql, _, err := buildQueryGeoFeatureGroupsSQL("00000000-0000-0000-0000-000000000001", args, fixedAgentToolNow())
	if err != nil {
		t.Fatalf("buildQueryGeoFeatureGroupsSQL returned error: %v", err)
	}
	if !strings.Contains(sql, "COALESCE(NULLIF(s.poi_name, ''), s.poi_type, 'unknown') AS group_key") {
		t.Fatalf("feature group must use source columns, got:\n%s", sql)
	}
}

func TestExecuteQueryGeoFeaturesSetsRLSContext(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	activityID := insertActivityToolActivity(t, pool, userID, "Summit Tour")
	if _, err := pool.Exec(context.Background(), `UPDATE activities SET sport_type = 'hiking', started_at = '2025-07-01T10:00:00Z' WHERE id = $1`, activityID); err != nil {
		t.Fatalf("update activity: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO activity_geo_tags (activity_id, category, name, ele_m)
		VALUES ($1, 'summit', 'Test Summit', 1234)
	`, activityID); err != nil {
		t.Fatalf("insert geo tag: %v", err)
	}

	result, err := executeQueryGeoFeatures(context.Background(), &Deps{DB: &database.DB{Pool: pool}}, userID, json.RawMessage(`{
		"feature_type": "summit",
		"date_from": "2025-01-01",
		"date_to": "2025-12-31",
		"limit": 10
	}`))
	if err != nil {
		t.Fatalf("executeQueryGeoFeatures: %v", err)
	}
	out := result.(map[string]any)
	if out["total_count"] != int64(1) {
		t.Fatalf("total_count = %v, want 1", out["total_count"])
	}
	features := out["features"].([]map[string]any)
	if features[0]["ele"] != float64(1234) {
		t.Fatalf("ele = %v, want ele_m fallback", features[0]["ele"])
	}
	if features[0]["activity_id"] != activityID || features[0]["sport_type"] != "hiking" {
		t.Fatalf("feature activity fields = %#v", features[0])
	}
}

func TestBuildSearchPOIsNearbySQLDoesNotUseUserFilter(t *testing.T) {
	radius := 1000
	limitArg := 500
	sql, params, err := buildSearchPOIsNearbySQL(searchPOIsNearbyArgs{
		Lat: 47.2, Lng: 11.4, RadiusM: &radius, Category: "summit", Limit: &limitArg,
	})
	if err != nil {
		t.Fatalf("buildSearchPOIsNearbySQL returned error: %v", err)
	}
	if strings.Contains(sql, "user_id") {
		t.Fatalf("shared OSM POI SQL must not use user_id:\n%s", sql)
	}
	if !strings.Contains(sql, "FROM osm_pois") || !strings.Contains(sql, "category = $4") {
		t.Fatalf("unexpected POI SQL:\n%s", sql)
	}
	if params[len(params)-1] != 100 {
		t.Fatalf("limit param = %v, want clamp to 100", params[len(params)-1])
	}
}

func TestBuildSearchPOIsNearbyRejectsOutOfRangeCoordinates(t *testing.T) {
	if _, _, err := buildSearchPOIsNearbySQL(searchPOIsNearbyArgs{Lat: 91, Lng: 11.4}); err == nil {
		t.Fatal("buildSearchPOIsNearbySQL accepted invalid coordinates")
	}
}

func TestBuildSearchTrailsSQLDifficultyFilters(t *testing.T) {
	sql, params, err := buildSearchTrailsByDifficultySQL(searchTrailsByDifficultyArgs{
		Lat: 47.2, Lng: 11.4, MaxSacScale: "demanding_mountain_hiking", MaxMTBScale: "S2", Surface: "gravel",
	})
	if err != nil {
		t.Fatalf("buildSearchTrailsByDifficultySQL returned error: %v", err)
	}
	for _, want := range []string{
		"FROM osm_ways",
		"sac_scale = ANY($4)",
		"mtb_scale = ANY($5)",
		"surface = $6",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SQL missing %q:\n%s", want, sql)
		}
	}
	mtbValues, ok := params[4].([]string)
	if !ok {
		t.Fatalf("mtb param type = %T, want []string", params[4])
	}
	if strings.Join(mtbValues, ",") != "0,1,2" {
		t.Fatalf("mtb values = %#v, want 0..2", mtbValues)
	}
}

func TestBuildSearchTrailsSQLRejectsInvalidDifficulty(t *testing.T) {
	if _, _, err := buildSearchTrailsByDifficultySQL(searchTrailsByDifficultyArgs{
		Lat: 47.2, Lng: 11.4, MaxSacScale: "not-a-scale",
	}); err == nil {
		t.Fatal("buildSearchTrailsByDifficultySQL accepted invalid SAC scale")
	}
	if _, _, err := buildSearchTrailsByDifficultySQL(searchTrailsByDifficultyArgs{
		Lat: 47.2, Lng: 11.4, MaxMTBScale: "S9",
	}); err == nil {
		t.Fatal("buildSearchTrailsByDifficultySQL accepted invalid MTB scale")
	}
}

func fixedAgentToolNow() time.Time {
	return time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
}
