import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { fetchActivities } from '@/api/activities'
import type { Activity, Gear, MaintenanceAlert } from '@/api/types'
import { fetchGear, fetchMaintenanceAlerts } from '@/api/gear'
import { SportIcon } from '@/components/SportIcon'
import { sportLabel, sportColor } from '@/lib/sportTaxonomy'
import { formatDuration, formatDistance, formatElevationGain } from '@/lib/utils'
import PMCChart from '@/components/PMCChart'

// ── Helpers ──────────────────────────────────────────────────────────────────────

function startOfWeekISO(): string {
  const d = new Date()
  const day = d.getDay()
  // Monday = start of week (European convention)
  const diff = day === 0 ? 6 : day - 1
  d.setDate(d.getDate() - diff)
  d.setHours(0, 0, 0, 0)
  return d.toISOString().split('T')[0]
}

// ── Component ────────────────────────────────────────────────────────────────────

export default function DashboardPage() {
  const navigate = useNavigate()

  // ── Data Fetching ──────────────────────────────────────────────────────────

  const { data: activitiesData, isLoading: activitiesLoading } = useQuery({
    queryKey: ['dashboard-activities'],
    queryFn: () => fetchActivities(undefined, 8),
    retry: 1,
  })

  const weekStart = startOfWeekISO()
  const { data: weekActivitiesData } = useQuery({
    queryKey: ['activities-this-week', weekStart],
    queryFn: () => fetchActivities(undefined, 50, undefined, undefined, { dateFrom: weekStart }),
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

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Dashboard</h1>

      {/* ── Widget 1: Training Load ─────────────────────────────────────── */}
      <section>
        <PMCChart />
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
