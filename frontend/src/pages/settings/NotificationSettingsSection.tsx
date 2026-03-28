import { useQuery } from '@tanstack/react-query'
import Toggle from '@/components/Toggle'
import {
  getUserMLSettings,
  updateUserMLSetting,
  getDevices,
  removeDevice,
  type NotifyKey,
} from '@/api/ml'

const NOTIFICATION_TOGGLES = [
  { key: 'notify_enrichment' as NotifyKey, label: 'Enrichment abgeschlossen', description: 'Nach jeder automatischen Analyse' },
  { key: 'notify_anomaly' as NotifyKey,    label: 'Anomalie erkannt',         description: 'Bei ungewöhnlichen Trainingswerten' },
  { key: 'notify_gear' as NotifyKey,       label: 'Ausrüstungs-Alerts',       description: 'Wartungshinweise und Verschleiß' },
  { key: 'notify_chat' as NotifyKey,       label: 'Assistent-Antworten',      description: 'Wenn der KI-Assistent antwortet' },
] as const

export function NotificationSettingsSection() {
  const { data: userSettings, refetch: refetchUser } = useQuery({
    queryKey: ['ml-user-settings'],
    queryFn: getUserMLSettings,
  })

  const { data: devices, refetch: refetchDevices } = useQuery({
    queryKey: ['devices'],
    queryFn: getDevices,
  })

  async function toggleNotify(key: NotifyKey, value: boolean) {
    await updateUserMLSetting(key, value)
    refetchUser()
  }

  async function handleRemoveDevice(id: string) {
    await removeDevice(id)
    refetchDevices()
  }

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Benachrichtigungen</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Welche Ereignisse sollen als Benachrichtigung erscheinen.
        </p>
      </div>

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
    </section>
  )
}
