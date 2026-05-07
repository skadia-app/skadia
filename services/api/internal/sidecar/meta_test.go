package sidecar

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestGenerateMetaFullUsesFinalActivityTupleAndCommuteFlag(t *testing.T) {
	subType := "mountain"
	title := "Final Activity Title"
	data := &model.ActivityData{
		OriginalName: "Imported Source Title",
		SportType:    "cycling",
		SubType:      "road",
		StartedAt:    time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
	}
	activity := &model.Activity{
		ID:            "act-1",
		UserID:        "user-1",
		Title:         &title,
		SportType:     "cycling",
		SubType:       &subType,
		Visibility:    "private",
		CreatedAt:     time.Date(2026, 5, 1, 10, 1, 0, 0, time.UTC),
		TitleIsManual: true,
	}
	setGeneratedModifierFields(t, activity, true)

	meta := GenerateMetaFull("act-1", data, activity)

	if meta.SportType != "cycling" || meta.SubType != "mountain" {
		t.Fatalf("meta tuple = %s/%s, want final activity cycling/mountain", meta.SportType, meta.SubType)
	}
	if meta.Title != "Final Activity Title" {
		t.Fatalf("meta.Title = %q, want final manual activity title", meta.Title)
	}
	for _, key := range sport.SportModifierKeysWithCapability("activity_state") {
		modifier, _ := sport.SportModifierByKey(key)
		if !reflect.ValueOf(meta).Elem().FieldByName(modifier.GoField).Bool() {
			t.Fatalf("meta.%s = false, want true", modifier.GoField)
		}
	}
}

func TestSidecarActivityStateModifiersRoundTripThroughJSON(t *testing.T) {
	assertModifierJSONRoundTrip(t, &MetaFile{})
	assertModifierJSONRoundTrip(t, &ManualActivityFile{})
}

func TestSidecarModifierFieldsMatchGeneratedActivityStateMetadata(t *testing.T) {
	metaType := reflect.TypeOf(MetaFile{})
	manualType := reflect.TypeOf(ManualActivityFile{})
	for _, key := range sport.SportModifierKeysWithCapability("activity_state") {
		modifier, ok := sport.SportModifierByKey(key)
		if !ok {
			t.Fatalf("missing modifier %s", key)
		}
		assertSidecarModifierField(t, metaType, modifier.GoField, modifier.JSONKey)
		assertSidecarModifierField(t, manualType, modifier.GoField, modifier.JSONKey)
	}
}

func assertSidecarModifierField(t *testing.T, typ reflect.Type, fieldName, jsonKey string) {
	t.Helper()
	field, ok := typ.FieldByName(fieldName)
	if !ok {
		t.Fatalf("%s missing field %s", typ.Name(), fieldName)
	}
	if field.Type.Kind() != reflect.Bool {
		t.Fatalf("%s.%s type = %s, want bool", typ.Name(), fieldName, field.Type)
	}
	if got := field.Tag.Get("json"); got == "" || len(got) < len(jsonKey) || got[:len(jsonKey)] != jsonKey {
		t.Fatalf("%s.%s json tag = %q, want %q", typ.Name(), fieldName, got, jsonKey)
	}
}

func assertModifierJSONRoundTrip(t *testing.T, value any) {
	t.Helper()
	for _, key := range sport.SportModifierKeysWithCapability("activity_state") {
		modifier, _ := sport.SportModifierByKey(key)
		original := reflect.New(reflect.TypeOf(value).Elem()).Interface()
		reflect.ValueOf(original).Elem().FieldByName(modifier.GoField).SetBool(true)
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal %T: %v", original, err)
		}
		decoded := reflect.New(reflect.TypeOf(value).Elem()).Interface()
		if err := json.Unmarshal(data, decoded); err != nil {
			t.Fatalf("unmarshal %T: %v", decoded, err)
		}
		if !reflect.ValueOf(decoded).Elem().FieldByName(modifier.GoField).Bool() {
			t.Fatalf("%T.%s did not round-trip through JSON", decoded, modifier.GoField)
		}
	}
}

func setGeneratedModifierFields(t *testing.T, value any, selected bool) {
	t.Helper()
	target := reflect.ValueOf(value).Elem()
	for _, key := range sport.SportModifierKeysWithCapability("activity_state") {
		modifier, _ := sport.SportModifierByKey(key)
		field := target.FieldByName(modifier.GoField)
		if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.Bool {
			t.Fatalf("%T missing settable bool field %s", value, modifier.GoField)
		}
		field.SetBool(selected)
	}
}
