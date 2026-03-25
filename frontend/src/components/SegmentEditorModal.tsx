// frontend/src/components/SegmentEditorModal.tsx
import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
// @ts-ignore
import MapboxDraw from 'maplibre-gl-draw'
import { apiClient } from '@/api/client'
import { createSegment } from '@/api/segments'
import { nearestIndex, extractSubTrack, detectLapCrossings } from './segment-editor/TrackMode'
import { buildRasterStyle, RASTER_STYLES, type BaseStyle } from '@/lib/mapStyles'

type EditorMode = 'track' | 'draw' | 'lap'
type EditorPhase = 'edit' | 'form'

interface Props {
  mode: EditorMode
  trackPoints?: Array<{ lat: number; lng: number; time?: string }>
  sourceActivityId?: string
  defaultSportGroup?: string
  onSuccess: () => void
  onClose: () => void
}


const SPORT_GROUPS = [
  { value: 'bike', label: 'Rad' },
  { value: 'foot', label: 'Laufen/Wandern' },
  { value: 'swim', label: 'Schwimmen' },
  { value: 'ski', label: 'Winter' },
  { value: 'paddle', label: 'Wassersport' },
  { value: 'generic', label: 'Generisch' },
]

const SAC_SCALES = [
  { value: 'T1', label: 'T1 — Wandern' },
  { value: 'T2', label: 'T2 — Bergwandern' },
  { value: 'T3', label: 'T3 — Anspruchsvolles Bergwandern' },
  { value: 'T4', label: 'T4 — Alpinwandern' },
  { value: 'T5', label: 'T5 — Anspruchsvolles Alpinwandern' },
  { value: 'T6', label: 'T6 — Schwieriges Alpinwandern' },
]

const MTB_SCALES = [
  { value: 'S0', label: 'S0 — Leicht' },
  { value: 'S1', label: 'S1 — Leicht technisch' },
  { value: 'S2', label: 'S2 — Technisch' },
  { value: 'S3', label: 'S3 — Schwer technisch' },
  { value: 'S4', label: 'S4 — Sehr schwer' },
  { value: 'S5', label: 'S5 — Extrem' },
]

const SURFACES = ['Asphalt', 'Schotter', 'Erde', 'Sand', 'Gras', 'Fels', 'Pflaster', 'Holz']

