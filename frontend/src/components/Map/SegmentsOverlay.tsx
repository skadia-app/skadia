import { useEffect, useRef, useState, useCallback } from 'react'
import maplibregl from 'maplibre-gl'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchSegments, type Segment, type SegmentsResponse } from '@/api/segments'

export interface SegmentColorConfig {
  standard: string
  trail: string
  lift: string
  ridden: string
  favorite: string
}

export const DEFAULT_SEGMENT_COLORS: SegmentColorConfig = {
  standard: '#6B7280',
  trail: '#EC4899',
  lift: '#6366F1',
  ridden: '#F97316',
  favorite: '#FFD700',
}

export interface SegmentsOverlayFilters {
  terrainTypes: string[]   // [] = alle anzeigen; ['lift'] = nur Lift; etc.
  riddenOnly: boolean
  favoritesOnly: boolean
  sportGroups: string[]    // [] = alle Sport-Gruppen; ['bike'] = nur Rad etc.
}

export const DEFAULT_SEGMENT_FILTERS: SegmentsOverlayFilters = {
  terrainTypes: [],
  riddenOnly: false,
  favoritesOnly: false,
  sportGroups: [],
}

interface SegmentsOverlayProps {
  map: maplibregl.Map | null
  enabled: boolean
  filters?: Partial<SegmentsOverlayFilters>
  colorConfig?: Partial<SegmentColorConfig>
  styleVersion?: number
}

