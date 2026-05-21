// frontend/src/components/SegmentEditorModal.tsx
// Task 8.4e: Save-Mutation + Lap-Mode + Enrichment-Wiring (complete)
// Polish batch: A1-A4 (map/track), B1-B4 (top-bar), D1-D2 (bottom-bar chart)
import { createPortal } from 'react-dom'
import { useCallback, useEffect, useMemo, useRef, useState, useId } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import maplibregl, { type RequestParameters } from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
// @ts-ignore — maplibre-gl-draw has no bundled TS types for default import
import MapboxDraw from 'maplibre-gl-draw'
import { X } from 'lucide-react'
import { SegmentSidebar, type SegmentMetadata } from './segment-editor/SegmentSidebar'
import { TrackModeBottomBar, type TrackPoint } from './segment-editor/TrackModeBottomBar'
import { DrawModeBottomBar } from './segment-editor/DrawModeBottomBar'
import { LapModeBottomBar } from './segment-editor/LapModeBottomBar'
import { CheckpointModeBottomBar } from './segment-editor/CheckpointModeBottomBar'
import { CheckpointControlsList } from './segment-editor/CheckpointControlsList'
import { BIDIRECTIONAL_DEFAULT, deriveTrailDifficultyColor, type SegType } from './segment-editor/constants'
import { cn } from '@/lib/utils'
import { haversineM } from '@/lib/geoUtils'
import { detectLapCrossings } from './segment-editor/TrackMode'
import {
  addHeatmapLayers,
  removeHeatmapLayers,
  buildHeatmapTileUrl,
  loadHeatmapColor,
} from '@/lib/mapHeatmap'
import { buildLayerStyle } from '@/lib/mapStyles'
import { useMapLayers } from '@/hooks/useMapLayers'
import { useCapabilities } from '@/hooks/useCapabilities'
import { featureAvailable } from '@/lib/deploymentCapabilities'
import { apiClient } from '@/api/client'
import { getToken } from '@/lib/auth'
import { createCheckpointCourse, createSegment, suggestSegmentEnrichment, type CheckpointControl } from '@/api/segments'
import { useToast } from '@/lib/toast'
import { SPORT_DEFS } from '@/lib/sportTaxonomy'
import { buildNewSegmentSportPatch } from './segment-editor/segmentSave'

export interface SegmentEditorModalProps {
  mode: 'track' | 'draw' | 'lap' | 'checkpoint'
  trackPoints?: TrackPoint[]
  sourceActivityId?: string
  onSuccess: (segmentId?: string) => void
  onClose: () => void
}

