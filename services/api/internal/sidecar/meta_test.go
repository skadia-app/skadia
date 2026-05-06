package sidecar

import (
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
		IsCommute:     true,
		IsIndoor:      true,
		IsElectric:    true,
		Visibility:    "private",
		CreatedAt:     time.Date(2026, 5, 1, 10, 1, 0, 0, time.UTC),
		TitleIsManual: true,
	}

	meta := GenerateMetaFull("act-1", data, activity)

	if meta.SportType != "cycling" || meta.SubType != "mountain" {
		t.Fatalf("meta tuple = %s/%s, want final activity cycling/mountain", meta.SportType, meta.SubType)
	}
	if meta.Title != "Final Activity Title" {
		t.Fatalf("meta.Title = %q, want final manual activity title", meta.Title)
	}
	if !meta.IsCommute {
		t.Fatal("meta.IsCommute = false, want true")
	}
	if !meta.IsIndoor || !meta.IsElectric {
		t.Fatalf("meta modifiers indoor/electric = %v/%v, want true/true", meta.IsIndoor, meta.IsElectric)
	}
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
