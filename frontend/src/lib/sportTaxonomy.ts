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

export interface SportTuple {
  sportType: string
  subType: string
  modifiers: {
    isElectric?: boolean
    isIndoor?: boolean
  }
}

export interface SportLabelOptions {
  mode?: 'compact' | 'withParent'
}

export interface SportOption {
  value: string
  label: string
}

export interface WaterSportUIGroup {
  key: string
  label: string
  segmentGroups: SportDef['segmentGroup'][]
}

export interface RouteSportOption {
  sportType: string
  subType?: string
  label: string
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

export const WATER_SPORT_UI_GROUPS: WaterSportUIGroup[] = [
  { key: 'water_swim', label: 'Schwimmen', segmentGroups: ['swim'] },
  { key: 'water_paddle', label: 'Paddeln', segmentGroups: ['paddle'] },
  { key: 'water_surf', label: 'Surfen & Segeln', segmentGroups: ['generic'] },
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
    sub('street', 'Straße'),
    sub('trail', 'Trail'),
    sub('track', 'Bahn'),
    sub('treadmill', 'Laufband'),
    sub('indoor_running', 'Indoor'),
    sub('ultra', 'Ultra'),
    sub('obstacle', 'Hindernislauf'),
  ]),
  def('walking', 'run', 'foot', 'Gehen', true, false, [
    sub('casual_walking', 'Spaziergang'),
    sub('speed_walking', 'Speed Walking'),
    sub('indoor_walking', 'Indoor'),
  ]),
  def('hiking', 'run', 'foot', 'Wandern', true),
  def('mountaineering', 'run', 'foot', 'Bergsteigen', true),
  def('snowshoeing', 'run', 'foot', 'Schneeschuhwandern', true),
  def('floor_climbing', 'run', 'foot', 'Treppensteigen', false, true),
  def('wheelchair_push_walk', 'run', 'foot', 'Rollstuhl (Fahren)', true, false, [
    sub('indoor_wheelchair_walk', 'Indoor'),
  ]),
  def('wheelchair_push_run', 'run', 'foot', 'Rollstuhl (Rennen)', true, false, [
    sub('indoor_wheelchair_run', 'Indoor'),
  ]),

  // ── bike ──
  def('cycling', 'bike', 'bike', 'Radfahren', true, false, [
    sub('road', 'Rennrad'),
    sub('mountain', 'MTB'),
    sub('downhill', 'Downhill'),
    sub('cyclocross', 'Cyclocross'),
    sub('track_cycling', 'Bahnrad'),
    sub('bmx', 'BMX'),
    sub('gravel_cycling', 'Gravel'),
    sub('mixed_surface', 'Mixed'),
    sub('indoor_cycling', 'Indoor'),
    sub('spin', 'Spinning'),
    sub('hand_cycling', 'Handbike'),
    sub('recumbent', 'Liegerad'),
    sub('unicycling', 'Einrad'),
  ]),
  def('inline_skating', 'outdoor', 'generic', 'Inline-Skaten', true),

  // ── winter ──
  def('alpine_skiing', 'winter', 'ski', 'Alpin-Ski', true, false, [
    sub('resort', 'Piste'),
    sub('backcountry', 'Freeride'),
    sub('indoor_skiing', 'Indoor-Ski'),
  ]),
  def('backcountry_skiing', 'winter', 'ski', 'Skitour', true),
  def('cross_country_skiing', 'winter', 'ski', 'Langlauf', true, false, [
    sub('skate_skiing', 'Skating'),
  ]),
  def('snowboarding', 'winter', 'ski', 'Snowboard', true, false, [
    sub('backcountry', 'Backcountry'),
    sub('resort', 'Piste'),
  ]),
  def('ice_skating', 'winter', 'generic', 'Eislaufen', true),
  def('snowmobiling', 'winter', 'generic', 'Schneemobil', true),
  def('sledding', 'winter', 'generic', 'Rodeln', true),
  def('skiing', 'winter', 'ski', 'Skifahren', true),

  // ── water ──
  def('swimming', 'water', 'swim', 'Schwimmen', true, false, [
    sub('lap_swimming', 'Bahnschwimmen'),
    sub('open_water', 'Freiwasser'),
  ]),
  def('paddling', 'water', 'paddle', 'Paddeln (Generisch)', true),
  def('kayaking', 'water', 'paddle', 'Kajak', true, false, [
    sub('whitewater', 'Wildwasser'),
  ]),
  def('stand_up_paddleboarding', 'water', 'paddle', 'SUP', true),
  def('rowing', 'water', 'paddle', 'Rudern', true, false, [
    sub('indoor_rowing', 'Indoor-Rudern'),
  ]),
  def('rafting', 'water', 'paddle', 'Rafting', true, false, [
    sub('whitewater', 'Wildwasser'),
  ]),
  def('canoeing', 'water', 'paddle', 'Kanu', true),
  def('surfing', 'water', 'generic', 'Surfen', true),
  def('kitesurfing', 'water', 'generic', 'Kitesurfen', true),
  def('windsurfing', 'water', 'generic', 'Windsurfen', true),
  def('wakeboarding', 'water', 'generic', 'Wakeboard', true),
  def('wakesurfing', 'water', 'generic', 'Wakesurfen', true),
  def('water_skiing', 'water', 'generic', 'Wasserski', true),
  def('water_tubing', 'water', 'generic', 'Wassertubing', true),
  def('diving', 'water', 'generic', 'Tauchen', false, false, [
    sub('single_gas_diving', 'Eingas'),
    sub('multi_gas_diving', 'Mehrgas'),
    sub('gauge_diving', 'Gauge'),
    sub('apnea_diving', 'Apnoe'),
    sub('apnea_hunting', 'Apnoe-Jagd'),
    sub('apnea_pool', 'Apnoe Pool'),
    sub('ccr_diving', 'CCR'),
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
    sub('flexibility_training', 'Stretching'),
    sub('cardio_training', 'Cardio'),
    sub('yoga', 'Yoga'),
    sub('breathing', 'Atemübung'),
    sub('hiit', 'HIIT'),
  ]),
  def('fitness_equipment', 'fitness', 'generic', 'Fitnessgerät', false, true, [
    sub('treadmill', 'Laufband'),
    sub('indoor_cycling', 'Indoor-Rad'),
    sub('indoor_rowing', 'Ruder-Ergometer'),
    sub('elliptical', 'Crosstrainer'),
    sub('stair_climbing', 'Treppensteiger'),
    sub('pilates', 'Pilates'),
    sub('indoor_skiing', 'Indoor-Ski'),
    sub('indoor_walking', 'Indoor-Gehen'),
    sub('indoor_hand_cycling', 'Handbike-Ergometer'),
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
  def('soccer', 'game', 'generic', 'Fußball', false),
  def('basketball', 'game', 'generic', 'Basketball', false),
  def('american_football', 'game', 'generic', 'American Football', false),
  def('rugby', 'game', 'generic', 'Rugby', false),
  def('lacrosse', 'game', 'generic', 'Lacrosse', false),
  def('hockey', 'game', 'generic', 'Hockey', false),
  def('volleyball', 'game', 'generic', 'Volleyball', false),
  def('tennis', 'game', 'generic', 'Tennis', false, false, [
    sub('warm_up', 'Aufwärmen'),
    sub('match', 'Match'),
    sub('exercise', 'Übung'),
  ]),
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
    sub('indoor_climbing', 'Indoor'),
    sub('bouldering', 'Bouldern'),
  ]),

  // ── outdoor ──
  def('flying', 'outdoor', 'generic', 'Fliegen', true, false, [
    sub('rc_drone', 'Drohne'),
    sub('wingsuit', 'Wingsuit'),
    sub('fly_canopy', 'Fallschirm'),
    sub('fly_paraglide', 'Gleitschirm'),
    sub('fly_paramotor', 'Paramotor'),
    sub('fly_pressurized', 'Druckkabine'),
    sub('fly_vfr', 'Sichtflug (VFR)'),
    sub('fly_ifr', 'Instrumentenflug (IFR)'),
    sub('fly_navigate', 'Navigation'),
    sub('fly_timer', 'Timer'),
    sub('fly_altimeter', 'Höhenmesser'),
    sub('fly_wx', 'Wetter'),
  ]),
  def('hang_gliding', 'outdoor', 'generic', 'Gleitschirmfliegen', true),
  def('sky_diving', 'outdoor', 'generic', 'Fallschirmspringen', true),
  def('jumpmaster', 'outdoor', 'generic', 'Jumpmaster', true),
  def('horseback_riding', 'outdoor', 'generic', 'Reiten', true),
  def('hunting', 'outdoor', 'generic', 'Jagd', true),
  def('motorcycling', 'outdoor', 'generic', 'Motorrad', true, false, [
    sub('atv', 'Quad/ATV'),
    sub('motocross', 'Motocross'),
  ]),
  def('driving', 'outdoor', 'generic', 'Autofahren', true),
  def('boxing', 'fitness', 'generic', 'Boxen', false, true),
  def('mixed_martial_arts', 'fitness', 'generic', 'MMA', false, true),
  def('tactical', 'outdoor', 'generic', 'Taktisch', false),

  // ── multi ──
  def('multisport', 'multi', 'generic', 'Multisport', true),
  def('transition', 'multi', 'generic', 'Wechselzone', true, false, [
    sub('bike_to_run_transition', 'Rad→Lauf'),
    sub('run_to_bike_transition', 'Lauf→Rad'),
    sub('swim_to_bike_transition', 'Schwimmen→Rad'),
  ]),

  // ── other ──
  def('generic', 'other', 'generic', 'Aktivität', false),
  def('unknown', 'other', 'generic', 'Unbekannt', false),
]

