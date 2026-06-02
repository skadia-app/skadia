import { useEffect, useRef, useState, useCallback } from 'react'
import maplibregl from 'maplibre-gl'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { fetchSegments, type FetchSegmentsOpts, type Segment, type SegmentsResponse } from '@/api/segments'
import { enforceRouteLayersOnTop } from '@/lib/mapRouteLayers'

export interface SegmentColorConfig {
  standard: string
  trail: string
  lift: string
  wintersport: string
  wassersport: string
  checkpoint: string
  ridden: string
  favorite: string
}

export const DEFAULT_SEGMENT_COLORS: SegmentColorConfig = {
  standard: '#6B7280',
  trail: '#EC4899',
  lift: '#6366F1',
  wintersport: '#7DD3FC',
  wassersport: '#06B6D4',
  checkpoint: '#0F766E',
  ridden: '#F97316',
  favorite: '#FFD700',
}

export interface SegmentsOverlayFilters {
  terrainTypes: string[]   // [] = alle anzeigen; ['lift'] = nur Lift; etc.
  courseKinds: string[]    // [] = none selected; ['line'] = line courses; ['checkpoint_sequence'] = Checkpoint-Kurse
  riddenOnly: boolean
  favoritesOnly: boolean
  sportGroups: string[]    // [] = alle Sport-Gruppen; ['bike'] = nur Rad etc.
}

export const DEFAULT_SEGMENT_FILTERS: SegmentsOverlayFilters = {
  terrainTypes: [],
  courseKinds: ['line'],
  riddenOnly: false,
  favoritesOnly: false,
  sportGroups: [],
}

export function shouldQuerySegmentsOverlay(filters: SegmentsOverlayFilters): boolean {
  return filters.courseKinds.length > 0
}