// ── Arrow+Track layer helper (A2) ─────────────────────────────────────────────
// Adds direction-chevron symbols on the activity-track source.
// Called after the source is added (track-mode init) and after style restores.
function addArrowLayer(m: maplibregl.Map) {
  if (m.getLayer('activity-track-arrows')) return
  const arrowSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path d="M8 0 L16 8 L8 16 L8 10 L0 10 L0 6 L8 6 Z" fill="#fff" stroke="#333" stroke-width="1"/></svg>`
  const img = new Image(16, 16)
  img.onload = () => {
    if (!m.hasImage('arrow')) m.addImage('arrow', img)
    if (m.getLayer('activity-track-arrows')) return
    m.addLayer({
      id: 'activity-track-arrows',
      type: 'symbol',
      source: 'activity-track',
      layout: {
        'symbol-placement': 'line',
        'symbol-spacing': 100,
        'icon-image': 'arrow',
        'icon-size': 0.7,
        'icon-rotation-alignment': 'auto',
        'icon-allow-overlap': true,
      },
    })
  }
  img.onerror = () => { /* fallback: skip arrow layer */ }
  img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(arrowSvg)
}

function checkpointPointRadiusGeometry(coord: [number, number], radiusM: number): GeoJSON.Polygon {
  const steps = 48
  const latRad = coord[1] * Math.PI / 180
  const latDelta = radiusM / 111_320
  const lngDelta = radiusM / (111_320 * Math.max(Math.cos(latRad), 0.01))
  const ring: [number, number][] = []
  for (let i = 0; i <= steps; i++) {
    const angle = (i / steps) * Math.PI * 2
    ring.push([
      coord[0] + Math.cos(angle) * lngDelta,
      coord[1] + Math.sin(angle) * latDelta,
    ])
  }
  return { type: 'Polygon', coordinates: [ring] }
}

export default function SegmentEditorModal(props: SegmentEditorModalProps) {
  const { mode, trackPoints = [], sourceActivityId, onSuccess, onClose } = props
  const navigate = useNavigate()
  const { addToast } = useToast()

  // ── All sport types for the sidebar selector ──────────────────────────────
  const allSportTypes = useMemo(
    () => SPORT_DEFS.map(s => ({ key: s.sportType, label: s.label, category: s.categoryKey })),
    [],
  )

  // ── Metadata state ────────────────────────────────────────────────────────
  const [meta, setMeta] = useState<SegmentMetadata>(() => ({
    name: '',
    segType: (mode === 'lap' ? 'lap' : 'standard') as SegType,
    sportTypes: null,
    sacScale: null,
    mtbScale: null,
    surface: null,
    difficultyColor: null,
    bidirectional: BIDIRECTIONAL_DEFAULT[(mode === 'lap' ? 'lap' : 'standard') as SegType],
    userEditedBidirectional: false,
    isPublic: !(mode === 'checkpoint' && !!sourceActivityId),
  }))

  const patchMeta = useCallback((patch: Partial<SegmentMetadata>) => {
    setMeta(prev => {
      const next = { ...prev, ...patch }
      if (next.segType === 'trail') {
        next.difficultyColor = deriveTrailDifficultyColor(next.sacScale, next.mtbScale)
      }
      return next
    })
  }, [])

  // ── User-ID (for heatmap tile auth) ──────────────────────────────────────
  const { data: meData } = useQuery({
    queryKey: ['me'],
    queryFn: () => apiClient<{ user_id: string }>('/me'),
    staleTime: Infinity,
  })
  const currentUserId = meData?.user_id ?? ''

  // ── Initial-meta snapshot (for dirty-tracking of all meta fields) ─────────
  // Captured once when the component mounts (meta starts as all-empty defaults).
  // Any subsequent user change produces JSON inequality → isDirty fires.
  const initialMetaRef = useRef<SegmentMetadata | null>(null)
  useEffect(() => {
    if (initialMetaRef.current === null) {
      initialMetaRef.current = meta
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ── Map state ─────────────────────────────────────────────────────────────
  const mapContainerRef = useRef<HTMLDivElement | null>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const trackPointsRef = useRef(trackPoints)
  useEffect(() => { trackPointsRef.current = trackPoints }, [trackPoints])

  const { base } = useMapLayers()
  const { data: caps } = useCapabilities()
  const segmentEnrichmentSuggestUnavailable = !!caps && !featureAvailable(caps, 'segment_enrichment_suggest')
  const [mapStyle, setMapStyle] = useState<string>('osm')

  // Sync styleKey to DB reality: if the persisted key is no longer in base
  // (admin disabled it), fall back to base[0].
  useEffect(() => {
    if (base.length > 0 && !base.find((l) => l.key === mapStyle)) {
      setMapStyle(base[0].key)
    }
  }, [base, mapStyle])

  const selectedBaseLayer = base.find((l) => l.key === mapStyle) ?? base[0]
  const [heatmapOn, setHeatmapOn] = useState(false)

  // ── styleVersion: bumped by persistent style.load handler (A4) ───────────
  // Downstream effects that add custom layers depend on this so they re-run
  // after every setStyle() wipes the custom layer slate clean.
  const [styleVersion, setStyleVersion] = useState(0)

  // ── Camera-restore ref (A4) ────────────────────────────────────────────────
  // Populated just before setStyle(); the style.load handler reads + clears it.
  const pendingCameraRestoreRef = useRef<{ center: maplibregl.LngLat; zoom: number } | null>(null)

  // ── Draw-mode state ───────────────────────────────────────────────────────
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const drawRef = useRef<any>(null)
  const [drawnCoords, setDrawnCoords] = useState<Array<[number, number]>>([])

  // ── Track-mode state ──────────────────────────────────────────────────────
  const [editedTrackPoints, setEditedTrackPoints] = useState(trackPoints)
  useEffect(() => { setEditedTrackPoints(trackPoints) }, [trackPoints])

  // ── Hover-index: driven by TrackModeBottomBar chart hover (Task 2) ────────
  // Null = no hover; a number = index into editedTrackPoints.
  // The MapLibre hover-point layer reads this to place the orange circle marker.
  const [hoverIndex, setHoverIndex] = useState<number | null>(null)

  const initialRange = useMemo<[number, number]>(
    () => [0, Math.max(0, trackPoints.length - 1)],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [trackPoints.length],
  )
  const [range, setRange] = useState<[number, number]>(initialRange)
  // hasUserSelection: false = no orange sub-segment layer rendered (e.g. after Reset).
  // Becomes true on first map click-to-place or drag/brush interaction.
  const [hasUserSelection, setHasUserSelection] = useState(false)
  // (Vorschau-Phase entfernt — user-feedback: "macht nichts außer zu locken".
  // Save-Button saved direkt. Kein Intermediate-State mehr.)

  // ── Discard-confirmation dialog (replaces native window.confirm — Fix #13) ──
  const [discardDialogOpen, setDiscardDialogOpen] = useState(false)
  const discardDialogId = useId()

  // ── Lap-mode state ────────────────────────────────────────────────────────
  const [lapCrossings, setLapCrossings] = useState<number[]>([])
  const detectedLaps = Math.max(0, lapCrossings.length - 1)
  // Tracks how many lap-seg-{i} sources were added in the previous render so
  // cleanup iterates exactly that many (avoids the magic-number-20 upper bound).
  const prevLapSegCountRef = useRef(0)

  // Lap detection: runs when map loads in lap mode (click-to-place finish line)
  // is handled via the map click handler below.
  const [checkpointControls, setCheckpointControls] = useState<CheckpointControl[]>([])

  // ── Enrichment state ─────────────────────────────────────────────────────
  const [enrichmentSources, setEnrichmentSources] = useState<
    Partial<Record<'surface' | 'sac' | 'mtb', { coverage: number } | null>>
  >({})

  // Reset enrichment badges on range change
  useEffect(() => { setEnrichmentSources({}) }, [range])

  // ── Enrichment mutation ───────────────────────────────────────────────────
  const enrichmentMutation = useMutation({
    mutationFn: suggestSegmentEnrichment,
    onSuccess: (resp) => {
      if (resp.reason === 'wintersport' || resp.reason === 'wassersport') return
      if (resp.reason === 'low_coverage') {
        addToast({ type: 'warning', title: 'Keine OSM-Daten für diesen Abschnitt' })
        return
      }
      const patch: Partial<SegmentMetadata> = {}
      const src: typeof enrichmentSources = {}
      if (resp.surface) {
        patch.surface = resp.surface.mixed ? 'Wechselnd' : (resp.surface.dominant ?? null)
        src.surface = { coverage: resp.coverage_pct }
      }
      if (resp.sac_scale) {
        patch.sacScale = resp.sac_scale.dominant ?? null
        src.sac = { coverage: resp.coverage_pct }
      }
      if (resp.mtb_scale) {
        patch.mtbScale = resp.mtb_scale.dominant ?? null
        src.mtb = { coverage: resp.coverage_pct }
      }
      patchMeta(patch)
      setEnrichmentSources(src)
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Abrufen der OSM-Daten', message: 'Erneut versuchen' })
    },
  })

  const handleEnrichment = useCallback(() => {
    const coords: Array<[number, number]> =
      mode === 'track'
        ? editedTrackPoints.slice(range[0], range[1] + 1).map(p => [p.lng, p.lat])
        : mode === 'draw'
          ? drawnCoords
          : [] // lap-mode: enrichment disabled
    if (coords.length < 2) return
    enrichmentMutation.mutate({ coordinates: coords, seg_type: meta.segType })
  }, [mode, editedTrackPoints, range, drawnCoords, meta.segType, enrichmentMutation])

  const enrichmentDisabledReason = useMemo(() => {
    if (segmentEnrichmentSuggestUnavailable) return 'Nicht verfügbar in dieser Installation'
    if (meta.segType === 'wintersport') return 'Nicht verfügbar — Oberfläche implizit Schnee/Eis'
    if (mode === 'track' && range[0] === range[1]) return 'Keine Auswahl'
    if (mode === 'draw' && drawnCoords.length < 2) return 'Weniger als 2 Punkte'
    if (mode === 'lap') return 'Im Rundkurs-Modus nicht verfügbar'
    if (mode === 'checkpoint') return 'Im Checkpoint-Modus nicht verfügbar'
    return undefined
  }, [segmentEnrichmentSuggestUnavailable, meta.segType, mode, range, drawnCoords.length])

  // ── Enrichment status text (screen-reader aria-live) ─────────────────────
  // Derived from enrichmentMutation state so SegmentSidebar's sr-only live
  // region announces both loading state AND the result (success / errors).
  const enrichmentStatus = useMemo(() => {
    if (enrichmentMutation.isPending) return 'Lade OSM-Daten...'
    if (enrichmentMutation.isError) return 'Fehler beim Abrufen der OSM-Daten'
    const data = enrichmentMutation.data
    if (!data) return ''
    if (data.reason === 'low_coverage') return 'Keine OSM-Daten für diesen Abschnitt'
    if (data.reason === 'no_graph') return 'OSM-Graph nicht verfügbar'
    if (data.reason === 'no_data') return 'Keine OSM-Daten gefunden'
    if (data.reason === 'wintersport') return '' // intentional no-op for winter
    const parts: string[] = []
    if (data.surface?.dominant) parts.push(`Oberfläche ${data.surface.dominant}`)
    if (data.sac_scale?.dominant) parts.push(`SAC ${data.sac_scale.dominant}`)
    if (data.mtb_scale?.dominant) parts.push(`MTB ${data.mtb_scale.dominant}`)
    const pct = Math.round((data.coverage_pct ?? 0) * 100)
    return `OSM-Daten geladen: ${parts.join(', ')}${parts.length > 0 ? ', ' : ''}${pct}% Coverage`
  }, [enrichmentMutation.isPending, enrichmentMutation.isError, enrichmentMutation.data])

  // ── Save mutation ─────────────────────────────────────────────────────────
  const saveMutation = useMutation({
    mutationFn: async () => {
      if (mode === 'checkpoint') {
        const { userEditedBidirectional, ...savePayload } = meta
        void userEditedBidirectional
        const sportPatch = buildNewSegmentSportPatch('standard', savePayload.sportTypes)
        return createCheckpointCourse({
          name: savePayload.name,
          sport_group: sportPatch.sport_group,
          allowed_predicates: sportPatch.allowed_predicates,
          visibility: savePayload.isPublic ? 'public' : 'private',
          controls: checkpointControls,
          source_activity_id: sourceActivityId,
        })
      }
      // Build coordinates array (note: editedTrackPoints includes reverse)
      const coords =
        mode === 'track'
          ? editedTrackPoints.slice(range[0], range[1] + 1).map(p => [p.lng, p.lat])
          : mode === 'draw'
            ? drawnCoords
            // Lap-mode: only the single lap between the first two crossings.
            // lapCrossings[0..1] are point-indices into trackPoints; save is
            // disabled (LapModeBottomBar) when lapCrossings.length < 2.
            : (mode === 'lap' && lapCrossings.length >= 2
                ? trackPoints.slice(lapCrossings[0], lapCrossings[1] + 1).map(p => [p.lng, p.lat])
                : trackPoints.map(p => [p.lng, p.lat])) // fallback: full track (should not reach)

      const geojson_track = JSON.stringify({
        type: 'LineString',
        coordinates: coords,
      })

      // Strip pure-UI flag before send
      const { userEditedBidirectional, ...savePayload } = meta
      void userEditedBidirectional
      const sportPatch = buildNewSegmentSportPatch(savePayload.segType, savePayload.sportTypes)

      return createSegment({
        name: savePayload.name,
        sport_group: sportPatch.sport_group,
        segment_type: savePayload.segType,
        allowed_predicates: sportPatch.allowed_predicates,
        bidirectional: savePayload.bidirectional,
        segment_type_meta: {
          surface: savePayload.surface,
          sac_scale: savePayload.sacScale,
          mtb_scale: savePayload.mtbScale,
          difficulty_color: savePayload.difficultyColor,
        },
        visibility: savePayload.isPublic ? 'public' : 'private',
        climb_type: savePayload.climbType ?? 'standard',
        geojson_track,
        source_activity_id: sourceActivityId,
      })
    },
    onSuccess: (created) => {
      addToast({
        type: 'success',
        title: 'Segment gespeichert',
        message: created.name,
        duration: 8000,
        action: {
          label: 'Anzeigen',
          onClick: () => navigate(`/segments/${created.id}`),
        },
      })
      onSuccess(created.id)
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Speichern fehlgeschlagen', message: String(err) })
    },
  })

  // ── Map Init useEffect ────────────────────────────────────────────────────
  // Fires ONCE. Map style changes are handled by a separate effect (mapStyle dep)
  // via setStyle(). A persistent style.load handler restores camera + bumps
  // styleVersion so custom layers are rebuilt without full map recreation.
  useEffect(() => {
    const container = mapContainerRef.current
    if (!container || mapRef.current) return

    const pts = trackPointsRef.current
    const initialCenter: [number, number] =
      pts && pts.length > 0 ? [pts[0].lng, pts[0].lat] : [11.0, 48.0]

    const initLayer = base.find((l) => l.key === mapStyle) ?? base[0]
    const initStyle = initLayer ? buildLayerStyle(initLayer, caps ?? {}) : null
    if (!initStyle) return

    const map = new maplibregl.Map({
      container,
      style: initStyle as import('maplibre-gl').StyleSpecification | string,
      center: initialCenter,
      zoom: 12,
      transformRequest: (url: string): RequestParameters => {
        if (url.includes('/api/v1/tiles/')) {
          const token = getToken()
          if (token) return { url, headers: { Authorization: `Bearer ${token}` } }
        }
        return { url }
      },
    })
    mapRef.current = map

    // Persistent style.load handler (A4): fires on every setStyle() call.
    // Restores camera position saved before the style switch, then bumps
    // styleVersion so downstream effects rebuild the custom overlay layers.
    map.on('style.load', () => {
      if (pendingCameraRestoreRef.current) {
        map.jumpTo(pendingCameraRestoreRef.current)
        pendingCameraRestoreRef.current = null
      }
      map.resize()
      setStyleVersion(v => v + 1)
    })

    map.on('load', () => {
      map.getCanvas().style.cursor = 'crosshair'

      // B3: resize first so fitBounds works correctly after layout is settled
      map.resize()

      // Fit bounds to track
      const current = trackPointsRef.current
      if (current && current.length > 1) {
        const lngs = current.map(p => p.lng)
        const lats = current.map(p => p.lat)
        map.fitBounds(
          [[Math.min(...lngs), Math.min(...lats)], [Math.max(...lngs), Math.max(...lats)]],
          { padding: 40 },
        )
      }

      // Track-mode: draw activity line (A1: grey #6b7280, width 4)
      if (mode === 'track' && current && current.length >= 2) {
        map.addSource('activity-track', {
          type: 'geojson',
          data: {
            type: 'Feature',
            geometry: { type: 'LineString', coordinates: current.map(p => [p.lng, p.lat]) },
            properties: {},
          },
        })
        map.addLayer({
          id: 'activity-track-line',
          type: 'line',
          source: 'activity-track',
          layout: { 'line-join': 'round', 'line-cap': 'round' },
          paint: { 'line-color': '#6b7280', 'line-width': 4, 'line-opacity': 0.9 },
        })
        // A2: direction chevrons on track line (track-mode only)
        addArrowLayer(map)
      }

      // Draw-mode: attach MapboxDraw
      if (mode === 'draw') {
        const draw = new MapboxDraw({
          displayControlsDefault: false,
          controls: { line_string: false, trash: false },
          defaultMode: 'draw_line_string',
          // Custom styles: thick grey line matching the activity-track-line (A1).
          // inactive = finalized line (simple_select), active = line being drawn.
          styles: [
            {
              id: 'gl-draw-line-inactive',
              type: 'line',
              filter: ['all', ['==', 'active', 'false'], ['==', '$type', 'LineString'], ['!=', 'mode', 'static']],
              layout: { 'line-cap': 'round', 'line-join': 'round' },
              paint: { 'line-color': '#6b7280', 'line-width': 4, 'line-opacity': 0.9 },
            },
            {
              id: 'gl-draw-line-active',
              type: 'line',
              filter: ['all', ['==', 'active', 'true'], ['==', '$type', 'LineString']],
              layout: { 'line-cap': 'round', 'line-join': 'round' },
              paint: { 'line-color': '#6b7280', 'line-width': 3, 'line-dasharray': [2, 2], 'line-opacity': 0.9 },
            },
            {
              id: 'gl-draw-point-active',
              type: 'circle',
              filter: ['all', ['==', '$type', 'Point'], ['==', 'meta', 'vertex']],
              paint: {
                'circle-radius': 5,
                'circle-color': '#ffffff',
                'circle-stroke-width': 2,
                'circle-stroke-color': '#6b7280',
              },
            },
          ],
        })
        map.addControl(draw as unknown as maplibregl.IControl)
        drawRef.current = draw

        const refreshCoords = () => {
          const all = draw.getAll()
          const feat = all.features.find(
            (f: GeoJSON.Feature) => f.geometry.type === 'LineString'
          )
          if (feat && feat.geometry.type === 'LineString') {
            setDrawnCoords(feat.geometry.coordinates as Array<[number, number]>)
          } else {
            setDrawnCoords([])
          }
        }
        map.on('draw.create', refreshCoords)
        map.on('draw.update', refreshCoords)
        map.on('draw.delete', refreshCoords)
      }
    })

    return () => {
      if (drawRef.current) {
        try { map.removeControl(drawRef.current as unknown as maplibregl.IControl) } catch { /* ignore */ }
        drawRef.current = null
      }
      map.remove()
      mapRef.current = null
    }
    // base[0]?.key triggers init once layers arrive; mapRef.current guard prevents re-init.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [base[0]?.key])

  // ── Map style switch (A4) ──────────────────────────────────────────────────
  // When mapStyle changes, save camera position then call setStyle().
  // The persistent style.load handler (registered in map init) restores camera
  // and bumps styleVersion, which triggers the layer-rebuild effect below.
  const prevMapStyleRef = useRef<string>('osm')
  useEffect(() => {
    const map = mapRef.current
    if (!map || prevMapStyleRef.current === mapStyle || !selectedBaseLayer) return
    prevMapStyleRef.current = mapStyle
    pendingCameraRestoreRef.current = { center: map.getCenter(), zoom: map.getZoom() }
    map.setStyle(buildLayerStyle(selectedBaseLayer, caps ?? {}) as import('maplibre-gl').StyleSpecification | string)
  }, [mapStyle, selectedBaseLayer, caps])

  // ── Layer rebuild after style switch (A4) ─────────────────────────────────
  // setStyle() wipes all custom sources + layers. styleVersion bumps whenever
  // style.load fires (initial load + every setStyle()), causing this effect to
  // re-add every custom layer that should be present.
  // styleVersion === 0 means the initial load has not yet fired — skip to avoid
  // the race where map.isStyleLoaded() returns false.
  useEffect(() => {
    if (styleVersion === 0) return
    const map = mapRef.current
    if (!map) return

    // Re-add activity-track source+layer (track-mode only; A1 + A2)
    if (mode === 'track') {
      const current = trackPointsRef.current
      if (current && current.length >= 2 && !map.getSource('activity-track')) {
        map.addSource('activity-track', {
          type: 'geojson',
          data: {
            type: 'Feature',
            geometry: { type: 'LineString', coordinates: current.map(p => [p.lng, p.lat]) },
            properties: {},
          },
        })
        map.addLayer({
          id: 'activity-track-line',
          type: 'line',
          source: 'activity-track',
          layout: { 'line-join': 'round', 'line-cap': 'round' },
          paint: { 'line-color': '#6b7280', 'line-width': 4, 'line-opacity': 0.9 },
        })
        addArrowLayer(map)
      }
    }

    // Re-add heatmap layers if active
    if (heatmapOn && currentUserId) {
      addHeatmapLayers(map, buildHeatmapTileUrl(currentUserId), loadHeatmapColor())
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [styleVersion]) // only styleVersion — other state is read via refs / stable closures

  // ── Heatmap toggle useEffect ──────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    if (heatmapOn && currentUserId) {
      addHeatmapLayers(map, buildHeatmapTileUrl(currentUserId), loadHeatmapColor())
    } else {
      removeHeatmapLayers(map)
    }
  }, [heatmapOn, currentUserId])

  // ── Track-mode: segment preview layer ────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || mode !== 'track') return
    if (!map.isStyleLoaded()) return
    // No orange sub-segment when hasUserSelection is false (e.g. after Reset).
    if (!hasUserSelection) {
      const src = map.getSource('segment-preview') as maplibregl.GeoJSONSource | undefined
      if (src) {
        src.setData({ type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: [] } })
      }
      return
    }
    const subset = editedTrackPoints.slice(range[0], range[1] + 1)
    if (subset.length < 2) return
    const coords = subset.map(p => [p.lng, p.lat])
    const geom: GeoJSON.Feature = {
      type: 'Feature',
      properties: {},
      geometry: { type: 'LineString', coordinates: coords },
    }
    const src = map.getSource('segment-preview') as maplibregl.GeoJSONSource | undefined
    if (src) {
      src.setData(geom)
    } else {
      map.addSource('segment-preview', { type: 'geojson', data: geom })
      map.addLayer({
        id: 'segment-preview-line',
        type: 'line',
        source: 'segment-preview',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        // A3: reduced sub-segment line-width from 5 → 4; preserve orange + opacity
        paint: { 'line-color': '#F97316', 'line-width': 4, 'line-opacity': 1.0 },
      })
    }
  }, [mode, range, editedTrackPoints, hasUserSelection, styleVersion])

  // ── Track-mode: hover-point marker (Task 2) ──────────────────────────────
  // Syncs a MapLibre circle marker with the chart's hover position.
  // Source id: 'hover-point'  Layer id: 'hover-point-marker'
  // Only active when mode === 'track'; hidden when hoverIndex is null.
  useEffect(() => {
    const map = mapRef.current
    if (!map || mode !== 'track') return
    if (!map.isStyleLoaded()) return

    const emptyGeoJSON: GeoJSON.Feature = {
      type: 'Feature',
      properties: {},
      geometry: { type: 'Point', coordinates: [] },
    }

    const src = map.getSource('hover-point') as maplibregl.GeoJSONSource | undefined
    if (hoverIndex !== null && hoverIndex >= 0 && hoverIndex < editedTrackPoints.length) {
      const pt = editedTrackPoints[hoverIndex]
      const pointFeature: GeoJSON.Feature = {
        type: 'Feature',
        properties: {},
        geometry: { type: 'Point', coordinates: [pt.lng, pt.lat] },
      }
      if (src) {
        src.setData(pointFeature)
      } else {
        map.addSource('hover-point', { type: 'geojson', data: pointFeature })
        map.addLayer({
          id: 'hover-point-marker',
          type: 'circle',
          source: 'hover-point',
          paint: {
            'circle-radius': 8,
            'circle-color': '#F97316',
            'circle-stroke-color': '#ffffff',
            'circle-stroke-width': 2,
            'circle-opacity': 0.95,
          },
        })
      }
    } else {
      // No hover: hide the marker by emptying the source data (keeps layer registered)
      if (src) {
        src.setData(emptyGeoJSON)
      }
      // Source does not exist yet — nothing to do; it will be created on first hover
    }
  }, [mode, hoverIndex, editedTrackPoints, styleVersion])

  // ── Lap-mode: visualize detected crossings ────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || mode !== 'lap' || lapCrossings.length < 2) return
    if (!map.isStyleLoaded()) return

    // Remove old lap layers (only up to the count added in the previous render).
    const existingLayers = map.getStyle()?.layers ?? []
    existingLayers.forEach(l => {
      if (l.id.startsWith('lap-segment-')) map.removeLayer(l.id)
    })
    // prevLapSegCountRef tracks how many lap-seg-{i} sources were registered
    // previously (indices 1..prevCount), avoiding an arbitrary upper-bound magic number.
    for (let i = 1; i <= prevLapSegCountRef.current; i++) {
      if (map.getSource(`lap-seg-${i}`)) map.removeSource(`lap-seg-${i}`)
    }

    // Add colored lap segments
    const colors = ['#f97316', '#f59e0b']
    for (let i = 1; i < lapCrossings.length; i++) {
      const start = lapCrossings[i - 1]
      const end = lapCrossings[i]
      const coords = trackPoints.slice(start, end + 1).map(p => [p.lng, p.lat])
      const srcId = `lap-seg-${i}`
      map.addSource(srcId, {
        type: 'geojson',
        data: {
          type: 'Feature',
          properties: {},
          geometry: { type: 'LineString', coordinates: coords },
        } as GeoJSON.Feature,
      })
      map.addLayer({
        id: `lap-segment-${i}`,
        type: 'line',
        source: srcId,
        paint: {
          'line-color': colors[(i - 1) % colors.length],
          'line-width': 4,
          'line-opacity': 0.9,
        },
      })
    }
    // Record how many sources were added so cleanup is exact on the next render.
    prevLapSegCountRef.current = lapCrossings.length - 1
  }, [mode, lapCrossings, trackPoints])

  // ── Click-to-place: lat-corrected nearest-point (track) ──────────────────
  useEffect(() => {
    if (mode !== 'track') return
    const map = mapRef.current
    if (!map) return
    const handler = (e: maplibregl.MapMouseEvent) => {
      const { lng, lat } = e.lngLat
      const coslat = Math.cos(lat * Math.PI / 180) ** 2
      let bestIdx = 0
      let bestDist = Infinity
      for (let i = 0; i < editedTrackPoints.length; i++) {
        const dLat = editedTrackPoints[i].lat - lat
        const dLng = editedTrackPoints[i].lng - lng
        const d = dLat * dLat + coslat * dLng * dLng
        if (d < bestDist) { bestDist = d; bestIdx = i }
      }
      // Guard: don't hijack clicks that are far from the track (e.g. map panning).
      // Compute true metric distance to the best candidate point.
      const MAX_CLICK_DISTANCE_M = 200
      const bestPointDistM = haversineM(editedTrackPoints[bestIdx].lat, editedTrackPoints[bestIdx].lng, lat, lng)
      if (bestPointDistM > MAX_CLICK_DISTANCE_M) return
      setHasUserSelection(true)
      setRange(prev => {
        const distStart = Math.abs(bestIdx - prev[0])
        const distEnd = Math.abs(bestIdx - prev[1])
        // Tie-breaker: bei gleicher Distanz wird Start bewegt (Spec §4.3 Edge-Cases).
        if (distStart <= distEnd) return [Math.min(bestIdx, prev[1] - 1), prev[1]]
        return [prev[0], Math.max(bestIdx, prev[0] + 1)]
      })
    }
    map.on('click', handler)
    return () => { map.off('click', handler) }
  }, [mode, editedTrackPoints])

  // ── Lap-mode: click to place finish line ─────────────────────────────────
  useEffect(() => {
    if (mode !== 'lap' || lapCrossings.length > 0) return
    const map = mapRef.current
    if (!map) return
    const handler = (e: maplibregl.MapMouseEvent) => {
      // Find nearest track point to click
      const { lng, lat } = e.lngLat
      let bestIdx = 0
      let bestDist = Infinity
      for (let i = 0; i < trackPoints.length; i++) {
        const d = haversineM(trackPoints[i].lat, trackPoints[i].lng, lat, lng)
        if (d < bestDist) { bestDist = d; bestIdx = i }
      }
      const crossings = detectLapCrossings(trackPoints, bestIdx)
      setLapCrossings(crossings)
    }
    map.on('click', handler)
    return () => { map.off('click', handler) }
  }, [mode, lapCrossings.length, trackPoints])

  useEffect(() => {
    if (mode !== 'checkpoint') return
    const map = mapRef.current
    if (!map) return
    const handler = (e: maplibregl.MapMouseEvent) => {
      setCheckpointControls(prev => {
        const nextIndex = prev.length
        const role: 'start' | 'finish' = nextIndex === 0 ? 'start' : 'finish'
        const controls: CheckpointControl[] = prev.map((control, idx) => ({
          ...control,
          role: idx === 0 ? 'start' as const : 'checkpoint' as const,
          sequence_index: idx,
        }))
        controls.push({
          sequence_index: nextIndex,
          role,
          primitive: 'point',
          geometry_shape: 'point',
          geometry: { type: 'Point', coordinates: [e.lngLat.lng, e.lngLat.lat] },
          hit_radius_m: 30,
          required: true,
        })
        return controls.map((control, idx, all) => ({
          ...control,
          sequence_index: idx,
          role: idx === 0 ? 'start' : idx === all.length - 1 ? 'finish' : 'checkpoint',
        }))
      })
    }
    map.on('click', handler)
    return () => { map.off('click', handler) }
  }, [mode])

  // ── Track-mode: commit range → fitBounds ─────────────────────────────────
  const commitRangeToMap = useCallback((r: [number, number]) => {
    const map = mapRef.current
    if (!map || mode !== 'track') return
    if (editedTrackPoints.length === 0) return
    const subset = editedTrackPoints.slice(r[0], r[1] + 1)
    const bounds = new maplibregl.LngLatBounds()
    subset.forEach(p => bounds.extend([p.lng, p.lat]))
    map.fitBounds(bounds, { padding: 40, duration: 300 })
  }, [mode, editedTrackPoints])

  useEffect(() => {
    if (mode !== 'checkpoint') return
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    const sourceID = 'checkpoint-controls'
    const lineID = 'checkpoint-display-line'
    const radiusID = 'checkpoint-control-radii'
    const circleID = 'checkpoint-control-circles'
    const labelID = 'checkpoint-control-labels'
    const coords = checkpointControls
      .map(control => {
        const geometry = control.geometry as { type?: string; coordinates?: unknown }
        return geometry.type === 'Point' && Array.isArray(geometry.coordinates) ? geometry.coordinates as [number, number] : null
      })
      .filter((coord): coord is [number, number] => Array.isArray(coord) && coord.length >= 2)
    const data = {
      type: 'FeatureCollection' as const,
      features: [
        ...(coords.length >= 2 ? [{
          type: 'Feature' as const,
          properties: { kind: 'connector' },
          geometry: { type: 'LineString' as const, coordinates: coords },
        }] : []),
        ...checkpointControls.flatMap((control, idx) => {
          const coord = coords[idx]
          if (!coord || control.primitive !== 'point') return []
          return [{
            type: 'Feature' as const,
            properties: { kind: 'radius' },
            geometry: checkpointPointRadiusGeometry(coord, control.hit_radius_m ?? 30),
          }]
        }),
        ...coords.map((coord, idx) => ({
          type: 'Feature' as const,
          properties: {
            kind: 'control',
            label: idx === 0 ? 'Start' : idx === coords.length - 1 ? 'Ziel' : String(idx + 1),
          },
          geometry: { type: 'Point' as const, coordinates: coord },
        })),
      ],
    }
    if (!map.getSource(sourceID)) {
      map.addSource(sourceID, { type: 'geojson', data })
    } else {
      ;(map.getSource(sourceID) as maplibregl.GeoJSONSource).setData(data)
    }
    if (!map.getLayer(lineID)) {
      map.addLayer({
        id: lineID,
        type: 'line',
        source: sourceID,
        filter: ['==', ['get', 'kind'], 'connector'],
        paint: { 'line-color': '#525252', 'line-width': 2, 'line-dasharray': [2, 2], 'line-opacity': 0.65 },
      })
    }
    if (!map.getLayer(radiusID)) {
      map.addLayer({
        id: radiusID,
        type: 'fill',
        source: sourceID,
        filter: ['==', ['get', 'kind'], 'radius'],
        paint: { 'fill-color': '#f97316', 'fill-opacity': 0.13 },
      })
    }
    if (!map.getLayer(circleID)) {
      map.addLayer({
        id: circleID,
        type: 'circle',
        source: sourceID,
        filter: ['==', ['get', 'kind'], 'control'],
        paint: {
          'circle-radius': 13,
          'circle-color': '#ffffff',
          'circle-stroke-width': 2,
          'circle-stroke-color': '#f97316',
        },
      })
    }
    if (!map.getLayer(labelID)) {
      map.addLayer({
        id: labelID,
        type: 'symbol',
        source: sourceID,
        filter: ['==', ['get', 'kind'], 'control'],
        layout: { 'text-field': ['get', 'label'], 'text-size': 11 },
        paint: { 'text-color': '#111827' },
      })
    }
  }, [mode, checkpointControls, styleVersion])

  const handleRangeReset = useCallback(() => {
    setRange(initialRange)
    setHasUserSelection(false)
  }, [initialRange])

  // handleFullReset: resets both geometry selection AND all form fields (Fix #12).
  // The initial meta shape mirrors the useState initializer at mount.
  const handleFullReset = useCallback(() => {
    setRange(initialRange)
    setHasUserSelection(false)
    setCheckpointControls([])
    setMeta({
      name: '',
      segType: (mode === 'lap' ? 'lap' : 'standard') as SegType,
      sportTypes: null,
      sacScale: null,
      mtbScale: null,
      surface: null,
      difficultyColor: null,
      bidirectional: BIDIRECTIONAL_DEFAULT[(mode === 'lap' ? 'lap' : 'standard') as SegType],
      userEditedBidirectional: false,
      isPublic: true,
    })
  }, [initialRange, mode])

  const handleTrackReverse = useCallback(() => {
    setEditedTrackPoints(prev => {
      const N = prev.length
      if (N === 0) return prev
      const reversed = [...prev].reverse()
      // Sync ref immediately so detectLapCrossings (and any other consumer of
      // trackPointsRef.current) sees the updated order before the next render.
      trackPointsRef.current = reversed
      setRange(r => [N - 1 - r[1], N - 1 - r[0]])
      // Push reversed coords to the activity-track MapLibre source so the
      // direction chevrons (symbol-placement: line, icon-rotation-alignment:
      // auto) re-orient. Without this the line-geometry stays in original
      // order and arrows keep pointing the wrong way.
      const map = mapRef.current
      if (map) {
        const src = map.getSource('activity-track') as maplibregl.GeoJSONSource | undefined
        if (src) {
          src.setData({
            type: 'Feature',
            geometry: { type: 'LineString', coordinates: reversed.map(p => [p.lng, p.lat]) },
            properties: {},
          })
        }
      }
      return reversed
    })
  }, [])

  // ── Live-Info for sub-range ───────────────────────────────────────────────
  const info = useMemo(() => {
    if (mode === 'track') {
      const subset = editedTrackPoints.slice(range[0], range[1] + 1)
      let dist = 0, gain = 0, loss = 0
      for (let i = 1; i < subset.length; i++) {
        const a = subset[i - 1]; const b = subset[i]
        dist += haversineM(a.lat, a.lng, b.lat, b.lng) / 1000
        if (a.elevation != null && b.elevation != null) {
          const dh = b.elevation - a.elevation
          if (dh > 0) gain += dh; else loss += -dh
        }
      }
      return { distanceKm: dist, gainM: gain, lossM: loss }
    }
    if (mode === 'draw' && drawnCoords.length >= 2) {
      let dist = 0
      for (let i = 1; i < drawnCoords.length; i++) {
        const [lng1, lat1] = drawnCoords[i - 1]
        const [lng2, lat2] = drawnCoords[i]
        dist += haversineM(lat1, lng1, lat2, lng2) / 1000
      }
      return { distanceKm: dist, gainM: 0, lossM: 0 }
    }
    return null
  }, [mode, editedTrackPoints, range, drawnCoords])

  // ── Draw-mode actions ─────────────────────────────────────────────────────
  const drawUndoLast = useCallback(() => {
    const draw = drawRef.current
    if (!draw) return
    const all = draw.getAll()
    const feat = all.features.find((f: GeoJSON.Feature) => f.geometry.type === 'LineString')
    if (!feat || feat.geometry.type !== 'LineString') return
    const coords = feat.geometry.coordinates.slice(0, -1)
    if (coords.length < 2) {
      draw.deleteAll()
      setDrawnCoords([])
      return
    }
    feat.geometry.coordinates = coords
    draw.set({ type: 'FeatureCollection', features: [feat] })
    setDrawnCoords(coords as Array<[number, number]>)
  }, [])

  const drawReverse = useCallback(() => {
    setDrawnCoords(c => {
      const rev = [...c].reverse()
      const draw = drawRef.current
      if (draw) {
        draw.set({
          type: 'FeatureCollection',
          features: [{
            type: 'Feature',
            properties: {},
            geometry: { type: 'LineString', coordinates: rev },
          }],
        })
      }
      return rev
    })
  }, [])

  const drawClear = useCallback(() => {
    drawRef.current?.deleteAll()
    setDrawnCoords([])
  }, [])

  // ── Save with frontend validation (Fix #11) ───────────────────────────────
  // Provides immediate user feedback for missing required fields instead of
  // silently dispatching a request that the backend would reject with 4xx.
  const handleSave = useCallback(() => {
    if (!meta.name.trim()) {
      addToast({ type: 'warning', title: 'Name erforderlich', message: 'Bitte gib dem Segment einen Namen.' })
      return
    }
    if (mode === 'track') {
      const count = range[1] - range[0] + 1
      if (count < 2) {
        addToast({ type: 'warning', title: 'Zu kurze Auswahl', message: 'Wähle mindestens 2 Trackpunkte aus.' })
        return
      }
    }
    if (mode === 'draw' && drawnCoords.length < 2) {
      addToast({ type: 'warning', title: 'Segment zu kurz', message: 'Zeichne mindestens 2 Punkte.' })
      return
    }
    if (mode === 'checkpoint' && checkpointControls.length < 2) {
      addToast({ type: 'warning', title: 'Start und Ziel erforderlich', message: 'Setze mindestens zwei Checkpoints.' })
      return
    }
    saveMutation.mutate()
  }, [meta.name, mode, range, drawnCoords.length, checkpointControls.length, saveMutation, addToast])

  // ── Dirty state + close logic ─────────────────────────────────────────────
  // Checks all meta fields (name, sacScale, mtbScale, surface, difficultyColor,
  // sportTypes, segType, bidirectional, isPublic) via JSON deep-compare, plus
  // mode-specific coordinate/range changes.
  const isDirty = (() => {
    const initialMeta = initialMetaRef.current
    const metaChanged = initialMeta !== null
      ? JSON.stringify(meta) !== JSON.stringify(initialMeta)
      : meta.name.length > 0 // fallback before first effect fires (rare, same tick)
    return (
      metaChanged ||
      (mode === 'track' && (range[0] !== initialRange[0] || range[1] !== initialRange[1])) ||
      (mode === 'draw' && drawnCoords.length > 0) ||
      (mode === 'checkpoint' && checkpointControls.length > 0)
    )
  })()

  const handleClose = useCallback(() => {
    if (isDirty) {
      // Show custom discard dialog instead of native window.confirm (Fix #13)
      setDiscardDialogOpen(true)
      return
    }
    onClose()
  }, [isDirty, onClose])

  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape') handleClose() }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [handleClose])

  useEffect(() => {
    if (!isDirty) return
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [isDirty])

  // ── Body-scroll-lock (B2) ────────────────────────────────────────────────
  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = prev }
  }, [])

  // Focus-Trap
  const modalRootRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const trap = (e: KeyboardEvent) => {
      if (e.key !== 'Tab') return
      const root = modalRootRef.current
      if (!root) return
      const focusable = root.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )
      if (focusable.length === 0) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      const active = document.activeElement as HTMLElement | null
      if (e.shiftKey && active === first) { e.preventDefault(); last.focus() }
      else if (!e.shiftKey && active === last) { e.preventDefault(); first.focus() }
    }
    window.addEventListener('keydown', trap)
    return () => window.removeEventListener('keydown', trap)
  }, [])

  // ── Render ────────────────────────────────────────────────────────────────
  const content = (
    <div ref={modalRootRef} className="fixed inset-0 z-[200] flex flex-col bg-white dark:bg-neutral-950 text-neutral-900 dark:text-neutral-100">
      {/* Top-Bar (B1-B4) */}
      <div className="flex items-center gap-2 border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900 px-4 py-2.5 text-sm">
        <h2 className="font-semibold text-neutral-900 dark:text-neutral-100">
          {mode === 'track' ? 'Segment aus Track' : mode === 'draw' ? 'Segment zeichnen' : mode === 'checkpoint' ? 'Checkpoints' : 'Rundkurs definieren'}
        </h2>
        <div className="flex-1" />

        {/* B1 — Map-Style toggles: genuine segmented control */}
        <div className="flex items-center gap-0.5 rounded-lg bg-neutral-100 dark:bg-neutral-800 p-1">
          {base.filter((l) => l.source_kind === 'raster_tiles').map((l) => (
            <button
              key={l.key}
              type="button"
              onClick={() => setMapStyle(l.key)}
              className={cn(
                'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                mapStyle === l.key
                  ? 'bg-white dark:bg-neutral-700 shadow-sm text-neutral-900 dark:text-neutral-100'
                  : 'text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100',
              )}
            >
              {l.name}
            </button>
          ))}
        </div>

        {/* B3 — Heatmap toggle: visually separated from basemap group */}
        <div className="ml-3 border-l border-neutral-200 dark:border-neutral-700 pl-3">
          <button
            type="button"
            onClick={() => setHeatmapOn(v => !v)}
            aria-pressed={heatmapOn}
            className={cn(
              'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
              heatmapOn
                ? 'bg-accent-DEFAULT/10 text-accent-DEFAULT dark:text-accent-400 ring-1 ring-accent-DEFAULT/30'
                : 'text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 hover:bg-neutral-100 dark:hover:bg-neutral-800',
            )}
          >
            Heatmap
          </button>
        </div>

        {/* B2 — Close-X: plain icon-button, separated from toggles */}
        <button
          type="button"
          onClick={handleClose}
          aria-label="Schließen"
          className="ml-2 flex items-center justify-center rounded-md p-1.5 text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 hover:text-neutral-900 dark:hover:text-white transition-colors"
        >
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>

      {/* Middle — Sidebar (full-height) + right column (Map + BottomBar) */}
      <div className="flex min-h-0 flex-1">
        {/* Sidebar — always editable (Vorschau-Phase-Lock entfernt). */}
        {/* min-h-0 is required so that div participates in flex shrinking and
            allows the child SegmentSidebar's overflow-y-auto to actually scroll. */}
        <div className="flex min-h-0 w-72 shrink-0 flex-col">
          <SegmentSidebar
            mode={mode}
            meta={meta}
            onMetaChange={patchMeta}
            info={info}
            allSportTypes={allSportTypes}
            onEnrichmentRequest={handleEnrichment}
            enrichmentState={{ loading: enrichmentMutation.isPending }}
            enrichmentSources={enrichmentSources}
            onClearEnrichmentSource={(f) => setEnrichmentSources(s => ({ ...s, [f]: null }))}
            enrichmentDisabledReason={enrichmentDisabledReason}
            enrichmentStatus={enrichmentStatus}
          />
          {mode === 'checkpoint' && (
            <CheckpointControlsList
              controls={checkpointControls}
              onRemove={(index) => setCheckpointControls(prev => prev.filter((_, i) => i !== index).map((control, idx, all) => ({
                ...control,
                sequence_index: idx,
                role: idx === 0 ? 'start' : idx === all.length - 1 ? 'finish' : 'checkpoint',
              })))}
            />
          )}
        </div>

        {/* Right column: Map (flex-1) + BottomBar (intrinsic height) */}
        <div className="flex min-h-0 flex-1 flex-col">
          {/* Map container — fills remaining vertical space */}
          <div className="relative min-h-0 flex-1">
            <div ref={mapContainerRef} className="h-full w-full" />
            {/* Lap-mode instruction overlay */}
            {mode === 'lap' && lapCrossings.length === 0 && (
              <div
                role="status"
                aria-live="polite"
                className="pointer-events-none absolute bottom-4 left-1/2 -translate-x-1/2 rounded-md bg-black/70 px-4 py-2 text-sm text-neutral-200"
              >
                Klicke auf den Track um den Start/Ziel-Punkt zu setzen
              </div>
            )}
          </div>

          {/* BottomBar — docked below the map, only as wide as the right column */}
          {mode === 'track' && (
            <TrackModeBottomBar
              trackPoints={editedTrackPoints}
              range={range}
              onRangeChange={(r) => { setHasUserSelection(true); setRange(r) }}
              onCommitRange={commitRangeToMap}
              onReverse={handleTrackReverse}
              onReset={handleRangeReset}
              onResetAll={handleFullReset}
              onPreviewOrSave={handleSave}
              onHover={setHoverIndex}
            />
          )}
          {mode === 'draw' && (
            <DrawModeBottomBar
              pointCount={drawnCoords.length}
              onUndo={drawUndoLast}
              onClear={drawClear}
              onReverse={drawReverse}
              onReset={drawClear}
              onPreviewOrSave={handleSave}
            />
          )}
          {mode === 'lap' && (
            <LapModeBottomBar
              detectedLaps={detectedLaps}
              onBack={() => {
                // Reset lap state so user can reclick the finish line
                setLapCrossings([])
              }}
              onPreviewOrSave={handleSave}
            />
          )}
          {mode === 'checkpoint' && (
            <CheckpointModeBottomBar
              controlCount={checkpointControls.length}
              onClear={() => setCheckpointControls([])}
              onPreviewOrSave={handleSave}
            />
          )}
        </div>
      </div>

      {/* Custom discard-confirmation dialog (Fix #13 — replaces window.confirm) */}
      {discardDialogOpen && (
        <div
          className="fixed inset-0 z-[300] flex items-center justify-center bg-black/50"
          aria-modal="true"
          role="dialog"
          aria-labelledby={discardDialogId}
          onClick={(e) => { if (e.target === e.currentTarget) setDiscardDialogOpen(false) }}
        >
          <div className="mx-4 w-full max-w-sm rounded-xl border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 p-6 shadow-2xl">
            <h3 id={discardDialogId} className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
              Änderungen verwerfen?
            </h3>
            <p className="mt-2 text-sm text-neutral-600 dark:text-neutral-400">
              Du hast ungespeicherte Änderungen. Wenn du jetzt schließt, gehen sie verloren.
            </p>
            <div className="mt-5 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setDiscardDialogOpen(false)}
                className="rounded-md border border-neutral-200 dark:border-neutral-700 bg-neutral-100 dark:bg-neutral-800 px-3 py-1.5 text-sm text-neutral-700 dark:text-neutral-300 transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700"
              >
                Abbrechen
              </button>
              <button
                type="button"
                onClick={() => { setDiscardDialogOpen(false); onClose() }}
                className="rounded-md bg-accent-DEFAULT px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-accent-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1"
              >
                Änderungen verwerfen
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )

  return createPortal(content, document.body)
}
