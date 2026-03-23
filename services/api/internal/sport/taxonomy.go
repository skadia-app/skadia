package sport

import (
	"sort"
	"strings"
)

// Category groups related sport types into a UI-friendly bucket.
type Category struct {
	Key     string `json:"key"`
	LabelDE string `json:"label_de"`
}

// SubTypeDef describes a sub-sport variation within a parent sport.
type SubTypeDef struct {
	SubType     string `json:"sub_type"`
	LabelDE     string `json:"label_de"`
	DetailDE    string `json:"detail_de"`
	AutoTitleDE string `json:"auto_title_de"`
}

// SportDef holds every piece of metadata for a single sport type.
type SportDef struct {
	SportType        string                `json:"sport_type"`
	CategoryKey      string                `json:"category_key"`
	LabelDE          string                `json:"label_de"`
	AutoTitleDE      string                `json:"auto_title_de"`
	SegmentGroup     string                `json:"segment_group"`
	TypicallyHasGPS  bool                  `json:"typically_has_gps"`
	MovingThreshold  float64               `json:"moving_threshold"`
	SubTypePromotion bool                  `json:"sub_type_promotion"`
	SubTypes         map[string]SubTypeDef `json:"sub_types,omitempty"`
}

// ---- Categories ----

var categories = []Category{
	{Key: "run", LabelDE: "Läufe"},
	{Key: "bike", LabelDE: "Radfahrten"},
	{Key: "winter", LabelDE: "Wintersport"},
	{Key: "water", LabelDE: "Wassersport"},
	{Key: "fitness", LabelDE: "Fitness"},
	{Key: "game", LabelDE: "Spiele"},
	{Key: "climb", LabelDE: "Klettern"},
	{Key: "outdoor", LabelDE: "Outdoor"},
	{Key: "multi", LabelDE: "Multisport"},
	{Key: "other", LabelDE: "Sonstiges"},
}

var categoryByKey map[string]Category

func init() {
	categoryByKey = make(map[string]Category, len(categories))
	for _, c := range categories {
		categoryByKey[c.Key] = c
	}
}

// ---- Sport definitions ----

// registry maps sport_type string → SportDef.
var registry map[string]SportDef

