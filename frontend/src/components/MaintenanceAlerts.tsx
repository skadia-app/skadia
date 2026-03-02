import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { fetchMaintenanceAlerts } from '@/api/gear'
import type { MaintenanceAlert } from '@/api/types'

export default function MaintenanceAlerts() {
  const { data: alerts } = useQuery({
    queryKey: ['maintenance-alerts'],
    queryFn: fetchMaintenanceAlerts,
  })

  if (!alerts || alerts.length === 0) return null

  return (
    <div className="rounded-xl border border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/20 p-4 space-y-3">
      <h2 className="text-sm font-semibold text-amber-800 dark:text-amber-300">Wartungshinweise</h2>
      <div className="space-y-2">
        {alerts.map((alert) => (
          <AlertItem key={`${alert.gear_id}-${alert.component_id}`} alert={alert} />
        ))}
      </div>
    </div>
  )
}

function AlertItem({ alert }: { alert: MaintenanceAlert }) {
  const pct = alert.threshold_value > 0
    ? Math.round((alert.current_value / alert.threshold_value) * 100)
    : 0

  return (
    <Link
      to={`/gear/${alert.gear_id}`}
      className="flex items-center justify-between gap-3 rounded-lg px-3 py-2 hover:bg-amber-100 dark:hover:bg-amber-900/30 transition-colors"
    >
      <div className="min-w-0">
        <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
          {alert.gear_name} &mdash; {alert.component_name}
        </div>
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          {alert.interval_name}: {formatAlertValue(alert.interval_type, alert.current_value)} / {formatAlertValue(alert.interval_type, alert.threshold_value)}
        </div>
      </div>
      <span
        className={`flex-shrink-0 px-2 py-0.5 rounded text-xs font-medium ${
          alert.is_overdue
            ? 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-400'
            : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
        }`}
      >
        {alert.is_overdue ? `Ueberfaellig (${pct}%)` : `${pct}%`}
      </span>
    </Link>
  )
}

function formatAlertValue(type: string, value: number): string {
  if (type === 'distance') return `${(value / 1000).toFixed(0)} km`
  if (type === 'time') return `${Math.round(value / 3600)} h`
  return `${value}`
}
