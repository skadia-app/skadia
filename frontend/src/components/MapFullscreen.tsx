import { useRef, useEffect, useMemo, useCallback, useState } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  AxisPointerComponent,
  BrushComponent,
  ToolboxComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useQuery } from '@tanstack/react-query'
import type { LngLatLike } from 'maplibre-gl'
import type { StreamPoint } from '@/api/types'
import ColorPicker from '@/components/ColorPicker'
import { BASE_STYLES, buildRasterStyle, type BaseStyle } from '@/lib/mapStyles'
import { addHeatmapLayers, removeHeatmapLayers, buildHeatmapTileUrl, loadHeatmapColor, saveHeatmapColor } from '@/lib/mapHeatmap'

echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  AxisPointerComponent,
  BrushComponent,
  ToolboxComponent,
  LegendComponent,
  CanvasRenderer,
])

// ── Types ─────────────────────────────────────────────────────────────────────

type FieldKey = 'altitude' | 'heartRate' | 'power' | 'cadence' | 'speed' | 'temperature'
type XMode = 'distance' | 'time'

interface ChartData {
  distanceKm: number[]
  elapsedMin: number[]
  altitude: number[]
  heartRate: number[]
  power: number[]
  cadence: number[]
  speed: number[]
  temperature: number[]
  maxDist: number
  maxElapsed: number
}

interface StreamDef {
  key: FieldKey
  label: string
  unit: string
  color: string
  yAxisIndex: number
}

interface Props {
  streams: StreamPoint[]
  sportColor: string
  onClose: () => void
}

// ── Stream definitions ────────────────────────────────────────────────────────

const ALL_STREAMS: StreamDef[] = [
  { key: 'altitude', label: 'Höhe', unit: 'm', color: '#22c55e', yAxisIndex: 0 },
  { key: 'heartRate', label: 'HF', unit: 'bpm', color: '#ef4444', yAxisIndex: 1 },
  { key: 'power', label: 'Leistung', unit: 'W', color: '#a855f7', yAxisIndex: 2 },
  { key: 'cadence', label: 'Kadenz', unit: 'rpm', color: '#f97316', yAxisIndex: 3 },
  { key: 'speed', label: 'Geschw.', unit: 'km/h', color: '#3b82f6', yAxisIndex: 4 },
  { key: 'temperature', label: 'Temp.', unit: '\u00b0C', color: '#06b6d4', yAxisIndex: 5 },
]

// ── Data processing (mirrors ActivityCharts) ──────────────────────────────────

function buildChartData(streams: StreamPoint[]): ChartData {
  const distanceKm: number[] = []
  const elapsedMin: number[] = []
  const altitude: number[] = []
  const heartRate: number[] = []
  const power: number[] = []
  const cadence: number[] = []
  const speed: number[] = []
  const temperature: number[] = []

  const t0 = streams.length > 0 ? new Date(streams[0].time).getTime() : 0

  for (const s of streams) {
    distanceKm.push(
      s.distance_m != null
        ? s.distance_m / 1000
        : distanceKm.length > 0 ? distanceKm[distanceKm.length - 1] : 0,
    )
    elapsedMin.push((new Date(s.time).getTime() - t0) / 60000)
    altitude.push(s.altitude_m ?? NaN)
    heartRate.push(s.heart_rate ?? NaN)
    power.push(s.power_w ?? NaN)
    cadence.push(s.cadence ?? NaN)
    speed.push(s.speed_ms != null ? s.speed_ms * 3.6 : NaN)
    temperature.push(s.temperature_c ?? NaN)
  }

  const validDist = distanceKm.filter(v => !isNaN(v) && isFinite(v))
  const rawMax = validDist.length > 0 ? Math.max(...validDist) : 1
  const maxDist = Math.ceil(rawMax * 10) / 10
  const maxElapsed = elapsedMin.length > 0 ? Math.ceil(elapsedMin[elapsedMin.length - 1]) : 0

  return { distanceKm, elapsedMin, altitude, heartRate, power, cadence, speed, temperature, maxDist, maxElapsed }
}

function hasValidData(arr: number[]): boolean {
  return arr.some(v => !isNaN(v))
}

function formatElapsed(minutes: number, maxMinutes: number): string {
  if (maxMinutes < 60) return `${Math.round(minutes)}\u202fmin`
  const h = Math.floor(minutes / 60)
  const m = Math.round(minutes % 60)
  if (h === 0) return `${m}\u202fmin`
  return `${h}h${m.toString().padStart(2, '0')}`
}

