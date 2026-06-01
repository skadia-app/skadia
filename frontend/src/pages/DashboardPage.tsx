import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { fetchActivities } from '@/api/activities'
import type { Activity } from '@/api/types'
import { SportIcon } from '@/components/SportIcon'
import { sportLabel, sportColor } from '@/lib/sportTaxonomy'
import { displayActivityTitle } from '@/lib/activityTitle'
import { formatDuration } from '@/lib/utils'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { normTz } from '@/lib/format'
import { fetchHRVAnalysis } from '@/api/analytics'
import type { HRVAnalysis } from '@/api/analytics'
import { ReadinessCardCompact } from '@/components/analytics/ReadinessCard'
import { toLocalISO } from '@/lib/dateUtils'
import MaintenanceAlerts from '@/components/MaintenanceAlerts'
import { isSupportedLanguage, localeForLanguage } from '@/i18n/languages'

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

// ── Component ────────────────────────────────────────────────────────────────────

export default function DashboardPage() {
  const { t, i18n } = useTranslation()
  const { formatDistance, formatDistanceCompact, formatElevation, formatElevationGain } = useUnitFormatters()
  const navigate = useNavigate()
  const activeLanguage = isSupportedLanguage(i18n.language) ? i18n.language : 'de'
  const locale = localeForLanguage(activeLanguage)

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

  // ── HRV trend ─────────────────────────────────────────────────────────────

  const hrvAvg = hrv?.seven_day_avg ?? null
  const hrvBaseline = hrv?.baseline_mean ?? null
  const hrvDiff = hrvAvg != null && hrvBaseline != null ? hrvAvg - hrvBaseline : null

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{t('dashboardPage.title')}</h1>

      <MaintenanceAlerts />

      {/* ── Row 1: Readiness / this week / HRV ─────────────────────────── */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">

        {/* Readiness Score Card */}
        <ReadinessCardCompact />

        {/* This week card */}
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4">
          <div className="text-xs text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-3">{t('dashboardPage.thisWeek')}</div>
          <div className="grid grid-cols-2 gap-y-2 gap-x-3">
            <MiniStat label={t('dashboardPage.activities')} value={String(weekStats.count)} />
            <MiniStat
              label={t('dashboardPage.distance')}
              value={formatDistanceCompact(weekStats.distance)}
            />
            <MiniStat label={t('dashboardPage.time')} value={formatDuration(weekStats.duration)} />
            <MiniStat
              label={t('dashboardPage.elevation')}
              value={formatElevation(weekStats.elevation)}
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
          <div className="text-xs text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">{t('dashboardPage.hrvSevenDays')}</div>
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
              {t('dashboardPage.baseline', { value: Math.round(hrvBaseline) })}
            </div>
          )}
          {hrv == null && (
            <div className="text-sm text-neutral-400">{t('dashboardPage.noHrvData')}</div>
          )}
        </Link>
      </div>

      {/* ── Row 2: recent activities ───────────────────────────────────── */}
      <section>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
            {t('dashboardPage.recentActivities')}
          </h2>
          <button
            onClick={() => navigate('/activities')}
            className="cursor-pointer text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
          >
            {t('dashboardPage.showAll')}
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
                  <SportIcon sport={a.sport_type} subType={a.sub_type} isElectric={a.is_electric} className="w-4 h-4" />
                </span>
                <span className="flex-1 min-w-0 text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                  {displayActivityTitle(a, sportLabel(a.sport_type, a.sub_type))}
                </span>
                <span className="text-xs text-neutral-400 flex-shrink-0">
                  {new Date(a.started_at).toLocaleDateString(locale, {
                    day: '2-digit', month: 'short',
                    ...(normTz(a.timezone) ? { timeZone: normTz(a.timezone) } : {}),
                  })}
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
            {t('dashboardPage.noActivities')}
          </div>
        )}
      </section>

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
