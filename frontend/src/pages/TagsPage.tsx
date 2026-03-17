import { Fragment, useRef, useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router-dom'
import { fetchTags, createTag, updateTag, deleteTag, mergeTags, fetchTagActivities, unassignActivitiesFromTag, upsertTagZone, deleteTagZone, deleteOrphanTag, seedDefaultTags } from '@/api/tags'
import type { Tag, TagActivity } from '@/api/tags'
import ZoneEditor from '@/components/ZoneEditor'
import type { ZoneData } from '@/components/ZoneEditor'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import ColorPicker from '@/components/ColorPicker'

// ---------------------------------------------------------------------------
// Colour palette
// ---------------------------------------------------------------------------

const colorPalette = [
  { color: '#f97316', text: '#ffffff', label: 'Orange' },
  { color: '#3b82f6', text: '#ffffff', label: 'Blau' },
  { color: '#10b981', text: '#ffffff', label: 'Grün' },
  { color: '#8b5cf6', text: '#ffffff', label: 'Lila' },
  { color: '#f43f5e', text: '#ffffff', label: 'Rosa' },
  { color: '#06b6d4', text: '#ffffff', label: 'Cyan' },
  { color: '#eab308', text: '#1a1a1a', label: 'Gelb' },
  { color: '#6b7280', text: '#ffffff', label: 'Grau' },
]

// ---------------------------------------------------------------------------
// Location pin mini-map
// ---------------------------------------------------------------------------

function LocationPinMiniMap({
  lat,
  lng,
  onCoordinatesChange,
}: {
  lat: number | null
  lng: number | null
  onCoordinatesChange: (lat: number, lng: number) => void
}) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)

  useEffect(() => {
    const container = mapContainerRef.current
    if (!container) return

    const map = new maplibregl.Map({
      container,
      style: {
        version: 8,
        sources: { basemap: { type: 'raster', tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'], tileSize: 256, attribution: '&copy; OpenStreetMap' } },
        layers: [{ id: 'tiles', type: 'raster', source: 'basemap', minzoom: 0, maxzoom: 19 }],
      },
      center: [lng ?? 10.0, lat ?? 51.0],
      zoom: lat != null ? 11 : 5,
      attributionControl: false,
    })
    mapRef.current = map

    map.on('load', () => {
      const marker = new maplibregl.Marker({ color: '#f97316', draggable: true })
        .setLngLat([lng ?? 10.0, lat ?? 51.0])
        .addTo(map)
      markerRef.current = marker

      marker.on('dragend', () => {
        const { lat: newLat, lng: newLng } = marker.getLngLat()
        onCoordinatesChange(newLat, newLng)
      })

      map.on('click', (e) => {
        const { lat: newLat, lng: newLng } = e.lngLat
        marker.setLngLat([newLng, newLat])
        onCoordinatesChange(newLat, newLng)
      })
    })

    return () => { map.remove() }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Sync marker when lat/lng changes externally (e.g., user types in inputs)
  useEffect(() => {
    const map = mapRef.current
    const marker = markerRef.current
    if (!map || !marker || lat == null || lng == null) return
    marker.setLngLat([lng, lat])
    if (!map.getBounds().contains([lng, lat])) {
      map.flyTo({ center: [lng, lat], zoom: 11 })
    }
  }, [lat, lng])

  return (
    <div
      ref={mapContainerRef}
      className="w-full rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700"
      style={{ height: 400 }}
    />
  )
}

// ---------------------------------------------------------------------------
// Main TagForm component
// ---------------------------------------------------------------------------

// Location type for tag: pin=simple pin, circle=zone with radius, polygon=zone with polygon
type LocationType = 'pin' | 'circle' | 'polygon'

function deriveInitialLocationType(tag?: Tag): LocationType {
  if (!tag?.lat) return 'pin'
  if (tag.zone?.zone_type === 'circle') return 'circle'
  if (tag.zone?.zone_type === 'polygon') return 'polygon'
  return 'pin'
}

function TagForm({
  tag,
  onSave,
  onCancel,
  inline,
}: {
  tag?: Tag
  onSave: () => void
  onCancel: () => void
  inline?: boolean
}) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(tag?.name ?? '')
  const [color, setColor] = useState(tag?.color ?? colorPalette[0].color)
  const [textColor, setTextColor] = useState(tag?.text_color ?? colorPalette[0].text)
  const [description, setDescription] = useState(tag?.description ?? '')

  // Location state
  const [isLocationTag, setIsLocationTag] = useState(() => tag?.lat != null)
  const [locationLat, setLocationLat] = useState<string>(() => tag?.lat?.toString() ?? '')
  const [locationLng, setLocationLng] = useState<string>(() => tag?.lng?.toString() ?? '')
  // Location type: pin (simple pin), circle (zone with radius), polygon (zone with polygon)
  const [locationType, setLocationType] = useState<LocationType>(() => deriveInitialLocationType(tag))
  // Current zone value as entered in ZoneEditor
  const [zoneData, setZoneData] = useState<ZoneData>(() => {
    if (tag?.zone?.zone_type === 'circle' && tag.zone.center && tag.zone.radius_m != null) {
      return { mode: 'circle', center: tag.zone.center, radiusM: tag.zone.radius_m }
    }
    if (tag?.zone?.zone_type === 'polygon' && tag.zone.polygon) {
      const poly = typeof tag.zone.polygon === 'string' ? JSON.parse(tag.zone.polygon) : tag.zone.polygon
      return { mode: 'polygon', polygon: poly }
    }
    const lat = tag?.lat ? parseFloat(String(tag.lat)) : 51.16
    const lng = tag?.lng ? parseFloat(String(tag.lng)) : 10.45
    return { mode: locationType === 'polygon' ? 'polygon' : 'circle', center: [lng, lat], radiusM: 500 }
  })

  const createMut = useMutation({
    mutationFn: async () => {
      const lat = isLocationTag && locationLat ? parseFloat(locationLat) : undefined
      const lng = isLocationTag && locationLng ? parseFloat(locationLng) : undefined
      const newTag = await createTag({
        name: name.trim(),
        color,
        text_color: textColor,
        description: description.trim() || undefined,
        lat: isLocationTag ? (lat ?? null) : undefined,
        lng: isLocationTag ? (lng ?? null) : undefined,
      })
      // Save zone if applicable
      if (isLocationTag && locationType !== 'pin' && zoneData) {
        await upsertTagZone(newTag.id, {
          zone_type: zoneData.mode,
          center: zoneData.mode === 'circle' ? zoneData.center : undefined,
          radius_m: zoneData.mode === 'circle' ? zoneData.radiusM : undefined,
          polygon: zoneData.mode === 'polygon' && zoneData.polygon ? JSON.stringify(zoneData.polygon) : undefined,
        })
      }
      return newTag
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['tags'] }); onSave() },
  })

  const updateMut = useMutation({
    mutationFn: async () => {
      const lat = isLocationTag && locationLat ? parseFloat(locationLat) : null
      const lng = isLocationTag && locationLng ? parseFloat(locationLng) : null
      const updatedTag = await updateTag(tag!.id, {
        name: name.trim(),
        color,
        text_color: textColor,
        description: description.trim() || undefined,
        lat: isLocationTag ? lat : null,
        lng: isLocationTag ? lng : null,
      })
      // Upsert or delete zone based on type selection
      if (isLocationTag && locationType !== 'pin' && zoneData) {
        await upsertTagZone(tag!.id, {
          zone_type: zoneData.mode,
          center: zoneData.mode === 'circle' ? zoneData.center : undefined,
          radius_m: zoneData.mode === 'circle' ? zoneData.radiusM : undefined,
          polygon: zoneData.mode === 'polygon' && zoneData.polygon ? JSON.stringify(zoneData.polygon) : undefined,
        })
      } else if (tag?.zone && (locationType === 'pin' || !isLocationTag)) {
        // User switched to pin or removed location — delete any existing zone
        try { await deleteTagZone(tag!.id) } catch { /* ignore if no zone */ }
      }
      return updatedTag
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['tags'] }); onSave() },
  })

  const isPending = createMut.isPending || updateMut.isPending
  const errorMsg = (createMut.error ?? updateMut.error) instanceof Error
    ? (createMut.error ?? updateMut.error)?.message
    : null

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    tag ? updateMut.mutate() : createMut.mutate()
  }

  function selectColor(c: typeof colorPalette[number]) {
    setColor(c.color)
    setTextColor(c.text)
  }

  function selectCustomColor(hex: string) {
    setColor(hex)
    const r = parseInt(hex.slice(1, 3), 16)
    const g = parseInt(hex.slice(3, 5), 16)
    const b = parseInt(hex.slice(5, 7), 16)
    setTextColor((0.299 * r + 0.587 * g + 0.114 * b) / 255 > 0.5 ? '#1a1a1a' : '#ffffff')
  }

  // Inline (table row edit) — keep simple: name + colour only
  if (inline) {
    return (
      <form onSubmit={handleSubmit} className="flex items-center gap-3 flex-wrap">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Tag-Name"
          className="px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400 w-40"
          required
        />
        <div className="flex items-center gap-1.5 flex-wrap">
          {colorPalette.map((c) => (
            <button key={c.color} type="button" onClick={() => selectColor(c)}
              className="w-6 h-6 rounded-full border-2 transition-all cursor-pointer"
              style={{ backgroundColor: c.color, borderColor: color === c.color ? '#f97316' : 'transparent', transform: color === c.color ? 'scale(1.2)' : 'scale(1)' }}
              title={c.label}
            />
          ))}
          <ColorPicker value={color} onChange={selectCustomColor} />
        </div>
        <span className="px-2.5 py-1 rounded-full text-xs font-medium" style={{ backgroundColor: color, color: textColor }}>
          {name || 'Vorschau'}
        </span>
        <div className="flex items-center gap-2">
          <button type="submit" disabled={isPending || !name.trim()}
            className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
          >{tag ? 'Speichern' : 'Erstellen'}</button>
          <button type="button" onClick={onCancel}
            className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
          >Abbrechen</button>
        </div>
        {errorMsg && <p className="text-sm text-rose-500">Fehler: {errorMsg}</p>}
      </form>
    )
  }

  // Full form
  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {/* Name */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Name</label>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Tag-Name"
          className="w-full px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
          required
        />
      </div>

      {/* Description */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Beschreibung</label>
        <input
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optionale Beschreibung"
          className="w-full px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
        />
      </div>

      {/* Color */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5">Farbe</label>
        <div className="flex items-center gap-2 flex-wrap">
          {colorPalette.map((c) => (
            <button key={c.color} type="button" onClick={() => selectColor(c)}
              className="w-6 h-6 rounded-full border-2 transition-all cursor-pointer"
              style={{ backgroundColor: c.color, borderColor: color === c.color ? '#f97316' : 'transparent', transform: color === c.color ? 'scale(1.2)' : 'scale(1)' }}
              title={c.label}
            />
          ))}
          <ColorPicker value={color} onChange={selectCustomColor} />
          <span className="ml-1 px-2.5 py-1 rounded-full text-xs font-medium" style={{ backgroundColor: color, color: textColor }}>
            {name || 'Vorschau'}
          </span>
        </div>
      </div>

      {/* Standort-Abschnitt */}
      <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl overflow-hidden">
        <button
          type="button"
          onClick={() => setIsLocationTag(!isLocationTag)}
          className="w-full flex items-center justify-between px-4 py-3 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors cursor-pointer"
        >
          <span className="flex items-center gap-2">
            <svg className="w-4 h-4 text-neutral-400" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M9.69 18.933l.003.001C9.89 19.02 10 19 10 19s.11.02.308-.066l.002-.001.006-.003.018-.008a5.741 5.741 0 00.281-.14c.186-.096.446-.24.757-.433.62-.384 1.445-.966 2.274-1.765C15.302 15.957 17 13.514 17 10c0-3.866-3.582-7-8-7S2 6.134 2 10c0 3.514 1.698 5.957 3.355 7.584a13.731 13.731 0 002.273 1.765 11.842 11.842 0 00.757.433 5.766 5.766 0 00.282.14l.018.008.006.003zM10 11.25a1.25 1.25 0 100-2.5 1.25 1.25 0 000 2.5z" clipRule="evenodd" />
            </svg>
            <span>Standort</span>
            {isLocationTag && locationLat && locationLng && (
              <span className="text-xs font-normal text-accent-DEFAULT">
                {parseFloat(locationLat).toFixed(4)}, {parseFloat(locationLng).toFixed(4)}
              </span>
            )}
          </span>
          <div className="flex items-center gap-2">
            <span className={`w-9 h-5 rounded-full transition-colors flex items-center ${isLocationTag ? 'bg-accent-DEFAULT' : 'bg-neutral-200 dark:bg-neutral-700'}`}>
              <span className={`w-4 h-4 rounded-full bg-white shadow mx-0.5 transition-transform ${isLocationTag ? 'translate-x-4' : ''}`} />
            </span>
          </div>
        </button>

        {isLocationTag && (
          <div className="px-4 py-3 border-t border-neutral-200 dark:border-neutral-800 space-y-4">
            {/* Koordinaten-Eingabe + Karte */}
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              Klicke auf die Karte oder ziehe den Pin, um den Standort zu setzen.
            </p>
            <LocationPinMiniMap
              lat={locationLat ? parseFloat(locationLat) : null}
              lng={locationLng ? parseFloat(locationLng) : null}
              onCoordinatesChange={(lat, lng) => {
                setLocationLat(lat.toFixed(6))
                setLocationLng(lng.toFixed(6))
              }}
            />
            <div className="flex items-center gap-3">
              <div className="flex-1">
                <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">Breitengrad</label>
                <input
                  type="number"
                  step="any"
                  min="-90"
                  max="90"
                  value={locationLat}
                  onChange={(e) => setLocationLat(e.target.value)}
                  placeholder="z.B. 48.1234"
                  className="w-full px-2 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
                />
              </div>
              <div className="flex-1">
                <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">Längengrad</label>
                <input
                  type="number"
                  step="any"
                  min="-180"
                  max="180"
                  value={locationLng}
                  onChange={(e) => setLocationLng(e.target.value)}
                  placeholder="z.B. 11.5678"
                  className="w-full px-2 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
                />
              </div>
            </div>

            {/* Standort-Typ: Pin / Radius / Polygon */}
            <div>
              <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-2">
                Standort-Typ
              </label>
              <div className="flex items-center gap-2">
                {/* Pin */}
                <button
                  type="button"
                  onClick={() => setLocationType('pin')}
                  className={`flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium border transition-colors cursor-pointer ${
                    locationType === 'pin'
                      ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                      : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:border-accent-400'
                  }`}
                  title="Einfacher Pin ohne Zone"
                >
                  <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M9.69 18.933l.003.001C9.89 19.02 10 19 10 19s.11.02.308-.066l.002-.001.006-.003.018-.008a5.741 5.741 0 00.281-.14c.186-.096.446-.24.757-.433.62-.384 1.445-.966 2.274-1.765C15.302 15.957 17 13.514 17 10c0-3.866-3.582-7-8-7S2 6.134 2 10c0 3.514 1.698 5.957 3.355 7.584a13.731 13.731 0 002.273 1.765 11.842 11.842 0 00.757.433 5.766 5.766 0 00.282.14l.018.008.006.003zM10 11.25a1.25 1.25 0 100-2.5 1.25 1.25 0 000 2.5z" clipRule="evenodd" />
                  </svg>
                  Pin
                </button>

                {/* Radius */}
                <button
                  type="button"
                  onClick={() => setLocationType('circle')}
                  className={`flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium border transition-colors cursor-pointer ${
                    locationType === 'circle'
                      ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                      : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:border-accent-400'
                  }`}
                  title="Kreisförmige Zone mit Radius"
                >
                  <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm0-2a6 6 0 100-12 6 6 0 000 12z" clipRule="evenodd" />
                  </svg>
                  Radius
                </button>

                {/* Polygon */}
                <button
                  type="button"
                  onClick={() => setLocationType('polygon')}
                  className={`flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium border transition-colors cursor-pointer ${
                    locationType === 'polygon'
                      ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                      : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:border-accent-400'
                  }`}
                  title="Freihand-Polygon als Zone"
                >
                  <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M3 3a1 1 0 000 2h1.22l.305 1.222a.997.997 0 00.01.042l1.358 5.43-.893.892C3.74 12.846 4.632 14 6 14h11a1 1 0 100-2H6.414l1-1H14a1 1 0 00.894-.553l3-6A1 1 0 0017 3H3z" clipRule="evenodd" />
                  </svg>
                  Polygon
                </button>
              </div>
            </div>

            {/* ZoneEditor erscheint nur wenn Radius oder Polygon gewählt */}
            {(locationType === 'circle' || locationType === 'polygon') && (
              <div className="border-t border-neutral-200 dark:border-neutral-800 pt-3">
                <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-2">
                  {locationType === 'circle'
                    ? 'Ziehe den Pin und stelle den Radius ein.'
                    : 'Klicke "Polygon zeichnen" und setze Punkte auf der Karte.'}
                </p>
                <ZoneEditor
                  value={{
                    ...zoneData,
                    mode: locationType === 'polygon' ? 'polygon' : 'circle',
                  }}
                  onChange={setZoneData}
                  enableHoles
                />
              </div>
            )}
          </div>
        )}
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2 pt-1">
        <button type="submit" disabled={isPending || !name.trim()}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
        >{tag ? 'Speichern' : 'Erstellen'}</button>
        <button type="button" onClick={onCancel}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
        >Abbrechen</button>
      </div>

      {errorMsg && <p className="text-sm text-rose-500">Fehler: {errorMsg}</p>}
    </form>
  )
}

// ---------------------------------------------------------------------------
// TagActivitiesPanel
// ---------------------------------------------------------------------------

function TagActivitiesPanel({ tag, onClose }: { tag: Tag; onClose: () => void }) {
  const queryClient = useQueryClient()

  const { data: activities, isLoading } = useQuery({
    queryKey: ['tag-activities', tag.id],
    queryFn: () => fetchTagActivities(tag.id),
  })

  const unassignMut = useMutation({
    mutationFn: (activityId: string) => unassignActivitiesFromTag(tag.id, [activityId]),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tag-activities', tag.id] })
      queryClient.invalidateQueries({ queryKey: ['tags'] })
    },
  })

  return (
    <tr>
      <td colSpan={4} className="px-4 py-3 bg-neutral-50 dark:bg-neutral-900/40 border-b border-neutral-100 dark:border-neutral-800">
        <div className="flex items-center justify-between mb-2">
          <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
            Zugewiesene Aktivitäten
          </span>
          <button
            onClick={onClose}
            className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
          >
            Schliessen
          </button>
        </div>

        {isLoading ? (
          <p className="text-xs text-neutral-400">Lade...</p>
        ) : !activities || activities.length === 0 ? (
          <p className="text-xs text-neutral-400">Keine Aktivitäten zugewiesen.</p>
        ) : (
          <div className="space-y-1 max-h-64 overflow-y-auto">
            {activities.map((act: TagActivity) => (
              <div
                key={act.id}
                className="flex items-center justify-between gap-2 px-2 py-1.5 rounded-lg hover:bg-neutral-100 dark:hover:bg-neutral-800/50 group"
              >
                <div className="flex items-center gap-2 min-w-0">
                  <Link
                    to={`/activities/${act.id}`}
                    className="text-xs font-medium text-neutral-800 dark:text-neutral-200 hover:text-accent-DEFAULT truncate"
                  >
                    {act.title || act.title_auto || act.sport_type}
                  </Link>
                  {act.started_at && (
                    <span className="text-xs text-neutral-400 flex-shrink-0">
                      {format(new Date(act.started_at), 'dd.MM.yyyy', { locale: de })}
                    </span>
                  )}
                </div>
                <button
                  onClick={() => unassignMut.mutate(act.id)}
                  disabled={unassignMut.isPending}
                  className="opacity-0 group-hover:opacity-100 p-1 rounded text-neutral-400 hover:text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-900/20 transition-all cursor-pointer flex-shrink-0"
                  title="Vom Tag entfernen"
                >
                  <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                    <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                  </svg>
                </button>
              </div>
            ))}
          </div>
        )}
      </td>
    </tr>
  )
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export default function TagsPage() {
  const queryClient = useQueryClient()
  const location = useLocation()
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)
  // Track whether the current editingId came from a URL ?edit= param (needs auto-scroll)
  const [urlEditId, setUrlEditId] = useState<string | null>(null)
  const editFormRef = useRef<HTMLDivElement>(null)

  // Open edit form when navigated to with ?edit=<tagId>
  useEffect(() => {
    const params = new URLSearchParams(location.search)
    const editId = params.get('edit')
    if (editId) {
      setEditingId(editId)
      setUrlEditId(editId)
      setShowForm(false)
    }
  }, [location.search])

  const [expandedActivitiesId, setExpandedActivitiesId] = useState<string | null>(null)

  // Scroll to the edit form after it becomes visible (tags loaded + editingId set via URL).
  // No dependency array: runs after every render so it catches the moment editFormRef becomes
  // available (after the tags query resolves and the form div is mounted in the DOM).
  // The setUrlEditId(null) guard ensures it only scrolls once.
  useEffect(() => {
    if (urlEditId && editingId === urlEditId && editFormRef.current) {
      editFormRef.current.scrollIntoView({ behavior: 'smooth', block: 'start' })
      setUrlEditId(null)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  })

  // --- Merge state ---
  const [mergeSourceId, setMergeSourceId] = useState<string>('')
  const [mergeTargetId, setMergeTargetId] = useState<string>('')
  const [mergeError, setMergeError] = useState<string | null>(null)
  const [mergeSuccess, setMergeSuccess] = useState<string | null>(null)

  const { data: tags, isLoading } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })

  const deleteMut = useMutation({
    mutationFn: deleteTag,
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['tags'] }); setDeleteConfirmId(null) },
  })

  const mergeMut = useMutation({
    mutationFn: ({ targetId, sourceId }: { targetId: string; sourceId: string }) =>
      mergeTags(targetId, sourceId),
    onSuccess: (survivingTag) => {
      queryClient.invalidateQueries({ queryKey: ['tags'] })
      setMergeSuccess(`Tags fusioniert. "${survivingTag.name}" bleibt bestehen.`)
      setMergeSourceId('')
      setMergeTargetId('')
      setMergeError(null)
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : 'Unbekannter Fehler'
      setMergeError(`Fehler: ${msg}`)
      setMergeSuccess(null)
    },
  })

  const deleteOrphanMut = useMutation({
    mutationFn: (name: string) => deleteOrphanTag(name),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['tags'] }) },
  })

  // Auto-seed default tags on first load (creates Rundtour, weather tags etc.)
  const hasSeed = useRef(false)
  useEffect(() => {
    if (hasSeed.current) return
    hasSeed.current = true
    seedDefaultTags().then(() => {
      queryClient.invalidateQueries({ queryKey: ['tags'] })
    }).catch(() => { /* ignore */ })
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Tags...</div>
  }

  const allTags = tags ?? []

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link
            to="/workshop"
            className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
          >
            &#8592; Werkstatt
          </Link>
          <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Tags</h1>
        </div>
        <div className="flex items-center gap-3">
          <Link
            to="/workshop/rules?new=1"
            className="px-4 py-2 rounded-lg text-sm font-medium text-accent-600 dark:text-orange-400 border border-orange-300 dark:border-orange-700 hover:bg-orange-50 dark:hover:bg-orange-900/20 transition-colors"
          >
            Automatisierungsregel erstellen
          </Link>
          <button
            onClick={() => { setShowForm(true); setEditingId(null) }}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors cursor-pointer"
          >
            Tag erstellen
          </button>
        </div>
      </div>

      {/* Create form */}
      {showForm && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">Neuer Tag</h2>
          <TagForm
            onSave={() => { setShowForm(false); setEditingId(null) }}
            onCancel={() => { setShowForm(false); setEditingId(null) }}
          />
        </div>
      )}

      {/* Tags list */}
      {allTags.filter(t => t.category !== 'orphan').length > 0 ? (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-neutral-50 dark:bg-neutral-900/50 border-b border-neutral-200 dark:border-neutral-800">
                <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Tag</th>
                <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Kategorie</th>
                <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Aktivitäten</th>
                <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Aktionen</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
              {allTags.filter(t => t.category !== 'orphan').map((tag) => (
                <Fragment key={tag.id}>
                <tr className="group even:bg-neutral-50/50 dark:even:bg-neutral-800/20">
                  <td className="px-4 py-3">
                    {editingId === tag.id ? (
                      <TagForm
                        tag={tag}
                        onSave={() => setEditingId(null)}
                        onCancel={() => setEditingId(null)}
                        inline
                      />
                    ) : (
                      <div className="flex items-center gap-2">
                        <span
                          className="inline-block px-2.5 py-1 rounded-full text-xs font-medium"
                          style={{ backgroundColor: tag.color || '#f3f4f6', color: tag.text_color || '#374151' }}
                        >
                          {tag.name}
                        </span>
                        {(tag.has_automation || tag.is_auto) && (
                          <span
                            className="text-xs text-neutral-400 dark:text-neutral-500"
                            title="Automatisch vergeben (Bedingungen gesetzt)"
                          >
                            auto
                          </span>
                        )}
                        {tag.lat != null && (
                          <span
                            className="text-xs text-orange-400"
                            title={`Standort: ${tag.lat?.toFixed(4)}, ${tag.lng?.toFixed(4)}`}
                          >
                            📍
                          </span>
                        )}
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-3 text-neutral-500 dark:text-neutral-400">
                    {editingId === tag.id ? null : (
                      ({
                        system: 'System',
                        weather: 'Wetter',
                        standort: 'Standort',
                        custom: 'Eigene',
                        orphan: 'Verwaist',
                      } as Record<string, string>)[tag.category || 'custom'] || tag.category || '—'
                    )}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums">
                    {editingId === tag.id ? null : (
                      tag.activity_count > 0 ? (
                        <button
                          onClick={() => setExpandedActivitiesId(
                            expandedActivitiesId === tag.id ? null : tag.id
                          )}
                          className="text-sm text-accent-DEFAULT hover:underline cursor-pointer tabular-nums"
                          title="Aktivitäten anzeigen / verwalten"
                        >
                          {tag.activity_count}
                        </button>
                      ) : (
                        <span className="text-neutral-700 dark:text-neutral-300">{tag.activity_count}</span>
                      )
                    )}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {editingId !== tag.id && (
                      <div className="flex items-center justify-end gap-1.5 opacity-0 group-hover:opacity-100 transition-opacity">
                        {tag.is_system && !tag.is_auto ? (
                          <span className="text-xs text-neutral-400">System</span>
                        ) : (
                          <>
                            <button
                              onClick={() => {
                                setEditingId(null)
                                setShowForm(false)
                                setEditingId(tag.id)
                              }}
                              className="p-1.5 rounded-md text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                              title="Bearbeiten"
                            >
                              <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                                <path d="M2.695 14.763l-1.262 3.154a.5.5 0 00.65.65l3.155-1.262a4 4 0 001.343-.885L17.5 5.5a2.121 2.121 0 00-3-3L3.58 13.42a4 4 0 00-.885 1.343z" />
                              </svg>
                            </button>
                            <Link
                              to={`/workshop/rules?new=1&tag_name=${encodeURIComponent(tag.name)}`}
                              className="p-1.5 rounded-md text-neutral-400 hover:text-accent-DEFAULT hover:bg-orange-50 dark:hover:bg-orange-900/20 transition-colors"
                              title="Automatisierungsregel erstellen"
                            >
                              <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                                <path fillRule="evenodd" d="M3.25 3A2.25 2.25 0 001 5.25v9.5A2.25 2.25 0 003.25 17h9.5A2.25 2.25 0 0015 14.75v-9.5A2.25 2.25 0 0012.75 3h-9.5zM4 5.25a.75.75 0 01.75-.75h6.5a.75.75 0 01.75.75v.5a.75.75 0 01-.75.75h-6.5A.75.75 0 014 5.75v-.5zm0 3a.75.75 0 01.75-.75h6.5a.75.75 0 01.75.75v.5a.75.75 0 01-.75.75h-6.5A.75.75 0 014 8.75v-.5zm.75 2.25a.75.75 0 00-.75.75v.5c0 .414.336.75.75.75h3.5a.75.75 0 00.75-.75v-.5a.75.75 0 00-.75-.75h-3.5z" clipRule="evenodd" />
                                <path d="M16.5 9.5a3.5 3.5 0 11-7 0 3.5 3.5 0 017 0zm-3-1.75a.5.5 0 00-1 0v1.25H11.25a.5.5 0 000 1H12.5v1.25a.5.5 0 001 0V10h1.25a.5.5 0 000-1H13.5V7.75z" />
                              </svg>
                            </Link>
                            {!tag.is_system && (
                              <button
                                onClick={() => setDeleteConfirmId(tag.id)}
                                className="p-1.5 rounded-md text-neutral-400 hover:text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-900/20 transition-colors cursor-pointer"
                                title="Löschen"
                              >
                                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                                  <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4zM8.58 7.72a.75.75 0 00-1.5.06l.3 7.5a.75.75 0 101.5-.06l-.3-7.5zm4.34.06a.75.75 0 10-1.5-.06l-.3 7.5a.75.75 0 101.5.06l.3-7.5z" clipRule="evenodd" />
                                </svg>
                              </button>
                            )}
                          </>
                        )}
                      </div>
                    )}
                  </td>
                </tr>
                {(expandedActivitiesId === tag.id || editingId === tag.id) && (
                  <TagActivitiesPanel
                    tag={tag}
                    onClose={() => setExpandedActivitiesId(null)}
                  />
                )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="text-center py-16 text-neutral-400">
          Noch keine Tags vorhanden.
        </div>
      )}

      {/* Full edit form below table (for non-inline edit, i.e. autotags/system-autotags) */}
      {editingId !== null && allTags.find(t => t.id === editingId) && (
        <div ref={editFormRef} className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">
            Tag bearbeiten: <span style={{ color: allTags.find(t => t.id === editingId)?.color }}>
              {allTags.find(t => t.id === editingId)?.name}
            </span>
          </h2>
          <TagForm
            tag={allTags.find(t => t.id === editingId)!}
            onSave={() => setEditingId(null)}
            onCancel={() => setEditingId(null)}
          />
        </div>
      )}

      {/* Tags fusionieren */}
      {allTags.length >= 2 && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Tags fusionieren</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400">
            Alle Aktivitäten des Quell-Tags werden dem Ziel-Tag zugewiesen. Der Quell-Tag wird anschließend
            gelöscht.
          </p>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-1">
                Quell-Tag (wird gelöscht)
              </label>
              <select
                value={mergeSourceId}
                onChange={e => { setMergeSourceId(e.target.value); setMergeError(null); setMergeSuccess(null) }}
                className="w-full px-2.5 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
              >
                <option value="">— Quell-Tag wählen —</option>
                {allTags.filter(t => t.category !== 'orphan' && !t.is_system && t.id !== mergeTargetId).map(t => (
                  <option key={t.id} value={t.id}>{t.name}</option>
                ))}
              </select>
              <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-1">
                System-Tags (Wetter, Auto) können nicht als Quelle gewählt werden.
              </p>
            </div>
            <div>
              <label className="block text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-1">
                Ziel-Tag (bleibt bestehen)
              </label>
              <select
                value={mergeTargetId}
                onChange={e => { setMergeTargetId(e.target.value); setMergeError(null); setMergeSuccess(null) }}
                className="w-full px-2.5 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
              >
                <option value="">— Ziel-Tag wählen —</option>
                {allTags.filter(t => t.category !== 'orphan' && t.id !== mergeSourceId).map(t => (
                  <option key={t.id} value={t.id}>{t.name}</option>
                ))}
              </select>
            </div>
          </div>

          {mergeSourceId && mergeTargetId && (() => {
            const src = allTags.find(t => t.id === mergeSourceId)
            const tgt = allTags.find(t => t.id === mergeTargetId)
            if (!src || !tgt) return null
            return (
              <div className="rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 px-4 py-3 text-sm text-amber-800 dark:text-amber-200">
                Alle Aktivitäten von{' '}
                <span className="font-semibold" style={{ color: src.color }}>{src.name}</span>
                {' '}werden{' '}
                <span className="font-semibold" style={{ color: tgt.color }}>{tgt.name}</span>
                {' '}zugewiesen.{' '}
                <span className="font-semibold" style={{ color: src.color }}>{src.name}</span>
                {' '}wird anschließend gelöscht.
              </div>
            )
          })()}

          {mergeError && (
            <p className="text-sm text-rose-500">{mergeError}</p>
          )}
          {mergeSuccess && (
            <p className="text-sm text-emerald-600 dark:text-emerald-400">{mergeSuccess}</p>
          )}

          <button
            onClick={() => {
              if (!mergeSourceId || !mergeTargetId) return
              setMergeError(null)
              setMergeSuccess(null)
              mergeMut.mutate({ targetId: mergeTargetId, sourceId: mergeSourceId })
            }}
            disabled={!mergeSourceId || !mergeTargetId || mergeSourceId === mergeTargetId || mergeMut.isPending}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
          >
            {mergeMut.isPending ? 'Fusioniere…' : 'Fusionieren'}
          </button>
        </div>
      )}
      {/* Orphan tags: tag names in activities but not in tags table */}
      {allTags.filter(t => t.category === 'orphan').length > 0 && (
        <div className="border border-amber-200 dark:border-amber-800 rounded-xl p-6 space-y-4">
          <div>
            <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Verwaiste Tags</h2>
            <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
              Diese Tag-Namen sind in Aktivitäten vorhanden, haben aber keinen entsprechenden Eintrag in der Tag-Verwaltung.
              Sie stammen oft aus älteren Importen oder gelöschten Tags.
            </p>
          </div>
          <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-neutral-50 dark:bg-neutral-900/50 border-b border-neutral-200 dark:border-neutral-800">
                  <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Tag</th>
                  <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Aktivitäten</th>
                  <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Aktionen</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
                {allTags.filter(t => t.category === 'orphan').map(tag => (
                  <tr key={`orphan-${tag.name}`} className="group even:bg-neutral-50/50 dark:even:bg-neutral-800/20">
                    <td className="px-4 py-3">
                      <span
                        className="inline-block px-2.5 py-1 rounded-full text-xs font-medium"
                        style={{ backgroundColor: '#9CA3AF', color: '#FFFFFF' }}
                      >
                        {tag.name}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                      {tag.activity_count}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1.5 opacity-0 group-hover:opacity-100 transition-opacity">
                        <button
                          onClick={() => deleteOrphanMut.mutate(tag.name)}
                          disabled={deleteOrphanMut.isPending}
                          className="px-2.5 py-1 rounded-md text-xs font-medium text-rose-600 dark:text-rose-400 border border-rose-200 dark:border-rose-800 hover:bg-rose-50 dark:hover:bg-rose-900/20 disabled:opacity-50 transition-colors cursor-pointer"
                          title="Aus allen Aktivitäten entfernen"
                        >
                          Entfernen
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Delete confirm dialog */}
      {deleteConfirmId && (() => {
        const tagToDelete = allTags.find(t => t.id === deleteConfirmId)
        if (!tagToDelete) return null
        return (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setDeleteConfirmId(null)}>
            <div className="bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-700 p-6 shadow-xl max-w-sm w-full mx-4 space-y-4" onClick={(e) => e.stopPropagation()}>
              <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Tag löschen</h3>
              <p className="text-sm text-neutral-600 dark:text-neutral-400">
                Soll der Tag{' '}
                <span className="inline-block px-2 py-0.5 rounded-full text-xs font-medium" style={{ backgroundColor: tagToDelete.color || '#f3f4f6', color: tagToDelete.text_color || '#374151' }}>
                  {tagToDelete.name}
                </span>
                {' '}wirklich gelöscht werden?
                {tagToDelete.activity_count > 0 && (
                  <span className="block mt-2 text-amber-600 dark:text-amber-400">
                    Der Tag ist {tagToDelete.activity_count} Aktivität{tagToDelete.activity_count !== 1 ? 'en' : ''} zugeordnet.
                  </span>
                )}
              </p>
              <div className="flex items-center justify-end gap-3">
                <button
                  onClick={() => setDeleteConfirmId(null)}
                  className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                >
                  Abbrechen
                </button>
                <button
                  onClick={() => deleteMut.mutate(deleteConfirmId)}
                  disabled={deleteMut.isPending}
                  className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-rose-500 hover:bg-rose-600 disabled:opacity-50 transition-colors cursor-pointer"
                >
                  {deleteMut.isPending ? 'Wird gelöscht...' : 'Löschen'}
                </button>
              </div>
            </div>
          </div>
        )
      })()}
    </div>
  )
}
