package sport

import "testing"

func TestActivityFilterProjectionPresetByKey(t *testing.T) {
	if _, ok := ActivityFilterProjectionPresetByKey("activity_all"); !ok {
		t.Fatal("activity_all projection preset missing")
	}
	if _, ok := ActivityFilterProjectionPresetByKey("map_panel_gps"); !ok {
		t.Fatal("map_panel_gps projection preset missing")
	}
}

func TestActivityFilterProjectionWaterGroups(t *testing.T) {
	preset, ok := ActivityFilterProjectionPresetByKey("activity_all")
	if !ok {
		t.Fatal("activity_all projection preset missing")
	}
	got := groupKeysWithPrefix(preset.Groups, "water_")
	want := []string{"water_swim", "water_paddle", "water_board", "water_boat", "water_dive", "water_other"}
	if !sameStrings(got, want) {
		t.Fatalf("water groups = %v, want %v", got, want)
	}
	waterDive := projectionGroupByKey(preset, "water_dive")
	if waterDive == nil {
		t.Fatal("water_dive group missing")
	}
	if !projectionGroupHasLeaf(*waterDive, "diving", "") || !projectionGroupHasLeaf(*waterDive, "snorkeling", "") {
		t.Fatalf("water_dive leaves = %#v, want diving and snorkeling", waterDive.Leaves)
	}
}

func TestActivityFilterProjectionKeepsExplicitBroadFallbackLeaf(t *testing.T) {
	preset, ok := ActivityFilterProjectionPresetByKey("activity_all")
	if !ok {
		t.Fatal("activity_all projection preset missing")
	}
	winter := projectionGroupByKey(preset, "winter")
	if winter == nil {
		t.Fatal("winter group missing")
	}
	if !projectionGroupHasLeaf(*winter, "skiing", "") {
		t.Fatalf("winter leaves = %#v, want explicit skiing broad fallback leaf", winter.Leaves)
	}
}

func TestActivityFilterProjectionHasNoDuplicateTupleKeysOrLabels(t *testing.T) {
	for _, preset := range ActivityFilterProjectionPresets() {
		tupleKeys := map[string]struct{}{}
		for _, group := range preset.Groups {
			labels := map[string]struct{}{}
			for _, leaf := range group.Leaves {
				key := leaf.SportType
				if leaf.SubType != "" {
					key += ":" + leaf.SubType
				}
				if _, ok := tupleKeys[key]; ok {
					t.Fatalf("preset %s contains duplicate tuple %s", preset.Key, key)
				}
				tupleKeys[key] = struct{}{}
				if _, ok := labels[leaf.LabelDE]; ok {
					t.Fatalf("preset %s group %s contains duplicate label %q", preset.Key, group.Key, leaf.LabelDE)
				}
				labels[leaf.LabelDE] = struct{}{}
			}
		}
	}
}

func TestActivityFilterProjectionAccessorsReturnCopies(t *testing.T) {
	presets := ActivityFilterProjectionPresets()
	if len(presets) == 0 || len(presets[0].Groups) == 0 || len(presets[0].Groups[0].Leaves) == 0 {
		t.Fatal("projection fixture unexpectedly empty")
	}
	originalPresetKey := presets[0].Key
	originalGroupKey := presets[0].Groups[0].Key
	originalLeafLabel := presets[0].Groups[0].Leaves[0].LabelDE
	presets[0].Key = "mutated"
	presets[0].Groups[0].Key = "mutated"
	presets[0].Groups[0].Leaves[0].LabelDE = "mutated"

	again := ActivityFilterProjectionPresets()
	if again[0].Key != originalPresetKey ||
		again[0].Groups[0].Key != originalGroupKey ||
		again[0].Groups[0].Leaves[0].LabelDE != originalLeafLabel {
		t.Fatalf("ActivityFilterProjectionPresets returned mutable generated data: %#v", again[0])
	}
}

func TestSegmentProjectionEntriesForType(t *testing.T) {
	winter := SegmentProjectionEntriesForType("wintersport")
	if !segmentProjectionHasEntry(winter, "snowshoeing", "", "foot") {
		t.Fatalf("wintersport entries = %#v, want snowshoeing foot entry", winter)
	}
	water := SegmentProjectionEntriesForType("wassersport")
	if segmentProjectionHasEntry(water, "diving", "", "") {
		t.Fatalf("wassersport entries = %#v, want no diving entry", water)
	}
	if !segmentProjectionHasEntry(water, "snorkeling", "", "generic") {
		t.Fatalf("wassersport entries = %#v, want snorkeling generic entry", water)
	}
	for _, entry := range SegmentProjectionEntriesForType("standard") {
		if entry.Domain != "real_world_gps" {
			t.Fatalf("standard active entries include reserved domain: %#v", entry)
		}
	}

	original := SegmentProjectionEntriesForType("wintersport")
	if len(original) == 0 {
		t.Fatal("wintersport entries empty")
	}
	originalSport := original[0].SportType
	original[0].SportType = "mutated"
	again := SegmentProjectionEntriesForType("wintersport")
	if again[0].SportType != originalSport {
		t.Fatalf("SegmentProjectionEntriesForType returned mutable generated data: %#v", again[0])
	}
}

