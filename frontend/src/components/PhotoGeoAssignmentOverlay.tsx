import { useEffect, useRef, useState } from 'react'
import { useEscapeKey } from '@/hooks/useEscapeKey'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { patchActivityPhotoLocation, isVideoPhoto, type ActivityPhoto } from '@/api/activityPhotos'
import { buildRasterStyle, type BaseStyle } from '@/lib/mapStyles'
import { apiDownload } from '@/api/client'
import { useToast } from '@/lib/toast'
import AuthenticatedImage from '@/components/AuthenticatedImage'

// ── Types ─────────────────────────────────────────────────────────────────────

type PositionedPhoto = ActivityPhoto & { lat: number; lng: number }

interface Props {
  activityId: string
  /** GPS track coordinates as [lng, lat] pairs */
  trackCoords: [number, number][]
  /** Parallel array of timestamps (ISO strings) for each track point */
  trackTimes: string[]
  /** Photos that currently have no GPS position */
  unpositionedPhotos: ActivityPhoto[]
  /** Photos that already have a GPS position (shown on map, repositionable) */
  positionedPhotos?: PositionedPhoto[]
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

function nearestTrackPoint(coords: [number, number][], lng: number, lat: number): number {
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

// ── Style toggle options ──────────────────────────────────────────────────────

const OVERLAY_STYLES: Array<{ key: BaseStyle; label: string }> = [
  { key: 'osm', label: 'OSM' },
  { key: 'topo', label: 'Topo' },
  { key: 'satellite', label: 'Satellit' },
]

// ── Component ─────────────────────────────────────────────────────────────────

export default function PhotoGeoAssignmentOverlay({
  activityId,
  trackCoords,
  trackTimes,
  unpositionedPhotos,
  positionedPhotos = [],
  onClose,
}: Props) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  // Refs
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markersRef = useRef<Map<string, maplibregl.Marker>>(new Map())
  const blobUrlsRef = useRef<string[]>([])
  const dropIndicatorRef = useRef<HTMLDivElement | null>(null)
  const draggingRef = useRef<string | null>(null)

  // Refs kept in sync with latest render values (for use inside async callbacks)
  const assignedRef = useRef<PositionedPhoto[]>([])
  const addMarkerRef = useRef<((photo: PositionedPhoto, map: maplibregl.Map) => void) | null>(null)
  const patchMutateRef = useRef<((vars: PatchVars) => void) | null>(null)

  // State
  const [mapStyle, setMapStyle] = useState<BaseStyle>('osm')
  const [pending, setPending] = useState<ActivityPhoto[]>(unpositionedPhotos)
  const [assigned, setAssigned] = useState<PositionedPhoto[]>(positionedPhotos)

  // ── Mutation ──────────────────────────────────────────────────────────────

  type PatchVars = {
    photo: ActivityPhoto
    lat: number
    lng: number
    takenAt: string | undefined
    isReposition?: boolean
  }

  const patchMutation = useMutation({
    mutationFn: ({ photo, lat, lng, takenAt }: PatchVars) =>
      patchActivityPhotoLocation(activityId, photo.id, {
        lat,
        lng,
        taken_at: takenAt,
        location_source: 'manual',
      }),
    onSuccess: (_result, { photo, lat, lng, isReposition }) => {
      if (!isReposition) {
        // New assignment: remove from pending, add to assigned
        setPending(prev => prev.filter(p => p.id !== photo.id))
        const posPhoto = { ...photo, lat, lng } as PositionedPhoto
        setAssigned(prev => [...prev, posPhoto])
        // Create marker
        const map = mapRef.current
        if (map && map.loaded()) {
          addMarkerRef.current?.(posPhoto, map)
        }
      } else {
        // Reposition: update state + move existing marker
        setAssigned(prev => prev.map(a => a.id === photo.id ? { ...a, lat, lng } : a))
        markersRef.current.get(photo.id)?.setLngLat([lng, lat])
      }
      queryClient.invalidateQueries({ queryKey: ['activity-photos', activityId] })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Position konnte nicht gespeichert werden', duration: 0 })
    },
  })

  // ── Keep refs in sync ─────────────────────────────────────────────────────

  assignedRef.current = assigned
  patchMutateRef.current = patchMutation.mutate

  // ── Marker creation ───────────────────────────────────────────────────────

  function addMarkerForPhoto(photo: PositionedPhoto, map: maplibregl.Map) {
    if (markersRef.current.has(photo.id)) return

    // Custom marker element: thumbnail box + downward-pointing callout triangle
    const el = document.createElement('div')
    el.style.cssText = 'cursor:move;filter:drop-shadow(0 2px 6px rgba(0,0,0,0.6));'

    const imgBox = document.createElement('div')
    imgBox.style.cssText =
      'width:56px;height:42px;border-radius:4px;border:2px solid white;overflow:hidden;background:#333;'

    const triangle = document.createElement('div')
    triangle.style.cssText =
      'width:0;height:0;border-left:6px solid transparent;border-right:6px solid transparent;border-top:8px solid white;margin:0 auto;'

    el.appendChild(imgBox)
    el.appendChild(triangle)

    const marker = new maplibregl.Marker({ element: el, draggable: true, anchor: 'bottom' })
      .setLngLat([photo.lng, photo.lat])
      .addTo(map)

    markersRef.current.set(photo.id, marker)

    // Drag end: reposition photo
    marker.on('dragend', () => {
      const { lng, lat } = marker.getLngLat()
      const nearestIdx = nearestTrackPoint(trackCoords, lng, lat)
      patchMutateRef.current?.({ photo, lat, lng, takenAt: trackTimes[nearestIdx], isReposition: true })
    })

    // Load thumbnail async (requires auth)
    if (photo.thumb_url) {
      apiDownload(photo.thumb_url)
        .then(({ blob }) => {
          const blobUrl = URL.createObjectURL(blob)
          blobUrlsRef.current.push(blobUrl)
          const img = document.createElement('img')
          img.src = blobUrl
          img.style.cssText = 'width:100%;height:100%;object-fit:cover;'
          imgBox.innerHTML = ''
          imgBox.appendChild(img)
        })
        .catch(() => {
          // Keep gray placeholder on error
        })
    }
  }

  // Keep addMarkerRef pointing at the freshest version
  addMarkerRef.current = addMarkerForPhoto

  useEscapeKey(onClose)

  // ── Lock body scroll ───────────────────────────────────────────────────────

  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [])

