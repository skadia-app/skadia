package enrichment

import "slices"

// CanonicalModules is the authoritative list of per-submodule names accepted by
// both /api/v1/activities/re-enrich (re_enrich.go) and the per-submodule path
// in /api/v1/activities/{id}/reprocess (recompute.go).
//
// Asymmetries documented:
//   - "lifts" is supported by recompute (dispatches to ML Stage-2) but NOT by
//     re_enrich — lift detection requires the original FIT parse context that
//     Stage-1 provides; re-enriching without reparsing would produce stale results.
//   - "embeddings" is recompute-only for the same reason (Stage-1 ML pipeline).
//
// To add a new module: add it here and update both handlers; the CI test in
// modules_test.go will catch divergence.
var CanonicalModules = []string{
	"ascents",
	"stops",
	"intervals",
	"summits",
	"settlements",
	"forests",
	"water",
	"borders",
	"surfaces",
}

// ReEnrichModules are the modules supported by the re-enrich endpoint.
// This is the same as CanonicalModules (lifts and embeddings are excluded
// because they require a Stage-1 reparse, see CanonicalModules comment).
var ReEnrichModules = CanonicalModules

// IsValidReEnrichModule reports whether name is a valid re-enrich module.
func IsValidReEnrichModule(name string) bool {
	return slices.Contains(ReEnrichModules, name)
}
