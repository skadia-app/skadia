package brouterenrich

import (
	"math"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

const coordMatchToleranceDeg = 0.00003

type Coord struct {
	Lon, Lat float64
	Ele      float64
	EleOK    bool
}

func NormalizeWayTagProfileParams(params map[string]string) map[string]string {
	if params == nil {
		params = map[string]string{}
	}
	if _, exists := params["processUnusedTags"]; !exists {
		params["processUnusedTags"] = "1"
	}
	switch strings.ToLower(params["processUnusedTags"]) {
	case "true":
		params["processUnusedTags"] = "1"
	case "false":
		params["processUnusedTags"] = "0"
	}
	return params
}

func InjectWayTagEnrichment(gj map[string]any) {
	features, ok := gj["features"].([]any)
	if !ok {
		return
	}
	for _, f := range features {
		feat, ok := f.(map[string]any)
		if !ok {
			continue
		}
		props, ok := feat["properties"].(map[string]any)
		if !ok {
			continue
		}
		coords := ExtractCoordinates(feat)
		if len(coords) < 2 {
			continue
		}
		enriched, ranges := buildSegmentsFromMessages(props["messages"], coords)
		if len(enriched) == 0 {
			continue
		}
		props["segments"] = enriched
		if len(ranges.Sac) > 0 {
			props["sac_ranges"] = ranges.Sac
		}
		if len(ranges.Mtb) > 0 {
			props["mtb_ranges"] = ranges.Mtb
		}
		if len(ranges.Vf) > 0 {
			props["vf_ranges"] = ranges.Vf
		}
		if len(ranges.VfUnknown) > 0 {
			props["vf_unknown_ranges"] = ranges.VfUnknown
		}
	}
}

func ExtractCoordinates(feat map[string]any) []Coord {
	geomRaw, ok := feat["geometry"].(map[string]any)
	if !ok {
		return nil
	}
	coordsRaw, ok := geomRaw["coordinates"].([]any)
	if !ok {
		return nil
	}
	out := make([]Coord, 0, len(coordsRaw))
	for _, c := range coordsRaw {
		arr, ok := c.([]any)
		if !ok || len(arr) < 2 {
			continue
		}
		lon, lonOK := arr[0].(float64)
		lat, latOK := arr[1].(float64)
		if !lonOK || !latOK {
			continue
		}
		bc := Coord{Lon: lon, Lat: lat}
		if len(arr) >= 3 {
			if z, ok := arr[2].(float64); ok {
				bc.Ele = z
				bc.EleOK = true
			}
		}
		out = append(out, bc)
	}
	return out
}

func buildSegmentsFromMessages(rawMessages any, coords []Coord) ([]map[string]any, edgetags.AllRanges) {
	messages, ok := rawMessages.([]any)
	if !ok || len(messages) < 2 {
		return nil, edgetags.AllRanges{}
	}
	header, ok := messageRow(messages[0])
	if !ok {
		return nil, edgetags.AllRanges{}
	}
	lonCol, latCol, distCol, tagsCol := -1, -1, -1, -1
	for i, col := range header {
		switch col {
		case "Longitude":
			lonCol = i
		case "Latitude":
			latCol = i
		case "Distance":
			distCol = i
		case "WayTags":
			tagsCol = i
		}
	}
	if lonCol < 0 || latCol < 0 || distCol < 0 || tagsCol < 0 {
		return nil, edgetags.AllRanges{}
	}

	var segments []map[string]any
	ranges := edgetags.AllRanges{}
	prevIdx := 0
	prevDistM := 0.0
	for _, rawRow := range messages[1:] {
		row, ok := messageRow(rawRow)
		if !ok || len(row) <= max(lonCol, latCol, distCol, tagsCol) {
			continue
		}
		lon, ok := parseMessageCoord(row[lonCol], true)
		if !ok {
			continue
		}
		lat, ok := parseMessageCoord(row[latCol], false)
		if !ok {
			continue
		}
		endIdx, ok := findCoordIndex(coords, lon, lat, prevIdx)
		if !ok || endIdx <= prevIdx {
			if len(segments) == 0 {
				return nil, edgetags.AllRanges{}
			}
			break
		}
		segmentDistM, ok := parseMessageFloat(row[distCol])
		if !ok || segmentDistM < 0 {
			segmentDistM = 0
		}
		toDistM := prevDistM + segmentDistM
		rawTags := parseWayTags(row[tagsCol])
		tags1 := graph.EncodeOSMTags(rawTags)
		tags2 := graph.EncodeEdgeTags2(rawTags, &graph.RawEnrichment{})
		segment := segmentMap(prevIdx, endIdx, prevDistM, toDistM, tags1, tags2)
		segments = append(segments, segment)
		addScaleRanges(&ranges, prevIdx, endIdx, tags1, tags2)
		prevIdx = endIdx
		prevDistM = toDistM
	}
	if len(segments) == 0 {
		return nil, edgetags.AllRanges{}
	}
	return segments, ranges
}

func messageRow(raw any) ([]string, bool) {
	cells, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, len(cells))
	for i, cell := range cells {
		switch v := cell.(type) {
		case string:
			out[i] = v
		case float64:
			out[i] = strconv.FormatFloat(v, 'f', -1, 64)
		default:
			return nil, false
		}
	}
	return out, true
}

func parseMessageCoord(s string, lon bool) (float64, bool) {
	v, ok := parseMessageFloat(s)
	if !ok {
		return 0, false
	}
	limit := 90.0
	if lon {
		limit = 180.0
	}
	if math.Abs(v) > limit {
		v = v / 1_000_000
	}
	if math.Abs(v) > limit {
		return 0, false
	}
	return v, true
}

func parseMessageFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func findCoordIndex(coords []Coord, lon, lat float64, startIdx int) (int, bool) {
	for i := max(startIdx+1, 0); i < len(coords); i++ {
		if math.Abs(coords[i].Lon-lon) <= coordMatchToleranceDeg &&
			math.Abs(coords[i].Lat-lat) <= coordMatchToleranceDeg {
			return i, true
		}
	}
	return 0, false
}

func parseWayTags(s string) *graph.RawTags {
	tags := map[string]string{}
	for _, field := range strings.Fields(s) {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" || value == "" {
			continue
		}
		tags[key] = value
	}
	return &graph.RawTags{
		Highway:         tags["highway"],
		Surface:         tags["surface"],
		Tracktype:       tags["tracktype"],
		SacScale:        tags["sac_scale"],
		MtbScale:        tags["mtb:scale"],
		MtbScaleUphill:  tags["mtb:scale:uphill"],
		ViaFerrataScale: tags["via_ferrata_scale"],
		TrailVisibility: tags["trail_visibility"],
		Smoothness:      tags["smoothness"],
		Access:          tags["access"],
		Bicycle:         tags["bicycle"],
		Foot:            tags["foot"],
		Horse:           tags["horse"],
		Cycleway:        tags["cycleway"],
		CyclewayLeft:    tags["cycleway:left"],
		CyclewayRight:   tags["cycleway:right"],
		CyclewayBoth:    tags["cycleway:both"],
		Maxspeed:        tags["maxspeed"],
		Width:           tags["width"],
		Incline:         tags["incline"],
		Sidewalk:        tags["sidewalk"],
		Oneway:          tags["oneway"] == "yes",
		OnewayBicycle:   tags["oneway:bicycle"] == "yes",
		OnewayReverse:   tags["oneway"] == "-1",
		Bridge:          truthyOSMTag(tags["bridge"]),
		Tunnel:          truthyOSMTag(tags["tunnel"]),
		Ford:            truthyOSMTag(tags["ford"]),
		Lit:             truthyOSMTag(tags["lit"]),
		Segregated:      truthyOSMTag(tags["segregated"]),
		BicycleRoad:     truthyOSMTag(tags["bicycle_road"]),
	}
}

func truthyOSMTag(v string) bool {
	return v == "yes" || v == "true" || v == "1"
}

func segmentMap(fromIdx, toIdx int, fromDistM, toDistM float64, tags1, tags2 uint64) map[string]any {
	seg := map[string]any{
		"from_idx":    fromIdx,
		"to_idx":      toIdx,
		"from_dist_m": fromDistM,
		"to_dist_m":   toDistM,
		"highway":     graph.HighwayName(graph.DecodeHighway(tags1)),
		"surface":     graph.SurfaceName(graph.DecodeSurface(tags1)),
		"osm_way_id":  int64(0),
	}
	if tracktype := graph.TrackTypeName(uint8(graph.DecodeField(tags1, 11, 3))); tracktype != "" {
		seg["tracktype"] = tracktype
	}
	if sac := graph.SacScaleShort(graph.DecodeSacScale(tags1)); sac != "" {
		seg["sac_scale"] = sac
	}
	if mtb := graph.MtbScaleShort(graph.DecodeMtbScale(tags1)); mtb != "" {
		seg["mtb_scale"] = mtb
	}
	if mtbUp := graph.MtbScaleShort(graph.DecodeMtbScaleUphill(tags2)); mtbUp != "" {
		seg["mtb_scale_uphill"] = mtbUp
	}
	if vf := graph.ViaFerrataScaleShort(graph.DecodeViaFerrataScale(tags2)); vf != "" {
		seg["via_ferrata_scale"] = vf
	}
	if tv := graph.TrailVisibilityName(graph.DecodeTrailVisibility(tags2)); tv != "" {
		seg["trail_visibility"] = tv
	}
	return seg
}

func addScaleRanges(ranges *edgetags.AllRanges, fromIdx, toIdx int, tags1, tags2 uint64) {
	toExclusive := toIdx + 1
	if sac := graph.SacScaleShort(graph.DecodeSacScale(tags1)); sac != "" {
		if ranges.Sac == nil {
			ranges.Sac = map[string][]edgetags.CoordRange{}
		}
		appendCoordRange(ranges.Sac, sac, fromIdx, toExclusive)
	}
	if mtb := graph.MtbScaleShort(graph.DecodeMtbScale(tags1)); mtb != "" {
		if ranges.Mtb == nil {
			ranges.Mtb = map[string][]edgetags.CoordRange{}
		}
		appendCoordRange(ranges.Mtb, mtb, fromIdx, toExclusive)
	}
	if vf := graph.ViaFerrataScaleShort(graph.DecodeViaFerrataScale(tags2)); vf != "" {
		if ranges.Vf == nil {
			ranges.Vf = map[string][]edgetags.CoordRange{}
		}
		appendCoordRange(ranges.Vf, vf, fromIdx, toExclusive)
	} else if graph.DecodeHighway(tags1) == graph.HighwayViaFerrata {
		ranges.VfUnknown = appendFlatCoordRange(ranges.VfUnknown, fromIdx, toExclusive)
	}
}

func appendCoordRange(dst map[string][]edgetags.CoordRange, key string, from, to int) {
	dst[key] = appendFlatCoordRange(dst[key], from, to)
}

func appendFlatCoordRange(dst []edgetags.CoordRange, from, to int) []edgetags.CoordRange {
	if to <= from+1 {
		return dst
	}
	if len(dst) > 0 && dst[len(dst)-1].To == from+1 {
		dst[len(dst)-1].To = to
		return dst
	}
	return append(dst, edgetags.CoordRange{From: from, To: to})
}