const LEGACY_ALIASES = new Map<string, SportTuple>([
  ['mountain_biking', { sportType: 'cycling', subType: 'mountain', modifiers: {} }],
  ['gravel_cycling', { sportType: 'cycling', subType: 'gravel_cycling', modifiers: {} }],
  ['indoor_cycling', { sportType: 'cycling', subType: 'indoor_cycling', modifiers: { isIndoor: true } }],
  ['trail_running', { sportType: 'running', subType: 'trail', modifiers: {} }],
  ['treadmill_running', { sportType: 'running', subType: 'treadmill', modifiers: { isIndoor: true } }],
  ['lap_swimming', { sportType: 'swimming', subType: 'lap_swimming', modifiers: {} }],
  ['open_water_swimming', { sportType: 'swimming', subType: 'open_water', modifiers: {} }],
  ['road_cycling', { sportType: 'cycling', subType: 'road', modifiers: {} }],
  ['e_mountain_biking', { sportType: 'cycling', subType: 'mountain', modifiers: { isElectric: true } }],
  ['e_biking', { sportType: 'cycling', subType: '', modifiers: { isElectric: true } }],
  ['e_bike', { sportType: 'cycling', subType: '', modifiers: { isElectric: true } }],
])

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

export function normalizeSportTuple(sportType: string, subType: string | null = null): SportTuple {
  const trimmedSport = sportType.trim()
  const trimmedSub = subType?.trim() ?? ''
  const alias = LEGACY_ALIASES.get(trimmedSport)
  const tuple: SportTuple = alias
    ? {
        sportType: alias.sportType,
        subType: trimmedSub && trimmedSub !== 'generic' ? trimmedSub : alias.subType,
        modifiers: { ...alias.modifiers },
      }
    : {
        sportType: trimmedSport,
        subType: trimmedSub,
        modifiers: {},
      }

  if (tuple.sportType === 'running' && tuple.subType === 'treadmill') {
    tuple.modifiers.isIndoor = true
  }
  if (tuple.subType === 'generic') {
    tuple.subType = ''
  }

  return tuple
}

