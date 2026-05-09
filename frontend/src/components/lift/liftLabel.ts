import type { LiftCandidate } from '@/api/types'

/**
 * Whitelist of supported OSM aerialway= values mapped to localised German
 * labels. Drawn from the OSM wiki (Key:aerialway). Excludes:
 *   - `goods` (Materialseilbahn / freight only — not a passenger lift)
 *   - `pylon` and `station` (structural elements, not transport types)
 *   - any value not in this list (defensive: better empty than wrong)
 *
 * Order is roughly by passenger-lift relevance.
 */
export const AERIALWAY_LABELS: Record<string, string> = {
  cable_car: 'Seilbahn',
  gondola: 'Gondelbahn',
  mixed_lift: 'Kombilift',
  chair_lift: 'Sessellift',
  drag_lift: 'Schlepplift',
  't-bar': 'Bügellift',
  'j-bar': 'J-Bügellift',
  platter: 'Tellerlift',
  rope_tow: 'Seilschlepplift',
  magic_carpet: 'Förderband',
  zip_line: 'Zipline',
}

/**
 * Returns the display name and type label for a lift candidate.
 *
 *   - When osm_aerialway is NULL or not in the whitelist (e.g. 'goods',
 *     unknown values): returns {} — we deliberately show nothing rather than
 *     an unsupported / wrong-category label. Pure heuristic candidates that
 *     never snapped to an OSM segment fall into this branch.
 *   - When osm_aerialway is whitelisted: returns { type } and, if the OSM
 *     row carries a name, also { name }. Both are plain strings the caller
 *     can render directly.
 *
 * The function is source-agnostic: heuristic candidates that snapped to an
 * OSM lift via segment_effort_id (Phase 4c snap pipeline) carry the same
 * JOIN-derived osm_aerialway/osm_name as source='osm' candidates and get a
 * chip on equal footing.
 */
export function liftDisplayLabel(c: LiftCandidate): { name?: string; type?: string } {
  const aerialway = c.osm_aerialway ?? ''
  const type = AERIALWAY_LABELS[aerialway]
  if (!type) return {}
  const name = c.osm_name?.trim() ? c.osm_name.trim() : undefined
  return { name, type }
}