export function SegmentsOverlay({ map, enabled, filters, colorConfig, styleVersion }: SegmentsOverlayProps) {
  const navigate = useNavigate()
  const popupRef = useRef<maplibregl.Popup | null>(null)

  const mergedFilters: SegmentsOverlayFilters = { ...DEFAULT_SEGMENT_FILTERS, ...filters }
  const colors: SegmentColorConfig = { ...DEFAULT_SEGMENT_COLORS, ...colorConfig }

  // Viewport-bbox für räumlichen Filter — nur Segmente im sichtbaren Bereich laden
  const [bbox, setBbox] = useState('')
  const updateBbox = useCallback(() => {
    if (!map) return
    const b = map.getBounds()
    setBbox(`${b.getWest().toFixed(5)},${b.getSouth().toFixed(5)},${b.getEast().toFixed(5)},${b.getNorth().toFixed(5)}`)
  }, [map])

  useEffect(() => {
    if (!map || !enabled) return
    updateBbox()
    map.on('moveend', updateBbox)
    return () => { map.off('moveend', updateBbox) }
  }, [map, enabled, updateBbox])

  const { data: rawData } = useQuery<SegmentsResponse>({
    queryKey: ['segments-overlay-list', bbox],
    queryFn: () => fetchSegments({ include_geojson: true, limit: 500, ...(bbox ? { bbox } : {}) }),
    enabled: enabled && !!map && bbox !== '',
    staleTime: 5 * 60 * 1000,
  })
  const segmentData: Segment[] = rawData?.segments ?? []

  useEffect(() => {
    if (!map) return

    const sourceId = 'segments-overlay-source'
    const favoriteLayerId = 'segments-overlay-favorites'
    const trailLayerId = 'segments-overlay-trails'
    const lineLayerId = 'segments-overlay-lines'
    const dotLayerId = 'segments-overlay-dots'

    const cleanup = () => {
      if (popupRef.current) {
        popupRef.current.remove()
        popupRef.current = null
      }
      try {
        if (map.getLayer(dotLayerId)) map.removeLayer(dotLayerId)
        if (map.getLayer(lineLayerId)) map.removeLayer(lineLayerId)
        if (map.getLayer(trailLayerId)) map.removeLayer(trailLayerId)
        if (map.getLayer(favoriteLayerId)) map.removeLayer(favoriteLayerId)
        if (map.getSource(sourceId)) map.removeSource(sourceId)
      } catch {
        // map may already be removed/unmounted
      }
    }

    if (!enabled || !map.isStyleLoaded()) {
      cleanup()
      return
    }

    cleanup()

    // Filter anwenden
    const segments = segmentData.filter((seg: Segment) => {
      const segType = seg.segment_type || 'standard'
      if (mergedFilters.terrainTypes.length > 0 && !mergedFilters.terrainTypes.includes(segType)) return false
      if (mergedFilters.riddenOnly && (seg.ridden_count ?? 0) === 0) return false
      if (mergedFilters.favoritesOnly && !seg.is_favorite) return false
      if (mergedFilters.sportGroups.length > 0 && !mergedFilters.sportGroups.includes(seg.sport_group)) return false
      return true
    })

    // Farbe je Segment ermitteln
    function resolveColor(seg: Segment): string {
      const segType = seg.segment_type || 'standard'
      const isRidden = (seg.ridden_count ?? 0) > 0
      if (segType === 'lift') return colors.lift
      if (segType === 'trail') return colors.trail
      if (isRidden) return colors.ridden
      return colors.standard
    }

    // Features aus Segmenten mit track_geojson aufbauen
    const lineFeatures: GeoJSON.Feature[] = segments.reduce<GeoJSON.Feature[]>((acc, seg) => {
      if (!seg.track_geojson) return acc
      let geometry: GeoJSON.Geometry
      try {
        geometry = JSON.parse(seg.track_geojson) as GeoJSON.Geometry
      } catch {
        return acc
      }
      acc.push({
        type: 'Feature' as const,
        geometry,
        properties: {
          id: seg.id,
          name: seg.name,
          segment_type: seg.segment_type || 'standard',
          sport_group: seg.sport_group,
          effort_count: seg.effort_count ?? 0,
          ridden_count: seg.ridden_count ?? 0,
          distance_m: seg.distance_m ?? 0,
          is_favorite: seg.is_favorite ?? false,
          color: resolveColor(seg),
        },
      })
      return acc
    }, [])

    // Dot-Features für Segmente ohne GeoJSON (nur Startpunkt aus segment_type_meta)
    const dotFeatures: GeoJSON.Feature[] = segments
      .filter(seg => !seg.track_geojson)
      .reduce<GeoJSON.Feature[]>((acc, seg) => {
        const meta = seg.segment_type_meta as Record<string, unknown> | undefined
        if (!meta || typeof meta.start_lat !== 'number' || typeof meta.start_lng !== 'number') {
          return acc
        }
        acc.push({
          type: 'Feature' as const,
          geometry: {
            type: 'Point' as const,
            coordinates: [meta.start_lng as number, meta.start_lat as number],
          },
          properties: {
            id: seg.id,
            name: seg.name,
            segment_type: seg.segment_type || 'standard',
            sport_group: seg.sport_group,
            effort_count: seg.effort_count ?? 0,
            ridden_count: seg.ridden_count ?? 0,
            is_favorite: seg.is_favorite ?? false,
            color: resolveColor(seg),
          },
        })
        return acc
      }, [])

    const allFeatures = [...lineFeatures, ...dotFeatures]
    if (allFeatures.length === 0) return

    const geojson: GeoJSON.FeatureCollection = {
      type: 'FeatureCollection',
      features: allFeatures,
    }

    map.addSource(sourceId, { type: 'geojson', data: geojson })

    // Favoriten-Glow-Layer (breite halbtransparente Gold-Linie hinter den normalen Linien)
    map.addLayer({
      id: favoriteLayerId,
      type: 'line',
      source: sourceId,
      filter: ['all',
        ['==', ['geometry-type'], 'LineString'],
        ['==', ['get', 'is_favorite'], true],
      ],
      paint: {
        'line-color': colors.favorite ?? '#FFD700',
        'line-width': 6,
        'line-opacity': 0.4,
      },
      layout: { 'line-cap': 'round', 'line-join': 'round' },
    })

    // Trail-Layer: gestrichelte Linien
    map.addLayer({
      id: trailLayerId,
      type: 'line',
      source: sourceId,
      filter: ['all',
        ['==', ['geometry-type'], 'LineString'],
        ['==', ['get', 'segment_type'], 'trail'],
      ],
      paint: {
        'line-color': ['get', 'color'],
        'line-width': [
          'case',
          ['>', ['get', 'ridden_count'], 0], 3,
          2,
        ],
        'line-opacity': [
          'case',
          ['>', ['get', 'ridden_count'], 0], 0.9,
          0.6,
        ],
        'line-dasharray': [4, 2],
      },
      layout: { 'line-cap': 'round', 'line-join': 'round' },
    })

    // Standard-Linie-Layer (alle außer Trails)
    map.addLayer({
      id: lineLayerId,
      type: 'line',
      source: sourceId,
      filter: ['all',
        ['==', ['geometry-type'], 'LineString'],
        ['!=', ['get', 'segment_type'], 'trail'],
      ],
      paint: {
        'line-color': ['get', 'color'],
        'line-width': [
          'case',
          ['>', ['get', 'ridden_count'], 0], 3,
          2,
        ],
        'line-opacity': [
          'case',
          ['>', ['get', 'ridden_count'], 0], 0.9,
          0.6,
        ],
      },
      layout: {
        'line-cap': 'round',
        'line-join': 'round',
      },
    })

    // Punkt-Layer für Segmente ohne Track
    map.addLayer({
      id: dotLayerId,
      type: 'circle',
      source: sourceId,
      filter: ['==', ['geometry-type'], 'Point'],
      paint: {
        'circle-radius': 6,
        'circle-color': ['get', 'color'],
        'circle-stroke-width': 2,
        'circle-stroke-color': '#ffffff',
        'circle-opacity': 0.85,
      },
    })

    // Hover-Tooltip
    const popup = new maplibregl.Popup({
      closeButton: false,
      closeOnClick: false,
      offset: 8,
      className: 'segment-overlay-popup',
    })
    popupRef.current = popup

    const onMouseEnterLine = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      map.getCanvas().style.cursor = 'pointer'
      const feature = e.features?.[0]
      if (!feature) return
      const name = feature.properties?.name ?? ''
      const effortCount = feature.properties?.effort_count ?? 0
      const segType = feature.properties?.segment_type ?? 'standard'
      const typeLabel = segType === 'lift' ? 'Lift' : segType === 'trail' ? 'Trail' : 'Standard'
      const effortLabel = effortCount > 0 ? ` · ${effortCount}× gefahren` : ''
      const favLabel = feature.properties?.is_favorite ? ' \u2605' : ''
      const coords = e.lngLat
      popup
        .setLngLat([coords.lng, coords.lat])
        .setHTML(
          `<span style="font-size:12px;font-weight:500">${name}${favLabel}</span>` +
          `<span style="font-size:11px;color:#9ca3af;margin-left:6px">${typeLabel}${effortLabel}</span>`,
        )
        .addTo(map)
    }

    const onMouseEnterDot = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      map.getCanvas().style.cursor = 'pointer'
      const feature = e.features?.[0]
      if (!feature) return
      const name = feature.properties?.name ?? ''
      const coords = (feature.geometry as GeoJSON.Point).coordinates.slice() as [number, number]
      popup.setLngLat(coords).setHTML(`<span style="font-size:12px;font-weight:500">${name}</span>`).addTo(map)
    }

    const onMouseLeave = () => {
      map.getCanvas().style.cursor = ''
      popup.remove()
    }

    const onClickLine = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      const id = e.features?.[0]?.properties?.id
      if (id) navigate(`/segments/${id}`)
    }

    const onClickDot = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      const id = e.features?.[0]?.properties?.id
      if (id) navigate(`/segments/${id}`)
    }

    map.on('mouseenter', lineLayerId, onMouseEnterLine)
    map.on('mouseleave', lineLayerId, onMouseLeave)
    map.on('click', lineLayerId, onClickLine)
    map.on('mouseenter', trailLayerId, onMouseEnterLine)
    map.on('mouseleave', trailLayerId, onMouseLeave)
    map.on('click', trailLayerId, onClickLine)
    map.on('mouseenter', favoriteLayerId, onMouseEnterLine)
    map.on('mouseleave', favoriteLayerId, onMouseLeave)
    map.on('click', favoriteLayerId, onClickLine)
    map.on('mouseenter', dotLayerId, onMouseEnterDot)
    map.on('mouseleave', dotLayerId, onMouseLeave)
    map.on('click', dotLayerId, onClickDot)

    return () => {
      map.off('mouseenter', lineLayerId, onMouseEnterLine)
      map.off('mouseleave', lineLayerId, onMouseLeave)
      map.off('click', lineLayerId, onClickLine)
      map.off('mouseenter', trailLayerId, onMouseEnterLine)
      map.off('mouseleave', trailLayerId, onMouseLeave)
      map.off('click', trailLayerId, onClickLine)
      map.off('mouseenter', favoriteLayerId, onMouseEnterLine)
      map.off('mouseleave', favoriteLayerId, onMouseLeave)
      map.off('click', favoriteLayerId, onClickLine)
      map.off('mouseenter', dotLayerId, onMouseEnterDot)
      map.off('mouseleave', dotLayerId, onMouseLeave)
      map.off('click', dotLayerId, onClickDot)
      cleanup()
    }
  }, [map, enabled, segmentData, navigate, styleVersion, filters, colorConfig]) // eslint-disable-line react-hooks/exhaustive-deps

  return null
}
