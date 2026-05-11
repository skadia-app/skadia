package gear

// applicableSportPredicatesDefaults maps gear type keys to their default
// applicable-sport predicates as defined in
// config/sport-taxonomy/projections/gear_defaults.yaml.
// Drift between this literal and the YAML is caught by
// TestApplicableSportPredicatesDefaultsMatchYAML in
// applicable_sport_predicates_defaults_drift_test.go.
//
// Legacy schema keys 'apparel' and 'climbing_harness' have no entry —
// items using those keys are deprecated and should migrate to 'apparel.*' / 'climb.harness'.
var applicableSportPredicatesDefaults = map[string][]GearDefaultPredicate{
	"bike": {
		{SportType: "cycling"},
	},
	"bike.balance": {
		{SportType: "cycling"},
	},
	"bike.cargo": {
		{SportType: "cycling"},
	},
	"bike.city": {
		{SportType: "cycling"},
	},
	"bike.consumable.brake_pads": {
		{SportType: "cycling"},
	},
	"bike.consumable.chain_lube": {
		{SportType: "cycling"},
	},
	"bike.consumable.tire_sealant": {
		{SportType: "cycling"},
	},
	"bike.gravel": {
		{SportType: "cycling", SubType: strPtr("gravel_cycling")},
	},
	"bike.gravel.cyclocross": {
		{SportType: "cycling", SubType: strPtr("gravel_cycling")},
		{SportType: "cycling", SubType: strPtr("cyclocross")},
	},
	"bike.handcycle": {
		{SportType: "cycling", SubType: strPtr("hand_cycling")},
	},
	"bike.indoor": {
		{SportType: "cycling", SubType: strPtr("indoor_cycling")},
	},
	"bike.kids": {
		{SportType: "cycling"},
	},
	"bike.mtb": {
		{SportType: "cycling", SubType: strPtr("mountain")},
	},
	"bike.recumbent": {
		{SportType: "cycling", SubType: strPtr("recumbent")},
	},
	"bike.road": {
		{SportType: "cycling", SubType: strPtr("road")},
	},
	"bike.tandem": {
		{SportType: "cycling"},
	},
	"bike.trekking": {
		{SportType: "cycling"},
	},
	"buoyancy_compensator": {
		{SportType: "diving"},
	},
	"climb.belay_device": {
		{SportType: "rock_climbing"},
	},
	"climb.chalkbag": {
		{SportType: "rock_climbing"},
	},
	"climb.crampons": {
		{SportType: "rock_climbing"},
		{SportType: "mountaineering"},
	},
	"climb.crashpad": {
		{SportType: "rock_climbing"},
	},
	"climb.harness": {
		{SportType: "rock_climbing"},
	},
	"climb.ice_axe": {
		{SportType: "mountaineering"},
	},
	"climb.quickdraw": {
		{SportType: "rock_climbing"},
	},
	"climb.rope": {
		{SportType: "rock_climbing"},
	},
	"climbing_harness": {
		{SportType: "rock_climbing"},
	},
	"dive_computer": {
		{SportType: "diving"},
	},
	"dive_mask": {
		{SportType: "diving"},
		{SportType: "snorkeling"},
	},
	"dive_tank": {
		{SportType: "diving"},
	},
	"dive_weights": {
		{SportType: "diving"},
	},
	"fins": {
		{SportType: "diving"},
		{SportType: "snorkeling"},
		{SportType: "swimming"},
	},
	"fishing_gear": {
		{SportType: "fishing"},
	},
	"goggles": {
		{SportType: "alpine_skiing"},
		{SportType: "snowboarding"},
		{SportType: "cycling"},
		{SportType: "swimming"},
	},
	"helmet": {
		{SportType: "cycling"},
		{SportType: "alpine_skiing"},
		{SportType: "snowboarding"},
		{SportType: "rock_climbing"},
	},
	"ice_skates": {
		{SportType: "ice_skating"},
	},
	"paddle": {
		{SportType: "kayaking"},
		{SportType: "canoeing"},
		{SportType: "stand_up_paddleboarding"},
		{SportType: "rowing"},
	},
	"regulator": {
		{SportType: "diving"},
	},
	"shoes": {
		{SportType: "cycling"},
		{SportType: "running"},
		{SportType: "hiking"},
	},
	"shoes.climbing": {
		{SportType: "rock_climbing"},
	},
	"shoes.cycling": {
		{SportType: "cycling"},
	},
	"shoes.cycling.gravel": {
		{SportType: "cycling", SubType: strPtr("gravel_cycling")},
	},
	"shoes.hiking": {
		{SportType: "hiking"},
		{SportType: "mountaineering"},
	},
	"shoes.mountaineering": {
		{SportType: "mountaineering"},
	},
	"shoes.running": {
		{SportType: "running"},
	},
	"shoes.ski": {
		{SportType: "alpine_skiing"},
		{SportType: "backcountry_skiing"},
		{SportType: "cross_country_skiing"},
	},
	"shoes.soccer": {
		{SportType: "soccer"},
	},
	"shoes.training": {
		{SportType: "training"},
		{SportType: "strength_training"},
	},
	"ski": {
		{SportType: "alpine_skiing"},
		{SportType: "backcountry_skiing"},
		{SportType: "cross_country_skiing"},
	},
	"ski_boots": {
		{SportType: "skiing"},
		{SportType: "alpine_skiing"},
		{SportType: "backcountry_skiing"},
	},
	"ski_poles": {
		{SportType: "skiing"},
		{SportType: "cross_country_skiing"},
		{SportType: "backcountry_skiing"},
	},
	"sled": {
		{SportType: "sledding"},
	},
	"snorkel": {
		{SportType: "diving"},
		{SportType: "snorkeling"},
	},
	"snowboard": {
		{SportType: "snowboarding"},
	},
	"snowboard_boots": {
		{SportType: "snowboarding"},
	},
	"snowshoes": {
		{SportType: "snowshoeing"},
	},
	"water_ski": {
		{SportType: "water_skiing"},
	},
	"watercraft": {
		{SportType: "kayaking"},
		{SportType: "rowing"},
		{SportType: "paddling"},
		{SportType: "stand_up_paddleboarding"},
	},
	"watercraft.boat": {
		{SportType: "boating"},
	},
	"watercraft.canoe": {
		{SportType: "canoeing"},
	},
	"watercraft.kayak": {
		{SportType: "kayaking"},
	},
	"watercraft.kiteboard": {
		{SportType: "kitesurfing"},
	},
	"watercraft.raft": {
		{SportType: "rafting"},
	},
	"watercraft.rowing_boat": {
		{SportType: "rowing"},
	},
	"watercraft.sailboat": {
		{SportType: "sailing"},
	},
	"watercraft.sup_board": {
		{SportType: "stand_up_paddleboarding"},
	},
	"watercraft.surfboard": {
		{SportType: "surfing"},
	},
	"watercraft.wakeboard": {
		{SportType: "wakeboarding"},
		{SportType: "wakesurfing"},
	},
	"watercraft.windsurf_board": {
		{SportType: "windsurfing"},
	},
}

// ApplicableSportPredicatesDefaults returns a shallow copy of the
// applicableSportPredicatesDefaults map for use in tests and drift guards.
func ApplicableSportPredicatesDefaults() map[string][]GearDefaultPredicate {
	out := make(map[string][]GearDefaultPredicate, len(applicableSportPredicatesDefaults))
	for k, v := range applicableSportPredicatesDefaults {
		out[k] = v
	}
	return out
}

func strPtr(s string) *string { return &s }
