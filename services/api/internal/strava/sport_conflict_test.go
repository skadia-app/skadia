package strava

import "testing"

func TestSportConflictMetadataKeepsTupleAndModifiers(t *testing.T) {
	meta := sportConflictMetadata(
		sportConflictSnapshot{Sport: "cycling", SubType: "road", IsElectric: true},
		sportConflictSnapshot{Sport: "cycling", SubType: "mountain", IsIndoor: true, IsVirtual: true},
		"fit",
	)

	if meta["platform_sub_type"] != "road" {
		t.Fatalf("platform_sub_type = %#v, want road", meta["platform_sub_type"])
	}
	if meta["file_sub_type"] != "mountain" {
		t.Fatalf("file_sub_type = %#v, want mountain", meta["file_sub_type"])
	}
	if meta["platform_is_electric"] != true {
		t.Fatalf("platform_is_electric = %#v, want true", meta["platform_is_electric"])
	}
	if meta["file_is_indoor"] != true || meta["file_is_virtual"] != true {
		t.Fatalf("file modifiers = indoor %#v virtual %#v, want true/true", meta["file_is_indoor"], meta["file_is_virtual"])
	}
}