export function buildSegmentsOverlayFetchParams({
  bbox,
  filters,
  cursor,
}: {
  bbox: string
  filters: SegmentsOverlayFilters
  cursor?: string
}): FetchSegmentsOpts {
  const requestedCourseKinds = filters.courseKinds
  const fetchCourseKinds = requestedCourseKinds.includes('checkpoint_sequence') && requestedCourseKinds.includes('line')
    ? ['line', 'checkpoint_sequence'] as const
    : undefined
  const fetchCourseKind = !fetchCourseKinds && requestedCourseKinds.includes('checkpoint_sequence')
    ? 'checkpoint_sequence' as const
    : !fetchCourseKinds && requestedCourseKinds.includes('line')
      ? 'line' as const
    : undefined

  return {
    include_geojson: true,
    limit: 500,
    ...(bbox ? { bbox } : {}),
    ...(cursor ? { cursor } : {}),
    ...(requestedCourseKinds.includes('checkpoint_sequence') ? { include_control_map_features: true } : {}),
    ...(filters.riddenOnly ? { completed_only: true } : {}),
    ...(filters.terrainTypes.length > 0 && requestedCourseKinds.includes('line') ? { segment_types: filters.terrainTypes.join(',') } : {}),
    ...(filters.sportGroups.length === 1 ? { sport_group: filters.sportGroups[0] } : {}),
    ...(fetchCourseKinds ? { course_kinds: [...fetchCourseKinds] } : {}),
    ...(fetchCourseKind ? { course_kind: fetchCourseKind } : {}),
  }
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
  const { t } = useTranslation()
  const popupRef = useRef<maplibregl.Popup | null>(null)

  const mergedFilters: SegmentsOverlayFilters = { ...DEFAULT_SEGMENT_FILTERS, ...filters }
  const colors: SegmentColorConfig = { ...DEFAULT_SEGMENT_COLORS, ...colorConfig }

  // Viewport bbox for the spatial filter: load only segments in the visible area.
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
    queryKey: ['segments-overlay-list', bbox, mergedFilters],
    queryFn: async () => {
      const segments: Segment[] = []
      let cursor: string | undefined
      let hasMore = true
      while (hasMore) {
        const page = await fetchSegments(buildSegmentsOverlayFetchParams({ bbox, filters: mergedFilters, cursor }))
        segments.push(...page.segments)
        cursor = page.next_cursor
        hasMore = Boolean(page.has_more && cursor)
      }
      return { segments, has_more: false }
    },
    enabled: enabled && !!map && bbox !== '' && shouldQuerySegmentsOverlay(mergedFilters),
    staleTime: 5 * 60 * 1000,
  })
  const segmentData: Segment[] = rawData?.segments ?? []

  useEffect(() => {
    if (!map) return

    const sourceId = 'segments-overlay-source'
    const checkpointSourceId = 'segments-overlay-checkpoints-source'
    const favoriteLayerId = 'segments-overlay-favorites'
    const trailLayerId = 'segments-overlay-trails'
    const wintersportLayerId = 'segments-wintersport'
    const wassersportLayerId = 'segments-wassersport'
    const lineLayerId = 'segments-overlay-lines'
    const dotLayerId = 'segments-overlay-dots'
    const checkpointLineLayerId = 'segments-overlay-checkpoint-lines'
    const checkpointAreaLayerId = 'segments-overlay-checkpoint-areas'
    const checkpointAreaOutlineLayerId = 'segments-overlay-checkpoint-area-outlines'
    const checkpointPointRadiusLayerId = 'segments-overlay-checkpoint-point-radii'
    const checkpointPointLayerId = 'segments-overlay-checkpoint-points'
    const checkpointLabelLayerId = 'segments-overlay-checkpoint-labels'

    const cleanup = () => {
      if (popupRef.current) {
        popupRef.current.remove()
        popupRef.current = null
      }
      try {
        if (map.getLayer(checkpointLabelLayerId)) map.removeLayer(checkpointLabelLayerId)
        if (map.getLayer(checkpointPointLayerId)) map.removeLayer(checkpointPointLayerId)
        if (map.getLayer(checkpointPointRadiusLayerId)) map.removeLayer(checkpointPointRadiusLayerId)
        if (map.getLayer(checkpointAreaOutlineLayerId)) map.removeLayer(checkpointAreaOutlineLayerId)
        if (map.getLayer(checkpointAreaLayerId)) map.removeLayer(checkpointAreaLayerId)
        if (map.getLayer(checkpointLineLayerId)) map.removeLayer(checkpointLineLayerId)
        if (map.getLayer(dotLayerId)) map.removeLayer(dotLayerId)
        if (map.getLayer(lineLayerId)) map.removeLayer(lineLayerId)
        if (map.getLayer(wintersportLayerId)) map.removeLayer(wintersportLayerId)
        if (map.getLayer(wassersportLayerId)) map.removeLayer(wassersportLayerId)
        if (map.getLayer(trailLayerId)) map.removeLayer(trailLayerId)
        if (map.getLayer(favoriteLayerId)) map.removeLayer(favoriteLayerId)
        if (map.getSource(checkpointSourceId)) map.removeSource(checkpointSourceId)
        if (map.getSource(sourceId)) map.removeSource(sourceId)
      } catch {
        // map may already be removed/unmounted
      }
    }

    if (!enabled || !shouldQuerySegmentsOverlay(mergedFilters) || !map.isStyleLoaded()) {
      cleanup()
      return
    }

    cleanup()

    // Filter anwenden
    const segments = segmentData.filter((seg: Segment) => {
      const courseKind = seg.course_kind ?? 'line'
      if (!mergedFilters.courseKinds.includes(courseKind)) return false
      const segType = seg.segment_type || 'standard'
      if (courseKind === 'line' && mergedFilters.terrainTypes.length > 0 && !mergedFilters.terrainTypes.includes(segType)) return false
      if (mergedFilters.riddenOnly && (seg.ridden_count ?? 0) === 0) return false
      if (mergedFilters.favoritesOnly && !seg.is_favorite) return false
      if (mergedFilters.sportGroups.length > 0 && !mergedFilters.sportGroups.includes(seg.sport_group)) return false
      return true
    })

    // Farbe je Segment ermitteln
    function resolveColor(seg: Segment): string {
      if (seg.course_kind === 'checkpoint_sequence') return colors.checkpoint
      const segType = seg.segment_type || 'standard'
      const isRidden = (seg.ridden_count ?? 0) > 0
      if (segType === 'lift') return colors.lift
      if (segType === 'trail') return colors.trail
      if (segType === 'wintersport') return colors.wintersport
      if (segType === 'wassersport') return colors.wassersport
      if (isRidden) return colors.ridden
      return colors.standard
    }

    function segmentTypeLabel(segType: string, courseKind?: string): string {
      if (courseKind === 'checkpoint_sequence') return t('mapEditor.segmentsOverlay.checkpointCourse')
      if (segType === 'trail') return t('mapEditor.segmentsOverlay.segmentTypes.trail')
      if (segType === 'lap') return t('mapEditor.segmentsOverlay.segmentTypes.lap')
      if (segType === 'lift') return t('mapEditor.segmentsOverlay.segmentTypes.lift')
      if (segType === 'wintersport') return t('mapEditor.segmentsOverlay.segmentTypes.wintersport')
      if (segType === 'wassersport') return t('mapEditor.segmentsOverlay.segmentTypes.wassersport')
      return t('mapEditor.segmentsOverlay.segmentTypes.standard')
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
      const isCheckpoint = seg.course_kind === 'checkpoint_sequence'
      acc.push({
        type: 'Feature' as const,
        geometry,
        properties: {
          id: seg.id,
          name: seg.name,
          course_kind: seg.course_kind ?? 'line',
          segment_type: seg.segment_type || 'standard',
          type_label: isCheckpoint ? t('mapEditor.segmentsOverlay.checkpointCourse') : undefined,
          sport_group: seg.sport_group,
          effort_count: seg.effort_count ?? 0,
          ridden_count: seg.ridden_count ?? 0,
          distance_m: seg.distance_m ?? 0,
          is_favorite: seg.is_favorite ?? false,
          color: resolveColor(seg),
          line_dash: isCheckpoint ? 'checkpoint' : 'normal',
          difficulty_color: (seg.segment_type_meta?.difficulty_color as string) ?? null,
        },
      })
      return acc
    }, [])

    // Dot features for segments without GeoJSON, using only the start point from segment_type_meta.
    const dotFeatures: GeoJSON.Feature[] = segments
      .filter(seg => !seg.track_geojson && seg.course_kind !== 'checkpoint_sequence')
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
    const checkpointFeatures = segments.flatMap((seg): GeoJSON.Feature[] => {
      if (seg.course_kind !== 'checkpoint_sequence') return []
      return (seg.control_map_features ?? []).flatMap((control): GeoJSON.Feature[] => {
        const label = control.role === 'start'
          ? t('mapEditor.segmentsOverlay.start')
          : control.role === 'finish'
            ? t('mapEditor.segmentsOverlay.finish')
            : String(control.sequence_index + 1)
        const baseProperties = {
          id: seg.id,
          name: seg.name,
          course_kind: 'checkpoint_sequence',
          type_label: t('mapEditor.segmentsOverlay.checkpointCourse'),
          effort_count: seg.effort_count ?? 0,
          ridden_count: seg.ridden_count ?? 0,
          visibility: seg.visibility,
          sequence_index: control.sequence_index,
          label,
          primitive: control.primitive,
          geometry_shape: control.geometry_shape,
          hit_radius_m: control.hit_radius_m ?? null,
          area_radius_m: control.area_radius_m ?? null,
          color: colors.checkpoint,
        }
        const features: GeoJSON.Feature[] = []
        if (control.geometry) {
          features.push({
            type: 'Feature',
            geometry: control.geometry as GeoJSON.Geometry,
            properties: { ...baseProperties, kind: 'control_area' },
          })
        }
        features.push({
          type: 'Feature',
          geometry: control.centroid as GeoJSON.Geometry,
          properties: { ...baseProperties, kind: 'control_point' },
        })
        return features
      })
    })
    if (allFeatures.length === 0 && checkpointFeatures.length === 0) return

    const geojson: GeoJSON.FeatureCollection = {
      type: 'FeatureCollection',
      features: allFeatures,
    }

    map.addSource(sourceId, { type: 'geojson', data: geojson })
    map.addSource(checkpointSourceId, {
      type: 'geojson',
      data: { type: 'FeatureCollection', features: checkpointFeatures },
    })

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

    // Standard line layer (everything except trails, winter sports, and water sports)
    // below the specialized types. `lift` is intentionally not excluded here:
    // there is no dedicated lift layer. resolveColor() returns colors.lift
    // (#6366F1 indigo), so the generic layer renders lifts once with the correct
    // color. This avoids double rendering; it is only an architectural asymmetry
    // compared with trail/winter-sport/water-sport layers. A dedicated lift layer
    // is possible but currently unnecessary because no lift-specific paint
    // properties are needed.
    map.addLayer({
      id: lineLayerId,
      type: 'line',
      source: sourceId,
      filter: ['all',
        ['==', ['geometry-type'], 'LineString'],
        ['!=', ['get', 'course_kind'], 'checkpoint_sequence'],
        ['!', ['in', ['get', 'segment_type'], ['literal', ['trail', 'wintersport', 'wassersport']]]],
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
        'line-dasharray': [
          'case',
          ['==', ['get', 'line_dash'], 'checkpoint'], ['literal', [1.5, 2.5]],
          ['literal', [1, 0]],
        ],
      },
      layout: {
        'line-cap': 'round',
        'line-join': 'round',
      },
    })

    map.addLayer({
      id: checkpointLineLayerId,
      type: 'line',
      source: sourceId,
      filter: ['all',
        ['==', ['geometry-type'], 'LineString'],
        ['==', ['get', 'course_kind'], 'checkpoint_sequence'],
      ],
      paint: {
        'line-color': colors.checkpoint,
        'line-width': 2,
        'line-opacity': 0.55,
        'line-dasharray': [1.5, 2.5],
      },
      layout: { 'line-cap': 'round', 'line-join': 'round' },
    })

    map.addLayer({
      id: checkpointAreaLayerId,
      type: 'fill',
      source: checkpointSourceId,
      filter: ['==', ['get', 'kind'], 'control_area'],
      paint: {
        'fill-color': colors.checkpoint,
        'fill-opacity': 0.16,
      },
    })

    map.addLayer({
      id: checkpointAreaOutlineLayerId,
      type: 'line',
      source: checkpointSourceId,
      filter: ['==', ['get', 'kind'], 'control_area'],
      paint: {
        'line-color': colors.checkpoint,
        'line-width': 1.5,
        'line-opacity': 0.8,
      },
    })

    map.addLayer({
      id: checkpointPointLayerId,
      type: 'circle',
      source: checkpointSourceId,
      filter: ['==', ['get', 'kind'], 'control_point'],
      paint: {
        'circle-radius': 9,
        'circle-color': '#ffffff',
        'circle-stroke-width': 2,
        'circle-stroke-color': colors.checkpoint,
      },
    })

    map.addLayer({
      id: checkpointLabelLayerId,
      type: 'symbol',
      source: checkpointSourceId,
      filter: ['==', ['get', 'kind'], 'control_point'],
      layout: {
        'text-field': ['get', 'label'],
        'text-size': 10,
        'text-allow-overlap': true,
      },
      paint: {
        'text-color': '#0f172a',
      },
    })

    // Trail layer: dashed lines above standard lines.
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

    // Winter-sport layer: dashed lines with difficulty color above trail lines.
    map.addLayer({
      id: wintersportLayerId,
      type: 'line',
      source: sourceId,
      filter: ['all',
        ['==', ['geometry-type'], 'LineString'],
        ['==', ['get', 'segment_type'], 'wintersport'],
      ],
      paint: {
        'line-color': [
          'case',
          ['==', ['get', 'difficulty_color'], 'blue'], '#148cf0',
          ['==', ['get', 'difficulty_color'], 'red'], '#dc2626',
          ['==', ['get', 'difficulty_color'], 'black'], '#111827',
          ['get', 'color'],
        ],
        'line-width': 3,
        'line-opacity': [
          'case',
          ['>', ['get', 'ridden_count'], 0], 0.9,
          0.7,
        ],
        'line-dasharray': [2, 3],
      },
      layout: { 'line-cap': 'round', 'line-join': 'round' },
    })

    // Wassersport-Layer: durchgezogene Cyan-Linien
    map.addLayer({
      id: wassersportLayerId,
      type: 'line',
      source: sourceId,
      filter: ['all',
        ['==', ['geometry-type'], 'LineString'],
        ['==', ['get', 'segment_type'], 'wassersport'],
      ],
      paint: {
        'line-color': colors.wassersport,
        'line-width': 3,
        'line-opacity': 0.8,
      },
      layout: { 'line-cap': 'round', 'line-join': 'round' },
    })

    // Point layer for segments without a track.
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

    // Wave 3, Fix 5: move route and waypoints back to the top of the stack so
    // this overlay does not cover them.
    enforceRouteLayersOnTop(map)

    // Hover tooltip.
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
      const typeLabel = segmentTypeLabel(segType, feature.properties?.course_kind)
      const effortLabel = effortCount > 0 ? t('mapEditor.segmentsOverlay.effortsRidden', { count: effortCount }) : ''
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

    const onMouseEnterCheckpoint = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      map.getCanvas().style.cursor = 'pointer'
      const feature = e.features?.[0]
      if (!feature) return
      const name = feature.properties?.name ?? ''
      const effortCount = feature.properties?.effort_count ?? 0
      const effortLabel = effortCount > 0 ? t('mapEditor.segmentsOverlay.effortsRidden', { count: effortCount }) : ''
      popup
        .setLngLat(e.lngLat)
        .setHTML(
          `<span style="font-size:12px;font-weight:500">${name}</span>` +
          `<span style="font-size:11px;color:#9ca3af;margin-left:6px">${t('mapEditor.segmentsOverlay.checkpointCourse')}${effortLabel}</span>`,
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
    map.on('mouseenter', wintersportLayerId, onMouseEnterLine)
    map.on('mouseleave', wintersportLayerId, onMouseLeave)
    map.on('click', wintersportLayerId, onClickLine)
    map.on('mouseenter', wassersportLayerId, onMouseEnterLine)
    map.on('mouseleave', wassersportLayerId, onMouseLeave)
    map.on('click', wassersportLayerId, onClickLine)
    map.on('mouseenter', favoriteLayerId, onMouseEnterLine)
    map.on('mouseleave', favoriteLayerId, onMouseLeave)
    map.on('click', favoriteLayerId, onClickLine)
    map.on('mouseenter', dotLayerId, onMouseEnterDot)
    map.on('mouseleave', dotLayerId, onMouseLeave)
    map.on('click', dotLayerId, onClickDot)
    map.on('mouseenter', checkpointPointLayerId, onMouseEnterCheckpoint)
    map.on('mouseleave', checkpointPointLayerId, onMouseLeave)
    map.on('click', checkpointPointLayerId, onClickDot)
    map.on('mouseenter', checkpointLineLayerId, onMouseEnterCheckpoint)
    map.on('mouseleave', checkpointLineLayerId, onMouseLeave)
    map.on('click', checkpointLineLayerId, onClickLine)

    return () => {
      map.off('mouseenter', lineLayerId, onMouseEnterLine)
      map.off('mouseleave', lineLayerId, onMouseLeave)
      map.off('click', lineLayerId, onClickLine)
      map.off('mouseenter', trailLayerId, onMouseEnterLine)
      map.off('mouseleave', trailLayerId, onMouseLeave)
      map.off('click', trailLayerId, onClickLine)
      map.off('mouseenter', wintersportLayerId, onMouseEnterLine)
      map.off('mouseleave', wintersportLayerId, onMouseLeave)
      map.off('click', wintersportLayerId, onClickLine)
      map.off('mouseenter', wassersportLayerId, onMouseEnterLine)
      map.off('mouseleave', wassersportLayerId, onMouseLeave)
      map.off('click', wassersportLayerId, onClickLine)
      map.off('mouseenter', favoriteLayerId, onMouseEnterLine)
      map.off('mouseleave', favoriteLayerId, onMouseLeave)
      map.off('click', favoriteLayerId, onClickLine)
      map.off('mouseenter', dotLayerId, onMouseEnterDot)
      map.off('mouseleave', dotLayerId, onMouseLeave)
      map.off('click', dotLayerId, onClickDot)
      map.off('mouseenter', checkpointPointLayerId, onMouseEnterCheckpoint)
      map.off('mouseleave', checkpointPointLayerId, onMouseLeave)
      map.off('click', checkpointPointLayerId, onClickDot)
      map.off('mouseenter', checkpointLineLayerId, onMouseEnterCheckpoint)
      map.off('mouseleave', checkpointLineLayerId, onMouseLeave)
      map.off('click', checkpointLineLayerId, onClickLine)
      cleanup()
    }
  }, [map, enabled, segmentData, navigate, styleVersion, filters, colorConfig, t]) // eslint-disable-line react-hooks/exhaustive-deps

  return null
}
