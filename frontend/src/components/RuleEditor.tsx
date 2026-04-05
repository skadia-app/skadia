import { useRef, useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchGear } from '@/api/gear'
import { fetchTags } from '@/api/tags'
import { fetchDevices } from '@/api/hardware'
import type { Device } from '@/api/hardware'
import type { AutomationRule, RuleCondition, RuleConditionGroup, RuleAction, RuleBranch } from '@/api/types'
import { SPORT_DEFS } from '@/lib/sportTaxonomy'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import ZoneEditor, { type ZoneData } from '@/components/ZoneEditor'

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const SPORT_OPTIONS = SPORT_DEFS.map(d => ({
  value: d.sportType,
  label: d.label,
  category: d.categoryKey,
}))



const SOURCE_OPTIONS = [
  { value: 'garmin', label: 'Garmin' },
  { value: 'wahoo', label: 'Wahoo' },
  { value: 'strava', label: 'Strava' },
  { value: 'komoot', label: 'Komoot' },
  { value: 'endomondo', label: 'Endomondo' },
  { value: 'osmtracker', label: 'OSMTracker' },
  { value: 'manual', label: 'Manuell' },
]

const SEASON_OPTIONS = [
  { value: 'spring', label: 'Frühling (Mär–Mai)' },
  { value: 'summer', label: 'Sommer (Jun–Aug)' },
  { value: 'autumn', label: 'Herbst (Sep–Nov)' },
  { value: 'winter', label: 'Winter (Dez–Feb)' },
]

const WEEKDAY_OPTIONS = [
  { value: '1', label: 'Mo' },
  { value: '2', label: 'Di' },
  { value: '3', label: 'Mi' },
  { value: '4', label: 'Do' },
  { value: '5', label: 'Fr' },
  { value: '6', label: 'Sa' },
  { value: '0', label: 'So' },
]

const VISIBILITY_OPTIONS = [
  { value: 'private',    label: 'Privat' },
  { value: 'bekannte',   label: 'Bekannte' },
  { value: 'instanz',    label: 'Instanz' },
  { value: 'federation', label: 'Federation' },
  { value: 'public',     label: 'Öffentlich' },
]

// Field definitions: each entry tells the editor how to render the value input
interface ConditionField {
  field: string
  label: string
  inputType: 'number' | 'text' | 'bool' | 'enum' | 'tag' | 'segment' | 'geofence'
  unit?: string
  options?: { value: string; label: string }[]
  /** Override available operators for this specific field */
  operators?: string[]
}

interface ConditionCategory {
  key: string
  label: string
  fields: ConditionField[]
}

const MONTH_OPTIONS = [
  { value: '1', label: 'Januar' }, { value: '2', label: 'Februar' },
  { value: '3', label: 'März' }, { value: '4', label: 'April' },
  { value: '5', label: 'Mai' }, { value: '6', label: 'Juni' },
  { value: '7', label: 'Juli' }, { value: '8', label: 'August' },
  { value: '9', label: 'September' }, { value: '10', label: 'Oktober' },
  { value: '11', label: 'November' }, { value: '12', label: 'Dezember' },
]
const INTENSITY_OPTIONS = [
  { value: 'locker', label: 'Locker' },
  { value: 'moderat', label: 'Moderat' },
  { value: 'intensiv', label: 'Intensiv' },
]
const TIME_OF_DAY_OPTIONS = [
  { value: 'nacht', label: 'Nacht' },
  { value: 'frueh', label: 'Früh' },
  { value: 'morgen', label: 'Morgen' },
  { value: 'mittag', label: 'Mittag' },
  { value: 'nachmittag', label: 'Nachmittag' },
  { value: 'abend', label: 'Abend' },
]

