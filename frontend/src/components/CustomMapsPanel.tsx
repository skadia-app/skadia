import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listCustomMaps,
  createCustomMap,
  updateCustomMap,
  deleteCustomMap,
  listCustomMapMarkers,
  deleteCustomMapMarker,
  updateCustomMapMarker,
  uploadMarkerImage,
  type CustomMap,
  type CustomMapMarker,
} from '@/api/customMaps'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'
import ColorPicker from '@/components/ColorPicker'
import { ModalShell } from '@/components/ui/ModalShell'

interface Props {
  onAddMarkerMode: (mapID: string | null) => void
  addMarkerModeMapID: string | null
  onClose: () => void
}

export default function CustomMapsPanel({ onAddMarkerMode, addMarkerModeMapID, onClose }: Props) {
  const queryClient = useQueryClient()
  const { openModal } = useModal()
  const { addToast } = useToast()
  const [collapsed, setCollapsed] = useState(false)
  const [editingMarker, setEditingMarker] = useState<CustomMapMarker | null>(null)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editColor, setEditColor] = useState<string>('')
  const [editSaving, setEditSaving] = useState(false)
  const [expandedMapID, setExpandedMapID] = useState<string | null>(null)
  const [showNewForm, setShowNewForm] = useState(false)
  const [newName, setNewName] = useState('')
  const [newColor, setNewColor] = useState('#F97316')

  const { data: maps = [] } = useQuery({
    queryKey: ['custom-maps'],
    queryFn: listCustomMaps,
  })

  const { data: expandedMarkers = [] } = useQuery({
    queryKey: ['custom-map-markers', expandedMapID],
    queryFn: () => listCustomMapMarkers(expandedMapID!),
    enabled: !!expandedMapID,
  })

  const createMapMutation = useMutation({
    mutationFn: (req: { name: string; color: string }) => createCustomMap(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['custom-maps'] })
      setShowNewForm(false)
      setNewName('')
      setNewColor('#F97316')
    },
  })

  const toggleVisibilityMutation = useMutation({
    mutationFn: ({ id, is_visible }: { id: string; is_visible: boolean }) =>
      updateCustomMap(id, { is_visible }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['custom-maps'] })
      queryClient.invalidateQueries({ queryKey: ['all-custom-map-markers'] })
    },
  })

  const deleteMapMutation = useMutation({
    mutationFn: (id: string) => deleteCustomMap(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['custom-maps'] })
      queryClient.invalidateQueries({ queryKey: ['all-custom-map-markers'] })
    },
  })

  const deleteMarkerMutation = useMutation({
    mutationFn: ({ mapID, markerID }: { mapID: string; markerID: string }) =>
      deleteCustomMapMarker(mapID, markerID),
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ['custom-map-markers', vars.mapID] })
      queryClient.invalidateQueries({ queryKey: ['custom-maps'] })
      queryClient.invalidateQueries({ queryKey: ['all-custom-map-markers'] })
    },
  })

  function handleDeleteMap(map: CustomMap) {
    openModal({
      title: `Karte "${map.name}" löschen?`,
      message: 'Löschen ist nicht rückgängig zu machen.',
      buttons: [
        {
          label: 'Abbrechen',
          variant: 'default',
          onClick: () => {}
        },
        {
          label: 'Löschen',
          variant: 'destructive',
          onClick: async () => {
            deleteMapMutation.mutate(map.id)
            if (expandedMapID === map.id) setExpandedMapID(null)
          }
        }
      ]
    })
  }

  function handleDeleteMarker(marker: CustomMapMarker) {
    openModal({
      title: `Marker "${marker.name}" löschen?`,
      message: 'Löschen ist nicht rückgängig zu machen.',
      buttons: [
        {
          label: 'Abbrechen',
          variant: 'default',
          onClick: () => {}
        },
        {
          label: 'Löschen',
          variant: 'destructive',
          onClick: async () => {
            deleteMarkerMutation.mutate({ mapID: marker.map_id, markerID: marker.id })
          }
        }
      ]
    })
  }

  return (
    <div className="absolute top-4 right-[17.5rem] z-10 w-64">
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-lg overflow-hidden">
        {/* Header */}
        <div className="w-full flex items-center justify-between px-4 py-2.5 text-sm font-medium text-neutral-900 dark:text-neutral-100 border-b border-neutral-100 dark:border-neutral-800">
          <button
            onClick={() => setCollapsed(!collapsed)}
            className="flex-1 flex items-center justify-between hover:text-accent-DEFAULT cursor-pointer"
          >
            <span>Eigene Karten</span>
            <svg className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '-rotate-90' : ''}`} viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
            </svg>
          </button>
          <button
            onClick={onClose}
            className="ml-2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer flex-shrink-0"
            title="Schliessen"
          >
            <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        </div>

        {!collapsed && (
          <div className="px-3 pb-3 space-y-2 border-t border-neutral-100 dark:border-neutral-800 pt-2 max-h-96 overflow-y-auto">
            {maps.map((map) => (
              <div key={map.id} className="space-y-1">
                <div className="flex items-center gap-1.5">
                  {/* Color dot */}
                  <span
                    className="w-3 h-3 rounded-full flex-shrink-0"
                    style={{ backgroundColor: map.color }}
                  />
                  {/* Name + count */}
                  <button
                    onClick={() => setExpandedMapID(expandedMapID === map.id ? null : map.id)}
                    className="flex-1 text-left text-sm text-neutral-700 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-neutral-100 truncate cursor-pointer"
                  >
                    {map.name}
                    <span className="ml-1 text-xs text-neutral-400">({map.marker_count})</span>
                  </button>
                  {/* Visibility toggle */}
                  <button
                    onClick={() => toggleVisibilityMutation.mutate({ id: map.id, is_visible: !map.is_visible })}
                    className="p-0.5 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
                    title={map.is_visible ? 'Ausblenden' : 'Einblenden'}
                  >
                    {map.is_visible ? (
                      <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                        <path d="M10 12.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5z" />
                        <path fillRule="evenodd" d="M.664 10.59a1.651 1.651 0 010-1.186A10.004 10.004 0 0110 3c4.257 0 7.893 2.66 9.336 6.41.147.381.146.804 0 1.186A10.004 10.004 0 0110 17c-4.257 0-7.893-2.66-9.336-6.41zM14 10a4 4 0 11-8 0 4 4 0 018 0z" clipRule="evenodd" />
                      </svg>
                    ) : (
                      <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                        <path fillRule="evenodd" d="M3.28 2.22a.75.75 0 00-1.06 1.06l14.5 14.5a.75.75 0 101.06-1.06l-1.745-1.745a10.029 10.029 0 003.3-4.38 1.651 1.651 0 000-1.186A10.004 10.004 0 0010 3c-1.67 0-3.243.41-4.63 1.13L3.28 2.22zM7.25 5.88l1.411 1.41A4 4 0 0114 10c0 .441-.072.867-.205 1.264l1.504 1.504A8.474 8.474 0 0017.59 10 8.508 8.508 0 0010 4.5c-.96 0-1.882.16-2.75.463zM2.41 10a8.507 8.507 0 005.67 4.927l-1.66-1.66A4 4 0 018.34 7.34l-1.83-1.83A8.476 8.476 0 002.41 10z" clipRule="evenodd" />
                      </svg>
                    )}
                  </button>
                  {/* Delete map */}
                  <button
                    onClick={() => handleDeleteMap(map)}
                    className="p-0.5 text-neutral-300 hover:text-red-500 dark:text-neutral-600 dark:hover:text-red-400 cursor-pointer"
                    title="Karte löschen"
                  >
                    <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                      <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM12.5 3.75a1.25 1.25 0 00-1.25-1.25h-2.5a1.25 1.25 0 00-1.25 1.25v.19c1.474-.085 2.963-.085 4.437 0v-.19z" clipRule="evenodd" />
                    </svg>
                  </button>
                </div>

                {/* Expanded: show markers */}
                {expandedMapID === map.id && (
                  <div className="ml-4 space-y-1">
                    {expandedMarkers.map((marker) => (
                      <div key={marker.id} className="flex items-center gap-1.5 text-xs text-neutral-500 dark:text-neutral-400">
                        <span
                          className="w-2 h-2 rounded-full flex-shrink-0"
                          style={{ backgroundColor: marker.color ?? map.color }}
                        />
                        <button
                          className="flex-1 truncate text-left hover:text-accent-DEFAULT cursor-pointer"
                          onClick={() => {
                            setEditingMarker(marker)
                            setEditName(marker.name)
                            setEditDescription(marker.description ?? '')
                            setEditColor(marker.color ?? map.color)
                          }}
                        >
                          {marker.name}
                        </button>
                        <button
                          onClick={() => handleDeleteMarker(marker)}
                          className="p-0.5 text-neutral-300 hover:text-red-500 dark:text-neutral-600 dark:hover:text-red-400 cursor-pointer"
                          title="Marker löschen"
                        >
                          <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
                            <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                          </svg>
                        </button>
                      </div>
                    ))}
                    <button
                      onClick={() => onAddMarkerMode(addMarkerModeMapID === map.id ? null : map.id)}
                      className={`w-full text-left text-xs px-2 py-1 rounded cursor-pointer transition-colors ${
                        addMarkerModeMapID === map.id
                          ? 'bg-accent-100 dark:bg-orange-900/30 text-accent-600 dark:text-orange-400'
                          : 'text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                      }`}
                    >
                      {addMarkerModeMapID === map.id ? '+ Klick auf Karte...' : '+ Marker hinzufügen'}
                    </button>
                  </div>
                )}
              </div>
            ))}

            {/* New map form */}
            {showNewForm ? (
              <div className="space-y-2 pt-1">
                <input
                  type="text"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder="Kartenname"
                  className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
                  autoFocus
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && newName.trim()) {
                      createMapMutation.mutate({ name: newName.trim(), color: newColor })
                    } else if (e.key === 'Escape') {
                      setShowNewForm(false)
                    }
                  }}
                />
                <div className="flex items-center gap-2">
                  <ColorPicker value={newColor} onChange={setNewColor} />
                  <button
                    onClick={() => {
                      if (newName.trim()) {
                        createMapMutation.mutate({ name: newName.trim(), color: newColor })
                      }
                    }}
                    disabled={!newName.trim() || createMapMutation.isPending}
                    className="flex-1 text-xs px-2 py-1 rounded bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
                  >
                    Erstellen
                  </button>
                  <button
                    onClick={() => setShowNewForm(false)}
                    className="text-xs px-2 py-1 rounded text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer"
                  >
                    Abbrechen
                  </button>
                </div>
              </div>
            ) : (
              <button
                onClick={() => setShowNewForm(true)}
                className="w-full text-left text-xs text-neutral-400 hover:text-accent-DEFAULT cursor-pointer px-1 py-1"
              >
                + Neue Karte
              </button>
            )}
          </div>
        )}
      </div>

      {/* Marker Edit Modal */}
      {editingMarker && (
        <ModalShell onClose={() => setEditingMarker(null)} maxWidth="w-72" className="p-4 space-y-3">
              <div className="text-xs text-neutral-400 font-mono">
                {editingMarker.lat.toFixed(5)}, {editingMarker.lng.toFixed(5)}
              </div>

              {editingMarker.image_url ? (
                <div className="relative">
                  <img src={editingMarker.image_url} alt="" className="w-full h-32 object-cover rounded-lg" />
                  <button
                    className="absolute top-1 right-1 bg-black/50 text-white rounded-full w-5 h-5 text-xs flex items-center justify-center cursor-pointer"
                    onClick={async () => {
                      await updateCustomMapMarker(editingMarker.map_id, editingMarker.id, { image_url: '' })
                      queryClient.invalidateQueries({ queryKey: ['custom-map-markers', editingMarker.map_id] })
                      queryClient.invalidateQueries({ queryKey: ['all-custom-map-markers'] })
                      setEditingMarker({ ...editingMarker, image_url: undefined })
                    }}
                  >
                    <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
                      <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                    </svg>
                  </button>
                </div>
              ) : null}

              <input
                type="text"
                value={editName}
                onChange={e => setEditName(e.target.value)}
                placeholder="Name"
                className="w-full px-2.5 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
              />

              <textarea
                value={editDescription}
                onChange={e => setEditDescription(e.target.value)}
                placeholder="Beschreibung (optional)"
                rows={2}
                className="w-full px-2.5 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400 resize-none"
              />

              <div className="flex items-center gap-2">
                <span className="text-xs text-neutral-500 dark:text-neutral-400">Farbe:</span>
                <ColorPicker value={editColor} onChange={setEditColor} />
              </div>

              <label className="flex items-center gap-1.5 text-xs text-neutral-500 hover:text-accent-DEFAULT cursor-pointer">
                <input
                  type="file"
                  accept="image/*"
                  className="hidden"
                  onChange={async e => {
                    const file = e.target.files?.[0]
                    if (!file) return
                    try {
                      const updated = await uploadMarkerImage(editingMarker.map_id, editingMarker.id, file)
                      queryClient.invalidateQueries({ queryKey: ['custom-map-markers', editingMarker.map_id] })
                      queryClient.invalidateQueries({ queryKey: ['all-custom-map-markers'] })
                      setEditingMarker(updated)
                    } catch {
                      addToast({ type: 'error', title: 'Bild-Upload fehlgeschlagen' })
                    }
                  }}
                />
                <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M1 8a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 018.07 3h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0016.07 6H17a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V8zm13.5 3a4.5 4.5 0 11-9 0 4.5 4.5 0 019 0zM10 14a2.5 2.5 0 100-5 2.5 2.5 0 000 5z" clipRule="evenodd"/>
                </svg>
                {editingMarker.image_url ? 'Bild ersetzen' : 'Bild hinzufügen'}
              </label>

              <div className="flex gap-2 pt-1">
                <button
                  onClick={async () => {
                    setEditSaving(true)
                    try {
                      await updateCustomMapMarker(editingMarker.map_id, editingMarker.id, {
                        name: editName.trim(),
                        description: editDescription.trim() || undefined,
                        color: editColor,
                      })
                      queryClient.invalidateQueries({ queryKey: ['custom-map-markers', editingMarker.map_id] })
                      queryClient.invalidateQueries({ queryKey: ['all-custom-map-markers'] })
                      setEditingMarker(null)
                    } catch {
                      addToast({ type: 'error', title: 'Speichern fehlgeschlagen' })
                    } finally {
                      setEditSaving(false)
                    }
                  }}
                  disabled={editSaving || !editName.trim()}
                  className="flex-1 text-xs px-3 py-1.5 rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
                >
                  {editSaving ? 'Speichern...' : 'Speichern'}
                </button>
                <button
                  onClick={() => {
                    handleDeleteMarker(editingMarker)
                    setEditingMarker(null)
                  }}
                  className="text-xs px-3 py-1.5 rounded-lg border border-red-200 dark:border-red-800 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 cursor-pointer"
                  title="Marker löschen"
                >
                  <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM12.5 3.75a1.25 1.25 0 00-1.25-1.25h-2.5a1.25 1.25 0 00-1.25 1.25v.19c1.474-.085 2.963-.085 4.437 0v-.19z" clipRule="evenodd" />
                  </svg>
                </button>
              </div>
        </ModalShell>
      )}
    </div>
  )
}
