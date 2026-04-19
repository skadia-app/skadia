// frontend/src/lib/colorMetricLabels.ts
/**
 * Centralized UI strings for the map metric colorization feature.
 * German-only for V1 per frontend/CLAUDE.md §i18n-Vorbereitung.
 * Extraction point for future i18n sprint.
 */

export const METRIC_LABELS = {
  sport:          'Standard',
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

export const DROPDOWN_LABELS = {
  chooseColoring:     'Einfärbung wählen',
  triggerDefault:     'Einfärbung',
  zonesBasedTooltip:  'Zonen-basierte Einfärbung',
  zonesNotConfigured: 'Keine Zonen konfiguriert',
} as const

export type MetricKey = keyof typeof METRIC_LABELS