const CONDITION_CATEGORIES: ConditionCategory[] = [
  { key: 'title', label: 'Titel', fields: [{ field: 'title', label: 'Titel', inputType: 'text' }] },
  { key: 'description', label: 'Beschreibung', fields: [{ field: 'description', label: 'Beschreibung', inputType: 'text' }] },
  { key: 'sport', label: 'Sportart', fields: [
    { field: 'sport_type', label: 'Sportart', inputType: 'enum', options: SPORT_OPTIONS },
    { field: 'sub_type', label: 'Sub-Typ', inputType: 'text' },
  ]},
  { key: 'activity', label: 'Aktivität', fields: [
    { field: 'source', label: 'Quelle', inputType: 'enum', options: SOURCE_OPTIONS },
    { field: 'intensity', label: 'Intensität', inputType: 'enum', options: INTENSITY_OPTIONS },
    { field: 'is_indoor', label: 'Indoor', inputType: 'bool' },
    { field: 'has_gps', label: 'Hat GPS', inputType: 'bool' },
    { field: 'visibility', label: 'Sichtbarkeit', inputType: 'enum', options: VISIBILITY_OPTIONS },
  ]},
  { key: 'time', label: 'Zeit', fields: [
    { field: 'elapsed_time_s', label: 'Gesamtzeit', inputType: 'number', unit: 's' },
    { field: 'moving_time_s', label: 'Bewegungszeit', inputType: 'number', unit: 's' },
    { field: 'weekday', label: 'Wochentag', inputType: 'enum', options: WEEKDAY_OPTIONS },
    { field: 'month', label: 'Monat', inputType: 'enum', options: MONTH_OPTIONS },
    { field: 'season', label: 'Jahreszeit', inputType: 'enum', options: SEASON_OPTIONS },
    { field: 'time_of_day', label: 'Tageszeit', inputType: 'enum', options: TIME_OF_DAY_OPTIONS },
    { field: 'sun_elevation', label: 'Sonnenstand', inputType: 'number', unit: '°' },
  ]},
  { key: 'distance', label: 'Distanz', fields: [
    { field: 'distance_m', label: 'Distanz', inputType: 'number', unit: 'm' },
    { field: 'start_end_distance_m', label: 'Start-End-Abstand', inputType: 'number', unit: 'm' },
  ]},
  { key: 'speed', label: 'Geschwindigkeit', fields: [
    { field: 'avg_speed_ms', label: 'Ø Geschwindigkeit', inputType: 'number', unit: 'm/s' },
    { field: 'max_speed_ms', label: 'Max. Geschwindigkeit', inputType: 'number', unit: 'm/s' },
    { field: 'min_speed_ms', label: 'Min. Geschwindigkeit', inputType: 'number', unit: 'm/s' },
  ]},
  { key: 'pace', label: 'Tempo', fields: [
    { field: 'avg_pace_min_km', label: 'Ø Tempo', inputType: 'number', unit: 'min/km' },
    { field: 'min_pace_min_km', label: 'Bestes Tempo', inputType: 'number', unit: 'min/km' },
    { field: 'max_pace_min_km', label: 'Langsamstes Tempo', inputType: 'number', unit: 'min/km' },
  ]},
  { key: 'elevation', label: 'Anstiege', fields: [
    { field: 'elevation_gain_m', label: 'Höhenmeter aufwärts', inputType: 'number', unit: 'm' },
    { field: 'elevation_loss_m', label: 'Höhenmeter abwärts', inputType: 'number', unit: 'm' },
    { field: 'max_elevation_m', label: 'Max. Höhe', inputType: 'number', unit: 'm' },
    { field: 'min_elevation_m', label: 'Min. Höhe', inputType: 'number', unit: 'm' },
    { field: 'climb_count', label: 'Anzahl Anstiege', inputType: 'number' },
    { field: 'climb_category_max', label: 'Härteste Kategorie', inputType: 'number' },
  ]},
  { key: 'heartrate', label: 'Herzfrequenz', fields: [
    { field: 'avg_heart_rate', label: 'Ø HF', inputType: 'number', unit: 'bpm' },
    { field: 'max_heart_rate', label: 'Max. HF', inputType: 'number', unit: 'bpm' },
    { field: 'min_heart_rate', label: 'Min. HF', inputType: 'number', unit: 'bpm' },
  ]},
  { key: 'cadence', label: 'Kadenz', fields: [
    { field: 'avg_cadence', label: 'Ø Kadenz', inputType: 'number', unit: 'rpm' },
    { field: 'max_cadence', label: 'Max. Kadenz', inputType: 'number', unit: 'rpm' },
  ]},
  { key: 'power', label: 'Leistung', fields: [
    { field: 'avg_power_w', label: 'Ø Leistung', inputType: 'number', unit: 'W' },
    { field: 'max_power_w', label: 'Max. Leistung', inputType: 'number', unit: 'W' },
    { field: 'min_power_w', label: 'Min. Leistung', inputType: 'number', unit: 'W' },
    { field: 'normalized_power_w', label: 'Normalisierte Leistung', inputType: 'number', unit: 'W' },
    { field: 'tss', label: 'TSS', inputType: 'number' },
    { field: 'trimp', label: 'TRIMP', inputType: 'number' },
  ]},
  { key: 'sensor_temp', label: 'Sensor-Temperatur', fields: [
    { field: 'avg_temperature_c', label: 'Ø Temperatur', inputType: 'number', unit: '°C' },
    { field: 'min_temperature_c', label: 'Min. Temperatur', inputType: 'number', unit: '°C' },
    { field: 'max_temperature_c', label: 'Max. Temperatur', inputType: 'number', unit: '°C' },
  ]},
  { key: 'weather', label: 'Wetter', fields: [
    { field: 'weather_temp_c', label: 'Wetter-Temperatur', inputType: 'number', unit: '°C' },
    { field: 'weather_temp_min_c', label: 'Min. Wetter-Temp.', inputType: 'number', unit: '°C' },
    { field: 'weather_temp_max_c', label: 'Max. Wetter-Temp.', inputType: 'number', unit: '°C' },
    { field: 'weather_condition', label: 'Wetterlage', inputType: 'text' },
    { field: 'weather_humidity', label: 'Luftfeuchtigkeit', inputType: 'number', unit: '%' },
    { field: 'weather_wind_speed_ms', label: 'Windgeschwindigkeit', inputType: 'number', unit: 'm/s' },
    { field: 'weather_wind_gusts_ms', label: 'Windböen', inputType: 'number', unit: 'm/s' },
  ]},
  { key: 'location', label: 'Ort', fields: [
    { field: 'start_city', label: 'Startort', inputType: 'text' },
    { field: 'start_state', label: 'Bundesland/Region', inputType: 'text' },
    { field: 'start_country', label: 'Land', inputType: 'text' },
    { field: 'start_country_code', label: 'Ländercode', inputType: 'text' },
    { field: 'end_city', label: 'Endort', inputType: 'text' },
    { field: 'geofence', label: 'Geofence', inputType: 'geofence' },
  ]},
  { key: 'context', label: 'Kontext', fields: [
    { field: 'gear_id', label: 'Ausrüstung', inputType: 'enum', options: [] },
    { field: 'has_tag', label: 'Hat Tag', inputType: 'tag' },
    { field: 'photo_count', label: 'Anzahl Fotos', inputType: 'number' },
    { field: 'detected_lap_count', label: 'Anzahl Runden', inputType: 'number' },
    { field: 'hide_map', label: 'Karte verborgen', inputType: 'bool' },
  ]},
  { key: 'lift', label: 'Lift', fields: [
    { field: 'has_lift', label: 'Lift genutzt', inputType: 'bool' },
    { field: 'lift_count', label: 'Anzahl Lifte', inputType: 'number' },
    { field: 'lift_ascent_m', label: 'Lift-Aufstieg', inputType: 'number', unit: 'm' },
    { field: 'lift_descent_m', label: 'Lift-Abstieg', inputType: 'number', unit: 'm' },
    { field: 'lift_distance_m', label: 'Lift-Distanz', inputType: 'number', unit: 'm' },
    { field: 'lift_duration_s', label: 'Lift-Dauer', inputType: 'number', unit: 's' },
    { field: 'net_ascent_m', label: 'Netto-Aufstieg', inputType: 'number', unit: 'm' },
    { field: 'net_descent_m', label: 'Netto-Abstieg', inputType: 'number', unit: 'm' },
    { field: 'net_distance_m', label: 'Netto-Distanz', inputType: 'number', unit: 'm' },
  ]},
  { key: 'segments', label: 'Segmente', fields: [
    { field: 'has_segment_effort', label: 'Segment gefahren', inputType: 'segment' },
    { field: 'segment_effort_rank', label: 'Segment-Platzierung', inputType: 'segment' },
    { field: 'segment_effort_time_s', label: 'Segment-Zeit', inputType: 'segment' },
  ]},
  { key: 'device', label: 'Geräte & Sensoren', fields: [
    {
      field: 'recording_device_id',
      label: 'Aufzeichnungsgerät',
      inputType: 'enum',
      options: [], // dynamically filled via devices query in RuleEditor
    },
    { field: 'recording_device_product_name', label: 'Gerätename', inputType: 'text' },
    { field: 'recording_device_manufacturer', label: 'Hersteller', inputType: 'text' },
    {
      field: 'has_sensor_type',
      label: 'Sensortyp vorhanden',
      inputType: 'enum',
      options: [
        { value: 'heart_rate', label: 'Herzfrequenz' },
        { value: 'bike_power', label: 'Leistungsmesser' },
        { value: 'bike_cadence', label: 'Trittfrequenz' },
        { value: 'bike_speed', label: 'Geschwindigkeit' },
        { value: 'bike_speed_cadence', label: 'Geschwindigkeit/Trittfrequenz' },
        { value: 'stride_speed_distance', label: 'Laufpod' },
        { value: 'bike_radar', label: 'Rückwärtsradar' },
        { value: 'shifting', label: 'Schaltung (Di2/eTap)' },
        { value: 'env_sensor', label: 'Umgebungssensor' },
        { value: 'muscle_oxygen', label: 'Muskel-Sauerstoff' },
      ],
      operators: ['contains', 'not_contains'],
    },
    {
      field: 'sensor_device_ids',
      label: 'Konkreter Sensor',
      inputType: 'enum',
      options: [], // dynamically filled via devices query in RuleEditor
      operators: ['contains'],
    },
  ]},
  { key: 'enrichment', label: 'Enrichment', fields: [
    { field: 'has_summit', label: 'Gipfel erkannt', inputType: 'bool' },
    { field: 'summit_count', label: 'Anzahl Gipfel', inputType: 'number' },
    { field: 'has_pass', label: 'Pass erkannt', inputType: 'bool' },
    { field: 'border_crossing_count', label: 'Grenzquerungen', inputType: 'number' },
    { field: 'has_gastronomy_stop', label: 'Einkehr erkannt', inputType: 'bool' },
    { field: 'stop_count', label: 'Anzahl POI-Stops', inputType: 'number' },
    { field: 'commute_detected', label: 'Ist Pendeln', inputType: 'bool' },
    { field: 'has_structured_intervals', label: 'Intervalle erkannt', inputType: 'bool' },
    { field: 'interval_count', label: 'Anzahl Intervalle', inputType: 'number' },
    { field: 'has_hc_climb', label: 'HC-Anstieg', inputType: 'bool' },
  ]},
  { key: 'surface', label: 'Oberfläche', fields: [
    { field: 'surface_paved_pct', label: 'Asphalt-Anteil', inputType: 'number', unit: '%' },
    { field: 'surface_gravel_pct', label: 'Gravel-Anteil', inputType: 'number', unit: '%' },
    { field: 'surface_trail_pct', label: 'Trail-Anteil', inputType: 'number', unit: '%' },
    { field: 'surface_unknown_pct', label: 'Unbekannt-Anteil', inputType: 'number', unit: '%' },
  ]},
  { key: 'swimming', label: 'Schwimmen', fields: [
    { field: 'swim_stroke', label: 'Schwimmstil', inputType: 'enum', options: [
      { value: 'freestyle', label: 'Kraul' },
      { value: 'backstroke', label: 'Rücken' },
      { value: 'breaststroke', label: 'Brust' },
      { value: 'butterfly', label: 'Schmetterling' },
      { value: 'mixed', label: 'Gemischt' },
    ]},
    { field: 'pool_length_m', label: 'Bahnlänge', inputType: 'number', unit: 'm' },
    { field: 'num_active_lengths', label: 'Aktive Bahnen', inputType: 'number' },
  ]},
  { key: 'diving', label: 'Tauchen', fields: [
    { field: 'max_depth_m', label: 'Max. Tiefe', inputType: 'number', unit: 'm' },
    { field: 'avg_depth_m', label: 'Ø Tiefe', inputType: 'number', unit: 'm' },
    { field: 'end_cns_pct', label: 'End-CNS', inputType: 'number', unit: '%' },
  ]},
  { key: 'mtb', label: 'MTB', fields: [
    { field: 'jump_count', label: 'Sprünge', inputType: 'number' },
    { field: 'total_grit', label: 'Grit (Garmin)', inputType: 'number' },
    { field: 'avg_flow', label: 'Flow (Garmin)', inputType: 'number' },
  ]},
  { key: 'ebike', label: 'E-Bike', fields: [
    { field: 'avg_lev_motor_power_w', label: 'Ø Motorleistung', inputType: 'number', unit: 'W' },
    { field: 'lev_battery_consumption_pct', label: 'Batterieverbrauch', inputType: 'number', unit: '%' },
  ]},
  { key: 'multisport', label: 'Multisport', fields: [
    { field: 'is_multisport_child', label: 'Ist Sub-Session', inputType: 'bool' },
    { field: 'is_multisport_parent', label: 'Ist Multisport-Event', inputType: 'bool' },
  ]},
  { key: 'training', label: 'Training-Metriken', fields: [
    { field: 'aerobic_training_effect', label: 'Aerobes TE', inputType: 'number' },
    { field: 'anaerobic_training_effect', label: 'Anaerobes TE', inputType: 'number' },
    { field: 'aerobic_decoupling', label: 'Aerobic Decoupling', inputType: 'number', unit: '%' },
    { field: 'variability_index', label: 'Variability Index', inputType: 'number' },
    { field: 'efficiency_factor', label: 'Efficiency Factor', inputType: 'number' },
    { field: 'intensity_factor', label: 'Intensity Factor', inputType: 'number' },
    { field: 'workout_rpe', label: 'RPE (subjektiv)', inputType: 'number' },
    { field: 'workout_feel', label: 'Gefühl (subjektiv)', inputType: 'number' },
  ]},
]

