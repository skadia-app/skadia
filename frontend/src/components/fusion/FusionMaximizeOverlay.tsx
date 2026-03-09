import { useEffect, useRef, useState, useCallback } from 'react'
import { createPortal } from 'react-dom'
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
import FusionMap, { type FusionMapHandle } from '@/components/fusion/FusionMap'
import type { TrackInfo } from '@/components/fusion/FusionMap'
import type { StreamData } from '@/components/fusion/FusionStreamPreview'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, BrushComponent, ToolboxComponent, CanvasRenderer])

export interface FusionMaximizeOverlayProps {
  tracks: TrackInfo[]
  streams: StreamData[]
  onClose: () => void
}

type MetricKey = 'hr' | 'power' | 'altitude' | 'speed' | 'cadence' | 'temperature'

const METRICS: Array<{
  key: MetricKey
  label: string
  unit: string
  dataKey: keyof StreamData
}> = [
  { key: 'hr',          label: 'Herzfrequenz',   unit: 'bpm',   dataKey: 'hrData' },
  { key: 'power',       label: 'Leistung',        unit: 'W',     dataKey: 'powerData' },
  { key: 'altitude',    label: 'Höhenprofil',     unit: 'm',     dataKey: 'altitudeData' as keyof StreamData },
  { key: 'speed',       label: 'Geschwindigkeit', unit: 'km/h',  dataKey: 'speedData' as keyof StreamData },
  { key: 'cadence',     label: 'Kadenz',          unit: 'rpm',   dataKey: 'cadenceData' as keyof StreamData },
  { key: 'temperature', label: 'Temperatur',      unit: '°C',    dataKey: 'temperatureData' as keyof StreamData },
]

// ---------------------------------------------------------------------------
// Inline ECharts container with zoom/brush
// ---------------------------------------------------------------------------
function EChartsContainer({ option, onHoverElapsed }: { option: EChartsCoreOption; onHoverElapsed?: (elapsed: number | null) => void }) {
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

    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })

    type BrushArea = { range?: [number, number]; coordRange?: [number, number] }
    let pendingAreas: BrushArea[] | null = null

    chart.off('brush')
    chart.off('brushEnd')
    chart.off('dataZoom')

    chart.on('brush', (params: unknown) => {
      const p = params as { areas?: BrushArea[]; batch?: Array<{ areas?: BrushArea[] }> }
      const areas = p.areas ?? p.batch?.[0]?.areas
      if (areas && areas.length > 0) pendingAreas = [...areas]
    })

    chart.on('brushEnd', (params: unknown) => {
      const p = params as { areas?: BrushArea[]; batch?: Array<{ areas?: BrushArea[] }> }
      const direct = p.areas ?? p.batch?.[0]?.areas
      const areas = (direct && direct.length > 0) ? direct : pendingAreas
      pendingAreas = null
      if (!areas || areas.length === 0) return

      const area = areas[0]
      const xAxis = (option as { xAxis?: { max?: number } }).xAxis
      const xMax = xAxis?.max ?? 1

      let startData: number, endData: number
      if (area.range) {
        const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
        const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
        if (s == null || e == null) return
        startData = s as number
        endData = e as number
      } else if (area.coordRange) {
        [startData, endData] = area.coordRange
      } else return

      if (endData <= startData) return
      const startPct = Math.max(0, (Math.max(0, startData) / xMax) * 100)
      const endPct = Math.min(100, (Math.min(xMax, endData) / xMax) * 100)
      if (endPct - startPct < 0.5) return

      chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: startPct, end: endPct })
      chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: false },
      })
      setIsZoomed(true)
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
    <div className="relative w-full h-full">
      <div ref={containerRef} className="w-full h-full [&_canvas]:!cursor-crosshair" />
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

// ---------------------------------------------------------------------------
// Main overlay
// ---------------------------------------------------------------------------
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

