package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// gearSchemaTestHandler returns a minimal Handler connected to the test DB.
func gearSchemaTestHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "gschema-"+userID[:8], "gschema-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &Handler{DB: &database.DB{Pool: pool}}, userID
}

// TestGetGearSchemaReturnsCompleteSchema verifies the happy path:
// authenticated GET /api/gear/schema returns 200 with the required top-level keys.
func TestGetGearSchemaReturnsCompleteSchema(t *testing.T) {
	h, userID := gearSchemaTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/gear/schema", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.GetGearSchema(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for _, key := range []string{"assignmentRoles", "optionSets", "unitGroups", "attributes", "gearTypes", "componentTypes", "version"} {
		if _, ok := body[key]; !ok {
			t.Errorf("response missing top-level key %q", key)
		}
	}

	// version must be 1.
	var version int
	if err := json.Unmarshal(body["version"], &version); err != nil {
		t.Fatalf("decode version: %v", err)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1", version)
	}
}

// TestGetGearSchemaETagCaching verifies that a second request with the
// returned ETag gets a 304 Not Modified.
func TestGetGearSchemaETagCaching(t *testing.T) {
	h, userID := gearSchemaTestHandler(t)

	req1 := httptest.NewRequest(http.MethodGet, "/api/gear/schema", nil)
	req1 = req1.WithContext(context.WithValue(req1.Context(), middleware.UserIDKey, userID))
	rr1 := httptest.NewRecorder()
	h.GetGearSchema(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr1.Code)
	}

	etag := rr1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("first response has no ETag header")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/gear/schema", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.UserIDKey, userID))
	req2.Header.Set("If-None-Match", etag)
	rr2 := httptest.NewRecorder()
	h.GetGearSchema(rr2, req2)
	if rr2.Code != http.StatusNotModified {
		t.Fatalf("conditional request: expected 304, got %d", rr2.Code)
	}
}

