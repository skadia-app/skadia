import { localeForLanguage } from '@/i18n/languages'
import { activeLanguage, localizeString, type LocalizedString } from '@/i18n/localizedString'
import {
  formatDistance as formatUnitDistance,
  formatElevation as formatUnitElevation,
  type UnitFormatOptions,
} from '@/lib/units'

export type UsageMetricKey =
  | 'distance_m'
  | 'moving_time_s'
  | 'elapsed_time_s'
  | 'activity_count'
  | 'elapsed_days'
  | 'elevation_gain_m'
  | 'elevation_loss_m'
  | 'net_ascent_m'
  | 'net_descent_m'
  | 'lift_ascent_m'
  | 'lift_descent_m'

export type ServiceMetricKey =
  | 'distance_m'
  | 'moving_time_s'
  | 'elapsed_time_s'
  | 'activity_count'
  | 'elapsed_days'
  | 'elevation_gain_m'
  | 'elevation_loss_m'

export const PRODUCTIVE_SERVICE_METRICS: ServiceMetricKey[] = [
  'distance_m',
  'moving_time_s',
  'elapsed_time_s',
  'activity_count',
  'elapsed_days',
  'elevation_gain_m',
  'elevation_loss_m',
]

const USAGE_METRIC_LABEL_MAP: Record<UsageMetricKey, LocalizedString> = {
  distance_m: {
    de: 'Distanz',
    en: 'Distance',
    fr: 'Distance',
    it: 'Distanza',
    es: 'Distancia',
  },
  moving_time_s: {
    de: 'Bewegungszeit',
    en: 'Moving time',
    fr: 'Temps en mouvement',
    it: 'Tempo in movimento',
    es: 'Tiempo en movimiento',
  },
  elapsed_time_s: {
    de: 'Gesamtzeit',
    en: 'Elapsed time',
    fr: 'Temps total',
    it: 'Tempo totale',
    es: 'Tiempo total',
  },
  activity_count: {
    de: 'Aktivitäten',
    en: 'Activities',
    fr: 'Activités',
    it: 'Attività',
    es: 'Actividades',
  },
  elapsed_days: {
    de: 'Tage seit Service',
    en: 'Days since service',
    fr: 'Jours depuis service',
    it: 'Giorni dal service',
    es: 'Días desde servicio',
  },
  elevation_gain_m: {
    de: 'Anstieg',
    en: 'Ascent',
    fr: 'Dénivelé positif',
    it: 'Dislivello positivo',
    es: 'Ascenso',
  },
  elevation_loss_m: {
    de: 'Tiefenmeter',
    en: 'Descent',
    fr: 'Dénivelé négatif',
    it: 'Dislivello negativo',
    es: 'Descenso',
  },
  net_ascent_m: {
    de: 'Netto-Anstieg',
    en: 'Net ascent',
    fr: 'Montée nette',
    it: 'Salita netta',
    es: 'Ascenso neto',
  },
  net_descent_m: {
    de: 'Netto-Abstieg',
    en: 'Net descent',
    fr: 'Descente nette',
    it: 'Discesa netta',
    es: 'Descenso neto',
  },
  lift_ascent_m: {
    de: 'Lift-Anstieg',
    en: 'Lift ascent',
    fr: 'Montée en remontée',
    it: 'Salita con impianto',
    es: 'Ascenso en remonte',
  },
  lift_descent_m: {
    de: 'Lift-Abstieg',
    en: 'Lift descent',
    fr: 'Descente en remontée',
    it: 'Discesa con impianto',
    es: 'Descenso en remonte',
  },
}

const USAGE_METRIC_KEYS = Object.keys(USAGE_METRIC_LABEL_MAP) as UsageMetricKey[]

const DAY_UNIT_LABEL: LocalizedString = {
  de: 'T',
  en: 'd',
  fr: 'j',
  it: 'g',
  es: 'd',
}

export function usageMetricLabel(metric: UsageMetricKey): string {
  return localizeString(USAGE_METRIC_LABEL_MAP[metric])
}

export const USAGE_METRIC_LABELS = Object.defineProperties(
  {} as Record<UsageMetricKey, string>,
  Object.fromEntries(
    USAGE_METRIC_KEYS.map(metric => [
      metric,
      {
        enumerable: true,
        get: () => usageMetricLabel(metric),
      },
    ]),
  ),
)

function formatNumber(value: number, options?: Intl.NumberFormatOptions): string {
  return new Intl.NumberFormat(localeForLanguage(activeLanguage()), options).format(value)
}

export function formatUsageMetricValue(
  metric: string,
  value: number | null | undefined,
  unitOptions?: UnitFormatOptions,
): string {
  if (value == null || Number.isNaN(value)) return '—'
  switch (metric) {
    case 'distance_m':
      if (unitOptions?.units === 'imperial') return formatUnitDistance(value, unitOptions)
      return `${formatNumber(value / 1000, {
        minimumFractionDigits: value >= 100000 ? 0 : 1,
        maximumFractionDigits: value >= 100000 ? 0 : 1,
      })} km`
    case 'moving_time_s':
    case 'elapsed_time_s':
      return `${formatNumber(Math.round(value / 3600))} h`
    case 'activity_count':
      return formatNumber(Math.round(value))
    case 'elapsed_days':
      return `${formatNumber(Math.round(value))} ${localizeString(DAY_UNIT_LABEL)}`
    case 'elevation_gain_m':
    case 'elevation_loss_m':
    case 'net_ascent_m':
    case 'net_descent_m':
    case 'lift_ascent_m':
    case 'lift_descent_m':
      if (unitOptions?.units === 'imperial') return formatUnitElevation(value, unitOptions)
      return `${formatNumber(Math.round(value))} m`
    default:
      return formatNumber(value)
  }
}
