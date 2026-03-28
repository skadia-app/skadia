import { useState } from 'react'
import { useQueryClient, useMutation } from '@tanstack/react-query'
import type { Activity } from '@/api/types'
import { updateActivity } from '@/api/activities'

interface VisibilitySettingsProps {
  activity: Activity
}

export function VisibilitySettings({ activity }: VisibilitySettingsProps) {
  const queryClient = useQueryClient()

  const visibilityMut = useMutation({
    mutationFn: (visibility: Activity['visibility']) =>
      updateActivity(activity.id, { visibility }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activity.id], updated)
    },
  })

  // Stub state for fields not yet in API
  const [hideMap, setHideMap] = useState(false)
  const [dataVis, setDataVis] = useState({
    speed: true,
    heart_rate: true,
    power: true,
    cadence: true,
    temperature: true,
    altitude: true,
  })

  function toggleDataVis(key: keyof typeof dataVis) {
    setDataVis((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  const dataVisOptions: { key: keyof typeof dataVis; label: string }[] = [
    { key: 'speed', label: 'Geschwindigkeit' },
    { key: 'heart_rate', label: 'Herzfrequenz' },
    { key: 'power', label: 'Leistung' },
    { key: 'cadence', label: 'Kadenz' },
    { key: 'temperature', label: 'Temperatur' },
    { key: 'altitude', label: 'Höhe' },
  ]

  const visibilityOptions: { value: Activity['visibility']; label: string }[] = [
    { value: 'private', label: 'Privat' },
    { value: 'bekannte', label: 'Bekannte' },
    { value: 'instanz', label: 'Instanz' },
    { value: 'federation', label: 'Federation' },
    { value: 'public', label: 'Öffentlich' },
  ]

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Sichtbarkeit</h2>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {/* Visibility dropdown */}
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Wer kann diese Aktivität sehen?
          </label>
          <select
            value={activity.visibility}
            onChange={(e) => visibilityMut.mutate(e.target.value as Activity['visibility'])}
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          >
            {visibilityOptions.map(({ value, label }) => (
              <option key={value} value={value}>{label}</option>
            ))}
          </select>
        </div>

        {/* Hide map toggle */}
        <div>
          <label className="flex items-center gap-3 cursor-pointer mt-6">
            <input
              type="checkbox"
              checked={hideMap}
              onChange={() => setHideMap(!hideMap)}
              className="w-4 h-4 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
            />
            <span className="text-sm text-neutral-700 dark:text-neutral-300">
              Karte ausblenden
            </span>
          </label>
          <p className="text-xs text-neutral-400 mt-1 ml-7">
            Karte wird für andere Nutzer nicht angezeigt
          </p>
        </div>
      </div>

      {/* Data visibility checkboxes */}
      <div>
        <div className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          Sichtbare Daten
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          {dataVisOptions.map(({ key, label }) => (
            <label key={key} className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={dataVis[key]}
                onChange={() => toggleDataVis(key)}
                className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
              />
              <span className="text-sm text-neutral-700 dark:text-neutral-300">{label}</span>
            </label>
          ))}
        </div>
        <p className="text-xs text-neutral-400 mt-2">
          Nicht ausgewählte Daten werden für andere Nutzer ausgeblendet.
        </p>
      </div>
    </div>
  )
}
