import { useRef, useEffect, useState } from 'react'
import { HelpCircle } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { fetchGear } from '@/api/gear'
import { fetchTags } from '@/api/tags'
import { fetchDevices } from '@/api/hardware'
import type { Device } from '@/api/hardware'
import { fetchSegments, type Segment } from '@/api/segments'
import type { AutomationRule, Gear, RuleCondition, RuleConditionGroup, RuleAction, RuleBranch } from '@/api/types'
import { SPORT_DEFS, getSportDef, normalizeSportTuple, sportLabel } from '@/lib/sportTaxonomy'
import { ruleModifierControlsForTuple, ruleTraitConditionFields, ruleTupleOptions } from '@/lib/ruleSportProjection'
import { GEAR_RULE_CONDITION_CATEGORY } from '@/lib/ruleGearProjection'
import { isSupportedLanguage, type SupportedLanguage } from '@/i18n/languages'
import { localizeString, type LocalizedString } from '@/i18n/localizedString'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import {
  distanceFromDisplayValue,
  distanceInputUnit,
  distanceToDisplayValue,
  elevationFromDisplayValue,
  elevationInputUnit,
  elevationToDisplayValue,
  formatDistanceCompact as formatUnitDistanceCompact,
  formatElevation as formatUnitElevation,
  type UnitFormatOptions,
  type UnitSystem,
} from '@/lib/units'
import {
  modifierActionName,
  modifierActionParam,
  modifierLabel,
  modifierTagSync,
  modifierTagSyncOnlySportType,
  modifierTSField,
  type SportModifierKey,
} from '@/lib/sportModifiers'
import { RulesApplicableModifierKeys } from '@/lib/generatedRulesApplicableModifiers'
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

const METERS_PER_MILE = 1609.344
const SEGMENT_PREVIEW_TYPES = new Set(['standard', 'trail', 'lap', 'lift', 'wintersport', 'wassersport'])
const SEGMENT_PREVIEW_SPORT_GROUPS = new Set(['bike', 'foot', 'swim', 'ski', 'paddle', 'generic'])
const SEGMENT_PREVIEW_VISIBILITY = new Set(['public', 'private', 'shared'])

function formatSegmentDistance(m: number | undefined, unitOptions: UnitFormatOptions): string | null {
  if (m == null) return null
  return formatUnitDistanceCompact(m, unitOptions)
}

function segmentPreview(segment: Segment, t: TFunction, unitOptions: UnitFormatOptions): string {
  return [
    SEGMENT_PREVIEW_TYPES.has(segment.segment_type)
      ? t(`ruleEditor.segmentPreview.types.${segment.segment_type}`)
      : t('ruleEditor.segmentPreview.types.unknown'),
    SEGMENT_PREVIEW_SPORT_GROUPS.has(segment.sport_group)
      ? t(`ruleEditor.segmentPreview.sportGroups.${segment.sport_group}`)
      : t('ruleEditor.segmentPreview.sportGroups.unknown'),
    formatSegmentDistance(segment.distance_m, unitOptions),
    segment.elevation_gain_m != null ? formatUnitElevation(segment.elevation_gain_m, unitOptions) : null,
    SEGMENT_PREVIEW_VISIBILITY.has(segment.visibility)
      ? t(`ruleEditor.segmentPreview.visibility.${segment.visibility}`)
      : t('ruleEditor.segmentPreview.visibility.unknown'),
  ].filter(Boolean).join(' · ')
}

const RULE_TUPLE_OPTIONS = ruleTupleOptions()
const RULE_TRAIT_CONDITION_FIELDS = ruleTraitConditionFields()
const RULE_TRAIT_CONDITION_FIELD_KEYS = new Set(RULE_TRAIT_CONDITION_FIELDS.map((field) => field.field))

const RULE_TUPLE_SPORT_OPTIONS = Array.from(
  new Map(RULE_TUPLE_OPTIONS.map((opt) => {
    const parent = RULE_TUPLE_OPTIONS.find((candidate) => candidate.sportType === opt.sportType && !candidate.subType)
    return [opt.sportType, { value: opt.sportType, label: parent?.label ?? getSportDef(opt.sportType)?.label ?? opt.sportType }]
  })).values(),
)

function ruleTupleSubTypeOptions(sportType: string) {
  return RULE_TUPLE_OPTIONS.filter((opt) => opt.sportType === sportType && opt.subType)
}

function ruleTupleHasParentOption(sportType: string) {
  return RULE_TUPLE_OPTIONS.some((opt) => opt.sportType === sportType && !opt.subType)
}



const SOURCE_OPTIONS = [
  { value: 'garmin', label: 'Garmin' },
  { value: 'wahoo', label: 'Wahoo' },
  { value: 'strava', label: 'Strava' },
  { value: 'komoot', label: 'Komoot' },
  { value: 'endomondo', label: 'Endomondo' },
  { value: 'osmtracker', label: 'OSMTracker' },
  { value: 'manual', label: 'manual' },
]

const SEASON_OPTIONS = [
  { value: 'spring', label: 'spring' },
  { value: 'summer', label: 'summer' },
  { value: 'autumn', label: 'autumn' },
  { value: 'winter', label: 'winter' },
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
  { value: 'private',    label: 'private' },
  { value: 'bekannte',   label: 'bekannte' },
  { value: 'instanz',    label: 'instanz' },
  { value: 'federation', label: 'Federation' },
  { value: 'public',     label: 'public' },
]

// Field definitions: each entry tells the editor how to render the value input
interface ConditionField {
  field: string
  label: string
  labels?: LocalizedString
  inputType: 'number' | 'text' | 'bool' | 'enum' | 'tag' | 'segment' | 'geofence' | 'sportTupleList'
  unit?: RuleUnit
  options?: { value: string; label: string }[]
  /** Override available operators for this specific field */
  operators?: string[]
}

type RuleUnit = 'seconds' | 'distance' | 'length' | 'speed' | 'pace' | 'temperature' | 'percent' | 'degrees' | 'bpm' | 'rpm' | 'watts'

interface ConditionCategory {
  key: string
  label: string
  fields: ConditionField[]
}

const MONTH_OPTIONS = [
  { value: '1', label: '1' }, { value: '2', label: '2' },
  { value: '3', label: '3' }, { value: '4', label: '4' },
  { value: '5', label: '5' }, { value: '6', label: '6' },
  { value: '7', label: '7' }, { value: '8', label: '8' },
  { value: '9', label: '9' }, { value: '10', label: '10' },
  { value: '11', label: '11' }, { value: '12', label: '12' },
]
const INTENSITY_OPTIONS = [
  { value: 'locker', label: 'locker' },
  { value: 'moderat', label: 'moderat' },
  { value: 'intensiv', label: 'intensiv' },
]
const TIME_OF_DAY_OPTIONS = [
  { value: 'nacht', label: 'nacht' },
  { value: 'frueh', label: 'frueh' },
  { value: 'morgen', label: 'morgen' },
  { value: 'mittag', label: 'mittag' },
  { value: 'nachmittag', label: 'nachmittag' },
  { value: 'abend', label: 'abend' },
]

