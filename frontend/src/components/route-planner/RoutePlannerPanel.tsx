import { useState, useCallback, useRef } from 'react'
import type { RouteWaypoint, RouteSegment, PlannedRoute } from '@/api/routes'
import { useToast } from '@/lib/toast'
import type { ProfileConfig } from '@/lib/routingProfiles'
import { fmtDist, fmtDurationMin } from '@/lib/format'
import ProfileEditor from './ProfileEditor'

function formatElevation(m: number): string {
  return `${Math.round(m)} m`
}


function GripIcon() {
  return (
    <svg
      className="w-3 h-3 text-neutral-300 dark:text-neutral-600 cursor-grab active:cursor-grabbing flex-shrink-0"
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
  onMoveUp: (id: string) => void
  onMoveDown: (id: string) => void
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
  onMoveUp,
  onMoveDown,
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
        'group flex items-start gap-1.5 py-1 rounded transition-colors',
        isDragging ? 'opacity-40' : '',
        isDragOver ? 'bg-accent-100/40 dark:bg-accent-DEFAULT/10 ring-1 ring-accent-DEFAULT/30' : '',
      ].filter(Boolean).join(' ')}
    >
      {/* Grip handle for drag-to-reorder */}
      <div
        draggable
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
        className="flex-shrink-0 pt-1 opacity-0 group-hover:opacity-100 transition-opacity cursor-grab active:cursor-grabbing"
        title="Verschieben"
      >
        <GripIcon />
      </div>

      {/* Up/Down arrows */}
      <div className="flex flex-col gap-0.5 pt-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
        <button
          onClick={() => onMoveUp(waypoint.id)}
          disabled={index === 0}
          className="p-0.5 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 disabled:opacity-20 cursor-pointer disabled:cursor-not-allowed"
          title="Nach oben"
        >
          <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M14.77 12.79a.75.75 0 01-1.06-.02L10 8.832 6.29 12.77a.75.75 0 11-1.08-1.04l4.25-4.5a.75.75 0 011.08 0l4.25 4.5a.75.75 0 01-.02 1.06z" clipRule="evenodd" />
          </svg>
        </button>
        <button
          onClick={() => onMoveDown(waypoint.id)}
          disabled={index === total - 1}
          className="p-0.5 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 disabled:opacity-20 cursor-pointer disabled:cursor-not-allowed"
          title="Nach unten"
        >
          <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
          </svg>
        </button>
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
          {/* Remove button */}
          {total > 2 && (
            <button
              onClick={() => onRemove(waypoint.id)}
              className="p-0.5 text-neutral-300 hover:text-red-500 dark:text-neutral-600 dark:hover:text-red-400 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer flex-shrink-0"
              title="Wegpunkt entfernen"
            >
              <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
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
              <span className="text-green-600 dark:text-green-500">↑{formatElevation(segmentAfter.elevation_gain)}</span>
            )}
            {segmentAfter.elevation_loss > 0 && (
              <span className="text-red-500 dark:text-red-400">↓{formatElevation(segmentAfter.elevation_loss)}</span>
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

const BROUTER_PROFILES = [
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
  onMoveWaypointUp: (id: string) => void
  onMoveWaypointDown: (id: string) => void
  onReorderWaypoints: (waypoints: RouteWaypoint[]) => void
  onRecalculateAll: () => void
  onToggleSegmentMode: (waypointId: string) => void
  onToggleHeatmap: (enabled: boolean) => void
  onClearRoute: () => void
  onSave: () => void              // opens RouteSaveDialog in parent
  onSuggest: () => void           // opens RouteSuggestionDialog in parent
  onClose: () => void
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
  onMoveWaypointUp,
  onMoveWaypointDown,
  onReorderWaypoints,
  onRecalculateAll,
  onToggleSegmentMode,
  onToggleHeatmap,
  onClearRoute,
  onSave,
  onSuggest,
  onClose,
  editingRoute,
  isSaving = false,
}: RoutePlannerPanelProps) {
  const { addToast } = useToast()
  const [collapsed, setCollapsed] = useState(false)
  const dragIndexRef = useRef<number | null>(null)
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null)
  const [dragOverIndex, setDragOverIndex] = useState<number | null>(null)

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

  // Find the segment between waypoint[i] and waypoint[i+1]
  function segmentAfterWaypoint(waypoint: RouteWaypoint, index: number): RouteSegment | undefined {
    if (index >= waypoints.length - 1) return undefined
    const nextWaypoint = waypoints[index + 1]
    return segments.find(
      s => s.from_waypoint === waypoint.id && s.to_waypoint === nextWaypoint.id
    )
  }

  return (
    <div className="absolute top-4 left-4 z-10 w-72">
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-lg overflow-hidden">

        {/* Header */}
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-neutral-100 dark:border-neutral-800">
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
          <div className="flex flex-col">

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
              <ProfileEditor
                config={profileConfig}
                onChange={onProfileChange}
              />
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
                  {BROUTER_PROFILES.map(p => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
              </div>
            )}

            {/* Waypoint list */}
            <div className="px-3 pt-2 pb-1 max-h-56 overflow-y-auto">
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
                      onMoveUp={onMoveWaypointUp}
                      onMoveDown={onMoveWaypointDown}
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

            {/* Route statistics */}
            {hasRoute && (
              <div className="mx-3 mb-2 rounded-lg bg-neutral-50 dark:bg-neutral-800/60 px-3 py-2 grid grid-cols-2 gap-x-4 gap-y-1">
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
                    {totalElevationGain > 0 ? `↑ ${formatElevation(totalElevationGain)}` : '—'}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] text-red-500 dark:text-red-400 uppercase tracking-wide">Abstieg</div>
                  <div className="text-sm font-semibold text-red-600 dark:text-red-400">
                    {totalElevationLoss > 0 ? `↓ ${formatElevation(totalElevationLoss)}` : '—'}
                  </div>
                </div>
              </div>
            )}

            {/* Heatmap toggle */}
            <div className="px-3 py-2 border-t border-neutral-100 dark:border-neutral-800 flex items-center justify-between">
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
                  Oder nutze "Route vorschlagen" fur eine KI-generierte Route.
                </p>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
