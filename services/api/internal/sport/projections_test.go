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
