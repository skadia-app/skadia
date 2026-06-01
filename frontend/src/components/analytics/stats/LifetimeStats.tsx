import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { fetchLifetimeStats } from '@/api/analytics'
import type { LifetimeStats } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { fmtDuration } from '@/lib/format'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

function fmtNum(n: number, locale: string): string {
  return new Intl.NumberFormat(locale).format(Math.round(n))
}

function fmtDecimal(n: number, locale: string, digits: number): string {
  return new Intl.NumberFormat(locale, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(n)
}

interface StatCardProps {
  label: string
  value: string
  unit?: string
  accent?: boolean
}

function StatCard({ label, value, unit, accent }: StatCardProps) {
  return (
    <div className={`rounded-xl p-4 border ${accent
      ? 'border-[var(--color-accent-DEFAULT)] bg-[var(--color-accent-DEFAULT)]/5 dark:bg-[var(--color-accent-DEFAULT)]/10'
      : 'border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900'
    }`}>
      <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1 font-medium">{label}</div>
      <div className={`text-2xl font-bold tabular-nums leading-tight ${accent ? 'text-[var(--color-accent-DEFAULT)]' : 'text-neutral-900 dark:text-neutral-100'}`}>
        {value}
        {unit && <span className="text-base font-normal text-neutral-500 dark:text-neutral-400 ml-1">{unit}</span>}
      </div>
    </div>
  )
}

interface FunFactProps {
  emoji: string
  text: string
}

function FunFact({ emoji, text }: FunFactProps) {
  return (
    <div className="flex items-start gap-3 p-3 rounded-lg bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-100 dark:border-neutral-800">
      <span className="text-2xl leading-none mt-0.5">{emoji}</span>
      <span className="text-sm text-neutral-700 dark:text-neutral-300">{text}</span>
    </div>
  )
}

interface StreakBadgeProps {
  label: string
  count: number
  isRecord?: boolean
}

function StreakBadge({ label, count, isRecord }: StreakBadgeProps) {
  const { t } = useTranslation()
  const { options } = useUnitFormatters()
  const isFire = count >= 7
  return (
    <div className={`flex items-center gap-2 px-4 py-2.5 rounded-full border font-medium text-sm ${
      isRecord
        ? 'bg-amber-50 dark:bg-amber-900/20 border-amber-300 dark:border-amber-700 text-amber-800 dark:text-amber-300'
        : isFire
          ? 'bg-orange-50 dark:bg-orange-900/20 border-orange-300 dark:border-orange-700 text-orange-800 dark:text-orange-300'
          : 'bg-neutral-100 dark:bg-neutral-800 border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300'
    }`}>
      {isFire && <span>🔥</span>}
      <span>{label}</span>
      <span className="font-bold tabular-nums">{fmtNum(count, options.locale)}</span>
      <span className="text-xs opacity-70">{t('analyticsTrainingStats.units.day', { count })}</span>
    </div>
  )
}

function LoadingSkeleton() {
  return (
    <div className="space-y-6 animate-pulse">
      <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-3">
        {Array.from({ length: 7 }).map((_, i) => (
          <div key={i} className="rounded-xl p-4 border border-neutral-200 dark:border-neutral-800 bg-neutral-100 dark:bg-neutral-800 h-20" />
        ))}
      </div>
    </div>
  )
}

export default function LifetimeStats() {
  const { t } = useTranslation()
  const { options, formatDistance, formatElevation } = useUnitFormatters()
  const { toQueryString } = useAnalyticsFilters()
  const params = toQueryString()

  const { data, isLoading, isError } = useQuery<LifetimeStats>({
    queryKey: ['lifetime-stats', params],
    queryFn: () => fetchLifetimeStats(params),
    retry: 1,
    staleTime: 5 * 60 * 1000,
  })

  if (isLoading) return <LoadingSkeleton />

  if (isError || !data) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-400">
        {t('analyticsTrainingStats.lifetime.loadError')}
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Primary stats grid */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
        <StatCard
          label={t('analyticsTrainingStats.metrics.activities')}
          value={fmtNum(data.total_activities, options.locale)}
          accent
        />
        <StatCard
          label={t('analyticsTrainingStats.metrics.distance')}
          value={formatDistance(data.total_distance_m)}
        />
        <StatCard
          label={t('analyticsTrainingStats.metrics.elevationGainUp')}
          value={formatElevation(data.total_elevation_m)}
        />
        <StatCard
          label={t('analyticsTrainingStats.metrics.elevationLossDown')}
          value={formatElevation(data.total_elevation_loss_m)}
        />
        <StatCard
          label={t('analyticsTrainingStats.metrics.movingTime')}
          value={fmtDuration(data.total_moving_time_s, 'lifetime')}
        />
        <StatCard
          label={t('analyticsTrainingStats.metrics.activeDays')}
          value={fmtNum(data.active_days, options.locale)}
        />
        <StatCard
          label={t('analyticsTrainingStats.metrics.countries')}
          value={fmtNum(data.countries, options.locale)}
        />
      </div>

      {/* Fun facts */}
      {(() => {
        const earthLaps = data.total_distance_m / 40_075_000
        return (earthLaps >= 0.01 || data.moon_trips > 0 || data.everest_count > 0 || data.everest_count_down > 0) && (
          <div>
            <h3 className="text-sm font-semibold text-neutral-600 dark:text-neutral-400 mb-3 uppercase tracking-wide">
              {t('analyticsTrainingStats.lifetime.funFacts')}
            </h3>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              {earthLaps >= 0.01 && (
                <FunFact
                  emoji="🌍"
                  text={t('analyticsTrainingStats.lifetime.earthLaps', { count: fmtDecimal(earthLaps, options.locale, 3) })}
                />
              )}
              {data.moon_trips > 0 && (
                <FunFact
                  emoji="🌙"
                  text={t('analyticsTrainingStats.lifetime.moonTrips', { count: fmtDecimal(data.moon_trips, options.locale, 2) })}
                />
              )}
              {data.everest_count > 0 && (
                <FunFact
                  emoji="⛰️"
                  text={t('analyticsTrainingStats.lifetime.everestAscents', { count: fmtNum(data.everest_count, options.locale) })}
                />
              )}
              {data.everest_count_down > 0 && (
                <FunFact
                  emoji="⛰️"
                  text={t('analyticsTrainingStats.lifetime.everestDescents', { count: fmtNum(data.everest_count_down, options.locale) })}
                />
              )}
            </div>
          </div>
        )
      })()}

      {/* Streak badges */}
      {(data.current_day_streak > 0 || data.max_day_streak > 0) && (
        <div>
          <h3 className="text-sm font-semibold text-neutral-600 dark:text-neutral-400 mb-3 uppercase tracking-wide">
            {t('analyticsTrainingStats.streaks.title')}
          </h3>
          <div className="flex flex-wrap gap-2">
            {data.current_day_streak > 0 && (
              <StreakBadge label={t('analyticsTrainingStats.lifetime.currentStreak')} count={data.current_day_streak} />
            )}
            {data.max_day_streak > 0 && (
              <StreakBadge label={t('analyticsTrainingStats.lifetime.recordStreak')} count={data.max_day_streak} isRecord />
            )}
          </div>
        </div>
      )}

      {/* Lift stats — only shown when user has lift rides */}
      {data.lift_ride_count > 0 && (
        <div>
          <h3 className="text-sm font-semibold text-neutral-600 dark:text-neutral-400 mb-3 uppercase tracking-wide">
            {t('analyticsTrainingStats.lifetime.liftRidesLifetime')}
          </h3>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <StatCard label={t('analyticsTrainingStats.lifetime.liftRides')} value={fmtNum(data.lift_ride_count, options.locale)} />
            <StatCard label={t('analyticsTrainingStats.lifetime.liftUp')} value={formatElevation(data.lift_ascent_m)} />
            <StatCard label={t('analyticsTrainingStats.lifetime.liftDown')} value={formatElevation(data.lift_descent_m)} />
            <StatCard label={t('analyticsTrainingStats.lifetime.liftDistance')} value={formatDistance(data.lift_distance_m)} />
          </div>
        </div>
      )}
    </div>
  )
}
