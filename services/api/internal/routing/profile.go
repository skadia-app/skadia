package routing

import (
	"fmt"
	"math"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// ProfileConfig holds user-facing routing profile parameters.
// Nil fields inherit from the built-in profile defaults.
type ProfileConfig struct {
	Profile string `json:"profile"` // trekking/hiking/mtb/gravel/road

	// Elevation
	UphillCost        *float32 `json:"uphill_cost,omitempty"`
	DownhillCost      *float32 `json:"downhill_cost,omitempty"`
	UphillCutoffPct   *float32 `json:"uphill_cutoff_pct,omitempty"`    // slope % (e.g. 1.5 = 1.5%)
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
	PathPreference  *float32 `json:"path_preference,omitempty"`  // 0-30
	TurnCost        *float32 `json:"turn_cost,omitempty"`        // 0-150
	InitialCostRoad *float32 `json:"initial_cost_road,omitempty"`

	// A*
	Pass0Coefficient *float32 `json:"pass0_coefficient,omitempty"`
}

// --- Pointer helpers ---

func ptr32(v float32) *float32 { return &v }
func ptrInt(v int) *int        { return &v }
func ptrBool(v bool) *bool     { return &v }

// --- Built-in profiles ---

var builtinProfiles = map[string]ProfileConfig{
	"trekking": {
		Profile:            "trekking",
		UphillCost:         ptr32(0),
		DownhillCost:       ptr32(60),
		UphillCutoffPct:   ptr32(1.5),
		DownhillCutoffPct: ptr32(1.5),
		ElevPenaltyBuffer: ptr32(5),
		ElevMaxBuffer:     ptr32(10),
		ElevBufferReduce:  ptr32(0),
		AllowSteps:        ptrBool(true),
		AllowFerries:      ptrBool(true),
		CyclerouteBoost:   ptr32(0.6),
		TurnCost:          ptr32(90),
		Pass0Coefficient:  ptr32(3.0), // higher: downhillcost=60 inflates pass0 budget
	},
	"hiking": {
		Profile:           "hiking",
		UphillCost:        ptr32(7),
		DownhillCost:      ptr32(7),
		UphillCutoffPct:   ptr32(3),
		DownhillCutoffPct: ptr32(3),
		ElevPenaltyBuffer: ptr32(5),
		ElevMaxBuffer:     ptr32(10),
		ElevBufferReduce:  ptr32(0.01),
		MaxSacScale:       ptrInt(3),
		SacPreferred:      ptrInt(1),
		AllowSteps:        ptrBool(true),
		ForestBonus:       ptr32(0.5),
		NoisePenalty:      ptr32(0.4),
		TrafficPenalty:    ptr32(0.6),
		ProtectedBonus:    ptr32(0.3),
		HikerouteBoost:    ptr32(0.2),
		TurnCost:          ptr32(0),
		Pass0Coefficient:  ptr32(2.5), // higher: elevation costs inflate pass0 budget
	},
	"mtb": {
		Profile:           "mtb",
		UphillCost:        ptr32(0),
		DownhillCost:      ptr32(0),
		UphillCutoffPct:   ptr32(1.5),
		DownhillCutoffPct: ptr32(1.5),
		ElevPenaltyBuffer: ptr32(5),
		ElevMaxBuffer:     ptr32(10),
		ElevBufferReduce:  ptr32(0),
		MaxMtbScale:       ptrInt(2),
		AllowSteps:        ptrBool(false),
		PathPreference:    ptr32(20),
		ForestBonus:       ptr32(0.3),
		NoisePenalty:      ptr32(0.2),
		CyclerouteBoost:   ptr32(0.2),
		TurnCost:          ptr32(30),
		Pass0Coefficient:  ptr32(1.5),
	},
	"gravel": {
		Profile:           "gravel",
		UphillCost:        ptr32(80),
		DownhillCost:      ptr32(40),
		UphillCutoffPct:   ptr32(1.5),
		DownhillCutoffPct: ptr32(1.5),
		ElevPenaltyBuffer: ptr32(5),
		ElevMaxBuffer:     ptr32(10),
		ElevBufferReduce:  ptr32(0),
		MaxSacScale:       ptrInt(1),
		MaxMtbScale:       ptrInt(0),
		AllowSteps:        ptrBool(false),
		ForestBonus:       ptr32(0.3),
		NoisePenalty:      ptr32(0.4),
		TrafficPenalty:    ptr32(0.5),
		CyclerouteBoost:   ptr32(0.3),
		TurnCost:          ptr32(65),
		Pass0Coefficient:  ptr32(3.0), // higher: uphillcost=80 inflates pass0 budget
	},
	"road": {
		Profile:           "road",
		UphillCost:        ptr32(0),
		DownhillCost:      ptr32(60),
		UphillCutoffPct:   ptr32(1.5),
		DownhillCutoffPct: ptr32(1.5),
		ElevPenaltyBuffer: ptr32(5),
		ElevMaxBuffer:     ptr32(10),
		ElevBufferReduce:  ptr32(0),
		AllowSteps:        ptrBool(false),
		TrafficPenalty:    ptr32(0.1),
		TurnCost:          ptr32(90),
		Pass0Coefficient:  ptr32(3.0), // higher: downhillcost=60 inflates pass0 budget
	},
}

// BuiltinProfile returns the built-in defaults for the given profile name.
func BuiltinProfile(name string) (ProfileConfig, error) {
	p, ok := builtinProfiles[name]
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

	merged := mergeProfile(base, p)

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
	return cfg, nil
}

// DefaultCostConfig returns a CostConfig for the default trekking profile.
func DefaultCostConfig() *CostConfig {
	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "trekking"})
	return cfg
}

