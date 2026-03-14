package graph

// RawTags holds OSM tag strings as loaded from PostGIS before bit-packing.
type RawTags struct {
	Highway       string
	Surface       string
	Tracktype     string
	SacScale      string
	MtbScale      string
	Smoothness    string
	Access        string
	Bicycle       string
	Foot          string
	Horse         string
	Cycleway      string
	CyclewayLeft  string
	CyclewayRight string
	CyclewayBoth  string
	Maxspeed      string
	Lanes         int
	Oneway        bool
	OnewayBicycle bool
	Bridge        bool
	Tunnel        bool
	Ford          bool
	Lit           bool
	Segregated    bool
	BicycleRoad   bool
}

// RawEnrichment holds enrichment values from PostGIS nullable columns.
// Zero values represent NULL/unknown.
type RawEnrichment struct {
	ForestPct         int
	NoiseScore        int
	TrafficClass      int
	ProtectedArea     int
	IsCycleroute      bool
	IsHikeroute       bool
	CyclerouteNetwork string
	HikerouteNetwork  string
	LanduseClass      int
}

var highwayEnum = map[string]uint8{
	"motorway": 1, "motorway_link": 2, "trunk": 3, "trunk_link": 4,
	"primary": 5, "primary_link": 6, "secondary": 7, "secondary_link": 8,
	"tertiary": 9, "tertiary_link": 10, "residential": 11, "living_street": 12,
	"unclassified": 13, "service": 14, "track": 15, "path": 16,
	"footway": 17, "cycleway": 18, "bridleway": 19, "steps": 20,
	"pedestrian": 21, "via_ferrata": 22,
}

var surfaceEnum = map[string]uint8{
	"asphalt": 1, "concrete": 2, "paving_stones": 3, "sett": 4,
	"cobblestone": 5, "metal": 6, "wood": 7,
	"compacted": 8, "fine_gravel": 9, "gravel": 10, "pebblestone": 11,
	"dirt": 12, "earth": 13, "grass": 14, "grass_paver": 15,
	"ground": 16, "mud": 17, "sand": 18, "woodchips": 19,
	"snow": 20, "ice": 21, "rock": 22, "stone": 23,
	"unhewn_cobblestone": 24, "clay": 25, "tartan": 26, "artificial_turf": 27,
	"decoturf": 28, "carpet": 29, "rubber": 30, "acrylic": 31,
	"brick": 32, "chipseal": 33, "concrete_plates": 34, "concrete_lanes": 35,
	"metal_grid": 36, "stepping_stones": 37,
}

var tracktypeEnum = map[string]uint8{
	"grade1": 1, "grade2": 2, "grade3": 3, "grade4": 4, "grade5": 5,
}

var sacScaleEnum = map[string]uint8{
	"hiking": 1, "mountain_hiking": 2, "demanding_mountain_hiking": 3,
	"alpine_hiking": 4, "demanding_alpine_hiking": 5, "difficult_alpine_hiking": 6,
}

var mtbScaleEnum = map[string]uint8{
	"0": 1, "1": 2, "2": 3, "3": 4, "4": 5, "5": 6,
}

var smoothnessEnum = map[string]uint8{
	"excellent": 1, "good": 2, "intermediate": 3, "bad": 4,
	"very_bad": 5, "horrible": 6, "very_horrible": 7, "impassable": 8,
}

var accessEnum = map[string]uint8{
	"yes": 1, "no": 2, "private": 3, "permissive": 4,
	"destination": 5, "customers": 6, "designated": 7,
}

var cyclewayEnum = map[string]uint8{
	"lane": 1, "track": 2, "shared_lane": 3, "share_busway": 4,
	"opposite": 5, "opposite_lane": 6, "opposite_track": 7,
}

// routeNetworkEnum encodes route network hierarchy levels.
// Cycling and hiking networks are stored in separate 3-bit fields (bits 20-22
// and 23-25), so they use the same value range independently.
// Values: 1=international, 2=national, 3=regional, 4=local.
var routeNetworkEnum = map[string]uint8{
	"icn": 1, "ncn": 2, "rcn": 3, "lcn": 4,
	"iwn": 1, "nwn": 2, "rwn": 3, "lwn": 4,
}