  // ── Initialize MapLibre (runs once) ───────────────────────────────────────

  useEffect(() => {
    const el = mapContainerRef.current
    if (!el || trackCoords.length < 2) return

    const bounds = trackCoords.reduce(
      (b, c) => b.extend(c),
      new maplibregl.LngLatBounds(trackCoords[0], trackCoords[0]),
    )

    let fitDone = false

    const map = new maplibregl.Map({
      container: el,
      style: buildRasterStyle('osm'),
      center: [0, 0],
      zoom: 1,
    })
    mapRef.current = map
    requestAnimationFrame(() => map.resize())
    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    // Persistent style.load handler: re-add track after every style switch
    map.on('style.load', () => {
      if (!map.getSource('track')) {
        map.addSource('track', {
          type: 'geojson',
          data: {
            type: 'Feature',
            properties: {},
            geometry: { type: 'LineString', coordinates: trackCoords },
          },
        })
      }
      if (!map.getLayer('track-line')) {
        map.addLayer({
          id: 'track-line',
          type: 'line',
          source: 'track',
          layout: { 'line-join': 'round', 'line-cap': 'round' },
          paint: { 'line-color': '#F97316', 'line-width': 3, 'line-opacity': 0.85 },
        })
      }
      if (!fitDone) {
        map.fitBounds(bounds, { padding: 40 })
        fitDone = true
      }
    })

    // After initial load: create markers for already-positioned photos
    map.on('load', () => {
      for (const photo of assignedRef.current) {
        addMarkerRef.current?.(photo, map)
      }
    })

    return () => {
      markersRef.current.forEach(m => m.remove())
      markersRef.current.clear()
      blobUrlsRef.current.forEach(u => URL.revokeObjectURL(u))
      blobUrlsRef.current = []
      map.remove()
      mapRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []) // intentionally runs only once

  // ── Style change ──────────────────────────────────────────────────────────

  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    map.setStyle(buildRasterStyle(mapStyle))
  }, [mapStyle])

  // ── Drop indicator ────────────────────────────────────────────────────────

  function showDropIndicator(clientX: number, clientY: number) {
    const mapEl = mapContainerRef.current
    if (!mapEl) return
    const rect = mapEl.getBoundingClientRect()
    const x = clientX - rect.left
    const y = clientY - rect.top

    let indicator = dropIndicatorRef.current
    if (!indicator) {
      indicator = document.createElement('div')
      indicator.style.cssText =
        'position:absolute;pointer-events:none;z-index:10;width:24px;height:24px;transform:translate(-50%,-50%);'
      indicator.innerHTML =
        '<svg viewBox="0 0 24 24" fill="none" stroke="#F97316" stroke-width="2.5" xmlns="http://www.w3.org/2000/svg">' +
        '<line x1="12" y1="0" x2="12" y2="24"/>' +
        '<line x1="0" y1="12" x2="24" y2="12"/>' +
        '<circle cx="12" cy="12" r="4" fill="#F97316" opacity="0.7"/>' +
        '</svg>'
      mapEl.appendChild(indicator)
      dropIndicatorRef.current = indicator
    }
    indicator.style.left = `${x}px`
    indicator.style.top = `${y}px`
    indicator.style.display = 'block'
  }

