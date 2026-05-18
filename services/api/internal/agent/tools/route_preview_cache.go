package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const routePreviewTTL = 30 * time.Minute

type routePreviewSnapshot struct {
	UserID          string
	Token           string
	Hash            string
	GeometryGeoJSON string
	ProfileKey      string
	BaseProfile     string
	ProfileVersion  string
	Metadata        routePreviewMetadata
	CreatedAt       time.Time
}

var routePreviewCache sync.Map

type routePreviewMetadata struct {
	SubSegments      json.RawMessage
	ElevationProfile json.RawMessage
	Enrichment       json.RawMessage
	RoutingEngine    string
}

func attachRoutePreviewToken(userID string, result map[string]any, profileKey, baseProfile, profileVersion string) {
	geometry, err := routeGeometryGeoJSON(result)
	if err != nil {
		return
	}
	metadata := routePreviewMetadataFromResult(result)
	hash := routePreviewHash(userID, geometry, profileKey, baseProfile, profileVersion)
	token := uuid.NewString()
	routePreviewCache.Store(token, routePreviewSnapshot{
		UserID:          userID,
		Token:           token,
		Hash:            hash,
		GeometryGeoJSON: geometry,
		ProfileKey:      profileKey,
		BaseProfile:     baseProfile,
		ProfileVersion:  profileVersion,
		Metadata:        metadata,
		CreatedAt:       time.Now(),
	})
	result["route_preview_token"] = token
	result["route_preview_hash"] = hash
	result["geometry_geojson"] = geometry
	result["route_profile_version"] = profileVersion
}

func verifyRoutePreviewToken(userID, token, geometry, profile string) (*routePreviewSnapshot, error) {
	raw, ok := routePreviewCache.Load(token)
	if !ok {
		return nil, fmt.Errorf("route preview token not found or expired")
	}
	snapshot, ok := raw.(routePreviewSnapshot)
	if !ok || snapshot.UserID != userID {
		return nil, fmt.Errorf("route preview token not found or expired")
	}
	if time.Since(snapshot.CreatedAt) > routePreviewTTL {
		routePreviewCache.Delete(token)
		return nil, fmt.Errorf("route preview token expired")
	}
	normalizedGeometry, err := normalizeRouteGeometryGeoJSON(geometry)
	if err != nil {
		return nil, err
	}
	if snapshot.GeometryGeoJSON != normalizedGeometry {
		return nil, fmt.Errorf("route geometry does not match preview")
	}
	if snapshot.ProfileKey != profile && snapshot.BaseProfile != profile {
		return nil, fmt.Errorf("route profile does not match preview")
	}
	return &snapshot, nil
}

func consumeRoutePreviewToken(userID, token, geometry, profile string) (*routePreviewSnapshot, error) {
	raw, ok := routePreviewCache.LoadAndDelete(token)
	if !ok {
		return nil, fmt.Errorf("route preview token not found or expired")
	}
	snapshot, ok := raw.(routePreviewSnapshot)
	if !ok || snapshot.UserID != userID {
		return nil, fmt.Errorf("route preview token not found or expired")
	}
	if time.Since(snapshot.CreatedAt) > routePreviewTTL {
		return nil, fmt.Errorf("route preview token expired")
	}
	normalizedGeometry, err := normalizeRouteGeometryGeoJSON(geometry)
	if err != nil {
		return nil, err
	}
	if snapshot.GeometryGeoJSON != normalizedGeometry {
		return nil, fmt.Errorf("route geometry does not match preview")
	}
	if snapshot.ProfileKey != profile && snapshot.BaseProfile != profile {
		return nil, fmt.Errorf("route profile does not match preview")
	}
	return &snapshot, nil
}

