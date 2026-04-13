import { useRef, useState, useCallback } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Map as MapIcon,
  ArrowUpRight,
  Download,
  Trash2,
  Clock,
  TrendingUp,
  TrendingDown,
  Route,
  MoreVertical,
} from 'lucide-react'
import { deleteRoute, exportRouteGPX, type PlannedRoute } from '@/api/routes'
import { fmtDist } from '@/lib/format'
import { sportLabel, sportColor } from '@/lib/sportTaxonomy'
import { useToast } from '@/lib/toast'
import { useModal } from '@/lib/modal'
import { triggerBlobDownload } from '@/lib/utils'

// ── Mini-map SVG ──────────────────────────────────────────────────────────────

interface RoutePreviewProps {
  segments: PlannedRoute['segments']
  color: string
}

function RoutePreviewSVG({ segments, color }: RoutePreviewProps) {
  // Collect all coordinate pairs from segment geometries
  const allCoords: [number, number][] = []
  for (const seg of segments) {
    if (seg.geometry?.coordinates) {
      for (const c of seg.geometry.coordinates) {
        allCoords.push([c[0] as number, c[1] as number])
      }
    }
  }

  if (allCoords.length < 2) {
    return (
      <div className="w-full h-full flex items-center justify-center">
        <Route className="w-8 h-8 text-neutral-600" />
      </div>
    )
  }

  // Bounding box
  const lons = allCoords.map(c => c[0])
  const lats = allCoords.map(c => c[1])
  const minLon = Math.min(...lons)
  const maxLon = Math.max(...lons)
  const minLat = Math.min(...lats)
  const maxLat = Math.max(...lats)
  const lonRange = maxLon - minLon || 0.001
  const latRange = maxLat - minLat || 0.001

  const W = 200
  const H = 120
  const PAD = 12

  const toSVG = (lon: number, lat: number): [number, number] => [
    PAD + ((lon - minLon) / lonRange) * (W - PAD * 2),
    H - PAD - ((lat - minLat) / latRange) * (H - PAD * 2),
  ]

  // Build polyline points per segment
  const polylines = segments
    .filter(seg => seg.geometry?.coordinates && seg.geometry.coordinates.length >= 2)
    .map((seg, i) => {
      const pts = seg.geometry!.coordinates
        .map(c => toSVG(c[0] as number, c[1] as number).join(','))
        .join(' ')
      return <polyline key={i} points={pts} fill="none" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    })

  // Start and end markers
  const [sx, sy] = toSVG(allCoords[0][0], allCoords[0][1])
  const [ex, ey] = toSVG(allCoords[allCoords.length - 1][0], allCoords[allCoords.length - 1][1])

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-full">
      {polylines}
      {/* Start dot */}
      <circle cx={sx} cy={sy} r={4} fill={color} opacity={0.9} />
      {/* End dot */}
      <circle cx={ex} cy={ey} r={4} fill="white" stroke={color} strokeWidth="1.5" opacity={0.9} />
    </svg>
  )
}

// ── Difficulty Badge ──────────────────────────────────────────────────────────

const DIFFICULTY_CONFIG: Record<string, { label: string; className: string }> = {
  easy:     { label: 'Leicht',       className: 'bg-green-500/15 text-green-400 border border-green-500/30' },
  moderate: { label: 'Mittel',       className: 'bg-yellow-500/15 text-yellow-400 border border-yellow-500/30' },
  hard:     { label: 'Schwer',       className: 'bg-orange-500/15 text-orange-400 border border-orange-500/30' },
  expert:   { label: 'Experte',      className: 'bg-red-500/15 text-red-400 border border-red-500/30' },
}

