-- Migration 000189: Reset corrupted stage-2 enrichment module flags to TRUE.
--
-- Background: A bug in an earlier version of UpdateEnrichmentSettings sent the
-- full settings object from EnrichmentSettingsPage without preserving stage-2
-- fields, causing the backend (before the merge-over-existing fix) to write
-- false for all stage-2 flags that were not explicitly included in the payload.
-- Migration 000116 added these columns with DEFAULT true, but existing rows
-- that were saved via the buggy path ended up with false values.
--
-- Fix: Reset all stage-2 flags to true for every row where they are false.
-- This is safe because the default intent is "all modules enabled" and the
-- only way a user would have intentionally disabled a module is via the
-- Stage2EnrichmentSection in SettingsPage (which only existed after the bug
-- was already present). We preserve any row where the flag is already true.

UPDATE user_enrichment_settings
SET
    enable_peaks       = true,
    enable_lifts       = true,
    enable_settlements = true,
    enable_commute     = true,
    enable_borders     = true,
    enable_water       = true,
    enable_forests     = true,
    enable_surfaces    = true,
    enable_stops       = true,
    enable_intervals   = true,
    enable_ascents     = true,
    enable_embeddings  = true
WHERE
    enable_peaks       = false
    OR enable_lifts       = false
    OR enable_settlements = false
    OR enable_commute     = false
    OR enable_borders     = false
    OR enable_water       = false
    OR enable_forests     = false
    OR enable_surfaces    = false
    OR enable_stops       = false
    OR enable_intervals   = false
    OR enable_ascents     = false
    OR enable_embeddings  = false;