const CONDITION_CATEGORIES: ConditionCategory[] = [
  { key: 'title', label: 'title', fields: [{ field: 'title', label: 'title', inputType: 'text' }] },
  { key: 'description', label: 'description', fields: [{ field: 'description', label: 'description', inputType: 'text' }] },
  { key: 'sport', label: 'sport', fields: [
    { field: 'sport_type', label: 'sport_type', inputType: 'enum', options: SPORT_OPTIONS },
    { field: 'sport_tuple', label: 'sport_tuple', inputType: 'sportTupleList', operators: ['in'] },
  ]},
  { key: 'taxonomy', label: 'taxonomy', fields: RULE_TRAIT_CONDITION_FIELDS.map((field) => ({
    field: field.field,
    label: field.label,
    labels: field.labels,
    inputType: 'enum',
    options: field.options.map((option) => ({ ...option, label: option.value })),
    operators: ['in'],
  })) },
  { key: 'modifiers', label: 'Modifier', fields: RulesApplicableModifierKeys.map((key) => ({
    field: key,
    label: key,
    inputType: 'bool',
  })) },
  { key: 'activity', label: 'activity', fields: [
    { field: 'source', label: 'source', inputType: 'enum', options: SOURCE_OPTIONS },
    { field: 'intensity', label: 'intensity', inputType: 'enum', options: INTENSITY_OPTIONS },
    { field: 'has_gps', label: 'has_gps', inputType: 'bool' },
    { field: 'detected_lap_count', label: 'detected_lap_count', inputType: 'number' },
  ]},
  { key: 'time', label: 'time', fields: [
    { field: 'elapsed_time_s', label: 'elapsed_time_s', inputType: 'number', unit: 'seconds' },
    { field: 'moving_time_s', label: 'moving_time_s', inputType: 'number', unit: 'seconds' },
    { field: 'weekday', label: 'weekday', inputType: 'enum', options: WEEKDAY_OPTIONS },
    { field: 'month', label: 'month', inputType: 'enum', options: MONTH_OPTIONS },
    { field: 'season', label: 'season', inputType: 'enum', options: SEASON_OPTIONS },
    { field: 'time_of_day', label: 'time_of_day', inputType: 'enum', options: TIME_OF_DAY_OPTIONS },
    { field: 'sun_elevation', label: 'sun_elevation', inputType: 'number', unit: 'degrees' },
  ]},
  { key: 'distance', label: 'distance', fields: [
    { field: 'distance_m', label: 'distance_m', inputType: 'number', unit: 'distance' },
    { field: 'start_end_distance_m', label: 'start_end_distance_m', inputType: 'number', unit: 'distance' },
  ]},
  { key: 'speed', label: 'speed', fields: [
    { field: 'avg_speed_ms', label: 'avg_speed_ms', inputType: 'number', unit: 'speed' },
    { field: 'max_speed_ms', label: 'max_speed_ms', inputType: 'number', unit: 'speed' },
    { field: 'min_speed_ms', label: 'min_speed_ms', inputType: 'number', unit: 'speed' },
  ]},
  { key: 'pace', label: 'pace', fields: [
    { field: 'avg_pace_min_km', label: 'avg_pace_min_km', inputType: 'number', unit: 'pace' },
    { field: 'min_pace_min_km', label: 'min_pace_min_km', inputType: 'number', unit: 'pace' },
    { field: 'max_pace_min_km', label: 'max_pace_min_km', inputType: 'number', unit: 'pace' },
  ]},
  { key: 'elevation', label: 'elevation', fields: [
    { field: 'elevation_gain_m', label: 'elevation_gain_m', inputType: 'number', unit: 'length' },
    { field: 'elevation_loss_m', label: 'elevation_loss_m', inputType: 'number', unit: 'length' },
    { field: 'max_elevation_m', label: 'max_elevation_m', inputType: 'number', unit: 'length' },
    { field: 'min_elevation_m', label: 'min_elevation_m', inputType: 'number', unit: 'length' },
    { field: 'climb_count', label: 'climb_count', inputType: 'number' },
    { field: 'climb_category_max', label: 'climb_category_max', inputType: 'number' },
  ]},
  { key: 'heartrate', label: 'heartrate', fields: [
    { field: 'avg_heart_rate', label: 'avg_heart_rate', inputType: 'number', unit: 'bpm' },
    { field: 'max_heart_rate', label: 'max_heart_rate', inputType: 'number', unit: 'bpm' },
    { field: 'min_heart_rate', label: 'min_heart_rate', inputType: 'number', unit: 'bpm' },
  ]},
  { key: 'cadence', label: 'cadence', fields: [
    { field: 'avg_cadence', label: 'avg_cadence', inputType: 'number', unit: 'rpm' },
    { field: 'max_cadence', label: 'max_cadence', inputType: 'number', unit: 'rpm' },
  ]},
  { key: 'power', label: 'power', fields: [
    { field: 'avg_power_w', label: 'avg_power_w', inputType: 'number', unit: 'watts' },
    { field: 'max_power_w', label: 'max_power_w', inputType: 'number', unit: 'watts' },
    { field: 'min_power_w', label: 'min_power_w', inputType: 'number', unit: 'watts' },
    { field: 'normalized_power_w', label: 'normalized_power_w', inputType: 'number', unit: 'watts' },
    { field: 'tss', label: 'TSS', inputType: 'number' },
    { field: 'trimp', label: 'TRIMP', inputType: 'number' },
  ]},
  { key: 'sensor_temp', label: 'sensor_temp', fields: [
    { field: 'avg_temperature_c', label: 'avg_temperature_c', inputType: 'number', unit: 'temperature' },
    { field: 'min_temperature_c', label: 'min_temperature_c', inputType: 'number', unit: 'temperature' },
    { field: 'max_temperature_c', label: 'max_temperature_c', inputType: 'number', unit: 'temperature' },
  ]},
  { key: 'weather', label: 'weather', fields: [
    { field: 'weather_temp_c', label: 'weather_temp_c', inputType: 'number', unit: 'temperature' },
    { field: 'weather_temp_min_c', label: 'weather_temp_min_c', inputType: 'number', unit: 'temperature' },
    { field: 'weather_temp_max_c', label: 'weather_temp_max_c', inputType: 'number', unit: 'temperature' },
    { field: 'weather_condition', label: 'weather_condition', inputType: 'text' },
    { field: 'weather_humidity', label: 'weather_humidity', inputType: 'number', unit: 'percent' },
    { field: 'weather_wind_speed_ms', label: 'weather_wind_speed_ms', inputType: 'number', unit: 'speed' },
    { field: 'weather_wind_gusts_ms', label: 'weather_wind_gusts_ms', inputType: 'number', unit: 'speed' },
  ]},
  { key: 'location', label: 'location', fields: [
    { field: 'start_city', label: 'start_city', inputType: 'text' },
    { field: 'start_state', label: 'start_state', inputType: 'text' },
    { field: 'start_country', label: 'start_country', inputType: 'text' },
    { field: 'start_country_code', label: 'start_country_code', inputType: 'text' },
    { field: 'end_city', label: 'end_city', inputType: 'text' },
    { field: 'geofence', label: 'Geofence', inputType: 'geofence' },
  ]},
  GEAR_RULE_CONDITION_CATEGORY,
  { key: 'tags', label: 'Tags', fields: [
    { field: 'has_tag', label: 'has_tag', inputType: 'tag' },
  ]},
  { key: 'media', label: 'media', fields: [
    { field: 'photo_count', label: 'photo_count', inputType: 'number' },
  ]},
  { key: 'visibility', label: 'visibility', fields: [
    { field: 'visibility', label: 'visibility', inputType: 'enum', options: VISIBILITY_OPTIONS },
    { field: 'hide_map', label: 'hide_map', inputType: 'bool' },
  ]},
  { key: 'lift', label: 'Lift', fields: [
    { field: 'has_lift', label: 'has_lift', inputType: 'bool' },
    { field: 'lift_count', label: 'lift_count', inputType: 'number' },
    { field: 'lift_ascent_m', label: 'lift_ascent_m', inputType: 'number', unit: 'length' },
    { field: 'lift_descent_m', label: 'lift_descent_m', inputType: 'number', unit: 'length' },
    { field: 'lift_distance_m', label: 'lift_distance_m', inputType: 'number', unit: 'distance' },
    { field: 'lift_duration_s', label: 'lift_duration_s', inputType: 'number', unit: 'seconds' },
    { field: 'net_ascent_m', label: 'net_ascent_m', inputType: 'number', unit: 'length' },
    { field: 'net_descent_m', label: 'net_descent_m', inputType: 'number', unit: 'length' },
    { field: 'net_distance_m', label: 'net_distance_m', inputType: 'number', unit: 'distance' },
  ]},
  { key: 'segments', label: 'segments', fields: [
    { field: 'has_segment_effort', label: 'has_segment_effort', inputType: 'segment' },
    { field: 'segment_effort_rank', label: 'segment_effort_rank', inputType: 'segment' },
    { field: 'segment_effort_time_s', label: 'segment_effort_time_s', inputType: 'segment' },
  ]},
  { key: 'device', label: 'device', fields: [
    {
      field: 'recording_device_id',
      label: 'recording_device_id',
      inputType: 'enum',
      options: [], // dynamically filled via devices query in RuleEditor
    },
    { field: 'recording_device_product_name', label: 'recording_device_product_name', inputType: 'text' },
    { field: 'recording_device_manufacturer', label: 'recording_device_manufacturer', inputType: 'text' },
    {
      field: 'has_sensor_type',
      label: 'has_sensor_type',
      inputType: 'enum',
      options: [
        { value: 'heart_rate', label: 'heart_rate' },
        { value: 'bike_power', label: 'bike_power' },
        { value: 'bike_cadence', label: 'bike_cadence' },
        { value: 'bike_speed', label: 'bike_speed' },
        { value: 'bike_speed_cadence', label: 'bike_speed_cadence' },
        { value: 'stride_speed_distance', label: 'stride_speed_distance' },
        { value: 'bike_radar', label: 'bike_radar' },
        { value: 'shifting', label: 'shifting' },
        { value: 'env_sensor', label: 'env_sensor' },
        { value: 'muscle_oxygen', label: 'muscle_oxygen' },
      ],
      operators: ['contains', 'not_contains'],
    },
    {
      field: 'sensor_device_ids',
      label: 'sensor_device_ids',
      inputType: 'enum',
      options: [], // dynamically filled via devices query in RuleEditor
      operators: ['contains'],
    },
  ]},
  { key: 'enrichment', label: 'Enrichment', fields: [
    { field: 'has_summit', label: 'has_summit', inputType: 'bool' },
    { field: 'summit_count', label: 'summit_count', inputType: 'number' },
    { field: 'has_pass', label: 'has_pass', inputType: 'bool' },
    { field: 'border_crossing_count', label: 'border_crossing_count', inputType: 'number' },
    { field: 'has_gastronomy_stop', label: 'has_gastronomy_stop', inputType: 'bool' },
    { field: 'stop_count', label: 'stop_count', inputType: 'number' },
    { field: 'commute_detected', label: 'commute_detected', inputType: 'bool' },
    { field: 'has_structured_intervals', label: 'has_structured_intervals', inputType: 'bool' },
    { field: 'interval_count', label: 'interval_count', inputType: 'number' },
    { field: 'has_hc_climb', label: 'has_hc_climb', inputType: 'bool' },
  ]},
  { key: 'surface', label: 'surface', fields: [
    { field: 'surface_paved_pct', label: 'surface_paved_pct', inputType: 'number', unit: 'percent' },
    { field: 'surface_gravel_pct', label: 'surface_gravel_pct', inputType: 'number', unit: 'percent' },
    { field: 'surface_trail_pct', label: 'surface_trail_pct', inputType: 'number', unit: 'percent' },
    { field: 'surface_unknown_pct', label: 'surface_unknown_pct', inputType: 'number', unit: 'percent' },
  ]},
  { key: 'swimming', label: 'swimming', fields: [
    { field: 'swim_stroke', label: 'swim_stroke', inputType: 'enum', options: [
      { value: 'freestyle', label: 'freestyle' },
      { value: 'backstroke', label: 'backstroke' },
      { value: 'breaststroke', label: 'breaststroke' },
      { value: 'butterfly', label: 'butterfly' },
      { value: 'mixed', label: 'mixed' },
    ]},
    { field: 'pool_length_m', label: 'pool_length_m', inputType: 'number', unit: 'length' },
    { field: 'num_active_lengths', label: 'num_active_lengths', inputType: 'number' },
  ]},
  { key: 'diving', label: 'diving', fields: [
    { field: 'max_depth_m', label: 'max_depth_m', inputType: 'number', unit: 'length' },
    { field: 'avg_depth_m', label: 'avg_depth_m', inputType: 'number', unit: 'length' },
    { field: 'end_cns_pct', label: 'End-CNS', inputType: 'number', unit: 'percent' },
  ]},
  { key: 'mtb', label: 'MTB', fields: [
    { field: 'jump_count', label: 'jump_count', inputType: 'number' },
    { field: 'total_grit', label: 'Grit (Garmin)', inputType: 'number' },
    { field: 'avg_flow', label: 'Flow (Garmin)', inputType: 'number' },
  ]},
  { key: 'ebike', label: 'E-Bike', fields: [
    { field: 'avg_lev_motor_power_w', label: 'avg_lev_motor_power_w', inputType: 'number', unit: 'watts' },
    { field: 'lev_battery_consumption_pct', label: 'lev_battery_consumption_pct', inputType: 'number', unit: 'percent' },
  ]},
  { key: 'multisport', label: 'Multisport', fields: [
    { field: 'is_multisport_child', label: 'is_multisport_child', inputType: 'bool' },
    { field: 'is_multisport_parent', label: 'is_multisport_parent', inputType: 'bool' },
  ]},
  { key: 'training', label: 'training', fields: [
    { field: 'aerobic_training_effect', label: 'aerobic_training_effect', inputType: 'number' },
    { field: 'anaerobic_training_effect', label: 'anaerobic_training_effect', inputType: 'number' },
    { field: 'aerobic_decoupling', label: 'Aerobic Decoupling', inputType: 'number', unit: 'percent' },
    { field: 'variability_index', label: 'Variability Index', inputType: 'number' },
    { field: 'efficiency_factor', label: 'Efficiency Factor', inputType: 'number' },
    { field: 'intensity_factor', label: 'Intensity Factor', inputType: 'number' },
    { field: 'workout_rpe', label: 'workout_rpe', inputType: 'number' },
    { field: 'workout_feel', label: 'workout_feel', inputType: 'number' },
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
    case 'sportTupleList': return ['in']
  }
}

function defaultValueForField(field: ConditionField): unknown {
  if (field.inputType === 'bool') return true
  if (field.inputType === 'sportTupleList') return [{ sport_type: RULE_TUPLE_SPORT_OPTIONS[0]?.value ?? 'running' }]
  if (field.inputType === 'enum' && field.operators?.[0] === 'in') return field.options?.[0]?.value ? [field.options[0].value] : []
  return ''
}

const OP_LABELS: Record<string, string> = {
  eq: '=', neq: '!=', gt: '>', lt: '<', gte: '>=', lte: '<=',
  contains: 'contains', not_contains: 'notContains',
  starts_with: 'startsWith', ends_with: 'endsWith', in: 'in',
  between: 'between',
}

function operatorLabel(op: string, t: TFunction): string {
  const label = OP_LABELS[op]
  if (!label) return op
  if (['=', '!=', '>', '<', '>=', '<='].includes(label)) return label
  return t(`ruleEditor.operators.${label}`)
}

function currentLanguage(language: string): SupportedLanguage {
  const code = language.split('-')[0]
  return isSupportedLanguage(code) ? code : 'de'
}

function conditionCategoryLabel(category: ConditionCategory, t: TFunction): string {
  return t(`ruleEditor.conditionCategories.${category.key}`)
}

function conditionFieldLabel(field: ConditionField, t: TFunction, language: SupportedLanguage): string {
  if ((RulesApplicableModifierKeys as readonly string[]).includes(field.field)) {
    return modifierLabel(field.field as SportModifierKey, language)
  }
  if (RULE_TRAIT_CONDITION_FIELD_KEYS.has(field.field) && field.labels) {
    return localizeString(field.labels, language)
  }
  return t(`automationRules.summary.fields.${field.field}`)
}

function optionLabel(field: string, option: { value: string; label: string; labels?: LocalizedString }, t: TFunction, language: SupportedLanguage): string {
  if (field === 'sport_type') return sportLabel(option.value, null, { language })
  if (RULE_TRAIT_CONDITION_FIELD_KEYS.has(field)) {
    return localizeString(option.labels, language) || option.label
  }
  if (field === 'source') return option.value === 'manual' ? t('ruleEditor.optionLabels.sources.manual') : option.label
  if (field === 'season') return t(`ruleEditor.optionLabels.seasons.${option.value}`)
  if (field === 'weekday') return t(`ruleEditor.optionLabels.weekdays.${option.value}`)
  if (field === 'month') return t(`ruleEditor.optionLabels.months.${option.value}`)
  if (field === 'visibility') return t(`ruleEditor.optionLabels.visibility.${option.value}`)
  if (field === 'intensity') return t(`ruleEditor.optionLabels.intensity.${option.value}`)
  if (field === 'time_of_day') return t(`ruleEditor.optionLabels.timeOfDay.${option.value}`)
  if (field === 'has_sensor_type') return t(`ruleEditor.optionLabels.sensorTypes.${option.value}`)
  if (field === 'swim_stroke') return t(`ruleEditor.optionLabels.swimStrokes.${option.value}`)
  return option.label
}

const MODIFIER_ACTION_TYPES = RulesApplicableModifierKeys.map((key) => ({
  value: modifierActionName(key),
  modifierKey: key,
}))

const ACTION_TYPES = [
  { value: 'set_gear',        labelKey: 'ruleEditor.actionTypes.setGear' },
  { value: 'add_tag',         labelKey: 'ruleEditor.actionTypes.addTag' },
  { value: 'remove_tag',      labelKey: 'ruleEditor.actionTypes.removeTag' },
  { value: 'set_visibility',  labelKey: 'ruleEditor.actionTypes.setVisibility' },
  { value: 'set_sport_type',  labelKey: 'ruleEditor.actionTypes.setSportType' },
  ...MODIFIER_ACTION_TYPES,
  { value: 'set_name',        labelKey: 'ruleEditor.actionTypes.setName' },
  { value: 'set_intensity',   labelKey: 'ruleEditor.actionTypes.setIntensity' },
  { value: 'set_exclude_from_stats',   labelKey: 'ruleEditor.actionTypes.setExcludeFromStats' },
  { value: 'set_exclude_from_heatmap', labelKey: 'ruleEditor.actionTypes.setExcludeFromHeatmap' },
  { value: 'set_hide_map',             labelKey: 'ruleEditor.actionTypes.setHideMap' },
]

function actionTypeLabel(action: (typeof ACTION_TYPES)[number] | undefined, t: TFunction, language: SupportedLanguage): string {
  if (!action) return ''
  if ('modifierKey' in action) return t('ruleEditor.actionTypes.setModifier', { label: modifierLabel(action.modifierKey, language) })
  return t(action.labelKey)
}

function actionLabelByValue(value: string, t: TFunction, language: SupportedLanguage): string {
  return actionTypeLabel(ACTION_TYPES.find((action) => action.value === value), t, language) || value
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

function defaultCondition(): RuleCondition {
  return { field: 'title', op: 'eq', value: '' }
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

type ModifierChoice = '' | 'true' | 'false'

const LEGACY_SPORT_TUPLE_ANCHOR_FIELDS = new Set(['sport_type', 'sub_type'])
const RULE_TUPLE_MODIFIER_KEYS = RulesApplicableModifierKeys as SportModifierKey[]
const LEGACY_SPORT_TUPLE_FIELDS = new Set(['sport_type', 'sub_type', ...RULE_TUPLE_MODIFIER_KEYS])
const CANONICAL_SPORT_TUPLE_FIELDS = new Set(['sport_tuple'])

function canUseSportTupleControl(conditions: RuleCondition[], groupLogic: RuleConditionGroup['logic'] = 'and') {
  const canonicalTupleConditions = conditions.filter((c) => c.field === 'sport_tuple')
  if (canonicalTupleConditions.length > 0) {
    return canonicalTupleConditions.length === 1 && String(canonicalTupleConditions[0].op ?? '').toLowerCase() === 'in'
  }
  if (groupLogic !== 'and') return false
  const tupleConditions = conditions.filter((c) => LEGACY_SPORT_TUPLE_FIELDS.has(c.field))
  const sportConditions = tupleConditions.filter((c) => c.field === 'sport_type')
  if (sportConditions.length !== 1) return false
  const sportCondition = sportConditions[0]
  if (!sportCondition || String(sportCondition.value ?? '') === '') return false
  if (!tupleConditions.some((c) => LEGACY_SPORT_TUPLE_ANCHOR_FIELDS.has(c.field))) return false
  return tupleConditions.every((c) => String(c.op ?? 'eq').toLowerCase() === 'eq')
}

function sportTupleControlFields(conditions: RuleCondition[], groupLogic: RuleConditionGroup['logic'] = 'and'): Set<string> {
  if (!canUseSportTupleControl(conditions, groupLogic)) return new Set()
  if (conditions.some((c) => c.field === 'sport_tuple')) return CANONICAL_SPORT_TUPLE_FIELDS
  return LEGACY_SPORT_TUPLE_FIELDS
}

function firstSelectableSubType(sportType: string): string {
  if (ruleTupleHasParentOption(sportType)) return ''
  return ruleTupleSubTypeOptions(sportType)[0]?.subType ?? ''
}

type RuleTupleModifierKey = SportModifierKey
type SportTupleListRow = {
  sport_type: string
  sub_type?: string
} & Partial<Record<RuleTupleModifierKey, boolean>>

const ACTION_MODIFIER_PARAM_BY_KEY = Object.fromEntries(
  RulesApplicableModifierKeys.map((key) => [key, modifierActionParam(key)]),
) as Record<RuleTupleModifierKey, string>
const MODIFIER_KEY_BY_ACTION = new Map(
  RulesApplicableModifierKeys.map((key) => [modifierActionName(key), key]),
)

function modifierKeyForAction(action: string): SportModifierKey | undefined {
  return MODIFIER_KEY_BY_ACTION.get(action)
}

function tupleModifierChoice(row: SportTupleListRow, key: RuleTupleModifierKey): ModifierChoice {
  return row[key] === true ? 'true' : row[key] === false ? 'false' : ''
}

function patchTupleModifier(row: SportTupleListRow, key: RuleTupleModifierKey, value: ModifierChoice): SportTupleListRow {
  const next = { ...row }
  if (value === '') {
    delete next[key]
  } else {
    next[key] = value === 'true'
  }
  return next
}

function sanitizeTupleRow(row: SportTupleListRow, surface: 'condition' | 'action'): SportTupleListRow {
  const controls = ruleModifierControlsForTuple(row.sport_type, row.sub_type ?? '')
  const applicable = new Set(controls.filter((control) => surface === 'condition' ? control.conditionApplicable : control.actionApplicable).map((control) => control.key))
  const next = { ...row }
  for (const key of RULE_TUPLE_MODIFIER_KEYS) {
    if (!applicable.has(key)) delete next[key]
  }
  return next
}

function tupleDefaultModifier(sportType: string, subType: string, key: RuleTupleModifierKey): boolean | undefined {
  const tuple = normalizeSportTuple(sportType, subType || null)
  return tuple.modifiers[modifierTSField(key) as keyof typeof tuple.modifiers]
}

function modifierChoiceFromActionParams(params: Record<string, any>, sportType: string, subType: string, key: RuleTupleModifierKey): ModifierChoice {
  const paramKey = ACTION_MODIFIER_PARAM_BY_KEY[key]
  const explicit = params[paramKey]
  const value = explicit === true || explicit === false ? explicit : tupleDefaultModifier(sportType, subType, key)
  return value === true ? 'true' : value === false ? 'false' : ''
}

function patchActionModifier(params: Record<string, any>, key: RuleTupleModifierKey, value: ModifierChoice) {
  const paramKey = ACTION_MODIFIER_PARAM_BY_KEY[key]
  const next = { ...params }
  if (value === '') {
    delete next[paramKey]
  } else {
    next[paramKey] = value === 'true'
  }
  return next
}

function sanitizeSportActionParams(params: Record<string, any>): Record<string, any> {
  const tuple = normalizeSportTuple(String(params.sport_type ?? ''), params.sub_type == null ? null : String(params.sub_type))
  const sportType = tuple.sportType
  const subType = tuple.subType && tuple.subType !== 'generic' ? tuple.subType : ''
  const controls = ruleModifierControlsForTuple(sportType, subType)
  const applicable = new Set(controls.filter((control) => control.actionApplicable).map((control) => control.key))
  const next: Record<string, any> = {
    ...params,
    sport_type: sportType,
    sub_type: subType,
  }
  for (const key of RULE_TUPLE_MODIFIER_KEYS) {
    if (!applicable.has(key)) delete next[ACTION_MODIFIER_PARAM_BY_KEY[key]]
  }
  return next
}

function normalizeTupleRow(row: SportTupleListRow): SportTupleListRow {
  const tuple = normalizeSportTuple(row.sport_type, row.sub_type == null ? null : row.sub_type)
  return {
    ...row,
    sport_type: tuple.sportType,
    sub_type: tuple.subType && tuple.subType !== 'generic' ? tuple.subType : undefined,
  }
}

function sportTupleRowsFromConditions(conditions: RuleCondition[]): SportTupleListRow[] {
  const sportTuple = conditions.find((c) => c.field === 'sport_tuple')
  if (sportTuple) {
    const rows = Array.isArray(sportTuple.value) ? sportTuple.value : []
    return rows.map((row) => normalizeTupleRow(row as SportTupleListRow)).filter((row) => row.sport_type)
  }
  const sport = conditions.find((c) => c.field === 'sport_type')
  const sub = conditions.find((c) => c.field === 'sub_type')
  const tuple = normalizeSportTuple(String(sport?.value ?? ''), sub?.value == null ? null : String(sub.value))
  const row: SportTupleListRow = {
    sport_type: tuple.sportType,
    sub_type: tuple.subType && tuple.subType !== 'generic' ? tuple.subType : undefined,
  }
  for (const key of RULE_TUPLE_MODIFIER_KEYS) {
    const value = tuple.modifiers[modifierTSField(key) as keyof typeof tuple.modifiers]
    if (value !== undefined) row[key] = value
  }
  for (const key of RULE_TUPLE_MODIFIER_KEYS) {
    const condition = conditions.find((c) => c.field === key)
    if (condition?.value === true) row[key] = true
    if (condition?.value === false) row[key] = false
  }
  return [row]
}

function replaceSportTupleConditions(conditions: RuleCondition[], rows: SportTupleListRow[]): RuleCondition[] {
  const replacedFields = conditions.some((c) => c.field === 'sport_tuple') ? CANONICAL_SPORT_TUPLE_FIELDS : LEGACY_SPORT_TUPLE_FIELDS
  const rest = conditions.filter((c) => !replacedFields.has(c.field))
  const value = rows.map((row) => {
    const normalized = normalizeTupleRow(row)
    const out: Record<string, any> = { sport_type: normalized.sport_type }
    if (normalized.sub_type) out.sub_type = normalized.sub_type
    for (const key of RULE_TUPLE_MODIFIER_KEYS) {
      if (normalized[key] !== undefined) out[key] = normalized[key]
    }
    return out
  })
  return [{ field: 'sport_tuple', op: 'in', value }, ...rest]
}

function tupleStableKey(row: SportTupleListRow): string {
  const normalized = normalizeTupleRow(row)
  const modifiers = RULE_TUPLE_MODIFIER_KEYS
    .filter((key) => normalized[key] !== undefined)
    .map((key) => `${key}=${normalized[key]}`)
    .join('|')
  return [normalized.sport_type, normalized.sub_type ?? '', modifiers].join(':')
}

function findDuplicateSportTupleGroup(groups: RuleConditionGroup[]): number | null {
  for (let i = 0; i < groups.length; i++) {
    const group = groups[i]
    if (!canUseSportTupleControl(group.conditions, group.logic)) continue
    const seen = new Set<string>()
    for (const row of sportTupleRowsFromConditions(group.conditions)) {
      const key = tupleStableKey(row)
      if (seen.has(key)) return i
      seen.add(key)
    }
  }
  return null
}

function validateConditionGroupsForSubmit(groups: RuleConditionGroup[], t: TFunction) {
  for (let groupIndex = 0; groupIndex < groups.length; groupIndex++) {
    for (const condition of groups[groupIndex].conditions) {
      if (!RULE_TRAIT_CONDITION_FIELD_KEYS.has(condition.field)) continue
      if (!Array.isArray(condition.value) || condition.value.length === 0) {
        throw new Error(t('ruleEditor.validation.taxonomyConditionValueRequired', { index: groupIndex + 1 }))
      }
    }
  }
}

function normaliseSportTupleConditionGroup(group: RuleConditionGroup): RuleConditionGroup {
  if (!canUseSportTupleControl(group.conditions, group.logic)) return group
  return {
    ...group,
    conditions: replaceSportTupleConditions(group.conditions, sportTupleRowsFromConditions(group.conditions)),
  }
}

function SportTupleListConditionControl({
  conditions,
  onChange,
  onRemove,
}: {
  conditions: RuleCondition[]
  onChange: (conditions: RuleCondition[]) => void
  onRemove: () => void
}) {
  const { t, i18n } = useTranslation()
  const language = currentLanguage(i18n.language)
  const rows = sportTupleRowsFromConditions(conditions)

  function updateRows(nextRows: SportTupleListRow[]) {
    onChange(replaceSportTupleConditions(conditions, nextRows))
  }

  function patchRow(index: number, patch: Partial<SportTupleListRow>, changedTuple = false) {
    const current = rows[index] ?? { sport_type: RULE_TUPLE_SPORT_OPTIONS[0]?.value ?? 'running' }
    let next = { ...current, ...patch }
    if (changedTuple) next = sanitizeTupleRow(next, 'condition')
    updateRows(rows.map((row, i) => i === index ? next : row))
  }

  function addRow() {
    updateRows([...rows, { sport_type: RULE_TUPLE_SPORT_OPTIONS[0]?.value ?? 'running' }])
  }

  function removeRow(index: number) {
    const nextRows = rows.filter((_, i) => i !== index)
    if (nextRows.length === 0) {
      onRemove()
      return
    }
    updateRows(nextRows)
  }

  return (
    <div className="space-y-2">
      {rows.map((row, index) => {
        const subTypes = ruleTupleSubTypeOptions(row.sport_type)
        const controls = ruleModifierControlsForTuple(row.sport_type, row.sub_type ?? '')
        return (
          <div key={index} className="flex items-end gap-2 flex-wrap">
            <label className="text-xs text-neutral-500 dark:text-neutral-400 w-44">
              <span className="block mb-1">{t('ruleEditor.conditionControls.sportType')}</span>
              <select
                aria-label={t('ruleEditor.conditionControls.sportType')}
                value={row.sport_type}
                onChange={(e) => {
                  const sportType = e.target.value
                  patchRow(index, { sport_type: sportType, sub_type: firstSelectableSubType(sportType) || undefined }, true)
                }}
                className={inputClass + ' w-full'}
              >
                {RULE_TUPLE_SPORT_OPTIONS.map((s) => <option key={s.value} value={s.value}>{sportLabel(s.value, null, { language })}</option>)}
              </select>
            </label>
            <label className="text-xs text-neutral-500 dark:text-neutral-400 w-40">
              <span className="block mb-1">{t('ruleEditor.conditionControls.subSportType')}</span>
              <select
                aria-label={t('ruleEditor.conditionControls.subSportType')}
                value={row.sub_type ?? ''}
                onChange={(e) => patchRow(index, { sub_type: e.target.value || undefined }, true)}
                className={inputClass + ' w-full'}
              >
                {ruleTupleHasParentOption(row.sport_type) && <option value="">{t('ruleEditor.choices.any')}</option>}
                {subTypes.map((s) => <option key={s.subType} value={s.subType}>{sportLabel(s.sportType, s.subType, { language })}</option>)}
              </select>
            </label>
            {controls.map((control) => {
              const key = control.key as RuleTupleModifierKey
              const hasStoredValue = row[key] !== undefined
              const label = modifierLabel(key, language)
              if (!control.conditionApplicable && !(hasStoredValue && control.renderExisting)) return null
              return (
                <label key={control.key} className="text-xs text-neutral-500 dark:text-neutral-400 w-32">
                  <span className="block mb-1">{label}</span>
                  <select
                    aria-label={label}
                    value={tupleModifierChoice(row, key)}
                    disabled={!control.conditionApplicable}
                    onChange={(e) => patchRow(index, patchTupleModifier(row, key, e.target.value as ModifierChoice))}
                    className={inputClass + ' w-full'}
                  >
                    <option value="">{t('ruleEditor.choices.any')}</option>
                    <option value="true">{t('ruleEditor.choices.yes')}</option>
                    <option value="false">{t('ruleEditor.choices.no')}</option>
                  </select>
                </label>
              )
            })}
            <button
              type="button"
              onClick={() => removeRow(index)}
              className="text-neutral-400 hover:text-rose-500 transition-colors flex-shrink-0 cursor-pointer mb-2"
            >
              <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
              </svg>
            </button>
          </div>
        )
      })}
      <button type="button" onClick={addRow} className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer">
        {t('ruleEditor.conditionControls.addSportType')}
      </button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Small UI bits
// ---------------------------------------------------------------------------

const inputClass = 'border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent'

function HelpHint({ text }: { text: string }) {
  return (
    <span className="group relative inline-flex">
      <HelpCircle className="w-3.5 h-3.5 text-neutral-400 dark:text-neutral-500 cursor-help" aria-label={text} />
      <span
        role="tooltip"
        className="absolute left-1/2 top-full mt-3 z-50 -translate-x-1/2 w-64 rounded-md
                   bg-neutral-900 dark:bg-neutral-100 text-neutral-100 dark:text-neutral-900
                   text-xs leading-snug px-3 py-2 shadow-lg pointer-events-none normal-case
                   opacity-0 invisible group-hover:opacity-100 group-hover:visible
                   transition-[opacity,visibility] duration-100 delay-500"
      >
        {text}
      </span>
    </span>
  )
}

const SPEED_INPUT_FIELDS = new Set(['avg_speed_ms', 'max_speed_ms', 'min_speed_ms', 'weather_wind_speed_ms', 'weather_wind_gusts_ms'])
const PACE_INPUT_FIELDS = new Set(['avg_pace_min_km', 'min_pace_min_km', 'max_pace_min_km'])
const TEMPERATURE_INPUT_FIELDS = new Set(['avg_temperature_c', 'min_temperature_c', 'max_temperature_c', 'weather_temp_c', 'weather_temp_min_c', 'weather_temp_max_c'])

function roundRuleNumber(value: number): number {
  return Number(value.toFixed(3))
}

function speedToDisplayValue(metersPerSecond: number, units: UnitSystem): number {
  const display = units === 'imperial'
    ? (metersPerSecond * 3600) / METERS_PER_MILE
    : metersPerSecond * 3.6
  return roundRuleNumber(display)
}

function speedFromDisplayValue(value: number, units: UnitSystem): number {
  const stored = units === 'imperial'
    ? (value * METERS_PER_MILE) / 3600
    : value / 3.6
  return roundRuleNumber(stored)
}

function paceToDisplayValue(minutesPerKm: number, units: UnitSystem): number {
  return roundRuleNumber(units === 'imperial' ? minutesPerKm * (METERS_PER_MILE / 1000) : minutesPerKm)
}

function paceFromDisplayValue(value: number, units: UnitSystem): number {
  return roundRuleNumber(units === 'imperial' ? value / (METERS_PER_MILE / 1000) : value)
}

function temperatureToDisplayValue(celsius: number, units: UnitSystem): number {
  return roundRuleNumber(units === 'imperial' ? (celsius * 9) / 5 + 32 : celsius)
}

function temperatureFromDisplayValue(value: number, units: UnitSystem): number {
  return roundRuleNumber(units === 'imperial' ? ((value - 32) * 5) / 9 : value)
}

function displayNumberValue(fieldDef: ConditionField, value: unknown, unitOptions: UnitFormatOptions): number | string {
  if (value === '' || value == null) return ''
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return ''
  if (fieldDef.unit === 'distance') return roundRuleNumber(distanceToDisplayValue(numeric, unitOptions.units))
  if (fieldDef.unit === 'length') return roundRuleNumber(elevationToDisplayValue(numeric, unitOptions.units))
  if (SPEED_INPUT_FIELDS.has(fieldDef.field)) return speedToDisplayValue(numeric, unitOptions.units)
  if (PACE_INPUT_FIELDS.has(fieldDef.field)) return paceToDisplayValue(numeric, unitOptions.units)
  if (TEMPERATURE_INPUT_FIELDS.has(fieldDef.field)) return temperatureToDisplayValue(numeric, unitOptions.units)
  return numeric
}

function storedNumberValue(fieldDef: ConditionField, value: string, unitOptions: UnitFormatOptions): number | string {
  if (value === '') return ''
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return ''
  if (fieldDef.unit === 'distance') return roundRuleNumber(distanceFromDisplayValue(numeric, unitOptions.units))
  if (fieldDef.unit === 'length') return roundRuleNumber(elevationFromDisplayValue(numeric, unitOptions.units))
  if (SPEED_INPUT_FIELDS.has(fieldDef.field)) return speedFromDisplayValue(numeric, unitOptions.units)
  if (PACE_INPUT_FIELDS.has(fieldDef.field)) return paceFromDisplayValue(numeric, unitOptions.units)
  if (TEMPERATURE_INPUT_FIELDS.has(fieldDef.field)) return temperatureFromDisplayValue(numeric, unitOptions.units)
  return numeric
}

function displayRangePart(fieldDef: ConditionField, value: string, unitOptions: UnitFormatOptions): string {
  const display = displayNumberValue(fieldDef, value, unitOptions)
  return display === '' ? '' : String(display)
}

function storedRangePart(fieldDef: ConditionField, value: string, unitOptions: UnitFormatOptions): string {
  const stored = storedNumberValue(fieldDef, value, unitOptions)
  return stored === '' ? '' : String(stored)
}

function unitLabel(unit: RuleUnit | undefined, unitOptions: UnitFormatOptions): string | null {
  switch (unit) {
    case 'seconds': return 's'
    case 'distance': return distanceInputUnit(unitOptions.units)
    case 'length': return elevationInputUnit(unitOptions.units)
    case 'speed': return unitOptions.units === 'imperial' ? 'mph' : 'km/h'
    case 'pace': return unitOptions.units === 'imperial' ? 'min/mi' : 'min/km'
    case 'temperature': return unitOptions.units === 'imperial' ? '°F' : '°C'
    case 'percent': return '%'
    case 'degrees': return '°'
    case 'bpm': return 'bpm'
    case 'rpm': return 'rpm'
    case 'watts': return 'W'
    default: return null
  }
}

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
  const { t } = useTranslation()
  const unitFormatters = useUnitFormatters()
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

  const radiusDisplay = unitFormatters.formatDistanceCompact(radiusM)
  const radiusInputValue = roundRuleNumber(distanceToDisplayValue(radiusM, unitFormatters.options.units))
  const radiusInputStep = unitFormatters.options.units === 'imperial'
    ? roundRuleNumber(50 / METERS_PER_MILE)
    : 0.05
  const radiusInputUnit = distanceInputUnit(unitFormatters.options.units)

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 flex-wrap">
        <div className="flex items-center gap-2 flex-1 min-w-[260px]">
          <label className="text-xs text-neutral-600 dark:text-neutral-400 whitespace-nowrap">
            {t('ruleEditor.geofence.radius')} <span className="font-medium text-neutral-900 dark:text-neutral-100">{radiusDisplay}</span>
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
            min={radiusInputStep}
            step={radiusInputStep}
            value={radiusInputValue}
            onChange={(e) => {
              const fallback = distanceToDisplayValue(1000, unitFormatters.options.units)
              const rawValue = Number(e.target.value) || fallback
              const v = Math.max(50, Math.round(distanceFromDisplayValue(rawValue, unitFormatters.options.units)))
              onChange(lat, lon, v)
            }}
            className={inputClass + ' w-24 py-1 text-xs'}
            title={t('ruleEditor.geofence.radius')}
          />
          <span className="text-xs text-neutral-400">{radiusInputUnit}</span>
        </div>
        <div className="flex items-center gap-1 text-xs text-neutral-500">
          <span>{t('ruleEditor.geofence.latitude')}</span>
          <span className="font-mono tabular-nums">{lat ? lat.toFixed(5) : '—'}</span>
          <span className="ml-2">{t('ruleEditor.geofence.longitude')}</span>
          <span className="font-mono tabular-nums">{lon ? lon.toFixed(5) : '—'}</span>
        </div>
      </div>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">{t('ruleEditor.geofence.setCenterHint')}</p>
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
  const { t } = useTranslation()
  const selectClass = 'text-xs border border-neutral-300 dark:border-neutral-600 rounded px-1.5 py-1 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100'

  return (
    <div className="space-y-3 border border-neutral-200 dark:border-neutral-700 rounded-lg p-3 bg-neutral-50 dark:bg-neutral-800/30">
      <div className="flex gap-2 flex-wrap">
        <label className="flex items-center gap-1 text-xs text-neutral-500">
          {t('ruleEditor.geofence.check')}
          <select
            value={value.check}
            onChange={(e) => onChange({ ...value, check: e.target.value as 'start' | 'end' | 'track' })}
            className={selectClass}
          >
            <option value="start">{t('ruleEditor.geofence.start')}</option>
            <option value="end">{t('ruleEditor.geofence.end')}</option>
            <option value="track">{t('ruleEditor.geofence.track')}</option>
          </select>
        </label>
        <label className="flex items-center gap-1 text-xs text-neutral-500">
          {t('ruleEditor.geofence.mode')}
          <select
            value={value.mode}
            onChange={(e) => onChange({ ...value, mode: e.target.value as 'inside' | 'outside' })}
            className={selectClass}
          >
            <option value="inside">{t('ruleEditor.geofence.inside')}</option>
            <option value="outside">{t('ruleEditor.geofence.outside')}</option>
          </select>
        </label>
        <label className="flex items-center gap-1 text-xs text-neutral-500">
          {t('ruleEditor.geofence.shape')}
          <select
            value={value.shape}
            onChange={(e) => onChange({ ...value, shape: e.target.value as 'circle' | 'polygon' })}
            className={selectClass}
          >
            <option value="circle">{t('ruleEditor.geofence.circle')}</option>
            <option value="polygon">{t('ruleEditor.geofence.polygon')}</option>
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
  gearList,
  recorders,
  sensors,
}: {
  fieldDef: ConditionField
  cond: RuleCondition
  onChange: (patch: Partial<RuleCondition>) => void
  tags: Array<{ id: string; name: string; color: string }>
  gearList: Gear[]
  recorders?: Device[]
  sensors?: Device[]
}) {
  const { t, i18n } = useTranslation()
  const language = currentLanguage(i18n.language)
  const unitFormatters = useUnitFormatters()
  const { inputType } = fieldDef
  const segmentQuery = useQuery({
    queryKey: ['rule-editor-segments'],
    queryFn: () => fetchSegments({ limit: 100 }),
    enabled: inputType === 'segment',
    staleTime: 60_000,
  })
  const segmentOptions = segmentQuery.data?.segments ?? []
  const selectedSegment = segmentOptions.find(segment => segment.id === String(cond.value ?? ''))

  // Resolve dynamic options for device fields
  function deviceOption(d: Device) {
    return { value: d.id, label: d.custom_name || d.product_name || d.manufacturer || d.id }
  }
  function gearOptions() {
    const options = gearList
      .filter((gear) => !gear.is_retired)
      .map((gear) => ({ value: gear.id, label: gear.name || gear.id }))
    const known = new Set(options.map((option) => option.value))
    const selected = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
    for (const value of selected) {
      const id = String(value)
      if (!id || known.has(id)) continue
      options.push({ value: id, label: t('ruleEditor.conditionControls.unlistedGear', { id }) })
      known.add(id)
    }
    return options
  }
  const resolvedFieldDef: ConditionField = (() => {
    if (fieldDef.field === 'gear_id') {
      return { ...fieldDef, options: gearOptions() }
    }
    if (fieldDef.field === 'recording_device_id' && recorders) {
      return { ...fieldDef, options: recorders.map(deviceOption) }
    }
    if (fieldDef.field === 'sensor_device_ids' && sensors) {
      return { ...fieldDef, options: sensors.map(deviceOption) }
    }
    return fieldDef
  })()
  const effectiveOptions = resolvedFieldDef.options ?? fieldDef.options

  if (inputType === 'sportTupleList') {
    return (
      <div className="flex-1 min-w-0">
        <SportTupleListConditionControl
          conditions={[cond]}
          onChange={(conditions) => {
            const sportTuple = conditions.find((condition) => condition.field === 'sport_tuple')
            onChange({ field: 'sport_tuple', op: 'in', value: sportTuple?.value ?? [] })
          }}
          onRemove={() => onChange({ value: [] })}
        />
      </div>
    )
  }

  if (inputType === 'bool') {
    return (
      <select
        value={String(cond.value ?? 'true')}
        onChange={(e) => onChange({ value: e.target.value === 'true' })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="true">{t('ruleEditor.choices.yes')}</option>
        <option value="false">{t('ruleEditor.choices.no')}</option>
      </select>
    )
  }

  if (inputType === 'enum' && effectiveOptions) {
    if (cond.op === 'in' || cond.op === 'contains' || cond.op === 'not_contains') {
      const selected: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
      function toggle(v: string) {
        if (RULE_TRAIT_CONDITION_FIELD_KEYS.has(cond.field) && selected.includes(v) && selected.length === 1) {
          return
        }
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
              {optionLabel(cond.field, s, t, language)}
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
        <option value="">{t('ruleEditor.conditionControls.choose')}</option>
        {effectiveOptions.map(s => <option key={s.value} value={s.value}>{optionLabel(cond.field, s, t, language)}</option>)}
      </select>
    )
  }

  if (inputType === 'number' && cond.op === 'between') {
    const parts = typeof cond.value === 'string' ? cond.value.split(',') : ['', '']
    const from = displayRangePart(fieldDef, parts[0] ?? '', unitFormatters.options)
    const to = displayRangePart(fieldDef, parts[1] ?? '', unitFormatters.options)
    const displayUnit = unitLabel(fieldDef.unit, unitFormatters.options)
    return (
      <div className="flex items-center gap-2 flex-1 min-w-0">
        <input type="number" placeholder={t('ruleEditor.conditionControls.from')} value={from}
          onChange={(e) => onChange({ value: `${storedRangePart(fieldDef, e.target.value, unitFormatters.options)},${storedRangePart(fieldDef, to, unitFormatters.options)}` })}
          className={inputClass + ' w-24'}
        />
        <span className="text-neutral-500 text-xs">–</span>
        <input type="number" placeholder={t('ruleEditor.conditionControls.to')} value={to}
          onChange={(e) => onChange({ value: `${storedRangePart(fieldDef, from, unitFormatters.options)},${storedRangePart(fieldDef, e.target.value, unitFormatters.options)}` })}
          className={inputClass + ' w-24'}
        />
        {displayUnit && <span className="text-xs text-neutral-400">{displayUnit}</span>}
      </div>
    )
  }

  if (inputType === 'number') {
    const displayUnit = unitLabel(fieldDef.unit, unitFormatters.options)
    return (
      <div className="flex items-center gap-1 flex-1 min-w-0">
        <input type="number" value={displayNumberValue(fieldDef, cond.value, unitFormatters.options)}
          onChange={(e) => onChange({ value: storedNumberValue(fieldDef, e.target.value, unitFormatters.options) })}
          placeholder={t('ruleEditor.conditionControls.valuePlaceholder')}
          className={inputClass + ' flex-1 min-w-0'}
        />
        {displayUnit && <span className="text-xs text-neutral-400">{displayUnit}</span>}
      </div>
    )
  }

  if (inputType === 'tag') {
    return (
      <select value={String(cond.value ?? '')} onChange={(e) => onChange({ value: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">{t('ruleEditor.conditionControls.chooseTag')}</option>
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
      <div className="flex-1 min-w-0 space-y-1">
        <select
          aria-label={t('ruleEditor.conditionControls.chooseSegment')}
          value={String(cond.value ?? '')}
          onChange={(e) => onChange({ value: e.target.value })}
          className={inputClass + ' w-full min-w-0'}
        >
          <option value="">{t('ruleEditor.conditionControls.chooseSegmentWithDots')}</option>
          {segmentOptions.map(segment => (
            <option key={segment.id} value={segment.id}>{segment.name}</option>
          ))}
          {cond.value && !selectedSegment && (
            <option value={String(cond.value)}>{t('ruleEditor.conditionControls.unlistedSegment', { id: String(cond.value) })}</option>
          )}
        </select>
        {selectedSegment && (
          <p className="text-[11px] text-neutral-500 dark:text-neutral-400">
            {segmentPreview(selectedSegment, t, unitFormatters.options)}
          </p>
        )}
      </div>
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
      placeholder={t('ruleEditor.conditionControls.valuePlaceholder')}
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
  gearList,
  recorders,
  sensors,
}: {
  cond: RuleCondition
  onUpdate: (patch: Partial<RuleCondition>) => void
  onRemove: () => void
  tags: Array<{ id: string; name: string; color: string }>
  gearList: Gear[]
  recorders?: Device[]
  sensors?: Device[]
}) {
  const { t, i18n } = useTranslation()
  const language = currentLanguage(i18n.language)
  const currentCat = findCategory(cond.field)
  const currentField = findFieldDef(cond.field)
  const isInternalField = !currentCat  // geofence, is_round_trip etc.

  // For internal fields (non-category), show a simplified row
  if (isInternalField && currentField) {
    return (
      <div className={currentField.inputType === 'geofence' ? 'space-y-2' : 'flex items-center gap-2 flex-wrap'}>
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">{conditionFieldLabel(currentField, t, language)}</span>
        <ConditionValueEditor fieldDef={currentField} cond={cond} onChange={onUpdate} tags={tags} gearList={gearList} recorders={recorders} sensors={sensors} />
        <button type="button" onClick={onRemove}
          className="text-neutral-400 hover:text-rose-500 transition-colors flex-shrink-0 cursor-pointer"
          title={t('ruleEditor.conditionControls.removeCondition')}
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
  const isGeofence = safeField.inputType === 'geofence'

  function handleCategoryChange(catKey: string) {
    const cat = CONDITION_CATEGORIES.find(c => c.key === catKey) ?? CONDITION_CATEGORIES[0]
    const firstField = cat.fields[0]
    const firstOps = opsForField(firstField)
    onUpdate({ field: firstField.field, op: firstOps[0], value: defaultValueForField(firstField) })
  }

  function handleFieldChange(fieldName: string) {
    const fd = safeCat.fields.find(f => f.field === fieldName) ?? safeCat.fields[0]
    const fieldOps = opsForField(fd)
    onUpdate({ field: fd.field, op: fieldOps[0], value: defaultValueForField(fd) })
  }

  return (
    <div className={isGeofence ? 'space-y-2' : 'flex items-center gap-2 flex-wrap'}>
      {/* Selectors row — always flex */}
      <div className={isGeofence ? 'flex items-center gap-2 flex-wrap' : 'contents'}>
        {/* Category selector */}
        <select
          value={safeCat.key}
          onChange={(e) => handleCategoryChange(e.target.value)}
          className={inputClass}
        >
          {CONDITION_CATEGORIES.map(cat => (
            <option key={cat.key} value={cat.key}>{conditionCategoryLabel(cat, t)}</option>
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
              <option key={f.field} value={f.field}>{conditionFieldLabel(f, t, language)}</option>
            ))}
          </select>
        )}

        {/* Operator selector (non-geofence only — geofence has no operator) */}
        {!isGeofence && ops.length > 1 && (
          <select
            value={cond.op}
            onChange={(e) => onUpdate({ op: e.target.value })}
            className={inputClass}
          >
            {ops.map(op => (
              <option key={op} value={op}>{operatorLabel(op, t)}</option>
            ))}
          </select>
        )}

        {/* Remove button for geofence rows (inside the header flex row, right-aligned) */}
        {isGeofence && (
          <button
            type="button"
            onClick={onRemove}
            className="text-neutral-400 hover:text-rose-500 transition-colors flex-shrink-0 cursor-pointer ml-auto"
            title={t('ruleEditor.conditionControls.removeCondition')}
          >
            <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          </button>
        )}
      </div>

      {/* Value editor — inline for normal fields, second row for geofence */}
      <ConditionValueEditor fieldDef={safeField} cond={cond} onChange={onUpdate} tags={tags} gearList={gearList} recorders={recorders} sensors={sensors} />

      {/* Remove button for normal rows — at the end of the flex row after the value editor */}
      {!isGeofence && (
        <button
          type="button"
          onClick={onRemove}
          className="text-neutral-400 hover:text-rose-500 transition-colors flex-shrink-0 cursor-pointer"
          title={t('ruleEditor.conditionControls.removeCondition')}
        >
          <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
          </svg>
        </button>
      )}
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
  gearList,
  recorders,
  sensors,
}: {
  group: RuleConditionGroup
  groupIndex: number
  totalGroups: number
  onChange: (g: RuleConditionGroup) => void
  onRemove: () => void
  tags: Array<{ id: string; name: string; color: string }>
  gearList: Gear[]
  recorders?: Device[]
  sensors?: Device[]
}) {
  const { t } = useTranslation()
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
  const sportTupleFields = sportTupleControlFields(group.conditions, group.logic)
  const hasSportTuple = sportTupleFields.size > 0
  const visibleConditions = group.conditions
    .map((cond, index) => ({ cond, index }))
    .filter(({ cond }) => !(hasSportTuple && sportTupleFields.has(cond.field)))

  return (
    <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg p-3 space-y-2 bg-white dark:bg-neutral-900/30">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="text-xs text-neutral-500 dark:text-neutral-400 font-medium">{t('ruleEditor.conditionControls.groupLabel', { index: groupIndex + 1 })}</span>
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
                {l === 'and' ? t('ruleEditor.logic.and') : t('ruleEditor.logic.or')}
              </button>
            ))}
          </div>
          <span className="text-xs text-neutral-400">{t('ruleEditor.conditionControls.withinGroup')}</span>
        </div>
        {totalGroups > 1 && (
          <button type="button" onClick={onRemove}
            className="text-xs text-neutral-400 hover:text-rose-500 transition-colors cursor-pointer"
            title={t('ruleEditor.conditionControls.removeGroup')}
          >
            {t('ruleEditor.conditionControls.removeGroup')}
          </button>
        )}
      </div>

      <div className="space-y-2">
        {hasSportTuple && (
          <div>
            <SportTupleListConditionControl
              conditions={group.conditions}
              onChange={(conditions) => onChange({ ...group, conditions })}
              onRemove={() => onChange({ ...group, conditions: group.conditions.filter((c) => !sportTupleFields.has(c.field)) })}
            />
          </div>
        )}
        {visibleConditions.map(({ cond, index }, visibleIndex) => (
          <div key={index}>
            {(hasSportTuple || visibleIndex > 0) && (
              <div className="flex items-center gap-2 my-1">
                <span className="text-xs font-semibold text-accent-DEFAULT uppercase tracking-wide px-1">
                  {group.logic === 'and' ? t('ruleEditor.logic.and') : t('ruleEditor.logic.or')}
                </span>
              </div>
            )}
            <ConditionRow
              cond={cond}
              onUpdate={(patch) => updateCondition(index, patch)}
              onRemove={() => removeCondition(index)}
              tags={tags}
              gearList={gearList}
              recorders={recorders}
              sensors={sensors}
            />
          </div>
        ))}
      </div>

      <button
        type="button"
        onClick={addCondition}
        className="text-xs font-medium px-2.5 py-1 rounded border border-accent-DEFAULT/50 dark:border-orange-400/50 text-accent-600 dark:text-orange-400 hover:bg-accent-DEFAULT/10 dark:hover:bg-orange-400/10 transition-colors cursor-pointer"
      >
        {t('ruleEditor.conditionControls.addCondition')}
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
  t,
  language,
}: {
  action: string
  params: Record<string, any>
  gearList: any[]
  tags: Array<{ id: string; name: string; color: string }>
  onChange: (params: Record<string, any>) => void
  t: TFunction
  language: SupportedLanguage
}) {
  if (action === 'set_gear') {
    return (
      <select
        value={params.gear_id ?? ''}
        onChange={(e) => onChange({ gear_id: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">{t('ruleEditor.actionParams.chooseGear')}</option>
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
        <option value="">{t('ruleEditor.actionParams.chooseTag')}</option>
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
        <option value="">{t('ruleEditor.actionParams.chooseTag')}</option>
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
        <option value="">{t('ruleEditor.actionParams.chooseVisibility')}</option>
        {VISIBILITY_OPTIONS.map((option) => (
          <option key={option.value} value={option.value}>{optionLabel('visibility', option, t, language)}</option>
        ))}
      </select>
    )
  }
  if (action === 'set_sport_type') {
    const tuple = normalizeSportTuple(String(params.sport_type ?? ''), params.sub_type == null ? null : String(params.sub_type))
    const sportType = tuple.sportType
    const subType = tuple.subType && tuple.subType !== 'generic' ? tuple.subType : ''
    const subTypes = ruleTupleSubTypeOptions(sportType)
    const controls = ruleModifierControlsForTuple(sportType, subType)
    return (
      <div className="space-y-2 flex-1 min-w-0">
        <div className="flex items-end gap-2 flex-wrap">
          <label className="text-xs text-neutral-500 dark:text-neutral-400 flex-1 min-w-48">
            <span className="block mb-1">{t('ruleEditor.actionParams.setSportType')}</span>
            <select
              aria-label={t('ruleEditor.actionParams.setSportType')}
              value={sportType}
              onChange={(e) => {
                const nextSport = e.target.value
                onChange(sanitizeSportActionParams({ ...params, sport_type: nextSport, sub_type: firstSelectableSubType(nextSport) }))
              }}
              className={inputClass + ' w-full'}
            >
              <option value="">{t('ruleEditor.actionParams.chooseSport')}</option>
              {RULE_TUPLE_SPORT_OPTIONS.map(s => <option key={s.value} value={s.value}>{sportLabel(s.value, null, { language })}</option>)}
            </select>
          </label>
          <label className="text-xs text-neutral-500 dark:text-neutral-400">
            <span className="block mb-1">{t('ruleEditor.actionParams.setSubSportType')}</span>
            <select
              aria-label={t('ruleEditor.actionParams.setSubSportType')}
              value={subType}
              onChange={(e) => onChange(sanitizeSportActionParams({ ...params, sport_type: sportType, sub_type: e.target.value }))}
              className={inputClass + ' w-40'}
            >
              {ruleTupleHasParentOption(sportType) && <option value="">{t('ruleEditor.actionParams.noSubSport')}</option>}
              {subTypes.map((s) => <option key={s.subType} value={s.subType}>{sportLabel(s.sportType, s.subType, { language })}</option>)}
            </select>
          </label>
        </div>
        <div className="flex items-end gap-2 flex-wrap">
          {controls.map((control) => {
            const key = control.key as RuleTupleModifierKey
            const paramKey = ACTION_MODIFIER_PARAM_BY_KEY[key]
            const hasStoredValue = params[paramKey] !== undefined
            const label = modifierLabel(key, language)
            if (!control.actionApplicable && !(hasStoredValue && control.renderExisting)) return null
            return (
              <label key={control.key} className="text-xs text-neutral-500 dark:text-neutral-400">
                <span className="block mb-1">{t('ruleEditor.actionParams.setModifierLabel', { label })}</span>
                <select
                  aria-label={t('ruleEditor.actionParams.setModifierLabel', { label })}
                  value={modifierChoiceFromActionParams(params, sportType, subType, key)}
                  disabled={!control.actionApplicable}
                  onChange={(e) => onChange(sanitizeSportActionParams({
                    ...patchActionModifier(params, key, e.target.value as ModifierChoice),
                    sport_type: sportType,
                    sub_type: subType,
                  }))}
                  className={inputClass + ' w-36'}
                >
                  <option value="">{t('ruleEditor.choices.any')}</option>
                  <option value="true">{t('ruleEditor.choices.yes')}</option>
                  <option value="false">{t('ruleEditor.choices.no')}</option>
                </select>
              </label>
            )
          })}
        </div>
      </div>
    )
  }
  const modifierActionKey = modifierKeyForAction(action)
  if (modifierActionKey) {
    const label = modifierLabel(modifierActionKey, language)
    const paramKey = modifierActionParam(modifierActionKey)
    return (
      <select
        value={String(params[paramKey] ?? 'true')}
        onChange={(e) => onChange({ [paramKey]: e.target.value === 'true' })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="true">{t('ruleEditor.actionParams.yesWithLabel', { label })}</option>
        <option value="false">{t('ruleEditor.actionParams.noWithoutLabel', { label })}</option>
      </select>
    )
  }
  if (action === 'set_name') {
    return (
      <div className="flex flex-col gap-1 flex-1 min-w-0">
        <input
          type="text"
          value={params.template ?? ''}
          onChange={(e) => onChange({ template: e.target.value })}
          placeholder={t('ruleEditor.actionParams.newTitle')}
          className={inputClass + ' w-full'}
        />
        <span className="text-xs text-neutral-400">{t('ruleEditor.actionParams.templateHelp')}</span>
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
        <option value="">{t('ruleEditor.actionParams.chooseIntensity')}</option>
        {INTENSITY_OPTIONS.map((option) => (
          <option key={option.value} value={option.value}>{optionLabel('intensity', option, t, language)}</option>
        ))}
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
        <option value="true">{t('ruleEditor.actionParams.yesExclude')}</option>
        <option value="false">{t('ruleEditor.actionParams.noInclude')}</option>
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
        <option value="true">{t('ruleEditor.actionParams.yesExclude')}</option>
        <option value="false">{t('ruleEditor.actionParams.noInclude')}</option>
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
          <option value="true">{t('ruleEditor.actionParams.yesHideMap')}</option>
          <option value="false">{t('ruleEditor.actionParams.noShowMap')}</option>
        </select>
        <p className="text-xs text-amber-600 mt-1">
          {t('ruleEditor.actionParams.hideMapWarning')}
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

const MODIFIER_ACTION_SPECS = RulesApplicableModifierKeys.map((key) => ({
  action: modifierActionName(key),
  field: key,
  param: modifierActionParam(key),
  tag: modifierTagSync(key)?.systemTag,
  onlySportType: modifierTagSyncOnlySportType(key),
}))

/** When loading saved rules, translate set_field actions back to frontend action types. */
function denormaliseActions(raw: RuleAction[]): RuleAction[] {
  const out: RuleAction[] = []
  for (let i = 0; i < raw.length; i++) {
    const a = raw[i]
    if (a.action === 'set_field' && a.params?.field && SET_FIELD_FIELD_TO_ACTION[a.params.field]) {
      out.push({ action: SET_FIELD_FIELD_TO_ACTION[a.params.field], params: { value: a.params.value ?? false } })
      continue
    }
    if (a.action === 'set_sport_type') {
      const tuple = normalizeSportTuple(String(a.params?.sport_type ?? ''), a.params?.sub_type == null ? null : String(a.params.sub_type))
      const params: Record<string, any> = {
        ...a.params,
        sport_type: tuple.sportType,
        sub_type: tuple.subType && tuple.subType !== 'generic' ? tuple.subType : '',
      }
      for (const key of RULE_TUPLE_MODIFIER_KEYS) {
        const value = tuple.modifiers[modifierTSField(key) as keyof typeof tuple.modifiers]
        if (value !== undefined) params[modifierActionParam(key)] = value
      }
      out.push({ action: 'set_sport_type', params })
      continue
    }
    out.push(a)
  }
  return out
}

function dedupeSystemTagActions(actions: RuleAction[], tagNameForAction: (action: RuleAction) => string | undefined, t: TFunction): RuleAction[] {
  const out: RuleAction[] = []
  const modifierState = new Map<string, boolean>()
  const specs = MODIFIER_ACTION_SPECS
  const cyclingActionContext = actions.some((action) =>
    action.action === 'set_sport_type' && action.params?.sport_type === 'cycling'
  )

  function tagAppliesToModifier(spec: (typeof MODIFIER_ACTION_SPECS)[number], tagName: string): boolean {
    if (!spec.tag) return false
    if (spec.tag !== tagName) return false
    if (spec.onlySportType === 'cycling') return cyclingActionContext
    if (spec.onlySportType) return actions.some((action) =>
      action.action === 'set_sport_type' && action.params?.sport_type === spec.onlySportType
    )
    return true
  }

  for (const action of actions) {
    const spec = specs.find((s) => s.action === action.action)
    if (spec) {
      const value = action.params?.[spec.param] ?? action.params?.value ?? true
      for (let i = out.length - 1; i >= 0; i--) {
        const existing = out[i]
        const tagName = tagNameForAction(existing)
        if (!tagName || !tagAppliesToModifier(spec, tagName)) continue
        if (existing.action === 'add_tag' && value === true) {
          out.splice(i, 1)
        } else if (existing.action === 'add_tag' && value === false) {
          throw new Error(t('ruleEditor.errors.actionConflictsTag', { action: `${spec.action}(false)`, tag: spec.tag }))
        } else if (existing.action === 'remove_tag' && value === true) {
          throw new Error(t('ruleEditor.errors.actionConflictsTagRemoval', { action: `${spec.action}(true)`, tag: spec.tag }))
        }
      }
      modifierState.set(spec.action, value === true)
      out.push(action)
      continue
    }

    const tagName = tagNameForAction(action)
    if ((action.action === 'add_tag' || action.action === 'remove_tag') && tagName) {
      const matchingSpec = specs.find((s) => tagAppliesToModifier(s, tagName))
      if (matchingSpec) {
        const state = modifierState.get(matchingSpec.action)
        if (action.action === 'add_tag' && state === true) {
          continue
        }
        if (action.action === 'add_tag' && state === false) {
          throw new Error(t('ruleEditor.errors.actionConflictsTag', { action: `${matchingSpec.action}(false)`, tag: matchingSpec.tag }))
        }
        if (action.action === 'remove_tag' && state === true) {
          throw new Error(t('ruleEditor.errors.actionConflictsTagRemoval', { action: `${matchingSpec.action}(true)`, tag: matchingSpec.tag }))
        }
      }
    }

    out.push(action)
  }
  return out
}

function nonEmptyParam(params: Record<string, any> | undefined, key: string): boolean {
  const value = params?.[key]
  return typeof value === 'string' ? value.trim() !== '' : value !== undefined && value !== null
}

function validateRuleActionForSubmit(action: RuleAction, t: TFunction): string | null {
  switch (action.action) {
    case 'set_gear':
      return nonEmptyParam(action.params, 'gear_id') ? null : t('ruleEditor.validation.chooseGear')
    case 'add_tag':
    case 'remove_tag':
      return nonEmptyParam(action.params, 'tag_id') ? null : t('ruleEditor.validation.chooseTag')
    case 'set_visibility':
      return nonEmptyParam(action.params, 'visibility') ? null : t('ruleEditor.validation.chooseVisibility')
    case 'set_sport_type':
      return nonEmptyParam(action.params, 'sport_type') ? null : t('ruleEditor.validation.chooseSport')
    case 'set_name':
      return nonEmptyParam(action.params, 'template') ? null : t('ruleEditor.validation.enterTitleTemplate')
    case 'set_intensity':
      return nonEmptyParam(action.params, 'intensity') ? null : t('ruleEditor.validation.chooseIntensity')
    case 'set_exclude_from_stats':
    case 'set_exclude_from_heatmap':
    case 'set_hide_map':
      return action.params?.value === true || action.params?.value === false ? null : t('ruleEditor.validation.chooseYesNo')
    default:
      return null
  }
}

function validateRuleActionsForSubmit(actions: RuleAction[], t: TFunction, language: SupportedLanguage, surface = t('ruleEditor.validation.actionSurface')) {
  for (let i = 0; i < actions.length; i++) {
    const action = actions[i]
    const missing = validateRuleActionForSubmit(action, t)
    if (!missing) continue
    const label = actionLabelByValue(action.action, t, language)
    throw new Error(t('ruleEditor.validation.incompleteAction', { surface, label, missing }))
  }
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
  const { t, i18n } = useTranslation()
  const language = currentLanguage(i18n.language)
  const seed = rule ?? initialData
  const [name, setName] = useState(seed?.name ?? '')
  const [priority, setPriority] = useState(seed?.priority ?? 0)
  const [triggerEvent, setTriggerEvent] = useState<'on_import' | 'on_enrichment_complete' | 'on_manual_apply' | 'on_gear_usage_update'>(seed?.trigger_event ?? 'on_import')
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
    ifBranch ? ifBranch.condition_groups : normaliseGroups(seed as AutomationRule | null)
  )
  const [actions, setActions] = useState<RuleAction[]>(() =>
    ifBranch ? denormaliseActions(ifBranch.actions) : denormaliseActions(seed?.actions ?? [])
  )
  const [branches, setBranches] = useState<RuleBranch[]>(elseIfBranches)
  const [actionError, setActionError] = useState<string | null>(null)

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

  /** Before saving, translate frontend action types back to canonical JSON. */
  function normaliseActions(raw: RuleAction[]): RuleAction[] {
    const tagNameByID = new Map(tags.map((tag) => [tag.id, tag.name]))
    function tagNameForAction(a: RuleAction): string | undefined {
      const tagID = a.params?.tag_id
      if (!tagID) return undefined
      const explicit = tagNameByID.get(String(tagID))
      if (explicit) return explicit
      const lowered = String(tagID).toLowerCase()
      if (lowered.includes('ebike') || lowered.includes('e-bike')) return 'E-Bike'
      if (lowered.includes('indoor')) return 'Indoor'
      if (lowered.includes('virtual') || lowered.includes('virtuell')) return 'virtuell'
      if (lowered.includes('commute') || lowered.includes('pendel')) return 'Pendeln'
      return undefined
    }
    const normalized: RuleAction[] = []
    for (const a of raw) {
      const fieldName = SET_FIELD_ACTION_TO_FIELD[a.action]
      if (fieldName) {
        normalized.push({ action: 'set_field', params: { field: fieldName, value: a.params.value ?? false } })
        continue
      }
      if (a.action === 'set_sport_type') {
        const tuple = normalizeSportTuple(String(a.params?.sport_type ?? ''), a.params?.sub_type == null ? null : String(a.params.sub_type))
        const subType = tuple.subType && tuple.subType !== 'generic' ? tuple.subType : ''
        normalized.push({
          action: 'set_sport_type',
          params: {
            sport_type: tuple.sportType,
            sub_type: subType,
          },
        })
        const controls = ruleModifierControlsForTuple(tuple.sportType, subType)
        for (const control of controls) {
          if (!control.actionApplicable) continue
          const key = control.key as RuleTupleModifierKey
          const paramKey = ACTION_MODIFIER_PARAM_BY_KEY[key]
          const value = a.params?.[paramKey] ?? tupleDefaultModifier(tuple.sportType, subType, key)
          if (value === undefined) continue
          normalized.push({ action: MODIFIER_ACTION_SPECS.find((spec) => spec.param === paramKey)!.action, params: { [paramKey]: value === true } })
        }
        continue
      }
      normalized.push(a)
    }
    return dedupeSystemTagActions(normalized, tagNameForAction, t)
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim() || totalConditions === 0 || actions.length === 0) return
    setActionError(null)
    let normalizedActions: RuleAction[]
    let normalizedBranches: RuleBranch[] | undefined
    try {
      validateRuleActionsForSubmit(actions, t, language)
      normalizedActions = normaliseActions(actions)
      const normalizedGroups = groups.map(normaliseSportTupleConditionGroup)
      validateConditionGroupsForSubmit(normalizedGroups, t)
      const duplicateGroup = findDuplicateSportTupleGroup(normalizedGroups)
      if (duplicateGroup !== null) {
        throw new Error(t('ruleEditor.validation.duplicateSportSelection', { index: duplicateGroup + 1 }))
      }
      normalizedBranches = branches.length > 0 ? [
        { type: 'if' as const, condition_groups: normalizedGroups, actions: normalizedActions },
        ...branches.map(b => ({
          ...b,
          condition_groups: b.condition_groups.map(normaliseSportTupleConditionGroup),
          actions: (() => {
            validateRuleActionsForSubmit(b.actions, t, language, t('ruleEditor.validation.branchActionSurface'))
            return normaliseActions(b.actions)
          })(),
        })),
      ] : undefined
      const flat = flattenGroups(normalizedGroups)
      onSave({
        name: name.trim(),
        priority,
        trigger_event: triggerEvent,
        stop_on_match: stopOnMatch,
        condition_groups: normalizedGroups,
        // Also send flat for backward compatibility:
        conditions: flat.conditions,
        conditions_logic: flat.conditions_logic,
        actions: normalizedActions,
        // When branches exist, reconstruct: main groups/actions become the 'if' branch
        branches: normalizedBranches,
      })
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t('ruleEditor.errors.actionsConflict'))
      return
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {/* Name & Priority */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="sm:col-span-2">
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">{t('ruleEditor.nameRequiredLabel')}</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            className={inputClass + ' w-full'}
            placeholder={t('ruleEditor.namePlaceholder')}
          />
        </div>
        <div>
          <label className="flex items-center gap-1.5 text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            {t('ruleEditor.priority')}
            <HelpHint text={t('ruleEditor.priorityHelp')} />
          </label>
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
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">{t('ruleEditor.trigger')}</label>
          <select
            value={triggerEvent}
            onChange={e => setTriggerEvent(e.target.value as typeof triggerEvent)}
            className={inputClass + ' w-full'}
          >
            <option value="on_import">{t('ruleEditor.triggers.onImport')}</option>
            <option value="on_enrichment_complete">{t('ruleEditor.triggers.onEnrichmentComplete')}</option>
            <option value="on_manual_apply">{t('ruleEditor.triggers.onManualApply')}</option>
            <option value="on_gear_usage_update">{t('ruleEditor.triggers.onGearUsageUpdate')}</option>
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
            {t('ruleEditor.stopFurtherRules')}
          </label>
        </div>
      </div>

      {/* Condition groups */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <div>
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('ruleEditor.conditionsRequired')}</h3>
            {groups.length > 1 && (
              <p className="text-xs text-neutral-400 mt-0.5">{t('ruleEditor.groupsAndHint')}</p>
            )}
          </div>
          <button
            type="button"
            onClick={addGroup}
            className="text-xs font-medium px-2.5 py-1 rounded border border-accent-DEFAULT/50 dark:border-orange-400/50 text-accent-600 dark:text-orange-400 hover:bg-accent-DEFAULT/10 dark:hover:bg-orange-400/10 transition-colors cursor-pointer"
          >
            {t('ruleEditor.addGroup')}
          </button>
        </div>
        {totalConditions === 0 && (
          <p className="text-sm text-neutral-400 py-2">{t('ruleEditor.minConditionRequired')}</p>
        )}
        <div className="space-y-3">
          {groups.map((g, i) => (
            <div key={i}>
              {i > 0 && (
                <div className="flex items-center gap-2 my-2">
                  <div className="flex-1 border-t border-dashed border-neutral-300 dark:border-neutral-700" />
                  <span className="text-xs font-semibold text-neutral-500 uppercase tracking-wide px-2">{t('ruleEditor.logic.and')}</span>
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
                gearList={gearList ?? []}
                recorders={recorders}
                sensors={sensors}
              />
            </div>
          ))}
        </div>


      </div>

      {/* Actions */}
      <div>
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">{t('ruleEditor.actionsRequired')}</h3>
        <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg p-3 space-y-2 bg-white dark:bg-neutral-900/30">
        {actions.length === 0 && (
          <p className="text-sm text-neutral-400 py-2">{t('ruleEditor.minActionRequired')}</p>
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
                  <option key={a.value} value={a.value}>{actionTypeLabel(a, t, language)}</option>
                ))}
              </select>
              <ActionParams
                action={act.action}
                params={act.params}
                gearList={gearList ?? []}
                tags={tags}
                onChange={(params) => updateAction(idx, act.action, params)}
                t={t}
                language={language}
              />
              <button
                type="button"
                onClick={() => removeAction(idx)}
                className="text-neutral-400 hover:text-rose-500 transition-colors cursor-pointer"
                title={t('ruleEditor.removeAction')}
              >
                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                </svg>
              </button>
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={addAction}
          className="text-xs font-medium px-2.5 py-1 rounded border border-accent-DEFAULT/50 dark:border-orange-400/50 text-accent-600 dark:text-orange-400 hover:bg-accent-DEFAULT/10 dark:hover:bg-orange-400/10 transition-colors cursor-pointer"
        >
          {t('ruleEditor.addAction')}
        </button>
        </div>
      </div>

      {/* Branches (ELSE-IF / ELSE) */}
      {branches.map((branch, branchIdx) => (
        <div key={branchIdx} className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-3">
          {/* Branch header */}
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
              {branch.type === 'else' ? t('ruleEditor.branches.else') : t('ruleEditor.branches.elseIf')}
            </span>
            <button
              type="button"
              onClick={() => removeBranch(branchIdx)}
              className="ml-auto text-xs text-rose-500 hover:text-rose-700 dark:hover:text-rose-400 cursor-pointer"
            >
              {t('ruleEditor.branches.removeBranch')}
            </button>
          </div>

          {/* Conditions for else_if branches */}
          {branch.type === 'else_if' && (
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">{t('ruleEditor.conditions')}</span>
                <button
                  type="button"
                  onClick={() => addBranchGroup(branchIdx)}
                  className="text-xs font-medium px-2.5 py-1 rounded border border-accent-DEFAULT/50 dark:border-orange-400/50 text-accent-600 dark:text-orange-400 hover:bg-accent-DEFAULT/10 dark:hover:bg-orange-400/10 transition-colors cursor-pointer"
                >
                  {t('ruleEditor.addGroup')}
                </button>
              </div>
              {branch.condition_groups.length === 0 && (
                <p className="text-xs text-neutral-400 italic">{t('ruleEditor.noBranchConditions')}</p>
              )}
              <div className="space-y-2">
                {branch.condition_groups.map((g, gi) => (
                  <div key={gi}>
                    {gi > 0 && (
                      <div className="flex items-center gap-2 my-1">
                        <div className="flex-1 border-t border-dashed border-neutral-300 dark:border-neutral-700" />
                        <span className="text-xs font-semibold text-neutral-500 uppercase tracking-wide px-2">{t('ruleEditor.logic.and')}</span>
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
                      gearList={gearList ?? []}
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
            <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">{t('ruleEditor.branches.then')}</span>
            {branch.actions.length === 0 && (
              <p className="text-xs text-neutral-400 italic">{t('ruleEditor.noActions')}</p>
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
                      <option key={a.value} value={a.value}>{actionTypeLabel(a, t, language)}</option>
                    ))}
                  </select>
                  <ActionParams
                    action={act.action}
                    params={act.params}
                    gearList={gearList ?? []}
                    tags={tags}
                    onChange={(params) => updateBranchAction(branchIdx, actionIdx, act.action, params)}
                    t={t}
                    language={language}
                  />
                  <button
                    type="button"
                    onClick={() => removeBranchAction(branchIdx, actionIdx)}
                    className="text-neutral-400 hover:text-rose-500 transition-colors cursor-pointer"
                    title={t('ruleEditor.removeAction')}
                  >
                    <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                      <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                    </svg>
                  </button>
                </div>
              ))}
            </div>
            <button
              type="button"
              onClick={() => addBranchAction(branchIdx)}
              className="mt-1 text-xs font-medium px-2.5 py-1 rounded border border-accent-DEFAULT/50 dark:border-orange-400/50 text-accent-600 dark:text-orange-400 hover:bg-accent-DEFAULT/10 dark:hover:bg-orange-400/10 transition-colors cursor-pointer"
            >
              {t('ruleEditor.addAction')}
            </button>
          </div>
        </div>
      ))}

      {/* Add-branch buttons */}
      <div className="flex gap-2">
        <button
          type="button"
          onClick={() => addBranch('else_if')}
          className="text-xs font-medium px-2.5 py-1 rounded border border-accent-DEFAULT/50 dark:border-orange-400/50 text-accent-600 dark:text-orange-400 hover:bg-accent-DEFAULT/10 dark:hover:bg-orange-400/10 transition-colors cursor-pointer"
        >
          {t('ruleEditor.addElseIf')}
        </button>
        {!branches.some(b => b.type === 'else') && (
          <button
            type="button"
            onClick={() => addBranch('else')}
            className="text-xs font-medium px-2.5 py-1 rounded border border-accent-DEFAULT/50 dark:border-orange-400/50 text-accent-600 dark:text-orange-400 hover:bg-accent-DEFAULT/10 dark:hover:bg-orange-400/10 transition-colors cursor-pointer"
          >
            {t('ruleEditor.addElse')}
          </button>
        )}
      </div>

      {/* Submit */}
      <div className="flex flex-col items-end gap-1 pt-2">
        {(!name.trim() || totalConditions === 0 || actions.length === 0) && (
          <p className="text-xs text-amber-600 dark:text-amber-400">
            {!name.trim() ? t('ruleEditor.nameRequired') : totalConditions === 0 ? t('ruleEditor.minConditionRequired') : t('ruleEditor.minActionRequired')}
          </p>
        )}
        {actionError && (
          <p role="alert" className="text-xs text-rose-600 dark:text-rose-400">
            {actionError}
          </p>
        )}
        <div className="flex gap-3">
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-2 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
          >
            {t('common.cancel')}
          </button>
          <button
            type="submit"
            disabled={isSaving || !name.trim() || totalConditions === 0 || actions.length === 0}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
          >
            {isSaving ? t('ruleEditor.saving') : t('account.actions.save')}
          </button>
        </div>
      </div>
    </form>
  )
}
