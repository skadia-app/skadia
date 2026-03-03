import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { fetchPMC } from '@/api/stats'
import type { PMCDataPoint } from '@/api/stats'
import { fetchActivities } from '@/api/activities'
import type { Activity, Gear, MaintenanceAlert } from '@/api/types'
import { fetchGear, fetchMaintenanceAlerts } from '@/api/gear'
import { SportIcon } from '@/components/SportIcon'
import { sportLabel, sportColor, formatDuration, formatDistance, formatElevationGain } from '@/lib/utils'

// ── Helpers ──────────────────────────────────────────────────────────────────────

function todayISO(): string {
  return new Date().toISOString().split('T')[0]
}

function daysAgoISO(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() - days)
  return d.toISOString().split('T')[0]
}

function startOfWeekISO(): string {
  const d = new Date()
  const day = d.getDay()
  // Monday = start of week (European convention)
  const diff = day === 0 ? 6 : day - 1
  d.setDate(d.getDate() - diff)
  d.setHours(0, 0, 0, 0)
  return d.toISOString().split('T')[0]
}

function trendArrow(current: number, previous: number): string {
  const diff = current - previous
  if (Math.abs(diff) < 0.5) return ''
  return diff > 0 ? '\u2191' : '\u2193'
}

// ── Sparkline (inline SVG) ───────────────────────────────────────────────────────

