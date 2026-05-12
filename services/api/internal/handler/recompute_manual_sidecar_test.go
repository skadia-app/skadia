package handler

import (
	"reflect"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestActivityDataFromManualSidecarPreservesModifierState(t *testing.T) {
	startedAt := time.Date(2026, 5, 6, 12, 30, 0, 0, time.UTC)
	manual := &sidecar.ManualActivityFile{
		FileFormat:     "manual",
		Source:         "manual",
		SportType:      "cycling",
		SubType:        "mountain",
		StartedAt:      startedAt,
		ElapsedTimeS:   3600,
		MovingTimeS:    3500,
		DistanceM:      12345,
		ElevationGainM: 450,
		ElevationLossM: 440,
	}
	setGeneratedManualSidecarModifierFields(t, manual, true)
	data := activityDataFromManualSidecar(manual)

	for _, key := range sport.SportModifierKeysWithCapability("activity_state") {
		modifier, _ := sport.SportModifierByKey(key)
		if !reflect.ValueOf(data).Elem().FieldByName(modifier.GoField).Bool() {
			t.Fatalf("activity data %s = false, want true", modifier.GoField)
		}
	}
	if !data.EndedAt.Equal(startedAt.Add(time.Hour)) {
		t.Fatalf("EndedAt = %v, want %v", data.EndedAt, startedAt.Add(time.Hour))
	}
}

func TestManualSidecarEnrichmentFieldsPreservesManualTitle(t *testing.T) {
	title := "Parsed sidecar title"
	description := "Parsed sidecar description"
	manual := &sidecar.ManualActivityFile{
		Title:       title,
		Description: description,
	}

	fields := manualSidecarEnrichmentFields(manual, &model.Activity{TitleIsManual: true})
	if _, ok := fields["title"]; ok {
		t.Fatal("manual title was included in enrichment fields")
	}
	if got := fields["description"]; got != description {
		t.Fatalf("description = %#v, want %q", got, description)
	}

	fields = manualSidecarEnrichmentFields(manual, &model.Activity{TitleIsManual: false})
	if got := fields["title"]; got != title {
		t.Fatalf("title = %#v, want %q", got, title)
	}
}

func setGeneratedManualSidecarModifierFields(t *testing.T, value *sidecar.ManualActivityFile, selected bool) {
	t.Helper()
	target := reflect.ValueOf(value).Elem()
	for _, key := range sport.SportModifierKeysWithCapability("activity_state") {
		modifier, _ := sport.SportModifierByKey(key)
		field := target.FieldByName(modifier.GoField)
		if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.Bool {
			t.Fatalf("ManualActivityFile missing settable bool field %s", modifier.GoField)
		}
		field.SetBool(selected)
	}
}
