// Sport Taxonomy — Single Source of Truth for all sport-related display logic
// in the frontend. Must match the backend Go taxonomy exactly (same sport_type
// keys, category keys).

// ── Types ────────────────────────────────────────────────────────────────────────

export interface SportCategory {
  key: string
  label: string
  color: string
}

export interface SubTypeDef {
  subType: string
  label: string
  detail: string
}

export interface SportDef {
  sportType: string
  categoryKey: string
  label: string
  typicallyHasGPS: boolean
  subTypes: SubTypeDef[]
}

// ── Categories ──────────────────────────────────────────────────────────────────

export const CATEGORIES: SportCategory[] = [
  { key: 'run', label: 'Läufe', color: '#EF4444' },
  { key: 'bike', label: 'Radfahrten', color: '#F97316' },
  { key: 'winter', label: 'Wintersport', color: '#06B6D4' },
  { key: 'water', label: 'Wassersport', color: '#3B82F6' },
  { key: 'fitness', label: 'Fitness', color: '#8B5CF6' },
  { key: 'game', label: 'Spiele', color: '#10B981' },
  { key: 'climb', label: 'Klettern', color: '#A855F7' },
  { key: 'outdoor', label: 'Outdoor', color: '#84CC16' },
  { key: 'multi', label: 'Mehrkampf', color: '#F59E0B' },
  { key: 'other', label: 'Sonstiges', color: '#6B7280' },
]

const categoryMap = new Map<string, SportCategory>(CATEGORIES.map((c) => [c.key, c]))

// ── Helper ──────────────────────────────────────────────────────────────────────

function def(
  sportType: string,
  categoryKey: string,
  label: string,
  typicallyHasGPS: boolean,
  subTypes: SubTypeDef[] = [],
): SportDef {
  return { sportType, categoryKey, label, typicallyHasGPS, subTypes }
}

function sub(subType: string, label: string, detail?: string): SubTypeDef {
  return { subType, label, detail: detail ?? label }
}

// ── Sport Definitions ───────────────────────────────────────────────────────────

