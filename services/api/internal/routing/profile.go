package routing

import (
	"fmt"
	"math"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// ProfileConfig holds user-facing routing profile parameters.
// Nil fields inherit from the built-in profile defaults.
type ProfileConfig struct {
	Profile    string   `json:"profile"`              // trekking/hiking/mtb/gravel/road
	Name       string   `json:"name,omitempty"`
	SportTypes []string `json:"sport_types,omitempty"`

	// Elevation
	UphillCost        *float32 `json:"uphill_cost,omitempty"`
	DownhillCost      *float32 `json:"downhill_cost,omitempty"`
	UphillCutoffPct   *float32 `json:"uphill_cutoff_pct,omitempty"` // slope % (e.g. 1.5 = 1.5%)
	DownhillCutoffPct *float32 `json:"downhill_cutoff_pct,omitempty"`
	ElevPenaltyBuffer *float32 `json:"elev_penalty_buffer_m,omitempty"`
	ElevMaxBuffer     *float32 `json:"elev_max_buffer_m,omitempty"`
	ElevBufferReduce  *float32 `json:"elev_buffer_reduce,omitempty"`

	// Difficulty
	MaxSacScale   *int  `json:"max_sac_scale,omitempty"`   // 0-6
	SacPreferred  *int  `json:"sac_preferred,omitempty"`   // 0-6
	MaxMtbScale   *int  `json:"max_mtb_scale,omitempty"`   // 0-5
	MaxViaFerrata *int  `json:"max_via_ferrata,omitempty"` // 0-6
	AllowSteps    *bool `json:"allow_steps,omitempty"`
	AllowFerries  *bool `json:"allow_ferries,omitempty"`

	// Enrichment weights (0=ignore, 1=full BRouter-calibrated effect)
	ForestBonus    *float32 `json:"forest_bonus,omitempty"`
	NoisePenalty   *float32 `json:"noise_penalty,omitempty"`
	TrafficPenalty *float32 `json:"traffic_penalty,omitempty"`
	ProtectedBonus *float32 `json:"protected_bonus,omitempty"`

	// Route adherence
	CyclerouteBoost *float32 `json:"cycleroute_boost,omitempty"`
	HikerouteBoost  *float32 `json:"hikeroute_boost,omitempty"`

	// Behavior
	WetWeather      *bool    `json:"wet_weather,omitempty"`
	PathPreference  *float32 `json:"path_preference,omitempty"` // 0-30
	TurnCost        *float32 `json:"turn_cost,omitempty"`       // 0-150
	InitialCostRoad *float32 `json:"initial_cost_road,omitempty"`

	// A*
	Pass0Coefficient *float32 `json:"pass0_coefficient,omitempty"`

	// Snap
	SnapThresholdM *float32 `json:"snap_threshold_m,omitempty"` // 10-500, default 100

	// Highway cost overrides (nil = profile default)
	PathCost         *float32 `json:"path_cost,omitempty"`
	TrackCost        *float32 `json:"track_cost,omitempty"`
	TrackGrade1Cost  *float32 `json:"track_grade1_cost,omitempty"`
	TrackGrade2Cost  *float32 `json:"track_grade2_cost,omitempty"`
	TrackGrade3Cost  *float32 `json:"track_grade3_cost,omitempty"`
	TrackGrade4Cost  *float32 `json:"track_grade4_cost,omitempty"`
	TrackGrade5Cost  *float32 `json:"track_grade5_cost,omitempty"`
	FootwayCost      *float32 `json:"footway_cost,omitempty"`
	CyclewayCost     *float32 `json:"cycleway_cost,omitempty"`
	BridlewayCost    *float32 `json:"bridleway_cost,omitempty"`
	PedestrianCost   *float32 `json:"pedestrian_cost,omitempty"`
	ResidentialCost  *float32 `json:"residential_cost,omitempty"`
	ServiceCost      *float32 `json:"service_cost,omitempty"`
	UnclassifiedCost *float32 `json:"unclassified_cost,omitempty"`
	TertiaryCost     *float32 `json:"tertiary_cost,omitempty"`
	SecondaryCost    *float32 `json:"secondary_cost,omitempty"`
	PrimaryCost      *float32 `json:"primary_cost,omitempty"`
}

// BuiltinProfile returns the built-in defaults for the given profile name.
func BuiltinProfile(name string) (ProfileConfig, error) {
	m := loadedBuiltins.Load()
	if m == nil {
		return ProfileConfig{}, fmt.Errorf("profiles not loaded (call LoadBuiltinProfiles first)")
	}
	p, ok := (*m)[name]
	if !ok {
		return ProfileConfig{}, fmt.Errorf("unknown profile: %q", name)
	}
	return p, nil
}

// isBikeProfile returns true for bike-oriented profiles.
func isBikeProfile(profile string) bool {
	return profile == "trekking" || profile == "mtb" || profile == "gravel" || profile == "road"
}

// BuildCostConfig merges user overrides into a built-in profile and produces a CostConfig.
func BuildCostConfig(p ProfileConfig) (*CostConfig, error) {
	base, err := BuiltinProfile(p.Profile)
	if err != nil {
		return nil, err
	}

	merged := MergeProfile(base, p)

	cfg := &CostConfig{
		UphillCost:        deref32(merged.UphillCost),
		DownhillCost:      deref32(merged.DownhillCost),
		UphillCutoff:      deref32(merged.UphillCutoffPct) / 100.0,
		DownhillCutoff:    deref32(merged.DownhillCutoffPct) / 100.0,
		ElevPenaltyBuffer: deref32(merged.ElevPenaltyBuffer),
		ElevMaxBuffer:     deref32(merged.ElevMaxBuffer),
		ElevBufferReduce:  deref32(merged.ElevBufferReduce),
		TurnCost:          deref32(merged.TurnCost),
		Pass0Coefficient:  deref32(merged.Pass0Coefficient),
	}

	cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCost, cfg.DownhillCost)
	cfg.EdgeCost = buildEdgeCostClosure(merged)
	setAvgCostPerKm(cfg, p.Profile)

	if merged.SnapThresholdM != nil {
		cfg.SnapThresholdM = *merged.SnapThresholdM
	} else {
		cfg.SnapThresholdM = 100 // default
	}

	cfg.EdgeCostRaw = cfg.EdgeCost

	return cfg, nil
}

