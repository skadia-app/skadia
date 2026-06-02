// frontend/src/lib/colorMetricLabels.ts
import { i18n } from '@/i18n/i18n'

const METRIC_LABEL_KEYS = {
  sport:          'colorMetricLabels.metrics.sport',
  hr:             'colorMetricLabels.metrics.hr',
  speed:          'colorMetricLabels.metrics.speed',
  pace:           'colorMetricLabels.metrics.pace',
  altitude:       'colorMetricLabels.metrics.altitude',
  cadence_foot:   'colorMetricLabels.metrics.cadenceFoot',
  cadence_bike:   'colorMetricLabels.metrics.cadenceBike',
  cadence_paddle: 'colorMetricLabels.metrics.cadencePaddle',
  power:          'colorMetricLabels.metrics.power',
  temperature:    'colorMetricLabels.metrics.temperature',
} as const

const MODE_LABEL_KEYS = {
  zones:  'colorMetricLabels.modes.zones',
  linear: 'colorMetricLabels.modes.linear',
} as const

const DROPDOWN_LABEL_KEYS = {
  chooseColoring:     'colorMetricLabels.dropdown.chooseColoring',
  triggerDefault:     'colorMetricLabels.dropdown.triggerDefault',
  zonesBasedTooltip:  'colorMetricLabels.dropdown.zonesBasedTooltip',
  zonesNotConfigured: 'colorMetricLabels.dropdown.zonesNotConfigured',
} as const

function translatedRecord<T extends Record<string, string>>(keys: T): { readonly [K in keyof T]: string } {
  return new Proxy(keys, {
    get(target, prop: string) {
      const key = target[prop]
      return key ? i18n.t(key) : undefined
    },
  }) as { readonly [K in keyof T]: string }
}

export const METRIC_LABELS = translatedRecord(METRIC_LABEL_KEYS)
export const MODE_LABELS = translatedRecord(MODE_LABEL_KEYS)
export const DROPDOWN_LABELS = translatedRecord(DROPDOWN_LABEL_KEYS)

export type MetricKey = keyof typeof METRIC_LABEL_KEYS
