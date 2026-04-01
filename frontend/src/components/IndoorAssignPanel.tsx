import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchActivities } from '@/api/activities'
import { createTag, syncTagLocation, assignActivitiesToTag } from '@/api/tags'
import type { Tag } from '@/api/tags'
import { sportLabel } from '@/lib/sportTaxonomy'
import { normTz } from '@/lib/format'
import ColorPicker from '@/components/ColorPicker'

interface Props {
  locationTags: Tag[]
  onClose: () => void
  onPickLocation: (name: string, color: string, activityIDs: string[]) => void
  onAssigned: () => void
  onDragStateChange?: (isDragging: boolean) => void
}

const colorPalette = [
  '#f97316', '#3b82f6', '#10b981', '#8b5cf6',
  '#f43f5e', '#06b6d4', '#eab308', '#6b7280',
]

export default function IndoorAssignPanel({ locationTags, onClose, onPickLocation, onAssigned, onDragStateChange }: Props) {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [sportFilter, setSportFilter] = useState('')
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [selectedTagId, setSelectedTagId] = useState<string>('')
  const [showNewLocation, setShowNewLocation] = useState(false)
  const [newLocName, setNewLocName] = useState('')
  const [newLocColor, setNewLocColor] = useState('#F97316')

  // Fetch unlocated activities (no GPS and no manually assigned location)
  const { data: activitiesData, isLoading } = useQuery({
    queryKey: ['unlocated-activities'],
    queryFn: () => fetchActivities(undefined, 100, undefined, undefined, { hasGPS: false, hasLocation: false }),
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

  // Ausgewählte Items ganz oben anzeigen
  const sortedFiltered = useMemo(() => {
    return [...filtered].sort((a, b) => {
      const aSelected = selectedIds.has(a.id) ? 0 : 1
      const bSelected = selectedIds.has(b.id) ? 0 : 1
      return aSelected - bSelected
    })
  }, [filtered, selectedIds])

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
      await assignActivitiesToTag(tag.id, Array.from(selectedIds))
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      queryClient.invalidateQueries({ queryKey: ['unlocated-activities'] })
      onAssigned()
      setSelectedIds(new Set())
      if (selectedTagId) syncTagLocation(selectedTagId).catch(() => {})
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
  const [dragHint, setDragHint] = useState(false)

  function showDragHint() {
    setDragHint(true)
    setTimeout(() => setDragHint(false), 2500)
  }

  // actId: das direkt gezogene Item. Ist es selektiert → alle selektierten mitnehmen.
  // Ist es nicht selektiert → nur dieses eine Item.
  function handleItemDragStart(e: React.DragEvent, actId: string) {
    const ids = selectedIds.has(actId) ? Array.from(selectedIds) : [actId]
    e.dataTransfer.setData('application/skadia-activities', JSON.stringify(ids))
    e.dataTransfer.effectAllowed = 'copy'
    onDragStateChange?.(true)
  }

  // Drag-Handle-Button (zieht alle selektierten)
  function handleDragStart(e: React.DragEvent) {
    if (!hasSelection) { e.preventDefault(); return }
    e.dataTransfer.setData('application/skadia-activities', JSON.stringify(Array.from(selectedIds)))
    e.dataTransfer.effectAllowed = 'copy'
    onDragStateChange?.(true)
  }

  function handleDragEnd() {
    onDragStateChange?.(false)
  }

  return (
    <div className="absolute top-[3.5rem] right-[17.5rem] z-10 w-72 max-h-[calc(100vh-8.75rem)] flex flex-col rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-lg overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-neutral-100 dark:border-neutral-800">
        <span className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">Standort zuweisen</span>
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
          className="px-2.5 py-1.5 text-xs text-accent-DEFAULT border border-accent-DEFAULT/30 rounded-lg hover:bg-accent-DEFAULT/10 cursor-pointer whitespace-nowrap"
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
          onDragEnd={handleDragEnd}
          onClick={hasSelection ? showDragHint : undefined}
          className={`flex items-center gap-1 px-2 py-1.5 text-xs rounded-lg border transition-colors select-none ${
            hasSelection
              ? 'border-accent-DEFAULT/40 text-accent-DEFAULT cursor-grab active:cursor-grabbing hover:bg-accent-DEFAULT/10'
              : 'border-neutral-200 dark:border-neutral-700 text-neutral-300 dark:text-neutral-600 cursor-not-allowed'
          }`}
          title={hasSelection ? 'Auf Karte ziehen zum Zuweisen' : 'Erst Aktivitäten auswählen'}
        >
          {/* drag-hand icon */}
          <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
            <path d="M9 3.5a1 1 0 00-2 0V9H5.5a1 1 0 000 2H7v1H5.5a1 1 0 000 2H7v.5a4.5 4.5 0 009 0v-5a1 1 0 00-2 0v2h-.5V9a1 1 0 00-2 0v2H11V3.5z" />
          </svg>
          <span>Ziehen</span>
        </div>
      </div>
      {dragHint && (
        <div className="px-3 py-1.5 bg-accent-DEFAULT/10 border-b border-accent-DEFAULT/30 text-xs text-accent-DEFAULT text-center animate-pulse">
          Diesen Button auf einen Pin auf der Karte ziehen
        </div>
      )}

      {/* Drag hint */}
      {!hasSelection && (
        <p className="px-3 py-1.5 text-xs text-center border-b border-neutral-100 dark:border-neutral-800">
          <span className="flex items-center justify-center gap-1.5 text-neutral-500 dark:text-neutral-400">
            <svg className="w-3.5 h-3.5 flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
              <path d="M10 2a.75.75 0 01.75.75v8.69l3.22-3.22a.75.75 0 111.06 1.06l-4.5 4.5a.75.75 0 01-1.06 0l-4.5-4.5a.75.75 0 111.06-1.06l3.22 3.22V2.75A.75.75 0 0110 2z" />
            </svg>
            Aktivitäten wählen und auf die Karte ziehen
          </span>
        </p>
      )}

      {/* Search + Sport filter */}
      <div className="px-3 py-2 space-y-2 border-b border-neutral-100 dark:border-neutral-800">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Titel / Beschreibung suchen..."
          className="w-full px-2.5 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
        />
        <select
          value={sportFilter}
          onChange={(e) => setSportFilter(e.target.value)}
          className="w-full px-2.5 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
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
          <div className="py-8 text-center text-xs text-neutral-400">Keine Aktivitäten ohne Standort gefunden</div>
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
            {sortedFiltered.map(act => {
              const dateStr = act.started_at
                ? new Date(act.started_at).toLocaleDateString('de-DE', {
                    weekday: 'short', day: '2-digit', month: '2-digit',
                    ...(normTz(act.timezone) ? { timeZone: normTz(act.timezone) } : {}),
                  })
                : ''
              const dur = act.elapsed_time_s
                ? `${Math.floor(act.elapsed_time_s / 3600)}h ${Math.floor((act.elapsed_time_s % 3600) / 60)}m`
                : ''
              return (
                <label
                  key={act.id}
                  draggable
                  onDragStart={(e) => handleItemDragStart(e, act.id)}
                  onDragEnd={handleDragEnd}
                  className="flex items-start gap-2 px-3 py-2 select-none cursor-grab active:cursor-grabbing hover:bg-neutral-50 dark:hover:bg-neutral-800/30 border-b border-neutral-50 dark:border-neutral-800/50 last:border-0"
                >
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
              className="w-full px-2.5 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
            >
              <option value="">An Standort zuweisen...</option>
              {locationTags.map(t => (
                <option key={t.id} value={t.id}>{t.name}</option>
              ))}
            </select>
            <button
              onClick={() => assignMutation.mutate()}
              disabled={selectedIds.size === 0 || !selectedTagId || assignMutation.isPending}
              className="w-full py-1.5 text-xs font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
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
              className="w-full px-2.5 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
            />
            <div className="space-y-1.5">
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
              </div>
              <ColorPicker value={newLocColor} onChange={setNewLocColor} />
            </div>
            <div className="flex gap-2">
              <button
                onClick={handleNewLocationSubmit}
                disabled={!newLocName.trim()}
                className="flex-1 py-1.5 text-xs font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
              >
                {selectedIds.size > 0 ? 'Auf Karte setzen' : 'Erstellen'}
              </button>
              {selectedIds.size === 0 && (
                <button
                  onClick={handleCreateLocationOnly}
                  disabled={!newLocName.trim()}
                  className="px-2.5 py-1.5 text-xs border border-neutral-200 dark:border-neutral-700 rounded-lg hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
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
