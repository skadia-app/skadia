import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery } from '@tanstack/react-query'
import { createCommuteRule } from '@/api/rules'
import { listPrivacyZones } from '@/api/privacy'
import { useToast } from '@/lib/toast'
import { ModalShell } from '@/components/ui/ModalShell'

interface CommuteModalProps {
  activityId: string
  isRoundTrip?: boolean
  onClose: () => void
}

export default function CommuteModal({ activityId, isRoundTrip, onClose }: CommuteModalProps) {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const [selectedZone, setSelectedZone] = useState('')

  const { data: zones } = useQuery({
    queryKey: ['privacy-zones'],
    queryFn: listPrivacyZones,
    enabled: !!isRoundTrip,
  })

  const createMut = useMutation({
    mutationFn: () =>
      isRoundTrip && selectedZone
        ? createCommuteRule(activityId, { zoneId: selectedZone })
        : createCommuteRule(activityId),
    onSuccess: (result) => {
      addToast({
        type: 'success',
        title: result.created ? t('activityDetail.commute.toasts.ruleCreated') : t('activityDetail.commute.toasts.routeAdded'),
      })
      onClose()
    },
    onError: (error: unknown) => {
      const apiErr = error as { status?: number } | null
      if (apiErr?.status === 409) {
        addToast({ type: 'info', title: t('activityDetail.commute.toasts.routeAlreadyExists') })
        onClose()
        return
      }
      addToast({ type: 'error', title: t('activityDetail.commute.toasts.createFailed'), duration: 0 })
      onClose()
    },
  })

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md">
      {isRoundTrip ? (
        <>
          <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            {t('activityDetail.commute.roundTripTitle')}
          </h3>
          <p className="text-sm text-neutral-600 dark:text-neutral-400 mt-3">
            {t('activityDetail.commute.roundTripDescription')}
          </p>
          {zones && zones.length > 0 ? (
            <select
              value={selectedZone}
              onChange={e => setSelectedZone(e.target.value)}
              className="mt-3 w-full px-3 py-2 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
            >
              <option value="">{t('activityDetail.commute.chooseZone')}</option>
              {zones.map(z => (
                <option key={z.id} value={z.id}>{z.name}</option>
              ))}
            </select>
          ) : (
            <>
              <p className="mt-3 text-sm text-neutral-500 dark:text-neutral-400">
                {t('activityDetail.commute.createZonesPrefix')}{' '}
                <a href="/settings/privacy" className="underline hover:text-neutral-700 dark:hover:text-neutral-300">
                  {t('activityDetail.commute.privacySettingsLink')}
                </a>
              </p>
              <button
                onClick={onClose}
                className="w-full mt-4 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 rounded-lg cursor-pointer transition-colors"
              >
                {t('common.cancel')}
              </button>
            </>
          )}
          {zones && zones.length > 0 && (
            <div className="flex gap-3 mt-4">
              <button
                onClick={() => createMut.mutate()}
                disabled={!selectedZone || createMut.isPending}
                className="flex-1 px-4 py-2 text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 rounded-lg cursor-pointer transition-colors disabled:opacity-50"
              >
                {createMut.isPending ? t('activityDetail.commute.creating') : t('activityDetail.commute.createRule')}
              </button>
              <button
                onClick={onClose}
                className="flex-1 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 rounded-lg cursor-pointer transition-colors"
              >
                {t('common.cancel')}
              </button>
            </div>
          )}
        </>
      ) : (
        <>
          <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            {t('activityDetail.commute.routeTitle')}
          </h3>
          <p className="text-sm text-neutral-600 dark:text-neutral-400 mt-3">
            {t('activityDetail.commute.routeDescription')}
          </p>
          <div className="flex gap-3 mt-4">
            <button
              onClick={() => createMut.mutate()}
              disabled={createMut.isPending}
              className="flex-1 px-4 py-2 text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 rounded-lg cursor-pointer transition-colors disabled:opacity-50"
            >
              {createMut.isPending ? t('activityDetail.commute.creating') : t('activityDetail.commute.autoTag')}
            </button>
            <button
              onClick={onClose}
              className="flex-1 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 rounded-lg cursor-pointer transition-colors"
            >
              {t('activityDetail.commute.onlyThisTime')}
            </button>
          </div>
          <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-3">
            {t('activityDetail.commute.rulesHintPrefix')}{' '}
            <a href="/workshop/rules" className="underline hover:text-neutral-600 dark:hover:text-neutral-300">
              {t('activityDetail.commute.rulesLink')}
            </a>{' '}
            {t('activityDetail.commute.rulesHintSuffix')}
          </p>
        </>
      )}
    </ModalShell>
  )
}
