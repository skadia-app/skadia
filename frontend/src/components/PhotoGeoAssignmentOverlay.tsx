import { useEffect, useRef, useCallback, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { patchActivityPhotoLocation, type ActivityPhoto } from '@/api/activityPhotos'
import { useToast } from '@/lib/toast'
import AuthenticatedImage from '@/components/AuthenticatedImage'

// ── Types ─────────────────────────────────────────────────────────────────────

interface Props {
  activityId: string
  /** GPS track coordinates as [lng, lat] pairs */
  trackCoords: [number, number][]
  /** Parallel array of timestamps (ISO strings) for each track point */
  trackTimes: string[]
  /** Photos that currently have no GPS position */
  unpositionedPhotos: ActivityPhoto[]
  onClose: () => void
}

// ── Haversine distance (meters) ───────────────────────────────────────────────

function haversine(lat1: number, lng1: number, lat2: number, lng2: number): number {
  const R = 6371000
  const dLat = ((lat2 - lat1) * Math.PI) / 180
  const dLng = ((lng2 - lng1) * Math.PI) / 180
  const a =
    Math.sin(dLat / 2) ** 2 +
    Math.cos((lat1 * Math.PI) / 180) *
      Math.cos((lat2 * Math.PI) / 180) *
      Math.sin(dLng / 2) ** 2
  return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a))
}

/** Find the nearest track point index to a given [lng, lat] */
function nearestTrackPoint(
  coords: [number, number][],
  lng: number,
  lat: number,
): number {
  let best = 0
  let bestDist = Infinity
  for (let i = 0; i < coords.length; i++) {
    const d = haversine(lat, lng, coords[i][1], coords[i][0])
    if (d < bestDist) {
      bestDist = d
      best = i
    }
  }
  return best
}

// ── Map style (OSM raster, consistent with rest of codebase) ──────────────────