// EncodeOSMTags packs RawTags into the first uint64.
//
// Bit layout (see spec section 4.4):
//
//	Bits  0- 4: highway (5), 5-10: surface (6), 11-13: tracktype (3),
//	14-16: sac_scale (3), 17-19: mtb_scale (3), 20-23: smoothness (4),
//	24-26: access (3), 27-29: bicycle (3), 30-32: foot (3), 33-35: horse (3),
//	36: oneway, 37: oneway_bicycle, 38: bridge, 39: tunnel, 40: ford, 41: lit,
//	42-44: cycleway (3), 45-52: maxspeed (8), 53-55: lanes (3),
//	56: segregated, 57: bicycle_road, 58-60: cycleway_side (3), 61-63: reserve
func EncodeOSMTags(t *RawTags) uint64 {
	var v uint64
	v |= uint64(highwayEnum[t.Highway])
	v |= uint64(surfaceEnum[t.Surface]) << 5
	v |= uint64(tracktypeEnum[t.Tracktype]) << 11
	v |= uint64(sacScaleEnum[t.SacScale]) << 14
	v |= uint64(mtbScaleEnum[t.MtbScale]) << 17
	v |= uint64(smoothnessEnum[t.Smoothness]) << 20
	v |= uint64(accessEnum[t.Access]) << 24
	v |= uint64(accessEnum[t.Bicycle]) << 27
	v |= uint64(accessEnum[t.Foot]) << 30
	v |= uint64(accessEnum[t.Horse]) << 33
	if t.Oneway {
		v |= 1 << 36
	}
	if t.OnewayBicycle {
		v |= 1 << 37
	}
	if t.Bridge {
		v |= 1 << 38
	}
	if t.Tunnel {
		v |= 1 << 39
	}
	if t.Ford {
		v |= 1 << 40
	}
	if t.Lit {
		v |= 1 << 41
	}
	v |= uint64(cyclewayEnum[t.Cycleway]) << 42
	v |= uint64(ParseMaxspeed(t.Maxspeed)) << 45
	lanes := min(t.Lanes, 7)
	if lanes < 0 {
		lanes = 0
	}
	v |= uint64(lanes) << 53
	if t.Segregated {
		v |= 1 << 56
	}
	if t.BicycleRoad {
		v |= 1 << 57
	}
	v |= uint64(encodeCyclewaySide(t)) << 58
	return v
}

func encodeCyclewaySide(t *RawTags) uint8 {
	if t.CyclewayBoth != "" {
		return 3 // both
	}
	hasLeft := t.CyclewayLeft != ""
	hasRight := t.CyclewayRight != ""
	switch {
	case hasLeft && hasRight:
		return 3 // both
	case hasLeft:
		return 1 // left
	case hasRight:
		return 2 // right
	default:
		return 0 // none
	}
}

// EncodeEnrichmentTags packs RawEnrichment into the second uint64.
//
// Bit layout (see spec section 4.4):
//
//	Bits 0-6: forest_pct (7), 7-13: noise_score (7), 14-15: traffic_class (2),
//	16-17: protected_area (2), 18: is_cycleroute, 19: is_hikeroute,
//	20-22: cycleroute_network (3), 23-25: hikeroute_network (3),
//	26-28: landuse_class (3), 29-63: reserve
func EncodeEnrichmentTags(e *RawEnrichment) uint64 {
	var v uint64
	v |= uint64(clamp(e.ForestPct, 0, 100))
	v |= uint64(clamp(e.NoiseScore, 0, 100)) << 7
	v |= uint64(clamp(e.TrafficClass, 0, 3)) << 14
	v |= uint64(clamp(e.ProtectedArea, 0, 3)) << 16
	if e.IsCycleroute {
		v |= 1 << 18
	}
	if e.IsHikeroute {
		v |= 1 << 19
	}
	v |= uint64(routeNetworkEnum[e.CyclerouteNetwork]) << 20
	v |= uint64(routeNetworkEnum[e.HikerouteNetwork]) << 23
	v |= uint64(clamp(e.LanduseClass, 0, 7)) << 26
	return v
}

// --- Decode helpers ---

// DecodeHighway extracts the highway enum (bits 0-4).
func DecodeHighway(v uint64) uint8 { return uint8(v & 0x1F) }

// DecodeSurface extracts the surface enum (bits 5-10).
func DecodeSurface(v uint64) uint8 { return uint8((v >> 5) & 0x3F) }

// DecodeAccess extracts the access enum (bits 24-26).
func DecodeAccess(v uint64) uint8 { return uint8((v >> 24) & 0x07) }

// DecodeBool extracts a single boolean bit at position pos.
func DecodeBool(v uint64, pos int) bool { return (v>>pos)&1 == 1 }

// DecodeField extracts a field of width bits starting at bit position start.
func DecodeField(v uint64, start, width int) uint64 {
	mask := uint64((1 << width) - 1)
	return (v >> start) & mask
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Reverse lookup maps (enum value → string).
var highwayNames = reverseMap(highwayEnum)
var surfaceNames = reverseMap(surfaceEnum)
var tracktypeNames = reverseMap(tracktypeEnum)
var sacScaleNames = reverseMap(sacScaleEnum)
var mtbScaleNames = reverseMap(mtbScaleEnum)

func reverseMap(m map[string]uint8) map[uint8]string {
	r := make(map[uint8]string, len(m))
	for k, v := range m {
		r[v] = k
	}
	return r
}

// HighwayName returns the string name for a highway enum value.
func HighwayName(v uint8) string { return highwayNames[v] }

// SurfaceName returns the string name for a surface enum value.
func SurfaceName(v uint8) string { return surfaceNames[v] }

// TrackTypeName returns the string name for a tracktype enum value.
func TrackTypeName(v uint8) string { return tracktypeNames[v] }

// SacScaleName returns the string name for a sac_scale enum value.
func SacScaleName(v uint8) string { return sacScaleNames[v] }

// MtbScaleName returns the string name for an mtb_scale enum value.
func MtbScaleName(v uint8) string { return mtbScaleNames[v] }
