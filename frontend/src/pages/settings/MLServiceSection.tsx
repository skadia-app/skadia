import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import Toggle from '@/components/Toggle'
import {
  getMLStatus,
  getMLSystemSettings,
  updateNightlyJob,
  type NightlyJobKey,
} from '@/api/ml'
import { useToast } from '@/lib/toast'

const NIGHTLY_JOBS = [
  {
    key: 'nightly_feature_engineering' as NightlyJobKey,
    label: 'Feature Engineering',
    description: 'NeuroKit2 HRV-Metriken + TSFEL-Features',
  },
  {
    key: 'nightly_tsfm_analysis' as NightlyJobKey,
    label: 'TSFM-Analyse',
    description: 'TSPulse Anomalie-Erkennung + TTM Prognosen',
  },
  {
    key: 'nightly_changepoint_detection' as NightlyJobKey,
    label: 'Changepoint-Erkennung',
    description: 'ruptures PELT Changepoints in Trainingsdaten',
  },
] as const

export function MLServiceSection() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const { data: mlStatus } = useQuery({
    queryKey: ['ml-status'],
    queryFn: getMLStatus,
    refetchInterval: 30_000,
  })

  const { data: mlSettings } = useQuery({
    queryKey: ['ml-system-settings'],
    queryFn: getMLSystemSettings,
  })

  const toggleJobMutation = useMutation({
    mutationFn: ({ key, enabled }: { key: NightlyJobKey; enabled: boolean }) => {
      const current = mlSettings?.[key] ?? { enabled: false, time: '02:00', description: '' }
      return updateNightlyJob(key, { ...current, enabled })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ml-system-settings'] })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Nightly-Job konnte nicht aktualisiert werden', duration: 0 })
    },
  })

  function toggleJob(key: NightlyJobKey, enabled: boolean) {
    toggleJobMutation.mutate({ key, enabled })
  }

  const isAvailable = mlStatus?.available ?? false

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">ML-Service</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Status des ML-Dienstes und n&auml;chtliche Analysejobs.
          </p>
        </div>
        <span
          className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium flex-shrink-0 ${
            isAvailable
              ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
              : 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'
          }`}
        >
          <span
            className={`h-2 w-2 rounded-full ${isAvailable ? 'bg-emerald-500' : 'bg-neutral-400'}`}
          />
          {isAvailable ? 'Erreichbar' : 'Nicht erreichbar'}
        </span>
      </div>

      <div className="space-y-2">
        <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          N&auml;chtliche Jobs
        </p>
        {NIGHTLY_JOBS.map(({ key, label, description }) => {
          const enabled = mlSettings?.[key]?.enabled ?? false
          const time = mlSettings?.[key]?.time ?? '—'
          return (
            <div key={key} className="flex items-center justify-between gap-4 py-1">
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{label}</span>
                <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-2">
                  — {description}
                </span>
                <span className="text-xs text-neutral-400 ml-2">({time} Uhr)</span>
              </div>
              <Toggle
                checked={enabled}
                onChange={() => toggleJob(key, !enabled)}
                aria-label={`${label} aktivieren`}
              />
            </div>
          )
        })}
      </div>
    </section>
  )
}
