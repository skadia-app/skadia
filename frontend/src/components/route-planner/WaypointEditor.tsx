import { useRef, useState } from 'react'
import type { RouteWaypoint, RouteSegment } from '@/api/routes'

// ─── Icons (inline SVG, no external icon lib dependency) ────────────────────

function GripIcon() {
  return (
    <svg
      className="w-4 h-4 text-neutral-400 dark:text-neutral-500 cursor-grab active:cursor-grabbing"
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

function TrashIcon() {
  return (
    <svg
      className="w-4 h-4"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <polyline points="3 6 5 6 21 6" />
      <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
      <path d="M10 11v6M14 11v6" />
      <path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2" />
    </svg>
  )
}

function MapPinIcon({ type }: { type: 'start' | 'via' | 'end' }) {
  const color =
    type === 'start'
      ? 'text-emerald-500'
      : type === 'end'
      ? 'text-red-500'
      : 'text-accent-DEFAULT'
  return (
    <svg
      className={`w-5 h-5 shrink-0 ${color}`}
      viewBox="0 0 24 24"
      fill="currentColor"
    >
      {type === 'start' ? (
        // Flag icon for start
        <path d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1v12zm0 7v-7" />
      ) : type === 'end' ? (
        // Checkered flag / filled pin for end
        <>
          <path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z" />
          <path
            d="M12 6.5a2.5 2.5 0 1 1 0 5 2.5 2.5 0 0 1 0-5z"
            fill="white"
          />
        </>
      ) : (
        // Small circle for via
        <>
          <circle cx="12" cy="9" r="7" opacity="0.15" />
          <circle cx="12" cy="9" r="3.5" />
        </>
      )}
    </svg>
  )
}

function ToggleIcon({ on }: { on: boolean }) {
  return (
    <svg
      className={`w-4 h-4 transition-colors ${on ? 'text-accent-DEFAULT' : 'text-neutral-400 dark:text-neutral-600'}`}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {on ? (
        // Routed: connected path
        <>
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z" />
          <path d="M8 12h8M12 8l4 4-4 4" />
        </>
      ) : (
        // Offroad: dashes
        <>
          <line x1="2" y1="12" x2="5" y2="12" strokeDasharray="3 2" />
          <line x1="8" y1="12" x2="11" y2="12" strokeDasharray="3 2" />
          <line x1="14" y1="12" x2="17" y2="12" strokeDasharray="3 2" />
          <line x1="20" y1="12" x2="22" y2="12" strokeDasharray="3 2" />
        </>
      )}
    </svg>
  )
}

// ─── Segment connector between waypoints ─────────────────────────────────────

interface SegmentConnectorProps {
  segment?: RouteSegment
  onToggleMode?: () => void
}

function SegmentConnector({ segment, onToggleMode }: SegmentConnectorProps) {
  const isOffroad = segment?.mode === 'offroad'
  const isRouted = segment?.mode === 'routed'

  return (
    <div className="flex items-center gap-2 pl-6 py-0.5 select-none">
      {/* Vertical line */}
      <div className="flex flex-col items-center w-5 shrink-0">
        {isOffroad ? (
          // Dashed line for offroad
          <div className="w-0.5 h-6 flex flex-col gap-0.5">
            {[0, 1, 2, 3].map((i) => (
              <div
                key={i}
                className="flex-1 bg-amber-500/70 dark:bg-amber-400/70 rounded-full"
              />
            ))}
          </div>
        ) : (
          // Solid line for routed / unknown
          <div className="w-0.5 h-6 bg-neutral-300 dark:bg-neutral-600 rounded-full" />
        )}
      </div>

      {/* Mode toggle button */}
      {onToggleMode && (
        <button
          type="button"
          onClick={onToggleMode}
          title={isOffroad ? 'Offroad — klicken für geroutete Strecke' : 'Geroutet — klicken für Offroad'}
          className="flex items-center gap-1.5 px-2 py-0.5 rounded text-xs
            text-neutral-500 dark:text-neutral-400
            hover:bg-neutral-100 dark:hover:bg-neutral-800
            transition-colors"
        >
          <ToggleIcon on={isRouted} />
          <span>{isOffroad ? 'Offroad' : isRouted ? 'Geroutet' : 'Direkt'}</span>
          {(segment?.distance_m ?? 0) > 0 && (
            <span className="ml-1 text-neutral-400 dark:text-neutral-500">
              {segment!.distance_m >= 1000
                ? `${(segment!.distance_m / 1000).toFixed(1)} km`
                : `${Math.round(segment!.distance_m)} m`}
            </span>
          )}
        </button>
      )}
    </div>
  )
}

// ─── Single waypoint row ──────────────────────────────────────────────────────

interface WaypointRowProps {
  waypoint: RouteWaypoint
  index: number
  isDragging: boolean
  isDragOver: boolean
  onDelete: () => void
  onDragStart: (e: React.DragEvent) => void
  onDragOver: (e: React.DragEvent) => void
  onDragLeave: () => void
  onDrop: (e: React.DragEvent) => void
  onDragEnd: () => void
}

function WaypointRow({
  waypoint,
  index,
  isDragging,
  isDragOver,
  onDelete,
  onDragStart,
  onDragOver,
  onDragLeave,
  onDrop,
  onDragEnd,
}: WaypointRowProps) {
  const label =
    waypoint.type === 'start'
      ? 'Start'
      : waypoint.type === 'end'
      ? 'Ziel'
      : `Via ${index}`

  const displayName = waypoint.poi_name ?? label

  const [lat, lng] = waypoint.latlng
  const coordStr = `${lat.toFixed(5)}, ${lng.toFixed(5)}`

  return (
    <div
      draggable
      onDragStart={onDragStart}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
      onDragEnd={onDragEnd}
      className={[
        'flex items-center gap-2 px-2 py-1.5 rounded-lg',
        'transition-all duration-150',
        isDragging ? 'opacity-40 scale-[0.98]' : '',
        isDragOver
          ? 'bg-accent-100/60 dark:bg-accent-DEFAULT/15 ring-1 ring-accent-DEFAULT/50'
          : 'hover:bg-neutral-50 dark:hover:bg-neutral-800/50',
      ]
        .filter(Boolean)
        .join(' ')}
    >
      {/* Drag handle */}
      <div className="shrink-0 p-0.5" title="Verschieben">
        <GripIcon />
      </div>

      {/* Waypoint type icon */}
      <div className="shrink-0">
        <MapPinIcon type={waypoint.type} />
      </div>

      {/* Name + coords */}
      <div className="flex-1 min-w-0">
        <div className="text-sm font-medium text-neutral-800 dark:text-neutral-200 truncate">
          {displayName}
        </div>
        <div className="text-xs text-neutral-400 dark:text-neutral-500 font-mono truncate">
          {coordStr}
        </div>
      </div>

      {/* POI type badge */}
      {waypoint.poi_type && (
        <span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded-full bg-neutral-100 dark:bg-neutral-700 text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          {waypoint.poi_type.replace(/_/g, ' ')}
        </span>
      )}

      {/* Delete button — not shown for start/end if only 2 waypoints */}
      <button
        type="button"
        onClick={onDelete}
        title="Wegpunkt entfernen"
        className="shrink-0 p-1 rounded text-neutral-400 hover:text-red-500 dark:hover:text-red-400
          hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors"
      >
        <TrashIcon />
      </button>
    </div>
  )
}

// ─── Main component ───────────────────────────────────────────────────────────

export interface WaypointEditorProps {
  waypoints: RouteWaypoint[]
  segments?: RouteSegment[]
  onChange: (waypoints: RouteWaypoint[]) => void
  onSegmentModeToggle?: (fromId: string, toId: string) => void
  className?: string
}

export default function WaypointEditor({
  waypoints,
  segments = [],
  onChange,
  onSegmentModeToggle,
  className = '',
}: WaypointEditorProps) {
  const dragIndexRef = useRef<number | null>(null)
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null)
  const [dragOverIndex, setDragOverIndex] = useState<number | null>(null)

  // ── DnD handlers ──

  function handleDragStart(e: React.DragEvent, index: number) {
    dragIndexRef.current = index
    setDraggingIndex(index)
    e.dataTransfer.effectAllowed = 'move'
    // Needed for Firefox
    e.dataTransfer.setData('text/plain', String(index))
  }

  function handleDragOver(e: React.DragEvent, index: number) {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    if (dragIndexRef.current !== index) {
      setDragOverIndex(index)
    }
  }

  function handleDragLeave() {
    setDragOverIndex(null)
  }

  function handleDrop(_e: React.DragEvent, dropIndex: number) {
    const from = dragIndexRef.current
    if (from === null || from === dropIndex) {
      setDragOverIndex(null)
      return
    }

    const reordered = [...waypoints]
    const [moved] = reordered.splice(from, 1)
    reordered.splice(dropIndex, 0, moved)

    // Re-assign types: first = start, last = end, rest = via
    const reassigned = reordered.map((wp, i) => ({
      ...wp,
      type:
        i === 0
          ? ('start' as const)
          : i === reordered.length - 1
          ? ('end' as const)
          : ('via' as const),
    }))

    onChange(reassigned)
    setDragOverIndex(null)
  }

  function handleDragEnd() {
    dragIndexRef.current = null
    setDraggingIndex(null)
    setDragOverIndex(null)
  }

  // ── Delete ──

  function handleDelete(index: number) {
    const next = waypoints.filter((_, i) => i !== index)
    // Re-assign types
    const reassigned = next.map((wp, i) => ({
      ...wp,
      type:
        i === 0
          ? ('start' as const)
          : i === next.length - 1
          ? ('end' as const)
          : ('via' as const),
    }))
    onChange(reassigned)
  }

  // ── Segment lookup ──

  function findSegment(fromId: string, toId: string): RouteSegment | undefined {
    return segments.find(
      (s) => s.from_waypoint === fromId && s.to_waypoint === toId,
    )
  }

  // ── Empty state ──

  if (waypoints.length === 0) {
    return (
      <div
        className={`flex flex-col items-center justify-center py-8 text-sm
          text-neutral-400 dark:text-neutral-500 ${className}`}
      >
        <svg
          className="w-8 h-8 mb-2 opacity-40"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth={1.5}
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"
          />
          <circle cx="12" cy="9" r="2.5" />
        </svg>
        <span>Klicke auf die Karte, um Wegpunkte zu setzen</span>
      </div>
    )
  }

  return (
    <div className={`flex flex-col ${className}`} role="list" aria-label="Wegpunkte">
      {waypoints.map((wp, i) => (
        <div key={wp.id} role="listitem">
          <WaypointRow
            waypoint={wp}
            index={i}
            isDragging={draggingIndex === i}
            isDragOver={dragOverIndex === i}
            onDelete={() => handleDelete(i)}
            onDragStart={(e) => handleDragStart(e, i)}
            onDragOver={(e) => handleDragOver(e, i)}
            onDragLeave={handleDragLeave}
            onDrop={(e) => handleDrop(e, i)}
            onDragEnd={handleDragEnd}
          />

          {/* Segment connector between consecutive waypoints */}
          {i < waypoints.length - 1 && (
            <SegmentConnector
              segment={findSegment(wp.id, waypoints[i + 1].id)}
              onToggleMode={
                onSegmentModeToggle
                  ? () => onSegmentModeToggle(wp.id, waypoints[i + 1].id)
                  : undefined
              }
            />
          )}
        </div>
      ))}
    </div>
  )
}
