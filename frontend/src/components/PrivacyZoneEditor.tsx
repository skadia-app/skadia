import { useRef, useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { createPrivacyZone, updatePrivacyZone, type PrivacyZone } from '@/api/privacy'

interface Props {
  zone: PrivacyZone | null
  onClose: () => void
}

function circlePolygon(center: [number, number], radiusM: number, steps = 64): GeoJSON.Feature {
  const coords: [number, number][] = []
  const lat = center[1]
  const lng = center[0]

  for (let i = 0; i <= steps; i++) {
    const angle = (i / steps) * 2 * Math.PI
    const dx = radiusM * Math.cos(angle)
    const dy = radiusM * Math.sin(angle)
    const dlat = dy / 111320
    const dlng = dx / (111320 * Math.cos((lat * Math.PI) / 180))
    coords.push([lng + dlng, lat + dlat])
  }

  return {
    type: 'Feature',
    properties: {},
    geometry: { type: 'Polygon', coordinates: [coords] },
  }
}

export default function PrivacyZoneEditor({ zone, onClose }: Props) {
  const queryClient = useQueryClient()
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)

  const [name, setName] = useState(zone?.name ?? '')
  const [lat, setLat] = useState(zone?.lat ?? 51.16)
  const [lng, setLng] = useState(zone?.lng ?? 10.45)
  const [radius, setRadius] = useState(zone?.radius_m ?? 200)
  const hasInitialPosition = zone != null

  const createMut = useMutation({
    mutationFn: createPrivacyZone,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['privacy-zones'] })
      onClose()
    },
  })

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Parameters<typeof updatePrivacyZone>[1] }) =>
      updatePrivacyZone(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['privacy-zones'] })
      onClose()
    },
  })

  function handleSave() {
    const trimmed = name.trim()
    if (!trimmed) return

    if (zone) {
      updateMut.mutate({
        id: zone.id,
        data: { name: trimmed, lat, lng, radius_m: radius },
      })
    } else {
      createMut.mutate({ name: trimmed, lat, lng, radius_m: radius })
    }
  }

  // Update circle overlay
  function updateCircle(map: maplibregl.Map, center: [number, number], r: number) {
    const source = map.getSource('zone-circle') as maplibregl.GeoJSONSource | undefined
    if (source) {
      source.setData(circlePolygon(center, r))
    }
  }

  // Initialize map
  useEffect(() => {
    if (!mapContainerRef.current) return

    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: {
        version: 8,
        sources: {
          osm: {
            type: 'raster',
            tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
            tileSize: 256,
            attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
          },
        },
        layers: [
          { id: 'osm-tiles', type: 'raster', source: 'osm', minzoom: 0, maxzoom: 19 },
        ],
      },
      center: [lng, lat],
      zoom: hasInitialPosition ? 14 : 6,
      attributionControl: {},
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      // Circle zone overlay
      map.addSource('zone-circle', {
        type: 'geojson',
        data: circlePolygon([lng, lat], radius),
      })
      map.addLayer({
        id: 'zone-circle-fill',
        type: 'fill',
        source: 'zone-circle',
        paint: {
          'fill-color': '#F97316',
          'fill-opacity': 0.15,
        },
      })
      map.addLayer({
        id: 'zone-circle-border',
        type: 'line',
        source: 'zone-circle',
        paint: {
          'line-color': '#F97316',
          'line-width': 2,
          'line-opacity': 0.6,
        },
      })

      // Place marker
      if (hasInitialPosition) {
        const marker = new maplibregl.Marker({ color: '#F97316', draggable: true })
          .setLngLat([lng, lat])
          .addTo(map)
        marker.on('dragend', () => {
          const pos = marker.getLngLat()
          setLat(pos.lat)
          setLng(pos.lng)
          updateCircle(map, [pos.lng, pos.lat], radius)
        })
        markerRef.current = marker
      }
    })

    // Click on map to set position
    map.on('click', (e) => {
      const newLat = e.lngLat.lat
      const newLng = e.lngLat.lng
      setLat(newLat)
      setLng(newLng)

      if (markerRef.current) {
        markerRef.current.setLngLat([newLng, newLat])
      } else {
        const marker = new maplibregl.Marker({ color: '#F97316', draggable: true })
          .setLngLat([newLng, newLat])
          .addTo(map)
        marker.on('dragend', () => {
          const pos = marker.getLngLat()
          setLat(pos.lat)
          setLng(pos.lng)
          updateCircle(map, [pos.lng, pos.lat], radius)
        })
        markerRef.current = marker
      }

      updateCircle(map, [newLng, newLat], radius)
    })

    mapRef.current = map
    return () => {
      map.remove()
      mapRef.current = null
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Update circle when radius changes
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    const apply = () => updateCircle(map, [lng, lat], radius)
    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
  }, [radius, lat, lng])

  const isSaving = createMut.isPending || updateMut.isPending
  const error = createMut.error || updateMut.error

  return (
    <div className="space-y-4">
      {/* Map */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <div ref={mapContainerRef} className="h-[400px] w-full" />
      </div>

      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        Klicke auf die Karte, um das Zentrum der Zone festzulegen. Den Marker kannst du auch verschieben.
      </p>

      {/* Name */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Name
        </label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="z.B. Zuhause, Arbeitsplatz"
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-orange-500 focus:border-transparent"
        />
      </div>

      {/* Radius slider */}
      <div>
        <div className="flex items-center justify-between mb-1">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            Radius
          </label>
          <span className="text-sm text-neutral-500 tabular-nums">{radius} m</span>
        </div>
        <input
          type="range"
          min={50}
          max={500}
          step={10}
          value={radius}
          onChange={(e) => setRadius(Number(e.target.value))}
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-orange-500 cursor-pointer"
        />
        <div className="flex justify-between text-xs text-neutral-400 mt-1">
          <span>50 m</span>
          <span>500 m</span>
        </div>
      </div>

      {/* Coordinates display */}
      <div className="text-xs text-neutral-400 tabular-nums">
        {lat.toFixed(5)}, {lng.toFixed(5)}
      </div>

      {error && (
        <div className="text-sm text-red-500">
          Fehler beim Speichern. Bitte versuche es erneut.
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center gap-3">
        <button
          onClick={handleSave}
          disabled={!name.trim() || isSaving}
          className="bg-orange-500 hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed text-white rounded-lg px-4 py-2 text-sm font-medium transition-colors"
        >
          {isSaving ? 'Speichern...' : 'Speichern'}
        </button>
        <button
          onClick={onClose}
          className="text-sm text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 transition-colors"
        >
          Abbrechen
        </button>
      </div>
    </div>
  )
}