// Internal-only fields not shown in category dropdown
const INTERNAL_FIELDS: ConditionField[] = []

// Flat lookup: find a ConditionField by its field name
const ALL_CONDITION_FIELDS: ConditionField[] = [
  ...CONDITION_CATEGORIES.flatMap(c => c.fields),
  ...INTERNAL_FIELDS,
]

function findFieldDef(field: string): ConditionField | undefined {
  return ALL_CONDITION_FIELDS.find(f => f.field === field)
}

function findCategory(field: string): ConditionCategory | undefined {
  return CONDITION_CATEGORIES.find(cat => cat.fields.some(f => f.field === field))
}

// Operators per input type
function opsForField(cf: ConditionField): string[] {
  // Field-level override takes precedence
  if (cf.operators && cf.operators.length > 0) return cf.operators
  switch (cf.inputType) {
    case 'bool': return ['eq']
    case 'enum': return ['eq', 'neq', 'in']
    case 'text': return ['eq', 'neq', 'contains', 'not_contains', 'starts_with', 'ends_with']
    case 'number': return ['gt', 'lt', 'gte', 'lte', 'eq', 'between']
    case 'tag': return ['eq', 'neq']
    case 'segment': return ['eq']
    case 'geofence': return ['eq']
  }
}

const OP_LABELS: Record<string, string> = {
  eq: '=', neq: '!=', gt: '>', lt: '<', gte: '>=', lte: '<=',
  contains: 'enthält', not_contains: 'enthält nicht',
  starts_with: 'beginnt mit', ends_with: 'endet mit', in: 'in',
  between: 'zwischen',
}

const ACTION_TYPES = [
  { value: 'set_gear',        label: 'Ausrüstung zuweisen' },
  { value: 'add_tag',         label: 'Tag hinzufügen' },
  { value: 'remove_tag',      label: 'Tag entfernen' },
  { value: 'set_visibility',  label: 'Sichtbarkeit setzen' },
  { value: 'set_sport_type',  label: 'Sportart setzen' },
  { value: 'set_name',        label: 'Titel setzen' },
  { value: 'set_intensity',   label: 'Intensität setzen' },
  { value: 'set_indoor',      label: 'Indoor-Flag setzen' },
  { value: 'set_exclude_from_stats',   label: 'Aus Statistiken ausschließen' },
  { value: 'set_exclude_from_heatmap', label: 'Aus Heatmap ausschließen' },
  { value: 'set_hide_map',             label: 'Karte verstecken' },
]

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

function defaultCondition(): RuleCondition {
  return { field: 'sport_type', op: 'eq', value: '' }
}

function defaultGroup(): RuleConditionGroup {
  return { logic: 'and', conditions: [defaultCondition()] }
}

/** Normalise a rule to always have condition_groups (migrate flat conditions if needed) */
function normaliseGroups(rule?: AutomationRule | null): RuleConditionGroup[] {
  if (rule?.condition_groups && rule.condition_groups.length > 0) {
    return rule.condition_groups
  }
  if (rule?.conditions && rule.conditions.length > 0) {
    return [{ logic: rule.conditions_logic ?? 'and' as 'and' | 'or', conditions: rule.conditions }]
  }
  return [defaultGroup()]
}

/** Flatten groups to a single condition list + logic for legacy API compat */
function flattenGroups(groups: RuleConditionGroup[]): { conditions: RuleCondition[]; conditions_logic: 'and' | 'or' } {
  const all: RuleCondition[] = groups.flatMap(g => g.conditions)
  return { conditions: all, conditions_logic: groups[0]?.logic ?? 'and' }
}

// ---------------------------------------------------------------------------
// Small UI bits
// ---------------------------------------------------------------------------

const inputClass = 'border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent'

// ---------------------------------------------------------------------------
// Unified Geofence components
// ---------------------------------------------------------------------------

interface UnifiedGeofenceValue {
  check: 'start' | 'end' | 'track'
  mode: 'inside' | 'outside'
  shape: 'circle' | 'polygon'
  lat?: number
  lon?: number
  radius_m?: number
  polygon?: string
}

