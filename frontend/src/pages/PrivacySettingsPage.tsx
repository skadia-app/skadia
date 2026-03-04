import { useRef, useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { listPrivacyZones, deletePrivacyZone, updatePrivacyZone, type PrivacyZone } from '@/api/privacy'
import PrivacyZoneEditor from '@/components/PrivacyZoneEditor'

function circlePolygon(center: [number, number], radiusM: number, steps = 64): GeoJSON.Feature {
  const coords: [number, number][] = []
  const [lng, lat] = center
  for (let i = 0; i <= steps; i++) {
    const angle = (i / steps) * 2 * Math.PI
    const dx = radiusM * Math.cos(angle)
    const dy = radiusM * Math.sin(angle)
    const dlat = dy / 111320
    const dlng = dx / (111320 * Math.cos((lat * Math.PI) / 180))
    coords.push([lng + dlng, lat + dlat])
  }
  return { type: 'Feature', properties: {}, geometry: { type: 'Polygon', coordinates: [coords] } }
}

function buildZonesGeoJSON(zones: PrivacyZone[]): GeoJSON.FeatureCollection {
  return {
    type: 'FeatureCollection',
    features: zones.map(z => ({
      ...circlePolygon([z.lng, z.lat], z.radius_m),
      properties: { id: z.id, is_active: z.is_active },
    })),
  }
}

interface OverviewMapProps {
  zones: PrivacyZone[]
  onZoneClick: (zone: PrivacyZone) => void
}

function OverviewMap({ zones, onZoneClick }: OverviewMapProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markersRef = useRef<maplibregl.Marker[]>([])

  // Init map once
  useEffect(() => {
    if (!containerRef.current) return

    const map = new maplibregl.Map({
      container: containerRef.current,
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
        layers: [{ id: 'osm-tiles', type: 'raster', source: 'osm', minzoom: 0, maxzoom: 19 }],
      },
      center: [10.45, 51.16],
      zoom: 5,
      attributionControl: {},
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      map.addSource('zones', { type: 'geojson', data: buildZonesGeoJSON([]) })
      map.addLayer({
        id: 'zones-fill',
        type: 'fill',
        source: 'zones',
        paint: { 'fill-color': '#F97316', 'fill-opacity': ['case', ['get', 'is_active'], 0.15, 0.06] },
      })
      map.addLayer({
        id: 'zones-border',
        type: 'line',
        source: 'zones',
        paint: {
          'line-color': '#F97316',
          'line-width': 2,
          'line-opacity': ['case', ['get', 'is_active'], 0.7, 0.3],
          'line-dasharray': ['case', ['get', 'is_active'], ['literal', [1]], ['literal', [4, 3]]],
        },
      })
    })

    mapRef.current = map
    return () => { map.remove(); mapRef.current = null }
  }, [])

  // Update circles + markers whenever zones change
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    const apply = () => {
      // Update GeoJSON circles
      const src = map.getSource('zones') as maplibregl.GeoJSONSource | undefined
      src?.setData(buildZonesGeoJSON(zones))

      // Remove old markers
      markersRef.current.forEach(m => m.remove())
      markersRef.current = []

      if (zones.length === 0) return

      // Add new markers
      zones.forEach(zone => {
        const el = document.createElement('div')
        el.title = zone.name
        el.style.cssText = `
          width:28px;height:28px;border-radius:50%;cursor:pointer;
          background:${zone.is_active ? '#F97316' : '#9ca3af'};
          border:3px solid white;box-shadow:0 2px 6px rgba(0,0,0,.3);
          display:flex;align-items:center;justify-content:center;
          font-size:10px;font-weight:700;color:white;
        `
        el.addEventListener('click', () => onZoneClick(zone))

        const marker = new maplibregl.Marker({ element: el })
          .setLngLat([zone.lng, zone.lat])
          .setPopup(new maplibregl.Popup({ offset: 18, closeButton: false })
            .setHTML(`<span style="font-size:12px;font-weight:500">${zone.name}</span><br/><span style="font-size:11px;color:#9ca3af">${zone.radius_m} m · ${zone.is_active ? 'Aktiv' : 'Inaktiv'}</span>`))
          .addTo(map)

        markersRef.current.push(marker)
      })

      // Fit bounds to all zones + padding
      if (zones.length > 0) {
        const bounds = new maplibregl.LngLatBounds()
        zones.forEach(z => {
          const r = z.radius_m / 111320
          bounds.extend([z.lng - r, z.lat - r])
          bounds.extend([z.lng + r, z.lat + r])
        })
        map.fitBounds(bounds, { padding: 60, maxZoom: 15, duration: 600 })
      }
    }

    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
  }, [zones, onZoneClick])

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={containerRef} className="h-[340px] w-full" />
      {zones.length === 0 && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <span className="text-xs text-neutral-400 bg-white/80 dark:bg-neutral-900/80 px-3 py-1 rounded-full">
            Noch keine Zonen — erstelle deine erste Zone
          </span>
        </div>
      )}
    </div>
  )
}

