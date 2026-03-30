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
  segmentGroup: 'bike' | 'foot' | 'swim' | 'ski' | 'paddle' | 'generic'
  label: string
  typicallyHasGPS: boolean
  isIndoor: boolean
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
  { key: 'multi', label: 'Multisport', color: '#F59E0B' },
  { key: 'other', label: 'Sonstiges', color: '#6B7280' },
]

const categoryMap = new Map<string, SportCategory>(CATEGORIES.map((c) => [c.key, c]))

// ── Helper ──────────────────────────────────────────────────────────────────────

function def(
  sportType: string,
  categoryKey: string,
  segmentGroup: 'bike' | 'foot' | 'swim' | 'ski' | 'paddle' | 'generic',
  label: string,
  typicallyHasGPS: boolean,
  isIndoor: boolean = false,
  subTypes: SubTypeDef[] = [],
): SportDef {
  return { sportType, categoryKey, segmentGroup, label, typicallyHasGPS, isIndoor, subTypes }
}

function sub(subType: string, label: string, detail?: string): SubTypeDef {
  return { subType, label, detail: detail ?? label }
}

// ── Sport Definitions ───────────────────────────────────────────────────────────

export const SPORT_DEFS: SportDef[] = [
  // ── run ──
  def('running', 'run', 'foot', 'Laufen', true, false, [
    sub('treadmill', 'Laufband'),
    sub('street', 'Straße'),
    sub('trail', 'Trail'),
    sub('track', 'Bahn'),
    sub('indoor_running', 'Indoor'),
    sub('ultra', 'Ultra'),
    sub('obstacle', 'Hindernislauf'),
  ]),
  def('walking', 'run', 'foot', 'Gehen', true, false, [
    sub('casual_walking', 'Gehen'),
    sub('speed_walking', 'Speedwalking'),
    sub('indoor_walking', 'Indoor'),
  ]),
  def('hiking', 'run', 'foot', 'Wandern', true),
  def('mountaineering', 'run', 'foot', 'Bergsteigen', true),
  def('snowshoeing', 'run', 'foot', 'Schneeschuhwandern', true),
  def('floor_climbing', 'run', 'foot', 'Treppensteigen', false, true),
  def('wheelchair_push_walk', 'run', 'foot', 'Rollstuhl (Fahren)', true),
  def('wheelchair_push_run', 'run', 'foot', 'Rollstuhl (Rennen)', true),
  def('trail_running', 'run', 'foot', 'Trailrunning', true),
  def('treadmill_running', 'run', 'foot', 'Laufband', false, true),

  // ── bike ──
  def('cycling', 'bike', 'bike', 'Radfahren', true, false, [
    sub('road', 'Rennrad'),
    sub('mountain', 'MTB'),
    sub('downhill', 'Downhill'),
    sub('cyclocross', 'Cyclocross'),
    sub('track_cycling', 'Bahnrad'),
    sub('bmx', 'BMX'),
    sub('gravel_cycling', 'Gravel'),
    sub('mixed_surface', 'Mixed Surface'),
    sub('indoor_cycling', 'Indoor'),
    sub('spin', 'Spinning'),
    sub('hand_cycling', 'Handbike'),
    sub('recumbent', 'Liegerad'),
    sub('unicycling', 'Einrad'),
  ]),
  def('mountain_biking', 'bike', 'bike', 'Mountainbiken', true),
  def('gravel_cycling', 'bike', 'bike', 'Gravel', true),
  def('indoor_cycling', 'bike', 'bike', 'Indoor-Radfahren', false, true),
  def('inline_skating', 'outdoor', 'generic', 'Inline-Skaten', true),

  // ── winter ──
  def('alpine_skiing', 'winter', 'ski', 'Alpin-Ski', true, false, [
    sub('resort', 'Piste'),
    sub('backcountry', 'Backcountry'),
    sub('indoor_skiing', 'Indoor'),
  ]),
  def('backcountry_skiing', 'winter', 'ski', 'Skitour', true),
  def('cross_country_skiing', 'winter', 'ski', 'Langlauf', true, false, [
    sub('skate_skiing', 'Skating'),
  ]),
  def('snowboarding', 'winter', 'ski', 'Snowboard', true),
  def('ice_skating', 'winter', 'generic', 'Eislaufen', true),
  def('snowmobiling', 'winter', 'generic', 'Schneemobil', true),
  def('sledding', 'winter', 'generic', 'Rodeln', true),
  def('skiing', 'winter', 'ski', 'Skifahren', true),

  // ── water ──
  def('swimming', 'water', 'swim', 'Schwimmen', true, false, [
    sub('lap_swimming', 'Hallenbad'),
    sub('open_water', 'Freiwasser'),
  ]),
  def('lap_swimming', 'water', 'swim', 'Bahnschwimmen', false, true),
  def('open_water_swimming', 'water', 'swim', 'Freiwasserschwimmen', true),
  def('paddling', 'water', 'paddle', 'Wassersport', true),
  def('kayaking', 'water', 'paddle', 'Kajak', true, false, [
    sub('whitewater', 'Wildwasser'),
  ]),
  def('stand_up_paddleboarding', 'water', 'paddle', 'SUP', true),
  def('rowing', 'water', 'paddle', 'Rudern', true, false, [
    sub('indoor_rowing', 'Indoor-Rudern'),
  ]),
  def('rafting', 'water', 'paddle', 'Rafting', true),
  def('canoeing', 'water', 'paddle', 'Kanu', true),
  def('surfing', 'water', 'generic', 'Surfen', true),
  def('kitesurfing', 'water', 'generic', 'Kitesurfen', true),
  def('windsurfing', 'water', 'generic', 'Windsurfen', true),
  def('wakeboarding', 'water', 'generic', 'Wakeboard', true),
  def('wakesurfing', 'water', 'generic', 'Wakesurfen', true),
  def('water_skiing', 'water', 'generic', 'Wasserski', true),
  def('water_tubing', 'water', 'generic', 'Tubing', true),
  def('diving', 'water', 'generic', 'Tauchen', false, false, [
    sub('single_gas_diving', 'Eingas-Tauchen'),
    sub('multi_gas_diving', 'Mehrgas-Tauchen'),
    sub('gauge_diving', 'Gauge-Tauchen'),
    sub('apnea_diving', 'Apnoe-Tauchen'),
    sub('apnea_hunting', 'Apnoe-Jagdtauchen'),
  ]),
  def('snorkeling', 'water', 'generic', 'Schnorcheln', true),
  def('boating', 'water', 'generic', 'Bootfahren', true),
  def('sailing', 'water', 'generic', 'Segeln', true, false, [
    sub('sail_race', 'Regatta'),
  ]),
  def('fishing', 'water', 'generic', 'Angeln', true),

  // ── fitness ──
  def('training', 'fitness', 'generic', 'Training', false, true, [
    sub('strength_training', 'Krafttraining'),
    sub('flexibility_training', 'Flexibilität'),
    sub('cardio_training', 'Cardio'),
    sub('yoga', 'Yoga'),
    sub('pilates', 'Pilates'),
  ]),
  def('fitness_equipment', 'fitness', 'generic', 'Fitnessgerät', false, true, [
    sub('elliptical', 'Crosstrainer'),
    sub('stair_climbing', 'Treppensteiger'),
  ]),
  def('strength_training', 'fitness', 'generic', 'Krafttraining', false, true, [
    sub('amrap', 'AMRAP', 'As Many Reps As Possible'),
    sub('emom', 'EMOM', 'Every Minute On the Minute'),
  ]),
  def('hiit', 'fitness', 'generic', 'HIIT', false, true, [
    sub('tabata', 'Tabata'),
  ]),
  def('yoga', 'fitness', 'generic', 'Yoga', false, true),
  def('pilates', 'fitness', 'generic', 'Pilates', false, true),
  def('dance', 'fitness', 'generic', 'Tanzen', false, true),
  def('jump_rope', 'fitness', 'generic', 'Seilspringen', false, true),
  def('meditation', 'fitness', 'generic', 'Meditation', false, true),
  def('strength', 'fitness', 'generic', 'Krafttraining', false, true),

  // ── game ──
  def('soccer', 'game', 'generic', 'Fußball', false, false, [
    sub('warm_up', 'Aufwärmen'),
    sub('match', 'Spiel'),
    sub('exercise', 'Übung'),
  ]),
  def('basketball', 'game', 'generic', 'Basketball', false),
  def('american_football', 'game', 'generic', 'American Football', false),
  def('rugby', 'game', 'generic', 'Rugby', false),
  def('lacrosse', 'game', 'generic', 'Lacrosse', false),
  def('hockey', 'game', 'generic', 'Hockey', false),
  def('volleyball', 'game', 'generic', 'Volleyball', false),
  def('tennis', 'game', 'generic', 'Tennis', true),
  def('racket', 'game', 'generic', 'Schlägersport', false, false, [
    sub('pickleball', 'Pickleball'),
    sub('padel', 'Padel'),
    sub('squash', 'Squash'),
    sub('badminton', 'Badminton'),
    sub('racquetball', 'Racquetball'),
    sub('table_tennis', 'Tischtennis'),
  ]),
  def('cricket', 'game', 'generic', 'Cricket', true),
  def('baseball', 'game', 'generic', 'Baseball', false),
  def('golf', 'game', 'generic', 'Golf', true),
  def('disc_golf', 'game', 'generic', 'Disc Golf', true),

  // ── climb ──
  def('rock_climbing', 'climb', 'generic', 'Klettern', true, false, [
    sub('bouldering', 'Bouldern'),
    sub('indoor_climbing', 'Klettern (Halle)'),
  ]),

  // ── outdoor ──
  def('flying', 'outdoor', 'generic', 'Fliegen', true),
  def('hang_gliding', 'outdoor', 'generic', 'Gleitschirmfliegen', true),
  def('sky_diving', 'outdoor', 'generic', 'Fallschirmspringen', true),
  def('jumpmaster', 'outdoor', 'generic', 'Jumpmaster', true),
  def('horseback_riding', 'outdoor', 'generic', 'Reiten', true),
  def('hunting', 'outdoor', 'generic', 'Jagen', true),
  def('motorcycling', 'outdoor', 'generic', 'Motorrad', true),
  def('driving', 'outdoor', 'generic', 'Autofahren', true),
  def('boxing', 'fitness', 'generic', 'Boxen', false, true),
  def('mixed_martial_arts', 'fitness', 'generic', 'MMA', false, true),
  def('tactical', 'outdoor', 'generic', 'Taktisch', false),

  // ── multi ──
  def('multisport', 'multi', 'generic', 'Multisport', true),
  def('transition', 'multi', 'generic', 'Wechselzone', true),

  // ── other ──
  def('generic', 'other', 'generic', 'Sonstige', false),
  def('unknown', 'other', 'generic', 'Unbekannt', false),
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

export const PROMOTE_SUB_TYPE = new Set(['training', 'fitness_equipment', 'racket'])

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
 * Get the segment group for a sport type. Returns 'generic' for unknown types.
 */
export function getSegmentGroup(sportType: string): 'bike' | 'foot' | 'swim' | 'ski' | 'paddle' | 'generic' {
  return sportMap.get(sportType)?.segmentGroup ?? 'generic'
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
