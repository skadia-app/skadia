import type { TFunction } from 'i18next'
import type { MapLayer } from '@/api/mapLayers'

type TagLike = {
  name: string
  display_key?: string | null
}

const SYSTEM_TAG_KEYS: Record<string, string> = {
  Rundtour: 'tags.system.round_trip.label',
  'E-Bike': 'tags.system.is_electric.label',
  Pendeln: 'tags.system.is_commute.label',
  Indoor: 'tags.system.is_indoor.label',
  virtuell: 'tags.system.is_virtual.label',
  Virtuell: 'tags.system.is_virtual.label',
  Frost: 'tags.system.frost.label',
  frost: 'tags.system.frost.label',
  bewölkt: 'tags.system.weather_cloudy.label',
  'Bewölkt': 'tags.system.weather_cloudy.label',
  sonnig: 'tags.system.weather_sunny.label',
  Sonnig: 'tags.system.weather_sunny.label',
  Regen: 'tags.system.weather_rain.label',
  regen: 'tags.system.weather_rain.label',
  Schnee: 'tags.system.weather_snow.label',
  schnee: 'tags.system.weather_snow.label',
  Nebel: 'tags.system.weather_fog.label',
  nebel: 'tags.system.weather_fog.label',
  Gewitter: 'tags.system.weather_thunderstorm.label',
  gewitter: 'tags.system.weather_thunderstorm.label',
  Nacht: 'tags.system.nacht.label',
  Lift: 'tags.system.lift.label',
  Ultramarathon: 'tags.system.ultra_marathon.label',
  Marathon: 'tags.system.marathon.label',
  Halbmarathon: 'tags.system.half_marathon.label',
  Intervalltraining: 'tags.system.interval_training.label',
  'Intervall-Training': 'tags.system.interval_training.label',
  'Mit Einkehr': 'tags.system.food_stop.label',
  Grenzquerung: 'tags.system.border_crossing.label',
  Laufdistanz: 'tags.system.running_distance.label',
}

const SERVICE_INTERVAL_KEYS: Record<string, string> = {
  Tauschen: 'gearPages.service.intervalNames.swap',
  Wechseln: 'gearPages.service.intervalNames.swap',
  Pruefen: 'gearPages.service.intervalNames.check',
  Prüfen: 'gearPages.service.intervalNames.check',
}

function translatedOrFallback(t: TFunction, key: string | null | undefined, fallback: string): string {
  if (!key) return fallback
  const value = t(key)
  return value === key ? fallback : value
}

export function displaySystemTagName(name: string, t: TFunction): string {
  return translatedOrFallback(t, SYSTEM_TAG_KEYS[name], name)
}

export function displayTagName(tag: TagLike, t: TFunction): string {
  return translatedOrFallback(t, tag.display_key, displaySystemTagName(tag.name, t))
}

export function displayAutomationRuleName(
  rule: { name: string; is_system?: boolean; system_type?: string | null },
  t: TFunction,
): string {
  if (rule.is_system && rule.system_type) {
    return translatedOrFallback(t, `automation.system_rules.${rule.system_type}.label`, rule.name)
  }
  return displaySystemTagName(rule.name, t)
}

export function displayServiceIntervalName(name: string, t: TFunction): string {
  return translatedOrFallback(t, SERVICE_INTERVAL_KEYS[name], name)
}

export function displayMapLayerName(layer: MapLayer, t: TFunction): string {
  if (!layer.is_system) return layer.name
  return translatedOrFallback(t, `map.baseLayers.${layer.key}`, layer.name)
}