// DefaultCostConfig returns a CostConfig for the default trekking profile.
func DefaultCostConfig() *CostConfig {
	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "trekking"})
	return cfg
}

// --- Merge logic ---

func MergeProfile(base, override ProfileConfig) ProfileConfig {
	m := base
	m.Profile = base.Profile // profile name is always from base

	mergePtr(&m.UphillCost, override.UphillCost)
	mergePtr(&m.DownhillCost, override.DownhillCost)
	mergePtr(&m.UphillCutoffPct, override.UphillCutoffPct)
	mergePtr(&m.DownhillCutoffPct, override.DownhillCutoffPct)
	mergePtr(&m.ElevPenaltyBuffer, override.ElevPenaltyBuffer)
	mergePtr(&m.ElevMaxBuffer, override.ElevMaxBuffer)
	mergePtr(&m.ElevBufferReduce, override.ElevBufferReduce)
	mergePtrInt(&m.MaxSacScale, override.MaxSacScale)
	mergePtrInt(&m.SacPreferred, override.SacPreferred)
	mergePtrInt(&m.MaxMtbScale, override.MaxMtbScale)
	mergePtrInt(&m.MaxViaFerrata, override.MaxViaFerrata)
	mergePtrBool(&m.AllowSteps, override.AllowSteps)
	mergePtrBool(&m.AllowFerries, override.AllowFerries)
	mergePtr(&m.ForestBonus, override.ForestBonus)
	mergePtr(&m.NoisePenalty, override.NoisePenalty)
	mergePtr(&m.TrafficPenalty, override.TrafficPenalty)
	mergePtr(&m.ProtectedBonus, override.ProtectedBonus)
	mergePtr(&m.CyclerouteBoost, override.CyclerouteBoost)
	mergePtr(&m.HikerouteBoost, override.HikerouteBoost)
	mergePtrBool(&m.WetWeather, override.WetWeather)
	mergePtr(&m.PathPreference, override.PathPreference)
	mergePtr(&m.TurnCost, override.TurnCost)
	mergePtr(&m.InitialCostRoad, override.InitialCostRoad)
	mergePtr(&m.Pass0Coefficient, override.Pass0Coefficient)
	mergePtr(&m.SnapThresholdM, override.SnapThresholdM)
	mergePtr(&m.PathCost, override.PathCost)
	mergePtr(&m.TrackCost, override.TrackCost)
	mergePtr(&m.TrackGrade1Cost, override.TrackGrade1Cost)
	mergePtr(&m.TrackGrade2Cost, override.TrackGrade2Cost)
	mergePtr(&m.TrackGrade3Cost, override.TrackGrade3Cost)
	mergePtr(&m.TrackGrade4Cost, override.TrackGrade4Cost)
	mergePtr(&m.TrackGrade5Cost, override.TrackGrade5Cost)
	mergePtr(&m.FootwayCost, override.FootwayCost)
	mergePtr(&m.CyclewayCost, override.CyclewayCost)
	mergePtr(&m.BridlewayCost, override.BridlewayCost)
	mergePtr(&m.PedestrianCost, override.PedestrianCost)
	mergePtr(&m.ResidentialCost, override.ResidentialCost)
	mergePtr(&m.ServiceCost, override.ServiceCost)
	mergePtr(&m.UnclassifiedCost, override.UnclassifiedCost)
	mergePtr(&m.TertiaryCost, override.TertiaryCost)
	mergePtr(&m.SecondaryCost, override.SecondaryCost)
	mergePtr(&m.PrimaryCost, override.PrimaryCost)
	return m
}

func mergePtr(dst **float32, src *float32) {
	if src != nil {
		*dst = src
	}
}

func mergePtrInt(dst **int, src *int) {
	if src != nil {
		*dst = src
	}
}

func mergePtrBool(dst **bool, src *bool) {
	if src != nil {
		*dst = src
	}
}