  function hideDropIndicator() {
    if (dropIndicatorRef.current) dropIndicatorRef.current.style.display = 'none'
  }

  // ── Drag & Drop handlers ──────────────────────────────────────────────────

  function handleThumbnailDragStart(e: React.DragEvent<HTMLDivElement>, photoId: string) {
    draggingRef.current = photoId
    e.dataTransfer.effectAllowed = 'move'
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

    // Accept both unpositioned (from pending) and already-positioned (reposition)
    const photo =
      pending.find(p => p.id === photoId) ??
      assigned.find(a => a.id === photoId)
    if (!photo) return
    const isReposition = assigned.some(a => a.id === photoId)

    const map = mapRef.current
    if (!map) return

    const rect = e.currentTarget.getBoundingClientRect()
    const x = e.clientX - rect.left
    const y = e.clientY - rect.top
    const lngLat = map.unproject([x, y])

    const nearestIdx = nearestTrackPoint(trackCoords, lngLat.lng, lngLat.lat)
    const takenAt = trackTimes[nearestIdx]

    patchMutation.mutate({ photo, lat: lngLat.lat, lng: lngLat.lng, takenAt, isReposition })
  }

  // ── Render ────────────────────────────────────────────────────────────────

  const hasTrack = trackCoords.length >= 2

