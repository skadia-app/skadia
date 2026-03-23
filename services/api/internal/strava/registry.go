package strava

import (
	"github.com/skadia-app/skadia/services/api/internal/importutil"
)

// Type aliases delegate to the shared importutil package.
type DataImporter = importutil.DataImporter[ExportSource]
type ImportParams = importutil.ImportParams[ExportSource]
type ImportResult = importutil.ImportResult
type DetectedModule = importutil.DetectedModule
type ImportRegistry = importutil.ImportRegistry[ExportSource]

// NewRegistry creates an empty registry.
func NewRegistry() *ImportRegistry { return importutil.NewRegistry[ExportSource]() }
