package routing

import "encoding/json"

// IsT0FormCompliant reports whether a route satisfies the T0 elevation-profile
// layout requirements before being written to the database with
// route_schema_version = 1.
//
// A route is T0-form-compliant iff:
//
//  1. numSegments > 0 — the route has at least one segment.
//  2. rawProfile decodes to a non-empty []ElevationPoint — the merged route-level
//     profile is present and contains at least one point.
//  3. profile[0].DistM == 0 — the distance axis starts at 0 (approach-inclusive
//     per Spec §5.5 reversal; click-point is anchor, not projection-point).
//  4. DistM is strictly monotonically increasing across all profile points —
//     guarantees a valid 1-to-1 distance axis for the elevation chart.
//
// EleM values are NOT validated: null is legal for DEM no-data gaps (Spec §6
// Class A). Only the structural distance axis is checked.
//
// Returns false if rawProfile is nil, "null", or unparseable.
func IsT0FormCompliant(numSegments int, rawProfile json.RawMessage) bool {
	if numSegments == 0 {
		return false
	}
	if len(rawProfile) == 0 || string(rawProfile) == "null" {
		return false
	}
	var pts []ElevationPoint
	if err := json.Unmarshal(rawProfile, &pts); err != nil || len(pts) == 0 {
		return false
	}
	if pts[0].DistM != 0 {
		return false
	}
	for i := 1; i < len(pts); i++ {
		if pts[i].DistM <= pts[i-1].DistM {
			return false
		}
	}
	return true
}

// schemaVersionFor returns 1 if the route data is T0-form-compliant, 0 otherwise.
// Used by all save paths in the database layer to compute route_schema_version
// dynamically rather than hard-coding 1.
func schemaVersionFor(numSegments int, rawProfile json.RawMessage) int16 {
	if IsT0FormCompliant(numSegments, rawProfile) {
		return 1
	}
	return 0
}
