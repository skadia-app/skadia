-- Align enable_difficulty_scales with enable_surface_detection for every existing
-- user_enrichment_settings row.
--
-- Background: migration 000327 added enable_difficulty_scales with DEFAULT true,
-- which silently re-enabled difficulty enrichment for users who had previously
-- opted out of surface detection (enable_surface_detection=false). The flags are
-- now treated as mirrors of one another (track-summary surfaces the difficulty
-- mirror as a deprecated alias of enable_surface_detection); without this
-- alignment, users whose legacy state diverged from the new mirror semantics
-- would never converge — the one-shot frontend sync only fires when the user
-- opens the settings page.
UPDATE user_enrichment_settings
SET enable_difficulty_scales = enable_surface_detection
WHERE enable_difficulty_scales <> enable_surface_detection;
