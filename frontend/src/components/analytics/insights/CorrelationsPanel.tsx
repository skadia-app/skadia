import { useQuery } from '@tanstack/react-query'
import { fetchInsightsCorrelations } from '@/api/analytics'
import type { CorrelationInsight } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'

function correlationColor(r: number): string {
  if (r >= 0.5) return '#10b981'
  if (r >= 0.2) return '#6ee7b7'
  if (r > -0.2) return '#9ca3af'
  if (r > -0.5) return '#93c5fd'
  return '#3b82f6'
}

function correlationLabel(r: number, direction: CorrelationInsight['direction']): string {
  const abs = Math.abs(r)
  const strength = abs >= 0.7 ? 'stark' : abs >= 0.4 ? 'moderat' : 'schwach'
  if (direction === 'neutral') return `${strength} (neutral)`
  return `${strength} ${direction === 'positive' ? 'positiv' : 'negativ'}`
}

function formatFactorName(name: string): string {
  const labels: Record<string, string> = {
    tss: 'TSS',
    ctl: 'CTL (Fitness)',
    atl: 'ATL (Ermüdung)',
    tsb: 'TSB (Form)',
    hrv: 'HRV',
    heart_rate: 'Herzfrequenz',
    pace: 'Pace',
    distance: 'Distanz',
    elevation: 'Höhenmeter',
    sleep_score: 'Schlaf-Score',
    sleep_duration: 'Schlafdauer',
    weight: 'Gewicht',
    resting_hr: 'Ruhepuls',
    duration: 'Dauer',
    cadence: 'Kadenz',
    power: 'Leistung',
    temperature: 'Temperatur',
  }
  return labels[name.toLowerCase()] ?? name.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase())
}

function CorrelationBar({ r }: { r: number }) {
  const pct = Math.abs(r) * 100
  const color = correlationColor(r)
  const isNeg = r < 0

  return (
    <div className="flex items-center gap-1 w-full">
      {/* Negative side */}
      <div className="flex-1 flex justify-end">
        {isNeg ? (
          <div
            className="h-3 rounded-l"
            style={{
              width: `${pct}%`,
              backgroundColor: color,
              maxWidth: '100%',
            }}
          />
        ) : <div className="h-3" />}
      </div>

      {/* Center line */}
      <div className="w-px h-4 bg-neutral-300 dark:bg-neutral-600 flex-shrink-0" />

      {/* Positive side */}
      <div className="flex-1">
        {!isNeg ? (
          <div
            className="h-3 rounded-r"
            style={{
              width: `${pct}%`,
              backgroundColor: color,
              maxWidth: '100%',
            }}
          />
        ) : <div className="h-3" />}
      </div>
    </div>
  )
}

export default function CorrelationsPanel() {
  const { toQueryString } = useAnalyticsFilters()
  const queryParams = toQueryString()

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'insights', 'correlations', queryParams],
    queryFn: async () => {
      try {
        return await fetchInsightsCorrelations(queryParams)
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
        <div className="h-4 w-40 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
        <div className="space-y-3">
          {[1, 2, 3].map(i => (
            <div key={i} className="h-12 bg-neutral-200 dark:bg-neutral-700 rounded" />
          ))}
        </div>
      </div>
    )
  }

  const correlations = data ?? []

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200">Korrelationen</h3>

      {(isError || correlations.length === 0) && (
        <div className="flex flex-col items-center justify-center h-[150px] gap-2 text-neutral-400">
          <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
              d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z" />
          </svg>
          <span className="text-sm">Keine Korrelationsdaten verfügbar</span>
        </div>
      )}

      {correlations.length > 0 && (
        <>
          {/* Header: scale labels */}
          <div className="flex items-center gap-1 text-xs text-neutral-400 px-1">
            <div className="flex-1 text-right">-1</div>
            <div className="w-px" />
            <div className="flex-1 text-left pl-1">+1</div>
          </div>

          <div className="space-y-3">
            {correlations.map((corr, i) => (
              <div key={i} className="space-y-1">
                <div className="flex items-center justify-between gap-2 text-xs">
                  <span className="text-neutral-600 dark:text-neutral-300 font-medium truncate max-w-[45%]">
                    {formatFactorName(corr.factor_a)}
                  </span>
                  <span className="text-neutral-400 flex-shrink-0">&#8596;</span>
                  <span className="text-neutral-600 dark:text-neutral-300 font-medium truncate max-w-[45%] text-right">
                    {formatFactorName(corr.factor_b)}
                  </span>
                </div>

                <CorrelationBar r={corr.correlation} />

                <div className="flex items-center justify-between text-xs">
                  <span
                    className="text-neutral-500"
                    style={{ color: correlationColor(corr.correlation) }}
                  >
                    r = {corr.correlation.toFixed(2)} &middot; {correlationLabel(corr.correlation, corr.direction)}
                  </span>
                </div>

                {corr.explanation && (
                  <p className="text-xs text-neutral-400 dark:text-neutral-500 italic">{corr.explanation}</p>
                )}

                {i < correlations.length - 1 && (
                  <div className="border-t border-neutral-100 dark:border-neutral-800 mt-2" />
                )}
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
