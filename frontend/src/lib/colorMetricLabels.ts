// frontend/src/lib/colorMetricLabels.ts
/**
 * Centralized UI strings for the map metric colorization feature.
 * German-only for V1 per frontend/CLAUDE.md §i18n-Vorbereitung.
 * Extraction point for future i18n sprint.
 */

export const METRIC_LABELS = {
  sport:          'Sportart',
  hr:             'Herzfrequenz',
  speed:          'Geschwindigkeit',
  pace:           'Tempo',
  altitude:       'Höhe',
  cadence_foot:   'Schrittfrequenz',
  cadence_bike:   'Trittfrequenz',
  cadence_paddle: 'Paddelschläge',
  power:          'Leistung',
  temperature:    'Temperatur',
} as const

export const MODE_LABELS = {
  zones:  'Zonen',
  linear: 'Linear',
} as const

export const LEGEND_UNITS = {
  kmh:         'km/h',
  min_per_km:  'min/km',
  bpm:         'bpm',
  w:           'W',
  spm:         'spm',
  rpm:         'rpm',
  m:           'm',
  celsius:     '°C',
} as const

export const LEGEND_NOTES = {
  outOfRangeLow:  '< Min',
  outOfRangeHigh: '> Max',
} as const

export type MetricKey = keyof typeof METRIC_LABELS
