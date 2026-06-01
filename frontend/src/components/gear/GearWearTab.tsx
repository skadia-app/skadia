import { Link } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { listServiceEvents, listServiceIntervals } from '@/api/gear'
import type { Gear, GearComponent, GearServiceEvent, GearServiceInterval } from '@/api/types'
import { formatDuration } from '@/lib/utils'
import { formatUsageMetricValue } from '@/lib/gearUsageMetrics'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import type { UnitFormatOptions } from '@/lib/units'

interface GearWearTabProps {
  item: Gear
}

export default function GearWearTab({ item }: GearWearTabProps) {
  const { t, i18n } = useTranslation()
  const unitFormatters = useUnitFormatters()
  const components = item.components ?? []
  const intervalQueries = useQueries({
    queries: components.map(component => ({
      queryKey: ['component-service-intervals', component.id],
      queryFn: () => listServiceIntervals(component.id),
      staleTime: 60_000,
    })),
  })
  const eventQueries = useQueries({
    queries: components.map(component => ({
      queryKey: ['component-services', component.id],
      queryFn: () => listServiceEvents(component.id),
      staleTime: 60_000,
    })),
  })

  const intervalRows = components.flatMap((component, index) => {
    const intervals = (intervalQueries[index]?.data ?? []) as GearServiceInterval[]
    return intervals.map(interval => {
      const currentValue = intervalCurrentValue(component, interval)
      const ratio = interval.interval_value > 0 ? currentValue / interval.interval_value : 0
      return { component, interval, currentValue, ratio }
    })
  }).sort((a, b) => b.ratio - a.ratio)

  const recentEvents = components.flatMap((component, index) => {
    const events = (eventQueries[index]?.data ?? []) as GearServiceEvent[]
    return events.map(event => ({ component, event }))
  }).sort((a, b) => Date.parse(b.event.performed_at) - Date.parse(a.event.performed_at)).slice(0, 8)

  const intervalsLoading = intervalQueries.some(query => query.isLoading)
  const eventsLoading = eventQueries.some(query => query.isLoading)

  return (
    <div className="space-y-6">
      {/* Usage aggregates: ski, downhill-MTB, and touring ski gear care about
          descent volume, so descent sits next to ascent. */}
      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">
          {t('gearPages.wear.totalUsage')}
        </h3>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
          <WearCard label={t('gearPages.metrics.distance')} value={unitFormatters.formatDistance(item.total_distance_m)} />
          <WearCard
            label={t('gearPages.metrics.elevationGain')}
            value={unitFormatters.formatElevationGain(item.total_elevation_gain_m) ?? '—'}
          />
          <WearCard
            label={t('gearPages.metrics.elevationLoss')}
            value={unitFormatters.formatElevationLoss(item.total_elevation_loss_m) ?? '—'}
          />
          <WearCard label={t('gearPages.metrics.time')} value={formatDuration(item.total_time_s)} />
          <WearCard label={t('gearPages.metrics.activities')} value={String(item.total_activities)} />
        </div>
      </section>

      <ServiceIntervalOverview itemId={item.id} rows={intervalRows} isLoading={intervalsLoading} unitOptions={unitFormatters.options} />
      <RecentServiceEvents itemId={item.id} rows={recentEvents} isLoading={eventsLoading} locale={i18n.language} />
    </div>
  )
}