export default function FusionMaximizeOverlay({ tracks, streams, onClose }: FusionMaximizeOverlayProps) {
  const [activeMetric, setActiveMetric] = useState<MetricKey>('hr')
  const fusionMapRef = useRef<FusionMapHandle>(null)

  const handleHoverElapsed = useCallback((elapsed: number | null) => {
    if (elapsed === null) { fusionMapRef.current?.setHoverPoints(null); return }
    const points = streams
      .map(s => {
        const gps = findNearestGPS(s.gpsPoints, elapsed)
        return gps ? { lat: gps.lat, lng: gps.lng, color: s.color } : null
      })
      .filter((p): p is { lat: number; lng: number; color: string } => p !== null)
    fusionMapRef.current?.setHoverPoints(points.length > 0 ? points : null)
  }, [streams])

  // Escape key to close
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  // Lock body scroll
  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = prev }
  }, [])

  // Determine which metrics have any data
  const availableMetrics = METRICS.filter(m =>
    streams.some(s => {
      const data = s[m.dataKey]
      return Array.isArray(data) && (data as unknown[]).length > 0
    })
  )

  const resolvedMetric =
    availableMetrics.find(m => m.key === activeMetric) ?? availableMetrics[0]

  // Build ECharts option for selected metric
  const chartOption: EChartsCoreOption = (() => {
    if (!resolvedMetric) return {}

    const seriesData = streams
      .map(s => ({
        name: s.label,
        color: s.color,
        data: s[resolvedMetric.dataKey] as Array<[number, number]>,
      }))
      .filter(s => s.data.length > 0)

    const xMax = seriesData.reduce((m, s) => {
      const last = s.data[s.data.length - 1]
      return last ? Math.max(m, last[0]) : m
    }, 1)

    return {
      backgroundColor: 'transparent',
      animation: false,
      grid: { left: 52, right: 16, top: 28, bottom: 8, containLabel: false },
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
        backgroundColor: 'rgba(17,24,39,0.92)',
        borderColor: '#374151',
        textStyle: { color: '#f3f4f6', fontSize: 11 },
      },
      legend: {
        top: 4,
        right: 12,
        textStyle: { fontSize: 11, color: '#9ca3af' },
        itemHeight: 6,
        itemWidth: 14,
        selectedMode: false,
      },
      xAxis: { type: 'value', show: false, max: xMax },
      yAxis: {
        type: 'value',
        name: resolvedMetric.unit,
        nameTextStyle: { fontSize: 10, color: '#6b7280', padding: [0, 0, 0, -14] },
        axisLabel: { fontSize: 10, color: '#6b7280' },
        splitLine: { lineStyle: { color: '#374151', type: 'dashed' } },
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
      series: seriesData.map((s, idx) => ({
        name: s.name,
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
  })()

  const content = (
    <div className="fixed inset-0 z-50 flex flex-col bg-black overflow-hidden">
      {/* Map fills the top */}
      <div className="relative flex-1 min-h-0 overflow-hidden">
        <FusionMap ref={fusionMapRef} tracks={tracks} />

        {/* Close button — floating top-right on map */}
        <button
          onClick={onClose}
          className="absolute top-3 right-14 z-10 flex items-center justify-center w-9 h-9 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm text-neutral-700 dark:text-neutral-200 shadow-md hover:bg-white dark:hover:bg-neutral-700 transition-colors cursor-pointer"
          title="Schließen (Esc)"
        >
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-5 h-5">
            <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
          </svg>
        </button>
      </div>

      {/* Bottom chart panel — like MapFullscreen */}
      <div className="relative z-10 bg-white/90 dark:bg-neutral-900/90 backdrop-blur-sm border-t border-neutral-200/50 dark:border-neutral-700/50">
        {/* Controls: metric selector */}
        <div className="flex items-center gap-1 px-4 py-1.5 overflow-x-auto">
          {METRICS.map(m => {
            const hasData = availableMetrics.some(a => a.key === m.key)
            const isActive = resolvedMetric?.key === m.key
            return (
              <button
                key={m.key}
                onClick={() => hasData && setActiveMetric(m.key)}
                disabled={!hasData}
                className={[
                  'px-2.5 py-0.5 rounded text-xs font-medium whitespace-nowrap transition-colors',
                  isActive
                    ? 'bg-neutral-800 dark:bg-neutral-200 text-white dark:text-neutral-900 cursor-pointer'
                    : hasData
                      ? 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-800 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer'
                      : 'text-neutral-300 dark:text-neutral-600 cursor-not-allowed',
                ].join(' ')}
              >
                {m.label}
                <span className="ml-1 opacity-50">{m.unit}</span>
              </button>
            )
          })}
        </div>

        {/* Chart */}
        <div className="h-[200px] w-full">
          {resolvedMetric ? (
            <EChartsContainer option={chartOption} onHoverElapsed={handleHoverElapsed} />
          ) : (
            <div className="flex items-center justify-center h-full text-neutral-400 text-sm">
              Keine Zeitreihendaten verfügbar
            </div>
          )}
        </div>
      </div>
    </div>
  )

  return createPortal(content, document.body)
}
