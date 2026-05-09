package sport

import (
	"reflect"
	"strings"
	"testing"
)

func TestModifierPatchHasGeneratedFieldsForModifierPatchKeys(t *testing.T) {
	typ := reflect.TypeOf(ModifierPatch{})
	for _, key := range SportModifierKeysWithCapability("modifier_patch") {
		modifier, ok := SportModifierByKey(key)
		if !ok {
			t.Fatalf("missing modifier %s", key)
		}
		field, ok := typ.FieldByName(modifier.GoField)
		if !ok {
			t.Fatalf("ModifierPatch missing %s for %s", modifier.GoField, key)
		}
		if field.Type.Kind() != reflect.Ptr || field.Type.Elem().Kind() != reflect.Bool {
			t.Fatalf("ModifierPatch.%s type = %s, want *bool", modifier.GoField, field.Type)
		}
		if got := field.Tag.Get("json"); !strings.HasPrefix(got, modifier.JSONKey) {
			t.Fatalf("ModifierPatch.%s json tag = %q, want prefix %q", modifier.GoField, got, modifier.JSONKey)
		}
	}
}

func TestSportModifierExists(t *testing.T) {
	if !SportModifierExists("is_electric") {
		t.Fatal("expected true for known key")
	}
	if SportModifierExists("nonexistent_key") {
		t.Fatal("expected false for unknown key")
	}
}

func TestModifierValuesHasGeneratedFieldsForActivityStateKeys(t *testing.T) {
	typ := reflect.TypeOf(ModifierValues{})
	for _, key := range SportModifierKeysWithCapability("activity_state") {
		modifier, ok := SportModifierByKey(key)
		if !ok {
			t.Fatalf("missing modifier %s", key)
		}
		field, ok := typ.FieldByName(modifier.GoField)
		if !ok {
			t.Fatalf("ModifierValues missing %s for %s", modifier.GoField, key)
		}
		if field.Type.Kind() != reflect.Bool {
			t.Fatalf("ModifierValues.%s type = %s, want bool", modifier.GoField, field.Type)
		}
	}
}