func TestSegmentProjectionGroupForPredicates(t *testing.T) {
	if got := SegmentProjectionGroupForPredicates([]Predicate{{Sport: "snowshoeing"}}); got != "foot" {
		t.Fatalf("snowshoeing group = %q, want foot", got)
	}
	if got := SegmentProjectionGroupForPredicates([]Predicate{{Sport: "alpine_skiing"}, {Sport: "snowshoeing"}}); got != "generic" {
		t.Fatalf("mixed winter group = %q, want generic", got)
	}
	if got := SegmentProjectionGroupForPredicates(nil); got != "generic" {
		t.Fatalf("nil predicate group = %q, want generic", got)
	}
}

func TestExpandActivityTraitBucketsIncludesModifierStates(t *testing.T) {
	key := "atb:v1:sport=cycling:sub=mountain:indoor=0:virtual=1:electric=1"
	bucket, ok := ActivityTraitBucketByKey(key)
	if !ok {
		t.Fatalf("ActivityTraitBucketByKey(%q) missing", key)
	}
	if bucket.SportType != "cycling" || bucket.SubType != "mountain" || !bucket.IsVirtual || !bucket.IsElectric || bucket.IsIndoor {
		t.Fatalf("bucket = %#v, want cycling mountain virtual electric", bucket)
	}
}

func TestBuildActivityTraitBucketKeyIsStable(t *testing.T) {
	want := "atb:v1:sport=cycling:sub=mountain:indoor=0:virtual=0:electric=1"
	got := BuildActivityTraitBucketKey(ActivityTraitBucketInput{
		SportType:  "cycling",
		SubType:    "mountain",
		IsElectric: true,
	})
	if got != want {
		t.Fatalf("BuildActivityTraitBucketKey() = %q, want %q", got, want)
	}

	want = "atb:v1:sport=running:sub=_:indoor=1:virtual=0:electric=0"
	got = BuildActivityTraitBucketKey(ActivityTraitBucketInput{
		SportType: "running",
		IsIndoor:  true,
	})
	if got != want {
		t.Fatalf("BuildActivityTraitBucketKey() = %q, want %q", got, want)
	}
}

func TestCourseEligibilityPolicyForSegmentPreservesLegacyNullWildcard(t *testing.T) {
	rows := CourseEligibilityRowsForSegment(testCourseEligibilityInput("standard", "bike", nil, false))
	if !courseEligibilityRowsContain(rows, "cycling", "mountain", true) {
		t.Fatalf("rows missing cycling mountain bucket: %#v", rows)
	}
	if courseEligibilityRowsContainGroup(rows, "foot") {
		t.Fatalf("bike wildcard included foot rows: %#v", rows)
	}
}

func TestCourseEligibilityPolicyForSegmentPreservesWinterWaterNullGuard(t *testing.T) {
	for _, segmentType := range []string{"wintersport", "wassersport"} {
		rows := CourseEligibilityRowsForSegment(testCourseEligibilityInput(segmentType, "ski", nil, false))
		if len(rows) != 0 {
			t.Fatalf("%s null wildcard rows = %#v, want none", segmentType, rows)
		}
	}
}

func TestCourseEligibilityPolicyForSegmentTreatsEmptyPredicatesAsNoRows(t *testing.T) {
	rows := CourseEligibilityRowsForSegment(testCourseEligibilityInput("standard", "bike", []Predicate{}, true))
	if len(rows) != 0 {
		t.Fatalf("empty-predicate rows = %#v, want none", rows)
	}
}

func TestCourseEligibilityPolicyForParentPredicateIncludesSubtypeBuckets(t *testing.T) {
	rows := CourseEligibilityRowsForSegment(testCourseEligibilityInput("standard", "bike", []Predicate{{Sport: "cycling"}}, true))
	if !courseEligibilityRowsContain(rows, "cycling", "mountain", true) {
		t.Fatalf("parent cycling rows missing mountain subtype: %#v", rows)
	}
}

func TestCourseEligibilityPolicyForStoredGroupFiltersLegacyWildcard(t *testing.T) {
	rows := CourseEligibilityRowsForSegment(testCourseEligibilityInput("standard", "foot", nil, false))
	if courseEligibilityRowsContain(rows, "cycling", "mountain", true) {
		t.Fatalf("foot null wildcard included cycling bucket: %#v", rows)
	}
	if !courseEligibilityRowsContain(rows, "running", "", true) {
		t.Fatalf("foot null wildcard missing running bucket: %#v", rows)
	}
}

