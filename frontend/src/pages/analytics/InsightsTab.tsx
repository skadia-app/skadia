import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import {
  fetchInsightsReadiness,
  fetchInsightsWarnings,
  fetchInsightsClusters,
  refreshInsights,
} from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import ReadinessGauge from '@/components/analytics/insights/ReadinessGauge'
import WarningsPanel from '@/components/analytics/insights/WarningsPanel'
import FitnessProjection from '@/components/analytics/insights/FitnessProjection'
import TrainingClusters from '@/components/analytics/insights/TrainingClusters'
import CorrelationsPanel from '@/components/analytics/insights/CorrelationsPanel'
import ReadinessV2Card from '@/components/analytics/ReadinessV2Card'

export default function InsightsTab() {
  const { toQueryString } = useAnalyticsFilters()
  const queryParams = toQueryString()
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  // Probe queries to determine if any data is available yet
  const { data: readiness, isLoading: readinessLoading } = useQuery({
    queryKey: ['analytics', 'insights', 'readiness', queryParams],
    queryFn: async () => {
      try {
        return await fetchInsightsReadiness(queryParams)
      } catch {
        return null
      }
    },
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const { data: warnings, isLoading: warningsLoading } = useQuery({
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

  const { data: clusters, isLoading: clustersLoading } = useQuery({
    queryKey: ['analytics', 'insights', 'clusters', queryParams],
    queryFn: async () => {
      try {
        return await fetchInsightsClusters(queryParams)
      } catch {
        return null
      }
    },
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const refreshMutation = useMutation({
    mutationFn: refreshInsights,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['analytics', 'insights'] })
    },
  })

  const isProbing = readinessLoading && warningsLoading && clustersLoading
  const hasNoData = !isProbing && !readiness && (!warnings || warnings.length === 0) && (!clusters || clusters.length === 0)

  // Show empty state when all data is absent
  if (hasNoData) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-4">
        <div className="rounded-xl border border-accent/30 bg-accent/5 p-8 max-w-md w-full text-center space-y-4">
          <div className="flex justify-center">
            <svg className="w-12 h-12 text-accent opacity-60" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
            </svg>
          </div>
          <div className="space-y-2">
            <h3 className="text-base font-semibold text-neutral-800 dark:text-neutral-100">
              Insights noch nicht verfügbar
            </h3>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">
              Insights werden nach der ersten Aktivität automatisch berechnet. Du kannst die Berechnung auch jetzt starten.
            </p>
          </div>
          <button
            onClick={() => refreshMutation.mutate()}
            disabled={refreshMutation.isPending}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-accent text-white text-sm font-medium
                       hover:bg-accent/90 cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {refreshMutation.isPending ? (
              <>
                <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Berechne…
              </>
            ) : (
              <>
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                    d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
                Jetzt berechnen
              </>
            )}
          </button>
          {refreshMutation.isError && (
            <p className="text-xs text-rose-500">Fehler beim Berechnen der Insights</p>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Header with refresh button */}
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-base font-semibold text-neutral-800 dark:text-neutral-100">
          Intelligente Insights
        </h2>
        <button
          onClick={() => refreshMutation.mutate()}
          disabled={refreshMutation.isPending}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700
                     text-xs font-medium text-neutral-600 dark:text-neutral-300
                     hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors
                     disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {refreshMutation.isPending ? (
            <>
              <svg className="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
              Aktualisiere…
            </>
          ) : (
            <>
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                  d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              Insights aktualisieren
            </>
          )}
        </button>
      </div>

      {refreshMutation.isSuccess && (
        <div className="rounded-lg bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 px-3 py-2 text-xs text-emerald-700 dark:text-emerald-300">
          Insights wurden erfolgreich aktualisiert.
        </div>
      )}

      {refreshMutation.isError && (
        <div className="rounded-lg bg-rose-50 dark:bg-rose-900/20 border border-rose-200 dark:border-rose-800 px-3 py-2 text-xs text-rose-700 dark:text-rose-300">
          Fehler beim Aktualisieren der Insights.
        </div>
      )}

      {/* Readiness v2 — full width */}
      <ReadinessV2Card />

      {/* Frag den Assistenten */}
      <div className="flex justify-end">
        <button
          onClick={() => navigate(`/chat?context=readiness&date=${new Date().toISOString().slice(0, 10)}`)}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-accent text-white text-sm font-medium
                     hover:bg-accent/90 cursor-pointer transition-colors"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
              d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z" />
          </svg>
          Frag den Assistenten
        </button>
      </div>

      {/* Row 1: Readiness (left) | Warnings (right) */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <ReadinessGauge />
        <WarningsPanel />
      </div>

      {/* Row 2: Fitness Projection (full width) */}
      <FitnessProjection />

      {/* Row 3: Training Clusters (left) | Correlations (right) */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <TrainingClusters />
        <CorrelationsPanel />
      </div>
    </div>
  )
}
