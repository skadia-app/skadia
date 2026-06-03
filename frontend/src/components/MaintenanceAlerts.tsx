import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { fetchMaintenanceAlerts } from '@/api/gear'
import type { MaintenanceAlert } from '@/api/types'
import { formatUsageMetricValue } from '@/lib/gearUsageMetrics'
import { displayServiceIntervalName } from '@/lib/i18nDisplay'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

type AlertGroup = {
  gearId: string
  gearName: string
  alerts: MaintenanceAlert[]
}

export default function MaintenanceAlerts() {
  const { t } = useTranslation()
  const { data: alerts } = useQuery({
    queryKey: ['maintenance-alerts'],
    queryFn: fetchMaintenanceAlerts,
  })

  if (!alerts || alerts.length === 0) return null

  const groups = groupAlertsByGear(alerts)

  return (
    <section className="rounded-xl border border-amber-300 bg-amber-50/80 p-4 shadow-sm shadow-amber-950/5 dark:border-amber-800/70 dark:bg-amber-950/20 dark:shadow-none">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold text-amber-900 dark:text-amber-200">{t('activityOperations.maintenance.title')}</h2>
        <span className="rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800 dark:bg-amber-900/40 dark:text-amber-200">
          {t('activityOperations.maintenance.entryCount', { count: alerts.length })}
        </span>
      </div>
      <div className="space-y-3">
        {groups.map((group) => (
          <div key={group.gearId} className="rounded-lg border border-amber-200/80 bg-white/60 p-2 dark:border-amber-800/60 dark:bg-neutral-950/25">
            <div className="mb-1 flex items-center justify-between gap-3 px-2 py-1">
              <h3 className="truncate text-sm font-semibold text-neutral-900 dark:text-neutral-100">
                {group.gearName}
              </h3>
              <span className="shrink-0 text-xs font-medium text-amber-700 dark:text-amber-300">
                {group.alerts.some(alert => alert.is_overdue)
                  ? t('activityOperations.maintenance.due')
                  : t('activityOperations.maintenance.dueSoon')}
              </span>
            </div>
            <div className="space-y-1">
              {group.alerts.map((alert) => (
                <AlertItem key={alert.interval_id} alert={alert} />
              ))}
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

function AlertItem({ alert }: { alert: MaintenanceAlert }) {
  const { t } = useTranslation()
  const { options: unitOptions } = useUnitFormatters()
  const pct = alert.threshold_value > 0
    ? Math.round((alert.current_value / alert.threshold_value) * 100)
    : 0

  return (
    <Link
      to={`/gear/${alert.gear_id}?tab=components&component=${alert.component_id}`}
      className="group flex items-center justify-between gap-3 rounded-md px-2 py-2 transition-colors hover:bg-amber-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500 dark:hover:bg-amber-900/30"
    >
      <div className="min-w-0">
        <div className="truncate text-sm font-medium text-neutral-900 group-hover:text-amber-950 dark:text-neutral-100 dark:group-hover:text-amber-100">
          {alert.component_name}
        </div>
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          {displayServiceIntervalName(alert.interval_name, t)}: {formatUsageMetricValue(alert.interval_type, alert.current_value, unitOptions)} / {formatUsageMetricValue(alert.interval_type, alert.threshold_value, unitOptions)}
        </div>
      </div>
      <span
        className={`flex-shrink-0 px-2 py-0.5 rounded text-xs font-medium ${
          alert.is_overdue
            ? 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-400'
            : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
        }`}
      >
        {alert.is_overdue ? t('activityOperations.maintenance.overduePercent', { pct }) : `${pct}%`}
      </span>
    </Link>
  )
}

function groupAlertsByGear(alerts: MaintenanceAlert[]): AlertGroup[] {
  const groups = new Map<string, AlertGroup>()

  for (const alert of alerts) {
    const existing = groups.get(alert.gear_id)
    if (existing) {
      existing.alerts.push(alert)
      continue
    }

    groups.set(alert.gear_id, {
      gearId: alert.gear_id,
      gearName: alert.gear_name,
      alerts: [alert],
    })
  }

  return Array.from(groups.values())
}
