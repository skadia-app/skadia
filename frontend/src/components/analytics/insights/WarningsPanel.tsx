import { useQuery } from '@tanstack/react-query'
import { fetchInsightsWarnings } from '@/api/analytics'
import type { Warning } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'

function severityBorderColor(severity: Warning['severity']): string {
  switch (severity) {
    case 'danger': return 'border-l-4 border-l-rose-500'
    case 'warning': return 'border-l-4 border-l-amber-500'
    case 'info': return 'border-l-4 border-l-blue-500'
  }
}

function severityBg(severity: Warning['severity']): string {
  switch (severity) {
    case 'danger': return 'bg-rose-50 dark:bg-rose-900/10'
    case 'warning': return 'bg-amber-50 dark:bg-amber-900/10'
    case 'info': return 'bg-blue-50 dark:bg-blue-900/10'
  }
}

function severityIcon(severity: Warning['severity']): string {
  switch (severity) {
    case 'danger': return '⚠️'
    case 'warning': return '⚡'
    case 'info': return 'ℹ️'
  }
}

function severityLabel(severity: Warning['severity']): string {
  switch (severity) {
    case 'danger': return 'Kritisch'
    case 'warning': return 'Warnung'
    case 'info': return 'Info'
  }
}

function severityBadge(severity: Warning['severity']): string {
  switch (severity) {
    case 'danger': return 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
    case 'warning': return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'info': return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
  }
}

function formatWarningType(type: string): string {
  return type
    .replace(/_/g, ' ')
    .replace(/\b\w/g, l => l.toUpperCase())
}


export default function WarningsPanel() {
  const { toQueryString } = useAnalyticsFilters()
  const queryParams = toQueryString()

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'insights', 'warnings', queryParams],
    queryFn: async () => {
      try {
        return await fetchInsightsWarnings(queryParams)
      } catch {
        return null
      }
    },
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 animate-pulse">
        <div className="h-4 w-32 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
        <div className="space-y-2">
          {[1, 2].map(i => (
            <div key={i} className="h-16 bg-neutral-200 dark:bg-neutral-700 rounded" />
          ))}
        </div>
      </div>
    )
  }

  // Sort: danger → warning → info
  const severityOrder: Record<Warning['severity'], number> = { danger: 0, warning: 1, info: 2 }
  const warnings = (data ?? []).slice().sort((a, b) => severityOrder[a.severity] - severityOrder[b.severity])

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-3">
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200">Warnungen &amp; Hinweise</h3>

      {isError && !data && (
        <div className="text-sm text-neutral-400 text-center py-4">
          Fehler beim Laden der Warnungen
        </div>
      )}

      {!isError && warnings.length === 0 && (
        <div className="flex flex-col items-center justify-center py-8 gap-2 text-neutral-400">
          <svg className="w-8 h-8 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
              d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span className="text-sm text-emerald-600 dark:text-emerald-400 font-medium">Keine Warnungen</span>
          <span className="text-xs text-neutral-400">Alles sieht gut aus!</span>
        </div>
      )}

      <div className="space-y-2">
        {warnings.map((warning, i) => (
          <div
            key={i}
            className={`rounded-lg ${severityBorderColor(warning.severity)} ${severityBg(warning.severity)} p-3 space-y-1`}
          >
            <div className="flex items-start justify-between gap-2">
              <div className="flex items-center gap-2">
                <span className="text-base leading-none" aria-hidden="true">{severityIcon(warning.severity)}</span>
                <span className="text-xs font-semibold text-neutral-800 dark:text-neutral-100">
                  {formatWarningType(warning.type)}
                </span>
              </div>
              <div className="flex items-center gap-1.5 flex-shrink-0">
                <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium ${severityBadge(warning.severity)}`}>
                  {severityLabel(warning.severity)}
                </span>
              </div>
            </div>

            <p className="text-xs text-neutral-600 dark:text-neutral-300">{warning.message}</p>

            <div className="flex items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400">
              <span>Wert: <strong className="text-neutral-700 dark:text-neutral-200">{warning.value.toFixed(1)}</strong></span>
              <span>Schwelle: <strong className="text-neutral-700 dark:text-neutral-200">{warning.threshold.toFixed(1)}</strong></span>
            </div>

            {warning.explanation && (
              <p className="text-xs text-neutral-400 dark:text-neutral-500 italic">{warning.explanation}</p>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