func init() {
	defs := []SportDef{
		// ── Run ──
		{
			SportType: "running", CategoryKey: "run", LabelDE: "Laufen", AutoTitleDE: "Lauf",
			SegmentGroup: "foot", TypicallyHasGPS: true, MovingThreshold: 0.5,
			SubTypes: map[string]SubTypeDef{
				"street":         {SubType: "street", LabelDE: "Straße", DetailDE: "Straßenlauf", AutoTitleDE: "Lauf"},
				"trail":          {SubType: "trail", LabelDE: "Trail", DetailDE: "Trailrun", AutoTitleDE: "Trailrun"},
				"track":          {SubType: "track", LabelDE: "Bahn", DetailDE: "Bahnlauf", AutoTitleDE: "Bahnlauf"},
				"treadmill":      {SubType: "treadmill", LabelDE: "Laufband", DetailDE: "Laufband", AutoTitleDE: "Laufband"},
				"indoor_running": {SubType: "indoor_running", LabelDE: "Indoor", DetailDE: "Indoor-Lauf", AutoTitleDE: "Indoor-Lauf"},
				"ultra":          {SubType: "ultra", LabelDE: "Ultra", DetailDE: "Ultramarathon", AutoTitleDE: "Ultramarathon"},
				"obstacle":       {SubType: "obstacle", LabelDE: "Hindernislauf", DetailDE: "Hindernislauf", AutoTitleDE: "Hindernislauf"},
			},
		},
		{
			SportType: "trail_running", CategoryKey: "run", LabelDE: "Trailrunning", AutoTitleDE: "Trailrun",
			SegmentGroup: "foot", TypicallyHasGPS: true, MovingThreshold: 0.5,
		},
		{
			SportType: "walking", CategoryKey: "run", LabelDE: "Gehen", AutoTitleDE: "Spaziergang",
			SegmentGroup: "foot", TypicallyHasGPS: true, MovingThreshold: 0.2,
			SubTypes: map[string]SubTypeDef{
				"casual_walking": {SubType: "casual_walking", LabelDE: "Spaziergang", DetailDE: "Spaziergang", AutoTitleDE: "Spaziergang"},
				"speed_walking":  {SubType: "speed_walking", LabelDE: "Speed Walking", DetailDE: "Speed Walking", AutoTitleDE: "Speed Walking"},
				"indoor_walking": {SubType: "indoor_walking", LabelDE: "Indoor", DetailDE: "Indoor-Gehen", AutoTitleDE: "Indoor-Gehen"},
			},
		},
		{
			SportType: "hiking", CategoryKey: "run", LabelDE: "Wandern", AutoTitleDE: "Wanderung",
			SegmentGroup: "foot", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},
		{
			SportType: "mountaineering", CategoryKey: "run", LabelDE: "Bergsteigen", AutoTitleDE: "Bergtour",
			SegmentGroup: "foot", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},
		{
			SportType: "snowshoeing", CategoryKey: "run", LabelDE: "Schneeschuhwandern", AutoTitleDE: "Schneeschuhtour",
			SegmentGroup: "foot", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},
		{
			SportType: "floor_climbing", CategoryKey: "run", LabelDE: "Treppensteigen", AutoTitleDE: "Treppensteigen",
			SegmentGroup: "foot", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "wheelchair_push_walk", CategoryKey: "run", LabelDE: "Rollstuhl (Fahren)", AutoTitleDE: "Rollstuhl-Spaziergang",
			SegmentGroup: "foot", TypicallyHasGPS: true, MovingThreshold: 0.2,
			SubTypes: map[string]SubTypeDef{
				"indoor_wheelchair_walk": {SubType: "indoor_wheelchair_walk", LabelDE: "Indoor", DetailDE: "Indoor-Rollstuhl", AutoTitleDE: "Indoor-Rollstuhl"},
			},
		},
		{
			SportType: "wheelchair_push_run", CategoryKey: "run", LabelDE: "Rollstuhl (Rennen)", AutoTitleDE: "Rollstuhl-Lauf",
			SegmentGroup: "foot", TypicallyHasGPS: true, MovingThreshold: 0.5,
			SubTypes: map[string]SubTypeDef{
				"indoor_wheelchair_run": {SubType: "indoor_wheelchair_run", LabelDE: "Indoor", DetailDE: "Indoor-Rollstuhl-Lauf", AutoTitleDE: "Indoor-Rollstuhl-Lauf"},
			},
		},

		// ── Bike ──
		{
			SportType: "cycling", CategoryKey: "bike", LabelDE: "Radfahren", AutoTitleDE: "Radfahrt",
			SegmentGroup: "bike", TypicallyHasGPS: true, MovingThreshold: 0.5,
			SubTypes: map[string]SubTypeDef{
				"road":            {SubType: "road", LabelDE: "Rennrad", DetailDE: "Rennrad", AutoTitleDE: "Rennradfahrt"},
				"mountain":        {SubType: "mountain", LabelDE: "MTB", DetailDE: "Mountainbike", AutoTitleDE: "MTB-Tour"},
				"downhill":        {SubType: "downhill", LabelDE: "Downhill", DetailDE: "Downhill", AutoTitleDE: "Downhill"},
				"recumbent":       {SubType: "recumbent", LabelDE: "Liegerad", DetailDE: "Liegerad", AutoTitleDE: "Liegerad-Tour"},
				"cyclocross":      {SubType: "cyclocross", LabelDE: "Cyclocross", DetailDE: "Cyclocross", AutoTitleDE: "Cyclocross"},
				"hand_cycling":    {SubType: "hand_cycling", LabelDE: "Handbike", DetailDE: "Handbike", AutoTitleDE: "Handbike-Tour"},
				"track_cycling":   {SubType: "track_cycling", LabelDE: "Bahnrad", DetailDE: "Bahnrad", AutoTitleDE: "Bahnrad"},
				"indoor_cycling":  {SubType: "indoor_cycling", LabelDE: "Indoor", DetailDE: "Indoor-Rad", AutoTitleDE: "Indoor-Radfahrt"},
				"spin":            {SubType: "spin", LabelDE: "Spinning", DetailDE: "Spinning", AutoTitleDE: "Spinning"},
				"gravel_cycling":  {SubType: "gravel_cycling", LabelDE: "Gravel", DetailDE: "Gravel", AutoTitleDE: "Gravel-Tour"},
				"unicycling":      {SubType: "unicycling", LabelDE: "Einrad", DetailDE: "Einrad", AutoTitleDE: "Einradfahrt"},
				"mixed_surface":   {SubType: "mixed_surface", LabelDE: "Mixed", DetailDE: "Mixed Surface", AutoTitleDE: "Mixed-Tour"},
				"bmx":             {SubType: "bmx", LabelDE: "BMX", DetailDE: "BMX", AutoTitleDE: "BMX"},
			},
		},
		{
			SportType: "mountain_biking", CategoryKey: "bike", LabelDE: "Mountainbike", AutoTitleDE: "MTB-Tour",
			SegmentGroup: "bike", TypicallyHasGPS: true, MovingThreshold: 0.5,
		},
		{
			SportType: "gravel_cycling", CategoryKey: "bike", LabelDE: "Gravel", AutoTitleDE: "Gravel-Tour",
			SegmentGroup: "bike", TypicallyHasGPS: true, MovingThreshold: 0.5,
		},
		{
			SportType: "indoor_cycling", CategoryKey: "bike", LabelDE: "Indoor-Radfahren", AutoTitleDE: "Indoor-Radfahrt",
			SegmentGroup: "bike", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "inline_skating", CategoryKey: "outdoor", LabelDE: "Inline-Skaten", AutoTitleDE: "Inline-Skaten",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.5,
		},
		{
		{
			SportType: "alpine_skiing", CategoryKey: "winter", LabelDE: "Alpin-Ski", AutoTitleDE: "Skifahrt",
			SegmentGroup: "ski", TypicallyHasGPS: true, MovingThreshold: 0.3,
			SubTypes: map[string]SubTypeDef{
				"backcountry":    {SubType: "backcountry", LabelDE: "Freeride", DetailDE: "Freeride/Backcountry", AutoTitleDE: "Freeride"},
				"resort":         {SubType: "resort", LabelDE: "Piste", DetailDE: "Pisten-Ski", AutoTitleDE: "Skifahrt"},
				"indoor_skiing":  {SubType: "indoor_skiing", LabelDE: "Indoor-Ski", DetailDE: "Indoor-Ski", AutoTitleDE: "Indoor-Skifahrt"},
			},
		},
		{
			SportType: "backcountry_skiing", CategoryKey: "winter", LabelDE: "Skitour", AutoTitleDE: "Skitour",
			SegmentGroup: "ski", TypicallyHasGPS: true, MovingThreshold: 0.3,
		},
		{
			SportType: "cross_country_skiing", CategoryKey: "winter", LabelDE: "Langlauf", AutoTitleDE: "Langlauf",
			SegmentGroup: "ski", TypicallyHasGPS: true, MovingThreshold: 0.3,
			SubTypes: map[string]SubTypeDef{
				"skate_skiing": {SubType: "skate_skiing", LabelDE: "Skating", DetailDE: "Skating-Stil", AutoTitleDE: "Skating-Langlauf"},
			},
		},
		{
			SportType: "snowboarding", CategoryKey: "winter", LabelDE: "Snowboard", AutoTitleDE: "Snowboard",
			SegmentGroup: "ski", TypicallyHasGPS: true, MovingThreshold: 0.3,
			SubTypes: map[string]SubTypeDef{
				"backcountry": {SubType: "backcountry", LabelDE: "Backcountry", DetailDE: "Backcountry-Snowboard", AutoTitleDE: "Backcountry-Snowboard"},
				"resort":      {SubType: "resort", LabelDE: "Piste", DetailDE: "Pisten-Snowboard", AutoTitleDE: "Snowboard"},
			},
		},
		{
			SportType: "ice_skating", CategoryKey: "winter", LabelDE: "Eislaufen", AutoTitleDE: "Eislaufen",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.3,
		},
		{
			SportType: "snowmobiling", CategoryKey: "winter", LabelDE: "Schneemobil", AutoTitleDE: "Schneemobil-Tour",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.5,
		},
		{
			SportType: "sledding", CategoryKey: "winter", LabelDE: "Rodeln", AutoTitleDE: "Rodeln",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},

		// ── Water ──
		{
			SportType: "swimming", CategoryKey: "water", LabelDE: "Schwimmen", AutoTitleDE: "Schwimmen",
			SegmentGroup: "swim", TypicallyHasGPS: false, MovingThreshold: 0.1,
			SubTypes: map[string]SubTypeDef{
				"lap_swimming": {SubType: "lap_swimming", LabelDE: "Bahnschwimmen", DetailDE: "Bahnschwimmen", AutoTitleDE: "Bahnschwimmen"},
				"open_water":   {SubType: "open_water", LabelDE: "Freiwasser", DetailDE: "Freiwasserschwimmen", AutoTitleDE: "Freiwasserschwimmen"},
			},
		},
		{
			SportType: "open_water_swimming", CategoryKey: "water", LabelDE: "Freiwasserschwimmen", AutoTitleDE: "Freiwasserschwimmen",
			SegmentGroup: "swim", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "lap_swimming", CategoryKey: "water", LabelDE: "Bahnschwimmen", AutoTitleDE: "Bahnschwimmen",
			SegmentGroup: "swim", TypicallyHasGPS: false, MovingThreshold: 0.1,
		},
		{
			SportType: "paddling", CategoryKey: "water", LabelDE: "Wassersport", AutoTitleDE: "Wassersport-Tour",
			SegmentGroup: "paddle", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "kayaking", CategoryKey: "water", LabelDE: "Kajak", AutoTitleDE: "Kajaktour",
			SegmentGroup: "paddle", TypicallyHasGPS: true, MovingThreshold: 0.1,
			SubTypes: map[string]SubTypeDef{
				"whitewater": {SubType: "whitewater", LabelDE: "Wildwasser", DetailDE: "Wildwasser-Kajak", AutoTitleDE: "Wildwasser-Kajak"},
			},
		},
		{
			SportType: "stand_up_paddleboarding", CategoryKey: "water", LabelDE: "SUP", AutoTitleDE: "SUP-Tour",
			SegmentGroup: "paddle", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "rowing", CategoryKey: "water", LabelDE: "Rudern", AutoTitleDE: "Rudern",
			SegmentGroup: "paddle", TypicallyHasGPS: true, MovingThreshold: 0.1,
			SubTypes: map[string]SubTypeDef{
				"indoor_rowing": {SubType: "indoor_rowing", LabelDE: "Indoor-Rudern", DetailDE: "Indoor-Rudern", AutoTitleDE: "Indoor-Rudern"},
			},
		},
		{
			SportType: "surfing", CategoryKey: "water", LabelDE: "Surfen", AutoTitleDE: "Surfen",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "kitesurfing", CategoryKey: "water", LabelDE: "Kitesurfen", AutoTitleDE: "Kitesurfen",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "windsurfing", CategoryKey: "water", LabelDE: "Windsurfen", AutoTitleDE: "Windsurfen",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "wakeboarding", CategoryKey: "water", LabelDE: "Wakeboard", AutoTitleDE: "Wakeboarden",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "wakesurfing", CategoryKey: "water", LabelDE: "Wakesurfen", AutoTitleDE: "Wakesurfen",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "water_skiing", CategoryKey: "water", LabelDE: "Wasserski", AutoTitleDE: "Wasserski",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "water_tubing", CategoryKey: "water", LabelDE: "Wassertubing", AutoTitleDE: "Wassertubing",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "diving", CategoryKey: "water", LabelDE: "Tauchen", AutoTitleDE: "Tauchen",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
			SubTypes: map[string]SubTypeDef{
				"single_gas_diving": {SubType: "single_gas_diving", LabelDE: "Eingas", DetailDE: "Eingas-Tauchen", AutoTitleDE: "Tauchen"},
				"multi_gas_diving":  {SubType: "multi_gas_diving", LabelDE: "Mehrgas", DetailDE: "Mehrgas-Tauchen", AutoTitleDE: "Tauchen"},
				"gauge_diving":      {SubType: "gauge_diving", LabelDE: "Gauge", DetailDE: "Gauge-Tauchen", AutoTitleDE: "Tauchen"},
				"apnea_diving":      {SubType: "apnea_diving", LabelDE: "Apnoe", DetailDE: "Apnoe-Tauchen", AutoTitleDE: "Apnoe-Tauchen"},
				"apnea_hunting":     {SubType: "apnea_hunting", LabelDE: "Apnoe-Jagd", DetailDE: "Apnoe-Jagdtauchen", AutoTitleDE: "Apnoe-Tauchen"},
			},
		},
		{
			SportType: "snorkeling", CategoryKey: "water", LabelDE: "Schnorcheln", AutoTitleDE: "Schnorcheln",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "boating", CategoryKey: "water", LabelDE: "Bootfahren", AutoTitleDE: "Bootstour",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
		{
			SportType: "sailing", CategoryKey: "water", LabelDE: "Segeln", AutoTitleDE: "Segeln",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.1,
			SubTypes: map[string]SubTypeDef{
				"sail_race": {SubType: "sail_race", LabelDE: "Regatta", DetailDE: "Segelregatta", AutoTitleDE: "Regatta"},
			},
		},
		{
			SportType: "fishing", CategoryKey: "water", LabelDE: "Angeln", AutoTitleDE: "Angeln",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.0,
		},
		{
			SportType: "rafting", CategoryKey: "water", LabelDE: "Rafting", AutoTitleDE: "Rafting",
			SegmentGroup: "paddle", TypicallyHasGPS: true, MovingThreshold: 0.1,
			SubTypes: map[string]SubTypeDef{
				"whitewater": {SubType: "whitewater", LabelDE: "Wildwasser", DetailDE: "Wildwasser-Rafting", AutoTitleDE: "Wildwasser-Rafting"},
			},
		},

		// ── Fitness ──
		{
			SportType: "training", CategoryKey: "fitness", LabelDE: "Training", AutoTitleDE: "Training",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
			SubTypePromotion: true,
			SubTypes: map[string]SubTypeDef{
				"strength_training":    {SubType: "strength_training", LabelDE: "Krafttraining", DetailDE: "Krafttraining", AutoTitleDE: "Krafttraining"},
				"flexibility_training": {SubType: "flexibility_training", LabelDE: "Stretching", DetailDE: "Dehnungstraining", AutoTitleDE: "Stretching"},
				"cardio_training":      {SubType: "cardio_training", LabelDE: "Cardio", DetailDE: "Cardio-Training", AutoTitleDE: "Cardio"},
				"yoga":                 {SubType: "yoga", LabelDE: "Yoga", DetailDE: "Yoga", AutoTitleDE: "Yoga"},
				"breathing":            {SubType: "breathing", LabelDE: "Atemübung", DetailDE: "Atemübung", AutoTitleDE: "Atemübung"},
				"hiit":                 {SubType: "hiit", LabelDE: "HIIT", DetailDE: "High Intensity Interval Training", AutoTitleDE: "HIIT"},
			},
		},
		{
			SportType: "fitness_equipment", CategoryKey: "fitness", LabelDE: "Fitnessgerät", AutoTitleDE: "Fitnessgerät",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
			SubTypePromotion: true,
			SubTypes: map[string]SubTypeDef{
				"treadmill":       {SubType: "treadmill", LabelDE: "Laufband", DetailDE: "Laufband", AutoTitleDE: "Laufband"},
				"indoor_cycling":  {SubType: "indoor_cycling", LabelDE: "Indoor-Rad", DetailDE: "Indoor-Rad", AutoTitleDE: "Indoor-Radfahrt"},
				"indoor_rowing":   {SubType: "indoor_rowing", LabelDE: "Ruder-Ergometer", DetailDE: "Ruder-Ergometer", AutoTitleDE: "Indoor-Rudern"},
				"elliptical":      {SubType: "elliptical", LabelDE: "Crosstrainer", DetailDE: "Crosstrainer", AutoTitleDE: "Crosstrainer"},
				"stair_climbing":  {SubType: "stair_climbing", LabelDE: "Treppensteiger", DetailDE: "Treppensteiger", AutoTitleDE: "Treppensteiger"},
				"pilates":         {SubType: "pilates", LabelDE: "Pilates", DetailDE: "Pilates", AutoTitleDE: "Pilates"},
				"indoor_skiing":   {SubType: "indoor_skiing", LabelDE: "Indoor-Ski", DetailDE: "Skimaschine", AutoTitleDE: "Indoor-Ski"},
				"indoor_walking":  {SubType: "indoor_walking", LabelDE: "Indoor-Gehen", DetailDE: "Indoor-Gehen", AutoTitleDE: "Indoor-Gehen"},
				"indoor_hand_cycling": {SubType: "indoor_hand_cycling", LabelDE: "Handbike-Ergometer", DetailDE: "Handbike-Ergometer", AutoTitleDE: "Indoor-Handbike"},
			},
		},
		{
			SportType: "strength_training", CategoryKey: "fitness", LabelDE: "Krafttraining", AutoTitleDE: "Krafttraining",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
			SubTypes: map[string]SubTypeDef{
				"amrap": {SubType: "amrap", LabelDE: "AMRAP", DetailDE: "As Many Reps As Possible", AutoTitleDE: "AMRAP"},
				"emom": {SubType: "emom", LabelDE: "EMOM", DetailDE: "Every Minute On the Minute", AutoTitleDE: "EMOM"},
			},
		},
		{
			SportType: "hiit", CategoryKey: "fitness", LabelDE: "HIIT", AutoTitleDE: "HIIT",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
			SubTypes: map[string]SubTypeDef{
				"tabata": {SubType: "tabata", LabelDE: "Tabata", DetailDE: "Tabata", AutoTitleDE: "Tabata"},
			},
		},
		{
			SportType: "dance", CategoryKey: "fitness", LabelDE: "Tanzen", AutoTitleDE: "Tanzen",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "jump_rope", CategoryKey: "fitness", LabelDE: "Seilspringen", AutoTitleDE: "Seilspringen",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "meditation", CategoryKey: "fitness", LabelDE: "Meditation", AutoTitleDE: "Meditation",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "yoga", CategoryKey: "fitness", LabelDE: "Yoga", AutoTitleDE: "Yoga",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "pilates", CategoryKey: "fitness", LabelDE: "Pilates", AutoTitleDE: "Pilates",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},

		// ── Game ──
		{
			SportType: "soccer", CategoryKey: "game", LabelDE: "Fußball", AutoTitleDE: "Fußball",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},
		{
			SportType: "basketball", CategoryKey: "game", LabelDE: "Basketball", AutoTitleDE: "Basketball",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "american_football", CategoryKey: "game", LabelDE: "American Football", AutoTitleDE: "American Football",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},
		{
			SportType: "rugby", CategoryKey: "game", LabelDE: "Rugby", AutoTitleDE: "Rugby",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},
		{
			SportType: "lacrosse", CategoryKey: "game", LabelDE: "Lacrosse", AutoTitleDE: "Lacrosse",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},
		{
			SportType: "hockey", CategoryKey: "game", LabelDE: "Hockey", AutoTitleDE: "Hockey",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},
		{
			SportType: "volleyball", CategoryKey: "game", LabelDE: "Volleyball", AutoTitleDE: "Volleyball",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "tennis", CategoryKey: "game", LabelDE: "Tennis", AutoTitleDE: "Tennis",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
			SubTypes: map[string]SubTypeDef{
				"warm_up":  {SubType: "warm_up", LabelDE: "Aufwärmen", DetailDE: "Aufwärmen", AutoTitleDE: "Tennis-Aufwärmen"},
				"match":    {SubType: "match", LabelDE: "Match", DetailDE: "Tennis-Match", AutoTitleDE: "Tennis-Match"},
				"exercise": {SubType: "exercise", LabelDE: "Übung", DetailDE: "Tennis-Übung", AutoTitleDE: "Tennis-Übung"},
			},
		},
		{
			SportType: "racket", CategoryKey: "game", LabelDE: "Schlägersport", AutoTitleDE: "Schlägersport",
			SubTypePromotion: true,
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
			SubTypes: map[string]SubTypeDef{
				"pickleball":  {SubType: "pickleball", LabelDE: "Pickleball", DetailDE: "Pickleball", AutoTitleDE: "Pickleball"},
				"padel":       {SubType: "padel", LabelDE: "Padel", DetailDE: "Padel", AutoTitleDE: "Padel"},
				"squash":      {SubType: "squash", LabelDE: "Squash", DetailDE: "Squash", AutoTitleDE: "Squash"},
				"badminton":   {SubType: "badminton", LabelDE: "Badminton", DetailDE: "Badminton", AutoTitleDE: "Badminton"},
				"racquetball": {SubType: "racquetball", LabelDE: "Racquetball", DetailDE: "Racquetball", AutoTitleDE: "Racquetball"},
				"table_tennis": {SubType: "table_tennis", LabelDE: "Tischtennis", DetailDE: "Tischtennis", AutoTitleDE: "Tischtennis"},
			},
		},
		{
			SportType: "cricket", CategoryKey: "game", LabelDE: "Cricket", AutoTitleDE: "Cricket",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.0,
		},
		{
			SportType: "baseball", CategoryKey: "game", LabelDE: "Baseball", AutoTitleDE: "Baseball",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "golf", CategoryKey: "game", LabelDE: "Golf", AutoTitleDE: "Golf",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.0,
		},
		{
			SportType: "disc_golf", CategoryKey: "game", LabelDE: "Disc Golf", AutoTitleDE: "Disc Golf",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.0,
		},

		// ── Climb ──
		{
			SportType: "rock_climbing", CategoryKey: "climb", LabelDE: "Klettern", AutoTitleDE: "Klettern",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
			SubTypes: map[string]SubTypeDef{
				"indoor_climbing": {SubType: "indoor_climbing", LabelDE: "Indoor", DetailDE: "Hallenklettern", AutoTitleDE: "Hallenklettern"},
				"bouldering":      {SubType: "bouldering", LabelDE: "Bouldern", DetailDE: "Bouldern", AutoTitleDE: "Bouldern"},
			},
		},

		// ── Outdoor ──
		{
			SportType: "flying", CategoryKey: "outdoor", LabelDE: "Fliegen", AutoTitleDE: "Fliegen",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.5,
			SubTypes: map[string]SubTypeDef{
				"rc_drone":        {SubType: "rc_drone", LabelDE: "Drohne", DetailDE: "Drohnenflug", AutoTitleDE: "Drohnenflug"},
				"wingsuit":        {SubType: "wingsuit", LabelDE: "Wingsuit", DetailDE: "Wingsuit", AutoTitleDE: "Wingsuit-Flug"},
				"fly_canopy":      {SubType: "fly_canopy", LabelDE: "Fallschirm", DetailDE: "Fallschirmspringen", AutoTitleDE: "Fallschirmsprung"},
				"fly_paraglide":   {SubType: "fly_paraglide", LabelDE: "Gleitschirm", DetailDE: "Gleitschirmfliegen", AutoTitleDE: "Gleitschirmflug"},
				"fly_paramotor":   {SubType: "fly_paramotor", LabelDE: "Paramotor", DetailDE: "Paramotorfliegen", AutoTitleDE: "Paramotorflug"},
				"fly_pressurized": {SubType: "fly_pressurized", LabelDE: "Druckkabine", DetailDE: "Druckkabinenflugzeug", AutoTitleDE: "Flug"},
				"fly_vfr":         {SubType: "fly_vfr", LabelDE: "Sichtflug (VFR)", DetailDE: "Sichtflugregeln (VFR)", AutoTitleDE: "VFR-Flug"},
				"fly_ifr":         {SubType: "fly_ifr", LabelDE: "Instrumentenflug (IFR)", DetailDE: "Instrumentenflugregeln (IFR)", AutoTitleDE: "IFR-Flug"},
				"fly_navigate":    {SubType: "fly_navigate", LabelDE: "Navigation", DetailDE: "Luftfahrt-Navigation", AutoTitleDE: "Flug"},
				"fly_timer":       {SubType: "fly_timer", LabelDE: "Timer", DetailDE: "Flug-Timer", AutoTitleDE: "Flug"},
				"fly_altimeter":   {SubType: "fly_altimeter", LabelDE: "Höhenmesser", DetailDE: "Höhenmesser-Modus", AutoTitleDE: "Flug"},
				"fly_wx":          {SubType: "fly_wx", LabelDE: "Wetter", DetailDE: "Flugwetter", AutoTitleDE: "Flug"},
			},
		},
		{
			SportType: "hang_gliding", CategoryKey: "outdoor", LabelDE: "Gleitschirmfliegen", AutoTitleDE: "Gleitschirmflug",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.5,
		},
		{
			SportType: "sky_diving", CategoryKey: "outdoor", LabelDE: "Fallschirmspringen", AutoTitleDE: "Fallschirmsprung",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.0,
		},
		{
			SportType: "jumpmaster", CategoryKey: "outdoor", LabelDE: "Jumpmaster", AutoTitleDE: "Jumpmaster",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.0,
		},
		{
			SportType: "horseback_riding", CategoryKey: "outdoor", LabelDE: "Reiten", AutoTitleDE: "Ausritt",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},
		{
			SportType: "hunting", CategoryKey: "outdoor", LabelDE: "Jagd", AutoTitleDE: "Jagd",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.0,
		},
		{
			SportType: "motorcycling", CategoryKey: "outdoor", LabelDE: "Motorrad", AutoTitleDE: "Motorradtour",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.5,
			SubTypes: map[string]SubTypeDef{
				"atv":       {SubType: "atv", LabelDE: "Quad/ATV", DetailDE: "Quad/ATV", AutoTitleDE: "Quad-Tour"},
				"motocross": {SubType: "motocross", LabelDE: "Motocross", DetailDE: "Motocross", AutoTitleDE: "Motocross"},
			},
		},
		{
			SportType: "driving", CategoryKey: "outdoor", LabelDE: "Autofahren", AutoTitleDE: "Autofahrt",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.5,
		},
		{
			SportType: "boxing", CategoryKey: "fitness", LabelDE: "Boxen", AutoTitleDE: "Boxen",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "mixed_martial_arts", CategoryKey: "fitness", LabelDE: "MMA", AutoTitleDE: "MMA",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "tactical", CategoryKey: "outdoor", LabelDE: "Taktisch", AutoTitleDE: "Taktisches Training",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.2,
		},

		// ── Multi ──
		{
			SportType: "multisport", CategoryKey: "multi", LabelDE: "Multisport", AutoTitleDE: "Multisport",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.0,
		},
		{
			SportType: "transition", CategoryKey: "multi", LabelDE: "Wechselzone", AutoTitleDE: "Wechselzone",
			SegmentGroup: "generic", TypicallyHasGPS: true, MovingThreshold: 0.0,
			SubTypes: map[string]SubTypeDef{
				"bike_to_run_transition": {SubType: "bike_to_run_transition", LabelDE: "Rad→Lauf", DetailDE: "Wechsel Rad zu Lauf", AutoTitleDE: "Wechselzone"},
				"run_to_bike_transition": {SubType: "run_to_bike_transition", LabelDE: "Lauf→Rad", DetailDE: "Wechsel Lauf zu Rad", AutoTitleDE: "Wechselzone"},
				"swim_to_bike_transition": {SubType: "swim_to_bike_transition", LabelDE: "Schwimmen→Rad", DetailDE: "Wechsel Schwimmen zu Rad", AutoTitleDE: "Wechselzone"},
			},
		},

		// ── Other ──
		{
			SportType: "generic", CategoryKey: "other", LabelDE: "Aktivität", AutoTitleDE: "Training",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "unknown", CategoryKey: "other", LabelDE: "Unbekannt", AutoTitleDE: "Training",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},

		// Aliases used in existing codebase — kept for backwards compatibility.
		{
			SportType: "skiing", CategoryKey: "winter", LabelDE: "Skifahren", AutoTitleDE: "Skifahrt",
			SegmentGroup: "ski", TypicallyHasGPS: true, MovingThreshold: 0.3,
		},
		{
			SportType: "treadmill_running", CategoryKey: "run", LabelDE: "Laufband", AutoTitleDE: "Laufband",
			SegmentGroup: "foot", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "strength", CategoryKey: "fitness", LabelDE: "Krafttraining", AutoTitleDE: "Krafttraining",
			SegmentGroup: "generic", TypicallyHasGPS: false, MovingThreshold: 0.0,
		},
		{
			SportType: "canoeing", CategoryKey: "water", LabelDE: "Kanu", AutoTitleDE: "Kanutour",
			SegmentGroup: "paddle", TypicallyHasGPS: true, MovingThreshold: 0.1,
		},
	}

	registry = make(map[string]SportDef, len(defs))
	for _, d := range defs {
		registry[d.SportType] = d
	}
}

// ---- Lookup functions ----

// GetSportDef returns the full definition for a sport type.
// The second return value is false when the sport type is unknown.
func GetSportDef(sportType string) (SportDef, bool) {
	d, ok := registry[sportType]
	return d, ok
}

// SportLabel returns the German display label, using sub-type promotion
// when appropriate. For sports with SubTypePromotion (training, fitness_equipment),
// the sub-type label is returned directly instead of the parent label.
func SportLabel(sportType, subType string) string {
	d, ok := registry[sportType]
	if !ok {
		return titleCase(sportType)
	}
	if d.SubTypePromotion && subType != "" && subType != "generic" {
		if st, stOK := d.SubTypes[subType]; stOK {
			return st.LabelDE
		}
	}
	return d.LabelDE
}

// SportDetailLabel returns a detailed German label including sub-type info.
// Examples: "Radfahren (MTB)", "Krafttraining", "Kajak (Wildwasser)".
func SportDetailLabel(sportType, subType string) string {
	d, ok := registry[sportType]
	if !ok {
		return titleCase(sportType)
	}
	if subType == "" || subType == "generic" {
		return d.LabelDE
	}
	if st, stOK := d.SubTypes[subType]; stOK {
		if d.SubTypePromotion {
			return st.DetailDE
		}
		return d.LabelDE + " (" + st.LabelDE + ")"
	}
	return d.LabelDE
}

// AutoTitle returns the German auto-title string used for auto-naming activities.
// Sub-type-promoted sports use the sub-type's title. Fallback is "Training".
func AutoTitle(sportType, subType string) string {
	d, ok := registry[sportType]
	if !ok {
		return "Training"
	}
	if subType != "" && subType != "generic" {
		if st, stOK := d.SubTypes[subType]; stOK && st.AutoTitleDE != "" {
			return st.AutoTitleDE
		}
	}
	return d.AutoTitleDE
}

// SegmentGroup returns the segment matching group for a sport type.
// Possible values: bike, foot, swim, ski, paddle, generic.
func SegmentGroup(sportType string) string {
	d, ok := registry[sportType]
	if !ok {
		return "generic"
	}
	return d.SegmentGroup
}

// MovingThreshold returns the minimum speed (m/s) that counts as "moving"
// for the given sport type.
func MovingThreshold(sportType string) float64 {
	d, ok := registry[sportType]
	if !ok {
		return 0.0
	}
	return d.MovingThreshold
}

// CategoryForSport returns the category for a given sport type.
// Falls back to the "other" category for unknown sport types.
func CategoryForSport(sportType string) Category {
	d, ok := registry[sportType]
	if !ok {
		return categoryByKey["other"]
	}
	c, cOK := categoryByKey[d.CategoryKey]
	if !cOK {
		return categoryByKey["other"]
	}
	return c
}

// AllCategories returns all categories in display order.
func AllCategories() []Category {
	result := make([]Category, len(categories))
	copy(result, categories)
	return result
}

// AllSports returns all registered sport definitions, sorted alphabetically by SportType.
func AllSports() []SportDef {
	result := make([]SportDef, 0, len(registry))
	for _, d := range registry {
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SportType < result[j].SportType
	})
	return result
}

// SportsInCategory returns all sport definitions belonging to a category,
// sorted alphabetically by SportType.
func SportsInCategory(categoryKey string) []SportDef {
	var result []SportDef
	for _, d := range registry {
		if d.CategoryKey == categoryKey {
			result = append(result, d)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SportType < result[j].SportType
	})
	return result
}

// ---- Helpers ----

// titleCase converts a snake_case string to a human-readable title.
func titleCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
