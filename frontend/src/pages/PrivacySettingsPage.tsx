import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listPrivacyZones, deletePrivacyZone, updatePrivacyZone, type PrivacyZone } from '@/api/privacy'
import PrivacyZoneEditor from '@/components/PrivacyZoneEditor'

export default function PrivacySettingsPage() {
  const queryClient = useQueryClient()
  const [editingZone, setEditingZone] = useState<PrivacyZone | null>(null)
  const [showEditor, setShowEditor] = useState(false)

  const { data: zones, isLoading } = useQuery({
    queryKey: ['privacy-zones'],
    queryFn: listPrivacyZones,
  })

  const deleteMutation = useMutation({
    mutationFn: deletePrivacyZone,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['privacy-zones'] })
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, is_active }: { id: string; is_active: boolean }) =>
      updatePrivacyZone(id, { is_active }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['privacy-zones'] })
    },
  })

  function handleEdit(zone: PrivacyZone) {
    setEditingZone(zone)
    setShowEditor(true)
  }

  function handleNew() {
    setEditingZone(null)
    setShowEditor(true)
  }

  function handleEditorClose() {
    setShowEditor(false)
    setEditingZone(null)
  }

  function handleDelete(zone: PrivacyZone) {
    if (confirm(`"${zone.name}" wirklich löschen?`)) {
      deleteMutation.mutate(zone.id)
    }
  }

  if (showEditor) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          {editingZone ? 'Privacy-Zone bearbeiten' : 'Neue Privacy-Zone'}
        </h1>
        <PrivacyZoneEditor zone={editingZone} onClose={handleEditorClose} />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Privacy-Zonen
        </h1>
        <button
          onClick={handleNew}
          className="bg-orange-500 hover:bg-orange-600 text-white rounded-lg px-4 py-2 text-sm font-medium transition-colors"
        >
          Neue Zone
        </button>
      </div>

      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        GPS-Daten innerhalb aktiver Privacy-Zonen werden in öffentlichen Ansichten ausgeblendet.
        Damit schuetzt du sensible Orte wie dein Zuhause oder deinen Arbeitsplatz.
      </p>

      {isLoading ? (
        <div className="flex items-center justify-center h-32 text-neutral-400">
          Lade Privacy-Zonen...
        </div>
      ) : !zones || zones.length === 0 ? (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-8 text-center">
          <p className="text-neutral-500 dark:text-neutral-400 mb-4">
            Noch keine Privacy-Zonen definiert.
          </p>
          <button
            onClick={handleNew}
            className="text-sm text-orange-500 hover:text-orange-600 font-medium transition-colors"
          >
            Erste Zone erstellen
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          {zones.map((zone) => (
            <div
              key={zone.id}
              className="flex items-center gap-4 rounded-xl border border-neutral-200 dark:border-neutral-800 p-4"
            >
              {/* Toggle active */}
              <button
                onClick={() => toggleMutation.mutate({ id: zone.id, is_active: !zone.is_active })}
                className={`relative w-10 h-6 rounded-full transition-colors flex-shrink-0 ${
                  zone.is_active
                    ? 'bg-orange-500'
                    : 'bg-neutral-300 dark:bg-neutral-600'
                }`}
                title={zone.is_active ? 'Deaktivieren' : 'Aktivieren'}
              >
                <span
                  className={`absolute top-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform ${
                    zone.is_active ? 'translate-x-[18px]' : 'translate-x-0.5'
                  }`}
                />
              </button>

              {/* Zone info */}
              <div className="flex-1 min-w-0">
                <div className="font-medium text-neutral-900 dark:text-neutral-100 truncate">
                  {zone.name}
                </div>
                <div className="text-sm text-neutral-500 dark:text-neutral-400">
                  Radius: {zone.radius_m} m
                  <span className="mx-2 text-neutral-300 dark:text-neutral-600">|</span>
                  {zone.is_active ? 'Aktiv' : 'Inaktiv'}
                </div>
              </div>

              {/* Actions */}
              <div className="flex items-center gap-2">
                <button
                  onClick={() => handleEdit(zone)}
                  className="p-2 rounded-lg text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
                  title="Bearbeiten"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
                    <path d="M2.695 14.763l-1.262 3.154a.5.5 0 00.65.65l3.155-1.262a4 4 0 001.343-.885L17.5 5.5a2.121 2.121 0 00-3-3L3.58 13.42a4 4 0 00-.885 1.343z" />
                  </svg>
                </button>
                <button
                  onClick={() => handleDelete(zone)}
                  className="p-2 rounded-lg text-neutral-400 hover:text-red-500 dark:hover:text-red-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
                  title="Löschen"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
                    <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4zM8.58 7.72a.75.75 0 01.78.72l.5 6.5a.75.75 0 01-1.49.12l-.5-6.5a.75.75 0 01.71-.84zm3.62.72a.75.75 0 10-1.49-.12l-.5 6.5a.75.75 0 101.49.12l.5-6.5z" clipRule="evenodd" />
                  </svg>
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
