package graph

import "testing"

func TestEncodeDecodeHighway(t *testing.T) {
	for name, val := range highwayEnum {
		tags := RawTags{Highway: name}
		encoded := EncodeOSMTags(&tags)
		got := DecodeHighway(encoded)
		if got != val {
			t.Errorf("highway %q: encode/decode got %d, want %d", name, got, val)
		}
	}
}

func TestEncodeDecodeSurface(t *testing.T) {
	for name, val := range surfaceEnum {
		tags := RawTags{Surface: name}
		encoded := EncodeOSMTags(&tags)
		got := DecodeSurface(encoded)
		if got != val {
			t.Errorf("surface %q: encode/decode got %d, want %d", name, got, val)
		}
	}
}

func TestEncodeDecodeAccess(t *testing.T) {
	for name, val := range accessEnum {
		tags := RawTags{Access: name}
		encoded := EncodeOSMTags(&tags)
		got := DecodeAccess(encoded)
		if got != val {
			t.Errorf("access %q: encode/decode got %d, want %d", name, got, val)
		}
	}
}

func TestEncodeBooleanFlags(t *testing.T) {
	tags := RawTags{
		Oneway: true, Bridge: true, Tunnel: false,
		Ford: true, Lit: true, Segregated: true, BicycleRoad: true,
	}
	encoded := EncodeOSMTags(&tags)
	if !DecodeBool(encoded, 36) { t.Error("oneway should be true") }
	if DecodeBool(encoded, 37) { t.Error("oneway_bicycle should be false") }
	if !DecodeBool(encoded, 38) { t.Error("bridge should be true") }
	if DecodeBool(encoded, 39) { t.Error("tunnel should be false") }
	if !DecodeBool(encoded, 40) { t.Error("ford should be true") }
	if !DecodeBool(encoded, 41) { t.Error("lit should be true") }
	if !DecodeBool(encoded, 56) { t.Error("segregated should be true") }
	if !DecodeBool(encoded, 57) { t.Error("bicycle_road should be true") }
}

func TestEncodeEnrichmentTags(t *testing.T) {
	tags := RawEnrichment{
		ForestPct: 75, NoiseScore: 30, TrafficClass: 2,
		ProtectedArea: 1, IsCycleroute: true, IsHikeroute: false,
		CyclerouteNetwork: "rcn", HikerouteNetwork: "rwn",
		LanduseClass: 4, // forest
	}
	encoded := EncodeEnrichmentTags(&tags)

	if got := DecodeField(encoded, 0, 7); got != 75 {
		t.Errorf("forest_pct: got %d, want 75", got)
	}
	if got := DecodeField(encoded, 7, 7); got != 30 {
		t.Errorf("noise_score: got %d, want 30", got)
	}
	if got := DecodeField(encoded, 14, 2); got != 2 {
		t.Errorf("traffic_class: got %d, want 2", got)
	}
	if got := DecodeField(encoded, 16, 2); got != 1 {
		t.Errorf("protected_area: got %d, want 1", got)
	}
	if !DecodeBool(encoded, 18) {
		t.Error("is_cycleroute should be true")
	}
	if DecodeBool(encoded, 19) {
		t.Error("is_hikeroute should be false")
	}
	if got := DecodeField(encoded, 20, 3); got != uint64(routeNetworkEnum["rcn"]) {
		t.Errorf("cycleroute_network: got %d, want %d", got, routeNetworkEnum["rcn"])
	}
	if got := DecodeField(encoded, 23, 3); got != uint64(routeNetworkEnum["rwn"]) {
		t.Errorf("hikeroute_network: got %d, want %d", got, routeNetworkEnum["rwn"])
	}
	if got := DecodeField(encoded, 26, 3); got != 4 {
		t.Errorf("landuse_class: got %d, want 4", got)
	}
}

func TestEnrichmentLocalNetwork(t *testing.T) {
	tags := RawEnrichment{
		CyclerouteNetwork: "lcn",
		HikerouteNetwork:  "lwn",
	}
	encoded := EncodeEnrichmentTags(&tags)
	if got := DecodeField(encoded, 20, 3); got != uint64(routeNetworkEnum["lcn"]) {
		t.Errorf("cycleroute_network lcn: got %d, want %d", got, routeNetworkEnum["lcn"])
	}
	if got := DecodeField(encoded, 23, 3); got != uint64(routeNetworkEnum["lwn"]) {
		t.Errorf("hikeroute_network lwn: got %d, want %d", got, routeNetworkEnum["lwn"])
	}
}

