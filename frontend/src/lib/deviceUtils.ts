import type { TFunction } from 'i18next'

/** Compose a short device label from source_device / source / original_file_format fields. */
export function deviceLabel(device?: string | null, source?: string | null, format?: string | null): string | null {
  const devicePart = device || source || null
  const formatPart = format ? format.toUpperCase() : null
  if (!devicePart && !formatPart) return null
  if (devicePart && formatPart) return `${devicePart} · ${formatPart}`
  return devicePart ?? formatPart
}

export function connectionTypeLabel(sourceType?: string | null): string | null {
  switch (sourceType) {
    case 'antplus': return 'ANT+'
    case 'bluetooth': return 'Bluetooth LE'
    default: return null  // no badge for recorders or unknown values
  }
}

const knownBatteryStatuses = new Set([
  'new',
  'good',
  'ok',
  'low',
  'critical',
  'charging',
])

export function batteryStatusLabel(status: string, t: TFunction): string {
  return knownBatteryStatuses.has(status)
    ? t(`activityTables.sensors.batteryStatuses.${status}`)
    : status
}

const knownDeviceTypes = new Set([
  'heart_rate',
  'bike_power',
  'bike_speed',
  'bike_cadence',
  'bike_speed_cadence',
  'combined_speed_cadence',
  'stride_speed_distance',
  'env_sensor',
  'environment_sensor_legacy',
  'bike_radar',
  'shifting',
  'muscle_oxygen',
  'fitness_equipment',
  'multi_sport_speed_distance',
  'activity_class',
  'blood_pressure',
  'connected_gps',
  'gps',
  'footpod',
  'bike_trainer',
  'lights',
  'running_dynamics',
  'barometric_altimeter',
  'temperature',
  'pulse_oximetry',
  'spo2',
  'power',
  'speed',
  'cadence',
])

export function deviceTypeBadgeLabel(type: string, t?: TFunction): string {
  if (!t) return 'Unknown'
  return knownDeviceTypes.has(type)
    ? t(`devicePages.deviceTypes.${type}`)
    : t('devicePages.deviceTypes.unknown')
}

/** Filter raw ManufacturerInvalid/GarminProductInvalid strings out of display labels. */
export function cleanDisplayName(name?: string | null): string | null {
  if (!name) return null
  if (name.includes('Invalid') || name.includes('Manufacturer(')) return null
  return name
}
