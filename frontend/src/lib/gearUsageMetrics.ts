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

export const USAGE_METRIC_LABELS: Record<UsageMetricKey, string> = {
  distance_m: 'Distanz',
  moving_time_s: 'Bewegungszeit',
  elapsed_time_s: 'Gesamtzeit',
  activity_count: 'Aktivitäten',
  elapsed_days: 'Tage seit Service',
  elevation_gain_m: 'Anstieg',
  elevation_loss_m: 'Tiefenmeter',
  net_ascent_m: 'Netto-Anstieg',
  net_descent_m: 'Netto-Abstieg',
  lift_ascent_m: 'Lift-Anstieg',
  lift_descent_m: 'Lift-Abstieg',
}

export function formatUsageMetricValue(metric: string, value: number | null | undefined): string {
  if (value == null || Number.isNaN(value)) return '—'
  switch (metric) {
    case 'distance_m':
      return `${(value / 1000).toFixed(value >= 100000 ? 0 : 1)} km`
    case 'moving_time_s':
    case 'elapsed_time_s':
      return `${Math.round(value / 3600)} h`
    case 'activity_count':
      return `${Math.round(value)}`
    case 'elapsed_days':
      return `${Math.round(value)} d`
    case 'elevation_gain_m':
    case 'elevation_loss_m':
    case 'net_ascent_m':
    case 'net_descent_m':
    case 'lift_ascent_m':
    case 'lift_descent_m':
      return `${Math.round(value)} m`
    default:
      return `${value}`
  }
}
