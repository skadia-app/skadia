package dataimport

import (
	"reflect"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestExportedActivityFileTagsCarryGeneratedActivityStateModifiers(t *testing.T) {
	exportType := reflect.TypeOf(exportedActivityFileTags{})
	inputType := reflect.TypeOf(shadowfs.ActivitySchemaTagsInput{})
	for _, key := range sport.SportModifierKeysWithCapability("activity_state") {
		modifier, ok := sport.SportModifierByKey(key)
		if !ok {
			t.Fatalf("missing modifier %s", key)
		}
		assertBoolFieldWithJSONKey(t, exportType, modifier.GoField, modifier.JSONKey)
		if _, ok := inputType.FieldByName(modifier.GoField); !ok {
			t.Fatalf("ActivitySchemaTagsInput missing %s for %s", modifier.GoField, key)
		}
	}
}

func assertBoolFieldWithJSONKey(t *testing.T, typ reflect.Type, fieldName, jsonKey string) {
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
