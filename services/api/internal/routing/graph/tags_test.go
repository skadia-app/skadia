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
	if !DecodeBool(encoded, 36) {
		t.Error("oneway should be true")
	}
	if DecodeBool(encoded, 37) {
		t.Error("oneway_bicycle should be false")
	}
	if !DecodeBool(encoded, 38) {
		t.Error("bridge should be true")
	}
	if DecodeBool(encoded, 39) {
		t.Error("tunnel should be false")
	}
	if !DecodeBool(encoded, 40) {
		t.Error("ford should be true")
	}
	if !DecodeBool(encoded, 41) {
		t.Error("lit should be true")
	}
	if !DecodeBool(encoded, 56) {
		t.Error("segregated should be true")
	}
	if !DecodeBool(encoded, 57) {
		t.Error("bicycle_road should be true")
	}
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
	if got := DecodeField(encoded, 14, 3); got != 2 {
		t.Errorf("traffic_class: got %d, want 2", got)
	}
	if got := DecodeField(encoded, 17, 2); got != 1 {
		t.Errorf("protected_area: got %d, want 1", got)
	}
	if !DecodeBool(encoded, 19) {
		t.Error("is_cycleroute should be true")
	}
	if DecodeBool(encoded, 20) {
		t.Error("is_hikeroute should be false")
	}
	if got := DecodeField(encoded, 21, 3); got != uint64(routeNetworkEnum["rcn"]) {
		t.Errorf("cycleroute_network: got %d, want %d", got, routeNetworkEnum["rcn"])
	}
	if got := DecodeField(encoded, 24, 3); got != uint64(routeNetworkEnum["rwn"]) {
		t.Errorf("hikeroute_network: got %d, want %d", got, routeNetworkEnum["rwn"])
	}
	if got := DecodeField(encoded, 27, 3); got != 4 {
		t.Errorf("landuse_class: got %d, want 4", got)
	}
}

