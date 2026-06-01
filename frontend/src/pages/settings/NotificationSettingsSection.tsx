import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import Toggle from '@/components/Toggle'
import {
  getUserMLSettings,
  updateUserMLSetting,
  getDevices,
  removeDevice,
  type NotifyKey,
} from '@/api/ml'
import { useToast } from '@/lib/toast'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { useUser } from '@/providers/UserProvider'
import { localeForLanguage } from '@/i18n/languages'

const NOTIFICATION_TOGGLES = [
  { key: 'notify_enrichment' as NotifyKey, translationKey: 'enrichment' },
  { key: 'notify_anomaly' as NotifyKey, translationKey: 'anomaly' },
  { key: 'notify_gear' as NotifyKey, translationKey: 'gear' },
  { key: 'notify_chat' as NotifyKey, translationKey: 'chat' },
] as const

export function NotificationSettingsSection() {
  const { t } = useTranslation()
  const { locale } = useUser()
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const { data: userSettings } = useQuery({
    queryKey: ['ml-user-settings'],
    queryFn: getUserMLSettings,
  })

  const { data: devices } = useQuery({
    queryKey: ['devices'],
    queryFn: getDevices,
  })

  const toggleNotifyMutation = useMutation({
    mutationFn: ({ key, value }: { key: NotifyKey; value: boolean }) =>
      updateUserMLSetting(key, value),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ml-user-settings'] })
    },
    onError: () => {
      addToast({ type: 'error', title: t('settings.notifications.saveFailed'), duration: 0 })
    },
  })

  const removeDeviceMutation = useMutation({
    mutationFn: (id: string) => removeDevice(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
    onError: () => {
      addToast({ type: 'error', title: t('settings.notifications.removeDeviceFailed'), duration: 0 })
    },
  })

  function toggleNotify(key: NotifyKey, value: boolean) {
    toggleNotifyMutation.mutate({ key, value })
  }

  function handleRemoveDevice(id: string) {
    removeDeviceMutation.mutate(id)
  }

  return (
    <SettingsSection title={t('settings.notifications.title')} description={t('settings.notifications.description')}>

      <div className="space-y-2">
        {NOTIFICATION_TOGGLES.map(({ key, translationKey }) => {
          const enabled = userSettings?.[key] ?? false
          const label = t(`settings.notifications.toggles.${translationKey}.label`)
          const description = t(`settings.notifications.toggles.${translationKey}.description`)
          return (
            <div key={key} className="flex items-center justify-between gap-4 py-1">
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{label}</span>
                <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-2">— {description}</span>
              </div>
              <Toggle
                checked={enabled}
                onChange={() => toggleNotify(key, !enabled)}
                aria-label={t('settings.notifications.toggleAria', { label })}
              />
            </div>
          )
        })}
      </div>

      {devices && devices.length > 0 && (
        <div className="pt-3 border-t border-neutral-100 dark:border-neutral-800/60 space-y-2">
          <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
            {t('settings.notifications.registeredDevices')}
          </p>
          <div className="space-y-1.5">
            {devices.map(device => (
              <div
                key={device.id}
                className="flex items-center justify-between gap-3 py-1.5 px-3 rounded-lg bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-100 dark:border-neutral-800"
              >
                <div className="flex items-center gap-2 min-w-0">
                  <span className="text-sm">{device.platform === 'ios' ? '🍎' : '🤖'}</span>
                  <span className="text-sm text-neutral-800 dark:text-neutral-200 truncate">
                    {device.device_name ?? (device.platform === 'ios' ? 'iPhone/iPad' : t('settings.notifications.androidDevice'))}
                  </span>
                  <span className="text-xs text-neutral-400 flex-shrink-0">
                    {new Date(device.created_at).toLocaleDateString(localeForLanguage(locale))}
                  </span>
                </div>
                <button
                  onClick={() => handleRemoveDevice(device.id)}
                  className="text-xs text-red-500 hover:text-red-600 dark:hover:text-red-400 cursor-pointer transition-colors flex-shrink-0"
                >
                  {t('settings.notifications.removeDevice')}
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </SettingsSection>
  )
}
