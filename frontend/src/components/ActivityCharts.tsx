import { useRef, useEffect, useMemo, useCallback, useState } from 'react'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  AxisPointerComponent,
  BrushComponent,
  MarkLineComponent,
  ToolboxComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { StreamPoint } from '@/api/types'

echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  AxisPointerComponent,
  BrushComponent,
  MarkLineComponent,
  ToolboxComponent,
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

// ── Data processing ────────────────────────────────────────────────────────────

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

function calcAltitudeYRange(values: number[]): { min: number; max: number } | undefined {
  const valid = values.filter(v => !isNaN(v))
  if (valid.length === 0) return undefined
  const actualMin = Math.min(...valid)
  const actualMax = Math.max(...valid)
  const range = actualMax - actualMin
  const padding = Math.max(5, range * 0.12)
  let yMin = actualMin - padding
  let yMax = actualMax + padding
  const minRange = 50
  if (yMax - yMin < minRange) {
    const center = (yMin + yMax) / 2
    yMin = center - minRange / 2
    yMax = center + minRange / 2
  }
  return { min: Math.floor(yMin), max: Math.ceil(yMax) }
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

function formatElapsed(minutes: number, maxMinutes: number): string {
  if (maxMinutes < 60) return `${Math.round(minutes)}\u202fmin`
  const h = Math.floor(minutes / 60)
  const m = Math.round(minutes % 60)
  if (h === 0) return `${m}\u202fmin`
  return `${h}h${m.toString().padStart(2, '0')}`
}

// ── Exported types ─────────────────────────────────────────────────────────────

export interface ZoomRange { startKm: number; endKm: number }

interface Props {
  streams: StreamPoint[]
  onHover: (index: number | null) => void
  onZoomChange?: (range: ZoomRange | null) => void
}

// ── Main component ─────────────────────────────────────────────────────────────

export default function ActivityCharts({ streams, onHover, onZoomChange }: Props) {
  const data = useMemo(() => buildChartData(streams), [streams])
  const [isZoomed, setIsZoomed] = useState(false)
  const [xMode, setXMode] = useState<XMode>('distance')

  const chartInstancesRef = useRef<Map<string, echarts.ECharts>>(new Map())

  const registerChart = useCallback((id: string, chart: echarts.ECharts) => {
    chartInstancesRef.current.set(id, chart)
    chart.group = 'activity-charts'
    echarts.connect('activity-charts')
  }, [])

  const unregisterChart = useCallback((id: string) => {
    chartInstancesRef.current.delete(id)
  }, [])

  // Clear brushes on every chart except the one currently being drawn on.
  // This prevents multiple independent selections from appearing simultaneously.
  const clearOtherBrushes = useCallback((sourceChartId: string) => {
    chartInstancesRef.current.forEach((chart, id) => {
      if (id !== sourceChartId) {
        chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
      }
    })
  }, [])

  // Latest brush-zoom handler, kept current via a ref so ChartPanel closures
  // always call the most recent version (avoids stale-closure with xMode/data).
  const handleBrushZoomFn = useCallback((startPct: number, endPct: number) => {
    // Derive absolute values for current mode
    const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
    const startVal = (startPct / 100) * xMax
    const endVal = (endPct / 100) * xMax

    // Convert to km for the map (in time mode we look up the nearest point)
    let startKm: number, endKm: number
    if (xMode === 'distance') {
      startKm = startVal
      endKm = endVal
    } else {
      const si = findClosestIdx(data.elapsedMin, startVal)
      const ei = findClosestIdx(data.elapsedMin, endVal)
      startKm = data.distanceKm[si] ?? 0
      endKm = data.distanceKm[ei] ?? data.maxDist
    }

    // Zoom all charts to the selected range (percentage-based is most reliable)
    chartInstancesRef.current.forEach(chart => {
      chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: startPct, end: endPct })
      chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
    })
    setIsZoomed(true)
    onZoomChange?.({ startKm, endKm })
  }, [xMode, data, onZoomChange])

  const brushHandlerRef = useRef(handleBrushZoomFn)
  useEffect(() => { brushHandlerRef.current = handleBrushZoomFn }, [handleBrushZoomFn])
  const stableBrushZoom = useCallback((s: number, e: number) => brushHandlerRef.current(s, e), [])

  const handleResetZoom = useCallback(() => {
    chartInstancesRef.current.forEach(chart => {
      chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
    })
    setIsZoomed(false)
    onZoomChange?.(null)
  }, [onZoomChange])

  const switchXMode = useCallback((mode: XMode) => {
    if (isZoomed) {
      chartInstancesRef.current.forEach(chart => {
        chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
      })
      setIsZoomed(false)
      onZoomChange?.(null)
    }
    setXMode(mode)
  }, [isZoomed, onZoomChange])

  if (streams.length === 0) return null

  // xMode in the key forces a clean remount when mode is toggled
  const k = (id: string) => `${id}-${xMode}`

  const common = {
    data, onHover,
    onBrushZoom: stableBrushZoom,
    onClearOtherBrushes: clearOtherBrushes,
    onRegister: registerChart,
    onUnregister: unregisterChart,
    xMode,
  }

  const panels: React.ReactNode[] = []

  if (hasValidData(data.altitude)) panels.push(
    <ChartPanel key={k('altitude')} chartId="altitude" {...common}
      field="altitude" label="Höhe" unit="m" color="#22c55e" area
      markLines="minmax" altitudeSmartY showGradient />,
  )
  if (hasValidData(data.heartRate)) panels.push(
    <ChartPanel key={k('hr')} chartId="hr" {...common}
      field="heartRate" label="Herzfrequenz" unit="bpm" color="#ef4444"
      markLines="max" />,
  )
  if (hasValidData(data.power)) panels.push(
    <ChartPanel key={k('power')} chartId="power" {...common}
      field="power" label="Leistung" unit="W" color="#a855f7" />,
  )
  if (hasValidData(data.cadence)) panels.push(
    <ChartPanel key={k('cadence')} chartId="cadence" {...common}
      field="cadence" label="Kadenz" unit="rpm" color="#f97316" />,
  )
  if (hasValidData(data.speed)) panels.push(
    <ChartPanel key={k('speed')} chartId="speed" {...common}
      field="speed" label="Geschwindigkeit" unit="km/h" color="#3b82f6" />,
  )
  if (hasValidData(data.temperature)) panels.push(
    <ChartPanel key={k('temperature')} chartId="temperature" {...common}
      field="temperature" label="Temperatur" unit="°C" color="#06b6d4"
      markLines="minmax" />,
  )

  if (panels.length === 0) return null

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Datenströme</h2>
        <div className="flex items-center gap-2">
          {isZoomed && (
            <button onClick={handleResetZoom}
              className="text-sm px-3 py-1 rounded-md bg-orange-100 text-orange-700 hover:bg-orange-200
                         dark:bg-orange-900/30 dark:text-orange-300 dark:hover:bg-orange-900/50 transition-colors">
              ✕ Zoom zurücksetzen
            </button>
          )}
          <div className="flex rounded-lg border border-neutral-200 dark:border-neutral-700 overflow-hidden text-sm">
            <button onClick={() => switchXMode('distance')}
              className={`px-3 py-1 transition-colors ${xMode === 'distance'
                ? 'bg-neutral-800 text-white dark:bg-neutral-200 dark:text-neutral-900'
                : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'}`}>
              km
            </button>
            <button onClick={() => switchXMode('time')}
              className={`px-3 py-1 transition-colors border-l border-neutral-200 dark:border-neutral-700 ${xMode === 'time'
                ? 'bg-neutral-800 text-white dark:bg-neutral-200 dark:text-neutral-900'
                : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'}`}>
              Zeit
            </button>
          </div>
        </div>
      </div>
      {panels}
      <p className="text-xs text-neutral-400 dark:text-neutral-600">
        Ziehen zum Auswählen · Mausrad zum Zoomen
      </p>
    </div>
  )
}