function ServiceIntervalOverview({
  itemId,
  rows,
  isLoading,
  unitOptions,
}: {
  itemId: string
  rows: Array<{
    component: GearComponent
    interval: GearServiceInterval
    currentValue: number
    ratio: number
  }>
  isLoading: boolean
  unitOptions: UnitFormatOptions
}) {
  const { t } = useTranslation()
  return (
    <section
      data-testid="service-interval-overview"
      className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 space-y-3"
    >
      <div>
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
          {t('gearPages.wear.activeIntervals')}
        </h3>
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          {t('gearPages.wear.intervalDescription')}
        </p>
      </div>

      {isLoading && <p className="text-sm text-neutral-400">{t('gearPages.wear.loadingIntervals')}</p>}

      {!isLoading && rows.length === 0 && (
        <p className="text-sm text-neutral-400">{t('gearPages.wear.noIntervals')}</p>
      )}

      {rows.length > 0 && (
        <div className="space-y-2">
          {rows.map(row => {
            const percent = Math.max(0, Math.round(row.ratio * 100))
            const barWidth = `${Math.min(100, percent)}%`
            return (
              <Link
                key={row.interval.id}
                to={`/gear/${itemId}?tab=components&component=${row.component.id}`}
                className="block rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 hover:bg-neutral-50 dark:hover:bg-neutral-900"
              >
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                      {row.component.name} - {row.interval.name}
                    </div>
                    <div className="text-xs text-neutral-500 dark:text-neutral-400">
                      {formatUsageMetricValue(row.interval.metric_key, row.currentValue, unitOptions)} / {formatUsageMetricValue(row.interval.metric_key, row.interval.interval_value, unitOptions)}
                    </div>
                  </div>
                  <span className={`rounded px-2 py-0.5 text-xs font-medium ${row.ratio >= 1 ? 'bg-rose-100 text-rose-700 dark:bg-rose-950/40 dark:text-rose-300' : 'bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'}`}>
                    {percent}%
                  </span>
                </div>
                <div className="mt-2 h-1.5 rounded bg-neutral-100 dark:bg-neutral-800 overflow-hidden">
                  <div
                    className={row.ratio >= 1 ? 'h-full bg-rose-500' : 'h-full bg-amber-500'}
                    style={{ width: barWidth }}
                  />
                </div>
              </Link>
            )
          })}
        </div>
      )}
    </section>
  )
}

function RecentServiceEvents({
  itemId,
  rows,
  isLoading,
  locale,
}: {
  itemId: string
  rows: Array<{ component: GearComponent; event: GearServiceEvent }>
  isLoading: boolean
  locale: string
}) {
  const { t } = useTranslation()
  return (
    <section
      data-testid="service-events-overview"
      className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 space-y-3"
    >
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
        {t('gearPages.wear.recentServiceEvents')}
      </h3>

      {isLoading && <p className="text-sm text-neutral-400">{t('gearPages.wear.loadingServiceEvents')}</p>}

      {!isLoading && rows.length === 0 && (
        <p className="text-sm text-neutral-400">{t('gearPages.wear.noServiceEvents')}</p>
      )}

      {rows.length > 0 && (
        <div className="divide-y divide-neutral-100 dark:divide-neutral-800">
          {rows.map(({ component, event }) => (
            <Link
              key={event.id}
              to={`/gear/${itemId}?tab=components&component=${component.id}`}
              className="flex items-center justify-between gap-3 py-2 text-sm hover:bg-neutral-50 dark:hover:bg-neutral-900"
            >
              <div>
                <div className="font-medium text-neutral-800 dark:text-neutral-100">
                  {component.name}
                </div>
                <div className="text-xs text-neutral-500 dark:text-neutral-400">
                  {event.description || t('gearPages.wear.serviceRecorded')}
                  {event.cost != null && ` - ${event.cost.toFixed(2)} EUR`}
                </div>
              </div>
              <span className="text-xs text-neutral-500 dark:text-neutral-400 whitespace-nowrap">
                {formatDateShort(event.performed_at, locale)}
              </span>
            </Link>
          ))}
        </div>
      )}
    </section>
  )
}

function intervalCurrentValue(component: GearComponent, interval: GearServiceInterval): number {
  const metric = interval.metric_key || interval.interval_type
  const lastValue = interval.last_service_metric_value ?? (metric === 'distance_m' ? interval.last_service_distance_m ?? 0 : 0)
  switch (metric) {
    case 'distance_m':
      return Math.max(0, component.total_distance_m - lastValue)
    case 'moving_time_s':
      return Math.max(0, component.total_moving_time_s - lastValue)
    case 'elapsed_time_s':
      return Math.max(0, component.total_time_s - lastValue)
    case 'activity_count':
      return Math.max(0, component.total_activities - lastValue)
    case 'elapsed_days':
      return elapsedDaysSince(interval.last_service_at ?? component.installed_at)
    case 'elevation_gain_m':
      return Math.max(0, component.total_elevation_gain_m - lastValue)
    case 'elevation_loss_m':
      return Math.max(0, component.total_elevation_loss_m - lastValue)
    default:
      return 0
  }
}

function elapsedDaysSince(value?: string | null): number {
  if (!value) return 0
  const timestamp = Date.parse(value)
  if (!Number.isFinite(timestamp)) return 0
  return Math.max(0, (Date.now() - timestamp) / 86_400_000)
}

function formatDateShort(dateStr: string, locale: string): string {
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime())) return dateStr
  return date.toLocaleDateString(locale || undefined, { day: '2-digit', month: '2-digit', year: 'numeric' })
}

function WearCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
      <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">{label}</div>
      <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
        {value}
      </div>
    </div>
  )
}
