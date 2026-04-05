import { useState, useCallback, useRef, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type { RouteWaypoint, RouteSegment, PlannedRoute } from '@/api/routes'
import { useToast } from '@/lib/toast'
import type { ProfileConfig } from '@/lib/routingProfiles'
import { getProfileDefaults } from '@/lib/routingProfiles'
import { fmtDist, fmtDurationMin } from '@/lib/format'
import ProfileEditor from './ProfileEditor'
import {
  fetchAllProfiles,
  createProfile,
  deleteProfile,
  importProfiles,
  exportProfileUrl,
} from '@/api/routingProfiles'
import { apiDownload } from '@/api/client'

function formatElevation(m: number): string {
  return `${Math.round(m)} m`
}


function GripIcon({ dragging }: { dragging?: boolean }) {
  return (
    <svg
      className={`w-4 h-4 text-neutral-400 dark:text-neutral-500 flex-shrink-0 ${dragging ? 'cursor-grabbing' : 'cursor-grab'}`}
      viewBox="0 0 24 24"
      fill="currentColor"
    >
      <circle cx="9" cy="6" r="1.5" />
      <circle cx="15" cy="6" r="1.5" />
      <circle cx="9" cy="12" r="1.5" />
      <circle cx="15" cy="12" r="1.5" />
      <circle cx="9" cy="18" r="1.5" />
      <circle cx="15" cy="18" r="1.5" />
    </svg>
  )
}

function WaypointIcon({ type }: { type: RouteWaypoint['type'] }) {
  if (type === 'start') {
    return (
      <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-green-500 text-white text-[9px] font-bold flex-shrink-0">
        S
      </span>
    )
  }
  if (type === 'end') {
    return (
      <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-red-500 text-white text-[9px] font-bold flex-shrink-0">
        Z
      </span>
    )
  }
  return (
    <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-accent-DEFAULT text-white text-[9px] font-bold flex-shrink-0">
      •
    </span>
  )
}

interface WaypointItemProps {
  waypoint: RouteWaypoint
  index: number
  total: number
  isDragging: boolean
  isDragOver: boolean
  onRemove: (id: string) => void
  onToggleMode: (id: string) => void
  onDragStart: (e: React.DragEvent) => void
  onDragOver: (e: React.DragEvent) => void
  onDragLeave: () => void
  onDrop: (e: React.DragEvent) => void
  onDragEnd: () => void
  segmentAfter?: RouteSegment
}

function WaypointItem({
  waypoint,
  index,
  total,
  isDragging,
  isDragOver,
  onRemove,
  onDragStart,
  onDragOver,
  onDragLeave,
  onDrop,
  onDragEnd,
  segmentAfter,
}: WaypointItemProps) {
  const label = waypoint.poi_name
    ?? `${waypoint.latlng[0].toFixed(4)}, ${waypoint.latlng[1].toFixed(4)}`

  return (
    <div
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
      className={[
        'flex items-start gap-1.5 py-1 rounded transition-colors',
        isDragging ? 'opacity-40' : '',
        isDragOver ? 'bg-accent-100/40 dark:bg-accent-DEFAULT/10 ring-1 ring-accent-DEFAULT/30' : '',
      ].filter(Boolean).join(' ')}
    >
      {/* Grip handle for drag-to-reorder — always visible */}
      <div
        draggable
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
        className="flex-shrink-0 pt-1"
        title="Verschieben"
      >
        <GripIcon dragging={isDragging} />
      </div>

      {/* Waypoint type indicator + connector line */}
      <div className="flex flex-col items-center gap-0.5">
        <WaypointIcon type={waypoint.type} />
        {index < total - 1 && (
          <div className="w-px flex-1 min-h-[12px] bg-neutral-200 dark:bg-neutral-700 my-0.5" />
        )}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1">
          <span className="text-xs text-neutral-700 dark:text-neutral-300 truncate flex-1" title={label}>
            {label}
          </span>
          {/* Remove button — always visible, dezent */}
          {total > 2 && (
            <button
              onClick={() => onRemove(waypoint.id)}
              className="w-5 h-5 flex items-center justify-center text-neutral-300 hover:text-red-500 dark:text-neutral-600 dark:hover:text-red-400 transition-colors cursor-pointer flex-shrink-0"
              title="Wegpunkt entfernen"
            >
              <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
              </svg>
            </button>
          )}
        </div>

        {/* Segment info between waypoints */}
        {segmentAfter && (
          <div className="flex items-center gap-2 mt-0.5 text-[10px] text-neutral-400 dark:text-neutral-500">
            <span>{fmtDist(segmentAfter.distance_m)}</span>
            {segmentAfter.elevation_gain > 0 && (
              <span className="text-green-600 dark:text-green-500">↑{segmentAfter.elevation_estimated ? '~' : ''}{formatElevation(segmentAfter.elevation_gain)}</span>
            )}
            {segmentAfter.elevation_loss > 0 && (
              <span className="text-red-500 dark:text-red-400">↓{segmentAfter.elevation_estimated ? '~' : ''}{formatElevation(segmentAfter.elevation_loss)}</span>
            )}
            <span className={`px-1 rounded text-[9px] font-medium ${
              segmentAfter.mode === 'offroad'
                ? 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400'
                : 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
            }`}>
              {segmentAfter.mode === 'offroad' ? 'Freihand' : 'Route'}
            </span>
          </div>
        )}
      </div>
    </div>
  )
}

// Offline fallback for BRouter profiles (used when API is not available)
const BROUTER_PROFILES_FALLBACK = [
  { value: 'hiking-mountain', label: 'Wandern (Alpin, SAC T1-T3)' },
  { value: 'hiking-sac2',     label: 'Wandern (SAC T2 max)' },
  { value: 'cycling-road',    label: 'Rennrad' },
  { value: 'cycling-mountain', label: 'MTB' },
  { value: 'cycling-gravel',  label: 'Gravel' },
  { value: 'trekking',        label: 'Trekking (Rad)' },
  { value: 'running',         label: 'Laufen' },
  { value: 'walking',         label: 'Spaziergang' },
]


export interface RoutePlannerPanelProps {
  // Current planning state (controlled from parent / MapPage)
  waypoints: RouteWaypoint[]
  segments: RouteSegment[]
  profileConfig: ProfileConfig
  showHeatmap: boolean
  // Routing engine
  routingEngine: 'badrouter' | 'brouter'
  brouterProfile: string
  badRouterAvailable: boolean
  onRoutingEngineChange: (engine: 'badrouter' | 'brouter') => void
  onBrouterProfileChange: (profile: string) => void
  // Stats (computed by parent from segments)
  totalDistanceM: number
  totalElevationGain: number
  totalElevationLoss: number
  estimatedTimeMin?: number
  // Callbacks
  onProfileChange: (config: ProfileConfig) => void
  onRemoveWaypoint: (id: string) => void
  onReorderWaypoints: (waypoints: RouteWaypoint[]) => void
  onRecalculateAll: () => void
  onToggleSegmentMode: (waypointId: string) => void
  onToggleHeatmap: (enabled: boolean) => void
  onClearRoute: () => void
  onSave: () => void              // opens RouteSaveDialog in parent
  onSuggest: () => void           // opens RouteSuggestionDialog in parent
  onClose: () => void
  // Error state
  calculationError?: string | null
  onClearError?: () => void
  // Optional: existing route being edited
  editingRoute?: PlannedRoute | null
  isSaving?: boolean
}

export default function RoutePlannerPanel({
  waypoints,
  segments,
  profileConfig,
  showHeatmap,
  routingEngine,
  brouterProfile,
  badRouterAvailable,
  onRoutingEngineChange,
  onBrouterProfileChange,
  totalDistanceM,
  totalElevationGain,
  totalElevationLoss,
  estimatedTimeMin,
  onProfileChange,
  onRemoveWaypoint,
  onReorderWaypoints,
  onRecalculateAll,
  onToggleSegmentMode,
  onToggleHeatmap,
  onClearRoute,
  onSave,
  onSuggest,
  onClose,
  calculationError,
  onClearError,
  editingRoute,
  isSaving = false,
}: RoutePlannerPanelProps) {
  const { addToast } = useToast()
  const queryClient = useQueryClient()
  const [collapsed, setCollapsed] = useState(false)
  const dragIndexRef = useRef<number | null>(null)
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null)
  const [dragOverIndex, setDragOverIndex] = useState<number | null>(null)

  // ── Profile management state ───────────────────────────────────────────
  const [showSaveDialog, setShowSaveDialog] = useState(false)
  const [saveProfileName, setSaveProfileName] = useState('')
  const [showGearMenu, setShowGearMenu] = useState(false)
  const gearMenuRef = useRef<HTMLDivElement | null>(null)
  const importInputRef = useRef<HTMLInputElement | null>(null)

  // Close gear menu on click outside
  useEffect(() => {
    if (!showGearMenu) return
    function handleClickOutside(e: MouseEvent) {
      if (gearMenuRef.current && !gearMenuRef.current.contains(e.target as Node)) {
        setShowGearMenu(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [showGearMenu])

  // Fetch all profiles from the API
  const { data: profilesData } = useQuery({
    queryKey: ['routing-profiles'],
    queryFn: fetchAllProfiles,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })

  // Derived lists
  const badRouterProfiles = profilesData?.profiles.filter(p => p.engine === 'badrouter') ?? []
  const brouterProfiles = profilesData?.profiles.filter(p => p.engine === 'brouter') ?? []
  const brouterOptions = brouterProfiles.length > 0
    ? brouterProfiles.map(p => ({ value: p.name, label: p.name }))
    : BROUTER_PROFILES_FALLBACK

  // Current BadRouter profile entry from API (for defaults reset and delete)
  const currentBadRouterEntry = badRouterProfiles.find(p => p.name === profileConfig.profile)

  // Mutations
  const createMutation = useMutation({
    mutationFn: createProfile,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['routing-profiles'] })
      setShowSaveDialog(false)
      setSaveProfileName('')
      addToast({ type: 'success', title: 'Profil gespeichert' })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: 'Profil speichern fehlgeschlagen', message: err.message, duration: 0 })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteProfile,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['routing-profiles'] })
      addToast({ type: 'success', title: 'Profil gelöscht' })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: 'Profil löschen fehlgeschlagen', message: err.message, duration: 0 })
    },
  })

  function handleSaveProfile() {
    const name = saveProfileName.trim()
    if (!name) return
    const config = { ...profileConfig } as Record<string, unknown>
    const baseProfile = (profileConfig.profile ?? 'hiking') as string
    createMutation.mutate({
      name,
      base_profile: baseProfile,
      sport_types: currentBadRouterEntry?.sport_types ?? [],
      config,
    })
  }

  function handleResetToDefault() {
    const profileName = profileConfig.profile ?? 'hiking'
    // Use API config if available, else fall back to TS defaults
    const entry = badRouterProfiles.find(p => p.name === profileName && p.source === 'system')
    if (entry?.config) {
      const apiConfig = entry.config as Partial<ProfileConfig>
      onProfileChange({ profile: profileName, ...apiConfig } as ProfileConfig)
    } else {
      onProfileChange(getProfileDefaults(profileName))
    }
  }

  async function handleExportProfile(id: string, name: string) {
    try {
      const { blob, filename } = await apiDownload(exportProfileUrl(id))
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename ?? `${name}.json`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      addToast({ type: 'error', title: 'Export fehlgeschlagen', message: String(err), duration: 0 })
    }
  }

  async function handleImportFiles(files: FileList | null) {
    if (!files || files.length === 0) return
    const fileArr = Array.from(files)
    try {
      const res = await importProfiles(fileArr)
      const failed = res.results.filter(r => !r.success)
      const succeeded = res.results.filter(r => r.success)
      if (succeeded.length > 0) {
        void queryClient.invalidateQueries({ queryKey: ['routing-profiles'] })
        addToast({ type: 'success', title: `${succeeded.length === 1 ? '1 Profil' : `${succeeded.length} Profile`} importiert` })
      }
      if (failed.length > 0) {
        addToast({
          type: 'error',
          title: `${failed.length === 1 ? '1 Profil' : `${failed.length} Profile`} nicht importiert`,
          message: failed.map(f => `${f.name}: ${f.error ?? 'Unbekannter Fehler'}`).join('\n'),
          duration: 0,
        })
      }
    } catch (err) {
      addToast({ type: 'error', title: 'Import fehlgeschlagen', message: String(err), duration: 0 })
    }
  }

  const hasRoute = waypoints.length >= 2

  function handleDragStart(e: React.DragEvent, index: number) {
    dragIndexRef.current = index
    setDraggingIndex(index)
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(index))
  }

  function handleDragOver(e: React.DragEvent, index: number) {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    if (dragIndexRef.current !== index) setDragOverIndex(index)
  }

  function handleDragLeave() {
    setDragOverIndex(null)
  }

  function handleDrop(_e: React.DragEvent, dropIndex: number) {
    const from = dragIndexRef.current
    if (from === null || from === dropIndex) { setDragOverIndex(null); return }
    const reordered = [...waypoints]
    const [moved] = reordered.splice(from, 1)
    reordered.splice(dropIndex, 0, moved)
    const reassigned = reordered.map((wp, i) => ({
      ...wp,
      type: i === 0 ? ('start' as const) : i === reordered.length - 1 ? ('end' as const) : ('via' as const),
    }))
    onReorderWaypoints(reassigned)
    void onRecalculateAll()
    setDragOverIndex(null)
  }

  function handleDragEnd() {
    dragIndexRef.current = null
    setDraggingIndex(null)
    setDragOverIndex(null)
  }

  const handleClear = useCallback(() => {
    if (waypoints.length > 0) {
      onClearRoute()
      addToast({ type: 'info', title: 'Route geleert' })
    }
  }, [waypoints.length, onClearRoute, addToast])

  const anyElevationEstimated = segments.some(s => s.elevation_estimated === true)

  // Find the segment between waypoint[i] and waypoint[i+1]
  function segmentAfterWaypoint(waypoint: RouteWaypoint, index: number): RouteSegment | undefined {
    if (index >= waypoints.length - 1) return undefined
    const nextWaypoint = waypoints[index + 1]
    return segments.find(
      s => s.from_waypoint === waypoint.id && s.to_waypoint === nextWaypoint.id
    )
  }

  return (
    <div className="h-full w-72 flex flex-col border-r border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 flex-shrink-0">
      <div className="flex flex-col overflow-hidden flex-1 min-h-0">

        {/* Header */}
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-neutral-100 dark:border-neutral-800 flex-shrink-0">
          <button
            onClick={() => setCollapsed(!collapsed)}
            className="flex-1 flex items-center gap-2 hover:text-accent-DEFAULT cursor-pointer transition-colors"
          >
            {/* Route icon */}
            <svg className="w-4 h-4 text-accent-DEFAULT flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M12.207 2.232a.75.75 0 00.025 1.06l4.146 3.958H6.375a5.375 5.375 0 000 10.75H9.75a.75.75 0 000-1.5H6.375a3.875 3.875 0 010-7.75h10.003l-4.146 3.957a.75.75 0 001.036 1.085l5.5-5.25a.75.75 0 000-1.085l-5.5-5.25a.75.75 0 00-1.061.025z" clipRule="evenodd" />
            </svg>
            <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              {editingRoute ? `Route: ${editingRoute.name}` : 'Route planen'}
            </span>
            <svg
              className={`w-4 h-4 text-neutral-400 ml-auto transition-transform ${collapsed ? '-rotate-90' : ''}`}
              viewBox="0 0 20 20"
              fill="currentColor"
            >
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
          <div className="flex flex-col overflow-y-auto min-h-0">

            {/* Routing engine toggle */}
            <div className="px-3 pt-2.5 pb-1.5 border-b border-neutral-100 dark:border-neutral-800">
              <div className="flex items-center gap-1 p-0.5 rounded-lg bg-neutral-100 dark:bg-neutral-800">
                <button
                  onClick={() => onRoutingEngineChange('badrouter')}
                  disabled={!badRouterAvailable}
                  title={!badRouterAvailable ? 'Routing-Engine nicht verfügbar (keine Tiles)' : undefined}
                  className={`flex-1 px-2 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-40 ${
                    routingEngine === 'badrouter'
                      ? 'bg-accent-DEFAULT text-white shadow-sm'
                      : 'bg-transparent text-neutral-600 dark:text-neutral-400 hover:text-neutral-800 dark:hover:text-neutral-200'
                  }`}
                >
                  BadRouter
                </button>
                <button
                  onClick={() => onRoutingEngineChange('brouter')}
                  className={`flex-1 px-2 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer ${
                    routingEngine === 'brouter'
                      ? 'bg-accent-DEFAULT text-white shadow-sm'
                      : 'bg-transparent text-neutral-600 dark:text-neutral-400 hover:text-neutral-800 dark:hover:text-neutral-200'
                  }`}
                >
                  BRouter
                </button>
              </div>
            </div>

            {/* Profile editor (BadRouter) or BRouter profile selector */}
            {routingEngine === 'badrouter' ? (
              <>
                {/* Hidden file input for import */}
                <input
                  ref={importInputRef}
                  type="file"
                  accept=".json"
                  multiple
                  className="hidden"
                  onChange={e => { void handleImportFiles(e.target.files); e.target.value = '' }}
                />

                {/* Gear menu for profile management actions */}
                <div className="px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800 flex items-center justify-end" ref={gearMenuRef}>
                  <div className="relative">
                    <button
                      onClick={() => setShowGearMenu(v => !v)}
                      title="Profilverwaltung"
                      className="p-1 rounded text-neutral-400 dark:text-neutral-500 hover:bg-neutral-100 dark:hover:bg-neutral-800 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors cursor-pointer"
                    >
                      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
                        <circle cx="12" cy="12" r="3" />
                      </svg>
                    </button>
                    {showGearMenu && (
                      <div className="absolute right-0 top-full mt-1 z-50 min-w-[180px] rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg py-1">
                        <button
                          onClick={() => { handleResetToDefault(); setShowGearMenu(false) }}
                          className="w-full flex items-center gap-2 px-3 py-1.5 text-xs text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer text-left"
                        >
                          <svg className="w-3.5 h-3.5 flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
                            <path fillRule="evenodd" d="M15.312 11.424a5.5 5.5 0 01-9.201 2.466l-.312-.311h2.433a.75.75 0 000-1.5H4.28a.75.75 0 00-.75.75v3.955a.75.75 0 001.5 0v-2.146l.312.31a7 7 0 0011.712-3.138.75.75 0 00-1.449-.386zm-10.624-2.85a5.5 5.5 0 019.201-2.466l.312.311h-2.433a.75.75 0 000 1.5h3.952a.75.75 0 00.75-.75V3.214a.75.75 0 00-1.5 0v2.146l-.312-.31A7 7 0 003.244 8.188a.75.75 0 001.449.386z" clipRule="evenodd" />
                          </svg>
                          Auf Standard zuruecksetzen
                        </button>
                        <button
                          onClick={() => { setSaveProfileName(''); setShowSaveDialog(true); setShowGearMenu(false) }}
                          className="w-full flex items-center gap-2 px-3 py-1.5 text-xs text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer text-left"
                        >
                          <svg className="w-3.5 h-3.5 flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
                            <path d="M3.75 3A1.75 1.75 0 002 4.75v10.5c0 .966.784 1.75 1.75 1.75h12.5A1.75 1.75 0 0018 15.25v-8.5A1.75 1.75 0 0016.25 5H15V3.75A1.75 1.75 0 0013.25 2h-3.5A1.75 1.75 0 008 3.75V5H4.75A1.75 1.75 0 003.75 3zm4.75.75A.25.25 0 018.75 4h2.5a.25.25 0 01.25.25V5h-3.5V3.75z" />
                          </svg>
                          Profil speichern...
                        </button>
                        <button
                          onClick={() => { importInputRef.current?.click(); setShowGearMenu(false) }}
                          className="w-full flex items-center gap-2 px-3 py-1.5 text-xs text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer text-left"
                        >
                          <svg className="w-3.5 h-3.5 flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
                            <path fillRule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-7.707a1 1 0 011.414 0L9 10.586V3a1 1 0 112 0v7.586l1.293-1.293a1 1 0 111.414 1.414l-3 3a1 1 0 01-1.414 0l-3-3a1 1 0 010-1.414z" clipRule="evenodd" />
                          </svg>
                          Profil importieren...
                        </button>
                        {currentBadRouterEntry && (
                          <button
                            onClick={() => { void handleExportProfile(currentBadRouterEntry.id, currentBadRouterEntry.name); setShowGearMenu(false) }}
                            className="w-full flex items-center gap-2 px-3 py-1.5 text-xs text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer text-left"
                          >
                            <svg className="w-3.5 h-3.5 flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
                              <path fillRule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-9.707a1 1 0 000 1.414L9 11.414V3a1 1 0 112 0v8.414l2.707-2.707a1 1 0 111.414 1.414l-4.414 4.414a1 1 0 01-1.414 0L4.293 8.707a1 1 0 010-1.414z" clipRule="evenodd" />
                            </svg>
                            Profil exportieren
                          </button>
                        )}
                      </div>
                    )}
                  </div>
                </div>

                {/* BadRouter: user/filesystem profiles list (editable entries) */}
                {badRouterProfiles.filter(p => p.editable || p.source === 'filesystem').length > 0 && (
                  <div className="px-3 py-2 border-b border-neutral-100 dark:border-neutral-800">
                    <div className="text-[10px] text-neutral-400 dark:text-neutral-500 uppercase tracking-wide mb-1.5">
                      Eigene Profile
                    </div>
                    <div className="space-y-1">
                      {badRouterProfiles
                        .filter(p => p.editable || p.source === 'filesystem')
                        .map(p => (
                          <div key={p.id} className="flex items-center gap-1.5">
                            <button
                              onClick={() => {
                                const cfg = p.config ? { profile: p.base_profile, ...p.config } as ProfileConfig : getProfileDefaults(p.base_profile)
                                onProfileChange(cfg)
                              }}
                              className={`flex-1 text-left px-2 py-1 rounded text-xs transition-colors cursor-pointer truncate ${
                                profileConfig.profile === p.base_profile && !badRouterProfiles.find(s => s.source === 'system' && s.name === profileConfig.profile)
                                  ? 'bg-accent-DEFAULT/10 text-accent-DEFAULT font-medium'
                                  : 'text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                              }`}
                              title={p.name}
                            >
                              {p.name}
                              {p.source === 'filesystem' && (
                                <span className="ml-1 text-[9px] text-neutral-400 dark:text-neutral-500">(fs)</span>
                              )}
                            </button>
                            {/* Export */}
                            <button
                              onClick={() => void handleExportProfile(p.id, p.name)}
                              title="Profil exportieren"
                              className="text-neutral-300 dark:text-neutral-600 hover:text-accent-DEFAULT dark:hover:text-accent-DEFAULT transition-colors cursor-pointer flex-shrink-0"
                            >
                              <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                                <path fillRule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-9.707a1 1 0 000 1.414L9 11.414V3a1 1 0 112 0v8.414l2.707-2.707a1 1 0 111.414 1.414l-4.414 4.414a1 1 0 01-1.414 0L4.293 8.707a1 1 0 010-1.414z" clipRule="evenodd" />
                              </svg>
                            </button>
                            {/* Delete — editable profiles only (not system or filesystem) */}
                            {p.editable && (
                              <button
                                onClick={() => {
                                  if (confirm(`Profil "${p.name}" wirklich löschen?`)) {
                                    deleteMutation.mutate(p.id)
                                  }
                                }}
                                title="Profil löschen"
                                className="text-neutral-300 dark:text-neutral-600 hover:text-red-500 dark:hover:text-red-400 transition-colors cursor-pointer flex-shrink-0"
                              >
                                <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                                  <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 11.25a.75.75 0 01.75.75v2a.75.75 0 01-1.5 0v-2a.75.75 0 01.75-.75zM7.75 12a.75.75 0 00-1.5 0v2a.75.75 0 001.5 0v-2zm4 .75a.75.75 0 01.75-.75.75.75 0 01.75.75v2a.75.75 0 01-1.5 0v-2z" clipRule="evenodd" />
                                </svg>
                              </button>
                            )}
                          </div>
                        ))}
                    </div>
                  </div>
                )}

                <ProfileEditor
                  config={profileConfig}
                  onChange={onProfileChange}
                />

                {/* Save-as dialog (inline, below profile tabs) */}
                {showSaveDialog && (
                  <div className="px-3 py-2.5 border-b border-neutral-100 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-800/50">
                    <div className="text-[10px] text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-1.5">
                      Profil speichern
                    </div>
                    <div className="flex gap-1.5">
                      <input
                        type="text"
                        placeholder="Profilname..."
                        value={saveProfileName}
                        onChange={e => setSaveProfileName(e.target.value)}
                        onKeyDown={e => {
                          if (e.key === 'Enter') handleSaveProfile()
                          else if (e.key === 'Escape') setShowSaveDialog(false)
                        }}
                        autoFocus
                        className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                      />
                      <button
                        onClick={handleSaveProfile}
                        disabled={!saveProfileName.trim() || createMutation.isPending}
                        className="px-2.5 py-1.5 rounded-md bg-accent-DEFAULT text-white text-xs font-medium hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
                      >
                        OK
                      </button>
                      <button
                        onClick={() => setShowSaveDialog(false)}
                        className="px-2.5 py-1.5 rounded-md border border-neutral-200 dark:border-neutral-700 text-neutral-500 text-xs hover:bg-neutral-100 dark:hover:bg-neutral-700 cursor-pointer transition-colors"
                      >
                        Abbruch
                      </button>
                    </div>
                  </div>
                )}
                {/* Offroad snap threshold (BadRouter only) */}
                <div className="px-3 py-2 border-t border-neutral-100 dark:border-neutral-800">
                  <div className="space-y-1">
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-neutral-700 dark:text-neutral-300">Offroad-Schwelle</span>
                      <span className="text-xs font-mono text-neutral-500 dark:text-neutral-400 tabular-nums">
                        {profileConfig.snap_threshold_m ?? 100} m
                      </span>
                    </div>
                    <input
                      type="range"
                      min={10}
                      max={500}
                      step={10}
                      value={profileConfig.snap_threshold_m ?? 100}
                      onChange={e => onProfileChange({ ...profileConfig, snap_threshold_m: parseInt(e.target.value, 10) })}
                      className="w-full h-1.5 rounded-full appearance-none cursor-pointer accent-[var(--color-accent-DEFAULT,#F97316)] bg-neutral-200 dark:bg-neutral-700"
                    />
                    <p className="text-[10px] text-neutral-400 dark:text-neutral-500 leading-snug">
                      Maximale Distanz zum nächsten Weg beim Snap
                    </p>
                  </div>
                </div>
              </>
            ) : (
              <div className="px-3 py-2 border-b border-neutral-100 dark:border-neutral-800">
                <label className="block text-[10px] text-neutral-400 dark:text-neutral-500 uppercase tracking-wide mb-1">
                  Profil
                </label>
                <select
                  value={brouterProfile}
                  onChange={e => onBrouterProfileChange(e.target.value)}
                  className="w-full text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 text-neutral-800 dark:text-neutral-200 px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT cursor-pointer"
                >
                  {brouterOptions.map(p => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
              </div>
            )}

            {/* Waypoint list */}
            <div className="px-3 pt-2 pb-1">
              {waypoints.length === 0 ? (
                <div className="text-xs text-neutral-400 dark:text-neutral-500 text-center py-4">
                  Klicke auf die Karte, um Wegpunkte zu setzen
                </div>
              ) : (
                <div className="space-y-0">
                  {waypoints.map((wp, i) => (
                    <WaypointItem
                      key={wp.id}
                      waypoint={wp}
                      index={i}
                      total={waypoints.length}
                      isDragging={draggingIndex === i}
                      isDragOver={dragOverIndex === i}
                      onRemove={onRemoveWaypoint}
                      onToggleMode={onToggleSegmentMode}
                      onDragStart={(e) => handleDragStart(e, i)}
                      onDragOver={(e) => handleDragOver(e, i)}
                      onDragLeave={handleDragLeave}
                      onDrop={(e) => handleDrop(e, i)}
                      onDragEnd={handleDragEnd}
                      segmentAfter={segmentAfterWaypoint(wp, i)}
                    />
                  ))}
                </div>
              )}
            </div>

            {/* No-route inline dialog */}
            {calculationError === 'no_route' && (
              <div className="mx-3 mb-2 rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 px-3 py-2.5">
                <p className="text-xs text-amber-800 dark:text-amber-300 font-medium mb-2">
                  Keine Route gefunden
                </p>
                <p className="text-[10px] text-amber-600 dark:text-amber-400 mb-2.5">
                  Zwischen den Wegpunkten gibt es keinen Weg im Netzwerk.
                </p>
                <div className="flex gap-2">
                  <button
                    onClick={() => {
                      const lastWpIdx = waypoints.length - 1
                      if (lastWpIdx >= 1) {
                        onToggleSegmentMode(waypoints[lastWpIdx - 1].id)
                      }
                      onClearError?.()
                    }}
                    className="flex-1 px-2 py-1.5 rounded-md bg-amber-500 text-white text-xs font-medium hover:bg-amber-600 transition-colors cursor-pointer"
                  >
                    Offroad-Segment anlegen
                  </button>
                  <button
                    onClick={() => onClearError?.()}
                    className="px-2 py-1.5 rounded-md border border-amber-300 dark:border-amber-700 text-amber-700 dark:text-amber-400 text-xs font-medium hover:bg-amber-100 dark:hover:bg-amber-900/30 transition-colors cursor-pointer"
                  >
                    Abbrechen
                  </button>
                </div>
              </div>
            )}

          </div>
        )}

        {/* ── Fixierter unterer Bereich (Stats + Buttons) ──────────────── */}
        {!collapsed && (
          <div className="flex-shrink-0 border-t border-neutral-100 dark:border-neutral-800">
            {/* Route statistics */}
            {hasRoute && (
              <div className="mx-3 mt-2 mb-2 rounded-lg bg-neutral-50 dark:bg-neutral-800/60 px-3 py-2 grid grid-cols-2 gap-x-4 gap-y-1">
                <div>
                  <div className="text-[10px] text-neutral-400 dark:text-neutral-500 uppercase tracking-wide">Distanz</div>
                  <div className="text-sm font-semibold text-neutral-800 dark:text-neutral-100">
                    {fmtDist(totalDistanceM)}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] text-neutral-400 dark:text-neutral-500 uppercase tracking-wide">Zeit (est.)</div>
                  <div className="text-sm font-semibold text-neutral-800 dark:text-neutral-100">
                    {estimatedTimeMin != null ? fmtDurationMin(estimatedTimeMin) : '—'}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] text-green-600 dark:text-green-500 uppercase tracking-wide">Aufstieg</div>
                  <div className="text-sm font-semibold text-green-700 dark:text-green-400">
                    {totalElevationGain > 0 ? `↑ ${anyElevationEstimated ? '~' : ''}${formatElevation(totalElevationGain)}` : '—'}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] text-red-500 dark:text-red-400 uppercase tracking-wide">Abstieg</div>
                  <div className="text-sm font-semibold text-red-600 dark:text-red-400">
                    {totalElevationLoss > 0 ? `↓ ${anyElevationEstimated ? '~' : ''}${formatElevation(totalElevationLoss)}` : '—'}
                  </div>
                </div>
              </div>
            )}

            {/* Heatmap toggle */}
            <div className="px-3 py-2 flex items-center justify-between">
              <label className="flex items-center gap-2 cursor-pointer select-none">
                <button
                  role="switch"
                  aria-checked={showHeatmap}
                  onClick={() => onToggleHeatmap(!showHeatmap)}
                  className={`relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none ${
                    showHeatmap ? 'bg-accent-DEFAULT' : 'bg-neutral-300 dark:bg-neutral-600'
                  }`}
                >
                  <span
                    className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ${
                      showHeatmap ? 'translate-x-4' : 'translate-x-0'
                    }`}
                  />
                </button>
                <span className="text-xs text-neutral-600 dark:text-neutral-400">Heatmap anzeigen</span>
              </label>
            </div>

            {/* Action buttons */}
            <div className="px-3 pb-3 pt-1 space-y-2">
              {/* Suggest route button */}
              <button
                onClick={onSuggest}
                className="w-full flex items-center justify-center gap-1.5 px-3 py-2 rounded-lg border border-accent-DEFAULT text-accent-DEFAULT hover:bg-accent-100 dark:hover:bg-accent-DEFAULT/10 text-xs font-medium transition-colors cursor-pointer"
              >
                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M10.75 4.75a.75.75 0 00-1.5 0v4.5h-4.5a.75.75 0 000 1.5h4.5v4.5a.75.75 0 001.5 0v-4.5h4.5a.75.75 0 000-1.5h-4.5v-4.5z" />
                </svg>
                Route vorschlagen
              </button>

              {/* Save / Clear row */}
              <div className="flex gap-2">
                <button
                  onClick={onSave}
                  disabled={!hasRoute || isSaving}
                  className="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed text-xs font-medium transition-colors cursor-pointer"
                >
                  {isSaving ? (
                    <>
                      <svg className="w-3.5 h-3.5 animate-spin" viewBox="0 0 24 24" fill="none">
                        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                      </svg>
                      Speichern...
                    </>
                  ) : (
                    <>
                      <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                        <path d="M3.75 3A1.75 1.75 0 002 4.75v10.5c0 .966.784 1.75 1.75 1.75h12.5A1.75 1.75 0 0018 15.25v-8.5A1.75 1.75 0 0016.25 5H15V3.75A1.75 1.75 0 0013.25 2h-3.5A1.75 1.75 0 008 3.75V5H4.75A1.75 1.75 0 003.75 3H3.75zm4.75.75A.25.25 0 018.75 4h2.5a.25.25 0 01.25.25V5h-3.5V3.75zm3.25 7.5a.75.75 0 01-.75.75H9.75a.75.75 0 010-1.5H11v-1.25a.75.75 0 011.5 0v1.5z" />
                      </svg>
                      {editingRoute ? 'Aktualisieren' : 'Speichern'}
                    </>
                  )}
                </button>

                <button
                  onClick={handleClear}
                  disabled={waypoints.length === 0}
                  className="px-3 py-2 rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-500 dark:text-neutral-400 hover:border-red-300 hover:text-red-500 dark:hover:border-red-700 dark:hover:text-red-400 disabled:opacity-40 disabled:cursor-not-allowed text-xs font-medium transition-colors cursor-pointer"
                  title="Route leeren"
                >
                  <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 11.25a.75.75 0 01.75.75v2a.75.75 0 01-1.5 0v-2a.75.75 0 01.75-.75zM7.75 12a.75.75 0 00-1.5 0v2a.75.75 0 001.5 0v-2zm4 .75a.75.75 0 01.75-.75.75.75 0 01.75.75v2a.75.75 0 01-1.5 0v-2z" clipRule="evenodd" />
                  </svg>
                </button>
              </div>

              {/* Hint text when no waypoints */}
              {!hasRoute && waypoints.length === 0 && (
                <p className="text-[10px] text-neutral-400 dark:text-neutral-500 text-center leading-snug">
                  Klicke auf die Karte um Start und Ziel zu setzen.{' '}
                  Oder nutze "Route vorschlagen" für eine KI-generierte Route.
                </p>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
