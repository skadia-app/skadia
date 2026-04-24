import { useEffect, useRef } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { buildRasterStyle } from '@/lib/mapStyles'

interface MapPreviewProps {
  geojsonTrack: string | null | undefined
  /**
   * CSS height string for the map container.
   * Default '200px' suits inline-editor contexts (track/draw mode).
   * Pass '160px' for compact modal contexts (SegmentEditModal).
   */
  height?: string
  className?: string
}

export function MapPreview({ geojsonTrack, height = '200px', className }: MapPreviewProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  
  useEffect(() => {
    if (!containerRef.current || !geojsonTrack) return
    
    let geojson: GeoJSON.Feature<GeoJSON.LineString>
    try {
      if (typeof geojsonTrack === 'string') {
        geojson = JSON.parse(geojsonTrack)
      } else {
        // Assume it's already an object if not string
        geojson = geojsonTrack as any
      }
    } catch (e) {
      console.warn('MapPreview: invalid GeoJSON', e)
      return
    }
    
    const coords = geojson.geometry?.coordinates ?? []
    
    const map = new maplibregl.Map({
      container: containerRef.current,
      style: buildRasterStyle('osm'),
      center: (coords[0] as [number, number]) ?? [11, 47],
      zoom: coords.length < 2 ? 14 : 10,
      interactive: true,
      attributionControl: false,
    })
    mapRef.current = map
    
    map.on('load', () => {
      map.addSource('track', { type: 'geojson', data: geojson })
      // White casing underneath for contrast against busy topo tiles.
      map.addLayer({
        id: 'track-line-casing',
        type: 'line',
        source: 'track',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': '#ffffff', 'line-width': 7, 'line-opacity': 0.95 },
      })
      map.addLayer({
        id: 'track-line',
        type: 'line',
        source: 'track',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': '#F97316', 'line-width': 4, 'line-opacity': 1.0 },
      })
      
      if (coords.length >= 2) {
        const bounds = new maplibregl.LngLatBounds(coords[0] as [number, number], coords[0] as [number, number])
        coords.forEach(c => bounds.extend(c as [number, number]))
        map.fitBounds(bounds, { padding: 20, animate: false })
      }
    })
    
    return () => { 
      if (mapRef.current) {
        mapRef.current.remove()
        mapRef.current = null
      }
    }
  }, [geojsonTrack])
  
  if (!geojsonTrack) {
    return (
      <div className={`flex items-center justify-center bg-neutral-100 dark:bg-neutral-800 text-neutral-500 text-sm rounded-lg ${className}`} style={{ height }}>
        Kartenvorschau nicht verfügbar
      </div>
    )
  }
  
  return <div ref={containerRef} style={{ height, width: '100%' }} className={`rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 ${className}`} />
}