const MAP_STYLE: maplibregl.StyleSpecification = {
  version: 8,
  sources: {
    basemap: {
      type: 'raster',
      tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
      tileSize: 256,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
      maxzoom: 19,
    },
  },
  layers: [{ id: 'basemap-tiles', type: 'raster', source: 'basemap' }],
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function PhotoGeoAssignmentOverlay({
  activityId,
  trackCoords,
  trackTimes,
  unpositionedPhotos,
  onClose,
}: Props) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const dropOverlayRef = useRef<HTMLDivElement>(null)

  // Local state: photos still waiting to be positioned
  const [pending, setPending] = useState<ActivityPhoto[]>(unpositionedPhotos)
  // Photos that have been assigned during this session (shown as pins on map)
  const [assigned, setAssigned] = useState<Array<ActivityPhoto & { lat: number; lng: number }>>([])

  // Currently dragged photo id
  const draggingRef = useRef<string | null>(null)
  // Drop indicator element
  const dropIndicatorRef = useRef<HTMLDivElement | null>(null)

  // ── Patch mutation ─────────────────────────────────────────────────────────

  const patchMutation = useMutation({
    mutationFn: ({
      photo,
      lat,
      lng,
      takenAt,
    }: {
      photo: ActivityPhoto
      lat: number
      lng: number
      takenAt: string | undefined
    }) =>
      patchActivityPhotoLocation(activityId, photo.id, {
        lat,
        lng,
        taken_at: takenAt,
        location_source: 'manual',
      }),
    onSuccess: (_result, vars) => {
      const { photo, lat, lng } = vars
      // Remove from pending
      setPending(prev => prev.filter(p => p.id !== photo.id))
      // Add to assigned (for map pins)
      setAssigned(prev => [...prev, { ...photo, lat, lng }])
      // Invalidate photo list so the section reflects new location
      queryClient.invalidateQueries({ queryKey: ['activity-photos', activityId] })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Position konnte nicht gespeichert werden', duration: 0 })
    },
  })

  // ── ESC key to close ───────────────────────────────────────────────────────

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  // ── Lock body scroll ───────────────────────────────────────────────────────

  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [])

  // ── Initialize MapLibre ────────────────────────────────────────────────────

  useEffect(() => {
    const el = mapContainerRef.current
    if (!el || trackCoords.length < 2) return

    // Compute bounds from track
    const bounds = trackCoords.reduce(
      (b, c) => b.extend(c),
      new maplibregl.LngLatBounds(trackCoords[0], trackCoords[0]),
    )

    const map = new maplibregl.Map({
      container: el,
      style: MAP_STYLE,
      bounds,
      fitBoundsOptions: { padding: 40 },
    })

    mapRef.current = map
    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      // Track line
      map.addSource('track', {
        type: 'geojson',
        data: {
          type: 'Feature',
          properties: {},
          geometry: { type: 'LineString', coordinates: trackCoords },
        },
      })
      map.addLayer({
        id: 'track-line',
        type: 'line',
        source: 'track',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': '#F97316', 'line-width': 3, 'line-opacity': 0.85 },
      })

      // Assigned photo pins source (empty initially, updated later)
      map.addSource('assigned-photos', {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    })

    return () => {
      map.remove()
      mapRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []) // intentionally run only once

  // ── Update assigned-photo markers on map ──────────────────────────────────

  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return

    const source = map.getSource('assigned-photos') as maplibregl.GeoJSONSource | undefined
    if (!source) return

    source.setData({
      type: 'FeatureCollection',
      features: assigned.map(p => ({
        type: 'Feature',
        properties: { thumbUrl: p.thumb_url, id: p.id },
        geometry: { type: 'Point', coordinates: [p.lng, p.lat] },
      })),
    })

    // Ensure the symbol layer exists — if not, create a simple circle layer
    if (!map.getLayer('assigned-circles')) {
      map.addLayer({
        id: 'assigned-circles',
        type: 'circle',
        source: 'assigned-photos',
        paint: {
          'circle-radius': 8,
          'circle-color': '#ffffff',
          'circle-stroke-color': '#F97316',
          'circle-stroke-width': 2,
        },
      })
    }
  }, [assigned])

  // ── Drop indicator management ──────────────────────────────────────────────

  const showDropIndicator = useCallback((clientX: number, clientY: number) => {
    const mapEl = mapContainerRef.current
    if (!mapEl) return
    const rect = mapEl.getBoundingClientRect()
    const x = clientX - rect.left
    const y = clientY - rect.top

    let indicator = dropIndicatorRef.current
    if (!indicator) {
      indicator = document.createElement('div')
      indicator.style.cssText = `
        position: absolute;
        pointer-events: none;
        z-index: 10;
        width: 24px;
        height: 24px;
        transform: translate(-50%, -50%);
      `
      indicator.innerHTML = `
        <svg viewBox="0 0 24 24" fill="none" stroke="#F97316" stroke-width="2.5" xmlns="http://www.w3.org/2000/svg">
          <line x1="12" y1="0" x2="12" y2="24"/>
          <line x1="0" y1="12" x2="24" y2="12"/>
          <circle cx="12" cy="12" r="4" fill="#F97316" opacity="0.7"/>
        </svg>
      `
      mapEl.style.position = 'relative'
      mapEl.appendChild(indicator)
      dropIndicatorRef.current = indicator
    }
    indicator.style.left = `${x}px`
    indicator.style.top = `${y}px`
    indicator.style.display = 'block'
  }, [])

  const hideDropIndicator = useCallback(() => {
    if (dropIndicatorRef.current) {
      dropIndicatorRef.current.style.display = 'none'
    }
  }, [])

  // ── Drag & Drop handlers ───────────────────────────────────────────────────

  function handleThumbnailDragStart(e: React.DragEvent<HTMLDivElement>, photoId: string) {
    draggingRef.current = photoId
    e.dataTransfer.effectAllowed = 'move'
    // Drag image: use the thumbnail itself
    if (e.currentTarget instanceof HTMLElement) {
      e.dataTransfer.setDragImage(e.currentTarget, 48, 48)
    }
  }

  function handleThumbnailDragEnd() {
    draggingRef.current = null
    hideDropIndicator()
  }

  function handleMapDragOver(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    showDropIndicator(e.clientX, e.clientY)
  }

  function handleMapDragLeave() {
    hideDropIndicator()
  }

  function handleMapDrop(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault()
    hideDropIndicator()

    const photoId = draggingRef.current
    if (!photoId) return
    const photo = pending.find(p => p.id === photoId)
    if (!photo) return

    const map = mapRef.current
    if (!map) return

    const rect = e.currentTarget.getBoundingClientRect()
    const x = e.clientX - rect.left
    const y = e.clientY - rect.top
    const lngLat = map.unproject([x, y])

    const nearestIdx = nearestTrackPoint(trackCoords, lngLat.lng, lngLat.lat)
    const takenAt = trackTimes[nearestIdx]

    patchMutation.mutate({
      photo,
      lat: lngLat.lat,
      lng: lngLat.lng,
      takenAt,
    })
  }

  // ── Render ─────────────────────────────────────────────────────────────────

  const hasTrack = trackCoords.length >= 2

  return (
    <div className="fixed inset-0 z-[200] flex flex-col bg-neutral-950/95">
      {/* Header bar */}
      <div className="flex items-center justify-between px-4 py-2 bg-neutral-900 border-b border-neutral-800 shrink-0">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-neutral-100">
            Fotos auf Karte zuweisen
          </span>
          {pending.length > 0 && (
            <span className="text-xs text-neutral-400">
              {pending.length} {pending.length === 1 ? 'Foto' : 'Fotos'} ohne Position
            </span>
          )}
          {patchMutation.isPending && (
            <span className="text-xs text-accent-DEFAULT animate-pulse">Speichern...</span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {assigned.length > 0 && (
            <span className="text-xs text-green-400">
              {assigned.length} {assigned.length === 1 ? 'Foto' : 'Fotos'} positioniert
            </span>
          )}
          <button
            onClick={onClose}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg bg-neutral-800 hover:bg-neutral-700 text-neutral-200 transition-colors cursor-pointer"
          >
            Fertig
          </button>
          <button
            onClick={onClose}
            className="w-7 h-7 flex items-center justify-center rounded-lg hover:bg-neutral-700 text-neutral-400 hover:text-neutral-100 transition-colors cursor-pointer"
            title="Schliessen (Esc)"
          >
            <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        </div>
      </div>

      {/* Main content */}
      <div className="flex flex-1 min-h-0">
        {/* Map area (~75% width) */}
        <div className="flex-1 relative">
          {!hasTrack ? (
            <div className="absolute inset-0 flex items-center justify-center">
              <span className="text-neutral-500 text-sm">Kein GPS-Track vorhanden</span>
            </div>
          ) : (
            <div
              ref={mapContainerRef}
              className="absolute inset-0"
              onDragOver={handleMapDragOver}
              onDragLeave={handleMapDragLeave}
              onDrop={handleMapDrop}
            />
          )}

          {/* Drag hint overlay, shown when dragging over map */}
          {hasTrack && pending.length > 0 && (
            <div
              ref={dropOverlayRef}
              className="absolute bottom-4 left-1/2 -translate-x-1/2 pointer-events-none"
            >
              <div className="px-3 py-1.5 rounded-lg bg-black/60 text-white text-xs backdrop-blur-sm">
                Foto hierher ziehen, um Position zu setzen
              </div>
            </div>
          )}
        </div>

        {/* Photo sidebar (~25% width) */}
        <div className="w-56 shrink-0 flex flex-col border-l border-neutral-800 bg-neutral-900">
          <div className="px-3 pt-3 pb-2 text-xs font-medium text-neutral-400 uppercase tracking-wider shrink-0">
            Ohne Position
          </div>

          {pending.length === 0 ? (
            <div className="flex-1 flex flex-col items-center justify-center gap-2 px-4">
              <svg className="w-8 h-8 text-green-500" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              <span className="text-xs text-neutral-400 text-center">Alle Fotos positioniert</span>
            </div>
          ) : (
            <div className="flex-1 overflow-y-auto px-2 pb-2 space-y-2">
              {pending.map(photo => (
                <div
                  key={photo.id}
                  draggable
                  onDragStart={e => handleThumbnailDragStart(e, photo.id)}
                  onDragEnd={handleThumbnailDragEnd}
                  className="relative rounded-lg overflow-hidden border border-neutral-700 hover:border-accent-DEFAULT transition-colors cursor-grab active:cursor-grabbing select-none"
                  title={`${photo.original_filename} — auf Karte ziehen`}
                >
                  <AuthenticatedImage
                    src={photo.thumb_url}
                    alt={photo.original_filename}
                    className="w-full h-28 object-contain bg-neutral-950 pointer-events-none"
                  />
                  {/* Drag handle hint */}
                  <div className="absolute inset-0 flex items-center justify-center opacity-0 hover:opacity-100 transition-opacity bg-black/30">
                    <svg className="w-6 h-6 text-white drop-shadow" viewBox="0 0 20 20" fill="currentColor">
                      <path fillRule="evenodd" d="M10 3a1 1 0 00-1 1v1.586l-.293-.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l2-2a1 1 0 00-1.414-1.414L11 5.586V4a1 1 0 00-1-1zM5.293 11.293a1 1 0 011.414 0L10 14.586l3.293-3.293a1 1 0 011.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clipRule="evenodd" />
                    </svg>
                  </div>
                  {/* Filename label */}
                  <div className="absolute bottom-0 inset-x-0 bg-black/60 px-1.5 py-0.5">
                    <p className="text-[10px] text-white truncate">{photo.original_filename}</p>
                  </div>
                  {/* Pending save indicator */}
                  {patchMutation.isPending && (
                    <div className="absolute inset-0 bg-black/40 flex items-center justify-center">
                      <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