func verifyRoutePreviewSaveArgs(userID string, args savePlannedRouteArgs) (*routePreviewSnapshot, error) {
	snapshot, err := verifyRoutePreviewToken(userID, args.PreviewToken, args.GeometryGeoJSON, args.Profile)
	if err != nil {
		return nil, err
	}
	if err := snapshot.Metadata.validateSaveArgs(args); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func consumeRoutePreviewSaveArgs(userID string, args savePlannedRouteArgs) (*routePreviewSnapshot, error) {
	if _, err := verifyRoutePreviewSaveArgs(userID, args); err != nil {
		return nil, err
	}
	snapshot, err := consumeRoutePreviewToken(userID, args.PreviewToken, args.GeometryGeoJSON, args.Profile)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func routePreviewHash(userID, geometry, profileKey, baseProfile, profileVersion string) string {
	sum := sha256.Sum256([]byte(userID + "\x00" + geometry + "\x00" + profileKey + "\x00" + baseProfile + "\x00" + profileVersion))
	return hex.EncodeToString(sum[:])
}

func routeGeometryGeoJSON(result map[string]any) (string, error) {
	if result == nil {
		return "", fmt.Errorf("route preview result missing")
	}
	if raw, ok := result["geometry_geojson"].(string); ok && raw != "" {
		return normalizeRouteGeometryGeoJSON(raw)
	}
	if geometry, ok := result["geometry"]; ok {
		raw, err := json.Marshal(geometry)
		if err != nil {
			return "", err
		}
		return normalizeRouteGeometryGeoJSON(string(raw))
	}
	features, ok := result["features"].([]any)
	if !ok || len(features) == 0 {
		return "", fmt.Errorf("route preview geometry missing")
	}
	feature, ok := features[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("route preview feature invalid")
	}
	geometry, ok := feature["geometry"]
	if !ok {
		return "", fmt.Errorf("route preview geometry missing")
	}
	raw, err := json.Marshal(geometry)
	if err != nil {
		return "", err
	}
	return normalizeRouteGeometryGeoJSON(string(raw))
}

func normalizeRouteGeometryGeoJSON(raw string) (string, error) {
	var doc struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", fmt.Errorf("invalid route geometry GeoJSON: %w", err)
	}
	switch doc.Type {
	case "LineString":
		var coords [][]float64
		if err := json.Unmarshal(doc.Coordinates, &coords); err != nil {
			return "", fmt.Errorf("invalid LineString coordinates: %w", err)
		}
		line, err := routeLine2D(coords)
		if err != nil {
			return "", err
		}
		out := struct {
			Type        string      `json:"type"`
			Coordinates [][]float64 `json:"coordinates"`
		}{Type: "LineString", Coordinates: line}
		encoded, err := json.Marshal(out)
		if err != nil {
			return "", err
		}
		if err := validateRouteGeometry(string(encoded)); err != nil {
			return "", err
		}
		return string(encoded), nil
	case "MultiLineString":
		var coords [][][]float64
		if err := json.Unmarshal(doc.Coordinates, &coords); err != nil {
			return "", fmt.Errorf("invalid MultiLineString coordinates: %w", err)
		}
		lines := make([][][]float64, 0, len(coords))
		for _, line := range coords {
			line2D, err := routeLine2D(line)
			if err != nil {
				return "", err
			}
			lines = append(lines, line2D)
		}
		out := struct {
			Type        string        `json:"type"`
			Coordinates [][][]float64 `json:"coordinates"`
		}{Type: "MultiLineString", Coordinates: lines}
		encoded, err := json.Marshal(out)
		if err != nil {
			return "", err
		}
		if err := validateRouteGeometry(string(encoded)); err != nil {
			return "", err
		}
		return string(encoded), nil
	default:
		return "", fmt.Errorf("route geometry must be LineString or MultiLineString")
	}
}

func routeLine2D(coords [][]float64) ([][]float64, error) {
	line := make([][]float64, 0, len(coords))
	for i, coord := range coords {
		if len(coord) < 2 {
			return nil, fmt.Errorf("route coordinate %d must be [lng,lat]", i)
		}
		line = append(line, []float64{coord[0], coord[1]})
	}
	return line, nil
}

func routePreviewMetadataFromResult(result map[string]any) routePreviewMetadata {
	meta := routePreviewMetadata{}
	if raw, ok := result["routing_engine"].(string); ok {
		meta.RoutingEngine = raw
	}
	props := firstRoutePreviewProperties(result)
	if raw, ok, _ := canonicalJSONFromAny(props["segments"]); ok {
		meta.SubSegments = raw
	}
	if raw, ok, _ := canonicalJSONFromAny(result["elevation_profile"]); ok {
		meta.ElevationProfile = raw
	} else if raw, ok, _ := canonicalJSONFromAny(props["elevation_profile"]); ok {
		meta.ElevationProfile = raw
	}
	if raw, ok, _ := routePreviewEnrichmentFromProperties(props); ok {
		meta.Enrichment = raw
	} else if raw, ok, _ := canonicalJSONFromAny(result["enrichment"]); ok {
		meta.Enrichment = raw
	}
	return meta
}

func firstRoutePreviewProperties(result map[string]any) map[string]any {
	features, ok := result["features"].([]any)
	if !ok || len(features) == 0 {
		return nil
	}
	feature, ok := features[0].(map[string]any)
	if !ok {
		return nil
	}
	props, _ := feature["properties"].(map[string]any)
	return props
}

func routePreviewEnrichmentFromProperties(props map[string]any) (json.RawMessage, bool, error) {
	if props == nil {
		return nil, false, nil
	}
	enrichment := map[string]any{"enrichment_version": 1}
	for _, key := range []string{
		"forest_pct",
		"protected_pct",
		"forest_ranges",
		"protected_ranges",
		"sac_ranges",
		"mtb_ranges",
		"vf_ranges",
		"vf_unknown_ranges",
		"summits_passed",
	} {
		if value, exists := props[key]; exists {
			enrichment[key] = value
		}
	}
	if len(enrichment) == 1 {
		return nil, false, nil
	}
	return canonicalJSONFromAny(enrichment)
}

func (meta routePreviewMetadata) validateSaveArgs(args savePlannedRouteArgs) error {
	if raw, ok, err := routePreviewSubSegmentsFromSaveArgs(args); err != nil {
		return err
	} else if ok && !jsonRawEqual(raw, meta.SubSegments) {
		return fmt.Errorf("route metadata does not match preview")
	}
	if raw, ok, err := canonicalJSONFromRaw(args.ElevationProfile); err != nil {
		return err
	} else if ok && !jsonRawEqual(raw, meta.ElevationProfile) {
		return fmt.Errorf("route metadata does not match preview")
	}
	if raw, ok, err := canonicalJSONFromRaw(args.Enrichment); err != nil {
		return err
	} else if ok && !jsonRawEqual(raw, meta.Enrichment) {
		return fmt.Errorf("route metadata does not match preview")
	}
	if args.RoutingEngine != nil && meta.RoutingEngine != "" && *args.RoutingEngine != meta.RoutingEngine {
		return fmt.Errorf("route metadata does not match preview")
	}
	return nil
}

func routePreviewSubSegmentsFromSaveArgs(args savePlannedRouteArgs) (json.RawMessage, bool, error) {
	raw, ok, err := canonicalJSONFromRaw(args.Segments)
	if err != nil || !ok {
		return nil, ok, err
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, false, err
	}
	if len(items) == 0 {
		return nil, false, nil
	}
	if _, direct := items[0]["from_idx"]; direct {
		return raw, true, nil
	}
	var subSegments []any
	for _, item := range items {
		segRaw, exists := item["sub_segments"]
		if !exists {
			continue
		}
		var segs []any
		if err := json.Unmarshal(segRaw, &segs); err != nil {
			return nil, false, err
		}
		subSegments = append(subSegments, segs...)
	}
	return canonicalJSONFromAny(subSegments)
}

func canonicalJSONFromRaw(raw json.RawMessage) (json.RawMessage, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false, err
	}
	return canonicalJSONFromAny(value)
}

func canonicalJSONFromAny(value any) (json.RawMessage, bool, error) {
	if isEmptyJSONValue(value) {
		return nil, false, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, false, err
	}
	if isEmptyJSONValue(normalized) {
		return nil, false, nil
	}
	raw, err = json.Marshal(normalized)
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func isEmptyJSONValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	default:
		return false
	}
}

func jsonRawEqual(a, b json.RawMessage) bool {
	aa, okA, errA := canonicalJSONFromRaw(a)
	bb, okB, errB := canonicalJSONFromRaw(b)
	if errA != nil || errB != nil || okA != okB {
		return false
	}
	if !okA {
		return true
	}
	return string(aa) == string(bb)
}
