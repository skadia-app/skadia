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
    default: return null  // kein Badge für Recorder oder unbekannt
  }
}

export function deviceTypeBadgeLabel(type: string): string {
  const labels: Record<string, string> = {
    // ANT+ device types (wie in activity_sensors.all_device_types gespeichert)
    heart_rate: 'Herzfrequenz',
    bike_power: 'Leistungsmesser',
    bike_speed: 'Geschwindigkeit',
    bike_cadence: 'Trittfrequenz',
    bike_speed_cadence: 'Geschw./Trittfrequenz',
    combined_speed_cadence: 'Geschw./Trittfrequenz',
    stride_speed_distance: 'Laufpod',
    env_sensor: 'Umgebungssensor',
    environment_sensor_legacy: 'Temperatursensor',
    bike_radar: 'Rückwärtsradar',
    shifting: 'Schaltung',
    muscle_oxygen: 'Muskel-O2',
    fitness_equipment: 'Fitnessgerät',
    multi_sport_speed_distance: 'Multi-Sport',
    activity_class: 'Aktivitätsklasse',
    blood_pressure: 'Blutdruck',
    // BLE device types
    connected_gps: 'GPS',
    gps: 'GPS',
    footpod: 'Laufpod',
    bike_trainer: 'Rollentrainer',
    lights: 'Beleuchtung',
    running_dynamics: 'Laufdynamik',
    barometric_altimeter: 'Barometer',
    temperature: 'Temperatur',
    // Weitere Sensortypen
    pulse_oximetry: 'Blutsauerstoff',
    spo2: 'Blutsauerstoff',
    // Fallback-Aliase
    power: 'Leistung',
    speed: 'Geschwindigkeit',
    cadence: 'Trittfrequenz',
  }
  return labels[type] ?? type
}

/** Filtert rohe ManufacturerInvalid/GarminProductInvalid Strings aus Anzeige */
export function cleanDisplayName(name?: string | null): string | null {
  if (!name) return null
  if (name.includes('Invalid') || name.includes('Manufacturer(')) return null
  return name
}
