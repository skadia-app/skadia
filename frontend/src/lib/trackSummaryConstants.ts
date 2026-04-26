// Track-summary frontend constants.
// Backend source: services/api/internal/enrichment/surfaces_sql.go — surfaceResampleM = 10.0
//
// RESAMPLE_SPACING_M is a fallback for backend responses that do not yet include the
// resample_spacing_m field (backends older than the R1-B wave). TrackMap prefers the
// API-supplied value (ActivityTrackSummary.resample_spacing_m) when available.

/** Resampling interval in metres — fallback when resample_spacing_m is absent from API response. */
export const RESAMPLE_SPACING_M = 10
