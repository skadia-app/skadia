// Track-summary frontend constants that mirror backend values.
// When the backend ships resample_spacing_m in the API response (R1-B wave),
// replace RESAMPLE_SPACING_M with the value from the response and remove this constant.
// Backend source: services/api/internal/enrichment/surfaces_sql.go — surfaceResampleM = 10.0

/** Resampling interval in metres — must match backend surfaceResampleM. */
export const RESAMPLE_SPACING_M = 10