function Sparkline({ data, color, className }: { data: number[]; color: string; className?: string }) {
  if (data.length < 2) return null
  const min = Math.min(...data)
  const max = Math.max(...data)
  const range = max - min || 1
  const w = 200
  const h = 40
  const points = data.map((v, i) => {
    const x = (i / (data.length - 1)) * w
    const y = h - ((v - min) / range) * (h - 4) - 2
    return `${x},${y}`
  })

  return (
    <svg viewBox={`0 0 ${w} ${h}`} className={className} preserveAspectRatio="none">
      <polyline
        points={points.join(' ')}
        fill="none"
        stroke={color}
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

// ── Component ────────────────────────────────────────────────────────────────────

export default function DashboardPage() {
  const navigate = useNavigate()
  const today = todayISO()
  const ninetyDaysAgo = daysAgoISO(90)

  // ── Data Fetching ──────────────────────────────────────────────────────────

  const { data: pmcData, isLoading: pmcLoading } = useQuery({
    queryKey: ['pmc', ninetyDaysAgo, today],
    queryFn: () => fetchPMC(ninetyDaysAgo, today),
    retry: 1,
  })

  const { data: activitiesData, isLoading: activitiesLoading } = useQuery({
    queryKey: ['dashboard-activities'],
    queryFn: () => fetchActivities(undefined, 8),
    retry: 1,
  })

  const { data: gearList } = useQuery({
    queryKey: ['gear-active'],
    queryFn: () => fetchGear('active'),
    retry: 1,
  })

  const { data: maintenanceAlerts } = useQuery({
    queryKey: ['maintenance-alerts'],
    queryFn: () => fetchMaintenanceAlerts(),
    retry: 1,
  })

  // ── PMC Processing ─────────────────────────────────────────────────────────

  const pmc: PMCDataPoint[] = pmcData ?? []
  const latest = pmc.length > 0 ? pmc[pmc.length - 1] : null
  const yesterday = pmc.length > 1 ? pmc[pmc.length - 2] : null

  // Last 30 days for sparkline
  const sparklineData = useMemo(() => {
    const last30 = pmc.slice(-30)
    return last30.map(d => d.ctl)
  }, [pmc])

  // ── Activities ─────────────────────────────────────────────────────────────

  const activities: Activity[] = activitiesData?.activities ?? []

  // ── This Week Stats ────────────────────────────────────────────────────────

  const weekStart = startOfWeekISO()
  const weekStats = useMemo(() => {
    const thisWeek = activities.filter(a => a.started_at.split('T')[0] >= weekStart)
    return {
      count: thisWeek.length,
      distance: thisWeek.reduce((sum, a) => sum + (a.distance_m ?? 0), 0),
      duration: thisWeek.reduce((sum, a) => sum + (a.moving_time_s ?? a.elapsed_time_s ?? 0), 0),
      elevation: thisWeek.reduce((sum, a) => sum + (a.elevation_gain_m ?? 0), 0),
    }
  }, [activities, weekStart])

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

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Dashboard</h1>

      {/* ── Widget 1: Training Load ─────────────────────────────────────── */}
      <section>
        <h2 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-3 uppercase tracking-wide">
          Trainingsbelastung
        </h2>
        {pmcLoading ? (
          <div className="grid grid-cols-3 gap-3">
            {[0, 1, 2].map(i => (
              <div key={i} className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 animate-pulse">
                <div className="h-3 w-20 bg-neutral-200 dark:bg-neutral-700 rounded mb-2" />
                <div className="h-7 w-16 bg-neutral-200 dark:bg-neutral-700 rounded" />
              </div>
            ))}
          </div>
        ) : latest ? (
          <>
            <div className="grid grid-cols-3 gap-3">
              <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4">
                <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">CTL (Fitness)</div>
                <div className="flex items-baseline gap-1.5">
                  <span className="text-2xl font-semibold text-blue-500 tabular-nums">
                    {latest.ctl.toFixed(1)}
                  </span>
                  {yesterday && (
                    <span className={`text-sm font-medium ${latest.ctl >= yesterday.ctl ? 'text-emerald-500' : 'text-rose-500'}`}>
                      {trendArrow(latest.ctl, yesterday.ctl)}
                    </span>
                  )}
                </div>
              </div>
              <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4">
                <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">ATL (Ermüdung)</div>
                <div className="flex items-baseline gap-1.5">
                  <span className="text-2xl font-semibold text-orange-500 tabular-nums">
                    {latest.atl.toFixed(1)}
                  </span>
                  {yesterday && (
                    <span className={`text-sm font-medium ${latest.atl <= yesterday.atl ? 'text-emerald-500' : 'text-rose-500'}`}>
                      {trendArrow(latest.atl, yesterday.atl)}
                    </span>
                  )}
                </div>
              </div>
              <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4">
                <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">TSB (Form)</div>
                <div className="flex items-baseline gap-1.5">
                  <span className={`text-2xl font-semibold tabular-nums ${latest.tsb >= 0 ? 'text-emerald-500' : 'text-rose-500'}`}>
                    {latest.tsb >= 0 ? '+' : ''}{latest.tsb.toFixed(1)}
                  </span>
                  {yesterday && (
                    <span className={`text-sm font-medium ${latest.tsb >= yesterday.tsb ? 'text-emerald-500' : 'text-rose-500'}`}>
                      {trendArrow(latest.tsb, yesterday.tsb)}
                    </span>
                  )}
                </div>
              </div>
            </div>

            {/* Sparkline */}
            {sparklineData.length > 1 && (
              <div className="mt-3 rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
                <div className="text-xs text-neutral-400 mb-1">CTL (letzte 30 Tage)</div>
                <Sparkline data={sparklineData} color="#3b82f6" className="w-full h-10" />
              </div>
            )}
          </>
        ) : (
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-400">
            Keine Trainingsdaten vorhanden
          </div>
        )}
      </section>

      {/* ── Widget 3: Diese Woche ───────────────────────────────────────── */}
      <section>
        <h2 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-3 uppercase tracking-wide">
          Diese Woche
        </h2>
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <StatTile label="Aktivitäten" value={String(weekStats.count)} />
          <StatTile
            label="Distanz"
            value={weekStats.distance >= 1000
              ? `${(weekStats.distance / 1000).toFixed(1)} km`
              : `${Math.round(weekStats.distance)} m`}
          />
          <StatTile label="Zeit" value={formatDuration(weekStats.duration)} />
          <StatTile
            label="Höhenmeter"
            value={weekStats.elevation > 0 ? `${Math.round(weekStats.elevation)} m` : '0 m'}
          />
        </div>
      </section>

      {/* ── Widget 2: Letzte Aktivitäten ────────────────────────────────── */}
      <section>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
            Letzte Aktivitäten
          </h2>
          <button
            onClick={() => navigate('/activities')}
            className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
          >
            Alle anzeigen
          </button>
        </div>

        {activitiesLoading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
            {[0, 1, 2, 3].map(i => (
              <div key={i} className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 animate-pulse">
                <div className="h-4 w-24 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
                <div className="h-3 w-32 bg-neutral-200 dark:bg-neutral-700 rounded mb-2" />
                <div className="h-3 w-20 bg-neutral-200 dark:bg-neutral-700 rounded" />
              </div>
            ))}
          </div>
        ) : activities.length > 0 ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
            {activities.map(a => (
              <button
                key={a.id}
                onClick={() => navigate(`/activities/${a.id}`)}
                className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 text-left
                           hover:border-neutral-300 dark:hover:border-neutral-700
                           hover:bg-neutral-50 dark:hover:bg-neutral-900/50 transition-colors group"
              >
                <div className="flex items-center gap-2 mb-2">
                  <span style={{ color: sportColor(a.sport_type) }}>
                    <SportIcon sport={a.sport_type} subType={a.sub_type} className="w-4 h-4" />
                  </span>
                  <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                    {a.title ?? sportLabel(a.sport_type, a.sub_type)}
                  </span>
                </div>
                <div className="text-xs text-neutral-500 dark:text-neutral-400 space-y-0.5">
                  <div>{format(new Date(a.started_at), 'dd. MMM yyyy', { locale: de })}</div>
                  <div className="flex items-center gap-2 flex-wrap">
                    {a.distance_m != null && a.distance_m > 0 && (
                      <span>{formatDistance(a.distance_m)}</span>
                    )}
                    <span>{formatDuration(a.moving_time_s ?? a.elapsed_time_s)}</span>
                    {a.elevation_gain_m != null && a.elevation_gain_m > 0 && (
                      <span>{formatElevationGain(a.elevation_gain_m)}</span>
                    )}
                  </div>
                  {a.tss != null && (
                    <div className="text-neutral-400">TSS {Math.round(a.tss)}</div>
                  )}
                </div>
              </button>
            ))}
          </div>
        ) : (
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-400">
            Keine Aktivitäten vorhanden
          </div>
        )}
      </section>

      {/* ── Widget 4: Gear-Wartungs-Alerts ──────────────────────────────── */}
      {gear.length > 0 && (
        <section>
          <h2 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-3 uppercase tracking-wide">
            Ausrüstung
          </h2>
          {gearWithAlerts.length > 0 ? (
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
          ) : (
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2">
              {gear.slice(0, 8).map(g => (
                <div
                  key={g.id}
                  className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 text-sm"
                >
                  <div className="font-medium text-neutral-900 dark:text-neutral-100 truncate">{g.name}</div>
                  <div className="text-xs text-neutral-400 mt-0.5">
                    {g.total_distance_m >= 1000
                      ? `${(g.total_distance_m / 1000).toFixed(0)} km`
                      : `${Math.round(g.total_distance_m)} m`}
                    {' \u00b7 '}
                    {g.total_activities} {g.total_activities === 1 ? 'Aktivität' : 'Aktivitäten'}
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      )}
    </div>
  )
}

// ── StatTile ─────────────────────────────────────────────────────────────────────

function StatTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
      <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-0.5">{label}</div>
      <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">{value}</div>
    </div>
  )
}
