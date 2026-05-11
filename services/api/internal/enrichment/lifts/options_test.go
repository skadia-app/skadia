package lifts_test

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

func boolPtr(v bool) *bool { return &v }

func TestEffectiveLiftDetectionOpts(t *testing.T) {
	t.Parallel()

	type tc struct {
		name              string
		masterEnabled     bool
		userOSM           bool
		userHeuristic     bool
		overrideOSM       *bool
		overrideHeuristic *bool
		wantOSM           bool
		wantHeuristic     bool
	}

	cases := []tc{
		// master=false: always {false,false} regardless of user/override
		{name: "master_off/user_tt/override_nil_nil", masterEnabled: false, userOSM: true, userHeuristic: true, wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_tt/override_T_nil", masterEnabled: false, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(true), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_tt/override_F_nil", masterEnabled: false, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_tt/override_nil_T", masterEnabled: false, userOSM: true, userHeuristic: true, overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_tt/override_nil_F", masterEnabled: false, userOSM: true, userHeuristic: true, overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_tt/override_T_T", masterEnabled: false, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_tt/override_T_F", masterEnabled: false, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_tt/override_F_T", masterEnabled: false, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_tt/override_F_F", masterEnabled: false, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},

		{name: "master_off/user_ff/override_nil_nil", masterEnabled: false, userOSM: false, userHeuristic: false, wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ff/override_T_nil", masterEnabled: false, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(true), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ff/override_F_nil", masterEnabled: false, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ff/override_nil_T", masterEnabled: false, userOSM: false, userHeuristic: false, overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ff/override_nil_F", masterEnabled: false, userOSM: false, userHeuristic: false, overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ff/override_T_T", masterEnabled: false, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ff/override_T_F", masterEnabled: false, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ff/override_F_T", masterEnabled: false, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ff/override_F_F", masterEnabled: false, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},

		// master=true, user_tt: override nil → user setting wins
		{name: "master_on/user_tt/override_nil_nil", masterEnabled: true, userOSM: true, userHeuristic: true, wantOSM: true, wantHeuristic: true},
		// master=true, user_tt: override non-nil → override wins
		{name: "master_on/user_tt/override_T_nil", masterEnabled: true, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(true), wantOSM: true, wantHeuristic: true},
		{name: "master_on/user_tt/override_F_nil", masterEnabled: true, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(false), wantOSM: false, wantHeuristic: true},
		{name: "master_on/user_tt/override_nil_T", masterEnabled: true, userOSM: true, userHeuristic: true, overrideHeuristic: boolPtr(true), wantOSM: true, wantHeuristic: true},
		{name: "master_on/user_tt/override_nil_F", masterEnabled: true, userOSM: true, userHeuristic: true, overrideHeuristic: boolPtr(false), wantOSM: true, wantHeuristic: false},
		{name: "master_on/user_tt/override_T_T", masterEnabled: true, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(true), wantOSM: true, wantHeuristic: true},
		{name: "master_on/user_tt/override_T_F", masterEnabled: true, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(false), wantOSM: true, wantHeuristic: false},
		{name: "master_on/user_tt/override_F_T", masterEnabled: true, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: true},
		{name: "master_on/user_tt/override_F_F", masterEnabled: true, userOSM: true, userHeuristic: true, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},

		// master=true, user_ff: override nil → user setting (false) wins
		{name: "master_on/user_ff/override_nil_nil", masterEnabled: true, userOSM: false, userHeuristic: false, wantOSM: false, wantHeuristic: false},
		// master=true, user_ff: override non-nil → override wins (even re-enabling)
		{name: "master_on/user_ff/override_T_nil", masterEnabled: true, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(true), wantOSM: true, wantHeuristic: false},
		{name: "master_on/user_ff/override_F_nil", masterEnabled: true, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_on/user_ff/override_nil_T", masterEnabled: true, userOSM: false, userHeuristic: false, overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: true},
		{name: "master_on/user_ff/override_nil_F", masterEnabled: true, userOSM: false, userHeuristic: false, overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_on/user_ff/override_T_T", masterEnabled: true, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(true), wantOSM: true, wantHeuristic: true},
		{name: "master_on/user_ff/override_T_F", masterEnabled: true, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(false), wantOSM: true, wantHeuristic: false},
		{name: "master_on/user_ff/override_F_T", masterEnabled: true, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: true},
		{name: "master_on/user_ff/override_F_F", masterEnabled: true, userOSM: false, userHeuristic: false, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},

		// master=true, user_tf (OSM on, heuristic off): previously missing permutations.
		{name: "master_on/user_tf/override_nil_nil", masterEnabled: true, userOSM: true, userHeuristic: false, wantOSM: true, wantHeuristic: false},
		{name: "master_on/user_tf/override_T_nil", masterEnabled: true, userOSM: true, userHeuristic: false, overrideOSM: boolPtr(true), wantOSM: true, wantHeuristic: false},
		{name: "master_on/user_tf/override_F_nil", masterEnabled: true, userOSM: true, userHeuristic: false, overrideOSM: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_on/user_tf/override_nil_T", masterEnabled: true, userOSM: true, userHeuristic: false, overrideHeuristic: boolPtr(true), wantOSM: true, wantHeuristic: true},
		{name: "master_on/user_tf/override_nil_F", masterEnabled: true, userOSM: true, userHeuristic: false, overrideHeuristic: boolPtr(false), wantOSM: true, wantHeuristic: false},
		{name: "master_on/user_tf/override_T_T", masterEnabled: true, userOSM: true, userHeuristic: false, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(true), wantOSM: true, wantHeuristic: true},
		{name: "master_on/user_tf/override_T_F", masterEnabled: true, userOSM: true, userHeuristic: false, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(false), wantOSM: true, wantHeuristic: false},
		{name: "master_on/user_tf/override_F_T", masterEnabled: true, userOSM: true, userHeuristic: false, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: true},
		{name: "master_on/user_tf/override_F_F", masterEnabled: true, userOSM: true, userHeuristic: false, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},

		// master=true, user_ft (OSM off, heuristic on): previously missing permutations.
		{name: "master_on/user_ft/override_nil_nil", masterEnabled: true, userOSM: false, userHeuristic: true, wantOSM: false, wantHeuristic: true},
		{name: "master_on/user_ft/override_T_nil", masterEnabled: true, userOSM: false, userHeuristic: true, overrideOSM: boolPtr(true), wantOSM: true, wantHeuristic: true},
		{name: "master_on/user_ft/override_F_nil", masterEnabled: true, userOSM: false, userHeuristic: true, overrideOSM: boolPtr(false), wantOSM: false, wantHeuristic: true},
		{name: "master_on/user_ft/override_nil_T", masterEnabled: true, userOSM: false, userHeuristic: true, overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: true},
		{name: "master_on/user_ft/override_nil_F", masterEnabled: true, userOSM: false, userHeuristic: true, overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_on/user_ft/override_T_T", masterEnabled: true, userOSM: false, userHeuristic: true, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(true), wantOSM: true, wantHeuristic: true},
		{name: "master_on/user_ft/override_T_F", masterEnabled: true, userOSM: false, userHeuristic: true, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(false), wantOSM: true, wantHeuristic: false},
		{name: "master_on/user_ft/override_F_T", masterEnabled: true, userOSM: false, userHeuristic: true, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: true},
		{name: "master_on/user_ft/override_F_F", masterEnabled: true, userOSM: false, userHeuristic: true, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},

		// master=false, user_tf and user_ft: same all-false contract as other master=false rows.
		{name: "master_off/user_tf/override_nil_nil", masterEnabled: false, userOSM: true, userHeuristic: false, wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_tf/override_T_T", masterEnabled: false, userOSM: true, userHeuristic: false, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_tf/override_F_F", masterEnabled: false, userOSM: true, userHeuristic: false, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ft/override_nil_nil", masterEnabled: false, userOSM: false, userHeuristic: true, wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ft/override_T_T", masterEnabled: false, userOSM: false, userHeuristic: true, overrideOSM: boolPtr(true), overrideHeuristic: boolPtr(true), wantOSM: false, wantHeuristic: false},
		{name: "master_off/user_ft/override_F_F", masterEnabled: false, userOSM: false, userHeuristic: true, overrideOSM: boolPtr(false), overrideHeuristic: boolPtr(false), wantOSM: false, wantHeuristic: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := lifts.EffectiveLiftDetectionOpts(
				c.masterEnabled,
				c.userOSM, c.userHeuristic,
				c.overrideOSM, c.overrideHeuristic,
			)
			if got.OSMSnap != c.wantOSM {
				t.Errorf("OSMSnap: got %v, want %v", got.OSMSnap, c.wantOSM)
			}
			if got.Heuristic != c.wantHeuristic {
				t.Errorf("Heuristic: got %v, want %v", got.Heuristic, c.wantHeuristic)
			}
		})
	}
}
