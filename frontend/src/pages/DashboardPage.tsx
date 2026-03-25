import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Link } from 'react-router-dom'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { fetchActivities } from '@/api/activities'
import type { Activity, Gear, MaintenanceAlert } from '@/api/types'
import { fetchGear, fetchMaintenanceAlerts } from '@/api/gear'
import { SportIcon } from '@/components/SportIcon'
import { sportLabel, sportColor } from '@/lib/sportTaxonomy'
import { formatDuration, formatDistance, formatElevationGain } from '@/lib/utils'
import { fetchInsightsReadiness, fetchHRVAnalysis } from '@/api/analytics'
import type { ReadinessInsight, HRVAnalysis } from '@/api/analytics'
import { toLocalISO } from '@/lib/dateUtils'

// ── Helpers ──────────────────────────────────────────────────────────────────────

function startOfWeekISO(): string {
  const d = new Date()
  const day = d.getDay()
  // Monday = start of week (European convention)
  const diff = day === 0 ? 6 : day - 1
  d.setDate(d.getDate() - diff)
  d.setHours(0, 0, 0, 0)
  return toLocalISO(d)
}

function sevenDaysAgo(): string {
  const d = new Date()
  d.setDate(d.getDate() - 7)
  return toLocalISO(d)
}

// ── Readiness Ring ────────────────────────────────────────────────────────────────

function ReadinessRing({ score }: { score: number | null }) {
  const size = 72
  const strokeWidth = 7
  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius
  const pct = score != null ? Math.min(Math.max(score, 0), 100) / 100 : 0
  const offset = circumference * (1 - pct)

  const color =
    score == null ? '#6b7280'
    : score >= 70 ? '#22c55e'
    : score >= 40 ? '#eab308'
    : '#ef4444'

  return (
    <svg width={size} height={size} className="rotate-[-90deg]">
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill="none"
        stroke="currentColor"
        strokeWidth={strokeWidth}
        className="text-neutral-200 dark:text-neutral-700"
      />
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill="none"
        stroke={color}
        strokeWidth={strokeWidth}
        strokeDasharray={circumference}
        strokeDashoffset={offset}
        strokeLinecap="round"
      />
    </svg>
  )
}

function readinessLevel(score: number): string {
  if (score >= 70) return 'Bereit'
  if (score >= 40) return 'Moderat'
  return 'Erholen'
}

function readinessColor(score: number): string {
  if (score >= 70) return 'text-green-600 dark:text-green-400'
  if (score >= 40) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-red-500 dark:text-red-400'
}

// ── Component ────────────────────────────────────────────────────────────────────

