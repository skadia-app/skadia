package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
)

func enableMLServiceForTest(t *testing.T) {
	t.Helper()
	t.Cleanup(capabilities.SetMLServiceAvailableForTest(true))
}