func TestCourseEligibilityPolicyForNullWildcardIsNotPresetLimited(t *testing.T) {
	rows := CourseEligibilityRowsForSegment(testCourseEligibilityInput("trail", "swim", nil, false))
	if !courseEligibilityRowsContain(rows, "swimming", "open_water", true) {
		t.Fatalf("trail swim wildcard missing swimming bucket outside trail preset: %#v", rows)
	}
}

func TestActivityTraitBucketKeyForActivityFallsBackToParentForFutureSubtype(t *testing.T) {
	got, ok := ActivityTraitBucketKeyForActivityLegacyCandidate("cycling", "future_bike", ActivityModifierState{})
	want := "atb:v1:sport=cycling:sub=_:indoor=0:virtual=0:electric=0"
	if !ok || got != want {
		t.Fatalf("future subtype bucket = %q/%t, want %q/true", got, ok, want)
	}
}

func TestActivityTraitBucketKeyForActivityPreservesGenericFallback(t *testing.T) {
	got, ok := ActivityTraitBucketKeyForActivityLegacyCandidate("future_sport", "", ActivityModifierState{})
	want := "atb:v1:sport=generic:sub=_:indoor=0:virtual=0:electric=0"
	if !ok || got != want {
		t.Fatalf("generic fallback bucket = %q/%t, want %q/true", got, ok, want)
	}
}

func TestCourseEligibilityPolicySourceHashChangesWhenInputsChange(t *testing.T) {
	base := CourseEligibilityRowsForSegment(testCourseEligibilityInput("standard", "bike", nil, false))
	changed := CourseEligibilityRowsForSegment(testCourseEligibilityInput("standard", "foot", nil, false))
	if len(base) == 0 || len(changed) == 0 {
		t.Fatalf("unexpected empty rows: base=%#v changed=%#v", base, changed)
	}
	if base[0].SourceHash == changed[0].SourceHash {
		t.Fatalf("source hash did not change for changed sport_group: %q", base[0].SourceHash)
	}
}

func testCourseEligibilityInput(segmentType, sportGroup string, preds []Predicate, predsSet bool) CourseEligibilityInput {
	return CourseEligibilityInput{
		TargetKind:           "segment",
		TargetID:             "segment-1",
		TargetVersionID:      "version-1",
		SegmentType:          segmentType,
		SportGroup:           sportGroup,
		AllowedPredicates:    preds,
		AllowedPredicatesSet: predsSet,
		Bidirectional:        false,
		Visibility:           "public",
		ElevationKind:        "mixed",
		AdapterKey:           "legacy_segment_adapter_v1",
		AdapterVersion:       1,
	}
}

func courseEligibilityRowsContain(rows []CourseEligibilityProjectionRow, sportType, subType string, matchAllowed bool) bool {
	wantSubType := subType
	if wantSubType == "" {
		wantSubType = "_"
	}
	prefix := "atb:v1:sport=" + sportType + ":sub=" + wantSubType + ":"
	for _, row := range rows {
		if row.MatchAllowed == matchAllowed && len(row.ActivityTraitBucketKey) >= len(prefix) && row.ActivityTraitBucketKey[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func courseEligibilityRowsContainGroup(rows []CourseEligibilityProjectionRow, group string) bool {
	for _, row := range rows {
		if row.ActivityGroup == group {
			return true
		}
	}
	return false
}

func projectionGroupByKey(preset ActivityFilterProjectionPreset, key string) *ActivityFilterProjectionGroup {
	for i := range preset.Groups {
		if preset.Groups[i].Key == key {
			return &preset.Groups[i]
		}
	}
	return nil
}

func projectionGroupHasLeaf(group ActivityFilterProjectionGroup, sportType, subType string) bool {
	for _, leaf := range group.Leaves {
		if leaf.SportType == sportType && leaf.SubType == subType {
			return true
		}
	}
	return false
}

func segmentProjectionHasEntry(entries []SegmentProjectionEntry, sportType, subType, group string) bool {
	for _, entry := range entries {
		if entry.SportType == sportType && entry.SubType == subType && (group == "" || entry.SportGroup == group) {
			return true
		}
	}
	return false
}

func groupKeysWithPrefix(groups []ActivityFilterProjectionGroup, prefix string) []string {
	keys := []string{}
	for _, group := range groups {
		if len(group.Key) >= len(prefix) && group.Key[:len(prefix)] == prefix {
			keys = append(keys, group.Key)
		}
	}
	return keys
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
