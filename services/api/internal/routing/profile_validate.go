package routing

import (
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// ValidProfileNames lists all accepted base profile names (exported for handler validation).
var ValidProfileNames = []string{"hiking", "mtb", "gravel", "trekking", "road"}

// validProfiles is the lookup set for ValidProfileNames.
var validProfiles = map[string]bool{
	"hiking":   true,
	"mtb":      true,
	"gravel":   true,
	"trekking": true,
	"road":     true,
}

// ValidateProfileConfig validates all non-nil fields of a ProfileConfig.
// Returns a descriptive error for the first out-of-range field found.
func ValidateProfileConfig(p ProfileConfig) error {
	if p.Profile != "" {
		if !validProfiles[p.Profile] {
			return fmt.Errorf("profile %q is not valid: must be one of hiking, mtb, gravel, trekking, road", p.Profile)
		}
	}

	// Elevation
	if p.UphillCost != nil {
		if *p.UphillCost < 0 || *p.UphillCost > 200 {
			return fmt.Errorf("uphill_cost %.2f out of range [0, 200]", *p.UphillCost)
		}
	}
	if p.DownhillCost != nil {
		if *p.DownhillCost < 0 || *p.DownhillCost > 200 {
			return fmt.Errorf("downhill_cost %.2f out of range [0, 200]", *p.DownhillCost)
		}
	}
	if p.UphillCutoffPct != nil {
		if *p.UphillCutoffPct < 0 || *p.UphillCutoffPct > 30 {
			return fmt.Errorf("uphill_cutoff_pct %.2f out of range [0, 30]", *p.UphillCutoffPct)
		}
	}
	if p.DownhillCutoffPct != nil {
		if *p.DownhillCutoffPct < 0 || *p.DownhillCutoffPct > 30 {
			return fmt.Errorf("downhill_cutoff_pct %.2f out of range [0, 30]", *p.DownhillCutoffPct)
		}
	}
	if p.ElevPenaltyBuffer != nil {
		if *p.ElevPenaltyBuffer < 0 || *p.ElevPenaltyBuffer > 50 {
			return fmt.Errorf("elev_penalty_buffer_m %.2f out of range [0, 50]", *p.ElevPenaltyBuffer)
		}
	}
	if p.ElevMaxBuffer != nil {
		if *p.ElevMaxBuffer < 0 || *p.ElevMaxBuffer > 100 {
			return fmt.Errorf("elev_max_buffer_m %.2f out of range [0, 100]", *p.ElevMaxBuffer)
		}
	}
	if p.ElevBufferReduce != nil {
		if *p.ElevBufferReduce < 0 || *p.ElevBufferReduce > 1 {
			return fmt.Errorf("elev_buffer_reduce %.4f out of range [0, 1]", *p.ElevBufferReduce)
		}
	}

	// Difficulty
	if p.MaxSacScale != nil {
		if *p.MaxSacScale < 0 || *p.MaxSacScale > 6 {
			return fmt.Errorf("max_sac_scale %d out of range [0, 6]", *p.MaxSacScale)
		}
	}
	if p.SacPreferred != nil {
		if *p.SacPreferred < 0 || *p.SacPreferred > 6 {
			return fmt.Errorf("sac_preferred %d out of range [0, 6]", *p.SacPreferred)
		}
	}
	if p.MaxMtbScale != nil {
		if *p.MaxMtbScale < 0 || *p.MaxMtbScale > 5 {
			return fmt.Errorf("max_mtb_scale %d out of range [0, 5]", *p.MaxMtbScale)
		}
	}
	if p.MaxViaFerrata != nil {
		if *p.MaxViaFerrata < 0 || *p.MaxViaFerrata > 6 {
			return fmt.Errorf("max_via_ferrata %d out of range [0, 6]", *p.MaxViaFerrata)
		}
	}

	// Enrichment weights
	if p.ForestBonus != nil {
		if *p.ForestBonus < 0 || *p.ForestBonus > 1 {
			return fmt.Errorf("forest_bonus %.4f out of range [0, 1]", *p.ForestBonus)
		}
	}
	if p.NoisePenalty != nil {
		if *p.NoisePenalty < 0 || *p.NoisePenalty > 1 {
			return fmt.Errorf("noise_penalty %.4f out of range [0, 1]", *p.NoisePenalty)
		}
	}
	if p.TrafficPenalty != nil {
		if *p.TrafficPenalty < 0 || *p.TrafficPenalty > 1 {
			return fmt.Errorf("traffic_penalty %.4f out of range [0, 1]", *p.TrafficPenalty)
		}
	}
	if p.ProtectedBonus != nil {
		if *p.ProtectedBonus < 0 || *p.ProtectedBonus > 1 {
			return fmt.Errorf("protected_bonus %.4f out of range [0, 1]", *p.ProtectedBonus)
		}
	}

	// Route adherence
	if p.CyclerouteBoost != nil {
		if *p.CyclerouteBoost < 0 || *p.CyclerouteBoost > 1 {
			return fmt.Errorf("cycleroute_boost %.4f out of range [0, 1]", *p.CyclerouteBoost)
		}
	}
	if p.HikerouteBoost != nil {
		if *p.HikerouteBoost < 0 || *p.HikerouteBoost > 1 {
			return fmt.Errorf("hikeroute_boost %.4f out of range [0, 1]", *p.HikerouteBoost)
		}
	}

	// Behavior
	if p.PathPreference != nil {
		if *p.PathPreference < 0 || *p.PathPreference > 30 {
			return fmt.Errorf("path_preference %.2f out of range [0, 30]", *p.PathPreference)
		}
	}
	if p.TurnCost != nil {
		if *p.TurnCost < 0 || *p.TurnCost > 150 {
			return fmt.Errorf("turn_cost %.2f out of range [0, 150]", *p.TurnCost)
		}
	}
	if p.InitialCostRoad != nil {
		if *p.InitialCostRoad < 0 || *p.InitialCostRoad > 50000 {
			return fmt.Errorf("initial_cost_road %.2f out of range [0, 50000]", *p.InitialCostRoad)
		}
	}

	// A*
	if p.Pass0Coefficient != nil {
		if *p.Pass0Coefficient < 1.0 || *p.Pass0Coefficient > 5.0 {
			return fmt.Errorf("pass0_coefficient %.4f out of range [1.0, 5.0]", *p.Pass0Coefficient)
		}
	}

	// Snap
	if p.SnapThresholdM != nil {
		if *p.SnapThresholdM < 10 || *p.SnapThresholdM > 500 {
			return fmt.Errorf("snap_threshold_m %.2f out of range [10, 500]", *p.SnapThresholdM)
		}
	}

	// Highway costs
	type hwField struct {
		name string
		val  *float32
	}
	hwFields := []hwField{
		{"path_cost", p.PathCost},
		{"track_cost", p.TrackCost},
		{"track_grade1_cost", p.TrackGrade1Cost},
		{"track_grade2_cost", p.TrackGrade2Cost},
		{"track_grade3_cost", p.TrackGrade3Cost},
		{"footway_cost", p.FootwayCost},
		{"cycleway_cost", p.CyclewayCost},
		{"bridleway_cost", p.BridlewayCost},
		{"pedestrian_cost", p.PedestrianCost},
		{"residential_cost", p.ResidentialCost},
		{"service_cost", p.ServiceCost},
		{"unclassified_cost", p.UnclassifiedCost},
		{"tertiary_cost", p.TertiaryCost},
		{"secondary_cost", p.SecondaryCost},
		{"primary_cost", p.PrimaryCost},
	}
	for _, f := range hwFields {
		if f.val != nil {
			if *f.val < 0.5 || *f.val > 50 {
				return fmt.Errorf("%s %.4f out of range [0.5, 50]", f.name, *f.val)
			}
		}
	}

	return nil
}

// ValidateSportTypes checks that each entry in types is a known sport type key
// as registered in the sport taxonomy.
func ValidateSportTypes(types []string) error {
	for _, t := range types {
		if _, ok := sport.GetSportDef(t); !ok {
			return fmt.Errorf("unknown sport type %q", t)
		}
	}
	return nil
}