// --- Merge logic ---

func mergeProfile(base, override ProfileConfig) ProfileConfig {
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
		mtbScale := uint8(graph.DecodeField(tags1, 17, 3)) // bits 17-19
		if maxMtb > 0 && mtbScale > 0 && int(mtbScale) > maxMtb+1 { // enum is 1-based (0=1, 1=2, ...)
			return EdgeCostFactors{Flat: costBlocked}
		}

		// --- Tracktype ---
		tracktype := uint8(graph.DecodeField(tags1, 11, 3)) // bits 11-13

		// --- Compute base costfactor ---
		hwCF := profileHighwayCost(profile, hw, tracktype, sf)
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
		// --- Enrichment (additive, from tags2) ---
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

func profileHighwayCost(profile string, hw, tracktype, sf uint8) float32 {
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
		return roadHighwayCost(hw, sf)
	default:
		return hikingHighwayCost(hw)
	}
}

func hikingHighwayCost(hw uint8) float32 {
	switch hw {
	case 16: // path
		return 1.0
	case 15: // track
		return 1.0
	case 17: // footway
		return 1.0
	case 20: // steps
		return 1.0
	case 19: // bridleway
		return 1.2
	case 18: // cycleway
		return 1.1
	case 11, 12: // residential, living_street
		return 1.1
	case 13: // unclassified
		return 1.1
	case 14: // service
		return 1.1
	case 9, 10: // tertiary
		return 1.2
	case 7, 8: // secondary
		return 1.5
	case 5, 6: // primary
		return 2.0
	case 21: // pedestrian
		return 1.0
	case 22: // via_ferrata
		return 1.2
	default:
		return 1.5
	}
}

func mtbHighwayCost(hw uint8) float32 {
	switch hw {
	case 15: // track
		return 1.0
	case 16: // path
		return 1.1
	case 18: // cycleway
		return 1.0
	case 17: // footway
		return 1.2
	case 11, 12: // residential, living_street
		return 1.0
	case 14: // service
		return 1.1
	case 13: // unclassified
		return 1.0
	case 9, 10: // tertiary
		return 1.1
	case 7, 8: // secondary
		return 1.2
	case 5, 6: // primary
		return 1.5
	case 21: // pedestrian
		return 2.0
	case 19: // bridleway
		return 3.0
	case 20: // steps
		return costBlocked
	case 22: // via_ferrata
		return costBlocked
	default:
		return 1.5
	}
}