export default function PrivacySettingsPage() {
  const queryClient = useQueryClient()
  const [editingZone, setEditingZone] = useState<PrivacyZone | null>(null)
  const [showEditor, setShowEditor] = useState(false)

  const { data: zones = [], isLoading } = useQuery({
    queryKey: ['privacy-zones'],
    queryFn: listPrivacyZones,
  })

  const deleteMutation = useMutation({
    mutationFn: deletePrivacyZone,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['privacy-zones'] }),
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, is_active }: { id: string; is_active: boolean }) =>
      updatePrivacyZone(id, { is_active }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['privacy-zones'] }),
  })

  function handleEdit(zone: PrivacyZone) { setEditingZone(zone); setShowEditor(true) }
  function handleNew() { setEditingZone(null); setShowEditor(true) }
  function handleEditorClose() { setShowEditor(false); setEditingZone(null) }
  function handleDelete(zone: PrivacyZone) {
    if (confirm(`"${zone.name}" wirklich löschen?`)) deleteMutation.mutate(zone.id)
  }

  if (showEditor) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          {editingZone ? 'Privacy-Zone bearbeiten' : 'Neue Privacy-Zone'}
        </h1>
        <PrivacyZoneEditor zone={editingZone} onClose={handleEditorClose} />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Privacy-Zonen</h1>
        <button
          onClick={handleNew}
          className="bg-orange-500 hover:bg-orange-600 text-white rounded-lg px-4 py-2 text-sm font-medium transition-colors"
        >
          Neue Zone
        </button>
      </div>

      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        GPS-Daten innerhalb aktiver Privacy-Zonen werden in öffentlichen Ansichten ausgeblendet.
        Schütze sensible Orte wie dein Zuhause oder deinen Arbeitsplatz.
      </p>

      {/* Übersichtskarte */}
      {!isLoading && (
        <div className="relative">
          <OverviewMap zones={zones} onZoneClick={handleEdit} />
          {zones.length > 0 && (
            <p className="text-xs text-neutral-400 mt-1">
              Klicke auf einen Marker, um die Zone zu bearbeiten.
            </p>
          )}
        </div>
      )}

      {/* Zonenliste */}
      {isLoading ? (
        <div className="flex items-center justify-center h-20 text-neutral-400">Lade Privacy-Zonen…</div>
      ) : zones.length === 0 ? (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-8 text-center">
          <p className="text-neutral-500 dark:text-neutral-400 mb-4">Noch keine Privacy-Zonen definiert.</p>
          <button onClick={handleNew} className="text-sm text-orange-500 hover:text-orange-600 font-medium transition-colors">
            Erste Zone erstellen
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          {zones.map(zone => (
            <div key={zone.id} className="flex items-center gap-4 rounded-xl border border-neutral-200 dark:border-neutral-800 p-4">
              {/* Toggle */}
              <button
                onClick={() => toggleMutation.mutate({ id: zone.id, is_active: !zone.is_active })}
                className={`relative w-10 h-6 rounded-full transition-colors flex-shrink-0 ${zone.is_active ? 'bg-orange-500' : 'bg-neutral-300 dark:bg-neutral-600'}`}
                title={zone.is_active ? 'Deaktivieren' : 'Aktivieren'}
              >
                <span className={`absolute top-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform ${zone.is_active ? 'translate-x-[18px]' : 'translate-x-0.5'}`} />
              </button>

              {/* Info */}
              <div className="flex-1 min-w-0">
                <div className="font-medium text-neutral-900 dark:text-neutral-100 truncate">{zone.name}</div>
                <div className="text-sm text-neutral-500 dark:text-neutral-400">
                  Radius: {zone.radius_m} m
                  <span className="mx-2 text-neutral-300 dark:text-neutral-600">|</span>
                  {zone.is_active ? 'Aktiv' : 'Inaktiv'}
                </div>
              </div>

              {/* Actions */}
              <div className="flex items-center gap-2">
                <button onClick={() => handleEdit(zone)} className="p-2 rounded-lg text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors" title="Bearbeiten">
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
                    <path d="M2.695 14.763l-1.262 3.154a.5.5 0 00.65.65l3.155-1.262a4 4 0 001.343-.885L17.5 5.5a2.121 2.121 0 00-3-3L3.58 13.42a4 4 0 00-.885 1.343z" />
                  </svg>
                </button>
                <button onClick={() => handleDelete(zone)} className="p-2 rounded-lg text-neutral-400 hover:text-red-500 dark:hover:text-red-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors" title="Löschen">
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
                    <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4zM8.58 7.72a.75.75 0 01.78.72l.5 6.5a.75.75 0 01-1.49.12l-.5-6.5a.75.75 0 01.71-.84zm3.62.72a.75.75 0 10-1.49-.12l-.5 6.5a.75.75 0 101.49.12l.5-6.5z" clipRule="evenodd" />
                  </svg>
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
