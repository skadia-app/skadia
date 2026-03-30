import { useRef, useEffect, useState, useCallback } from 'react'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  BrushComponent,
  ToolboxComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, BrushComponent, ToolboxComponent, CanvasRenderer])

export interface StreamData {
  activityId: string
  label: string
  color: string
  hrData: Array<[number, number]>          // [elapsed_s, hr]
  powerData: Array<[number, number]>        // [elapsed_s, power]
  altitudeData: Array<[number, number]>     // [elapsed_s, altitude_m]
  speedData: Array<[number, number]>        // [elapsed_s, speed_km_h]
  cadenceData: Array<[number, number]>      // [elapsed_s, cadence_rpm]
  temperatureData: Array<[number, number]>  // [elapsed_s, temp_c]
  gpsPoints: Array<{ elapsed: number; lat: number; lng: number }>
}

export interface HoverGpsPoint { lat: number; lng: number; color: string }

interface FusionStreamPreviewProps {
  streams: StreamData[]
  onHoverGps?: (points: HoverGpsPoint[] | null) => void
}

type MetricKey = 'hr' | 'power' | 'altitude' | 'speed' | 'cadence' | 'temperature'

const METRICS: Array<{ key: MetricKey; label: string; unit: string; dataKey: keyof StreamData }> = [
  { key: 'hr',          label: 'Herzfrequenz', unit: 'bpm',  dataKey: 'hrData' },
  { key: 'power',       label: 'Leistung',     unit: 'W',    dataKey: 'powerData' },
  { key: 'altitude',    label: 'Höhe',         unit: 'm',    dataKey: 'altitudeData' },
  { key: 'speed',       label: 'Speed',        unit: 'km/h', dataKey: 'speedData' },
  { key: 'cadence',     label: 'Kadenz',       unit: 'rpm',  dataKey: 'cadenceData' },
  { key: 'temperature', label: 'Temperatur',   unit: '°C',   dataKey: 'temperatureData' },
]

function buildChartOption(
  data: Array<{ label: string; color: string; data: Array<[number, number]> }>,
  unit: string,
): EChartsCoreOption {
  return {
    backgroundColor: 'transparent',
    animation: false,
    grid: { left: 48, right: 12, top: 28, bottom: 8, containLabel: false },
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
    toolbox: {
      feature: {
        brush: { show: false },
        saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2 },
      },
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(17,24,39,0.92)',
      borderColor: '#374151',
      textStyle: { color: '#f3f4f6', fontSize: 11 },
    },
    legend: {
      top: 4,
      right: 8,
      textStyle: { fontSize: 10, color: '#9ca3af' },
      itemHeight: 6,
      itemWidth: 12,
      selectedMode: false,
    },
    xAxis: {
      type: 'value',
      show: false,
      max: data.reduce((m, s) => {
        const last = s.data[s.data.length - 1]
        return last ? Math.max(m, last[0]) : m
      }, 1),
    },
    yAxis: {
      type: 'value',
      name: unit,
      nameTextStyle: { fontSize: 9, color: '#6b7280', padding: [0, 0, 0, -12] },
      axisLabel: { fontSize: 9, color: '#6b7280' },
      splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
      scale: true,
    },
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
    series: data.map((s, idx) => ({
      name: s.label,
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: s.data,
      lineStyle: {
        color: s.color,
        width: idx === 0 ? 2.5 : idx === 1 ? 1.5 : 1,
        type: idx === 0 ? 'solid' : idx === 1 ? 'dashed' : 'dotted',
      },
      itemStyle: { color: s.color },
      sampling: 'lttb',
      emphasis: { focus: 'none' },
      blur: {
        lineStyle: { opacity: 1 },
        itemStyle: { opacity: 1 },
      },
    })),
  }
}

function EChartsContainer({
  option,
  height = 140,
  onHoverElapsed,
}: {
  option: EChartsCoreOption
  height?: number
  onHoverElapsed?: (elapsed: number | null) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)
  const [isZoomed, setIsZoomed] = useState(false)
  const onHoverElapsedRef = useRef(onHoverElapsed)
  useEffect(() => { onHoverElapsedRef.current = onHoverElapsed }, [onHoverElapsed])

  useEffect(() => {
    if (!containerRef.current) return
    if (chartRef.current) chartRef.current.dispose()
    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    // Hover → GPS marker
    const container = containerRef.current
    const handleMouseMove = (e: MouseEvent) => {
      const c = chartRef.current
      if (!c || !onHoverElapsedRef.current) return
      const rect = container.getBoundingClientRect()
      const elapsed = c.convertFromPixel({ xAxisIndex: 0 }, e.clientX - rect.left)
      if (elapsed != null) onHoverElapsedRef.current(elapsed as number)
    }
    const handleMouseLeave = () => onHoverElapsedRef.current?.(null)
    container.addEventListener('mousemove', handleMouseMove)
    container.addEventListener('mouseleave', handleMouseLeave)

    return () => {
      container.removeEventListener('mousemove', handleMouseMove)
      container.removeEventListener('mouseleave', handleMouseLeave)
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const chart = chartRef.current
    if (!chart) return
    chart.setOption(option, { notMerge: true })

    // Activate brush cursor after each option update
    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })

    type BrushArea = { range?: [number, number]; coordRange?: [number, number] }
    let brushZoomLock = false

    chart.off('brush')
    chart.off('brushEnd')
    chart.off('dataZoom')

    chart.on('brushEnd', (params: unknown) => {
      if (brushZoomLock) return
      brushZoomLock = true

      const p = params as { areas?: BrushArea[]; batch?: Array<{ areas?: BrushArea[] }> }
      const areas = p.areas ?? p.batch?.[0]?.areas
      if (!areas || areas.length === 0) { brushZoomLock = false; return }

      const area = areas[0]
      const xAxis = (option as { xAxis?: { max?: number } }).xAxis
      const xMax = xAxis?.max ?? 1

      let startData: number, endData: number
      if (area.coordRange) {
        [startData, endData] = area.coordRange
      } else if (area.range) {
        const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
        const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
        if (s == null || e == null) { brushZoomLock = false; return }
        startData = s as number
        endData = e as number
      } else { brushZoomLock = false; return }

      if (endData <= startData) { brushZoomLock = false; return }
      const startPct = Math.max(0, (Math.max(0, startData) / xMax) * 100)
      const endPct = Math.min(100, (Math.min(xMax, endData) / xMax) * 100)
      if (endPct - startPct < 0.5) { brushZoomLock = false; return }

      chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: startPct, end: endPct })
      chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: false },
      })
      setIsZoomed(true)
      requestAnimationFrame(() => { brushZoomLock = false })
    })

    chart.on('dataZoom', (params: unknown) => {
      const p = params as { batch?: Array<{ start: number; end: number }>; start?: number; end?: number }
      const z = p.batch?.[0] ?? (p.start != null && p.end != null ? p as { start: number; end: number } : null)
      if (z == null) return
      setIsZoomed(z.start > 0.5 || z.end < 99.5)
    })
  }, [option])

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
  }, [])

  return (
    <div className="relative">
      <div ref={containerRef} style={{ height }} className="w-full [&_canvas]:!cursor-crosshair" />
      {isZoomed && (
        <button
          onClick={handleResetZoom}
          className="absolute top-1 right-1 px-1.5 py-0.5 rounded text-[10px] bg-neutral-800/80 text-neutral-200 hover:bg-neutral-700 cursor-pointer"
        >
          Zoom zurücksetzen
        </button>
      )}
    </div>
  )
}