// ── Individual Chart Panel ─────────────────────────────────────────────────────

interface ChartPanelProps {
  chartId: string
  data: ChartData
  field: FieldKey
  label: string
  unit: string
  color: string
  area?: boolean
  onHover: (index: number | null) => void
  onBrushZoom: (startPct: number, endPct: number) => void
  onClearOtherBrushes: (sourceChartId: string) => void
  onRegister: (id: string, chart: echarts.ECharts) => void
  onUnregister: (id: string) => void
  markLines?: 'none' | 'max' | 'minmax'
  altitudeSmartY?: boolean
  showGradient?: boolean
  xMode: XMode
}

function ChartPanel({
  chartId, data, field, unit, label, color, area,
  onHover, onBrushZoom, onClearOtherBrushes, onRegister, onUnregister,
  markLines = 'none', altitudeSmartY, showGradient, xMode,
}: ChartPanelProps) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    onRegister(chartId, chart)

    const values = data[field]
    const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
    const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

    // ── Y-axis smart scaling ─────────────────────────────────────────────────
    const yAxisExtra: Record<string, unknown> = {}
    if (altitudeSmartY) {
      const range = calcAltitudeYRange(values)
      if (range) { yAxisExtra.min = range.min; yAxisExtra.max = range.max }
    }

    // ── Mark lines ──────────────────────────────────────────────────────────
    type MarkLineEntry = {
      type: 'min' | 'max'
      name: string
      label: { position: string }
    }
    const mlData: MarkLineEntry[] = []
    if (markLines === 'max' || markLines === 'minmax') {
      // Max line: label goes BELOW the line (inside chart body) to avoid top clipping
      mlData.push({ type: 'max', name: 'Max', label: { position: 'insideMiddleBottom' } })
    }
    if (markLines === 'minmax') {
      // Min line: label goes ABOVE the line (inside chart body) since data is above
      mlData.push({ type: 'min', name: 'Min', label: { position: 'insideMiddleTop' } })
    }

    // ── Tooltip builder ─────────────────────────────────────────────────────
    const decimals = field === 'altitude' || field === 'temperature' ? 0 : 1

    const buildTooltip = (idx: number): string => {
      const v = values[idx]
      if (isNaN(v)) return ''
      const kmStr = (data.distanceKm[idx] ?? 0).toFixed(1)
      const elapsed = data.elapsedMin[idx] ?? 0
      const timeStr = formatElapsed(elapsed, data.maxElapsed)
      const posLine = xMode === 'distance'
        ? `${kmStr}\u202fkm`
        : `${timeStr}\u2009·\u2009${kmStr}\u202fkm`

      let html = `<div style="line-height:1.75;min-width:130px">`
      html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
      html += `<div style="font-size:13px"><strong>${v.toFixed(decimals)}</strong>\u2009${unit}</div>`

      if (showGradient && idx > 0 && idx < values.length - 1) {
        const prevAlt = values[idx - 1]
        const nextAlt = values[idx + 1]
        const dDist = ((data.distanceKm[idx + 1] ?? 0) - (data.distanceKm[idx - 1] ?? 0)) * 1000
        if (dDist > 0 && !isNaN(prevAlt) && !isNaN(nextAlt)) {
          const pct = ((nextAlt - prevAlt) / dDist) * 100
          const sign = pct >= 0 ? '+' : ''
          const gc = Math.abs(pct) > 10 ? '#dc2626' : Math.abs(pct) > 4 ? '#ea580c' : '#16a34a'
          html += `<div style="color:${gc};font-size:12px;margin-top:1px">Steigung\u2009${sign}${pct.toFixed(1)}\u202f%</div>`
        }
      }
      return html + '</div>'
    }

    // ── X-axis formatter ────────────────────────────────────────────────────
    const xFormatter = xMode === 'distance'
      ? (v: number) => v.toFixed(1)
      : (v: number) => formatElapsed(v, data.maxElapsed)

    // ── ECharts option ───────────────────────────────────────────────────────
    const option: echarts.EChartsCoreOption = {
      animation: false,
      grid: { left: 52, right: 76, top: 8, bottom: 36 },

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
        padding: [8, 10],
        textStyle: { fontSize: 12, color: '#333' },
        axisPointer: {
          type: 'line',
          lineStyle: { color: '#f97316', width: 1, type: 'dashed' },
          animation: false,
        },
        formatter: (params: unknown) => {
          const arr = Array.isArray(params) ? params : [params]
          const p = arr[0] as { dataIndex?: number }
          if (p?.dataIndex == null) return ''
          return buildTooltip(p.dataIndex)
        },
      },

      xAxis: {
        type: 'value',
        min: 0,
        max: xMax,
        axisLabel: { fontSize: 11, color: '#666', formatter: xFormatter },
        splitLine: { show: false },
        axisLine: { lineStyle: { color: '#ddd' } },
        axisTick: { lineStyle: { color: '#ddd' } },
      },

      yAxis: {
        type: 'value',
        axisLabel: { fontSize: 11, color: '#666', margin: 8 },
        splitLine: { lineStyle: { color: '#eee', type: 'dashed' } },
        axisLine: { show: false },
        axisTick: { show: false },
        ...yAxisExtra,
      },

      series: [
        {
          type: 'line',
          data: xData.map((x, i) => [x, values[i]]),
          showSymbol: false,
          lineStyle: { width: 1.5, color },
          itemStyle: { color },
          areaStyle: area ? { color: `${color}20` } : undefined,
          connectNulls: false,
          // Note: 'large' mode is intentionally NOT used here — it breaks
          // programmatic dataZoom dispatch and leaves charts blank after zoom.
          sampling: 'lttb',
          ...(mlData.length > 0 ? {
            markLine: {
              silent: true,
              symbol: ['none', 'none'],
              label: {
                show: true,
                // per-item label.position overrides this; this is the base style
                backgroundColor: 'rgba(255,255,255,0.88)',
                padding: [2, 5],
                borderRadius: 3,
                formatter: (p: { value: number; name: string }) =>
                  `${p.name}: ${p.value.toFixed(decimals)}\u2009${unit}`,
                color: '#444',
                fontSize: 11,
              },
              lineStyle: { type: 'dashed', color: '#ccc', width: 1 },
              data: mlData,
            },
          } : {}),
        },
      ],

      dataZoom: [
        {
          type: 'inside',
          xAxisIndex: 0,
          // 'none': only the axis range changes, data is never filtered.
          // Required for correct rendering after programmatic zoom dispatch
          // (filterMode 'weakFilter'/'filter' conflicts with LTTB sampling).
          filterMode: 'none',
          zoomOnMouseWheel: true,
          moveOnMouseMove: true,
          moveOnMouseWheel: false,
        },
      ],
    }

    chart.setOption(option)

    // Activate brush cursor so click-drag draws a range selection
    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })

    // Hover → map marker
    chart.on('showTip', (params: unknown) => {
      const typed = params as { dataIndex?: number }
      if (typed.dataIndex != null) onHover(typed.dataIndex)
    })
    chart.on('hideTip', () => onHover(null))

    // Brush draw started on THIS chart → clear all other charts' selections
    // so only one selection rectangle is ever visible at a time.
    chart.on('brush', (params: unknown) => {
      const typed = params as { batch?: Array<{ areas?: Array<unknown> }> }
      const areas = typed.batch?.[0]?.areas
      if (areas && areas.length > 0) {
        onClearOtherBrushes(chartId)
      }
    })

    // Brush selection complete → zoom all charts.
    // ECharts sends areas[].range in PIXEL coordinates, not data coordinates.
    // Use convertFromPixel to map pixel → data value (km or minutes).
    chart.on('brushEnd', (params: unknown) => {
      const typed = params as {
        areas?: Array<{
          range?: [number, number]      // pixel coords (what ECharts actually sends)
          coordRange?: [number, number] // data coords (documented but not always present)
        }>
      }
      const areas = typed.areas
      if (!areas || areas.length === 0) return

      const area = areas[0]
      let startData: number, endData: number

      if (area.coordRange) {
        // Data coordinates available directly
        startData = area.coordRange[0]
        endData = area.coordRange[1]
      } else if (area.range) {
        // Convert pixel range to data values via ECharts API
        const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
        const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
        if (s == null || e == null) return
        startData = s as number
        endData = e as number
      } else {
        return
      }

      if (endData <= startData) return

      // Convert to percentages for reliable cross-chart dataZoom dispatch
      const startPct = Math.max(0, (startData / xMax) * 100)
      const endPct = Math.min(100, (endData / xMax) * 100)
      onBrushZoom(startPct, endPct)
    })

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    return () => {
      observer.disconnect()
      onUnregister(chartId)
      chart.dispose()
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, field, xMode])

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: color }} />
        <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
          {label}{' '}
          <span className="font-normal text-neutral-400 dark:text-neutral-600">({unit})</span>
        </span>
      </div>
      <div ref={containerRef} className="h-[180px] w-full" />
    </div>
  )
}