export const SPORT_DEFS: SportDef[] = [
  // ── run ──
  def('running', 'run', 'Laufen', true, [
    sub('treadmill', 'Laufband'),
    sub('street', 'Straße'),
    sub('trail', 'Trail'),
    sub('track', 'Bahn'),
    sub('indoor_running', 'Indoor'),
    sub('ultra', 'Ultra'),
    sub('obstacle', 'Hindernislauf'),
  ]),
  def('walking', 'run', 'Gehen', true, [
    sub('casual_walking', 'Gehen'),
    sub('speed_walking', 'Speedwalking'),
    sub('indoor_walking', 'Indoor'),
  ]),
  def('hiking', 'run', 'Wandern', true),
  def('mountaineering', 'run', 'Bergsteigen', true),
  def('snowshoeing', 'run', 'Schneeschuhwandern', true),
  def('floor_climbing', 'run', 'Treppensteigen', false),
  def('wheelchair_push_walk', 'run', 'Rollstuhl (Gehen)', true),
  def('wheelchair_push_run', 'run', 'Rollstuhl (Laufen)', true),

  // ── bike ──
  def('cycling', 'bike', 'Radfahren', true, [
    sub('road', 'Rennrad'),
    sub('mountain', 'MTB'),
    sub('downhill', 'Downhill'),
    sub('cyclocross', 'Cyclocross'),
    sub('track_cycling', 'Bahnrad'),
    sub('bmx', 'BMX'),
    sub('gravel_cycling', 'Gravel'),
    sub('commuting', 'Pendeln'),
    sub('mixed_surface', 'Mixed Surface'),
    sub('indoor_cycling', 'Indoor'),
    sub('spin', 'Spinning'),
    sub('hand_cycling', 'Handbike'),
    sub('recumbent', 'Liegerad'),
  ]),
  def('e_biking', 'bike', 'E-Bike', true, [
    sub('e_bike_fitness', 'E-Bike Fitness'),
    sub('e_bike_mountain', 'E-MTB'),
  ]),
  def('inline_skating', 'bike', 'Inline-Skaten', true),

  // ── winter ──
  def('alpine_skiing', 'winter', 'Alpin-Ski', true, [
    sub('resort', 'Piste'),
    sub('backcountry', 'Backcountry'),
    sub('indoor_skiing', 'Indoor'),
  ]),
  def('cross_country_skiing', 'winter', 'Langlauf', true, [
    sub('skate_skiing', 'Skating'),
  ]),
  def('snowboarding', 'winter', 'Snowboard', true),
  def('ice_skating', 'winter', 'Eislaufen', true),
  def('snowmobiling', 'winter', 'Schneemobil', true),

  // ── water ──
  def('swimming', 'water', 'Schwimmen', true, [
    sub('lap_swimming', 'Hallenbad'),
    sub('open_water', 'Freiwasser'),
  ]),
  def('paddling', 'water', 'Paddeln', true),
  def('kayaking', 'water', 'Kajak', true, [
    sub('whitewater', 'Wildwasser'),
  ]),
  def('stand_up_paddleboarding', 'water', 'SUP', true),
  def('rowing', 'water', 'Rudern', true, [
    sub('indoor_rowing', 'Indoor-Rudern'),
  ]),
  def('surfing', 'water', 'Surfen', true),
  def('kitesurfing', 'water', 'Kitesurfen', true),
  def('windsurfing', 'water', 'Windsurfen', true),
  def('wakeboarding', 'water', 'Wakeboard', true),
  def('wakesurfing', 'water', 'Wakesurfen', true),
  def('water_skiing', 'water', 'Wasserski', true),
  def('water_tubing', 'water', 'Tubing', true),
  def('diving', 'water', 'Tauchen', true, [
    sub('single_gas_diving', 'Eingas-Tauchen'),
    sub('multi_gas_diving', 'Mehrgas-Tauchen'),
    sub('apnea_diving', 'Apnoe-Tauchen'),
  ]),
  def('snorkeling', 'water', 'Schnorcheln', true),
  def('boating', 'water', 'Bootfahren', true),
  def('sailing', 'water', 'Segeln', true, [
    sub('sail_race', 'Regatta'),
  ]),
  def('fishing', 'water', 'Angeln', true),
  def('rafting', 'water', 'Rafting', true),

  // ── fitness ──
  def('training', 'fitness', 'Training', false, [
    sub('strength_training', 'Krafttraining'),
    sub('flexibility_training', 'Flexibilität'),
    sub('cardio_training', 'Cardio'),
    sub('yoga', 'Yoga'),
    sub('pilates', 'Pilates'),
  ]),
  def('fitness_equipment', 'fitness', 'Fitnessgerät', false, [
    sub('elliptical', 'Crosstrainer'),
    sub('stair_climbing', 'Treppensteiger'),
  ]),
  def('hiit', 'fitness', 'HIIT', false, [
    sub('amrap', 'AMRAP'),
    sub('emom', 'EMOM'),
    sub('tabata', 'Tabata'),
  ]),
  def('dance', 'fitness', 'Tanzen', false),
  def('jump_rope', 'fitness', 'Seilspringen', false),
  def('meditation', 'fitness', 'Meditation', false),

  // ── game ──
  def('soccer', 'game', 'Fußball', false, [
    sub('warm_up', 'Aufwärmen'),
    sub('match', 'Spiel'),
    sub('exercise', 'Übung'),
  ]),
  def('basketball', 'game', 'Basketball', false),
  def('american_football', 'game', 'American Football', false),
  def('rugby', 'game', 'Rugby', false),
  def('lacrosse', 'game', 'Lacrosse', false),
  def('hockey', 'game', 'Hockey', false),
  def('volleyball', 'game', 'Volleyball', false),
  def('tennis', 'game', 'Tennis', true),
  def('racket', 'game', 'Schlägersport', false, [
    sub('pickleball', 'Pickleball'),
    sub('padel', 'Padel'),
    sub('squash', 'Squash'),
    sub('badminton', 'Badminton'),
    sub('racquetball', 'Racquetball'),
    sub('table_tennis', 'Tischtennis'),
  ]),
  def('cricket', 'game', 'Cricket', true),
  def('baseball', 'game', 'Baseball', false),
  def('golf', 'game', 'Golf', true),
  def('disc_golf', 'game', 'Disc Golf', true),

  // ── climb ──
  def('rock_climbing', 'climb', 'Klettern', true, [
    sub('bouldering', 'Bouldern'),
    sub('indoor_climbing', 'Klettern (Halle)'),
  ]),

  // ── outdoor ──
  def('flying', 'outdoor', 'Fliegen', true),
  def('hang_gliding', 'outdoor', 'Gleitschirmfliegen', true),
  def('sky_diving', 'outdoor', 'Fallschirmspringen', true),
  def('jumpmaster', 'outdoor', 'Jumpmaster', true),
  def('horseback_riding', 'outdoor', 'Reiten', true),
  def('hunting', 'outdoor', 'Jagen', true),
  def('motorcycling', 'outdoor', 'Motorrad', true),
  def('driving', 'outdoor', 'Autofahren', true),
  def('boxing', 'outdoor', 'Boxen', false),
  def('mixed_martial_arts', 'outdoor', 'MMA', false),
  def('tactical', 'outdoor', 'Taktisch', false),

  // ── multi ──
  def('multisport', 'multi', 'Mehrkampf', true),
  def('transition', 'multi', 'Wechselzone', true),

  // ── other ──
  def('generic', 'other', 'Sonstige', false),
  def('unknown', 'other', 'Unbekannt', false),
]