func trekkingHighwayCost(hw, tracktype uint8) float32 {
	switch hw {
	case 18: // cycleway
		return 1.0
	case 15: // track (grade-dependent)
		switch tracktype {
		case 1: // grade1
			return 1.0
		case 2: // grade2
			return 1.3
		case 3: // grade3
			return 1.8
		case 4: // grade4
			return 2.5
		case 5: // grade5
			return 3.0
		default: // unknown
			return 1.3
		}
	case 16: // path
		return 1.0
	case 17: // footway
		return 1.2
	case 11, 12: // residential, living_street
		return 1.1
	case 14: // service
		return 1.3
	case 13: // unclassified
		return 1.3
	case 9, 10: // tertiary
		return 1.4
	case 7, 8: // secondary
		return 1.6
	case 5, 6: // primary
		return 3.0
	case 19: // bridleway
		return 5.0
	case 20: // steps
		return 40
	case 21: // pedestrian
		return 2.0
	case 22: // via_ferrata
		return costBlocked
	default:
		return 1.5
	}
}

func gravelHighwayCost(hw, tracktype uint8) float32 {
	switch hw {
	case 15: // track (grade-dependent)
		switch tracktype {
		case 1:
			return 1.0
		case 2:
			return 1.5
		case 3:
			return 2.0
		case 4:
			return 3.0
		case 5:
			return 4.0
		default:
			return 1.5
		}
	case 18: // cycleway
		return 1.0
	case 16: // path
		return 1.5
	case 11, 12: // residential, living_street
		return 1.8
	case 14: // service
		return 1.5
	case 13: // unclassified
		return 1.5
	case 9, 10: // tertiary
		return 3.0
	case 7, 8: // secondary
		return 4.0
	case 5, 6: // primary
		return 5.0
	case 17: // footway
		return 2.0
	case 19: // bridleway
		return 3.0
	case 20: // steps
		return costBlocked
	case 22: // via_ferrata
		return costBlocked
	case 21: // pedestrian
		return 3.0
	default:
		return 2.0
	}
}

func roadHighwayCost(hw, sf uint8) float32 {
	isPaved := sf >= 1 && sf <= 7 // asphalt, concrete, paving_stones, sett, cobblestone, metal, wood

	switch hw {
	case 9, 10: // tertiary
		return 1.0
	case 7, 8: // secondary
		return 1.1
	case 5, 6: // primary
		return 1.2
	case 18: // cycleway
		return 1.3
	case 11, 12: // residential, living_street
		if isPaved || sf == 0 {
			return 1.2
		}
		return 10
	case 14: // service
		if isPaved || sf == 0 {
			return 1.5
		}
		return 10
	case 13: // unclassified
		if isPaved || sf == 0 {
			return 1.3
		}
		return 10
	case 15: // track
		if isPaved {
			return 1.5
		}
		return 10
	case 16: // path
		return 10
	case 17: // footway
		return 10
	case 19: // bridleway
		return 10
	case 20: // steps
		return costBlocked
	case 22: // via_ferrata
		return costBlocked
	case 21: // pedestrian
		return 5.0
	default:
		return 5.0
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
	case 0: // unknown
		return 1.0
	case 1: // asphalt
		return 1.0
	case 2: // concrete
		return 1.1
	case 3: // paving_stones
		return 1.2
	case 8: // compacted
		return 1.3
	case 9: // fine_gravel
		return 1.5
	case 10: // gravel
		return 3.0
	case 12, 13, 16: // dirt, earth, ground
		return 5.0
	case 17: // mud
		return 10
	case 14: // grass
		return 5.0
	case 18: // sand
		return 5.0
	case 20: // snow
		return 10
	case 21: // ice
		return 10
	default:
		return 1.5
	}
}

func gravelSurfaceCost(sf uint8) float32 {
	switch sf {
	case 0: // unknown
		return 1.0
	case 8, 9: // compacted, fine_gravel
		return 1.0
	case 1, 2: // asphalt, concrete
		return 1.1
	case 3: // paving_stones
		return 1.1
	case 10: // gravel
		return 1.2
	case 12, 13, 16: // dirt, earth, ground
		return 1.8
	case 17: // mud
		return 3.0
	case 18: // sand
		return 3.0
	case 14: // grass
		return 2.0
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
