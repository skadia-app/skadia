package enrichment

// Bulk settings caching for enrichment loops.
//
// ReEnrichBatch (re_enrich.go) loads EnrichmentSettings once per user before
// the per-activity loop and passes the snapshot directly to
// enrichStreamLiftsCore. This avoids one SELECT per activity while keeping
// the per-activity override pointers (which reside on the Activity row, not
// on EnrichmentSettings) resolved correctly inside the core.
//
// The lift_recompute_all CLI (cmd/lift_recompute_all) dispatches through
// RunStreamEnrichment → enrichStreamLiftsOpts, which loads settings per
// activity. The per-activity refresh is intentional there: the CLI processes
// activities across a long time window where settings may change, and the
// additional round-trips against a local Postgres instance are negligible
// compared to the PostGIS snap pipeline work.
