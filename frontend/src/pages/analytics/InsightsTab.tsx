import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { refreshInsights } from '@/api/analytics'
import WarningsPanel from '@/components/analytics/insights/WarningsPanel'
import FitnessProjection from '@/components/analytics/insights/FitnessProjection'
import TrainingClusters from '@/components/analytics/insights/TrainingClusters'
import CorrelationsPanel from '@/components/analytics/insights/CorrelationsPanel'
import ReadinessCardFull from '@/components/analytics/ReadinessCard'

export default function InsightsTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const refreshMutation = useMutation({
    mutationFn: refreshInsights,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['analytics', 'insights'] })
      queryClient.invalidateQueries({ queryKey: ['analytics', 'readiness'] })
    },
  })

  return (
    <div className="space-y-4">
      {/* Header with refresh button */}
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-base font-semibold text-neutral-800 dark:text-neutral-100">
          {t('analyticsInsights.title')}
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
              {t('analyticsInsights.refreshing')}
            </>
          ) : (
            <>
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                  d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              {t('analyticsInsights.refresh')}
            </>
          )}
        </button>
      </div>

      {refreshMutation.isSuccess && (
        <div className="rounded-lg bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 px-3 py-2 text-xs text-emerald-700 dark:text-emerald-300">
          {t('analyticsInsights.refreshSuccess')}
        </div>
      )}

      {refreshMutation.isError && (
        <div className="rounded-lg bg-rose-50 dark:bg-rose-900/20 border border-rose-200 dark:border-rose-800 px-3 py-2 text-xs text-rose-700 dark:text-rose-300">
          {t('analyticsInsights.refreshError')}
        </div>
      )}

      {/* Readiness — always rendered; ReadinessCardFull manages its own empty/error state */}
      <ReadinessCardFull />

      {/* Ask the assistant */}
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
          {t('analyticsInsights.askAssistant')}
        </button>
      </div>

      {/* Each panel manages its own loading/empty/error state — no shared probe gate. */}
      <WarningsPanel />
      <FitnessProjection />
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <TrainingClusters />
        <CorrelationsPanel />
      </div>
    </div>
  )
}
