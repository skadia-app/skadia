import { useEffect, useRef } from 'react'
import maplibregl from 'maplibre-gl'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchSegments, type Segment } from '@/api/segments'

export interface SegmentColorConfig {
  standard: string
  trail: string
  lift: string
  ridden: string
}

export const DEFAULT_SEGMENT_COLORS: SegmentColorConfig = {
  standard: '#6B7280',
  trail: '#EC4899',
  lift: '#6366F1',
  ridden: '#F97316',
}

export interface SegmentsOverlayFilters {
  terrainTypes: string[]   // [] = alle anzeigen; ['lift'] = nur Lift; etc.
  riddenOnly: boolean
  sportGroups: string[]    // [] = alle Sport-Gruppen; ['bike'] = nur Rad etc.
}

export const DEFAULT_SEGMENT_FILTERS: SegmentsOverlayFilters = {
  terrainTypes: [],
  riddenOnly: false,
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

  const { data: segmentData = [] } = useQuery<Segment[]>({
    queryKey: ['segments-overlay-list'],
    queryFn: () => fetchSegments(),
    enabled: enabled && !!map,
    staleTime: 5 * 60 * 1000, // 5 Minuten cachen
  })

  useEffect(() => {
    if (!map) return

    const sourceId = 'segments-overlay-source'
    const lineLayerId = 'segments-overlay-lines'
    const dotLayerId = 'segments-overlay-dots'

    const cleanup = () => {
      if (popupRef.current) {
        popupRef.current.remove()
        popupRef.current = null
      }
      if (map.getLayer(dotLayerId)) map.removeLayer(dotLayerId)
      if (map.getLayer(lineLayerId)) map.removeLayer(lineLayerId)
      if (map.getSource(sourceId)) map.removeSource(sourceId)
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

    // Linie-Layer: farbig nach Typ
    map.addLayer({
      id: lineLayerId,
      type: 'line',
      source: sourceId,
      filter: ['==', ['geometry-type'], 'LineString'],
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
      const coords = e.lngLat
      popup
        .setLngLat([coords.lng, coords.lat])
        .setHTML(
          `<span style="font-size:12px;font-weight:500">${name}</span>` +
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
    map.on('mouseenter', dotLayerId, onMouseEnterDot)
    map.on('mouseleave', dotLayerId, onMouseLeave)
    map.on('click', dotLayerId, onClickDot)

    return () => {
      map.off('mouseenter', lineLayerId, onMouseEnterLine)
      map.off('mouseleave', lineLayerId, onMouseLeave)
      map.off('click', lineLayerId, onClickLine)
      map.off('mouseenter', dotLayerId, onMouseEnterDot)
      map.off('mouseleave', dotLayerId, onMouseLeave)
      map.off('click', dotLayerId, onClickDot)
      cleanup()
    }
  }, [map, enabled, segmentData, navigate, styleVersion, filters, colorConfig]) // eslint-disable-line react-hooks/exhaustive-deps

  return null
}
