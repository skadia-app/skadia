import { useQuery } from '@tanstack/react-query'
import { fetchLifetimeStats } from '@/api/analytics'
import type { LifetimeStats } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'

function fmtNum(n: number): string {
  return new Intl.NumberFormat('de-DE').format(Math.round(n))
}

function fmtDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  if (h >= 24 * 10) {
    const days = (seconds / 86400).toFixed(1)
    return `${days} Tage`
  }
  return `${fmtNum(h)} h`
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
      <span className="font-bold tabular-nums">{fmtNum(count)}</span>
      <span className="text-xs opacity-70">{count === 1 ? 'Tag' : 'Tage'}</span>
    </div>
  )
}

function LoadingSkeleton() {
  return (
    <div className="space-y-6 animate-pulse">
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="rounded-xl p-4 border border-neutral-200 dark:border-neutral-800 bg-neutral-100 dark:bg-neutral-800 h-20" />
        ))}
      </div>
    </div>
  )
}

export default function LifetimeStats() {
  const { toQueryString } = useAnalyticsFilters()
  const params = toQueryString()

  const { data, isLoading, isError } = useQuery<LifetimeStats>({
    queryKey: ['lifetime-stats', params],
    queryFn: () => fetchLifetimeStats(params),
    retry: 1,
  })

  if (isLoading) return <LoadingSkeleton />

  if (isError || !data) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-400">
        Fehler beim Laden der Lifetime-Statistiken
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Primary stats grid */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
        <StatCard
          label="Aktivitäten"
          value={fmtNum(data.total_activities)}
          accent
        />
        <StatCard
          label="Distanz"
          value={(data.total_distance_m / 1000).toFixed(0)}
          unit="km"
        />
        <StatCard
          label="Höhenmeter"
          value={fmtNum(data.total_elevation_m)}
          unit="m"
        />
        <StatCard
          label="Bewegungszeit"
          value={fmtDuration(data.total_moving_time_s)}
        />
        <StatCard
          label="Aktive Tage"
          value={fmtNum(data.active_days)}
        />
        <StatCard
          label="Länder"
          value={fmtNum(data.countries)}
        />
      </div>

      {/* Fun facts */}
      {(data.earth_laps > 0 || data.moon_trips > 0 || data.everest_count > 0) && (
        <div>
          <h3 className="text-sm font-semibold text-neutral-600 dark:text-neutral-400 mb-3 uppercase tracking-wide">
            Spass-Fakten
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            {data.earth_laps > 0 && (
              <FunFact
                emoji="🌍"
                text={`Du hast die Erde ${data.earth_laps.toFixed(2).replace('.', ',')}× umrundet`}
              />
            )}
            {data.moon_trips > 0 && (
              <FunFact
                emoji="🌙"
                text={`${data.moon_trips.toFixed(2).replace('.', ',')}× auf den Mond und zurück`}
              />
            )}
            {data.everest_count > 0 && (
              <FunFact
                emoji="⛰️"
                text={`${fmtNum(data.everest_count)}× den Mount Everest bestiegen`}
              />
            )}
          </div>
        </div>
      )}

      {/* Streak badges */}
      {(data.current_day_streak > 0 || data.max_day_streak > 0) && (
        <div>
          <h3 className="text-sm font-semibold text-neutral-600 dark:text-neutral-400 mb-3 uppercase tracking-wide">
            Serien
          </h3>
          <div className="flex flex-wrap gap-2">
            {data.current_day_streak > 0 && (
              <StreakBadge label="Aktuelle Serie" count={data.current_day_streak} />
            )}
            {data.max_day_streak > 0 && (
              <StreakBadge label="Rekord-Serie" count={data.max_day_streak} isRecord />
            )}
          </div>
        </div>
      )}
    </div>
  )
}
