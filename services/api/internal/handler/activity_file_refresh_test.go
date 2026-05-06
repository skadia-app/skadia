package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestActivityUpdateAffectsFileSchema(t *testing.T) {
	title := "Abendrunde"
	sportType := "cycling"
	subType := "mountain"
	visibility := "private"
	gearID := []byte(`null`)
	trueValue := true
	rpe := 5

	cases := []struct {
		name   string
		update model.ActivityUpdate
		want   bool
	}{
		{name: "empty patch refreshes after external tag writes", update: model.ActivityUpdate{}, want: true},
		{name: "title", update: model.ActivityUpdate{Title: &title}, want: true},
		{name: "sport type", update: model.ActivityUpdate{SportType: &sportType}, want: true},
		{name: "sub type", update: model.ActivityUpdate{SubType: &subType}, want: true},
		{name: "electric modifier", update: model.ActivityUpdate{IsElectric: &trueValue}, want: true},
		{name: "indoor modifier", update: model.ActivityUpdate{IsIndoor: &trueValue}, want: true},
		{name: "virtual modifier", update: model.ActivityUpdate{IsVirtual: &trueValue}, want: true},
		{name: "tags cleared", update: model.ActivityUpdate{Tags: []string{}}, want: true},
		{name: "visibility", update: model.ActivityUpdate{Visibility: &visibility}, want: false},
		{name: "gear", update: model.ActivityUpdate{GearID: gearID}, want: false},
		{name: "workout rpe", update: model.ActivityUpdate{WorkoutRpe: &rpe}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := activityUpdateAffectsFileSchema(&tc.update); got != tc.want {
				t.Fatalf("activityUpdateAffectsFileSchema() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDeferActivityFileRefresh(t *testing.T) {
	cases := []struct {
		rawQuery string
		want     bool
	}{
		{rawQuery: "", want: false},
		{rawQuery: "defer_activity_file_refresh=false", want: false},
		{rawQuery: "defer_activity_file_refresh=true", want: true},
		{rawQuery: "defer_activity_file_refresh=TRUE", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.rawQuery, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/tags/tag-1/assign-activities?"+tc.rawQuery, nil)
			if got := deferActivityFileRefresh(req); got != tc.want {
				t.Fatalf("deferActivityFileRefresh() = %v, want %v", got, tc.want)
			}
		})
	}
}