export default function DashboardPage() {
  const navigate = useNavigate()

  // ── Data Fetching ──────────────────────────────────────────────────────────

  const { data: activitiesData, isLoading: activitiesLoading } = useQuery({
    queryKey: ['dashboard-activities'],
    queryFn: () => fetchActivities(undefined, 5),
    staleTime: 2 * 60 * 1000,
    retry: false,
  })

  const weekStart = startOfWeekISO()
  const { data: weekActivitiesData } = useQuery({
    queryKey: ['activities-this-week', weekStart],
    queryFn: () => fetchActivities(undefined, 50, undefined, undefined, { dateFrom: weekStart }),
    staleTime: 2 * 60 * 1000,
    retry: false,
  })

  const { data: gearList } = useQuery({
    queryKey: ['gear-active'],
    queryFn: () => fetchGear('active'),
    staleTime: 5 * 60 * 1000,
    retry: false,
  })

  const { data: maintenanceAlerts } = useQuery({
    queryKey: ['maintenance-alerts'],
    queryFn: () => fetchMaintenanceAlerts(),
    staleTime: 5 * 60 * 1000,
    retry: false,
  })

  const { data: readiness } = useQuery<ReadinessInsight | null>({
    queryKey: ['analytics', 'insights', 'readiness'],
    queryFn: async () => {
      try { return await fetchInsightsReadiness('') }
      catch { return null }
    },
    staleTime: 5 * 60 * 1000,
    retry: false,
  })

  const { data: hrv } = useQuery<HRVAnalysis | null>({
    queryKey: ['analytics', 'hrv', '7d'],
    queryFn: async () => {
      try { return await fetchHRVAnalysis('from=' + sevenDaysAgo()) }
      catch { return null }
    },
    staleTime: 5 * 60 * 1000,
    retry: false,
  })

  // ── Activities ─────────────────────────────────────────────────────────────

  const activities: Activity[] = activitiesData?.activities ?? []

  // ── This Week Stats ────────────────────────────────────────────────────────

  const weekActivities: Activity[] = weekActivitiesData?.activities ?? []
  const weekStats = useMemo(() => {
    return {
      count: weekActivities.length,
      distance: weekActivities.reduce((sum, a) => sum + (a.distance_m ?? 0), 0),
      duration: weekActivities.reduce((sum, a) => sum + (a.moving_time_s ?? a.elapsed_time_s ?? 0), 0),
      elevation: weekActivities.reduce((sum, a) => sum + (a.elevation_gain_m ?? 0), 0),
    }
  }, [weekActivities])

  // ── Gear with Alerts ───────────────────────────────────────────────────────

  const alerts: MaintenanceAlert[] = maintenanceAlerts ?? []
  const gear: Gear[] = gearList ?? []

  const gearWithAlerts = useMemo(() => {
    const alertGearIds = new Set(alerts.map(a => a.gear_id))
    return gear
      .filter(g => alertGearIds.has(g.id))
      .map(g => ({
        ...g,
        alerts: alerts.filter(a => a.gear_id === g.id),
      }))
  }, [gear, alerts])

  // ── HRV trend ─────────────────────────────────────────────────────────────

  const hrvAvg = hrv?.seven_day_avg ?? null
  const hrvBaseline = hrv?.baseline_mean ?? null
  const hrvDiff = hrvAvg != null && hrvBaseline != null ? hrvAvg - hrvBaseline : null

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Dashboard</h1>

      {/* ── Row 1: Readiness / Diese Woche / HRV ───────────────────────── */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">

        {/* Readiness Score Card */}
        <Link
          to="/analytics/insights"
          className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 flex items-center gap-4
                     hover:border-neutral-300 dark:hover:border-neutral-700
                     hover:bg-neutral-50 dark:hover:bg-neutral-900/50 transition-colors group"
        >
          <div className="relative flex-shrink-0">
            <ReadinessRing score={readiness?.score ?? null} />
            <span className="absolute inset-0 flex items-center justify-center text-sm font-bold text-neutral-900 dark:text-neutral-100">
              {readiness?.score != null ? Math.round(readiness.score) : '–'}
            </span>
          </div>
          <div className="min-w-0">
            <div className="text-xs text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-0.5">Bereitschaft</div>
            {readiness?.score != null ? (
              <div className={`text-base font-semibold ${readinessColor(readiness.score)}`}>
                {readinessLevel(readiness.score)}
              </div>
            ) : (
              <div className="text-sm text-neutral-400">Keine Daten</div>
            )}
            {readiness?.explanation && (
              <div className="text-xs text-neutral-400 mt-0.5 line-clamp-2">{readiness.explanation}</div>
            )}
          </div>
        </Link>

        {/* Diese Woche Card */}
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4">
          <div className="text-xs text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-3">Diese Woche</div>
          <div className="grid grid-cols-2 gap-y-2 gap-x-3">
            <MiniStat label="Aktivitäten" value={String(weekStats.count)} />
            <MiniStat
              label="Distanz"
              value={weekStats.distance >= 1000
                ? `${(weekStats.distance / 1000).toFixed(1)} km`
                : `${Math.round(weekStats.distance)} m`}
            />
            <MiniStat label="Zeit" value={formatDuration(weekStats.duration)} />
            <MiniStat
              label="Höhenmeter"
              value={weekStats.elevation > 0 ? `${Math.round(weekStats.elevation)} m` : '0 m'}
            />
          </div>
        </div>

        {/* HRV Card */}
        <Link
          to="/analytics/health"
          className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4
                     hover:border-neutral-300 dark:hover:border-neutral-700
                     hover:bg-neutral-50 dark:hover:bg-neutral-900/50 transition-colors group"
        >
          <div className="text-xs text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">HRV (7 Tage)</div>
          <div className="flex items-end gap-2">
            <span className="text-2xl font-bold tabular-nums text-neutral-900 dark:text-neutral-100">
              {hrvAvg != null ? Math.round(hrvAvg) : '–'}
            </span>
            <span className="text-sm text-neutral-400 mb-0.5">ms</span>
            {hrvDiff != null && (
              <span className={`text-sm font-medium mb-0.5 ${
                hrvDiff > 0
                  ? 'text-green-600 dark:text-green-400'
                  : 'text-red-500 dark:text-red-400'
              }`}>
                {hrvDiff > 0 ? '↑' : '↓'} {Math.abs(Math.round(hrvDiff))}
              </span>
            )}
          </div>
          {hrvBaseline != null && (
            <div className="text-xs text-neutral-400 mt-1">
              Baseline: {Math.round(hrvBaseline)} ms
            </div>
          )}
          {hrv == null && (
            <div className="text-sm text-neutral-400">Keine HRV-Daten</div>
          )}
        </Link>
      </div>

      {/* ── Row 2: Letzte Aktivitäten ──────────────────────────────────── */}
      <section>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
            Letzte Aktivitäten
          </h2>
          <button
            onClick={() => navigate('/activities')}
            className="cursor-pointer text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
          >
            Alle anzeigen
          </button>
        </div>

        {activitiesLoading ? (
          <div className="space-y-2">
            {[0, 1, 2].map(i => (
              <div key={i} className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 animate-pulse">
                <div className="h-3 w-40 bg-neutral-200 dark:bg-neutral-700 rounded mb-2" />
                <div className="h-3 w-28 bg-neutral-200 dark:bg-neutral-700 rounded" />
              </div>
            ))}
          </div>
        ) : activities.length > 0 ? (
          <div className="space-y-1.5">
            {activities.map(a => (
              <button
                key={a.id}
                onClick={() => navigate(`/activities/${a.id}`)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-800 px-4 py-2.5 text-left
                           hover:border-neutral-300 dark:hover:border-neutral-700
                           hover:bg-neutral-50 dark:hover:bg-neutral-900/50 transition-colors
                           flex items-center gap-3 cursor-pointer"
              >
                <span style={{ color: sportColor(a.sport_type) }} className="flex-shrink-0">
                  <SportIcon sport={a.sport_type} subType={a.sub_type} isEbike={a.is_ebike} className="w-4 h-4" />
                </span>
                <span className="flex-1 min-w-0 text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                  {a.title ?? sportLabel(a.sport_type, a.sub_type)}
                </span>
                <span className="text-xs text-neutral-400 flex-shrink-0">
                  {format(new Date(a.started_at), 'dd. MMM', { locale: de })}
                </span>
                <span className="text-xs text-neutral-400 flex-shrink-0 hidden sm:inline">
                  {formatDuration(a.moving_time_s ?? a.elapsed_time_s)}
                </span>
                {a.distance_m != null && a.distance_m > 0 && (
                  <span className="text-xs text-neutral-400 flex-shrink-0 hidden sm:inline">
                    {formatDistance(a.distance_m)}
                  </span>
                )}
                {a.elevation_gain_m != null && a.elevation_gain_m > 0 && (
                  <span className="text-xs text-neutral-400 flex-shrink-0 hidden md:inline">
                    {formatElevationGain(a.elevation_gain_m)}
                  </span>
                )}
              </button>
            ))}
          </div>
        ) : (
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-400">
            Keine Aktivitäten vorhanden
          </div>
        )}
      </section>

      {/* ── Row 3: Gear-Alerts (nur wenn Alerts vorhanden) ─────────────── */}
      {gearWithAlerts.length > 0 && (
        <section>
          <h2 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-3 uppercase tracking-wide">
            Ausrüstung — Wartung fällig
          </h2>
          <div className="space-y-2">
            {gearWithAlerts.map(g => (
              <div
                key={g.id}
                className="rounded-lg border border-amber-200 dark:border-amber-800/50 bg-amber-50/50 dark:bg-amber-950/20 p-3"
              >
                <div className="flex items-center gap-2 mb-1">
                  <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                    {g.name}
                  </span>
                  <span className="text-xs px-1.5 py-0.5 rounded bg-amber-200 dark:bg-amber-800 text-amber-800 dark:text-amber-200 font-medium">
                    Wartung fällig
                  </span>
                </div>
                <div className="text-xs text-neutral-500 dark:text-neutral-400 space-y-0.5">
                  {g.alerts.map((alert, i) => (
                    <div key={i}>
                      {alert.component_name}: {alert.interval_name}
                      {alert.is_overdue && (
                        <span className="text-rose-500 ml-1">
                          ({Math.round(alert.current_value)} / {Math.round(alert.threshold_value)} {alert.interval_type === 'distance_km' ? 'km' : alert.interval_type === 'time_hours' ? 'h' : ''})
                        </span>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}

// ── MiniStat ─────────────────────────────────────────────────────────────────────

function MiniStat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs text-neutral-400 dark:text-neutral-500">{label}</div>
      <div className="text-sm font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">{value}</div>
    </div>
  )
}