function RouteDifficultyBadge({ difficulty }: { difficulty?: string }) {
  if (!difficulty) return null
  const cfg = DIFFICULTY_CONFIG[difficulty]
  const label = cfg?.label ?? difficulty
  const className = cfg?.className ?? 'bg-neutral-500/15 text-neutral-400 border border-neutral-500/30'

  return (
    <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold leading-none ${className}`}>
      {label}
    </span>
  )
}

// ── Time formatter ────────────────────────────────────────────────────────────

function fmtMinutes(min?: number): string {
  if (min == null) return '--'
  const h = Math.floor(min / 60)
  const m = Math.round(min % 60)
  if (h === 0) return `${m} min`
  return `${h}:${m.toString().padStart(2, '0')} h`
}

function fmtRelative(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const days = Math.floor(diff / 86_400_000)
  if (days === 0) return 'Heute'
  if (days === 1) return 'Gestern'
  if (days < 7) return `vor ${days} Tagen`
  if (days < 30) return `vor ${Math.floor(days / 7)} Wo.`
  if (days < 365) return `vor ${Math.floor(days / 30)} Mo.`
  return `vor ${Math.floor(days / 365)} J.`
}

// ── RouteCard ─────────────────────────────────────────────────────────────────

interface RouteCardProps {
  route: PlannedRoute
  onDeleted?: (id: string) => void
}

export default function RouteCard({ route, onDeleted }: RouteCardProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { openModal } = useModal()
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  const color = sportColor(route.sport_type)
  const label = sportLabel(route.sport_type)

  // Close menu on outside click
  useClickOutside(menuRef, () => setMenuOpen(false), menuOpen)

  // Delete mutation
  const deleteMutation = useMutation({
    mutationFn: () => deleteRoute(route.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['routes'] })
      onDeleted?.(route.id)
      addToast({ type: 'success', title: 'Route gelöscht' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Löschen', duration: 0 })
    },
  })

  // GPX export
  const handleExport = useCallback(async (e: React.MouseEvent) => {
    e.stopPropagation()
    setMenuOpen(false)
    try {
      const { blob, filename } = await exportRouteGPX(route.id)
      triggerBlobDownload(blob, filename ?? `route-${route.id}.gpx`)
    } catch {
      addToast({ type: 'error', title: 'GPX-Export fehlgeschlagen', duration: 0 })
    }
  }, [route.id, addToast])

  const handleDelete = useCallback((e: React.MouseEvent) => {
    e.stopPropagation()
    setMenuOpen(false)
    openModal({
      title: 'Route löschen?',
      message: `Route "${route.name}" wirklich löschen? Diese Aktion kann nicht rückgängig gemacht werden.`,
      buttons: [
        { label: 'Abbrechen', variant: 'default', onClick: () => {} },
        { label: 'Löschen', variant: 'destructive', onClick: () => deleteMutation.mutate() },
      ],
    })
  }, [route.name, deleteMutation, openModal])

  const handleOpenPlanner = useCallback((e: React.MouseEvent) => {
    e.stopPropagation()
    navigate(`/routes/${route.id}`)
  }, [navigate, route.id])

  return (
    <div
      className="group relative flex flex-col overflow-hidden rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 shadow-sm hover:shadow-md hover:border-neutral-300 dark:hover:border-neutral-700 transition-all duration-200 cursor-pointer"
      onClick={() => navigate(`/routes/${route.id}`)}
      role="article"
      aria-label={`Route: ${route.name}`}
    >
      {/* Mini-map area */}
      <div
        className="relative h-28 bg-neutral-100 dark:bg-neutral-800 overflow-hidden"
        style={{ borderBottom: `2px solid ${color}22` }}
      >
        <RoutePreviewSVG segments={route.segments} color={color} />

        {/* Gradient overlay bottom */}
        <div className="absolute inset-x-0 bottom-0 h-8 bg-gradient-to-t from-white/80 dark:from-neutral-900/80 to-transparent pointer-events-none" />

        {/* Sport color bar top */}
        <div
          className="absolute top-0 left-0 right-0 h-0.5"
          style={{ backgroundColor: color }}
        />

        {/* Difficulty badge — top-right */}
        <div className="absolute top-2 right-2">
          <RouteDifficultyBadge difficulty={route.difficulty} />
        </div>

        {/* Hover actions overlay */}
        <div className="absolute inset-0 flex items-center justify-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity duration-150 bg-black/20 dark:bg-black/40">
          <button
            onClick={handleOpenPlanner}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 text-xs font-medium shadow-md hover:bg-neutral-50 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
            title="Im Planer öffnen"
          >
            <ArrowUpRight className="w-3.5 h-3.5" />
            Öffnen
          </button>
          <button
            onClick={handleExport}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 text-xs font-medium shadow-md hover:bg-neutral-50 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
            title="GPX exportieren"
          >
            <Download className="w-3.5 h-3.5" />
            GPX
          </button>
        </div>
      </div>

      {/* Card body */}
      <div className="flex flex-col gap-2 p-3">
        {/* Header row: name + menu */}
        <div className="flex items-start gap-2">
          <div className="flex-1 min-w-0">
            <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100 truncate leading-snug">
              {route.name}
            </h3>
            <div className="flex items-center gap-1.5 mt-0.5">
              {/* Sport type indicator */}
              <span
                className="inline-block w-2 h-2 rounded-full flex-shrink-0"
                style={{ backgroundColor: color }}
              />
              <span className="text-[11px] text-neutral-500 dark:text-neutral-400 truncate">
                {label}
              </span>
              <span className="text-[11px] text-neutral-400 dark:text-neutral-600">·</span>
              <span className="text-[11px] text-neutral-400 dark:text-neutral-500">
                {fmtRelative(route.updated_at)}
              </span>
            </div>
          </div>

          {/* Context menu */}
          <div className="relative flex-shrink-0" ref={menuRef}>
            <button
              onClick={e => { e.stopPropagation(); setMenuOpen(v => !v) }}
              className="p-1 rounded-md text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors opacity-0 group-hover:opacity-100 focus:opacity-100 cursor-pointer"
              aria-label="Mehr Optionen"
            >
              <MoreVertical className="w-4 h-4" />
            </button>
            {menuOpen && (
              <div className="absolute right-0 top-full mt-1 z-50 min-w-[140px] rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 shadow-lg py-1">
                <button
                  onClick={handleOpenPlanner}
                  className="w-full flex items-center gap-2 px-3 py-1.5 text-sm text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
                >
                  <ArrowUpRight className="w-3.5 h-3.5" />
                  Im Planer öffnen
                </button>
                <button
                  onClick={handleExport}
                  className="w-full flex items-center gap-2 px-3 py-1.5 text-sm text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
                >
                  <Download className="w-3.5 h-3.5" />
                  GPX exportieren
                </button>
                <div className="my-1 h-px bg-neutral-100 dark:bg-neutral-700" />
                <button
                  onClick={handleDelete}
                  disabled={deleteMutation.isPending}
                  className="w-full flex items-center gap-2 px-3 py-1.5 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  Löschen
                </button>
              </div>
            )}
          </div>
        </div>

        {/* Stats row */}
        <div className="flex items-center gap-3 text-xs text-neutral-600 dark:text-neutral-400">
          {/* Distance */}
          <span className="flex items-center gap-1">
            <MapIcon className="w-3 h-3 flex-shrink-0 text-neutral-400" />
            {fmtDist(route.total_distance_m)}
          </span>

          {/* Elevation gain */}
          {route.total_elevation_gain != null && route.total_elevation_gain > 0 && (
            <span className="flex items-center gap-0.5">
              <TrendingUp className="w-3 h-3 flex-shrink-0 text-green-500" />
              {Math.round(route.total_elevation_gain)} m
            </span>
          )}

          {/* Elevation loss */}
          {route.total_elevation_loss != null && route.total_elevation_loss > 0 && (
            <span className="flex items-center gap-0.5">
              <TrendingDown className="w-3 h-3 flex-shrink-0 text-red-400" />
              {Math.round(route.total_elevation_loss)} m
            </span>
          )}

          {/* Estimated time */}
          {route.estimated_time_min != null && (
            <span className="flex items-center gap-1 ml-auto">
              <Clock className="w-3 h-3 flex-shrink-0" />
              {fmtMinutes(route.estimated_time_min)}
            </span>
          )}
        </div>

        {/* Tags */}
        {route.tags.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {route.tags.slice(0, 4).map(tag => (
              <span
                key={tag.id}
                className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium leading-none"
                style={{
                  backgroundColor: tag.color ? `${tag.color}22` : undefined,
                  color: tag.color ?? undefined,
                  border: tag.color ? `1px solid ${tag.color}44` : '1px solid rgb(115 115 115 / 0.3)',
                }}
              >
                {tag.name}
              </span>
            ))}
            {route.tags.length > 4 && (
              <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] text-neutral-400 border border-neutral-300 dark:border-neutral-700">
                +{route.tags.length - 4}
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
