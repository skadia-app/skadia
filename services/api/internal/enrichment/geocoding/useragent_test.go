package geocoding

import (
	"strings"
	"testing"
)

// TestCurrentUserAgent_Format verifies the UA format is stable and contains
// the identifying pieces expected by public geocoding endpoints: product name,
// version, and contact.
func TestCurrentUserAgent_Format(t *testing.T) {
	ua := CurrentUserAgent()
	if !strings.HasPrefix(ua, "Skadia/") {
		t.Errorf("UA missing Skadia/ prefix: %q", ua)
	}
	if !strings.Contains(ua, SkadiaVersion) {
		t.Errorf("UA missing version: %q", ua)
	}
	if !strings.Contains(ua, userAgentContact) {
		t.Errorf("UA missing contact URL: %q", ua)
	}
}
