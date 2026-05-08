package applicablemodifiers

import (
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// ValidateKeys checks that every key in the list exists in the sport modifier registry.
// Returns an error naming any unknown keys.
func ValidateKeys(keys []string) error {
	var unknown []string
	for _, key := range keys {
		if !sport.SportModifierExists(key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown modifier keys: %s", strings.Join(unknown, ", "))
	}
	return nil
}