func TestEnrichmentLocalNetwork(t *testing.T) {
	tags := RawEnrichment{
		CyclerouteNetwork: "lcn",
		HikerouteNetwork:  "lwn",
	}
	encoded := EncodeEnrichmentTags(&tags)
	if got := DecodeField(encoded, 21, 3); got != uint64(routeNetworkEnum["lcn"]) {
		t.Errorf("cycleroute_network lcn: got %d, want %d", got, routeNetworkEnum["lcn"])
	}
	if got := DecodeField(encoded, 24, 3); got != uint64(routeNetworkEnum["lwn"]) {
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

func TestEncodeEdgeTags2_ExtendedOSM(t *testing.T) {
	tags := RawTags{
		MtbScaleUphill:  "3",
		ViaFerrataScale: "K4",
		TrailVisibility: "bad",
		Width:           "2.5",
		Incline:         "15%",
		Sidewalk:        "both",
	}
	encoded := EncodeEdgeTags2(&tags, &RawEnrichment{})

	if got := DecodeField(encoded, 30, 3); got != uint64(mtbScaleUphillEnum["3"]) {
		t.Errorf("mtb_scale_uphill: got %d, want %d", got, mtbScaleUphillEnum["3"])
	}
	if got := DecodeField(encoded, 33, 3); got != uint64(viaFerrataScaleEnum["K4"]) {
		t.Errorf("via_ferrata_scale: got %d, want %d", got, viaFerrataScaleEnum["K4"])
	}
	// Lowercase via_ferrata_scale must also work
	tagsLower := RawTags{ViaFerrataScale: "k4"}
	encodedLower := EncodeEdgeTags2(&tagsLower, &RawEnrichment{})
	if got := DecodeField(encodedLower, 33, 3); got != uint64(viaFerrataScaleEnum["K4"]) {
		t.Errorf("via_ferrata_scale lowercase k4: got %d, want %d", got, viaFerrataScaleEnum["K4"])
	}
	if got := DecodeField(encoded, 36, 3); got != uint64(trailVisibilityEnum["bad"]) {
		t.Errorf("trail_visibility: got %d, want %d", got, trailVisibilityEnum["bad"])
	}
	if got := DecodeField(encoded, 39, 8); got != 25 { // 2.5m = 25dm
		t.Errorf("width: got %d, want 25", got)
	}
	if got := DecodeField(encoded, 47, 8); got != 143 { // 15% + 128 = 143
		t.Errorf("incline: got %d, want 143", got)
	}
	if got := DecodeField(encoded, 55, 3); got != uint64(sidewalkEnum["both"]) {
		t.Errorf("sidewalk: got %d, want %d", got, sidewalkEnum["both"])
	}
}

func TestParseWidth(t *testing.T) {
	tests := []struct {
		input string
		want  uint8
	}{
		{"2.5", 25},
		{"3", 30},
		{"0.8 m", 8},
		{"1.2m", 12},
		{"2,5", 25},   // European comma-decimal
		{"1,8 m", 18}, // European comma with unit
		{"", 0},
		{"5'", 15},    // 5 feet ≈ 1.524m → 15dm
		{"6ft", 18},   // 6 feet ≈ 1.829m → 18dm
		{"10 ft", 30}, // 10 feet ≈ 3.048m → 30dm
		{"narrow", 0},
		{"30", 255}, // clamped
	}
	for _, tt := range tests {
		got := ParseWidth(tt.input)
		if got != tt.want {
			t.Errorf("ParseWidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseIncline(t *testing.T) {
	tests := []struct {
		input string
		want  uint8
	}{
		{"15%", 143},
		{"-10%", 118},
		{"0%", 128},
		{"up", 138},
		{"down", 118},
		{"steep", 148},
		{"-steep", 108},
		{"flat", 128},
		{"", 0},
		{"unknown", 0},
		{"5", 133}, // bare number treated as percent
		{"-30%", 98},
		{"45°", 228},  // tan(45°)*100 = 100% → 128+100=228
		{"10°", 146},  // tan(10°)*100 ≈ 17.6% → 128+18=146
		{"-15°", 101}, // tan(-15°)*100 ≈ -26.8% → 128-27=101
	}
	for _, tt := range tests {
		got := ParseIncline(tt.input)
		if got != tt.want {
			t.Errorf("ParseIncline(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestNewFieldsDoNotAffectOldFields(t *testing.T) {
	// Verify OSM extended fields in bits 30-57 don't corrupt enrichment in bits 0-29.
	enrich := RawEnrichment{
		ForestPct: 75, NoiseScore: 30, TrafficClass: 2,
		ProtectedArea: 1, IsCycleroute: true, LanduseClass: 4,
	}
	emptyTags := RawTags{}
	fullTags := RawTags{
		MtbScaleUphill: "5", ViaFerrataScale: "K6",
		TrailVisibility: "no", Width: "3.0", Incline: "-20%", Sidewalk: "left",
	}
	oldEnc := EncodeEdgeTags2(&emptyTags, &enrich)
	combEnc := EncodeEdgeTags2(&fullTags, &enrich)

	// Lower 30 bits must be identical.
	mask := uint64((1 << 30) - 1)
	if oldEnc&mask != combEnc&mask {
		t.Errorf("OSM extended fields corrupted enrichment bits: old=%064b combined=%064b", oldEnc, combEnc)
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

// --- EdgeTags2 Decode-Helper Tests ---

func TestDecodeForestPct(t *testing.T) {
	for _, want := range []int{0, 1, 50, 75, 100} {
		e := &RawEnrichment{ForestPct: want}
		v := EncodeEnrichmentTags(e)
		got := DecodeForestPct(v)
		if int(got) != want {
			t.Errorf("ForestPct %d: got %d", want, got)
		}
	}
}

func TestDecodeNoiseScore(t *testing.T) {
	for _, want := range []int{0, 1, 30, 99, 100} {
		e := &RawEnrichment{NoiseScore: want}
		v := EncodeEnrichmentTags(e)
		got := DecodeNoiseScore(v)
		if int(got) != want {
			t.Errorf("NoiseScore %d: got %d", want, got)
		}
	}
}

func TestDecodeTrafficClass(t *testing.T) {
	for _, want := range []int{0, 1, 2, 3, 4, 5, 6} {
		e := &RawEnrichment{TrafficClass: want}
		v := EncodeEnrichmentTags(e)
		got := DecodeTrafficClass(v)
		if int(got) != want {
			t.Errorf("TrafficClass %d: got %d", want, got)
		}
	}
}

func TestDecodeProtectedArea(t *testing.T) {
	for _, want := range []int{0, 1, 2, 3} {
		e := &RawEnrichment{ProtectedArea: want}
		v := EncodeEnrichmentTags(e)
		got := DecodeProtectedArea(v)
		if int(got) != want {
			t.Errorf("ProtectedArea %d: got %d", want, got)
		}
	}
}

func TestDecodeIsCycleroute(t *testing.T) {
	if DecodeIsCycleroute(EncodeEnrichmentTags(&RawEnrichment{IsCycleroute: true})) != true {
		t.Error("IsCycleroute true: got false")
	}
	if DecodeIsCycleroute(EncodeEnrichmentTags(&RawEnrichment{IsCycleroute: false})) != false {
		t.Error("IsCycleroute false: got true")
	}
}

func TestDecodeIsHikeroute(t *testing.T) {
	if DecodeIsHikeroute(EncodeEnrichmentTags(&RawEnrichment{IsHikeroute: true})) != true {
		t.Error("IsHikeroute true: got false")
	}
	if DecodeIsHikeroute(EncodeEnrichmentTags(&RawEnrichment{IsHikeroute: false})) != false {
		t.Error("IsHikeroute false: got true")
	}
}

func TestDecodeCyclerouteNetwork(t *testing.T) {
	for _, name := range []string{"icn", "ncn", "rcn", "lcn"} {
		e := &RawEnrichment{CyclerouteNetwork: name}
		v := EncodeEnrichmentTags(e)
		got := DecodeCyclerouteNetwork(v)
		want := routeNetworkEnum[name]
		if got != want {
			t.Errorf("CyclerouteNetwork %q: got %d, want %d", name, got, want)
		}
	}
}

func TestDecodeHikerouteNetwork(t *testing.T) {
	for _, name := range []string{"iwn", "nwn", "rwn", "lwn"} {
		e := &RawEnrichment{HikerouteNetwork: name}
		v := EncodeEnrichmentTags(e)
		got := DecodeHikerouteNetwork(v)
		want := routeNetworkEnum[name]
		if got != want {
			t.Errorf("HikerouteNetwork %q: got %d, want %d", name, got, want)
		}
	}
}

func TestDecodeLanduseClass(t *testing.T) {
	for _, want := range []int{0, 1, 3, 7} {
		e := &RawEnrichment{LanduseClass: want}
		v := EncodeEnrichmentTags(e)
		got := DecodeLanduseClass(v)
		if int(got) != want {
			t.Errorf("LanduseClass %d: got %d", want, got)
		}
	}
}

func TestDecodeMtbScaleUphill(t *testing.T) {
	for _, name := range []string{"0", "1", "2", "3", "4", "5"} {
		tags := &RawTags{MtbScaleUphill: name}
		v := EncodeEdgeTags2(tags, &RawEnrichment{})
		got := DecodeMtbScaleUphill(v)
		want := mtbScaleUphillEnum[name]
		if got != want {
			t.Errorf("MtbScaleUphill %q: got %d, want %d", name, got, want)
		}
	}
}

func TestDecodeViaFerrataScale(t *testing.T) {
	for _, name := range []string{"K1", "K2", "K3", "K4", "K5", "K6"} {
		tags := &RawTags{ViaFerrataScale: name}
		v := EncodeEdgeTags2(tags, &RawEnrichment{})
		got := DecodeViaFerrataScale(v)
		want := viaFerrataScaleEnum[name]
		if got != want {
			t.Errorf("ViaFerrataScale %q: got %d, want %d", name, got, want)
		}
	}
}

func TestDecodeTrailVisibility(t *testing.T) {
	for _, name := range []string{"excellent", "good", "intermediate", "bad", "horrible", "no"} {
		tags := &RawTags{TrailVisibility: name}
		v := EncodeEdgeTags2(tags, &RawEnrichment{})
		got := DecodeTrailVisibility(v)
		want := trailVisibilityEnum[name]
		if got != want {
			t.Errorf("TrailVisibility %q: got %d, want %d", name, got, want)
		}
	}
}

func TestDecodeWidthDm(t *testing.T) {
	for _, input := range []string{"2.5", "3", "1.0"} {
		tags := &RawTags{Width: input}
		v := EncodeEdgeTags2(tags, &RawEnrichment{})
		got := DecodeWidthDm(v)
		want := ParseWidth(input)
		if got != want {
			t.Errorf("WidthDm %q: got %d, want %d", input, got, want)
		}
	}
}

func TestDecodeInclineOffset(t *testing.T) {
	for _, input := range []string{"15%", "-10%", "0%", "up", ""} {
		tags := &RawTags{Incline: input}
		v := EncodeEdgeTags2(tags, &RawEnrichment{})
		got := DecodeInclineOffset(v)
		want := ParseIncline(input)
		if got != want {
			t.Errorf("InclineOffset %q: got %d, want %d", input, got, want)
		}
	}
}

func TestDecodeSidewalk(t *testing.T) {
	for _, name := range []string{"yes", "both", "left", "right", "no"} {
		tags := &RawTags{Sidewalk: name}
		v := EncodeEdgeTags2(tags, &RawEnrichment{})
		got := DecodeSidewalk(v)
		want := sidewalkEnum[name]
		if got != want {
			t.Errorf("Sidewalk %q: got %d, want %d", name, got, want)
		}
	}
}

func TestDecodeAllEdgeTags2Combined(t *testing.T) {
	// All fields set simultaneously — no field should corrupt another.
	tags := &RawTags{
		MtbScaleUphill: "3", ViaFerrataScale: "K4",
		TrailVisibility: "bad", Width: "2.5", Incline: "15%", Sidewalk: "both",
	}
	enrich := &RawEnrichment{
		ForestPct: 75, NoiseScore: 30, TrafficClass: 2,
		ProtectedArea: 1, IsCycleroute: true, IsHikeroute: true,
		CyclerouteNetwork: "rcn", HikerouteNetwork: "rwn", LanduseClass: 4,
	}
	v := EncodeEdgeTags2(tags, enrich)

	if got := DecodeForestPct(v); got != 75 {
		t.Errorf("ForestPct: got %d, want 75", got)
	}
	if got := DecodeNoiseScore(v); got != 30 {
		t.Errorf("NoiseScore: got %d, want 30", got)
	}
	if got := DecodeTrafficClass(v); got != 2 {
		t.Errorf("TrafficClass: got %d, want 2", got)
	}
	if got := DecodeProtectedArea(v); got != 1 {
		t.Errorf("ProtectedArea: got %d, want 1", got)
	}
	if !DecodeIsCycleroute(v) {
		t.Error("IsCycleroute: got false, want true")
	}
	if !DecodeIsHikeroute(v) {
		t.Error("IsHikeroute: got false, want true")
	}
	if got := DecodeCyclerouteNetwork(v); got != routeNetworkEnum["rcn"] {
		t.Errorf("CyclerouteNetwork: got %d, want %d", got, routeNetworkEnum["rcn"])
	}
	if got := DecodeHikerouteNetwork(v); got != routeNetworkEnum["rwn"] {
		t.Errorf("HikerouteNetwork: got %d, want %d", got, routeNetworkEnum["rwn"])
	}
	if got := DecodeLanduseClass(v); got != 4 {
		t.Errorf("LanduseClass: got %d, want 4", got)
	}
	if got := DecodeMtbScaleUphill(v); got != mtbScaleUphillEnum["3"] {
		t.Errorf("MtbScaleUphill: got %d, want %d", got, mtbScaleUphillEnum["3"])
	}
	if got := DecodeViaFerrataScale(v); got != viaFerrataScaleEnum["K4"] {
		t.Errorf("ViaFerrataScale: got %d, want %d", got, viaFerrataScaleEnum["K4"])
	}
	if got := DecodeTrailVisibility(v); got != trailVisibilityEnum["bad"] {
		t.Errorf("TrailVisibility: got %d, want %d", got, trailVisibilityEnum["bad"])
	}
	if got := DecodeWidthDm(v); got != ParseWidth("2.5") {
		t.Errorf("WidthDm: got %d, want %d", got, ParseWidth("2.5"))
	}
	if got := DecodeInclineOffset(v); got != ParseIncline("15%") {
		t.Errorf("InclineOffset: got %d, want %d", got, ParseIncline("15%"))
	}
	if got := DecodeSidewalk(v); got != sidewalkEnum["both"] {
		t.Errorf("Sidewalk: got %d, want %d", got, sidewalkEnum["both"])
	}
}

// --- IsReverse (Bit 61 in EdgeTags1) Tests ---

func TestDecodeIsReverse(t *testing.T) {
	// Bit 61 not set
	var v uint64
	if DecodeIsReverse(v) {
		t.Error("IsReverse: expected false for zero value")
	}
	// Bit 61 set
	v |= 1 << 61
	if !DecodeIsReverse(v) {
		t.Error("IsReverse: expected true when bit 61 set")
	}
	// Bit 61 set alongside other bits — should not interfere
	v = ^uint64(0) // all bits set
	if !DecodeIsReverse(v) {
		t.Error("IsReverse: expected true when all bits set")
	}
	// Only bit 60 set — not bit 61
	v = 1 << 60
	if DecodeIsReverse(v) {
		t.Error("IsReverse: expected false when only bit 60 set")
	}
	// Only bit 62 set — not bit 61
	v = 1 << 62
	if DecodeIsReverse(v) {
		t.Error("IsReverse: expected false when only bit 62 set")
	}
}

func TestNegateInclineStr(t *testing.T) {
	tests := []struct{ in, want string }{
		{"15%", "-15%"},
		{"-10%", "10%"},
		{"up", "down"},
		{"down", "up"},
		{"steep", "-steep"},
		{"-steep", "steep"},
		{"flat", "flat"},
		{"", ""},
		{"5°", "-5°"},
		{"-12°", "12°"},
		{"3", "-3"},
		{"-7", "7"},
	}
	for _, tt := range tests {
		got := NegateInclineStr(tt.in)
		if got != tt.want {
			t.Errorf("NegateInclineStr(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsReverseDoesNotAffectOSMTags(t *testing.T) {
	// Bit 61 is reserve in EdgeTags1 — setting it must not corrupt other fields.
	tags := &RawTags{
		Highway: "track", Surface: "gravel", Oneway: true, Bridge: true,
	}
	tags1 := EncodeOSMTags(tags)
	tags1WithReverse := tags1 | (1 << 61)

	if DecodeHighway(tags1WithReverse) != DecodeHighway(tags1) {
		t.Error("IsReverse bit corrupted Highway field")
	}
	if DecodeSurface(tags1WithReverse) != DecodeSurface(tags1) {
		t.Error("IsReverse bit corrupted Surface field")
	}
	if !DecodeBool(tags1WithReverse, 36) {
		t.Error("IsReverse bit corrupted Oneway flag")
	}
	if !DecodeBool(tags1WithReverse, 38) {
		t.Error("IsReverse bit corrupted Bridge flag")
	}
	if !DecodeIsReverse(tags1WithReverse) {
		t.Error("IsReverse bit not readable after setting")
	}
}

func TestCyclewaySide(t *testing.T) {
	tests := []struct {
		left, right, both string
		want              uint8
	}{
		{"", "", "", 0},          // none
		{"lane", "", "", 1},      // left
		{"", "track", "", 2},     // right
		{"lane", "track", "", 3}, // both
		{"", "", "lane", 3},      // both (via cycleway_both)
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

func TestEdgeTags2AllBitsUsed(t *testing.T) {
	// All fields at max values should not overflow uint64.
	// Bit count: 7+7+3+2+1+1+3+3+3+3+3+3+8+8+3+3+3 = 64.
	tags := &RawTags{
		MtbScaleUphill: "5", ViaFerrataScale: "K6",
		TrailVisibility: "no", Width: "25.5", Incline: "100%", Sidewalk: "no",
	}
	enrich := &RawEnrichment{
		ForestPct: 100, NoiseScore: 100, TrafficClass: 6,
		ProtectedArea: 3, IsCycleroute: true, IsHikeroute: true,
		CyclerouteNetwork: "icn", HikerouteNetwork: "iwn", LanduseClass: 7,
		Urbanization: 255, RiverProximity: 255,
	}
	v := EncodeEdgeTags2(tags, enrich)

	// Verify all fields decode correctly even when all bits are set
	if got := DecodeForestPct(v); got != 100 {
		t.Errorf("ForestPct: got %d", got)
	}
	if got := DecodeNoiseScore(v); got != 100 {
		t.Errorf("NoiseScore: got %d", got)
	}
	if got := DecodeTrafficClass(v); got != 6 {
		t.Errorf("TrafficClass: got %d, want 6", got)
	}
	if got := DecodeProtectedArea(v); got != 3 {
		t.Errorf("ProtectedArea: got %d", got)
	}
	if !DecodeIsCycleroute(v) {
		t.Error("IsCycleroute should be true")
	}
	if !DecodeIsHikeroute(v) {
		t.Error("IsHikeroute should be true")
	}
	if got := DecodeUrbanization(v); got != 7 {
		t.Errorf("Urbanization: got %d, want 7", got)
	}
	if got := DecodeRiverProximity(v); got != 7 {
		t.Errorf("RiverProximity: got %d, want 7", got)
	}
}