func deref32(p *float32) float32 {
	if p == nil {
		return 0
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

// --- EdgeCost closure builder ---

func buildEdgeCostClosure(p ProfileConfig) func(tags1, tags2 uint64) EdgeCostFactors {
	profile := p.Profile
	bike := isBikeProfile(profile)

	// Precompute scalar parameters
	maxSac := derefInt(p.MaxSacScale)
	sacPref := derefInt(p.SacPreferred)
	maxMtb := derefInt(p.MaxMtbScale)
	maxVF := derefInt(p.MaxViaFerrata)
	allowSteps := derefBool(p.AllowSteps)
	allowFerries := derefBool(p.AllowFerries)
	_ = allowFerries // reserved for future use

	forestWeight := deref32(p.ForestBonus)
	noiseWeight := deref32(p.NoisePenalty)
	trafficWeight := deref32(p.TrafficPenalty)
	protectedWeight := deref32(p.ProtectedBonus)


	cyclerouteBoost := deref32(p.CyclerouteBoost)
	hikerouteBoost := deref32(p.HikerouteBoost)

	wetWeather := derefBool(p.WetWeather)

	// Pre-compute highway costs: [32]float32 indexed by highway enum (5-bit safe)
	var hwCosts [32]float32
	for hw := range 32 {
		hwCosts[hw] = profileHighwayCostDefault(profile, uint8(hw), 0)
	}

	// Track grades: separate array for tracktype-dependent costs
	var trackGradeCosts [6]float32
	for tt := range 6 {
		trackGradeCosts[tt] = profileHighwayCostDefault(profile, 15, uint8(tt))
	}

	// Apply user overrides with clamp [0.5, 50.0].
	// Skip override if default is costBlocked (permanently blocked type — safety).
	clampOverride := func(p *float32, indices ...int) {
		if p == nil {
			return
		}
		for _, i := range indices {
			if hwCosts[i] >= costBlocked {
				continue
			}
			hwCosts[i] = max(0.5, min(*p, 50.0))
		}
	}
	clampOverride(p.PathCost, 16)
	clampOverride(p.FootwayCost, 17)
	clampOverride(p.CyclewayCost, 18)
	clampOverride(p.BridlewayCost, 19)
	clampOverride(p.PedestrianCost, 21)
	clampOverride(p.ResidentialCost, 11, 12)
	clampOverride(p.ServiceCost, 14)
	clampOverride(p.UnclassifiedCost, 13)
	clampOverride(p.TertiaryCost, 9, 10)
	clampOverride(p.SecondaryCost, 7, 8)
	clampOverride(p.PrimaryCost, 5, 6)

	// Track overrides: TrackCost sets ONLY unknown grade (index 0)
	clampTrack := func(p *float32, idx int) {
		if p == nil || trackGradeCosts[idx] >= costBlocked {
			return
		}
		trackGradeCosts[idx] = max(0.5, min(*p, 50.0))
	}
	clampTrack(p.TrackCost, 0)
	clampTrack(p.TrackGrade1Cost, 1)
	clampTrack(p.TrackGrade2Cost, 2)
	clampTrack(p.TrackGrade3Cost, 3)
	clampTrack(p.TrackGrade4Cost, 4)
	clampTrack(p.TrackGrade5Cost, 5)

	// Road profile: isPaved blocking stays in the closure (not configurable)
	roadIsPavedBlock := profile == "road"

	// Precompute SAC penalty table (indices 0-6; 0 = untagged)
	var sacPenalty [7]float32
	for s := range 7 {
		if s == sacPref {
			sacPenalty[s] = 0
			continue
		}
		if maxSac > 0 && s > maxSac {
			sacPenalty[s] = 9000
			continue
		}
		diff := s - sacPref
		if diff < 0 {
			sacPenalty[s] = float32(math.Pow(1.05, float64(-diff)) - 1)
		} else {
			sacPenalty[s] = float32(math.Pow(1.6, float64(diff)) - 1)
		}
	}

	return func(tags1, tags2 uint64) EdgeCostFactors {
		hw := graph.DecodeHighway(tags1)
		sf := graph.DecodeSurface(tags1)
		access := graph.DecodeAccess(tags1)

		// --- Vehicle-oneway check for reverse edges ---
		// Reverse edges on vehicle-oneway streets exist so foot profiles can
		// traverse against traffic. Bike profiles respect oneway_bicycle.
		if tags1&(1<<61) != 0 { // reverse edge
			if tags1&(1<<36) != 0 { // oneway=yes on this way
				if bike {
					if tags1&(1<<37) != 0 { // oneway_bicycle=true
						return EdgeCostFactors{Flat: costBlocked}
					}
				}
				// foot/hiking: allowed — no penalty
			}
		}

		// --- Access checks ---
		if access == 3 { // private
			return EdgeCostFactors{Flat: costBlocked}
		}
		if access == 2 { // no
			if bike {
				return EdgeCostFactors{Flat: costBlocked}
			}
			// foot profiles: possible but not recommended
		}

		// --- Highway blocking ---
		// Block motorway/trunk (1-4) and unknown (0)
		if hw <= 4 || hw == 0 {
			return EdgeCostFactors{Flat: costBlocked}
		}

		// Steps
		if hw == 20 && !allowSteps { // steps
			return EdgeCostFactors{Flat: costBlocked}
		}

		// Via ferrata
		if hw == 22 { // via_ferrata
			vfScale := graph.DecodeViaFerrataScale(tags2)
			if maxVF == 0 && vfScale > 0 {
				return EdgeCostFactors{Flat: costBlocked}
			}
			if vfScale > 0 && int(vfScale) > maxVF {
				return EdgeCostFactors{Flat: costBlocked}
			}
		}

		// --- SAC scale check ---
		sacScale := uint8(graph.DecodeField(tags1, 14, 3)) // bits 14-16
		if maxSac > 0 && sacScale > 0 && int(sacScale) > maxSac {
			return EdgeCostFactors{Flat: costBlocked}
		}

		// --- MTB scale check ---
		mtbScale := uint8(graph.DecodeField(tags1, 17, 3))          // bits 17-19
		if maxMtb > 0 && mtbScale > 0 && int(mtbScale) > maxMtb+1 { // enum is 1-based (0=1, 1=2, ...)
			return EdgeCostFactors{Flat: costBlocked}
		}

		// --- Tracktype ---
		tracktype := uint8(graph.DecodeField(tags1, 11, 3)) // bits 11-13

		// Road-profile: heavy penalty for unpaved residential/service/unclassified/track.
		// Previously costBlocked, but that disconnects graph components in rural areas
		// where paved roads are only reachable through unpaved connectors.
		// sf==0 (unknown surface) passes through — many OSM roads lack surface tags,
		// cost 4.0 is prohibitive enough to avoid them unless no alternative exists.
		if roadIsPavedBlock {
			isPaved := sf >= 1 && sf <= 7
			if !isPaved && sf != 0 {
				switch hw {
				case 11, 12, 13, 14, 15:
					return EdgeCostFactors{Flat: 15.0}
				}
			}
		}

		// Pre-computed highway cost lookup (O(1), replaces profileHighwayCost switch)
		var hwCF float32
		if hw == 15 && tracktype <= 5 {
			hwCF = trackGradeCosts[tracktype]
		} else if int(hw) < len(hwCosts) {
			hwCF = hwCosts[hw]
		} else {
			hwCF = 2.0 // unknown highway type fallback
		}
		if hwCF >= costBlocked {
			return EdgeCostFactors{Flat: costBlocked}
		}
		sfCF := profileSurfaceCost(profile, sf)

		var penalty float32

		// Access penalty
		if access == 2 { // no (foot profile only — bike already blocked above)
			penalty += 10
		}
		if access == 5 { // destination
			penalty += 1.0
		}

		// Foot-only access for bike profiles
		if bike {
			bicycle := uint8(graph.DecodeField(tags1, 27, 3))
			foot := uint8(graph.DecodeField(tags1, 30, 3))
			if bicycle == 2 && (foot == 1 || foot == 7) { // no bicycle, foot=yes/designated
				penalty += 4.0
			}
		}

		// SAC penalty (additive)
		if sacScale > 0 && int(sacScale) <= 6 {
			penalty += sacPenalty[sacScale]
		} else {
			// Untagged ways: use sacPenalty[0]
			penalty += sacPenalty[0]
		}

		// Oneway reverse penalty (bike profiles)
		if bike && graph.DecodeBool(tags1, 36) && graph.DecodeIsReverse(tags1) {
			switch hw {
			case 5, 6: // primary
				penalty += 50
			case 7, 8: // secondary
				penalty += 30
			case 9, 10: // tertiary
				penalty += 20
			default:
				penalty += 4
			}
		}

		// Wet weather surface penalty
		var surfacePenalty float32
		if wetWeather {
			switch sf {
			case 12, 13, 16: // dirt, earth, ground
				surfacePenalty += 1.0
			case 17: // mud
				surfacePenalty += 2.0
			case 14: // grass
				surfacePenalty += 0.5
			case 18: // sand
				surfacePenalty += 0.5
			}
		}

		baseCF := hwCF*sfCF + penalty + surfacePenalty

		// --- Enrichment (additive, from EdgeTags2) ---
		var forestAdd, noiseAdd, trafficAdd, protectedSub float32
		if forestWeight > 0 {
			forestPct := graph.DecodeForestPct(tags2)
			if forestPct > 0 { // 0 = no data (don't penalize unknown)
				forestAdd = (1.0 - float32(forestPct)/100.0) * forestWeight // 1% → +1.0, 100% → +0.0
			}
		}
		if noiseWeight > 0 {
			noiseScore := graph.DecodeNoiseScore(tags2)
			if noiseScore > 0 { // 0 = no data
				noiseAdd = float32(noiseScore) / 100.0 * noiseWeight * 2.0 // max +2.0
			}
		}
		if trafficWeight > 0 {
			trafficClass := graph.DecodeTrafficClass(tags2)
			if trafficClass > 0 { // 0 = no data
				trafficAdd = float32(trafficClass) / 6.0 * trafficWeight // max +1.0 (6 classes now, was 3)
			}
		}
		if protectedWeight > 0 {
			protectedArea := graph.DecodeProtectedArea(tags2)
			if protectedArea > 0 { // 0 = no data
				protectedSub = float32(protectedArea) / 3.0 * protectedWeight * 0.5 // max -0.5
			}
		}

		// Cycleroute boost (LERP towards 1.0)
		if cyclerouteBoost > 0 && graph.DecodeIsCycleroute(tags2) {
			baseCF = baseCF*(1.0-cyclerouteBoost) + 1.0*cyclerouteBoost
		}
		// Hikeroute boost (LERP towards 1.0)
		if hikerouteBoost > 0 && graph.DecodeIsHikeroute(tags2) {
			baseCF = baseCF*(1.0-hikerouteBoost) + 1.0*hikerouteBoost
		}

		finalCF := baseCF + forestAdd + noiseAdd + trafficAdd - protectedSub

		// --- 3 Costfactors for hiking/mtb ---
		if profile == "hiking" || profile == "mtb" {
			upMod := surfaceUphillMod(sf, wetWeather)
			downMod := surfaceDownhillMod(sf, wetWeather)
			return EdgeCostFactors{
				Flat:     max(1.0, finalCF),
				Up:       max(1.0, finalCF+upMod),
				Down:     max(1.0, finalCF+downMod),
				WayClass: 0,
			}
		}

		return EdgeCostFactors{Flat: max(1.0, finalCF), WayClass: 0}
	}
}

// --- Profile-specific highway costs ---

// profileHighwayCostDefault returns the built-in highway cost factor for a
// profile/highway/tracktype combination. Called once per closure build (not hot path).
// Road profile's isPaved logic is handled separately in the closure.
func profileHighwayCostDefault(profile string, hw, tracktype uint8) float32 {
	switch profile {
	case "hiking":
		return hikingHighwayCost(hw)
	case "mtb":
		return mtbHighwayCost(hw)
	case "trekking":
		return trekkingHighwayCost(hw, tracktype)
	case "gravel":
		return gravelHighwayCost(hw, tracktype)
	case "road":
		return roadHighwayCostDefault(hw)
	default:
		return hikingHighwayCost(hw)
	}
}

// roadHighwayCostDefault returns the paved-surface cost for Road profile.
// The isPaved blocking logic is handled at runtime in the closure, not here.
func roadHighwayCostDefault(hw uint8) float32 {
	switch hw {
	case 18:
		return 1.0 // cycleway
	case 9, 10:
		return 1.1 // tertiary
	case 7, 8:
		return 1.2 // secondary
	case 11, 12:
		return 1.3 // residential (paved)
	case 13:
		return 1.4 // unclassified (paved)
	case 5, 6:
		return 1.5 // primary
	case 14:
		return 1.8 // service (paved)
	case 21:
		return 8.0 // pedestrian
	case 15:
		return 4.0 // track (paved only)
	case 16:
		return costBlocked // path
	case 17:
		return costBlocked // footway
	case 19:
		return costBlocked // bridleway
	case 20:
		return costBlocked // steps
	case 22:
		return costBlocked // via_ferrata
	default:
		return 8.0
	}
}

func hikingHighwayCost(hw uint8) float32 {
	// Hikers prefer paths/tracks (T1-T2). Roads and cycleways are unpleasant
	// due to traffic, noise, and hard surfaces. BRouter hiking profiles use
	// similar ratios (path=1.0, cycleway~1.5, primary~4.0).
	switch hw {
	case 16: // path
		return 1.0
	case 15: // track
		return 1.0
	case 17: // footway
		return 1.0
	case 20: // steps
		return 1.1
	case 21: // pedestrian
		return 1.0
	case 22: // via_ferrata
		return 1.2
	case 19: // bridleway
		return 1.2
	case 13: // unclassified
		return 1.5
	case 14: // service
		return 1.5
	case 18: // cycleway
		return 1.8
	case 11, 12: // residential, living_street
		return 2.0
	case 9, 10: // tertiary
		return 2.5
	case 7, 8: // secondary
		return 3.5
	case 5, 6: // primary
		return 5.0
	default:
		return 2.0
	}
}

func mtbHighwayCost(hw uint8) float32 {
	switch hw {
	case 16: // path — Singletrail: MTB-Paradies, aktiv bevorzugen
		return 0.8
	case 15: // track — Forstweg: gut, aber path bevorzugt
		return 0.9
	case 17: // footway — akzeptabel, aber kein Ziel
		return 1.3
	case 19: // bridleway — oft alpine Trails, nicht blockieren
		return 1.4
	case 14: // service — Zufahrtswege: nur Durchfahrt
		return 1.4
	case 13: // unclassified — Nebenstraße: besser als Hauptstraßen
		return 1.3
	case 11, 12: // residential, living_street — Wohnstraße: meiden
		return 1.5
	case 18: // cycleway — asphaltierter Radweg: für MTBler langweilig
		return 1.6
	case 9, 10: // tertiary — Dorfstraße: meiden
		return 1.5
	case 7, 8: // secondary — stark meiden
		return 2.0
	case 5, 6: // primary — sehr stark meiden
		return 3.0
	case 21: // pedestrian
		return 2.0
	case 20: // steps
		return costBlocked
	case 22: // via_ferrata
		return costBlocked
	default:
		return 1.5
	}
}

// trekkingHighwayCost berechnet den Highway-Kostenfaktor für Trekking/Tourenrad.
// Trekking-Nutzer bevorzugen Radwege und ruhige Straßen für lange Strecken.
// Stark befahrene Hauptstraßen und rauhe Pfade werden gemieden.
func trekkingHighwayCost(hw, tracktype uint8) float32 {
	switch hw {
	case 18: // cycleway — bevorzugt, sicher und komfortabel
		return 1.0
	case 15: // track (grade-dependent)
		switch tracktype {
		case 1: // grade1 — befestigter Forstweg, ideal
			return 1.0
		case 2: // grade2 — guter Schotterweg, noch OK
			return 1.5
		case 3: // grade3 — rauer Weg, grenzwertig für Tourenrad
			return 2.5
		case 4: // grade4 — sehr rau, kaum fahrbar
			return 4.0
		case 5: // grade5 — unbefahrbar für Tourenrad
			return 5.0
		default: // unknown — wie grade2 annehmen
			return 1.5
		}
	case 16: // path — zu schmal/rau für Tourenrad
		return 3.5
	case 17: // footway — Fußweg, Tourenrad ungeeignet
		return 3.0
	case 11, 12: // residential, living_street — ruhig, angenehm
		return 1.1
	case 14: // service — Zufahrtsweg, akzeptabel
		return 1.3
	case 13: // unclassified — ruhige Straße, gut geeignet
		return 1.2
	case 9, 10: // tertiary — etwas Verkehr, noch tolerierbar
		return 1.4
	case 7, 8: // secondary — mehr Verkehr, ungemütlich
		return 2.0
	case 5, 6: // primary — stark befahren, meiden
		return 4.0
	case 19: // bridleway — Reitweg, kein Radweg
		return 5.0
	case 20: // steps — nicht fahrbar
		return 40
	case 21: // pedestrian — Fußgängerzone
		return 2.0
	case 22: // via_ferrata — gesperrt
		return costBlocked
	default:
		return 1.5
	}
}

func gravelHighwayCost(hw, tracktype uint8) float32 {
	// Gravel riders prefer unpaved tracks and quiet roads.
	// Schotter/Forstwege (track grade1-3) are ideal terrain.
	// Busy roads (primary/secondary) are unpleasant but not impossible.
	switch hw {
	case 15: // track (grade-dependent) — core gravel terrain
		switch tracktype {
		case 1: // grade1: solid surface, fast gravel
			return 1.0
		case 2: // grade2: gravel/dirt, excellent gravel terrain
			return 1.1
		case 3: // grade3: rougher surface, still good for gravel
			return 1.5
		case 4: // grade4: very rough, challenging but possible
			return 2.5
		case 5: // grade5: extreme terrain, mostly unrideable
			return 4.0
		default: // unknown tracktype — assume grade2-quality
			return 1.1
		}
	case 18: // cycleway — paved but fine
		return 1.0
	case 13: // unclassified — quiet backroads, good gravel territory
		return 1.1
	case 11, 12: // residential, living_street — low traffic, acceptable
		return 1.3
	case 14: // service
		return 1.4
	case 16: // path — often singletrack, narrower than track
		return 1.6
	case 9, 10: // tertiary — moderate traffic, acceptable for connecting
		return 1.8
	case 7, 8: // secondary — more traffic, avoid if possible
		return 2.8
	case 5, 6: // primary — high traffic, dangerous, strongly avoid
		return 4.5
	case 17: // footway — not ideal but passable
		return 2.5
	case 19: // bridleway — soft surface, often too soft
		return 2.5
	case 20: // steps
		return costBlocked
	case 22: // via_ferrata
		return costBlocked
	case 21: // pedestrian
		return 2.5
	default:
		return 2.0
	}
}

// --- Profile-specific surface costs ---

func profileSurfaceCost(profile string, sf uint8) float32 {
	switch profile {
	case "road":
		return roadSurfaceCost(sf)
	case "gravel":
		return gravelSurfaceCost(sf)
	default: // hiking, mtb, trekking — outdoor
		return outdoorSurfaceCost(sf)
	}
}

func outdoorSurfaceCost(sf uint8) float32 {
	switch sf {
	case 0: // unknown
		return 1.0
	case 8, 9: // compacted, fine_gravel
		return 1.0
	case 12, 13, 16: // dirt, earth, ground
		return 1.0
	case 1, 2: // asphalt, concrete
		return 1.1
	case 10: // gravel
		return 1.1
	case 14: // grass
		return 1.1
	case 3: // paving_stones
		return 1.1
	case 17: // mud
		return 1.8
	case 18: // sand
		return 1.5
	case 22, 23: // rock, stone
		return 1.2
	case 20: // snow
		return 2.5
	case 21: // ice
		return 4.0
	default:
		return 1.1
	}
}

func roadSurfaceCost(sf uint8) float32 {
	switch sf {
	case 0: // unknown — auf Straßen meist Asphalt
		return 1.0
	case 1: // asphalt — ideal für Rennrad
		return 1.0
	case 2: // concrete — gut befahrbar
		return 1.05
	case 3: // paving_stones — holprig aber OK
		return 1.3
	case 4, 5: // sett, cobblestone — sehr holprig, Pannengefahr
		return 2.5
	case 6: // metal — rutschig bei Nässe
		return 1.5
	case 7: // wood — rutschig, selten
		return 1.8
	case 8: // compacted — fest, passierbar
		return 1.5
	case 9: // fine_gravel — riskant für Rennrad
		return 2.0
	case 10: // gravel — Pannengefahr, stark vermeiden
		return 4.0
	case 12, 13, 16: // dirt, earth, ground — unfahrbar
		return costBlocked
	case 17: // mud
		return costBlocked
	case 14: // grass
		return costBlocked
	case 18: // sand
		return costBlocked
	case 20: // snow
		return costBlocked
	case 21: // ice
		return costBlocked
	default:
		return 1.1 // unbekannte Oberfläche: auf Straßen meist befestigt
	}
}

func gravelSurfaceCost(sf uint8) float32 {
	// Gravel bikes are optimized for gravel/compacted surfaces.
	// Asphalt is slightly less preferred (hard, vibration), but fine.
	// Soft surfaces (dirt, mud) are harder and slower.
	switch sf {
	case 0: // unknown
		return 1.0
	case 8, 9: // compacted, fine_gravel — ideal gravel surface
		return 1.0
	case 10: // gravel — core terrain for gravel bikes
		return 1.0
	case 1, 2: // asphalt, concrete — smooth but not gravel's sweet spot
		return 1.1
	case 3: // paving_stones
		return 1.2
	case 4: // sett/cobblestone — rough paved
		return 1.4
	case 12, 13, 16: // dirt, earth, ground — soft, slower
		return 1.6
	case 14: // grass — soft, slow
		return 2.0
	case 17: // mud
		return 3.0
	case 18: // sand
		return 3.0
	case 20: // snow
		return 5.0
	case 21: // ice
		return 5.0
	default:
		return 1.3
	}
}

// --- Surface slope modifiers (hiking/mtb 3CF) ---

func surfaceUphillMod(sf uint8, wet bool) float32 {
	base := float32(0)
	switch sf {
	case 1, 2, 3: // paved
		base = -0.1
	case 10: // gravel
		base = 0.2
	case 17: // mud
		base = 0.3
	case 14: // grass
		base = 0.1
	case 18: // sand
		base = 0.3
	case 22, 23: // rock, stone
		base = 0.1
	case 20: // snow
		base = 0.4
	case 21: // ice
		base = 0.5
	}
	if wet {
		switch sf {
		case 12, 13, 16: // dirt, earth, ground
			base += 0.1
		case 17: // mud
			base += 0.2
		case 14: // grass
			base += 0.1
		}
	}
	return base
}

func surfaceDownhillMod(sf uint8, wet bool) float32 {
	base := float32(0)
	switch sf {
	case 1, 2, 3: // paved
		base = 0.2
	case 10: // gravel
		base = 0.3
	case 14: // grass
		base = -0.2
	case 17: // mud
		base = 0.5
	case 18: // sand
		base = 0.2
	case 22, 23: // rock, stone
		base = 0.3
	case 20: // snow
		base = 0.3
	case 21: // ice
		base = 0.8
	}
	if wet {
		switch sf {
		case 12, 13, 16: // dirt, earth, ground
			base += 0.2
		case 17: // mud
			base += 0.3
		case 14: // grass
			base += 0.1
		case 22, 23: // rock
			base += 0.1
		}
	}
	return base
}

// BuildLandmarkEdgeCostClosure returns an edge-cost closure suitable for landmark
// preprocessing. It uses the hiking profile as base (most universal for outdoor
// routing) with ALL enrichment/preference weights zeroed out so that landmark
// distances represent pure infrastructure + elevation cost — a guaranteed lower
// bound for any real profile that adds enrichment penalties on top.
func BuildLandmarkEdgeCostClosure() func(tags1, tags2 uint64) EdgeCostFactors {
	p := ProfileConfig{
		Profile:           "hiking",
		UphillCost:        new(float32(0)), // overridden per LandmarkCostConfig
		DownhillCost:      new(float32(0)), // overridden per LandmarkCostConfig
		UphillCutoffPct:   new(float32(3)),
		DownhillCutoffPct: new(float32(3)),
		ElevPenaltyBuffer: new(float32(5)),
		ElevMaxBuffer:     new(float32(10)),
		ElevBufferReduce:  new(float32(0.01)),
		MaxSacScale:       new(6), // allow everything
		SacPreferred:      new(0), // no preference penalty
		MaxMtbScale:       new(5), // allow everything
		MaxViaFerrata:     new(6), // allow everything
		AllowSteps:        new(true),
		AllowFerries:      new(true),
		// ALL enrichment weights = 0 (lower bound guarantee)
		ForestBonus:     new(float32(0)),
		NoisePenalty:    new(float32(0)),
		TrafficPenalty:  new(float32(0)),
		ProtectedBonus:  new(float32(0)),
		CyclerouteBoost: new(float32(0)),
		HikerouteBoost:  new(float32(0)),
		WetWeather:      new(false),
		PathPreference:  new(float32(0)),
		TurnCost:        new(float32(0)),
		InitialCostRoad: new(float32(0)),
	}
	return buildEdgeCostClosure(p)
}

// setAvgCostPerKm sets the calibrated average cost per km for isochrone budget calculation.
// Values are empirical estimates accounting for typical costfactors, elevation, and detour.
func setAvgCostPerKm(cfg *CostConfig, profileName string) {
	switch profileName {
	case "hiking":
		cfg.AvgCostPerKm = 1800
	case "mtb":
		cfg.AvgCostPerKm = 1700
	case "gravel":
		cfg.AvgCostPerKm = 1500
	case "trekking":
		cfg.AvgCostPerKm = 1400
	case "road":
		cfg.AvgCostPerKm = 1100
	default:
		cfg.AvgCostPerKm = 1800 // hiking fallback
	}
}

// ProfileDefaultsResponse is the JSON response for GET /profiles/{name}.
// Returns ALL profile defaults as single source of truth for the frontend.
// Uses pointer fields WITHOUT omitempty so null values are serialized.
type ProfileDefaultsResponse struct {
	Profile           string   `json:"profile"`
	UphillCost        *float32 `json:"uphill_cost"`
	DownhillCost      *float32 `json:"downhill_cost"`
	UphillCutoffPct   *float32 `json:"uphill_cutoff_pct"`
	DownhillCutoffPct *float32 `json:"downhill_cutoff_pct"`
	ElevPenaltyBuffer *float32 `json:"elev_penalty_buffer_m"`
	ElevMaxBuffer     *float32 `json:"elev_max_buffer_m"`
	ElevBufferReduce  *float32 `json:"elev_buffer_reduce"`
	TurnCost          *float32 `json:"turn_cost"`
	Pass0Coefficient  *float32 `json:"pass0_coefficient"`
	// Difficulty
	MaxSacScale   *int  `json:"max_sac_scale"`
	SacPreferred  *int  `json:"sac_preferred"`
	MaxMtbScale   *int  `json:"max_mtb_scale"`
	MaxViaFerrata *int  `json:"max_via_ferrata"`
	AllowSteps    *bool `json:"allow_steps"`
	AllowFerries  *bool `json:"allow_ferries"`
	// Enrichment weights
	ForestBonus    *float32 `json:"forest_bonus"`
	NoisePenalty   *float32 `json:"noise_penalty"`
	TrafficPenalty *float32 `json:"traffic_penalty"`
	ProtectedBonus *float32 `json:"protected_bonus"`
	// Route adherence
	CyclerouteBoost *float32 `json:"cycleroute_boost"`
	HikerouteBoost  *float32 `json:"hikeroute_boost"`
	// Behavior
	WetWeather      *bool    `json:"wet_weather"`
	PathPreference  *float32 `json:"path_preference"`
	InitialCostRoad *float32 `json:"initial_cost_road"`
	// Snap
	SnapThresholdM *float32 `json:"snap_threshold_m"`
	// Highway costs — null means permanently blocked for this profile
	PathCost         *float32 `json:"path_cost"`
	TrackCost        *float32 `json:"track_cost"`
	TrackGrade1Cost  *float32 `json:"track_grade1_cost"`
	TrackGrade2Cost  *float32 `json:"track_grade2_cost"`
	TrackGrade3Cost  *float32 `json:"track_grade3_cost"`
	TrackGrade4Cost  *float32 `json:"track_grade4_cost"`
	TrackGrade5Cost  *float32 `json:"track_grade5_cost"`
	FootwayCost      *float32 `json:"footway_cost"`
	CyclewayCost     *float32 `json:"cycleway_cost"`
	BridlewayCost    *float32 `json:"bridleway_cost"`
	PedestrianCost   *float32 `json:"pedestrian_cost"`
	ResidentialCost  *float32 `json:"residential_cost"`
	ServiceCost      *float32 `json:"service_cost"`
	UnclassifiedCost *float32 `json:"unclassified_cost"`
	TertiaryCost     *float32 `json:"tertiary_cost"`
	SecondaryCost    *float32 `json:"secondary_cost"`
	PrimaryCost      *float32 `json:"primary_cost"`
}

// ProfileDefaults returns the full defaults for a profile.
// Blocked highway types have nil cost (serialized as null).
func ProfileDefaults(name string) (*ProfileDefaultsResponse, error) {
	base, err := BuiltinProfile(name)
	if err != nil {
		return nil, err
	}

	r := &ProfileDefaultsResponse{
		Profile:           name,
		UphillCost:        base.UphillCost,
		DownhillCost:      base.DownhillCost,
		UphillCutoffPct:   base.UphillCutoffPct,
		DownhillCutoffPct: base.DownhillCutoffPct,
		ElevPenaltyBuffer: base.ElevPenaltyBuffer,
		ElevMaxBuffer:     base.ElevMaxBuffer,
		ElevBufferReduce:  base.ElevBufferReduce,
		TurnCost:          base.TurnCost,
		Pass0Coefficient:  base.Pass0Coefficient,
		// Difficulty
		MaxSacScale:   base.MaxSacScale,
		SacPreferred:  base.SacPreferred,
		MaxMtbScale:   base.MaxMtbScale,
		MaxViaFerrata: base.MaxViaFerrata,
		AllowSteps:    base.AllowSteps,
		AllowFerries:  base.AllowFerries,
		// Enrichment
		ForestBonus:    base.ForestBonus,
		NoisePenalty:   base.NoisePenalty,
		TrafficPenalty: base.TrafficPenalty,
		ProtectedBonus: base.ProtectedBonus,
		// Route adherence
		CyclerouteBoost: base.CyclerouteBoost,
		HikerouteBoost:  base.HikerouteBoost,
		// Behavior
		WetWeather:      base.WetWeather,
		PathPreference:  base.PathPreference,
		InitialCostRoad: base.InitialCostRoad,
		SnapThresholdM:  base.SnapThresholdM,
	}

	setIfPassable := func(dst **float32, cost float32) {
		if cost >= costBlocked {
			*dst = nil
		} else {
			*dst = new(cost)
		}
	}
	setIfPassable(&r.PathCost, profileHighwayCostDefault(name, 16, 0))
	setIfPassable(&r.TrackCost, profileHighwayCostDefault(name, 15, 0))
	setIfPassable(&r.TrackGrade1Cost, profileHighwayCostDefault(name, 15, 1))
	setIfPassable(&r.TrackGrade2Cost, profileHighwayCostDefault(name, 15, 2))
	setIfPassable(&r.TrackGrade3Cost, profileHighwayCostDefault(name, 15, 3))
	setIfPassable(&r.TrackGrade4Cost, profileHighwayCostDefault(name, 15, 4))
	setIfPassable(&r.TrackGrade5Cost, profileHighwayCostDefault(name, 15, 5))
	setIfPassable(&r.FootwayCost, profileHighwayCostDefault(name, 17, 0))
	setIfPassable(&r.CyclewayCost, profileHighwayCostDefault(name, 18, 0))
	setIfPassable(&r.BridlewayCost, profileHighwayCostDefault(name, 19, 0))
	setIfPassable(&r.PedestrianCost, profileHighwayCostDefault(name, 21, 0))
	setIfPassable(&r.ResidentialCost, profileHighwayCostDefault(name, 11, 0))
	setIfPassable(&r.ServiceCost, profileHighwayCostDefault(name, 14, 0))
	setIfPassable(&r.UnclassifiedCost, profileHighwayCostDefault(name, 13, 0))
	setIfPassable(&r.TertiaryCost, profileHighwayCostDefault(name, 9, 0))
	setIfPassable(&r.SecondaryCost, profileHighwayCostDefault(name, 7, 0))
	setIfPassable(&r.PrimaryCost, profileHighwayCostDefault(name, 5, 0))

	return r, nil
}

// ApplySurfaceTendency applies highway-cost multipliers for a surface tendency preset.
// Multipliers are relative to the profile's built-in defaults.
// Same package as profileHighwayCostDefault — direct access, no export needed.
func ApplySurfaceTendency(p *ProfileConfig, tendency string) {
	if tendency == "" || tendency == "mixed" {
		return
	}
	profile := p.Profile
	mul := func(target **float32, hw, tracktype uint8, m float32) {
		def := profileHighwayCostDefault(profile, hw, tracktype)
		if def >= costBlocked {
			return
		}
		v := max(float32(0.5), min(def*m, float32(50.0)))
		*target = &v
	}
	switch tendency {
	case "trails":
		mul(&p.PathCost, 16, 0, 0.5)
		mul(&p.TrackCost, 15, 0, 0.8)
		mul(&p.ResidentialCost, 11, 0, 2.0)
		mul(&p.TertiaryCost, 9, 0, 2.0)
	case "forest_roads":
		mul(&p.TrackCost, 15, 0, 0.5)
		mul(&p.TrackGrade1Cost, 15, 1, 0.6)
		mul(&p.PathCost, 16, 0, 1.5)
		mul(&p.ResidentialCost, 11, 0, 1.5)
	case "paved":
		mul(&p.ResidentialCost, 11, 0, 0.5)
		mul(&p.CyclewayCost, 18, 0, 0.5)
		mul(&p.PathCost, 16, 0, 3.0)
		mul(&p.TrackCost, 15, 0, 2.0)
	}
}
