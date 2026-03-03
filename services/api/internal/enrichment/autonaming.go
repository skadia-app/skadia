package enrichment

import (
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// GenerateAutoTitle creates a German auto-title like "Radfahrt in München".
// Delegates to the centralized sport taxonomy for label resolution.
func GenerateAutoTitle(sportType, city string) string {
	return GenerateAutoTitleWithSubType(sportType, "", city)
}

// GenerateAutoTitleWithSubType creates a German auto-title using both sport type
// and sub-type for more specific naming (e.g. "MTB-Tour in München").
func GenerateAutoTitleWithSubType(sportType, subType, city string) string {
	label := sport.AutoTitle(sportType, subType)

	if city != "" {
		return fmt.Sprintf("%s in %s", label, city)
	}
	return label
}