// ── Lookup Maps ─────────────────────────────────────────────────────────────────

const sportMap = new Map<string, SportDef>()
const catSportsMap = new Map<string, SportDef[]>()

for (const s of SPORT_DEFS) {
  sportMap.set(s.sportType, s)
  const list = catSportsMap.get(s.categoryKey)
  if (list) {
    list.push(s)
  } else {
    catSportsMap.set(s.categoryKey, [s])
  }
}

// ── Sub-type promotion set ──────────────────────────────────────────────────────

const PROMOTE_SUB_TYPE = new Set(['training', 'fitness_equipment', 'racket'])

// ── Fallback humanizer ──────────────────────────────────────────────────────────

function humanize(raw: string): string {
  return raw
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}

// ── Lookup Functions ────────────────────────────────────────────────────────────

/**
 * Get sport definition. Returns undefined for unknown sport types.
 */
export function getSportDef(sportType: string): SportDef | undefined {
  return sportMap.get(sportType)
}

/**
 * German display label with sub-type promotion for training, fitness_equipment,
 * and racket sport types.
 *
 * Examples:
 *   sportLabel("running", "trail")             → "Trail"
 *   sportLabel("training", "yoga")             → "Yoga"
 *   sportLabel("training", "strength_training") → "Krafttraining"
 *   sportLabel("racket", "squash")             → "Squash"
 *   sportLabel("cycling")                      → "Radfahren"
 *   sportLabel("some_unknown_type")            → "Some Unknown Type"
 */
export function sportLabel(sportType: string, subType?: string | null): string {
  const d = sportMap.get(sportType)
  if (!d) return humanize(sportType)

  if (subType) {
    const st = d.subTypes.find((s) => s.subType === subType)
    if (st) {
      // For promoted sports (training, fitness_equipment, racket) AND for sports
      // where the sub-type label is more specific than the parent label
      return st.label
    }
  }

  return d.label
}

/**
 * Full detail label including both sport and sub-type when applicable.
 *
 * Examples:
 *   sportDetailLabel("alpine_skiing", "backcountry") → "Alpin-Ski (Backcountry)"
 *   sportDetailLabel("training", "yoga")             → "Training (Yoga)"
 *   sportDetailLabel("cycling")                      → "Radfahren"
 *   sportDetailLabel("some_unknown_type")            → "Some Unknown Type"
 */
export function sportDetailLabel(sportType: string, subType?: string | null): string {
  const d = sportMap.get(sportType)
  if (!d) return humanize(sportType)

  if (subType) {
    const st = d.subTypes.find((s) => s.subType === subType)
    if (st) return `${d.label} (${st.label})`
  }

  return d.label
}

/**
 * Category color for a sport type. Returns gray (#6B7280) for unknown types.
 */
export function sportColor(sportType: string): string {
  const d = sportMap.get(sportType)
  if (!d) return '#6B7280'
  return categoryMap.get(d.categoryKey)?.color ?? '#6B7280'
}

/**
 * Get category for a sport type. Defaults to the 'other' category for unknowns.
 */
export function categoryForSport(sportType: string): SportCategory {
  const d = sportMap.get(sportType)
  const cat = d ? categoryMap.get(d.categoryKey) : undefined
  return cat ?? { key: 'other', label: 'Sonstiges', color: '#6B7280' }
}

/**
 * Get all sport definitions in a category.
 */
export function sportsInCategory(categoryKey: string): SportDef[] {
  return catSportsMap.get(categoryKey) ?? []
}