  return (
    <div className="fixed inset-0 z-[200] flex flex-col bg-neutral-950/95">
      {/* Header bar */}
      <div className="flex items-center justify-between px-4 py-2 bg-neutral-900 border-b border-neutral-800 shrink-0">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-neutral-100">
            Fotos &amp; Videos auf Karte zuweisen
          </span>
          {pending.length > 0 && (
            <span className="text-xs text-neutral-400">
              {pending.length} {pending.length === 1 ? 'Medium' : 'Medien'} ohne Position
            </span>
          )}
          {patchMutation.isPending && (
            <span className="text-xs text-accent-DEFAULT animate-pulse">Speichern...</span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {assigned.length > 0 && (
            <span className="text-xs text-green-400">
              {assigned.length} {assigned.length === 1 ? 'Medium' : 'Medien'} positioniert
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
        {/* Map area: mapContainerRef IS the flex item so MapLibre gets definite dimensions */}
        {!hasTrack ? (
          <div className="flex-1 flex items-center justify-center">
            <span className="text-neutral-500 text-sm">Kein GPS-Track vorhanden</span>
          </div>
        ) : (
          <div
            ref={mapContainerRef}
            className="flex-1 relative min-h-0"
            onDragOver={handleMapDragOver}
            onDragLeave={handleMapDragLeave}
            onDrop={handleMapDrop}
          >
            {/* Map style toggle */}
            <div className="absolute top-2 left-2 z-10 flex rounded-lg overflow-hidden shadow-lg">
              {OVERLAY_STYLES.map(({ key, label }) => (
                <button
                  key={key}
                  onClick={() => setMapStyle(key)}
                  className={`px-2.5 py-1 text-xs font-medium transition-colors ${
                    mapStyle === key
                      ? 'bg-accent-DEFAULT text-white'
                      : 'bg-neutral-900/90 text-neutral-300 hover:bg-neutral-800 hover:text-white'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>

            {/* Drag hint */}
            {pending.length > 0 && (
              <div className="absolute bottom-4 left-1/2 -translate-x-1/2 pointer-events-none z-10">
                <div className="px-3 py-1.5 rounded-lg bg-black/60 text-white text-xs backdrop-blur-sm">
                  Foto hierher ziehen, um Position zu setzen
                </div>
              </div>
            )}
          </div>
        )}

        {/* Sidebar */}
        <div className="w-56 shrink-0 flex flex-col border-l border-neutral-800 bg-neutral-900 overflow-y-auto">

          {/* "Ohne Position" section */}
          <div className="px-3 pt-3 pb-2 text-xs font-medium text-neutral-400 uppercase tracking-wider shrink-0">
            Ohne Position
          </div>

          {pending.length === 0 ? (
            <div className="flex items-center gap-2 px-3 py-2">
              <svg className="w-4 h-4 text-green-500 shrink-0" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              <span className="text-xs text-neutral-400">Alle positioniert</span>
            </div>
          ) : (
            <div className="px-2 pb-2 space-y-2">
              {pending.map(photo => (
                <SidebarTile
                  key={photo.id}
                  photo={photo}
                  saving={patchMutation.isPending && draggingRef.current === photo.id}
                  onDragStart={e => handleThumbnailDragStart(e, photo.id)}
                  onDragEnd={handleThumbnailDragEnd}
                />
              ))}
            </div>
          )}

          {/* "Positioniert" section — shown when any photos have a position */}
          {assigned.length > 0 && (
            <>
              <div className="px-3 pt-3 pb-2 text-xs font-medium text-neutral-400 uppercase tracking-wider shrink-0 border-t border-neutral-800">
                Positioniert
              </div>
              <div className="px-2 pb-3 space-y-2">
                {assigned.map(photo => (
                  <SidebarTile
                    key={photo.id}
                    photo={photo}
                    positioned
                    saving={false}
                    onDragStart={e => handleThumbnailDragStart(e, photo.id)}
                    onDragEnd={handleThumbnailDragEnd}
                  />
                ))}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

// ── Sidebar tile (pending or positioned) ─────────────────────────────────────

interface SidebarTileProps {
  photo: ActivityPhoto
  positioned?: boolean
  saving: boolean
  onDragStart: (e: React.DragEvent<HTMLDivElement>) => void
  onDragEnd: () => void
}

function SidebarTile({ photo, positioned, saving, onDragStart, onDragEnd }: SidebarTileProps) {
  const isVid = isVideoPhoto(photo)
  return (
    <div
      draggable
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      className={`relative rounded-lg overflow-hidden border transition-colors cursor-grab active:cursor-grabbing select-none ${
        positioned
          ? 'border-green-700/50 hover:border-green-500/70 opacity-80 hover:opacity-100'
          : 'border-neutral-700 hover:border-accent-DEFAULT'
      }`}
      title={positioned ? `${photo.original_filename} — auf Karte ziehen zum Verschieben` : `${photo.original_filename} — auf Karte ziehen`}
    >
      <AuthenticatedImage
        src={photo.thumb_url}
        alt={photo.original_filename}
        className="w-full h-28 object-contain bg-neutral-950 pointer-events-none"
      />

      {/* Video play icon */}
      {isVid && (
        <div className="absolute top-1 right-1 w-6 h-6 rounded-full bg-black/60 flex items-center justify-center pointer-events-none">
          <svg className="w-3 h-3 text-white ml-0.5" viewBox="0 0 20 20" fill="currentColor">
            <path d="M6.3 2.841A1.5 1.5 0 004 4.11v11.78a1.5 1.5 0 002.3 1.269l9.344-5.89a1.5 1.5 0 000-2.538L6.3 2.84z" />
          </svg>
        </div>
      )}

      {/* Positioned badge */}
      {positioned && (
        <div className="absolute top-1 left-1 w-5 h-5 rounded-full bg-green-600/80 flex items-center justify-center pointer-events-none">
          <svg className="w-3 h-3 text-white" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M5.05 4.05a7 7 0 119.9 9.9L10 18.9l-4.95-4.95a7 7 0 010-9.9zM10 11a2 2 0 100-4 2 2 0 000 4z" clipRule="evenodd" />
          </svg>
        </div>
      )}

      {/* Drag handle hint (unpositioned only) */}
      {!positioned && (
        <div className="absolute inset-0 flex items-center justify-center opacity-0 hover:opacity-100 transition-opacity bg-black/30">
          <svg className="w-6 h-6 text-white drop-shadow" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M10 3a1 1 0 00-1 1v1.586l-.293-.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l2-2a1 1 0 00-1.414-1.414L11 5.586V4a1 1 0 00-1-1zM5.293 11.293a1 1 0 011.414 0L10 14.586l3.293-3.293a1 1 0 011.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clipRule="evenodd" />
          </svg>
        </div>
      )}

      {/* Filename label */}
      <div className="absolute bottom-0 inset-x-0 bg-black/60 px-1.5 py-0.5">
        <p className="text-[10px] text-white truncate">{photo.original_filename}</p>
      </div>

      {/* Saving spinner */}
      {saving && (
        <div className="absolute inset-0 bg-black/40 flex items-center justify-center">
          <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
        </div>
      )}
    </div>
  )
}