// CircleGeofenceInner — pin + radius map (extracted from old GeofenceEditor)
function CircleGeofenceInner({
  lat,
  lon,
  radiusM,
  onChange,
}: {
  lat: number
  lon: number
  radiusM: number
  onChange: (lat: number, lon: number, radiusM: number) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)
  const circleSourceId = 'gfc-circle'
  const stateRef = useRef({ lat, lon, radiusM })
  stateRef.current = { lat, lon, radiusM }

  function buildCircleGeoJSON(clat: number, clon: number, r: number) {
    const points = 64
    const coords: [number, number][] = []
    const earthRadius = 6371000
    const angularDist = r / earthRadius
    const latRad = (clat * Math.PI) / 180
    const lonRad = (clon * Math.PI) / 180
    for (let i = 0; i <= points; i++) {
      const bearing = (i * 360) / points * (Math.PI / 180)
      const pLat = Math.asin(
        Math.sin(latRad) * Math.cos(angularDist) +
        Math.cos(latRad) * Math.sin(angularDist) * Math.cos(bearing)
      )
      const pLon = lonRad + Math.atan2(
        Math.sin(bearing) * Math.sin(angularDist) * Math.cos(latRad),
        Math.cos(angularDist) - Math.sin(latRad) * Math.sin(pLat)
      )
      coords.push([(pLon * 180) / Math.PI, (pLat * 180) / Math.PI])
    }
    return {
      type: 'Feature' as const,
      geometry: { type: 'Polygon' as const, coordinates: [coords] },
      properties: {},
    }
  }

  function updateCircle(map: maplibregl.Map, clat: number, clon: number, r: number) {
    const src = map.getSource(circleSourceId) as maplibregl.GeoJSONSource | undefined
    if (src) src.setData(buildCircleGeoJSON(clat, clon, r) as any)
  }

  useEffect(() => {
    if (!containerRef.current) return
    const map = new maplibregl.Map({
      container: containerRef.current,
      style: {
        version: 8,
        sources: {
          basemap: {
            type: 'raster',
            tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
            tileSize: 256,
            attribution: '&copy; OpenStreetMap',
          },
        },
        layers: [{ id: 'basemap-tiles', type: 'raster', source: 'basemap', minzoom: 0, maxzoom: 19 }],
      },
      center: [lon || 10.0, lat || 51.0],
      zoom: lat ? 11 : 5,
    })
    mapRef.current = map

    map.on('load', () => {
      map.addSource(circleSourceId, {
        type: 'geojson',
        data: buildCircleGeoJSON(lat || 51, lon || 10, radiusM || 1000) as any,
      })
      map.addLayer({
        id: 'gfc-fill',
        type: 'fill',
        source: circleSourceId,
        paint: { 'fill-color': '#f97316', 'fill-opacity': 0.2 },
      })
      map.addLayer({
        id: 'gfc-outline',
        type: 'line',
        source: circleSourceId,
        paint: { 'line-color': '#f97316', 'line-width': 2 },
      })

      const marker = new maplibregl.Marker({ color: '#f97316', draggable: true })
        .setLngLat([lon || 10.0, lat || 51.0])
        .addTo(map)
      markerRef.current = marker

      marker.on('dragend', () => {
        const lngLat = marker.getLngLat()
        const s = stateRef.current
        updateCircle(map, lngLat.lat, lngLat.lng, s.radiusM || 1000)
        onChange(lngLat.lat, lngLat.lng, s.radiusM || 1000)
      })

      map.on('click', (e) => {
        const { lat: clat, lng: clng } = e.lngLat
        marker.setLngLat([clng, clat])
        const s = stateRef.current
        updateCircle(map, clat, clng, s.radiusM || 1000)
        onChange(clat, clng, s.radiusM || 1000)
      })
    })

    return () => { map.remove() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    updateCircle(map, lat || 51, lon || 10, radiusM || 1000)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [radiusM])

  const radiusDisplay = radiusM >= 1000 ? `${(radiusM / 1000).toFixed(1)} km` : `${radiusM} m`

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 flex-wrap">
        <div className="flex items-center gap-2 flex-1 min-w-[260px]">
          <label className="text-xs text-neutral-600 dark:text-neutral-400 whitespace-nowrap">
            Radius: <span className="font-medium text-neutral-900 dark:text-neutral-100">{radiusDisplay}</span>
          </label>
          <input
            type="range"
            min={50}
            max={200000}
            step={50}
            value={radiusM}
            onChange={(e) => onChange(lat, lon, Number(e.target.value))}
            className="flex-1 accent-accent-DEFAULT"
          />
          <input
            type="number"
            min={50}
            step={50}
            value={radiusM}
            onChange={(e) => {
              const v = Math.max(50, Number(e.target.value) || 1000)
              onChange(lat, lon, v)
            }}
            className={inputClass + ' w-24 py-1 text-xs'}
            title="Radius in Metern"
          />
          <span className="text-xs text-neutral-400">m</span>
        </div>
        <div className="flex items-center gap-1 text-xs text-neutral-500">
          <span>Lat:</span>
          <span className="font-mono tabular-nums">{lat ? lat.toFixed(5) : '—'}</span>
          <span className="ml-2">Lon:</span>
          <span className="font-mono tabular-nums">{lon ? lon.toFixed(5) : '—'}</span>
        </div>
      </div>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">Klicke auf die Karte oder ziehe den Pin um den Mittelpunkt zu setzen.</p>
      <div ref={containerRef} className="w-full rounded-lg overflow-hidden" style={{ height: 260 }} />
    </div>
  )
}

// ZoneGeofenceInner — polygon drawing using ZoneEditor
function ZoneGeofenceInner({
  polygon,
  onChange,
}: {
  polygon?: string
  onChange: (polygon: string) => void
}) {
  const zoneData: ZoneData = (() => {
    if (polygon) {
      try {
        const poly = JSON.parse(polygon) as { type: 'Polygon'; coordinates: number[][][] }
        return { mode: 'polygon', polygon: poly }
      } catch {
        // fall through
      }
    }
    return { mode: 'polygon' }
  })()

  function handleZoneChange(zone: ZoneData) {
    if (zone.polygon) {
      onChange(JSON.stringify(zone.polygon))
    }
  }

  return (
    <ZoneEditor
      value={zoneData}
      onChange={handleZoneChange}
      minRadiusM={50}
      maxRadiusM={100000}
      enableHoles={false}
      className="text-sm"
    />
  )
}

// UnifiedGeofenceEditor — wraps CircleGeofenceInner / ZoneGeofenceInner with check/mode/shape selectors
function UnifiedGeofenceEditor({
  value,
  onChange,
}: {
  value: UnifiedGeofenceValue
  onChange: (v: UnifiedGeofenceValue) => void
}) {
  const selectClass = 'text-xs border border-neutral-300 dark:border-neutral-600 rounded px-1.5 py-1 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100'

  return (
    <div className="space-y-3 border border-neutral-200 dark:border-neutral-700 rounded-lg p-3 bg-neutral-50 dark:bg-neutral-800/30">
      <div className="flex gap-2 flex-wrap">
        <label className="flex items-center gap-1 text-xs text-neutral-500">
          Prüfe:
          <select
            value={value.check}
            onChange={(e) => onChange({ ...value, check: e.target.value as 'start' | 'end' | 'track' })}
            className={selectClass}
          >
            <option value="start">Startpunkt</option>
            <option value="end">Endpunkt</option>
            <option value="track">Track</option>
          </select>
        </label>
        <label className="flex items-center gap-1 text-xs text-neutral-500">
          Modus:
          <select
            value={value.mode}
            onChange={(e) => onChange({ ...value, mode: e.target.value as 'inside' | 'outside' })}
            className={selectClass}
          >
            <option value="inside">innerhalb</option>
            <option value="outside">außerhalb</option>
          </select>
        </label>
        <label className="flex items-center gap-1 text-xs text-neutral-500">
          Form:
          <select
            value={value.shape}
            onChange={(e) => onChange({ ...value, shape: e.target.value as 'circle' | 'polygon' })}
            className={selectClass}
          >
            <option value="circle">Kreis</option>
            <option value="polygon">Polygon</option>
          </select>
        </label>
      </div>
      {value.shape === 'polygon' ? (
        <ZoneGeofenceInner
          polygon={value.polygon}
          onChange={(polygon) => onChange({ ...value, polygon })}
        />
      ) : (
        <CircleGeofenceInner
          lat={value.lat ?? 51.0}
          lon={value.lon ?? 10.0}
          radiusM={value.radius_m ?? 1000}
          onChange={(clat, clon, r) => onChange({ ...value, lat: clat, lon: clon, radius_m: r })}
        />
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Condition value editor — renders the right input for each field type
// ---------------------------------------------------------------------------

function ConditionValueEditor({
  fieldDef,
  cond,
  onChange,
  tags,
  recorders,
  sensors,
}: {
  fieldDef: ConditionField
  cond: RuleCondition
  onChange: (patch: Partial<RuleCondition>) => void
  tags: Array<{ id: string; name: string; color: string }>
  recorders?: Device[]
  sensors?: Device[]
}) {
  const { inputType } = fieldDef

  // Resolve dynamic options for device fields
  function deviceOption(d: Device) {
    return { value: d.id, label: d.custom_name || d.product_name || d.manufacturer || d.id }
  }
  const resolvedFieldDef: ConditionField = (() => {
    if (fieldDef.field === 'recording_device_id' && recorders) {
      return { ...fieldDef, options: recorders.map(deviceOption) }
    }
    if (fieldDef.field === 'sensor_device_ids' && sensors) {
      return { ...fieldDef, options: sensors.map(deviceOption) }
    }
    return fieldDef
  })()
  const effectiveOptions = resolvedFieldDef.options ?? fieldDef.options

  if (inputType === 'bool') {
    return (
      <select
        value={String(cond.value ?? 'true')}
        onChange={(e) => onChange({ value: e.target.value === 'true' })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="true">Ja</option>
        <option value="false">Nein</option>
      </select>
    )
  }

  if (inputType === 'enum' && effectiveOptions) {
    if (cond.op === 'in' || cond.op === 'contains' || cond.op === 'not_contains') {
      const selected: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
      function toggle(v: string) {
        onChange({ value: selected.includes(v) ? selected.filter(x => x !== v) : [...selected, v] })
      }
      return (
        <div className="flex flex-wrap gap-1 flex-1 min-w-0">
          {effectiveOptions.map(s => (
            <button
              key={s.value}
              type="button"
              onClick={() => toggle(s.value)}
              className={`px-2 py-0.5 rounded text-xs font-medium border transition-colors cursor-pointer ${
                selected.includes(s.value)
                  ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                  : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:border-accent-400'
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
      )
    }
    return (
      <select
        value={String(cond.value ?? '')}
        onChange={(e) => onChange({ value: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Wählen...</option>
        {effectiveOptions.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
      </select>
    )
  }

  if (inputType === 'number' && cond.op === 'between') {
    const parts = typeof cond.value === 'string' ? cond.value.split(',') : ['', '']
    const from = parts[0] ?? ''
    const to = parts[1] ?? ''
    return (
      <div className="flex items-center gap-2 flex-1 min-w-0">
        <input type="number" placeholder="von" value={from}
          onChange={(e) => onChange({ value: `${e.target.value},${to}` })}
          className={inputClass + ' w-24'}
        />
        <span className="text-neutral-500 text-xs">–</span>
        <input type="number" placeholder="bis" value={to}
          onChange={(e) => onChange({ value: `${from},${e.target.value}` })}
          className={inputClass + ' w-24'}
        />
        {fieldDef.unit && <span className="text-xs text-neutral-400">{fieldDef.unit}</span>}
      </div>
    )
  }

  if (inputType === 'number') {
    return (
      <div className="flex items-center gap-1 flex-1 min-w-0">
        <input type="number" value={cond.value ?? ''}
          onChange={(e) => onChange({ value: Number(e.target.value) })}
          placeholder="Wert"
          className={inputClass + ' flex-1 min-w-0'}
        />
        {fieldDef.unit && <span className="text-xs text-neutral-400">{fieldDef.unit}</span>}
      </div>
    )
  }

  if (inputType === 'tag') {
    return (
      <select value={String(cond.value ?? '')} onChange={(e) => onChange({ value: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Tag wählen...</option>
        {tags.map(t => (
          <option key={t.id} value={t.name}>
            {t.name}
          </option>
        ))}
      </select>
    )
  }

  if (inputType === 'segment') {
    return (
      <input type="text" value={cond.value ?? ''}
        onChange={(e) => onChange({ value: e.target.value })}
        placeholder="Segment-ID"
        className={inputClass + ' flex-1 min-w-0'}
      />
    )
  }

  if (inputType === 'geofence') {
    const gfVal: UnifiedGeofenceValue = (cond.value && typeof cond.value === 'object' && cond.value.check)
      ? cond.value as UnifiedGeofenceValue
      : { check: 'start', mode: 'inside', shape: 'circle', lat: 51.0, lon: 10.0, radius_m: 1000 }
    return (
      <div className="flex-1 min-w-0 w-full">
        <UnifiedGeofenceEditor
          value={gfVal}
          onChange={(v) => onChange({ value: v })}
        />
      </div>
    )
  }

  // Default: free text
  return (
    <input type="text" value={cond.value ?? ''}
      onChange={(e) => onChange({ value: e.target.value })}
      placeholder="Wert"
      className={inputClass + ' flex-1 min-w-0'}
    />
  )
}

// ---------------------------------------------------------------------------
// Single condition row — two-level: Category → Field → Operator → Value
// ---------------------------------------------------------------------------

function ConditionRow({
  cond,
  onUpdate,
  onRemove,
  tags,
  recorders,
  sensors,
}: {
  cond: RuleCondition
  onUpdate: (patch: Partial<RuleCondition>) => void
  onRemove: () => void
  tags: Array<{ id: string; name: string; color: string }>
  recorders?: Device[]
  sensors?: Device[]
}) {
  const currentCat = findCategory(cond.field)
  const currentField = findFieldDef(cond.field)
  const isInternalField = !currentCat  // geofence, is_round_trip etc.

  // For internal fields (non-category), show a simplified row
  if (isInternalField && currentField) {
    return (
      <div className={currentField.inputType === 'geofence' ? 'space-y-2' : 'flex items-center gap-2 flex-wrap'}>
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">{currentField.label}</span>
        <ConditionValueEditor fieldDef={currentField} cond={cond} onChange={onUpdate} tags={tags} recorders={recorders} sensors={sensors} />
        <button type="button" onClick={onRemove}
          className="text-neutral-400 hover:text-rose-500 transition-colors flex-shrink-0 cursor-pointer"
          title="Bedingung entfernen"
        >
          <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
          </svg>
        </button>
      </div>
    )
  }

  const safeCat = currentCat ?? CONDITION_CATEGORIES[0]
  const safeField = currentField ?? safeCat.fields[0]
  const ops = opsForField(safeField)

  function handleCategoryChange(catKey: string) {
    const cat = CONDITION_CATEGORIES.find(c => c.key === catKey) ?? CONDITION_CATEGORIES[0]
    const firstField = cat.fields[0]
    const firstOps = opsForField(firstField)
    onUpdate({ field: firstField.field, op: firstOps[0], value: firstField.inputType === 'bool' ? true : '' })
  }

  function handleFieldChange(fieldName: string) {
    const fd = safeCat.fields.find(f => f.field === fieldName) ?? safeCat.fields[0]
    const fieldOps = opsForField(fd)
    onUpdate({ field: fd.field, op: fieldOps[0], value: fd.inputType === 'bool' ? true : '' })
  }

  // Geofence field needs a two-row layout: selectors + remove on top, map below
  if (safeField.inputType === 'geofence') {
    return (
      <div className="space-y-2">
        <div className="flex items-center gap-2 flex-wrap">
          <select
            value={safeCat.key}
            onChange={(e) => handleCategoryChange(e.target.value)}
            className={inputClass}
          >
            {CONDITION_CATEGORIES.map(cat => (
              <option key={cat.key} value={cat.key}>{cat.label}</option>
            ))}
          </select>
          {safeCat.fields.length > 1 && (
            <select
              value={cond.field}
              onChange={(e) => handleFieldChange(e.target.value)}
              className={inputClass}
            >
              {safeCat.fields.map(f => (
                <option key={f.field} value={f.field}>{f.label}</option>
              ))}
            </select>
          )}
          <button
            type="button"
            onClick={onRemove}
            className="text-neutral-400 hover:text-rose-500 transition-colors flex-shrink-0 cursor-pointer ml-auto"
            title="Bedingung entfernen"
          >
            <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          </button>
        </div>
        <ConditionValueEditor fieldDef={safeField} cond={cond} onChange={onUpdate} tags={tags} recorders={recorders} sensors={sensors} />
      </div>
    )
  }

  return (
    <div className="flex items-center gap-2 flex-wrap">
      {/* Category selector */}
      <select
        value={safeCat.key}
        onChange={(e) => handleCategoryChange(e.target.value)}
        className={inputClass}
      >
        {CONDITION_CATEGORIES.map(cat => (
          <option key={cat.key} value={cat.key}>{cat.label}</option>
        ))}
      </select>

      {/* Sub-field selector (only when category has > 1 field) */}
      {safeCat.fields.length > 1 && (
        <select
          value={cond.field}
          onChange={(e) => handleFieldChange(e.target.value)}
          className={inputClass}
        >
          {safeCat.fields.map(f => (
            <option key={f.field} value={f.field}>{f.label}</option>
          ))}
        </select>
      )}

      {/* Operator selector */}
      {ops.length > 1 && (
        <select
          value={cond.op}
          onChange={(e) => onUpdate({ op: e.target.value })}
          className={inputClass}
        >
          {ops.map(op => (
            <option key={op} value={op}>{OP_LABELS[op] ?? op}</option>
          ))}
        </select>
      )}

      {/* Value editor */}
      <ConditionValueEditor fieldDef={safeField} cond={cond} onChange={onUpdate} tags={tags} recorders={recorders} sensors={sensors} />

      {/* Remove button */}
      <button
        type="button"
        onClick={onRemove}
        className="text-neutral-400 hover:text-rose-500 transition-colors flex-shrink-0 cursor-pointer"
        title="Bedingung entfernen"
      >
        <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
          <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
        </svg>
      </button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Condition Group
// ---------------------------------------------------------------------------

function ConditionGroupBlock({
  group,
  groupIndex,
  totalGroups,
  onChange,
  onRemove,
  tags,
  recorders,
  sensors,
}: {
  group: RuleConditionGroup
  groupIndex: number
  totalGroups: number
  onChange: (g: RuleConditionGroup) => void
  onRemove: () => void
  tags: Array<{ id: string; name: string; color: string }>
  recorders?: Device[]
  sensors?: Device[]
}) {
  function addCondition() {
    onChange({ ...group, conditions: [...group.conditions, defaultCondition()] })
  }
  function removeCondition(i: number) {
    onChange({ ...group, conditions: group.conditions.filter((_, idx) => idx !== i) })
  }
  function updateCondition(i: number, patch: Partial<RuleCondition>) {
    onChange({
      ...group,
      conditions: group.conditions.map((c, idx) => idx === i ? { ...c, ...patch } : c),
    })
  }

  return (
    <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg p-3 space-y-2 bg-white dark:bg-neutral-900/30">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="text-xs text-neutral-500 dark:text-neutral-400 font-medium">Gruppe {groupIndex + 1}</span>
          {/* AND / OR toggle within this group */}
          <div className="flex rounded overflow-hidden border border-neutral-200 dark:border-neutral-700">
            {(['and', 'or'] as const).map(l => (
              <button
                key={l}
                type="button"
                onClick={() => onChange({ ...group, logic: l })}
                className={`px-2 py-0.5 text-xs font-medium transition-colors cursor-pointer ${
                  group.logic === l
                    ? 'bg-accent-DEFAULT text-white'
                    : 'text-neutral-500 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                {l === 'and' ? 'UND' : 'ODER'}
              </button>
            ))}
          </div>
          <span className="text-xs text-neutral-400">(innerhalb Gruppe)</span>
        </div>
        {totalGroups > 1 && (
          <button type="button" onClick={onRemove}
            className="text-xs text-neutral-400 hover:text-rose-500 transition-colors cursor-pointer"
            title="Gruppe entfernen"
          >
            Gruppe entfernen
          </button>
        )}
      </div>

      <div className="space-y-2">
        {group.conditions.map((cond, i) => (
          <div key={i}>
            {i > 0 && (
              <div className="flex items-center gap-2 my-1">
                <span className="text-xs font-semibold text-accent-DEFAULT uppercase tracking-wide px-1">
                  {group.logic === 'and' ? 'UND' : 'ODER'}
                </span>
              </div>
            )}
            <ConditionRow
              cond={cond}
              onUpdate={(patch) => updateCondition(i, patch)}
              onRemove={() => removeCondition(i)}
              tags={tags}
              recorders={recorders}
              sensors={sensors}
            />
          </div>
        ))}
      </div>

      <button
        type="button"
        onClick={addCondition}
        className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
      >
        + Bedingung hinzufügen
      </button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

function ActionParams({
  action,
  params,
  gearList,
  tags,
  onChange,
}: {
  action: string
  params: Record<string, any>
  gearList: any[]
  tags: Array<{ id: string; name: string; color: string }>
  onChange: (params: Record<string, any>) => void
}) {
  if (action === 'set_gear') {
    return (
      <select
        value={params.gear_id ?? ''}
        onChange={(e) => onChange({ gear_id: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Ausrüstung wählen...</option>
        {gearList.filter((g) => !g.is_retired).map((g) => (
          <option key={g.id} value={g.id}>{g.name}</option>
        ))}
      </select>
    )
  }
  if (action === 'add_tag') {
    return (
      <select
        value={params.tag_id ?? ''}
        onChange={(e) => onChange({ tag_id: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Tag wählen...</option>
        {tags.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}
      </select>
    )
  }
  if (action === 'remove_tag') {
    return (
      <select
        value={params.tag_id ?? ''}
        onChange={(e) => onChange({ tag_id: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Tag wählen...</option>
        {tags.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}
      </select>
    )
  }
  if (action === 'set_visibility') {
    return (
      <select
        value={params.visibility ?? ''}
        onChange={(e) => onChange({ visibility: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Sichtbarkeit wählen...</option>
        {VISIBILITY_OPTIONS.map(({ value, label }) => (
          <option key={value} value={value}>{label}</option>
        ))}
      </select>
    )
  }
  if (action === 'set_sport_type') {
    return (
      <div className="flex items-center gap-2 flex-1 min-w-0">
        <select
          value={params.sport_type ?? ''}
          onChange={(e) => onChange({ ...params, sport_type: e.target.value })}
          className={inputClass + ' flex-1 min-w-0'}
        >
          <option value="">Sportart wählen...</option>
          {SPORT_OPTIONS.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
        </select>
        <input
          type="text"
          value={params.sub_type ?? ''}
          onChange={(e) => onChange({ ...params, sub_type: e.target.value })}
          placeholder="Sub-Typ (optional)"
          className={inputClass + ' w-32'}
        />
      </div>
    )
  }
  if (action === 'set_name') {
    return (
      <div className="flex flex-col gap-1 flex-1 min-w-0">
        <input
          type="text"
          value={params.template ?? ''}
          onChange={(e) => onChange({ template: e.target.value })}
          placeholder="Neuer Titel"
          className={inputClass + ' w-full'}
        />
        <span className="text-xs text-neutral-400">Platzhalter: {'{city}'}, {'{distance_km}'}, {'{sport}'}</span>
      </div>
    )
  }
  if (action === 'set_intensity') {
    return (
      <select
        value={params.intensity ?? ''}
        onChange={(e) => onChange({ intensity: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Intensität wählen...</option>
        {INTENSITY_OPTIONS.map(({ value, label }) => (
          <option key={value} value={value}>{label}</option>
        ))}
      </select>
    )
  }
  if (action === 'set_indoor') {
    return (
      <select
        value={String(params.indoor ?? 'true')}
        onChange={(e) => onChange({ indoor: e.target.value === 'true' })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="true">Ja (Indoor)</option>
        <option value="false">Nein (Outdoor)</option>
      </select>
    )
  }
  if (action === 'set_exclude_from_stats') {
    return (
      <select
        value={String(params.value ?? 'true')}
        onChange={(e) => onChange({ value: e.target.value === 'true' })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="true">Ja (ausschließen)</option>
        <option value="false">Nein (einschließen)</option>
      </select>
    )
  }
  if (action === 'set_exclude_from_heatmap') {
    return (
      <select
        value={String(params.value ?? 'true')}
        onChange={(e) => onChange({ value: e.target.value === 'true' })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="true">Ja (ausschließen)</option>
        <option value="false">Nein (einschließen)</option>
      </select>
    )
  }
  if (action === 'set_hide_map') {
    return (
      <div className="flex flex-col gap-1 flex-1 min-w-0">
        <select
          value={String(params.value ?? 'true')}
          onChange={(e) => onChange({ value: e.target.value === 'true' })}
          className={inputClass + ' w-full'}
        >
          <option value="true">Ja (Karte verstecken)</option>
          <option value="false">Nein (Karte anzeigen)</option>
        </select>
        <p className="text-xs text-amber-600 mt-1">
          Achtung: Diese Aktion versteckt die Karte dauerhaft. Prüfe ob die Bedingungen korrekt sind.
        </p>
      </div>
    )
  }
  return null
}

// ---------------------------------------------------------------------------
// set_field action normalisation helpers
// ---------------------------------------------------------------------------

// Frontend-only action types that map to the generic set_field backend action.
// This lets us show dedicated dropdowns in the UI while storing canonical JSON.
const SET_FIELD_ACTION_TO_FIELD: Record<string, string> = {
  set_exclude_from_stats:   'exclude_from_stats',
  set_exclude_from_heatmap: 'exclude_from_heatmap',
  set_hide_map:             'hide_map',
}

const SET_FIELD_FIELD_TO_ACTION: Record<string, string> = {
  exclude_from_stats:   'set_exclude_from_stats',
  exclude_from_heatmap: 'set_exclude_from_heatmap',
  hide_map:             'set_hide_map',
}

/** When loading saved rules, translate set_field actions back to frontend action types. */
function denormaliseActions(raw: RuleAction[]): RuleAction[] {
  return raw.map((a) => {
    if (a.action === 'set_field' && a.params?.field && SET_FIELD_FIELD_TO_ACTION[a.params.field]) {
      return { action: SET_FIELD_FIELD_TO_ACTION[a.params.field], params: { value: a.params.value ?? false } }
    }
    return a
  })
}

// ---------------------------------------------------------------------------
// Main RuleEditor component
// ---------------------------------------------------------------------------

interface RuleEditorProps {
  rule?: AutomationRule | null
  /** Pre-fill values for new rules (e.g. from URL params) */
  initialData?: Partial<AutomationRule> | null
  onSave: (data: Partial<AutomationRule>) => void
  onCancel: () => void
  isSaving?: boolean
}

export default function RuleEditor({ rule, initialData, onSave, onCancel, isSaving }: RuleEditorProps) {
  const seed = rule ?? initialData
  const [name, setName] = useState(seed?.name ?? '')
  const [priority, setPriority] = useState(seed?.priority ?? 0)
  const [triggerEvent, setTriggerEvent] = useState<'on_import' | 'on_enrichment_complete' | 'on_manual_apply'>(seed?.trigger_event ?? 'on_import')
  const [stopOnMatch, setStopOnMatch] = useState(seed?.stop_on_match ?? false)
  // When a rule uses branches, the 'if' branch becomes the main conditions/actions,
  // and only 'else_if'/'else' branches are shown as SONST WENN/SONST blocks.
  const parsedBranches: RuleBranch[] = (() => {
    const raw = seed?.branches
    if (!raw) return []
    if (typeof raw === 'string') {
      try { return JSON.parse(raw) } catch { return [] }
    }
    return raw as RuleBranch[]
  })()
  const ifBranch = parsedBranches.find(b => b.type === 'if')
  const elseIfBranches = parsedBranches.filter(b => b.type !== 'if')

  const [groups, setGroups] = useState<RuleConditionGroup[]>(() =>
    ifBranch ? ifBranch.condition_groups : normaliseGroups(rule)
  )
  const [actions, setActions] = useState<RuleAction[]>(() =>
    ifBranch ? denormaliseActions(ifBranch.actions) : denormaliseActions(seed?.actions ?? [])
  )
  const [branches, setBranches] = useState<RuleBranch[]>(elseIfBranches)

  const { data: gearList } = useQuery({
    queryKey: ['gear-list'],
    queryFn: () => fetchGear(false),
  })
  const { data: tagList } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })
  const { data: recorderList } = useQuery({
    queryKey: ['devices-recorders'],
    queryFn: () => fetchDevices('recorder'),
  })
  const { data: sensorList } = useQuery({
    queryKey: ['devices-sensors'],
    queryFn: () => fetchDevices('sensor'),
  })

  const tags = (tagList ?? []).map(t => ({ id: t.id, name: t.name, color: t.color }))
  const recorders = recorderList ?? []
  const sensors = sensorList ?? []

  // Groups mutations
  function addGroup() {
    setGroups([...groups, defaultGroup()])
  }
  function updateGroup(i: number, g: RuleConditionGroup) {
    setGroups(groups.map((old, idx) => idx === i ? g : old))
  }
  function removeGroup(i: number) {
    setGroups(groups.filter((_, idx) => idx !== i))
  }

  // Actions mutations
  function addAction() {
    setActions([...actions, { action: 'add_tag', params: {} }])
  }
  function removeAction(idx: number) {
    setActions(actions.filter((_, i) => i !== idx))
  }
  function updateAction(idx: number, action: string, params: Record<string, any>) {
    setActions(actions.map((a, i) => (i === idx ? { action, params } : a)))
  }

  // Branch mutations
  function addBranch(type: 'else_if' | 'else') {
    setBranches(prev => [...prev, { type, condition_groups: [], actions: [] }])
  }
  function removeBranch(idx: number) {
    setBranches(prev => prev.filter((_, i) => i !== idx))
  }
  function addBranchAction(branchIdx: number) {
    setBranches(prev => prev.map((b, i) =>
      i === branchIdx ? { ...b, actions: [...b.actions, { action: 'add_tag', params: {} }] } : b
    ))
  }
  function removeBranchAction(branchIdx: number, actionIdx: number) {
    setBranches(prev => prev.map((b, i) =>
      i === branchIdx ? { ...b, actions: b.actions.filter((_, ai) => ai !== actionIdx) } : b
    ))
  }
  function updateBranchAction(branchIdx: number, actionIdx: number, action: string, params: Record<string, any>) {
    setBranches(prev => prev.map((b, i) =>
      i === branchIdx ? {
        ...b,
        actions: b.actions.map((a, ai) => ai === actionIdx ? { action, params } : a),
      } : b
    ))
  }
  function addBranchGroup(branchIdx: number) {
    setBranches(prev => prev.map((b, i) =>
      i === branchIdx ? { ...b, condition_groups: [...b.condition_groups, defaultGroup()] } : b
    ))
  }
  function updateBranchGroup(branchIdx: number, groupIdx: number, g: RuleConditionGroup) {
    setBranches(prev => prev.map((b, i) =>
      i === branchIdx ? {
        ...b,
        condition_groups: b.condition_groups.map((old, gi) => gi === groupIdx ? g : old),
      } : b
    ))
  }
  function removeBranchGroup(branchIdx: number, groupIdx: number) {
    setBranches(prev => prev.map((b, i) =>
      i === branchIdx ? {
        ...b,
        condition_groups: b.condition_groups.filter((_, gi) => gi !== groupIdx),
      } : b
    ))
  }

  const totalConditions = groups.reduce((s, g) => s + g.conditions.length, 0)

  /** Before saving, translate frontend action types back to canonical set_field JSON. */
  function normaliseActions(raw: RuleAction[]): RuleAction[] {
    return raw.map((a) => {
      const fieldName = SET_FIELD_ACTION_TO_FIELD[a.action]
      if (fieldName) {
        return { action: 'set_field', params: { field: fieldName, value: a.params.value ?? false } }
      }
      return a
    })
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim() || totalConditions === 0 || actions.length === 0) return
    const flat = flattenGroups(groups)
    onSave({
      name: name.trim(),
      priority,
      trigger_event: triggerEvent,
      stop_on_match: stopOnMatch,
      condition_groups: groups,
      // Also send flat for backward compatibility:
      conditions: flat.conditions,
      conditions_logic: flat.conditions_logic,
      actions: normaliseActions(actions),
      // When branches exist, reconstruct: main groups/actions become the 'if' branch
      branches: branches.length > 0 ? [
        { type: 'if' as const, condition_groups: groups, actions: normaliseActions(actions) },
        ...branches.map(b => ({
          ...b,
          actions: normaliseActions(b.actions),
        })),
      ] : undefined,
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {/* Name & Priority */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="sm:col-span-2">
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Name *</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            className={inputClass + ' w-full'}
            placeholder="z.B. MTB-Rides automatisch taggen"
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Priorität</label>
          <input
            type="number"
            value={priority}
            onChange={(e) => setPriority(Number(e.target.value))}
            min={0}
            className={inputClass + ' w-full'}
          />
        </div>
      </div>

      {/* Trigger Event & Stop-on-Match */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Auslöser</label>
          <select
            value={triggerEvent}
            onChange={e => setTriggerEvent(e.target.value as typeof triggerEvent)}
            className={inputClass + ' w-full'}
          >
            <option value="on_import">Beim Import</option>
            <option value="on_enrichment_complete">Nach Enrichment</option>
            <option value="on_manual_apply">Nur manuell</option>
          </select>
        </div>
        <div className="flex items-end pb-1">
          <label className="flex items-center gap-2 text-sm text-neutral-700 dark:text-neutral-300 cursor-pointer">
            <input
              type="checkbox"
              checked={stopOnMatch}
              onChange={e => setStopOnMatch(e.target.checked)}
              className="rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT"
            />
            Weitere Regeln stoppen
          </label>
        </div>
      </div>

      {/* Condition groups */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <div>
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Bedingungen *</h3>
            {groups.length > 1 && (
              <p className="text-xs text-neutral-400 mt-0.5">Gruppen werden mit UND verknüpft</p>
            )}
          </div>
          <button
            type="button"
            onClick={addGroup}
            className="text-sm text-accent-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300 cursor-pointer"
          >
            + Gruppe hinzufügen
          </button>
        </div>
        {totalConditions === 0 && (
          <p className="text-sm text-neutral-400 py-2">Mindestens eine Bedingung erforderlich.</p>
        )}
        <div className="space-y-3">
          {groups.map((g, i) => (
            <div key={i}>
              {i > 0 && (
                <div className="flex items-center gap-2 my-2">
                  <div className="flex-1 border-t border-dashed border-neutral-300 dark:border-neutral-700" />
                  <span className="text-xs font-semibold text-neutral-500 uppercase tracking-wide px-2">UND</span>
                  <div className="flex-1 border-t border-dashed border-neutral-300 dark:border-neutral-700" />
                </div>
              )}
              <ConditionGroupBlock
                group={g}
                groupIndex={i}
                totalGroups={groups.length}
                onChange={(updated) => updateGroup(i, updated)}
                onRemove={() => removeGroup(i)}
                tags={tags}
                recorders={recorders}
                sensors={sensors}
              />
            </div>
          ))}
        </div>


      </div>

      {/* Actions */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Aktionen *</h3>
          <button
            type="button"
            onClick={addAction}
            className="text-sm text-accent-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300 cursor-pointer"
          >
            + Aktion
          </button>
        </div>
        {actions.length === 0 && (
          <p className="text-sm text-neutral-400 py-2">Mindestens eine Aktion erforderlich.</p>
        )}
        <div className="space-y-2">
          {actions.map((act, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <select
                value={act.action}
                onChange={(e) => updateAction(idx, e.target.value, {})}
                className={inputClass}
              >
                {ACTION_TYPES.map(a => (
                  <option key={a.value} value={a.value}>{a.label}</option>
                ))}
              </select>
              <ActionParams
                action={act.action}
                params={act.params}
                gearList={gearList ?? []}
                tags={tags}
                onChange={(params) => updateAction(idx, act.action, params)}
              />
              <button
                type="button"
                onClick={() => removeAction(idx)}
                className="text-neutral-400 hover:text-rose-500 transition-colors cursor-pointer"
                title="Aktion entfernen"
              >
                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                </svg>
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* Branches (ELSE-IF / ELSE) */}
      {branches.map((branch, branchIdx) => (
        <div key={branchIdx} className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-3">
          {/* Branch header */}
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
              {branch.type === 'else' ? 'Sonst' : 'Sonst wenn'}
            </span>
            <button
              type="button"
              onClick={() => removeBranch(branchIdx)}
              className="ml-auto text-xs text-rose-500 hover:text-rose-700 dark:hover:text-rose-400 cursor-pointer"
            >
              Zweig entfernen
            </button>
          </div>

          {/* Conditions for else_if branches */}
          {branch.type === 'else_if' && (
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Bedingungen</span>
                <button
                  type="button"
                  onClick={() => addBranchGroup(branchIdx)}
                  className="text-xs text-accent-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300 cursor-pointer"
                >
                  + Gruppe
                </button>
              </div>
              {branch.condition_groups.length === 0 && (
                <p className="text-xs text-neutral-400 italic">Keine Bedingungen — Zweig greift immer.</p>
              )}
              <div className="space-y-2">
                {branch.condition_groups.map((g, gi) => (
                  <div key={gi}>
                    {gi > 0 && (
                      <div className="flex items-center gap-2 my-1">
                        <div className="flex-1 border-t border-dashed border-neutral-300 dark:border-neutral-700" />
                        <span className="text-xs font-semibold text-neutral-500 uppercase tracking-wide px-2">UND</span>
                        <div className="flex-1 border-t border-dashed border-neutral-300 dark:border-neutral-700" />
                      </div>
                    )}
                    <ConditionGroupBlock
                      group={g}
                      groupIndex={gi}
                      totalGroups={branch.condition_groups.length}
                      onChange={(updated) => updateBranchGroup(branchIdx, gi, updated)}
                      onRemove={() => removeBranchGroup(branchIdx, gi)}
                      tags={tags}
                      recorders={recorders}
                      sensors={sensors}
                    />
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Actions for this branch */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Dann</span>
              <button
                type="button"
                onClick={() => addBranchAction(branchIdx)}
                className="text-xs text-accent-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300 cursor-pointer"
              >
                + Aktion
              </button>
            </div>
            {branch.actions.length === 0 && (
              <p className="text-xs text-neutral-400 italic">Keine Aktionen.</p>
            )}
            <div className="space-y-2">
              {branch.actions.map((act, actionIdx) => (
                <div key={actionIdx} className="flex items-center gap-2">
                  <select
                    value={act.action}
                    onChange={(e) => updateBranchAction(branchIdx, actionIdx, e.target.value, {})}
                    className={inputClass}
                  >
                    {ACTION_TYPES.map(a => (
                      <option key={a.value} value={a.value}>{a.label}</option>
                    ))}
                  </select>
                  <ActionParams
                    action={act.action}
                    params={act.params}
                    gearList={gearList ?? []}
                    tags={tags}
                    onChange={(params) => updateBranchAction(branchIdx, actionIdx, act.action, params)}
                  />
                  <button
                    type="button"
                    onClick={() => removeBranchAction(branchIdx, actionIdx)}
                    className="text-neutral-400 hover:text-rose-500 transition-colors cursor-pointer"
                    title="Aktion entfernen"
                  >
                    <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                      <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                    </svg>
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>
      ))}

      {/* Add-branch buttons */}
      <div className="flex gap-2">
        <button
          type="button"
          onClick={() => addBranch('else_if')}
          className="text-sm border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-700 dark:text-neutral-300 cursor-pointer transition-colors"
        >
          + Sonst wenn
        </button>
        {!branches.some(b => b.type === 'else') && (
          <button
            type="button"
            onClick={() => addBranch('else')}
            className="text-sm border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-700 dark:text-neutral-300 cursor-pointer transition-colors"
          >
            + Sonst
          </button>
        )}
      </div>

      {/* Submit */}
      <div className="flex flex-col items-end gap-1 pt-2">
        {(!name.trim() || totalConditions === 0 || actions.length === 0) && (
          <p className="text-xs text-amber-600 dark:text-amber-400">
            {!name.trim() ? 'Name ist erforderlich' : totalConditions === 0 ? 'Mindestens eine Bedingung erforderlich' : 'Mindestens eine Aktion erforderlich'}
          </p>
        )}
        <div className="flex gap-3">
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-2 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
          >
            Abbrechen
          </button>
          <button
            type="submit"
            disabled={isSaving || !name.trim() || totalConditions === 0 || actions.length === 0}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
          >
            {isSaving ? 'Speichert...' : 'Speichern'}
          </button>
        </div>
      </div>
    </form>
  )
}
