import { useTranslation } from 'react-i18next'
import { useQueryClient, useMutation } from '@tanstack/react-query'
import type { Activity } from '@/api/types'
import { correctActivityElevation, resetActivityElevation, correctActivityDistance, resetActivityDistance } from '@/api/activities'
import { useCapabilities } from '@/hooks/useCapabilities'
import { useToast } from '@/lib/toast'
import { featureAvailable } from '@/lib/deploymentCapabilities'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

interface DataCorrectionsSectionProps {
  activity: Activity
}

export function DataCorrectionsSection({ activity }: DataCorrectionsSectionProps) {
  const { t } = useTranslation()
  const { formatDistance } = useUnitFormatters()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { data: routingCapabilities } = useCapabilities()

  const elevationCorrectionMutation = useMutation({
    mutationFn: () => correctActivityElevation(activity.id),
    onSuccess: () => {
      addToast({ type: 'success', title: t('activityDetail.dataCorrections.toasts.elevationCorrectionStarted'), message: t('activityDetail.dataCorrections.toasts.elevationCorrectionMessage') })
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
    },
    onError: () => addToast({ type: 'error', title: t('activityDetail.dataCorrections.toasts.elevationCorrectionFailed'), message: t('activityDetail.dataCorrections.toasts.retry') }),
  })

  const elevationResetMutation = useMutation({
    mutationFn: () => resetActivityElevation(activity.id),
    onSuccess: () => {
      addToast({ type: 'success', title: t('activityDetail.dataCorrections.toasts.elevationResetStarted'), message: t('activityDetail.dataCorrections.toasts.elevationResetMessage') })
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
    },
    onError: () => addToast({ type: 'error', title: t('activityDetail.dataCorrections.toasts.elevationResetFailed'), message: t('activityDetail.dataCorrections.toasts.retry') }),
  })

  const distanceCorrectionMutation = useMutation({
    mutationFn: () => correctActivityDistance(activity.id),
    onSuccess: () => {
      addToast({ type: 'success', title: t('activityDetail.dataCorrections.toasts.distanceCorrectionStarted'), message: t('activityDetail.dataCorrections.toasts.distanceCorrectionMessage') })
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
    },
    onError: () => addToast({ type: 'error', title: t('activityDetail.dataCorrections.toasts.distanceCorrectionFailed'), message: t('activityDetail.dataCorrections.toasts.retry') }),
  })

  const distanceResetMutation = useMutation({
    mutationFn: () => resetActivityDistance(activity.id),
    onSuccess: () => {
      addToast({ type: 'success', title: t('activityDetail.dataCorrections.toasts.distanceResetStarted'), message: t('activityDetail.dataCorrections.toasts.distanceResetMessage') })
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
    },
    onError: () => addToast({ type: 'error', title: t('activityDetail.dataCorrections.toasts.distanceResetFailed'), message: t('activityDetail.dataCorrections.toasts.retry') }),
  })

  const elevationSource = activity.elevation_source ?? 'device'
  const distanceSource = activity.distance_source ?? 'device'
  const isElevationCorrected = elevationSource === 'dem'
  const isDistanceCorrected = distanceSource === 'computed'
  const demCorrectionAvailable = featureAvailable(routingCapabilities, 'dem_correction')

  const distanceDisplay = formatDistance(activity.distance_m)

  const spinnerIcon = (
    <svg className="w-3.5 h-3.5 animate-spin" viewBox="0 0 24 24" fill="none">
      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2.5" strokeDasharray="50 20" />
    </svg>
  )
  const correctIcon = (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-3.5 h-3.5">
      <path fillRule="evenodd" d="M15.312 11.424a5.5 5.5 0 01-9.201 2.466l-.312-.311h2.451a.75.75 0 000-1.5H4.5a.75.75 0 00-.75.75v3.75a.75.75 0 001.5 0v-2.033a7 7 0 0011.712-3.138.75.75 0 00-1.449-.391zm-10.624-3.85a5.5 5.5 0 019.201-2.465l.312.31H11.75a.75.75 0 000 1.5h3.75a.75.75 0 00.75-.75V2.419a.75.75 0 00-1.5 0v2.034A7 7 0 003.038 7.59a.75.75 0 001.449.392z" clipRule="evenodd" />
    </svg>
  )
  const resetIcon = (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-3.5 h-3.5">
      <path fillRule="evenodd" d="M7.793 2.232a.75.75 0 01-.025 1.06L3.622 7.25h10.003a5.375 5.375 0 010 10.75H10.75a.75.75 0 010-1.5h2.875a3.875 3.875 0 000-7.75H3.622l4.146 3.957a.75.75 0 01-1.036 1.085l-5.5-5.25a.75.75 0 010-1.085l5.5-5.25a.75.75 0 011.061.025z" clipRule="evenodd" />
    </svg>
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="px-4 py-3 bg-neutral-50 dark:bg-neutral-900/60 border-b border-neutral-200 dark:border-neutral-800">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">{t('activityDetail.dataCorrections.title')}</h3>
      </div>
      <div className="divide-y divide-neutral-100 dark:divide-neutral-800">
        {/* Elevation uses an XOR swap: only one action is visible. */}
        <div className="px-4 py-3 flex items-center justify-between gap-4 flex-wrap">
          <div>
            <div className="text-sm font-medium text-neutral-800 dark:text-neutral-200">{t('activityDetail.dataCorrections.elevation')}</div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              {t('activityDetail.dataCorrections.source')}: <span className={isElevationCorrected ? 'text-emerald-600 dark:text-emerald-400' : ''}>
                {isElevationCorrected ? 'DEM (EEA-10)' : elevationSource === 'gps' ? 'GPS' : t('activityDetail.dataCorrections.device')}
              </span>
            </div>
          </div>
          {!isElevationCorrected && demCorrectionAvailable ? (
            <button
              onClick={() => elevationCorrectionMutation.mutate()}
              disabled={elevationCorrectionMutation.isPending}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium
                         bg-emerald-50 text-emerald-700 hover:bg-emerald-100
                         dark:bg-emerald-900/30 dark:text-emerald-400 dark:hover:bg-emerald-900/50
                         disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
              title={t('activityDetail.dataCorrections.applyDemTitle')}
            >
              {elevationCorrectionMutation.isPending ? spinnerIcon : correctIcon}
              {t('activityDetail.dataCorrections.applyDem')}
            </button>
          ) : isElevationCorrected ? (
            <button
              onClick={() => elevationResetMutation.mutate()}
              disabled={elevationResetMutation.isPending}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium
                         bg-neutral-100 text-neutral-600 hover:bg-neutral-200
                         dark:bg-neutral-800 dark:text-neutral-400 dark:hover:bg-neutral-700
                         disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
              title={t('activityDetail.dataCorrections.resetElevationTitle')}
            >
              {elevationResetMutation.isPending ? spinnerIcon : resetIcon}
              {t('activityDetail.dataCorrections.resetToGps')}
            </button>
          ) : null}
        </div>

        {/* Distance uses an XOR swap: only one action is visible. */}
        {activity.has_gps && (
          <div className="px-4 py-3 flex items-center justify-between gap-4 flex-wrap">
            <div>
              <div className="text-sm font-medium text-neutral-800 dark:text-neutral-200">{t('activityDetail.dataCorrections.distance')}</div>
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                {distanceDisplay} · {t('activityDetail.dataCorrections.source')}: <span className={isDistanceCorrected ? 'text-blue-600 dark:text-blue-400' : ''}>
                  {isDistanceCorrected ? 'GPS (Haversine)' : t('activityDetail.dataCorrections.device')}
                </span>
              </div>
            </div>
            {!isDistanceCorrected ? (
              <button
                onClick={() => distanceCorrectionMutation.mutate()}
                disabled={distanceCorrectionMutation.isPending}
                className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium
                           bg-blue-50 text-blue-700 hover:bg-blue-100
                           dark:bg-blue-900/30 dark:text-blue-400 dark:hover:bg-blue-900/50
                           disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
                title={t('activityDetail.dataCorrections.computeDistanceTitle')}
              >
                {distanceCorrectionMutation.isPending ? spinnerIcon : correctIcon}
                {t('activityDetail.dataCorrections.computeFromGps')}
              </button>
            ) : (
              <button
                onClick={() => distanceResetMutation.mutate()}
                disabled={distanceResetMutation.isPending}
                className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium
                           bg-neutral-100 text-neutral-600 hover:bg-neutral-200
                           dark:bg-neutral-800 dark:text-neutral-400 dark:hover:bg-neutral-700
                           disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
                title={t('activityDetail.dataCorrections.resetDistanceTitle')}
              >
                {distanceResetMutation.isPending ? spinnerIcon : resetIcon}
                {t('activityDetail.dataCorrections.reset')}
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
