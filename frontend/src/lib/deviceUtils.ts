export function connectionTypeLabel(sourceType?: string): string {
  switch (sourceType) {
    case 'antplus': return 'ANT+'
    case 'bluetooth': return 'Bluetooth LE'
    default: return sourceType ?? 'Unbekannt'
  }
}

export function deviceTypeBadgeLabel(type: string): string {
  const labels: Record<string, string> = {
    heart_rate: 'Herzfrequenz',
    power: 'Leistung',
    speed: 'Geschwindigkeit',
    cadence: 'Trittfrequenz',
    combined_speed_cadence: 'Geschw. + Kadenz',
    gps: 'GPS',
    temperature: 'Temperatur',
    barometric_altimeter: 'Barometer',
    bike_radar: 'Radar',
    lights: 'Beleuchtung',
    muscle_oxygen: 'Muskel-O2',
    running_dynamics: 'Laufdynamik',
    activity_class: 'Aktivitätsklasse',
    blood_pressure: 'Blutdruck',
  }
  return labels[type] ?? type
}