function findNearestGPS(gpsPoints: Array<{ elapsed: number; lat: number; lng: number }>, elapsed: number) {
  if (gpsPoints.length === 0) return null
  let lo = 0, hi = gpsPoints.length - 1
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (gpsPoints[mid].elapsed < elapsed) lo = mid + 1
    else hi = mid
  }
  if (lo > 0 && Math.abs(gpsPoints[lo - 1].elapsed - elapsed) < Math.abs(gpsPoints[lo].elapsed - elapsed)) lo--
  return gpsPoints[lo]
}

export default function FusionStreamPreview({ streams, onHoverGps }: FusionStreamPreviewProps) {
  const available = METRICS.filter(m =>
    streams.some(s => (s[m.dataKey] as Array<unknown>).length > 0)
  )

  const [selected, setSelected] = useState<MetricKey | null>(null)
  const activeMetric = available.find(m => m.key === selected) ?? available[0] ?? null

  const handleHoverElapsed = useCallback((elapsed: number | null) => {
    if (!onHoverGps) return
    if (elapsed === null) { onHoverGps(null); return }
    const points = streams
      .map(s => {
        const gps = findNearestGPS(s.gpsPoints, elapsed)
        return gps ? { lat: gps.lat, lng: gps.lng, color: s.color } : null
      })
      .filter((p): p is HoverGpsPoint => p !== null)
    onHoverGps(points.length > 0 ? points : null)
  }, [streams, onHoverGps])

  if (available.length === 0) {
    return (
      <div className="flex items-center justify-center h-32 text-neutral-400 text-xs rounded-xl border border-neutral-200 dark:border-neutral-800">
        Keine Zeitreihendaten verfügbar
      </div>
    )
  }

  const activeData = activeMetric
    ? streams
        .map(s => ({
          label: s.label,
          color: s.color,
          data: s[activeMetric.dataKey] as Array<[number, number]>,
        }))
        .filter(s => s.data.length > 0)
    : []

  const chartOption = activeMetric ? buildChartOption(activeData, activeMetric.unit) : {}

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 space-y-2">
      {/* Header */}
      <div className="flex items-center justify-between">
        <p className="text-xs font-semibold text-neutral-500">Stream-Vorschau</p>
        <span className="flex items-center gap-1 text-xs text-neutral-400">
          <svg xmlns="http://www.w3.org/2000/svg" className="h-3 w-3" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M3 4a1 1 0 011-1h4a1 1 0 010 2H6.414l2.293 2.293a1 1 0 01-1.414 1.414L5 6.414V8a1 1 0 01-2 0V4zm9 1a1 1 0 010-2h4a1 1 0 011 1v4a1 1 0 01-2 0V6.414l-2.293 2.293a1 1 0 11-1.414-1.414L13.586 5H12zm-9 7a1 1 0 012 0v1.586l2.293-2.293a1 1 0 011.414 1.414L6.414 15H8a1 1 0 010 2H4a1 1 0 01-1-1v-4zm13-1a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 010-2h1.586l-2.293-2.293a1 1 0 011.414-1.414L15 13.586V12a1 1 0 011-1z" clipRule="evenodd" />
          </svg>
          Karte &amp; Streams maximieren
        </span>
      </div>

      {/* Metrik-Tabs */}
      <div className="flex flex-wrap gap-1">
        {available.map(m => (
          <button
            key={m.key}
            onClick={() => setSelected(m.key)}
            className={`px-2 py-0.5 rounded text-xs font-medium transition-colors cursor-pointer ${
              m.key === activeMetric?.key
                ? 'bg-neutral-800 dark:bg-neutral-200 text-white dark:text-neutral-900'
                : 'text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300'
            }`}
          >
            {m.label}
          </button>
        ))}
      </div>

      {/* Chart */}
      {activeMetric && <EChartsContainer option={chartOption} height={130} onHoverElapsed={handleHoverElapsed} />}
    </div>
  )
}