function findClosestIdx(arr: number[], target: number): number {
  if (arr.length === 0) return 0
  let bestIdx = 0
  let bestDiff = Math.abs(arr[0] - target)
  for (let i = 1; i < arr.length; i++) {
    const d = Math.abs(arr[i] - target)
    if (d < bestDiff) { bestDiff = d; bestIdx = i }
  }
  return bestIdx
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function MapFullscreen({ streams, sportColor, onClose }: Props) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const chartContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  const [xMode, setXMode] = useState<XMode>('distance')
  const [isZoomed, setIsZoomed] = useState(false)
  const [enabledStreams, setEnabledStreams] = useState<Set<FieldKey>>(() => {
    // Alle möglichen FieldKeys vorausgewählt — ECharts blendet Serien ohne Daten automatisch aus
    return new Set<FieldKey>(['altitude', 'heartRate', 'power', 'cadence', 'speed', 'temperature'])
  })

  const [baseStyle, setBaseStyle] = useState<BaseStyle>(() => {
    try {
      const d = JSON.parse(localStorage.getItem('skadia-map-defaults') ?? '{}') as Record<string, unknown>
      return (d.baseStyle as BaseStyle) ?? 'osm'
    } catch { return 'osm' }
  })
  const [showHeatmap, setShowHeatmap] = useState(false)
  const [heatmapColor, setHeatmapColor] = useState<string>(loadHeatmapColor)
  const [styleVersion, setStyleVersion] = useState(0)
  const pendingCameraRestoreRef = useRef<{ center: LngLatLike; zoom: number } | null>(null)
  const initialFitDoneRef = useRef(false)

  const { data: meData } = useQuery({
    queryKey: ['me'],
    queryFn: () =>
      fetch('/api/v1/me', { credentials: 'include' })
        .then(r => r.json()) as Promise<{ user_id: string }>,
    staleTime: Infinity,
  })
  const userId = meData?.user_id ?? ''

  const data = useMemo(() => buildChartData(streams), [streams])

  // Filter to only streams with actual data
  const availableStreams = useMemo(
    () => ALL_STREAMS.filter(s => hasValidData(data[s.key])),
    [data],
  )

  // Build coordinates for map
  const coordinates = useMemo(
    () => streams
      .filter((s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null)
      .map(s => [s.lng, s.lat] as [number, number]),
    [streams],
  )

  // ESC key to close
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  // Lock body scroll
  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = prev }
  }, [])

  // ── Initialize map ────────────────────────────────────────────────────────
  // MapLibre needs a container with non-zero dimensions. In a flex-col layout
  // the container may have 0×0 at mount time. We use ResizeObserver to defer
  // initialization until the browser has computed the layout, then keep
  // observing for future size changes (e.g. window resize).
  useEffect(() => {
    const el = mapContainerRef.current
    if (!el || coordinates.length < 2) return

    let map: maplibregl.Map | null = null
    let disposed = false

    const addTrackLayers = (m: maplibregl.Map) => {
      if (m.getSource('track')) return // guard: don't double-add

      m.addSource('track', {
        type: 'geojson',
        data: { type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates } },
      })
      m.addLayer({
        id: 'track-line',
        type: 'line',
        source: 'track',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': sportColor, 'line-width': 3, 'line-opacity': 0.85 },
      })
      m.addSource('highlight', {
        type: 'geojson',
        data: { type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: [] } },
      })
      m.addLayer({
        id: 'highlight-line',
        type: 'line',
        source: 'highlight',
        layout: { 'line-join': 'round', 'line-cap': 'round', visibility: 'none' },
        paint: { 'line-color': sportColor, 'line-width': 4, 'line-opacity': 1 },
      })

      if (!initialFitDoneRef.current) {
        const bounds = new maplibregl.LngLatBounds()
        for (const c of coordinates) bounds.extend(c)
        m.fitBounds(bounds, { padding: 60, maxZoom: 15 })
        initialFitDoneRef.current = true
      }
    }

    const initMap = () => {
      if (disposed || map) return
      if (el.clientWidth === 0 || el.clientHeight === 0) return

      map = new maplibregl.Map({
        container: el,
        style: buildRasterStyle(baseStyle),
        attributionControl: {},
      })

      map.addControl(new maplibregl.NavigationControl(), 'top-right')

      map.on('load', () => {
        map!.resize()
        addTrackLayers(map!)
      })

      map.on('style.load', () => {
        if (pendingCameraRestoreRef.current) {
          map!.jumpTo(pendingCameraRestoreRef.current)
          pendingCameraRestoreRef.current = null
        }
        addTrackLayers(map!)
        setStyleVersion(v => v + 1)
      })

      mapRef.current = map
    }

    // Observe container: init when sized, resize when dimensions change
    const ro = new ResizeObserver(() => {
      if (!map) {
        initMap()
      } else {
        map.resize()
      }
    })
    ro.observe(el)

    // Try immediately (container might already have dimensions)
    initMap()

    return () => {
      disposed = true
      ro.disconnect()
      map?.remove()
      mapRef.current = null
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Style switcher effect ─────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    pendingCameraRestoreRef.current = {
      center: map.getCenter() as LngLatLike,
      zoom: map.getZoom(),
    }
    map.setStyle(buildRasterStyle(baseStyle))
  }, [baseStyle]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Heatmap effect ────────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    removeHeatmapLayers(map)
    if (showHeatmap && userId) {
      addHeatmapLayers(map, buildHeatmapTileUrl(userId), heatmapColor, 'track-line')
    }
  }, [showHeatmap, heatmapColor, styleVersion, userId])

  // ── Heatmap color save handler ────────────────────────────────────────────
  const handleHeatmapColorChange = (hex: string) => {
    setHeatmapColor(hex)
    saveHeatmapColor(hex)
  }

  // ── Move hover marker on map ──────────────────────────────────────────────
  const moveMarker = useCallback((index: number | null) => {
    const map = mapRef.current
    if (!map || coordinates.length < 2) return

    const point = index != null ? streams[index] : null
    const hasPos = point?.lat != null && point?.lng != null

    if (hasPos) {
      const lngLat: [number, number] = [point.lng!, point.lat!]
      if (!markerRef.current) {
        const el = document.createElement('div')
        el.style.cssText = 'width:12px;height:12px;border-radius:50%;background:#fff;border:2px solid #f97316;box-shadow:0 0 4px rgba(0,0,0,0.3);'
        markerRef.current = new maplibregl.Marker({ element: el }).setLngLat(lngLat).addTo(map)
      } else {
        markerRef.current.setLngLat(lngLat)
      }
    } else if (markerRef.current) {
      markerRef.current.remove()
      markerRef.current = null
    }
  }, [coordinates, streams])

  // ── Highlight map segment on zoom ─────────────────────────────────────────
  const highlightMapSegment = useCallback((startIdx: number, endIdx: number) => {
    const map = mapRef.current
    if (!map) return

    const applyHighlight = () => {
      const segCoords = streams
        .slice(startIdx, endIdx + 1)
        .filter((s): s is StreamPoint & { lat: number; lng: number } =>
          s.lat != null && s.lng != null)
        .map(s => [s.lng, s.lat] as [number, number])

      map.setPaintProperty('track-line', 'line-color', '#999999')
      map.setPaintProperty('track-line', 'line-opacity', 0.3)

      ;(map.getSource('highlight') as maplibregl.GeoJSONSource | undefined)?.setData({
        type: 'Feature',
        properties: {},
        geometry: { type: 'LineString', coordinates: segCoords },
      })
      map.setLayoutProperty('highlight-line', 'visibility', 'visible')

      if (segCoords.length >= 2) {
        const bounds = new maplibregl.LngLatBounds()
        for (const c of segCoords) bounds.extend(c)
        map.fitBounds(bounds, { padding: 80, maxZoom: 18, duration: 600 })
      }
    }

    if (map.isStyleLoaded()) applyHighlight()
    else map.once('load', applyHighlight)
  }, [streams])

  const clearMapHighlight = useCallback(() => {
    const map = mapRef.current
    if (!map) return

    const apply = () => {
      map.setPaintProperty('track-line', 'line-color', sportColor)
      map.setPaintProperty('track-line', 'line-opacity', 0.85)
      map.setLayoutProperty('highlight-line', 'visibility', 'none')
      ;(map.getSource('highlight') as maplibregl.GeoJSONSource | undefined)?.setData({
        type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: [] },
      })

      const bounds = new maplibregl.LngLatBounds()
      for (const c of coordinates) bounds.extend(c)
      map.fitBounds(bounds, { padding: 60, maxZoom: 15, duration: 600 })
    }

    if (map.isStyleLoaded()) apply()
    else map.once('load', apply)
  }, [coordinates, sportColor])

  // ── Initialize + update ECharts ───────────────────────────────────────────
  useEffect(() => {
    if (!chartContainerRef.current) return

    const chart = echarts.init(chartContainerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
    const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

    // Build y-axes and series for enabled streams
    const yAxes: echarts.EChartsCoreOption[] = []
    const series: echarts.EChartsCoreOption[] = []
    const legendData: { name: string; itemStyle: { color: string } }[] = []

    // We create a y-axis per available stream to keep scales independent
    // Axes alternate left/right positioning
    const enabledList = availableStreams.filter(s => enabledStreams.has(s.key))

    availableStreams.forEach((stream, idx) => {
      const isEnabled = enabledStreams.has(stream.key)
      const enabledIdx = enabledList.indexOf(stream)

      legendData.push({ name: stream.label, itemStyle: { color: stream.color } })

      // Only show y-axis if this stream is enabled
      const showAxis = isEnabled && enabledIdx >= 0
      // Position: first axis left, rest right (to keep chart area clean)
      const isRight = enabledIdx > 0

      yAxes.push({
        type: 'value',
        show: showAxis,
        position: isRight ? 'right' : 'left',
        offset: isRight ? Math.max(0, (enabledIdx - 1) * 55) : 0,
        axisLabel: {
          fontSize: 10,
          color: stream.color,
          formatter: (v: number) => `${Math.round(v)}`,
        },
        splitLine: { show: !isRight, lineStyle: { color: 'rgba(200,200,200,0.3)', type: 'dashed' } },
        axisLine: { show: showAxis, lineStyle: { color: stream.color } },
        axisTick: { show: false },
      })

      series.push({
        name: stream.label,
        type: 'line',
        yAxisIndex: idx,
        data: xData.map((x, i) => [x, data[stream.key][i]]),
        showSymbol: false,
        lineStyle: { width: 1.5, color: stream.color },
        itemStyle: { color: stream.color },
        areaStyle: stream.key === 'altitude' && isEnabled ? { color: `${stream.color}18` } : undefined,
        connectNulls: false,
        // Visibility controlled through legend selection
      })
    })

    // Build legend selected state from enabledStreams
    const legendSelected: Record<string, boolean> = {}
    availableStreams.forEach(s => {
      legendSelected[s.label] = enabledStreams.has(s.key)
    })

    // Right margin grows with number of right-side axes
    const rightAxes = Math.max(0, enabledList.length - 1)
    const rightMargin = 12 + rightAxes * 55

    const option: echarts.EChartsCoreOption = {
      animation: false,

      legend: {
        show: true,
        top: 4,
        left: 'center',
        itemWidth: 26,
        itemHeight: 16,
        itemGap: 20,
        textStyle: { fontSize: 13, color: '#888' },
        data: legendData,
        selected: legendSelected,
      },

      grid: { left: 50, right: rightMargin, top: 30, bottom: 8 },

      brush: {
        toolbox: [],
        brushType: 'lineX',
        brushStyle: {
          borderWidth: 1,
          color: 'rgba(249,115,22,0.12)',
          borderColor: '#f97316',
        },
        throttleType: 'debounce',
        throttleDelay: 100,
      },

      tooltip: {
        trigger: 'axis',
        transitionDuration: 0,
        confine: true,
        backgroundColor: 'rgba(255,255,255,0.96)',
        borderColor: '#e5e5e5',
        borderWidth: 1,
        padding: [6, 10],
        textStyle: { fontSize: 11, color: '#333' },
        axisPointer: {
          type: 'line',
          lineStyle: { color: '#f97316', width: 1, type: 'dashed' },
          animation: false,
        },
        formatter: (params: unknown) => {
          const arr = Array.isArray(params) ? params : [params]
          if (arr.length === 0) return ''
          const first = arr[0] as { dataIndex?: number }
          if (first?.dataIndex == null) return ''
          const idx = first.dataIndex

          const kmStr = (data.distanceKm[idx] ?? 0).toFixed(1)
          const elapsed = data.elapsedMin[idx] ?? 0
          const timeStr = formatElapsed(elapsed, data.maxElapsed)
          const posLine = xMode === 'distance'
            ? `${kmStr}\u202fkm`
            : `${timeStr}\u2009\u00b7\u2009${kmStr}\u202fkm`

          let html = `<div style="line-height:1.6;min-width:110px">`
          html += `<div style="color:#888;font-size:10px;margin-bottom:1px">${posLine}</div>`

          for (const item of arr as Array<{ seriesName?: string; value?: [number, number]; color?: string }>) {
            if (!item.value || isNaN(item.value[1])) continue
            const stream = availableStreams.find(s => s.label === item.seriesName)
            if (!stream) continue
            const decimals = stream.key === 'altitude' || stream.key === 'temperature' ? 0 : 1
            html += `<div style="font-size:12px">`
            html += `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${stream.color};margin-right:4px;vertical-align:middle"></span>`
            html += `<strong>${item.value[1].toFixed(decimals)}</strong>\u2009${stream.unit}`
            html += `</div>`
          }

          return html + '</div>'
        },
      },

      xAxis: {
        type: 'value',
        min: 0,
        max: xMax,
        axisLabel: { show: false },
        splitLine: { show: false },
        axisLine: { show: false },
        axisTick: { show: false },
      },

      yAxis: yAxes,
      series,

      dataZoom: [
        {
          type: 'inside',
          xAxisIndex: 0,
          filterMode: 'none',
          zoomOnMouseWheel: true,
          moveOnMouseMove: true,
          moveOnMouseWheel: false,
        },
      ],
    }

    chart.setOption(option)

    // Activate brush cursor
    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })

    // Hover -> map marker
    chart.on('showTip', (params: unknown) => {
      const typed = params as { dataIndex?: number }
      if (typed.dataIndex != null) moveMarker(typed.dataIndex)
    })
    chart.on('hideTip', () => moveMarker(null))

    // Legend toggle -> update enabledStreams
    chart.on('legendselectchanged', (params: unknown) => {
      const typed = params as { selected: Record<string, boolean> }
      const next = new Set<FieldKey>()
      availableStreams.forEach(s => {
        if (typed.selected[s.label]) next.add(s.key)
      })
      setEnabledStreams(next)
    })

    // Brush handling (same pattern as ActivityCharts)
    type BrushArea = { range?: [number, number]; coordRange?: [number, number] }
    let pendingBrushAreas: BrushArea[] | null = null

    chart.on('brush', (params: unknown) => {
      const p = params as {
        areas?: BrushArea[]
        batch?: Array<{ areas?: BrushArea[] }>
      }
      const areas = p.areas ?? p.batch?.[0]?.areas
      if (areas && areas.length > 0) {
        pendingBrushAreas = [...areas]
      }
    })

    chart.on('brushEnd', (params: unknown) => {
      const p = params as {
        areas?: BrushArea[]
        batch?: Array<{ areas?: BrushArea[] }>
      }
      const directAreas = p.areas ?? p.batch?.[0]?.areas
      const areas = (directAreas && directAreas.length > 0) ? directAreas : pendingBrushAreas
      pendingBrushAreas = null
      if (!areas || areas.length === 0) return

      const area = areas[0]
      let startData: number
      let endData: number

      // Prefer pixel range → convertFromPixel (most reliable across ECharts versions)
      if (area.range) {
        const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
        const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
        if (s == null || e == null) return
        startData = s as number
        endData = e as number
      } else if (area.coordRange) {
        [startData, endData] = area.coordRange
      } else {
        return
      }

      if (endData <= startData) return

      const sv = Math.max(0, startData)
      const ev = Math.min(xMax, endData)

      // Use percentage-based zoom (reliable with filterMode:'none')
      const startPct = Math.max(0, (sv / xMax) * 100)
      const endPct = Math.min(100, (ev / xMax) * 100)
      if (endPct - startPct < 0.5) return // ignore tiny selections

      chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: startPct, end: endPct })
      chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: false },
      })

      setIsZoomed(true)

      // Highlight segment on map using stream indices
      const xArr = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      highlightMapSegment(findClosestIdx(xArr, sv), findClosestIdx(xArr, ev))
    })

    // Mouse-wheel / pan zoom
    chart.on('dataZoom', (params: unknown) => {
      const p = params as {
        batch?: Array<{ start: number; end: number }>
        start?: number
        end?: number
      }
      const z = p.batch?.[0] ?? (p.start != null && p.end != null ? p as { start: number; end: number } : null)
      if (z == null) return

      const zoomed = z.start > 0.5 || z.end < 99.5
      setIsZoomed(zoomed)

      if (zoomed) {
        const startVal = (z.start / 100) * xMax
        const endVal = (z.end / 100) * xMax
        highlightMapSegment(findClosestIdx(xData, startVal), findClosestIdx(xData, endVal))
      } else {
        clearMapHighlight()
      }
    })

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(chartContainerRef.current)

    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [data, xMode, enabledStreams, availableStreams]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Reset zoom handler ──────────────────────────────────────────────────
  const handleResetZoom = useCallback(() => {
    const chart = chartRef.current
    if (!chart) return

    chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })

    setIsZoomed(false)
    clearMapHighlight()
  }, [clearMapHighlight])

  // ── Switch x-mode ─────────────────────────────────────────────────────────
  const switchXMode = useCallback((mode: XMode) => {
    if (isZoomed) {
      handleResetZoom()
    }
    setXMode(mode)
  }, [isZoomed, handleResetZoom])

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-black overflow-hidden">
      {/* Map fills the space */}
      <div className="relative flex-1 min-h-0 overflow-hidden">
        <div ref={mapContainerRef} className="w-full h-full" />

        {/* Kartenstil-Tabs + Heatmap */}
        <div className="absolute top-3 left-3 z-10 flex items-center gap-1.5 flex-wrap">
          <div className="flex items-center gap-0.5 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm rounded-lg shadow-md p-0.5">
            {(Object.keys(BASE_STYLES) as BaseStyle[]).map(key => (
              <button
                key={key}
                onClick={() => setBaseStyle(key)}
                className={[
                  'px-2 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer',
                  baseStyle === key
                    ? 'bg-accent-DEFAULT text-white'
                    : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700',
                ].join(' ')}
              >
                {BASE_STYLES[key].label}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-1 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm rounded-lg shadow-md px-2 py-1">
            <button
              onClick={() => setShowHeatmap(v => !v)}
              title={showHeatmap ? 'Heatmap ausblenden' : 'Heatmap einblenden'}
              className={[
                'flex items-center gap-1.5 text-xs font-medium rounded-md px-1.5 py-0.5 transition-colors cursor-pointer',
                showHeatmap
                  ? 'text-accent-DEFAULT'
                  : 'text-neutral-600 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-white',
              ].join(' ')}
            >
              <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
                <path d="M12 2C8.5 6 6 9 6 13a6 6 0 0012 0c0-2.5-1-4.5-2.5-6.5C14.5 8.5 13.5 10 13 12c-.5-1.5-.5-3 0-5.5C12.5 5 12.2 3.5 12 2z"/>
              </svg>
              Heatmap
            </button>
            {showHeatmap && (
              <ColorPicker value={heatmapColor} onChange={handleHeatmapColorChange} />
            )}
          </div>
        </div>

        {/* Close button */}
        <button
          onClick={onClose}
          className="absolute top-3 right-14 z-10 flex items-center justify-center w-9 h-9
                     rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm
                     text-neutral-700 dark:text-neutral-200 shadow-md cursor-pointer
                     hover:bg-white dark:hover:bg-neutral-700 transition-colors"
          title="Schließen (Esc)"
        >
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-5 h-5">
            <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
          </svg>
        </button>
      </div>

      {/* Overlay chart panel at the bottom */}
      <div className="relative z-10 bg-white/80 dark:bg-neutral-900/80 backdrop-blur-sm border-t border-neutral-200/50 dark:border-neutral-700/50">
        {/* Controls bar */}
        <div className="flex items-center justify-between px-4 py-1.5">
          <div className="flex items-center gap-2">
            {isZoomed && (
              <button
                onClick={handleResetZoom}
                className="text-xs px-2.5 py-1 rounded-md bg-accent-100 text-orange-700 hover:bg-orange-200
                           dark:bg-orange-900/30 dark:text-orange-300 dark:hover:bg-orange-900/50 transition-colors"
              >
                Zoom zurücksetzen
              </button>
            )}
          </div>
          <div className="flex items-center gap-2">
            <div className="flex rounded-md border border-neutral-300 dark:border-neutral-600 overflow-hidden text-xs">
              <button
                onClick={() => switchXMode('distance')}
                className={`px-2.5 py-0.5 transition-colors ${xMode === 'distance'
                  ? 'bg-neutral-800 text-white dark:bg-neutral-200 dark:text-neutral-900'
                  : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'}`}
              >
                km
              </button>
              <button
                onClick={() => switchXMode('time')}
                className={`px-2.5 py-0.5 transition-colors border-l border-neutral-300 dark:border-neutral-600 ${xMode === 'time'
                  ? 'bg-neutral-800 text-white dark:bg-neutral-200 dark:text-neutral-900'
                  : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'}`}
              >
                Zeit
              </button>
            </div>
          </div>
        </div>

        {/* Chart */}
        <div ref={chartContainerRef} className="h-[200px] w-full [&_canvas]:!cursor-crosshair" />
      </div>
    </div>
  )
}