// TestGetGearSchemaRequiresAuth verifies that an unauthenticated request
// is rejected with 401.
func TestGetGearSchemaRequiresAuth(t *testing.T) {
	h := &Handler{DB: &database.DB{Pool: nil}}
	req := httptest.NewRequest(http.MethodGet, "/api/gear/schema", nil)
	rr := httptest.NewRecorder()
	h.GetGearSchema(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// TestGetGearSchemaGearTypeAlwaysHasSliceKeys asserts that every gearType entry
// in the JSON response includes both "attributeKeys" and "modifierKeys" even when
// the schema-defined type has no keys (empty slice, not omitted).
// This validates that the GearType struct does NOT use omitempty on these fields.
func TestGetGearSchemaGearTypeAlwaysHasSliceKeys(t *testing.T) {
	h, userID := gearSchemaTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/gear/schema", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.GetGearSchema(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Decode into raw map to detect absent keys (typed structs coerce absent to zero).
	var raw map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	gearTypesRaw, ok := raw["gearTypes"].([]any)
	if !ok || len(gearTypesRaw) == 0 {
		t.Fatal("gearTypes must be a non-empty array")
	}
	for _, item := range gearTypesRaw {
		gt, ok := item.(map[string]any)
		if !ok {
			t.Fatal("each gearType element must be an object")
		}
		key, _ := gt["key"].(string)
		if _, hasAttr := gt["attributeKeys"]; !hasAttr {
			t.Errorf("gearType %q is missing \"attributeKeys\" in JSON (omitempty must not be used)", key)
		}
		if _, hasMod := gt["modifierKeys"]; !hasMod {
			t.Errorf("gearType %q is missing \"modifierKeys\" in JSON (omitempty must not be used)", key)
		}
	}
}

// TestGetGearSchemaWireFormatCamelCase asserts that the JSON response uses camelCase
// keys matching the frontend TS expectations (e.g. gearTypes[0].key, labelDe).
func TestGetGearSchemaWireFormatCamelCase(t *testing.T) {
	h, userID := gearSchemaTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/gear/schema", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.GetGearSchema(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Wire types with camelCase tags — these must decode without error for the
	// assertion to be meaningful.
	type wireOptionValue struct {
		Key     string            `json:"key"`
		LabelDe string            `json:"labelDe"`
		Labels  map[string]string `json:"labels"`
	}
	type wireOptionSet struct {
		Key     string            `json:"key"`
		Options []wireOptionValue `json:"options"`
	}
	type wireUnitGroup struct {
		Key     string            `json:"key"`
		LabelDe string            `json:"labelDe"`
		Labels  map[string]string `json:"labels"`
	}
	type wireAttribute struct {
		Key          string            `json:"key"`
		LabelDe      string            `json:"labelDe"`
		Labels       map[string]string `json:"labels"`
		ValueType    string            `json:"valueType"`
		OptionSetKey string            `json:"optionSetKey"`
	}
	type wireGearType struct {
		Key                      string            `json:"key"`
		LabelDe                  string            `json:"labelDe"`
		Labels                   map[string]string `json:"labels"`
		Scope                    string            `json:"scope"`
		DefaultAssignmentRoleKey string            `json:"defaultAssignmentRoleKey"`
		AttributeKeys            []string          `json:"attributeKeys"`
		ModifierKeys             []string          `json:"modifierKeys"`
	}
	type wireComponentType struct {
		Key     string            `json:"key"`
		LabelDe string            `json:"labelDe"`
		Labels  map[string]string `json:"labels"`
	}
	type wireAssignmentRole struct {
		Key               string            `json:"key"`
		LabelDe           string            `json:"labelDe"`
		Labels            map[string]string `json:"labels"`
		UniquePerActivity bool              `json:"uniquePerActivity"`
	}
	type wireSchema struct {
		Version         int                  `json:"version"`
		GearTypes       []wireGearType       `json:"gearTypes"`
		ComponentTypes  []wireComponentType  `json:"componentTypes"`
		AssignmentRoles []wireAssignmentRole `json:"assignmentRoles"`
		OptionSets      []wireOptionSet      `json:"optionSets"`
		UnitGroups      []wireUnitGroup      `json:"unitGroups"`
		Attributes      []wireAttribute      `json:"attributes"`
	}

	var schema wireSchema
	if err := json.NewDecoder(rr.Body).Decode(&schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}

	// Verify gearTypes are non-empty and have key + labelDe populated.
	if len(schema.GearTypes) == 0 {
		t.Fatal("gearTypes must be non-empty")
	}
	gt0 := schema.GearTypes[0]
	if gt0.Key == "" {
		t.Error("gearTypes[0].key must not be empty (camelCase JSON tag check)")
	}
	if gt0.LabelDe == "" {
		t.Error("gearTypes[0].labelDe must not be empty (camelCase JSON tag check)")
	}
	if gt0.Labels["de"] == "" {
		t.Error("gearTypes[0].labels.de must not be empty")
	}

	// Verify assignmentRoles are non-empty and have key + labelDe.
	if len(schema.AssignmentRoles) == 0 {
		t.Fatal("assignmentRoles must be non-empty")
	}
	ar0 := schema.AssignmentRoles[0]
	if ar0.Key == "" {
		t.Error("assignmentRoles[0].key must not be empty (camelCase JSON tag check)")
	}
	if ar0.LabelDe == "" {
		t.Error("assignmentRoles[0].labelDe must not be empty (camelCase JSON tag check)")
	}
	if ar0.Labels["en"] == "" {
		t.Error("assignmentRoles[0].labels.en must not be empty")
	}

	// Verify optionSets are non-empty with nested options.
	if len(schema.OptionSets) == 0 {
		t.Fatal("optionSets must be non-empty")
	}
	os0 := schema.OptionSets[0]
	if os0.Key == "" {
		t.Error("optionSets[0].key must not be empty (camelCase JSON tag check)")
	}
	if len(os0.Options) == 0 {
		t.Error("optionSets[0].options must be non-empty")
	}
	if os0.Options[0].LabelDe == "" {
		t.Error("optionSets[0].options[0].labelDe must not be empty (camelCase JSON tag check)")
	}
	if os0.Options[0].Labels["fr"] == "" {
		t.Error("optionSets[0].options[0].labels.fr must not be empty")
	}

	if len(schema.UnitGroups) == 0 {
		t.Fatal("unitGroups must be non-empty")
	}
	if schema.UnitGroups[0].Labels["it"] == "" {
		t.Error("unitGroups[0].labels.it must not be empty")
	}
	if len(schema.ComponentTypes) == 0 {
		t.Fatal("componentTypes must be non-empty")
	}
	if schema.ComponentTypes[0].Labels["es"] == "" {
		t.Error("componentTypes[0].labels.es must not be empty")
	}
}

// TestGetGearSchemaJSONKeysAreCamelCase decodes the response into map[string]any
// (no typed structs) and asserts that every structural key is camelCase.
// Absent keys and PascalCase/snake_case keys are both treated as failures.
func TestGetGearSchemaJSONKeysAreCamelCase(t *testing.T) {
	h, userID := gearSchemaTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/gear/schema", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.GetGearSchema(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var raw map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// requireKey asserts that the given map contains a key and that the key is
	// not in PascalCase or snake_case form.
	requireKey := func(obj map[string]any, context, key string) {
		t.Helper()
		if _, ok := obj[key]; !ok {
			t.Errorf("%s: missing required key %q", context, key)
		}
		// Detect PascalCase: first letter uppercase.
		if len(key) > 0 && key[0] >= 'A' && key[0] <= 'Z' {
			t.Errorf("%s: key %q appears to be PascalCase (must be camelCase)", context, key)
		}
		// Detect snake_case: contains underscore (camelCase identifiers do not).
		for _, ch := range key {
			if ch == '_' {
				t.Errorf("%s: key %q appears to be snake_case (must be camelCase)", context, key)
				break
			}
		}
	}

	// Top-level keys.
	for _, k := range []string{"assignmentRoles", "optionSets", "unitGroups", "attributes", "gearTypes", "componentTypes", "version"} {
		requireKey(raw, "top-level", k)
	}

	// gearTypes[0] keys.
	gearTypesRaw, _ := raw["gearTypes"].([]any)
	if len(gearTypesRaw) == 0 {
		t.Fatal("gearTypes must be non-empty")
	}
	gt0, _ := gearTypesRaw[0].(map[string]any)
	if gt0 == nil {
		t.Fatal("gearTypes[0] must be an object")
	}
	for _, k := range []string{"key", "labelDe", "labels", "scope", "defaultAssignmentRoleKey", "attributeKeys", "modifierKeys"} {
		requireKey(gt0, "gearTypes[0]", k)
	}
	// No PascalCase keys may be present anywhere in gearTypes[0].
	for k := range gt0 {
		if len(k) > 0 && k[0] >= 'A' && k[0] <= 'Z' {
			t.Errorf("gearTypes[0]: unexpected PascalCase key %q", k)
		}
	}

	// attributes[0] keys — must have key, labelDe, valueType.
	attributesRaw, _ := raw["attributes"].([]any)
	if len(attributesRaw) == 0 {
		t.Fatal("attributes must be non-empty")
	}
	attr0, _ := attributesRaw[0].(map[string]any)
	if attr0 == nil {
		t.Fatal("attributes[0] must be an object")
	}
	for _, k := range []string{"key", "labelDe", "labels", "valueType"} {
		requireKey(attr0, "attributes[0]", k)
	}
	for k := range attr0 {
		if len(k) > 0 && k[0] >= 'A' && k[0] <= 'Z' {
			t.Errorf("attributes[0]: unexpected PascalCase key %q", k)
		}
	}

	// optionSets[0] keys.
	optionSetsRaw, _ := raw["optionSets"].([]any)
	if len(optionSetsRaw) == 0 {
		t.Fatal("optionSets must be non-empty")
	}
	os0, _ := optionSetsRaw[0].(map[string]any)
	if os0 == nil {
		t.Fatal("optionSets[0] must be an object")
	}
	for _, k := range []string{"key", "options"} {
		requireKey(os0, "optionSets[0]", k)
	}
	for k := range os0 {
		if len(k) > 0 && k[0] >= 'A' && k[0] <= 'Z' {
			t.Errorf("optionSets[0]: unexpected PascalCase key %q", k)
		}
	}
}