func TestNullEnrichmentDefaultsToZero(t *testing.T) {
	tags := RawEnrichment{} // all zero/false = NULL in DB
	encoded := EncodeEnrichmentTags(&tags)
	if encoded != 0 {
		t.Errorf("empty enrichment should encode to 0, got %d", encoded)
	}
}

func TestEncodeDecodeAllFields(t *testing.T) {
	tags := RawTags{
		Highway: "track", Surface: "gravel", Tracktype: "grade3",
		SacScale: "mountain_hiking", MtbScale: "2", Smoothness: "bad",
		Access: "permissive", Bicycle: "designated", Foot: "yes", Horse: "no",
		Cycleway: "lane", Maxspeed: "50", Lanes: 3,
		Oneway: true, OnewayBicycle: true, Bridge: false, Tunnel: true,
		Ford: false, Lit: true, Segregated: true, BicycleRoad: false,
	}
	encoded := EncodeOSMTags(&tags)

	if got := DecodeHighway(encoded); got != highwayEnum["track"] {
		t.Errorf("highway: got %d, want %d", got, highwayEnum["track"])
	}
	if got := DecodeSurface(encoded); got != surfaceEnum["gravel"] {
		t.Errorf("surface: got %d, want %d", got, surfaceEnum["gravel"])
	}
	if got := DecodeField(encoded, 11, 3); got != uint64(tracktypeEnum["grade3"]) {
		t.Errorf("tracktype: got %d, want %d", got, tracktypeEnum["grade3"])
	}
	if got := DecodeField(encoded, 14, 3); got != uint64(sacScaleEnum["mountain_hiking"]) {
		t.Errorf("sac_scale: got %d, want %d", got, sacScaleEnum["mountain_hiking"])
	}
	if got := DecodeField(encoded, 17, 3); got != uint64(mtbScaleEnum["2"]) {
		t.Errorf("mtb_scale: got %d, want %d", got, mtbScaleEnum["2"])
	}
	if got := DecodeField(encoded, 20, 4); got != uint64(smoothnessEnum["bad"]) {
		t.Errorf("smoothness: got %d, want %d", got, smoothnessEnum["bad"])
	}
	if got := DecodeField(encoded, 27, 3); got != uint64(accessEnum["designated"]) {
		t.Errorf("bicycle: got %d, want %d", got, accessEnum["designated"])
	}
	if got := DecodeField(encoded, 30, 3); got != uint64(accessEnum["yes"]) {
		t.Errorf("foot: got %d, want %d", got, accessEnum["yes"])
	}
	if got := DecodeField(encoded, 33, 3); got != uint64(accessEnum["no"]) {
		t.Errorf("horse: got %d, want %d", got, accessEnum["no"])
	}
	if got := DecodeField(encoded, 42, 3); got != uint64(cyclewayEnum["lane"]) {
		t.Errorf("cycleway: got %d, want %d", got, cyclewayEnum["lane"])
	}
	if got := DecodeField(encoded, 45, 8); got != 50 {
		t.Errorf("maxspeed: got %d, want 50", got)
	}
	if got := DecodeField(encoded, 53, 3); got != 3 {
		t.Errorf("lanes: got %d, want 3", got)
	}
	if !DecodeBool(encoded, 37) {
		t.Error("oneway_bicycle should be true")
	}
	if !DecodeBool(encoded, 39) {
		t.Error("tunnel should be true")
	}
}

func TestCyclewaySide(t *testing.T) {
	tests := []struct {
		left, right, both string
		want              uint8
	}{
		{"", "", "", 0},           // none
		{"lane", "", "", 1},       // left
		{"", "track", "", 2},      // right
		{"lane", "track", "", 3},  // both
		{"", "", "lane", 3},       // both (via cycleway_both)
	}
	for _, tt := range tests {
		tags := RawTags{CyclewayLeft: tt.left, CyclewayRight: tt.right, CyclewayBoth: tt.both}
		encoded := EncodeOSMTags(&tags)
		got := DecodeField(encoded, 58, 3)
		if got != uint64(tt.want) {
			t.Errorf("cycleway_side left=%q right=%q both=%q: got %d, want %d",
				tt.left, tt.right, tt.both, got, tt.want)
		}
	}
}
