import type { LiftCandidate } from '@/api/types'
import { localizeString, type LocalizedString } from '@/i18n/localizedString'
import type { SupportedLanguage } from '@/i18n/languages'

/**
 * Whitelist of supported aerialway values mapped to localized lift labels.
 * Excludes:
 *   - `goods` (Materialseilbahn / freight only — not a passenger lift)
 *   - `pylon` and `station` (structural elements, not transport types)
 *   - any value not in this list (defensive: better empty than wrong)
 *
 * Order is roughly by passenger-lift relevance.
 */
const AERIALWAY_LABEL_MAPS: Record<string, LocalizedString> = {
  cable_car: {
    de: 'Seilbahn',
    en: 'Cable car',
    fr: 'Téléphérique',
    it: 'Funivia',
    es: 'Teleférico',
  },
  gondola: {
    de: 'Gondelbahn',
    en: 'Gondola',
    fr: 'Télécabine',
    it: 'Cabinovia',
    es: 'Telecabina',
  },
  mixed_lift: {
    de: 'Kombilift',
    en: 'Mixed lift',
    fr: 'Télémixte',
    it: 'Impianto misto',
    es: 'Remonte mixto',
  },
  chair_lift: {
    de: 'Sessellift',
    en: 'Chairlift',
    fr: 'Télésiège',
    it: 'Seggiovia',
    es: 'Telesilla',
  },
  drag_lift: {
    de: 'Schlepplift',
    en: 'Drag lift',
    fr: 'Téléski',
    it: 'Sciovia',
    es: 'Remonte de arrastre',
  },
  't-bar': {
    de: 'Bügellift',
    en: 'T-bar lift',
    fr: 'Téléski à archet',
    it: 'Skilift ad ancora',
    es: 'Remonte de barra T',
  },
  'j-bar': {
    de: 'J-Bügellift',
    en: 'J-bar lift',
    fr: 'Téléski à perche J',
    it: 'Skilift a J',
    es: 'Remonte de barra J',
  },
  platter: {
    de: 'Tellerlift',
    en: 'Platter lift',
    fr: 'Téléski à perche',
    it: 'Skilift a piattello',
    es: 'Remonte de plato',
  },
  rope_tow: {
    de: 'Seilschlepplift',
    en: 'Rope tow',
    fr: 'Télécorde',
    it: 'Manovia',
    es: 'Cuerda de remolque',
  },
  magic_carpet: {
    de: 'Förderband',
    en: 'Magic carpet',
    fr: 'Tapis roulant',
    it: 'Tappeto mobile',
    es: 'Cinta transportadora',
  },
  zip_line: {
    de: 'Zipline',
    en: 'Zipline',
    fr: 'Tyrolienne',
    it: 'Zipline',
    es: 'Tirolina',
  },
}

export const AERIALWAY_LABELS: Record<string, string> = Object.fromEntries(
  Object.entries(AERIALWAY_LABEL_MAPS).map(([key, labels]) => [key, labels.de]),
)

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
export function liftDisplayLabel(c: LiftCandidate, language?: SupportedLanguage): { name?: string; type?: string } {
  const aerialway = c.osm_aerialway ?? ''
  const type = localizeString(AERIALWAY_LABEL_MAPS[aerialway], language)
  if (!type) return {}
  const name = c.osm_name?.trim() ? c.osm_name.trim() : undefined
  return name ? { name, type } : { type }
}
