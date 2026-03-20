-- Fix text_color for system tags where the stored value gives poor contrast.
-- Applies the same YIQ luma formula used by the frontend (selectCustomColor)
-- and Go backend (contrastTextColor): luminance > 0.5 → dark text, else white.
--
-- Affected tags identified:
--   Rundtour  #10B981  luma≈0.502 → '#1a1a1a' (was hardcoded '#FFFFFF', only 2.43:1 contrast)
--
-- Only updates existing rows where the stored text_color is wrong.

-- YIQ luma of #10B981 = 0.502 → dark text required; '#1a1a1a' gives 7.46:1 contrast
UPDATE tags
SET text_color = '#1a1a1a'
WHERE name = 'Rundtour'
  AND is_system = true
  AND color = '#10B981'
  AND text_color = '#FFFFFF';
