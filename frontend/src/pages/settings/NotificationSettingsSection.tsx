import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
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

const NOTIFICATION_TOGGLES = [
  { key: 'notify_enrichment' as NotifyKey, label: 'Enrichment abgeschlossen', description: 'Nach jeder automatischen Analyse' },
  { key: 'notify_anomaly' as NotifyKey,    label: 'Anomalie erkannt',         description: 'Bei ungewöhnlichen Trainingswerten' },
  { key: 'notify_gear' as NotifyKey,       label: 'Ausrüstungs-Alerts',       description: 'Wartungshinweise und Verschleiß' },
  { key: 'notify_chat' as NotifyKey,       label: 'Assistent-Antworten',      description: 'Wenn der KI-Assistent antwortet' },
] as const

export function NotificationSettingsSection() {
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
      addToast({ type: 'error', title: 'Benachrichtigungseinstellung konnte nicht gespeichert werden', duration: 0 })
    },
  })

  const removeDeviceMutation = useMutation({
    mutationFn: (id: string) => removeDevice(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Gerät konnte nicht entfernt werden', duration: 0 })
    },
  })

  function toggleNotify(key: NotifyKey, value: boolean) {
    toggleNotifyMutation.mutate({ key, value })
  }

  function handleRemoveDevice(id: string) {
    removeDeviceMutation.mutate(id)
  }

  return (
    <SettingsSection title="Benachrichtigungen" description="Welche Ereignisse sollen als Benachrichtigung erscheinen.">

      <div className="space-y-2">
        {NOTIFICATION_TOGGLES.map(({ key, label, description }) => {
          const enabled = userSettings?.[key] ?? false
          return (
            <div key={key} className="flex items-center justify-between gap-4 py-1">
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{label}</span>
                <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-2">— {description}</span>
              </div>
              <Toggle
                checked={enabled}
                onChange={() => toggleNotify(key, !enabled)}
                aria-label={`${label} aktivieren`}
              />
            </div>
          )
        })}
      </div>

      {devices && devices.length > 0 && (
        <div className="pt-3 border-t border-neutral-100 dark:border-neutral-800/60 space-y-2">
          <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
            Registrierte Ger&auml;te
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
                    {device.device_name ?? (device.platform === 'ios' ? 'iPhone/iPad' : 'Android-Gerät')}
                  </span>
                  <span className="text-xs text-neutral-400 flex-shrink-0">
                    {new Date(device.created_at).toLocaleDateString('de-DE')}
                  </span>
                </div>
                <button
                  onClick={() => handleRemoveDevice(device.id)}
                  className="text-xs text-red-500 hover:text-red-600 dark:hover:text-red-400 cursor-pointer transition-colors flex-shrink-0"
                >
                  Entfernen
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </SettingsSection>
  )
}
