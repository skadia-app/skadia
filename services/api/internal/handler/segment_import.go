package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

type checkpointImportFeatureCollection struct {
	Type     string                    `json:"type"`
	Features []checkpointImportFeature `json:"features"`
}

type checkpointImportFeature struct {
	Type       string          `json:"type"`
	Properties map[string]any  `json:"properties"`
	Geometry   json.RawMessage `json:"geometry"`
}

// ImportSegment handles POST /api/v1/segments/import for exported checkpoint courses.
func (h *Handler) ImportSegment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	var payload checkpointImportFeatureCollection
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}
	create, err := checkpointCourseCreateFromImport(payload)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	segment, err := h.DB.CreateCheckpointCourse(r.Context(), userID, create)
	if err != nil {
		if errors.Is(err, database.ErrSegmentValidation) {
			httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", err.Error(), r.URL.Path)
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to import segment", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, segment)
}

func checkpointCourseCreateFromImport(payload checkpointImportFeatureCollection) (*model.CheckpointCourseCreate, error) {
	if payload.Type != "FeatureCollection" {
		return nil, fmt.Errorf("checkpoint import requires a FeatureCollection")
	}
	controls := make([]model.CheckpointControl, 0, len(payload.Features))
	name := "Checkpoint-Kurs"
	sportGroup := "generic"
	for _, feature := range payload.Features {
		if feature.Type != "Feature" {
			return nil, fmt.Errorf("checkpoint import contains non-Feature entry")
		}
		if boolProp(feature.Properties, "display_only") {
			continue
		}
		if stringProp(feature.Properties, "course_kind") != string(model.CourseKindCheckpointSequence) {
			return nil, fmt.Errorf("checkpoint import only accepts checkpoint_sequence features")
		}
		if stringProp(feature.Properties, "matching_contract_key") != database.CheckpointSequenceV1Key {
			return nil, fmt.Errorf("checkpoint import requires checkpoint_sequence_v1 contract")
		}
		if version, ok := intProp(feature.Properties, "matching_contract_version"); !ok || version != database.CheckpointSequenceV1Version {
			return nil, fmt.Errorf("checkpoint import requires matching_contract_version 1")
		}
		if sem := stringProp(feature.Properties, "import_semantics"); sem != "" && sem != "private_copy_rebased_on_import" {
			return nil, fmt.Errorf("checkpoint import does not support import_semantics %q", sem)
		}
		if v := stringProp(feature.Properties, "name"); v != "" {
			name = v
		}
		if v := stringProp(feature.Properties, "sport_group"); v != "" {
			sportGroup = v
		}
		seq, ok := intProp(feature.Properties, "sequence_index")
		if !ok {
			return nil, fmt.Errorf("checkpoint control is missing sequence_index")
		}
		required, ok := boolPropOK(feature.Properties, "required")
		if !ok {
			return nil, fmt.Errorf("checkpoint control is missing required flag")
		}
		control := model.CheckpointControl{
			SequenceIndex: seq,
			Role:          model.CheckpointControlRole(stringProp(feature.Properties, "role")),
			Primitive:     model.CheckpointControlPrimitive(stringProp(feature.Properties, "primitive")),
			GeometryShape: model.CheckpointControlGeometryShape(stringProp(feature.Properties, "geometry_shape")),
			Label:         stringProp(feature.Properties, "label"),
			Geometry:      cloneJSONRaw(feature.Geometry),
			Required:      required,
		}
		if v, ok := floatProp(feature.Properties, "hit_radius_m"); ok {
			control.HitRadiusM = &v
		}
		if v, ok := floatProp(feature.Properties, "area_radius_m"); ok {
			control.AreaRadiusM = &v
		}
		controls = append(controls, control)
	}
	if len(controls) == 0 {
		return nil, fmt.Errorf("checkpoint import contains no controls")
	}
	sort.Slice(controls, func(i, j int) bool {
		return controls[i].SequenceIndex < controls[j].SequenceIndex
	})
	return &model.CheckpointCourseCreate{
		Name:       name,
		SportGroup: sportGroup,
		Visibility: "private",
		Controls:   controls,
	}, nil
}

func cloneJSONRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}

func stringProp(props map[string]any, key string) string {
	if props == nil {
		return ""
	}
	v, _ := props[key].(string)
	return v
}

func boolProp(props map[string]any, key string) bool {
	v, _ := boolPropOK(props, key)
	return v
}

func boolPropOK(props map[string]any, key string) (bool, bool) {
	if props == nil {
		return false, false
	}
	v, ok := props[key].(bool)
	return v, ok
}

func intProp(props map[string]any, key string) (int, bool) {
	if props == nil {
		return 0, false
	}
	switch v := props[key].(type) {
	case float64:
		return int(v), v == float64(int(v))
	case int:
		return v, true
	default:
		return 0, false
	}
}

func floatProp(props map[string]any, key string) (float64, bool) {
	if props == nil {
		return 0, false
	}
	switch v := props[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
}