export default function SegmentEditorModal({
  mode, trackPoints, sourceActivityId, defaultSportGroup, onSuccess, onClose,
}: Props) {
  const { data: meData } = useQuery({
    queryKey: ['me'],
    queryFn: () => apiClient<{ user_id: string }>('/me'),
    staleTime: Infinity,
  })
  const currentUserId = meData?.user_id ?? ''

  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  // Ref to always expose the latest trackPoints to stale-closure callbacks (map init useEffect)
  const trackPointsRef = useRef(trackPoints)
  useEffect(() => { trackPointsRef.current = trackPoints }, [trackPoints])
  const [heatmapVisible, setHeatmapVisible] = useState(false)
  const [mapStyle, setMapStyle] = useState<BaseStyle>('osm')
  const [phase, setPhase] = useState<EditorPhase>('edit')

  // Gemeinsame State-Variablen (alle Modi)
  const [geoJSON, setGeoJSON] = useState('')
  const [lapMeta, setLapMeta] = useState<Record<string, unknown>>({})

  // Track-Modus State
  const [trackPhase, setTrackPhase] = useState<'pick-start' | 'pick-end' | 'adjust' | 'preview'>('pick-start')
  const [startIdx, setStartIdx] = useState(0)
  const [endIdx, setEndIdx] = useState(0)

  // Draw-Modus State
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const drawRef = useRef<any>(null)
  const [drawPhase, setDrawPhase] = useState<'drawing' | 'preview'>('drawing')
  const [drawGeoJSON, setDrawGeoJSON] = useState<string | null>(null)

  // Lap-Modus State
  const [lapPhase, setLapPhase] = useState<'pick-finish' | 'review'>('pick-finish')
  const [lapFinishIdx, setLapFinishIdx] = useState<number | null>(null)
  const [lapCrossings, setLapCrossings] = useState<number[]>([])

  // Form State
  const [segName, setSegName] = useState('')
  const [sportGroup, setSportGroup] = useState(defaultSportGroup ?? 'bike')
  const [segType, setSegType] = useState<'standard' | 'trail' | 'lap'>(
    mode === 'lap' ? 'lap' : 'standard'
  )
  const [sacScale, setSacScale] = useState('')
  const [mtbScale, setMtbScale] = useState('')
  const [diffColor, setDiffColor] = useState('')
  const [surface, setSurface] = useState('')
  const [isMountain, setIsMountain] = useState(false)
  const [climbDirection, setClimbDirection] = useState<'bergauf' | 'bergab'>('bergauf')
  const [submitError, setSubmitError] = useState<string | null>(null)

  function detectClimbDirection(geojsonStr: string): 'bergauf' | 'bergab' | null {
    try {
      const geom = JSON.parse(geojsonStr)
      if (geom.type !== 'LineString' || !geom.coordinates?.length) return null
      const first = geom.coordinates[0]
      const last = geom.coordinates[geom.coordinates.length - 1]
      if (first.length < 3 || last.length < 3) return null
      const diff = last[2] - first[2]
      if (Math.abs(diff) < 10) return null
      return diff > 0 ? 'bergauf' : 'bergab'
    } catch { return null }
  }

  function proceedToForm(geo: string) {
    setGeoJSON(geo)
    const detected = detectClimbDirection(geo)
    if (detected) {
      setIsMountain(true)
      setClimbDirection(detected)
    } else if (segType === 'trail') {
      // Sport-based defaults when elevation detection has no result
      setIsMountain(true)
      setClimbDirection(sportGroup === 'foot' ? 'bergauf' : 'bergab')
    }
    setPhase('form')
  }

  function getClimbType(): string {
    if (segType === 'lap') return 'standard'
    if (isMountain) return climbDirection
    return 'standard'
  }

  // Mutation
  const mutation = useMutation({
    mutationFn: createSegment,
    onSuccess: () => onSuccess(),
    onError: () => setSubmitError('Fehler beim Speichern.'),
  })

  function handleSubmit() {
    setSubmitError(null)
    const meta: Record<string, unknown> = {}
    if (segType === 'trail') {
      if (diffColor) meta.difficulty_color = diffColor
      if (sacScale) meta.sac_scale = sacScale
      if (mtbScale) meta.mtb_scale = mtbScale
    } else if (segType === 'standard') {
      if (surface) meta.surface = surface
    } else if (segType === 'lap') {
      Object.assign(meta, lapMeta)
    }
    mutation.mutate({
      name: segName.trim(),
      sport_group: sportGroup,
      segment_type: segType,
      segment_type_meta: Object.keys(meta).length > 0 ? meta : undefined,
      geojson_track: geoJSON,
      source_activity_id: sourceActivityId,
      terrain_type: segType === 'trail' ? 'trail' : 'standard',
      climb_type: getClimbType(),
    })
  }

  // Hilfsfunktion: Track-Line + Richtungspfeile hinzufügen (auch nach Style-Wechsel aufrufbar)
  function addTrackLayerToMap(m: maplibregl.Map) {
    const pts = trackPointsRef.current
    if (!pts || pts.length < 2) return
    if (m.getSource('activity-track')) return
    const coords = pts.map(p => [p.lng, p.lat])
    m.addSource('activity-track', {
      type: 'geojson',
      data: { type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: coords } } as GeoJSON.Feature,
    })
    m.addLayer({
      id: 'activity-track-line',
      type: 'line',
      source: 'activity-track',
      paint: { 'line-color': '#6b7280', 'line-width': 2, 'line-opacity': 0.7 },
    })
    // Richtungspfeile als Symbol-Layer
    if (m.hasImage('arrow')) {
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
  }

  // Pfeil-Icon registrieren (einmalig) und Track-Layer hinzufügen
  function registerArrowAndTrack(m: maplibregl.Map) {
    const arrowSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path d="M8 0 L16 8 L8 16 L8 10 L0 10 L0 6 L8 6 Z" fill="#fff" stroke="#333" stroke-width="1"/></svg>`
    const img = new Image(16, 16)
    // Assign onload/onerror BEFORE setting src to avoid race on data: URLs
    img.onload = () => {
      if (!m.hasImage('arrow')) m.addImage('arrow', img)
      addTrackLayerToMap(m)
    }
    img.onerror = () => addTrackLayerToMap(m)
    img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(arrowSvg)
  }

  // Map Init
  useEffect(() => {
    if (!mapContainerRef.current || mapRef.current) return
    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: buildRasterStyle(mapStyle),
      center: [11.0, 48.0],
      zoom: 10,
    })
    mapRef.current = map

    map.on('load', () => {
      // Read from ref so we always use the current trackPoints, not a stale closure value
      const pts = trackPointsRef.current
      if (pts && pts.length > 0) {
        const lngs = pts.map(p => p.lng)
        const lats = pts.map(p => p.lat)
        const bounds: maplibregl.LngLatBoundsLike = [
          [Math.min(...lngs), Math.min(...lats)],
          [Math.max(...lngs), Math.max(...lats)],
        ]
        map.fitBounds(bounds, { padding: 40 })
      }
      registerArrowAndTrack(map)
    })

    return () => { map.remove(); mapRef.current = null }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Heatmap-Layer Toggle
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    const layerId = 'editor-heatmap'
    if (heatmapVisible) {
      if (!map.getSource(layerId)) {
        map.addSource(layerId, {
          type: 'vector',
          tiles: [`${window.location.origin}/tiles/heatmap_lines_mvt/{z}/{x}/{y}${currentUserId ? `?user_id=${encodeURIComponent(currentUserId)}` : ''}`],
        })
        map.addLayer({
          id: layerId,
          type: 'line',
          source: layerId,
          'source-layer': 'heatmap_lines_mvt',
          paint: { 'line-color': '#f97316', 'line-width': 2, 'line-opacity': 0.5 },
        })
      }
    } else {
      if (map.getLayer(layerId)) map.removeLayer(layerId)
      if (map.getSource(layerId)) map.removeSource(layerId)
    }
  }, [heatmapVisible])

  // --- Track-Modus: Click Handler ---
  useEffect(() => {
    const map = mapRef.current
    if (!map || mode !== 'track') return
    const onClick = (e: maplibregl.MapMouseEvent) => {
      if (trackPhase === 'preview') return
      const idx = nearestIndex(trackPoints ?? [], e.lngLat.lat, e.lngLat.lng)
      if (trackPhase === 'pick-start') {
        setStartIdx(idx)
        setEndIdx(Math.min(idx + 50, (trackPoints?.length ?? 1) - 1))
        setTrackPhase('pick-end')
      } else if (trackPhase === 'pick-end') {
        if (idx > startIdx) {
          setEndIdx(idx)
          setTrackPhase('adjust')
        }
      }
    }
    map.on('click', onClick)
    return () => { map.off('click', onClick) }
  }, [mode, trackPhase, startIdx, trackPoints])

  // --- Track-Modus: Sub-Track Preview Layer ---
  useEffect(() => {
    const map = mapRef.current
    if (!map || !trackPoints || trackPhase === 'pick-start') return
    if (!map.isStyleLoaded()) return
    const coords = trackPoints.slice(startIdx, endIdx + 1).map(p => [p.lng, p.lat])
    const geom: GeoJSON.Feature = {
      type: 'Feature',
      properties: {},
      geometry: { type: 'LineString', coordinates: coords },
    }
    const src = map.getSource('segment-preview') as maplibregl.GeoJSONSource | undefined
    if (src) {
      src.setData(geom)
    } else {
      if (!map.getSource('segment-preview')) {
        map.addSource('segment-preview', { type: 'geojson', data: geom })
        map.addLayer({
          id: 'segment-preview-line',
          type: 'line',
          source: 'segment-preview',
          paint: { 'line-color': '#f97316', 'line-width': 4, 'line-opacity': 0.9 },
        })
      }
    }
  }, [startIdx, endIdx, trackPhase, trackPoints])

  // --- Draw-Modus Init ---
  useEffect(() => {
    const map = mapRef.current
    if (!map || mode !== 'draw') return
    const initDraw = () => {
      const draw = new MapboxDraw({
        displayControlsDefault: false,
        defaultMode: 'draw_line_string',
      })
      // MapboxDraw implements IControl interface
      map.addControl(draw as unknown as maplibregl.IControl)
      drawRef.current = draw
    }
    if (map.isStyleLoaded()) {
      initDraw()
    } else {
      map.on('load', initDraw)
    }
    return () => {
      if (drawRef.current) {
        try { map.removeControl(drawRef.current as unknown as maplibregl.IControl) } catch { /* ignore */ }
        drawRef.current = null
      }
    }
  }, [mode])

  // --- Lap-Modus: Click Handler ---
  useEffect(() => {
    const map = mapRef.current
    if (!map || mode !== 'lap' || lapPhase !== 'pick-finish') return
    const onClick = (e: maplibregl.MapMouseEvent) => {
      const idx = nearestIndex(trackPoints ?? [], e.lngLat.lat, e.lngLat.lng)
      setLapFinishIdx(idx)
      const crossings = detectLapCrossings(trackPoints ?? [], idx)
      setLapCrossings(crossings)
      setLapPhase('review')
    }
    map.on('click', onClick)
    return () => { map.off('click', onClick) }
  }, [mode, lapPhase, trackPoints])

  // --- Lap-Modus: Layer Visualisierung ---
  useEffect(() => {
    const map = mapRef.current
    if (!map || mode !== 'lap' || lapCrossings.length < 2) return
    if (!map.isStyleLoaded()) return

    // Remove old lap layers
    const existingLayers = map.getStyle()?.layers ?? []
    existingLayers.forEach(l => {
      if (l.id.startsWith('lap-segment-')) {
        map.removeLayer(l.id)
      }
    })
    // Remove old sources
    for (let i = 0; i < 20; i++) {
      if (map.getSource(`lap-seg-${i}`)) map.removeSource(`lap-seg-${i}`)
    }

    // Add colored lap segments
    const colors = ['#f97316', '#f59e0b']
    for (let i = 1; i < lapCrossings.length; i++) {
      const start = lapCrossings[i - 1]
      const end = lapCrossings[i]
      const coords = (trackPoints ?? []).slice(start, end + 1).map(p => [p.lng, p.lat])
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
  }, [lapCrossings, lapPhase, mode, trackPoints])

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-neutral-950">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 bg-neutral-900 border-b border-neutral-800">
        <h2 className="text-sm font-semibold text-neutral-100">
          {mode === 'track' ? 'Segment aus Track' : mode === 'draw' ? 'Segment zeichnen' : 'Rundkurs definieren'}
          {phase === 'form' && ' — Details'}
        </h2>
        <div className="flex items-center gap-3">
          {phase === 'edit' && (
            <>
              {/* Kartenstil-Auswahl */}
              <div className="flex rounded-lg overflow-hidden border border-neutral-600 bg-neutral-800/90">
                {RASTER_STYLES.map(([key, def]) => (
                  <button
                    key={key}
                    onClick={() => {
                      setMapStyle(key)
                      const map = mapRef.current
                      if (!map) return
                      map.setStyle(buildRasterStyle(key))
                      map.once('style.load', () => {
                        registerArrowAndTrack(map)
                        if (heatmapVisible) {
                          const layerId = 'editor-heatmap'
                          if (!map.getSource(layerId)) {
                            map.addSource(layerId, {
                              type: 'vector',
                              tiles: [`${window.location.origin}/tiles/heatmap_lines_mvt/{z}/{x}/{y}${currentUserId ? `?user_id=${encodeURIComponent(currentUserId)}` : ''}`],
                            })
                            map.addLayer({
                              id: layerId,
                              type: 'line',
                              source: layerId,
                              'source-layer': 'heatmap_lines_mvt',
                              paint: { 'line-color': '#f97316', 'line-width': 2, 'line-opacity': 0.5 },
                            })
                          }
                        }
                      })
                    }}
                    className={`px-2 py-1 text-xs font-medium transition-colors ${
                      mapStyle === key
                        ? 'bg-accent-DEFAULT text-white'
                        : 'text-neutral-400 hover:text-neutral-100 hover:bg-neutral-700'
                    }`}
                  >
                    {def.label}
                  </button>
                ))}
              </div>
              {/* Heatmap-Toggle */}
              <button
                onClick={() => setHeatmapVisible(v => !v)}
                className={`px-2 py-1 text-xs rounded-md border transition-colors ${
                  heatmapVisible
                    ? 'bg-accent-DEFAULT/20 border-accent-DEFAULT/50 text-orange-400'
                    : 'border-neutral-700 text-neutral-400 hover:text-neutral-200'
                }`}
              >
                Heatmap
              </button>
            </>
          )}
          <button onClick={onClose} className="cursor-pointer text-neutral-400 hover:text-neutral-100 text-lg leading-none">&#x2715;</button>
        </div>
      </div>

      {/* Karte (nur im edit-Phase) — hidden statt unmounted damit Map-Instanz erhalten bleibt */}
      <div ref={mapContainerRef} className={`flex-1 ${phase === 'form' ? 'hidden' : ''}`} />

      {/* Formular (nur im form-Phase) */}
      {phase === 'form' && (
        <div className="flex-1 overflow-y-auto px-4 py-4 space-y-4">
          <h3 className="text-sm font-semibold text-neutral-200">Segment-Details</h3>

          <input value={segName} onChange={e => setSegName(e.target.value)}
            placeholder="Segment-Name *" required
            className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100 placeholder-neutral-500" />

          <select value={sportGroup} onChange={e => setSportGroup(e.target.value)}
            className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100">
            {SPORT_GROUPS.map(g => <option key={g.value} value={g.value}>{g.label}</option>)}
          </select>

          {mode !== 'lap' && (
            <select value={segType} onChange={e => setSegType(e.target.value as 'standard' | 'trail')}
              className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100">
              <option value="standard">Standard</option>
              <option value="trail">Trail</option>
            </select>
          )}

          {/* Berg-Checkbox (nur bei Standard und Trail, nicht bei Lap) */}
          {mode !== 'lap' && (
            <div className="space-y-2">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={isMountain}
                  onChange={e => setIsMountain(e.target.checked)}
                  className="rounded border-neutral-600"
                />
                <span className="text-sm text-neutral-300">Berg (Auf-/Abstieg unterscheiden)</span>
              </label>

              {isMountain && (
                <div className="ml-6 flex gap-4">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="radio"
                      name="climb-dir"
                      checked={climbDirection === 'bergauf'}
                      onChange={() => setClimbDirection('bergauf')}
                      className="border-neutral-600"
                    />
                    <span className="text-sm text-neutral-300">Bergauf</span>
                  </label>
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="radio"
                      name="climb-dir"
                      checked={climbDirection === 'bergab'}
                      onChange={() => setClimbDirection('bergab')}
                      className="border-neutral-600"
                    />
                    <span className="text-sm text-neutral-300">Bergab</span>
                  </label>
                </div>
              )}
            </div>
          )}

          {segType === 'trail' && (
            <div className="space-y-2">
              <select value={diffColor} onChange={e => setDiffColor(e.target.value)}
                className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100">
                <option value="">Farbe — keine</option>
                <option value="blue">Blau (leicht)</option>
                <option value="red">Rot (mittel)</option>
                <option value="black">Schwarz (schwer)</option>
              </select>
              <select value={sacScale} onChange={e => setSacScale(e.target.value)}
                className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100">
                <option value="">SAC-Skala — keine</option>
                {SAC_SCALES.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
              </select>
              <select value={mtbScale} onChange={e => setMtbScale(e.target.value)}
                className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100">
                <option value="">MTB-Skala — keine</option>
                {MTB_SCALES.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
              </select>
            </div>
          )}

          {segType === 'standard' && (
            <select value={surface} onChange={e => setSurface(e.target.value)}
              className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100">
              <option value="">Oberfläche — keine</option>
              {SURFACES.map(s => <option key={s} value={s.toLowerCase()}>{s}</option>)}
            </select>
          )}

          {mode === 'lap' && lapCrossings.length > 1 && (
            <p className="text-xs text-neutral-500">Rundkurs-Segment · {lapCrossings.length - 1} Runden erkannt</p>
          )}

          {submitError && <p className="text-sm text-red-400">{submitError}</p>}

          <div className="flex gap-2 justify-end pt-2">
            <button onClick={() => setPhase('edit')}
              className="cursor-pointer px-3 py-1.5 text-sm border border-neutral-700 rounded-lg text-neutral-300 hover:bg-neutral-800">
              &#x2190; Zurück
            </button>
            <button onClick={handleSubmit} disabled={!segName.trim() || mutation.isPending}
              className="cursor-pointer px-4 py-1.5 text-sm bg-accent-DEFAULT rounded-lg text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed">
              {mutation.isPending ? 'Speichern...' : 'Segment speichern'}
            </button>
          </div>
        </div>
      )}

      {/* Bottom controls (nur im edit-Phase) */}
      {phase === 'edit' && (
        <div className="bg-neutral-900 border-t border-neutral-800 px-4 py-3">
          {/* Track-Modus Controls */}
          {mode === 'track' && (
            <div className="flex flex-col gap-3">
              {trackPhase === 'pick-start' && (
                <p className="text-xs text-neutral-400">Klicke auf den Track um den <strong className="text-neutral-200">Startpunkt</strong> zu setzen</p>
              )}
              {trackPhase === 'pick-end' && (
                <p className="text-xs text-neutral-400">Klicke auf den Track um den <strong className="text-neutral-200">Endpunkt</strong> zu setzen</p>
              )}
              {(trackPhase === 'adjust' || trackPhase === 'preview') && (
                <>
                  <div className="flex gap-6">
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-neutral-400 w-10">Start</span>
                      <button onClick={() => setStartIdx(i => Math.max(0, i - 1))}
                        disabled={trackPhase === 'preview'}
                        className="px-2 py-1 text-xs bg-neutral-800 rounded hover:bg-neutral-700 disabled:opacity-30">&#x25C4;</button>
                      <span className="text-xs text-neutral-300 tabular-nums w-14 text-center">{startIdx}</span>
                      <button onClick={() => setStartIdx(i => Math.min(endIdx - 1, i + 1))}
                        disabled={trackPhase === 'preview'}
                        className="px-2 py-1 text-xs bg-neutral-800 rounded hover:bg-neutral-700 disabled:opacity-30">&#x25BA;</button>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-neutral-400 w-10">Ende</span>
                      <button onClick={() => setEndIdx(i => Math.max(startIdx + 1, i - 1))}
                        disabled={trackPhase === 'preview'}
                        className="px-2 py-1 text-xs bg-neutral-800 rounded hover:bg-neutral-700 disabled:opacity-30">&#x25C4;</button>
                      <span className="text-xs text-neutral-300 tabular-nums w-14 text-center">{endIdx}</span>
                      <button onClick={() => setEndIdx(i => Math.min((trackPoints?.length ?? 1) - 1, i + 1))}
                        disabled={trackPhase === 'preview'}
                        className="px-2 py-1 text-xs bg-neutral-800 rounded hover:bg-neutral-700 disabled:opacity-30">&#x25BA;</button>
                    </div>
                  </div>
                  <div className="flex gap-2 justify-end">
                    {trackPhase === 'preview' ? (
                      <>
                        <button onClick={() => setTrackPhase('adjust')}
                          className="px-3 py-1.5 text-sm border border-neutral-700 rounded-lg text-neutral-300 hover:bg-neutral-800">
                          Zurück bearbeiten
                        </button>
                        <button onClick={() => proceedToForm(extractSubTrack(trackPoints!, startIdx, endIdx))}
                          className="px-3 py-1.5 text-sm bg-accent-DEFAULT rounded-lg text-white hover:bg-accent-600">
                          Weiter &#x2192;
                        </button>
                      </>
                    ) : (
                      <button onClick={() => setTrackPhase('preview')}
                        className="px-3 py-1.5 text-sm bg-neutral-700 rounded-lg text-neutral-200 hover:bg-neutral-600">
                        Vorschau
                      </button>
                    )}
                  </div>
                </>
              )}
            </div>
          )}

          {/* Draw-Modus Controls */}
          {mode === 'draw' && (
            <div className="flex justify-between items-center">
              {drawPhase === 'drawing' && (
                <>
                  <p className="text-xs text-neutral-400">Klicke um Punkte hinzuzufügen. Doppelklick zum Abschließen.</p>
                  <button
                    onClick={() => {
                      const draw = drawRef.current
                      if (!draw) return
                      const data = draw.getAll()
                      const feature = data.features[0]
                      if (!feature || feature.geometry.type !== 'LineString' || feature.geometry.coordinates.length < 2) return
                      setDrawGeoJSON(JSON.stringify(feature.geometry))
                      setDrawPhase('preview')
                    }}
                    className="px-3 py-1.5 text-sm bg-neutral-700 rounded-lg text-neutral-200 hover:bg-neutral-600"
                  >
                    Vorschau
                  </button>
                </>
              )}
              {drawPhase === 'preview' && (
                <div className="flex gap-2 justify-end w-full">
                  <button
                    onClick={() => {
                      const draw = drawRef.current
                      if (!draw || !drawGeoJSON) return
                      draw.deleteAll()
                      const geom = JSON.parse(drawGeoJSON)
                      const id = draw.add({ type: 'Feature', geometry: geom, properties: {} })
                      draw.changeMode('direct_select', { featureId: id[0] })
                      setDrawPhase('drawing')
                    }}
                    className="px-3 py-1.5 text-sm border border-neutral-700 rounded-lg text-neutral-300 hover:bg-neutral-800"
                  >
                    Zurück bearbeiten
                  </button>
                  <button
                    onClick={() => proceedToForm(drawGeoJSON!)}
                    className="px-3 py-1.5 text-sm bg-accent-DEFAULT rounded-lg text-white hover:bg-accent-600"
                  >
                    Weiter &#x2192;
                  </button>
                </div>
              )}
            </div>
          )}

          {/* Lap-Modus Controls */}
          {mode === 'lap' && (
            <div>
              {lapPhase === 'pick-finish' && (
                <p className="text-xs text-neutral-400">Klicke auf den Track um den <strong className="text-neutral-200">Start/Ziel-Punkt</strong> zu setzen</p>
              )}
              {lapPhase === 'review' && (
                <div className="flex justify-between items-center">
                  <p className="text-sm text-neutral-300">
                    <strong className="text-white">{Math.max(0, lapCrossings.length - 1)}</strong> Runden erkannt
                  </p>
                  <div className="flex gap-2">
                    <button onClick={() => { setLapPhase('pick-finish'); setLapFinishIdx(null); setLapCrossings([]) }}
                      className="px-3 py-1.5 text-sm border border-neutral-700 rounded-lg text-neutral-300 hover:bg-neutral-800">
                      Zurück
                    </button>
                    <button
                      onClick={() => {
                        if (lapCrossings.length < 2 || lapFinishIdx === null) return
                        const first = lapCrossings[0]
                        const last = lapCrossings[lapCrossings.length - 1]
                        const coords = (trackPoints ?? []).slice(first, last + 1).map(p => [p.lng, p.lat])
                        coords.push(coords[0]) // close loop
                        const finish = trackPoints![lapFinishIdx]
                        const geo = JSON.stringify({ type: 'LineString', coordinates: coords })
                        setLapMeta({ finish_lat: finish.lat, finish_lng: finish.lng })
                        proceedToForm(geo)
                      }}
                      className="px-3 py-1.5 text-sm bg-accent-DEFAULT rounded-lg text-white hover:bg-accent-600"
                    >
                      Weiter &#x2192;
                    </button>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
