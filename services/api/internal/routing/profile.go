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
	setAvgCostPerKm(cfg, p.Profile)
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

func roadHighwayCost(hw, sf uint8) float32 {
	isPaved := sf >= 1 && sf <= 7 // asphalt, concrete, paving_stones, sett, cobblestone, metal, wood

	switch hw {
	case 18: // cycleway — bevorzugt: sicher, glatt, rennradgerecht
		return 1.0
	case 9, 10: // tertiary — Hauptwahl auf Straßen
		return 1.1
	case 7, 8: // secondary — gut befahrbar
		return 1.2
	case 11, 12: // residential, living_street — OK wenn befestigt
		if isPaved || sf == 0 {
			return 1.3
		}
		return costBlocked
	case 13: // unclassified — nur wenn befestigt
		if isPaved || sf == 0 {
			return 1.4
		}
		return costBlocked
	case 5, 6: // primary — viel Verkehr, akzeptabel aber nicht ideal
		return 1.5
	case 14: // service — kurze Verbindungen OK wenn befestigt
		if isPaved || sf == 0 {
			return 1.8
		}
		return costBlocked
	case 21: // pedestrian — Rennrad nur im Notfall
		return 8.0
	case 15: // track — Schotter = Pannengefahr
		if isPaved {
			return 4.0
		}
		return costBlocked
	case 16: // path — unbefestigt, unfahrbar für Rennrad
		return costBlocked
	case 17: // footway — nicht für Rennrad
		return costBlocked
	case 19: // bridleway — nicht für Rennrad
		return costBlocked
	case 20: // steps
		return costBlocked
	case 22: // via_ferrata
		return costBlocked
	default:
		return 8.0
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
		UphillCost:        ptr32(0),  // overridden per LandmarkCostConfig
		DownhillCost:      ptr32(0),  // overridden per LandmarkCostConfig
		UphillCutoffPct:   ptr32(3),
		DownhillCutoffPct: ptr32(3),
		ElevPenaltyBuffer: ptr32(5),
		ElevMaxBuffer:     ptr32(10),
		ElevBufferReduce:  ptr32(0.01),
		MaxSacScale:       ptrInt(6),        // allow everything
		SacPreferred:      ptrInt(0),        // no preference penalty
		MaxMtbScale:       ptrInt(5),        // allow everything
		MaxViaFerrata:     ptrInt(6),        // allow everything
		AllowSteps:        ptrBool(true),
		AllowFerries:      ptrBool(true),
		// ALL enrichment weights = 0 (lower bound guarantee)
		ForestBonus:    ptr32(0),
		NoisePenalty:   ptr32(0),
		TrafficPenalty: ptr32(0),
		ProtectedBonus: ptr32(0),
		CyclerouteBoost: ptr32(0),
		HikerouteBoost:  ptr32(0),
		WetWeather:      ptrBool(false),
		PathPreference:  ptr32(0),
		TurnCost:        ptr32(0),
		InitialCostRoad: ptr32(0),
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
