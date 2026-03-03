import { useState, useMemo, useRef } from 'react'
import { createPortal } from 'react-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchActivities } from '@/api/activities'
import { updateActivity } from '@/api/activities'
import { createTag } from '@/api/tags'
import type { Tag } from '@/api/tags'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { sportLabel } from '@/lib/utils'

interface Props {
  locationTags: Tag[]
  onClose: () => void
  onPickLocation: (name: string, color: string, activityIDs: string[]) => void
  onAssigned: () => void
}

const colorPalette = [
  '#f97316', '#3b82f6', '#10b981', '#8b5cf6',
  '#f43f5e', '#06b6d4', '#eab308', '#6b7280',
]

export default function IndoorAssignPanel({ locationTags, onClose, onPickLocation, onAssigned }: Props) {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [sportFilter, setSportFilter] = useState('')
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [selectedTagId, setSelectedTagId] = useState<string>('')
  const [showNewLocation, setShowNewLocation] = useState(false)
  const [newLocName, setNewLocName] = useState('')
  const [newLocColor, setNewLocColor] = useState('#f97316')
  const colorInputRef = useRef<HTMLInputElement>(null)

  // Fetch indoor activities (no GPS)
  const { data: activitiesData, isLoading } = useQuery({
    queryKey: ['indoor-activities'],
    queryFn: () => fetchActivities(undefined, 100, undefined, undefined, { hasGPS: false }),
  })
  const activities = activitiesData?.activities ?? []

  // Client-side filtering
  const filtered = useMemo(() => {
    return activities.filter(act => {
      const q = search.toLowerCase()
      const matchSearch = !q || (
        (act.title ?? '').toLowerCase().includes(q) ||
        (act.description ?? '').toLowerCase().includes(q)
      )
      const matchSport = !sportFilter || act.sport_type === sportFilter
      return matchSearch && matchSport
    })
  }, [activities, search, sportFilter])

  // Unique sport types for dropdown
  const sportTypes = useMemo(() => {
    const types = new Set(activities.map(a => a.sport_type).filter(Boolean))
    return Array.from(types).sort()
  }, [activities])

  function toggleActivity(id: string) {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function toggleAll() {
    if (selectedIds.size === filtered.length) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(filtered.map(a => a.id)))
    }
  }

  const assignMutation = useMutation({
    mutationFn: async () => {
      const tag = locationTags.find(t => t.id === selectedTagId)
      if (!tag) throw new Error('Kein Standort ausgewählt')
      const actIds = Array.from(selectedIds)
      await Promise.all(actIds.map(async (actId) => {
        const act = activities.find(a => a.id === actId)
        const currentTags = act?.tags ?? []
        if (!currentTags.includes(tag.name)) {
          await updateActivity(actId, { tags: [...currentTags, tag.name] })
        }
      }))
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      queryClient.invalidateQueries({ queryKey: ['indoor-activities'] })
      onAssigned()
      setSelectedIds(new Set())
      setSelectedTagId('')
    },
  })

  function handleNewLocationSubmit() {
    if (!newLocName.trim()) return
    const actIds = Array.from(selectedIds)
    onPickLocation(newLocName.trim(), newLocColor, actIds)
  }

  function handleCreateLocationOnly() {
    if (!newLocName.trim()) return
    createTag({ name: newLocName.trim(), color: newLocColor })
      .then(() => {
        queryClient.invalidateQueries({ queryKey: ['tags'] })
        setShowNewLocation(false)
        setNewLocName('')
      })
  }

  const hasSelection = selectedIds.size > 0

  function handleDragStart(e: React.DragEvent) {
    if (!hasSelection) {
      e.preventDefault()
      return
    }
    e.dataTransfer.setData('application/skadia-activities', JSON.stringify(Array.from(selectedIds)))
    e.dataTransfer.effectAllowed = 'copy'
  }

  return (
    <div className="absolute top-4 right-[17.5rem] z-10 w-72 max-h-[calc(100vh-6rem)] flex flex-col rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-lg overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-neutral-100 dark:border-neutral-800">
        <span className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">Indoor-Aktivitäten</span>
        <button onClick={onClose} className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer">
          <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
          </svg>
        </button>
      </div>

      {/* Action bar */}
      <div className="px-3 py-2 border-b border-neutral-100 dark:border-neutral-800 flex items-center gap-2">
        <button
          onClick={() => setShowNewLocation(true)}
          className="px-2.5 py-1.5 text-xs text-orange-500 border border-orange-200 dark:border-orange-800 rounded-lg hover:bg-orange-50 dark:hover:bg-orange-900/20 cursor-pointer whitespace-nowrap"
        >
          + Neu
        </button>
        {hasSelection && (
          <span className="text-xs text-neutral-500 dark:text-neutral-400 flex-1 truncate">
            {selectedIds.size} ausgew.
          </span>
        )}
        <div
          draggable={hasSelection}
          onDragStart={handleDragStart}
          className={`flex items-center gap-1 px-2 py-1.5 text-xs rounded-lg border transition-colors select-none ${
            hasSelection
              ? 'border-orange-300 dark:border-orange-700 text-orange-600 dark:text-orange-400 cursor-grab active:cursor-grabbing hover:bg-orange-50 dark:hover:bg-orange-900/20'
              : 'border-neutral-200 dark:border-neutral-700 text-neutral-300 dark:text-neutral-600 cursor-not-allowed'
          }`}
          title={hasSelection ? 'Auf die Karte ziehen' : 'Erst Aktivitäten auswählen'}
        >
          <svg className="w-3.5 h-3.5" viewBox="0 0 16 16" fill="currentColor">
            <circle cx="5" cy="3" r="1.2" />
            <circle cx="11" cy="3" r="1.2" />
            <circle cx="5" cy="8" r="1.2" />
            <circle cx="11" cy="8" r="1.2" />
            <circle cx="5" cy="13" r="1.2" />
            <circle cx="11" cy="13" r="1.2" />
          </svg>
          <span>Karte</span>
        </div>
      </div>

      {/* Drag hint */}
      {!hasSelection && (
        <p className="px-3 py-1.5 text-xs text-neutral-400 dark:text-neutral-500 text-center border-b border-neutral-100 dark:border-neutral-800">
          Aktivitäten wählen und auf die Karte ziehen
        </p>
      )}

      {/* Search + Sport filter */}
      <div className="px-3 py-2 space-y-2 border-b border-neutral-100 dark:border-neutral-800">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Titel / Beschreibung suchen..."
          className="w-full px-2.5 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400"
        />
        <select
          value={sportFilter}
          onChange={(e) => setSportFilter(e.target.value)}
          className="w-full px-2.5 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400"
        >
          <option value="">Alle Sportarten</option>
          {sportTypes.map(s => (
            <option key={s} value={s}>{sportLabel(s)}</option>
          ))}
        </select>
      </div>

      {/* Activity list */}
      <div className="flex-1 overflow-y-auto">
        {isLoading ? (
          <div className="py-8 text-center text-xs text-neutral-400">Lade...</div>
        ) : filtered.length === 0 ? (
          <div className="py-8 text-center text-xs text-neutral-400">Keine Indoor-Aktivitäten gefunden</div>
        ) : (
          <>
            <div className="px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800">
              <label className="flex items-center gap-2 cursor-pointer text-xs text-neutral-500 dark:text-neutral-400">
                <input
                  type="checkbox"
                  checked={selectedIds.size === filtered.length && filtered.length > 0}
                  ref={(el) => { if (el) el.indeterminate = selectedIds.size > 0 && selectedIds.size < filtered.length }}
                  onChange={toggleAll}
                  className="w-3 h-3 rounded"
                />
                Alle auswählen ({filtered.length})
              </label>
            </div>
            {filtered.map(act => {
              const dateStr = act.started_at
                ? format(new Date(act.started_at), 'EEE dd.MM.', { locale: de })
                : ''
              const dur = act.elapsed_time_s
                ? `${Math.floor(act.elapsed_time_s / 3600)}h ${Math.floor((act.elapsed_time_s % 3600) / 60)}m`
                : ''
              return (
                <label key={act.id} className="flex items-start gap-2 px-3 py-2 hover:bg-neutral-50 dark:hover:bg-neutral-800/30 cursor-pointer border-b border-neutral-50 dark:border-neutral-800/50 last:border-0">
                  <input
                    type="checkbox"
                    checked={selectedIds.has(act.id)}
                    onChange={() => toggleActivity(act.id)}
                    className="w-3.5 h-3.5 rounded mt-0.5 flex-shrink-0"
                  />
                  <div className="min-w-0 flex-1">
                    <div className="text-xs font-medium text-neutral-800 dark:text-neutral-200 truncate">
                      {act.title || act.sport_type}
                    </div>
                    <div className="text-xs text-neutral-400 flex items-center gap-1.5 mt-0.5 flex-wrap">
                      <span>{sportLabel(act.sport_type)}</span>
                      {dateStr && <span>{dateStr}</span>}
                      {dur && <span>{dur}</span>}
                    </div>
                  </div>
                </label>
              )
            })}
          </>
        )}
      </div>

      {/* Assignment section */}
      <div className="border-t border-neutral-100 dark:border-neutral-800 px-3 py-3 space-y-2">
        {!showNewLocation ? (
          <>
            <select
              value={selectedTagId}
              onChange={(e) => setSelectedTagId(e.target.value)}
              className="w-full px-2.5 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400"
            >
              <option value="">An Standort zuweisen...</option>
              {locationTags.map(t => (
                <option key={t.id} value={t.id}>{t.name}</option>
              ))}
            </select>
            <button
              onClick={() => assignMutation.mutate()}
              disabled={selectedIds.size === 0 || !selectedTagId || assignMutation.isPending}
              className="w-full py-1.5 text-xs font-medium rounded-lg bg-orange-500 text-white hover:bg-orange-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
            >
              {assignMutation.isPending ? 'Zuweisen...' : 'Zuweisen'}
            </button>
          </>
        ) : (
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-neutral-700 dark:text-neutral-300">Neuer Standort</span>
              <button
                onClick={() => setShowNewLocation(false)}
                className="text-xs text-neutral-400 hover:text-neutral-600 cursor-pointer"
              >
                Abbrechen
              </button>
            </div>
            <input
              type="text"
              value={newLocName}
              onChange={(e) => setNewLocName(e.target.value)}
              placeholder="Name des Standorts"
              className="w-full px-2.5 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400"
            />
            <div className="flex items-center gap-1.5 flex-wrap">
              {colorPalette.map(c => (
                <button
                  key={c}
                  type="button"
                  onClick={() => setNewLocColor(c)}
                  className="w-5 h-5 rounded-full border-2 cursor-pointer"
                  style={{ backgroundColor: c, borderColor: newLocColor === c ? '#f97316' : 'transparent', transform: newLocColor === c ? 'scale(1.15)' : 'scale(1)' }}
                />
              ))}
              <button
                type="button"
                onClick={e => {
                  if (colorInputRef.current) {
                    const rect = e.currentTarget.getBoundingClientRect()
                    const pickerW = 220
                    const pickerH = 250
                    let left = rect.left - pickerW - 4
                    if (left < 8) left = rect.right + 4
                    let top = rect.top
                    if (top + pickerH > window.innerHeight - 8) top = window.innerHeight - pickerH - 8
                    if (top < 8) top = 8
                    colorInputRef.current.style.top = `${top}px`
                    colorInputRef.current.style.left = `${left}px`
                  }
                  colorInputRef.current?.click()
                }}
                className="w-5 h-5 rounded-full cursor-pointer border-0 overflow-hidden"
                style={{ background: 'conic-gradient(red, yellow, lime, aqua, blue, magenta, red)' }}
                title="Eigene Farbe"
              />
              {createPortal(
                <input
                  ref={colorInputRef}
                  type="color"
                  value={newLocColor}
                  onChange={(e) => setNewLocColor(e.target.value)}
                  style={{ position: 'fixed', top: 0, left: 0, width: 1, height: 1, opacity: 0, pointerEvents: 'none' }}
                  tabIndex={-1}
                />,
                document.body,
              )}
            </div>
            <div className="flex gap-2">
              <button
                onClick={handleNewLocationSubmit}
                disabled={!newLocName.trim()}
                className="flex-1 py-1.5 text-xs font-medium rounded-lg bg-orange-500 text-white hover:bg-orange-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
              >
                {selectedIds.size > 0 ? 'Auf Karte setzen' : 'Erstellen'}
              </button>
              {selectedIds.size === 0 && (
                <button
                  onClick={handleCreateLocationOnly}
                  disabled={!newLocName.trim()}
                  className="px-2.5 py-1.5 text-xs border border-neutral-200 dark:border-neutral-700 rounded-lg hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-40 cursor-pointer"
                >
                  Nur erstellen
                </button>
              )}
            </div>
            {selectedIds.size > 0 && (
              <p className="text-xs text-neutral-400">
                Klick auf die Karte setzt den Standort-Pin und weist {selectedIds.size} Aktivität(en) zu.
              </p>
            )}
          </div>
        )}

        {assignMutation.error && (
          <p className="text-xs text-rose-500">Fehler: {(assignMutation.error as Error).message}</p>
        )}
      </div>
    </div>
  )
}
