import { useRef, useEffect } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import type { SummitVisit } from '@/api/summits'

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

function addMarkers(
  map: maplibregl.Map,
  summits: SummitVisit[],
  onSelect: (summit: SummitVisit) => void,
): maplibregl.Marker[] {
  return summits.map(summit => {
    const el = document.createElement('div')
    el.className = 'summit-marker'
    el.style.cssText = `
      width: 28px; height: 28px; cursor: pointer;
      display: flex; align-items: center; justify-content: center;
    `
    el.innerHTML = `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#c2410c" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 20l6.5-11L13 15l4-6 4 11H3z"/></svg>`

    el.addEventListener('click', () => onSelect(summit))

    return new maplibregl.Marker({ element: el })
      .setLngLat([summit.lng!, summit.lat!])
      .addTo(map)
  })
}

interface SummitMapProps {
  summits: SummitVisit[]
  selectedSummit?: SummitVisit | null
  onSelectSummit: (summit: SummitVisit) => void
  onMaximize: () => void
}

export default function SummitMap({ summits, selectedSummit, onSelectSummit, onMaximize }: SummitMapProps) {
  const mapContainer = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markersRef = useRef<maplibregl.Marker[]>([])

  // Initialize map once
  useEffect(() => {
    if (!mapContainer.current || mapRef.current) return

    const summitsWithCoords = summits.filter(s => s.lat != null && s.lng != null)

    let center: [number, number] = [11.4, 47.3]
    if (summitsWithCoords.length > 0) {
      const lats = summitsWithCoords.map(s => s.lat!)
      const lngs = summitsWithCoords.map(s => s.lng!)
      center = [
        (Math.min(...lngs) + Math.max(...lngs)) / 2,
        (Math.min(...lats) + Math.max(...lats)) / 2,
      ]
    }

    const map = new maplibregl.Map({
      container: mapContainer.current,
      style: MAP_STYLE,
      center,
      zoom: 7,
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      markersRef.current = addMarkers(map, summitsWithCoords, onSelectSummit)

      if (summitsWithCoords.length > 1) {
        const bounds = new maplibregl.LngLatBounds()
        summitsWithCoords.forEach(s => bounds.extend([s.lng!, s.lat!]))
        map.fitBounds(bounds, { padding: 60, maxZoom: 13 })
      } else if (summitsWithCoords.length === 1) {
        map.setCenter([summitsWithCoords[0].lng!, summitsWithCoords[0].lat!])
        map.setZoom(12)
      }
    })

    mapRef.current = map

    return () => {
      map.remove()
      mapRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Update markers when summits or selection changes
  useEffect(() => {
    if (!mapRef.current) return
    const summitsWithCoords = summits.filter(s => s.lat != null && s.lng != null)
    markersRef.current.forEach(m => m.remove())
    markersRef.current = addMarkers(mapRef.current, summitsWithCoords, onSelectSummit)
  }, [summits, onSelectSummit])

  // Fly to selected summit
  useEffect(() => {
    if (!mapRef.current || !selectedSummit) return
    if (selectedSummit.lat == null || selectedSummit.lng == null) return
    mapRef.current.flyTo({ center: [selectedSummit.lng, selectedSummit.lat], zoom: 12 })
  }, [selectedSummit])

  return (
    <div className="relative w-full h-[500px] rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={mapContainer} className="absolute inset-0" />
      {/* Maximize button */}
      <button
        onClick={onMaximize}
        className="absolute top-3 left-3 z-10 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm
                   rounded-lg p-1.5 shadow-sm hover:bg-white dark:hover:bg-neutral-700 transition-colors"
        title="Karte maximieren"
      >
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4 text-neutral-600 dark:text-neutral-300">
          <path d="M13.28 7.78l3.22-3.22v2.69a.75.75 0 001.5 0v-4.5a.75.75 0 00-.75-.75h-4.5a.75.75 0 000 1.5h2.69l-3.22 3.22a.75.75 0 001.06 1.06zM2 17.25v-4.5a.75.75 0 011.5 0v2.69l3.22-3.22a.75.75 0 011.06 1.06L4.56 16.5h2.69a.75.75 0 010 1.5h-4.5a.75.75 0 01-.75-.75z" />
        </svg>
      </button>
    </div>
  )
}