/**
 * Get sport definition. Returns undefined for unknown sport types.
 */
export function getSportDef(sportType: string): SportDef | undefined {
  return sportMap.get(sportType.trim())
}

/**
 * Get the segment group for a sport type. Returns 'generic' for unknown types.
 */
export function getSegmentGroup(sportType: string): 'bike' | 'foot' | 'swim' | 'ski' | 'paddle' | 'generic' {
  return getSportDef(sportType)?.segmentGroup ?? 'generic'
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
export function sportLabel(sportType: string, subType?: string | null, opts?: SportLabelOptions): string {
  const trimmedSport = sportType.trim()
  if (LEGACY_ALIASES.has(trimmedSport)) {
    return `Veraltete Sportart (${humanize(trimmedSport)})`
  }
  const tuple = { sportType: trimmedSport, subType: subType?.trim() ?? '' }
  const d = sportMap.get(tuple.sportType)
  if (!d) return humanize(sportType)

  if (tuple.subType && tuple.subType !== 'generic') {
    const st = d.subTypes.find((s) => s.subType === tuple.subType)
    if (st) {
      if (opts?.mode === 'withParent' && !subTypeLabelCanStandAlone(d)) {
        return `${d.label} (${st.label})`
      }
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
  return sportLabel(sportType, subType, { mode: 'withParent' })
}

function subTypeLabelCanStandAlone(d: SportDef): boolean {
  return d.sportType === 'training' || d.sportType === 'fitness_equipment' || d.sportType === 'racket'
}

/**
 * Category color for a sport type. Returns gray (#6B7280) for unknown types.
 */
export function sportColor(sportType: string): string {
  const d = getSportDef(sportType)
  if (!d) return '#6B7280'
  return categoryMap.get(d.categoryKey)?.color ?? '#6B7280'
}

/**
 * Get category for a sport type. Defaults to the 'other' category for unknowns.
 */
export function categoryForSport(sportType: string): SportCategory {
  const d = getSportDef(sportType)
  const cat = d ? categoryMap.get(d.categoryKey) : undefined
  return cat ?? { key: 'other', label: 'Sonstiges', color: '#6B7280' }
}

/**
 * Get all sport definitions in a category.
 */
export function sportsInCategory(categoryKey: string): SportDef[] {
  return catSportsMap.get(categoryKey) ?? []
}

export function canonicalSportOptions(): SportOption[] {
  return SPORT_DEFS.map((s) => ({ value: s.sportType, label: s.label }))
}

const ROUTE_PROMOTED_SUBTYPES: Record<string, string[]> = {
  cycling: ['mountain', 'gravel_cycling', 'road'],
  running: ['trail'],
}

export function routeSportOptions(): RouteSportOption[] {
  return SPORT_DEFS
    .filter((s) => s.typicallyHasGPS && !s.isIndoor)
    .flatMap((s) => {
      const options: RouteSportOption[] = [{ sportType: s.sportType, label: s.label }]
      for (const subType of ROUTE_PROMOTED_SUBTYPES[s.sportType] ?? []) {
        if (s.subTypes.some((st) => st.subType === subType)) {
          options.push({ sportType: s.sportType, subType, label: sportLabel(s.sportType, subType) })
        }
      }
      return options
    })
}
