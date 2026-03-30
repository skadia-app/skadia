import { useState, useEffect, useRef, useCallback, useTransition, type ChangeEvent, type KeyboardEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { assignActivitiesToTag, unassignActivitiesFromTag } from '@/api/tags'
import type { Tag } from '@/api/tags'
import { updateActivity } from '@/api/activities'
import { useCreateLocationTag } from '@/hooks/useCreateLocationTag'
import { getTagBadgeStyle } from '@/lib/tagUtils'
import { useToast } from '@/lib/toast'
import ColorPicker from '@/components/ColorPicker'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

interface NominatimResult {
  place_id: number
  display_name: string
  lat: string
  lon: string
}

// ---------------------------------------------------------------------------
// Inline geocoder — emits a coordinate callback on selection
// ---------------------------------------------------------------------------

interface GeoSearchInlineProps {
  mapRef: React.RefObject<maplibregl.Map | null>
  onSelect?: (lat: number, lng: number, label: string) => void
}

function GeoSearchInline({ mapRef, onSelect }: GeoSearchInlineProps) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<NominatimResult[]>([])
  const [isOpen, setIsOpen] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [activeIndex, setActiveIndex] = useState(-1)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const searchNominatim = useCallback(async (q: string) => {
    if (q.length < 3) { setResults([]); setIsOpen(false); return }
    setIsLoading(true)
    try {
      const url = `https://nominatim.openstreetmap.org/search?format=json&q=${encodeURIComponent(q)}&limit=5`
      const resp = await fetch(url, { headers: { 'User-Agent': 'Skadia/1.0' } })
      if (!resp.ok) throw new Error(`Nominatim: ${resp.status}`)
      const data: NominatimResult[] = await resp.json()
      setResults(data)
      setIsOpen(data.length > 0)
      setActiveIndex(-1)
    } catch (err) {
      console.error('Geocoding error:', err)
      setResults([])
      setIsOpen(false)
    } finally {
      setIsLoading(false)
    }
  }, [])

  const handleChange = (e: ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setQuery(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (value.length < 3) { setResults([]); setIsOpen(false); return }
    debounceRef.current = setTimeout(() => searchNominatim(value), 500)
  }

  const handleSelect = (result: NominatimResult) => {
    const lat = parseFloat(result.lat)
    const lng = parseFloat(result.lon)
    mapRef.current?.flyTo({ center: [lng, lat], zoom: 15, duration: 1000 })
    setQuery(result.display_name)
    setIsOpen(false)
    onSelect?.(lat, lng, result.display_name)
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') { setIsOpen(false); return }
    if (!isOpen || results.length === 0) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex(prev => (prev < results.length - 1 ? prev + 1 : 0))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex(prev => (prev > 0 ? prev - 1 : results.length - 1))
    } else if (e.key === 'Enter' && activeIndex >= 0) {
      e.preventDefault()
      handleSelect(results[activeIndex])
    }
  }

  return (
    <div ref={containerRef} className="relative">
      <div className="relative">
        <input
          type="text"
          value={query}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onFocus={() => { if (results.length > 0) setIsOpen(true) }}
          placeholder="Ort suchen..."
          className="w-full px-3 py-1.5 pr-8 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 dark:placeholder-neutral-500 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
        {isLoading && (
          <div className="absolute right-2 top-1/2 -translate-y-1/2">
            <div className="w-4 h-4 border-2 border-neutral-300 border-t-orange-500 rounded-full animate-spin" />
          </div>
        )}
        {query && !isLoading && (
          <button
            onClick={() => { setQuery(''); setResults([]); setIsOpen(false) }}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
            aria-label="Suche leeren"
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        )}
      </div>
      {isOpen && results.length > 0 && (
        <ul className="absolute z-20 mt-1 w-full max-h-48 overflow-y-auto rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 shadow-lg">
          {results.map((r, i) => (
            <li key={r.place_id}>
              <button
                onClick={() => handleSelect(r)}
                className={`w-full text-left px-3 py-2 text-xs cursor-pointer transition-colors ${
                  i === activeIndex
                    ? 'bg-accent-DEFAULT text-white'
                    : 'text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700'
                }`}
              >
                <span className="line-clamp-2">{r.display_name}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Mini-Map Picker — collapsible map for picking a location
// ---------------------------------------------------------------------------

interface LocationPickerMapProps {
  onConfirm: (lat: number, lng: number) => void
  onCancel: () => void
}

function LocationPickerMap({ onConfirm, onCancel }: LocationPickerMapProps) {
  const mapContainer = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)
  const [pendingCoords, setPendingCoords] = useState<{ lat: number; lng: number } | null>(null)

  useEffect(() => {
    if (!mapContainer.current) return

    const map = new maplibregl.Map({
      container: mapContainer.current,
      style: MAP_STYLE,
      center: [10, 47],
      zoom: 4,
      interactive: true,
    })

    mapRef.current = map

    map.on('click', (e) => {
      const { lat, lng } = e.lngLat
      setPendingCoords({ lat, lng })

      if (markerRef.current) {
        markerRef.current.setLngLat([lng, lat])
      } else {
        markerRef.current = new maplibregl.Marker({ color: '#EF4444' })
          .setLngLat([lng, lat])
          .addTo(map)
      }
    })

    return () => {
      markerRef.current?.remove()
      markerRef.current = null
      map.remove()
      mapRef.current = null
    }
  }, [])

  const handleGeoSelect = useCallback((lat: number, lng: number) => {
    setPendingCoords({ lat, lng })
    if (markerRef.current) {
      markerRef.current.setLngLat([lng, lat])
    } else if (mapRef.current) {
      markerRef.current = new maplibregl.Marker({ color: '#EF4444' })
        .setLngLat([lng, lat])
        .addTo(mapRef.current)
    }
  }, [])

  return (
    <div className="mt-3 space-y-2">
      <p className="text-xs text-neutral-500 dark:text-neutral-400">
        Ort suchen oder auf die Karte klicken, um einen Standort zu setzen.
      </p>
      <GeoSearchInline mapRef={mapRef} onSelect={handleGeoSelect} />
      <div
        ref={mapContainer}
        style={{ height: '200px', width: '100%' }}
        className="rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 cursor-crosshair"
      />
      {pendingCoords && (
        <p className="text-xs text-neutral-400 dark:text-neutral-500">
          Gewählt: {pendingCoords.lat.toFixed(5)}, {pendingCoords.lng.toFixed(5)}
        </p>
      )}
      <div className="flex gap-2">
        <button
          onClick={() => pendingCoords && onConfirm(pendingCoords.lat, pendingCoords.lng)}
          disabled={!pendingCoords}
          className="px-3 py-1.5 text-xs rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
        >
          Standort speichern
        </button>
        <button
          onClick={onCancel}
          className="px-3 py-1.5 text-xs rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
        >
          Abbrechen
        </button>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface Props {
  activityId: string
  currentTags: string[]
  locationTags: Tag[]
  locationLat?: number | null
  locationLng?: number | null
  onUpdate: () => void
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export default function ActivityLocationTagSection({
  activityId,
  currentTags,
  locationTags,
  locationLat,
  locationLng,
  onUpdate,
}: Props) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [showPicker, setShowPicker] = useState(false)
  const [showMapPicker, setShowMapPicker] = useState(false)
  const [saveAsTag, setSaveAsTag] = useState(false)
  const [newTagName, setNewTagName] = useState('')
  const [newTagColor, setNewTagColor] = useState('#EF4444')
  const [, startTransition] = useTransition()

  // Tags that are both a location tag AND already assigned to this activity
  const assignedLocationTags = locationTags.filter(t => currentTags.includes(t.name))
  // Tags available to assign
  const availableTags = locationTags.filter(t => !currentTags.includes(t.name))

  const hasStandortTag = assignedLocationTags.length > 0
  const hasLocation = locationLat != null && locationLng != null

  // --------------------------------------------------------------------------
  // Mutations
  // --------------------------------------------------------------------------

  const assignMutation = useMutation({
    mutationFn: (tagId: string) => assignActivitiesToTag(tagId, [activityId]),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activity', activityId] })
      onUpdate()
      setShowPicker(false)
    },
  })

  const removeMutation = useMutation({
    mutationFn: (tagId: string) => unassignActivitiesFromTag(tagId, [activityId]),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activity', activityId] })
      onUpdate()
    },
  })

  const patchLocationMutation = useMutation({
    mutationFn: ({ lat, lng }: { lat: number; lng: number }) =>
      updateActivity(activityId, { location_lat: lat, location_lng: lng }),
    onSuccess: (updatedActivity) => {
      queryClient.invalidateQueries({ queryKey: ['activity', activityId] })
      onUpdate()
      setShowMapPicker(false)

      // If user also wants to create a tag, do that after location is saved
      if (saveAsTag && newTagName.trim()) {
        startTransition(() => {
          createLocationTagMut.mutate({
            name: newTagName.trim(),
            lat: updatedActivity.location_lat!,
            lng: updatedActivity.location_lng!,
            color: newTagColor,
            activityIds: [activityId],
          })
        })
      }
    },
    onError: () => {
      addToast({ type: 'error', title: 'Standort konnte nicht gespeichert werden', duration: 0 })
    },
  })

  const createLocationTagMut = useCreateLocationTag()

  const handleMapConfirm = (lat: number, lng: number) => {
    patchLocationMutation.mutate({ lat, lng })
  }

  const handleSaveAsExistingTag = (tagId: string) => {
    // Find the tag to sync location FROM it
    const tag = locationTags.find(t => t.id === tagId)
    if (!tag) return
    assignMutation.mutate(tagId)
    // Also patch activity location if the tag has coords and activity doesn't yet
    if (!hasLocation && tag.lat != null && tag.lng != null) {
      updateActivity(activityId, { location_lat: tag.lat, location_lng: tag.lng })
        .then(() => {
          queryClient.invalidateQueries({ queryKey: ['activity', activityId] })
        })
        .catch(() => {})
    }
  }

  const handleCreateNewTag = () => {
    if (!newTagName.trim() || !hasLocation) return
    createLocationTagMut.mutate(
      { name: newTagName.trim(), lat: locationLat!, lng: locationLng!, color: newTagColor, activityIds: [activityId] },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ['activity', activityId] })
          onUpdate()
          setSaveAsTag(false)
          setNewTagName('')
          addToast({ type: 'success', title: 'Standort-Tag erstellt und zugewiesen' })
        },
        onError: () => {
          addToast({ type: 'error', title: 'Standort-Tag konnte nicht erstellt werden', duration: 0 })
        },
      }
    )
  }

  // --------------------------------------------------------------------------
  // Render helpers
  // --------------------------------------------------------------------------

  const locationIcon = (
    <svg className="w-4 h-4 text-neutral-400" viewBox="0 0 20 20" fill="currentColor">
      <path fillRule="evenodd" d="M9.69 18.933l.003.001C9.89 19.02 10 19 10 19s.11.02.308-.066l.002-.001.006-.003.018-.008a5.741 5.741 0 00.281-.14c.186-.096.446-.24.757-.433.62-.384 1.445-.966 2.274-1.765C15.302 15.957 17 13.514 17 10c0-3.866-3.582-7-8-7S2 6.134 2 10c0 3.514 1.698 5.957 3.355 7.584a13.731 13.731 0 002.273 1.765 11.842 11.842 0 00.757.433 5.766 5.766 0 00.282.14l.018.008.006.003zM10 11.25a1.25 1.25 0 100-2.5 1.25 1.25 0 000 2.5z" clipRule="evenodd" />
    </svg>
  )

  // --------------------------------------------------------------------------
  // Case 1: No existing location tags at all → limited options, but still allow pin
  // --------------------------------------------------------------------------
  if (locationTags.length === 0) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4">
        <div className="flex items-center gap-2 mb-2">
          {locationIcon}
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Standort</h3>
        </div>

        {!hasLocation && !showMapPicker && (
          <div className="space-y-2">
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              Kein Standort gesetzt.{' '}
              <button
                onClick={() => setShowMapPicker(true)}
                className="text-accent-DEFAULT hover:underline cursor-pointer"
              >
                Standort auf Karte setzen
              </button>
            </p>
          </div>
        )}

        {showMapPicker && (
          <LocationPickerMap
            onConfirm={handleMapConfirm}
            onCancel={() => setShowMapPicker(false)}
          />
        )}

        {hasLocation && !showMapPicker && (
          <div className="space-y-2">
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              Standort: {locationLat!.toFixed(5)}, {locationLng!.toFixed(5)}
            </p>
            {!saveAsTag && (
              <button
                onClick={() => setSaveAsTag(true)}
                className="text-xs text-accent-DEFAULT hover:underline cursor-pointer"
              >
                Als Standort-Tag speichern
              </button>
            )}
            {saveAsTag && (
              <div className="space-y-2 pt-1">
                <div className="flex items-center gap-2">
                  <ColorPicker value={newTagColor} onChange={setNewTagColor} />
                  <input
                    type="text"
                    value={newTagName}
                    onChange={e => setNewTagName(e.target.value)}
                    placeholder="Name des Standort-Tags"
                    className="flex-1 px-2 py-1 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
                  />
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={handleCreateNewTag}
                    disabled={!newTagName.trim() || createLocationTagMut.isPending}
                    className="px-3 py-1.5 text-xs rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
                  >
                    {createLocationTagMut.isPending ? 'Speichern…' : 'Tag erstellen'}
                  </button>
                  <button
                    onClick={() => setSaveAsTag(false)}
                    className="px-3 py-1.5 text-xs rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
                  >
                    Abbrechen
                  </button>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    )
  }

  // --------------------------------------------------------------------------
  // Main render (location tags exist)
  // --------------------------------------------------------------------------

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          {locationIcon}
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Standort</h3>
        </div>
        {availableTags.length > 0 && !showPicker && !showMapPicker && (
          <button
            onClick={() => setShowPicker(true)}
            className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
          >
            + Standort zuweisen
          </button>
        )}
      </div>

      {/* Assigned location tags */}
      {assignedLocationTags.length > 0 && (
        <div className="flex flex-wrap gap-2 mb-3">
          {assignedLocationTags.map(tag => (
            <div
              key={tag.id}
              className="flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium"
              style={getTagBadgeStyle(tag.color)}
            >
              <svg className="w-3 h-3 opacity-70" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M9.69 18.933l.003.001C9.89 19.02 10 19 10 19s.11.02.308-.066l.002-.001.006-.003.018-.008a5.741 5.741 0 00.281-.14c.186-.096.446-.24.757-.433.62-.384 1.445-.966 2.274-1.765C15.302 15.957 17 13.514 17 10c0-3.866-3.582-7-8-7S2 6.134 2 10c0 3.514 1.698 5.957 3.355 7.584a13.731 13.731 0 002.273 1.765 11.842 11.842 0 00.757.433 5.766 5.766 0 00.282.14l.018.008.006.003zM10 11.25a1.25 1.25 0 100-2.5 1.25 1.25 0 000 2.5z" clipRule="evenodd" />
              </svg>
              {tag.name}
              {tag.lat != null && (
                <span className="opacity-60 text-xs">
                  {tag.lat.toFixed(3)}, {tag.lng?.toFixed(3)}
                </span>
              )}
              <button
                onClick={() => removeMutation.mutate(tag.id)}
                disabled={removeMutation.isPending}
                className="opacity-60 hover:opacity-100 cursor-pointer disabled:cursor-not-allowed disabled:opacity-30 ml-0.5"
                title="Standort entfernen"
              >
                <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                </svg>
              </button>
            </div>
          ))}
        </div>
      )}

      {/* State: no assigned tag, no location yet — show map picker option */}
      {!hasStandortTag && !hasLocation && !showPicker && !showMapPicker && (
        <p className="text-xs text-neutral-500 dark:text-neutral-400">
          Kein Standort zugeordnet.{' '}
          <button
            onClick={() => setShowMapPicker(true)}
            className="text-accent-DEFAULT hover:underline cursor-pointer"
          >
            Auf Karte setzen
          </button>
          {availableTags.length > 0 && (
            <>
              {' '}oder{' '}
              <button
                onClick={() => setShowPicker(true)}
                className="text-accent-DEFAULT hover:underline cursor-pointer"
              >
                aus bestehenden Tags wählen
              </button>
            </>
          )}
        </p>
      )}

      {/* State: location set but no standort tag — prompt to save as tag */}
      {!hasStandortTag && hasLocation && !showPicker && !showMapPicker && !saveAsTag && (
        <div className="space-y-1">
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            Standort: {locationLat!.toFixed(5)}, {locationLng!.toFixed(5)}
          </p>
          <div className="flex flex-wrap gap-x-3 gap-y-1">
            <button
              onClick={() => setSaveAsTag(true)}
              className="text-xs text-accent-DEFAULT hover:underline cursor-pointer"
            >
              Als Standort-Tag speichern
            </button>
            {availableTags.length > 0 && (
              <button
                onClick={() => setShowPicker(true)}
                className="text-xs text-accent-DEFAULT hover:underline cursor-pointer"
              >
                Vorhandenen Tag zuweisen
              </button>
            )}
          </div>
        </div>
      )}

      {/* State: location set, user wants to create a new tag */}
      {!hasStandortTag && hasLocation && saveAsTag && !showPicker && !showMapPicker && (
        <div className="space-y-2 mt-1">
          <div className="flex items-center gap-2">
            <ColorPicker value={newTagColor} onChange={setNewTagColor} />
            <input
              type="text"
              value={newTagName}
              onChange={e => setNewTagName(e.target.value)}
              placeholder="Name des Standort-Tags"
              className="flex-1 px-2 py-1 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
            />
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleCreateNewTag}
              disabled={!newTagName.trim() || createLocationTagMut.isPending}
              className="px-3 py-1.5 text-xs rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
            >
              {createLocationTagMut.isPending ? 'Speichern…' : 'Tag erstellen'}
            </button>
            <button
              onClick={() => setSaveAsTag(false)}
              className="px-3 py-1.5 text-xs rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
            >
              Abbrechen
            </button>
          </div>
        </div>
      )}

      {/* Map picker */}
      {showMapPicker && (
        <LocationPickerMap
          onConfirm={handleMapConfirm}
          onCancel={() => setShowMapPicker(false)}
        />
      )}

      {/* Location picker dropdown (assign existing tag) */}
      {showPicker && (
        <div className="mt-2 space-y-2">
          <div className="text-xs text-neutral-500 dark:text-neutral-400">Standort-Tag zuweisen:</div>
          <div className="flex flex-wrap gap-1.5">
            {availableTags.map(tag => (
              <button
                key={tag.id}
                onClick={() => handleSaveAsExistingTag(tag.id)}
                disabled={assignMutation.isPending}
                className="px-2.5 py-1 rounded-full text-xs font-medium cursor-pointer hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed transition-opacity"
                style={getTagBadgeStyle(tag.color)}
              >
                {tag.name}
              </button>
            ))}
          </div>
          <div className="flex gap-3">
            <button
              onClick={() => setShowPicker(false)}
              className="text-xs text-neutral-400 hover:text-neutral-600 cursor-pointer"
            >
              Abbrechen
            </button>
            <button
              onClick={() => { setShowPicker(false); setShowMapPicker(true) }}
              className="text-xs text-accent-DEFAULT hover:underline cursor-pointer"
            >
              Neuen Standort setzen
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
