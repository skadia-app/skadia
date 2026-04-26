package enrichment

import (
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// ShouldEnrichSurfaces reports whether surface/highway enrichment should run
// for the given activity. It returns false for virtual (indoor trainer,
// treadmill) activities and for water-sport activities (diving, swimming etc.)
// where OSM road-surface matching produces nonsensical results.
//
// Used by enrichment/re_enrich.go. For handler/recompute.go which operates on
// *model.ActivityData, use ShouldEnrichSurfacesData.
func ShouldEnrichSurfaces(act *model.Activity) bool {
	if act.IsVirtual {
		return false
	}
	if def, ok := sport.GetSportDef(act.SportType); ok {
		return def.CategoryKey != "water"
	}
	return true
}

// ShouldEnrichSurfacesData is the *model.ActivityData variant of ShouldEnrichSurfaces.
// Used by handler/recompute.go which operates on parsed activity data rather than
// the persisted activity record.
func ShouldEnrichSurfacesData(data *model.ActivityData) bool {
	if data.IsVirtual {
		return false
	}
	if def, ok := sport.GetSportDef(data.SportType); ok {
		return def.CategoryKey != "water"
	}
	return true
}
