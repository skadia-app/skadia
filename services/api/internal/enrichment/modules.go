package enrichment

import "slices"

// CanonicalModules is the authoritative list of per-submodule names accepted by
// both /api/v1/activities/re-enrich (re_enrich.go) and the per-submodule path
// in /api/v1/activities/{id}/reprocess (recompute.go).
//
// Asymmetries documented:
//   - "embeddings" is recompute-only — it runs in the Stage-1 ML pipeline and
//     re-enriching without reparsing would produce stale results.
//
// To add a new module: add it here and update both handlers; the CI test in
// modules_test.go will catch divergence.
var CanonicalModules = []string{
	"ascents",
	"stops",
	"intervals",
	"lifts",
	"summits",
	"settlements",
	"forests",
	"water",
	"borders",
	"surfaces",
}

// ReEnrichModules are the modules supported by the re-enrich endpoint.
// This is the same as CanonicalModules (embeddings is excluded because it
// requires a Stage-1 reparse, see CanonicalModules comment).
var ReEnrichModules = CanonicalModules

// IsValidReEnrichModule reports whether name is a valid re-enrich module.
func IsValidReEnrichModule(name string) bool {
	return slices.Contains(ReEnrichModules, name)
}
