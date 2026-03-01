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
  CanvasRenderer,
])

// ── Types ─────────────────────────────────────────────────────────────────────

type FieldKey = 'altitude' | 'heartRate' | 'power' | 'cadence' | 'speed' | 'temperature'

interface ChartData {
  distanceKm: number[]
  altitude: number[]
  heartRate: number[]
  power: number[]
  cadence: number[]
  speed: number[]
  temperature: number[]
  maxDist: number
}

// ── Data processing ───────────────────────────────────────────────────────────

function buildChartData(streams: StreamPoint[]): ChartData {
  const distanceKm: number[] = []
  const altitude: number[] = []
  const heartRate: number[] = []
  const power: number[] = []
  const cadence: number[] = []
  const speed: number[] = []
  const temperature: number[] = []

  for (const s of streams) {
    distanceKm.push(
      s.distance_m != null
        ? s.distance_m / 1000
        : distanceKm.length > 0 ? distanceKm[distanceKm.length - 1] : 0,
    )
    altitude.push(s.altitude_m ?? NaN)
    heartRate.push(s.heart_rate ?? NaN)
    power.push(s.power_w ?? NaN)
    cadence.push(s.cadence ?? NaN)
    speed.push(s.speed_ms != null ? s.speed_ms * 3.6 : NaN)
    temperature.push(s.temperature_c ?? NaN)
  }

  const validDist = distanceKm.filter(v => !isNaN(v) && isFinite(v))
  const maxDist = validDist.length > 0 ? Math.max(...validDist) : 1

  return { distanceKm, altitude, heartRate, power, cadence, speed, temperature, maxDist }
}

function hasValidData(arr: number[]): boolean {
  return arr.some(v => !isNaN(v))
}

// Smart Y-axis for altitude: tight to actual data range but with a minimum 50m
// window so flat rides don't look mountainous.
function calcAltitudeYRange(values: number[]): { min: number; max: number } | undefined {
  const valid = values.filter(v => !isNaN(v))
  if (valid.length === 0) return undefined
  const actualMin = Math.min(...valid)
  const actualMax = Math.max(...valid)
  const range = actualMax - actualMin
  const padding = Math.max(5, range * 0.12)
  let yMin = actualMin - padding
  let yMax = actualMax + padding
  // Enforce a minimum visible range so flat rides stay flat
  const minRange = 50
  if (yMax - yMin < minRange) {
    const center = (yMin + yMax) / 2
    yMin = center - minRange / 2
    yMax = center + minRange / 2
  }
  return { min: Math.floor(yMin), max: Math.ceil(yMax) }
}

// ── Main component ────────────────────────────────────────────────────────────

export interface ZoomRange {
  startKm: number
  endKm: number
}

interface Props {
  streams: StreamPoint[]
  onHover: (index: number | null) => void
  onZoomChange?: (range: ZoomRange | null) => void
}

export default function ActivityCharts({ streams, onHover, onZoomChange }: Props) {
  // Memoize chart data so chart panels don't reinitialise on unrelated parent re-renders
  // (e.g. hoverIndex state changes in ActivityDetailPage).
  const data = useMemo(() => buildChartData(streams), [streams])
  const [isZoomed, setIsZoomed] = useState(false)

  // Registry of live ECharts instances for manual cross-chart dispatch
  const chartInstancesRef = useRef<Map<string, echarts.ECharts>>(new Map())

  const registerChart = useCallback((id: string, chart: echarts.ECharts) => {
    chartInstancesRef.current.set(id, chart)
    // Connect all charts so the axis pointer syncs visually across all panels
    chart.group = 'activity-charts'
    echarts.connect('activity-charts')
  }, [])

  const unregisterChart = useCallback((id: string) => {
    chartInstancesRef.current.delete(id)
  }, [])

  // Apply brush selection as a dataZoom on all charts and notify parent
  const handleBrushZoom = useCallback((startKm: number, endKm: number) => {
    chartInstancesRef.current.forEach(chart => {
      chart.dispatchAction({
        type: 'dataZoom',
        dataZoomIndex: 0,
        startValue: startKm,
        endValue: endKm,
      })
      // Clear the brush overlay after applying zoom
      chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
    })
    setIsZoomed(true)
    onZoomChange?.({ startKm, endKm })
  }, [onZoomChange])

  // Reset all charts to full view
  const handleResetZoom = useCallback(() => {
    chartInstancesRef.current.forEach(chart => {
      chart.dispatchAction({
        type: 'dataZoom',
        dataZoomIndex: 0,
        start: 0,
        end: 100,
      })
    })
    setIsZoomed(false)
    onZoomChange?.(null)
  }, [onZoomChange])

  if (streams.length === 0) return null

  const panels: React.ReactNode[] = []

  if (hasValidData(data.altitude)) {
    panels.push(
      <ChartPanel
        key="altitude"
        chartId="altitude"
        data={data}
        field="altitude"
        label="Höhe (m)"
        color="#22c55e"
        area
        onHover={onHover}
        onBrushZoom={handleBrushZoom}
        onRegister={registerChart}
        onUnregister={unregisterChart}
        markLines="minmax"
        altitudeSmartY
        showGradient
      />,
    )
  }

  if (hasValidData(data.heartRate)) {
    panels.push(
      <ChartPanel
        key="hr"
        chartId="hr"
        data={data}
        field="heartRate"
        label="HR (bpm)"
        color="#ef4444"
        onHover={onHover}
        onBrushZoom={handleBrushZoom}
        onRegister={registerChart}
        onUnregister={unregisterChart}
        markLines="max"
      />,
    )
  }

  if (hasValidData(data.power)) {
    panels.push(
      <ChartPanel
        key="power"
        chartId="power"
        data={data}
        field="power"
        label="Leistung (W)"
        color="#a855f7"
        onHover={onHover}
        onBrushZoom={handleBrushZoom}
        onRegister={registerChart}
        onUnregister={unregisterChart}
      />,
    )
  }

  if (hasValidData(data.cadence)) {
    panels.push(
      <ChartPanel
        key="cadence"
        chartId="cadence"
        data={data}
        field="cadence"
        label="Kadenz (rpm)"
        color="#f97316"
        onHover={onHover}
        onBrushZoom={handleBrushZoom}
        onRegister={registerChart}
        onUnregister={unregisterChart}
      />,
    )
  }

  if (hasValidData(data.speed)) {
    panels.push(
      <ChartPanel
        key="speed"
        chartId="speed"
        data={data}
        field="speed"
        label="Geschw. (km/h)"
        color="#3b82f6"
        onHover={onHover}
        onBrushZoom={handleBrushZoom}
        onRegister={registerChart}
        onUnregister={unregisterChart}
      />,
    )
  }

  if (hasValidData(data.temperature)) {
    panels.push(
      <ChartPanel
        key="temperature"
        chartId="temperature"
        data={data}
        field="temperature"
        label="Temp. (°C)"
        color="#06b6d4"
        onHover={onHover}
        onBrushZoom={handleBrushZoom}
        onRegister={registerChart}
        onUnregister={unregisterChart}
        markLines="minmax"
      />,
    )
  }

  if (panels.length === 0) return null

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
          Datenströme
        </h2>
        {isZoomed && (
          <button
            onClick={handleResetZoom}
            className="text-sm px-3 py-1 rounded-md bg-orange-100 text-orange-700 hover:bg-orange-200
                       dark:bg-orange-900/30 dark:text-orange-300 dark:hover:bg-orange-900/50 transition-colors"
          >
            ✕ Zoom zurücksetzen
          </button>
        )}
      </div>
      {panels}
      <p className="text-xs text-neutral-400 dark:text-neutral-600">
        Klicken und ziehen zum Einzoomen · Mausrad scrollen zum Verschieben
      </p>
    </div>
  )
}

// ── Individual Chart Panel ────────────────────────────────────────────────────

interface ChartPanelProps {
  chartId: string
  data: ChartData
  field: FieldKey
  label: string
  color: string
  area?: boolean
  onHover: (index: number | null) => void
  onBrushZoom: (startKm: number, endKm: number) => void
  onRegister: (id: string, chart: echarts.ECharts) => void
  onUnregister: (id: string) => void
  markLines?: 'none' | 'max' | 'minmax'
  altitudeSmartY?: boolean
  showGradient?: boolean
}

function ChartPanel({
  chartId, data, field, label, color, area,
  onHover, onBrushZoom, onRegister, onUnregister,
  markLines = 'none', altitudeSmartY, showGradient,
}: ChartPanelProps) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    onRegister(chartId, chart)

    const values = data[field]
    const xData = data.distanceKm

    // ── Y-axis ──────────────────────────────────────────────────────────────
    const yAxisExtra: Record<string, unknown> = {}
    if (altitudeSmartY) {
      const range = calcAltitudeYRange(values)
      if (range) {
        yAxisExtra.min = range.min
        yAxisExtra.max = range.max
      }
    }

    // ── Mark lines (min/max) ─────────────────────────────────────────────────
    type MarkLineEntry = { type: 'min' | 'max'; name: string }
    const mlData: MarkLineEntry[] = []
    if (markLines === 'max' || markLines === 'minmax') mlData.push({ type: 'max', name: 'Max' })
    if (markLines === 'minmax') mlData.push({ type: 'min', name: 'Min' })

    // ── Tooltip formatter ───────────────────────────────────────────────────
    const unit = label.match(/\(([^)]+)\)/)?.[1] ?? ''
    const decimals = field === 'altitude' || field === 'temperature' ? 0 : 1

    const buildTooltip = (idx: number): string => {
      const v = values[idx]
      if (isNaN(v)) return ''
      const km = xData[idx]?.toFixed(2) ?? '?'
      let html = `${km} km: <strong>${v.toFixed(decimals)}</strong> ${unit}`

      // Gradient annotation on altitude chart
      if (showGradient && idx > 0 && idx < values.length - 1) {
        const prevAlt = values[idx - 1]
        const nextAlt = values[idx + 1]
        const prevDist = (xData[idx - 1] ?? 0) * 1000 // km → m
        const nextDist = (xData[idx + 1] ?? 0) * 1000
        const dDist = nextDist - prevDist
        if (dDist > 0 && !isNaN(prevAlt) && !isNaN(nextAlt)) {
          const pct = ((nextAlt - prevAlt) / dDist) * 100
          const sign = pct >= 0 ? '+' : ''
          html += `<br/><span style="color:#999;font-size:11px">Steigung: ${sign}${pct.toFixed(1)} %</span>`
        }
      }
      return html
    }

    // ── ECharts option ───────────────────────────────────────────────────────
    const option: echarts.EChartsCoreOption = {
      animation: false,
      grid: { left: 52, right: 16, top: 28, bottom: 40 },

      // Brush: always active in lineX mode so user can click-drag to zoom
      brush: {
        toolbox: [],
        brushType: 'lineX',
        brushStyle: {
          borderWidth: 1,
          color: 'rgba(249,115,22,0.12)',
          borderColor: '#f97316',
        },
        throttleType: 'debounce',
        throttleDelay: 150,
      },

      tooltip: {
        trigger: 'axis',
        transitionDuration: 0,
        confine: true,
        backgroundColor: 'rgba(255,255,255,0.92)',
        borderColor: '#e5e5e5',
        borderWidth: 1,
        textStyle: { fontSize: 12, color: '#333' },
        axisPointer: {
          type: 'line',
          lineStyle: { color: '#f97316', width: 1, type: 'dashed' },
          animation: false,
        },
        formatter: (params: unknown) => {
          const arr = Array.isArray(params) ? params : [params]
          const p = arr[0] as { dataIndex?: number; value?: [number, number] }
          if (p?.dataIndex == null) return ''
          return buildTooltip(p.dataIndex)
        },
      },

      xAxis: {
        type: 'value',
        min: 0,
        max: data.maxDist,
        name: 'km',
        nameLocation: 'end',
        nameTextStyle: { fontSize: 11, color: '#aaa' },
        axisLabel: { fontSize: 11, color: '#999' },
        splitLine: { show: false },
      },

      yAxis: {
        type: 'value',
        name: label,
        nameLocation: 'end',
        nameTextStyle: { fontSize: 11, color: '#aaa' },
        axisLabel: { fontSize: 11, color: '#999' },
        splitLine: { lineStyle: { color: '#e5e5e5', type: 'dashed' } },
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
          sampling: 'lttb',
          large: true,
          largeThreshold: 2000,
          ...(mlData.length > 0
            ? {
                markLine: {
                  silent: true,
                  symbol: ['none', 'none'],
                  label: {
                    show: true,
                    position: 'insideEndTop',
                    formatter: (p: { value: number; name: string }) =>
                      `${p.name}: ${p.value.toFixed(decimals)} ${unit}`,
                    color: '#666',
                    fontSize: 11,
                  },
                  lineStyle: { type: 'dashed', color: '#bbb', width: 1 },
                  data: mlData,
                },
              }
            : {}),
        },
      ],

      // Inside dataZoom: enabled for scroll-to-pan after brush zoom;
      // wheel zoom disabled to prevent accidental zoom
      dataZoom: [
        {
          type: 'inside',
          xAxisIndex: 0,
          filterMode: 'none',
          zoomOnMouseWheel: false,
          moveOnMouseMove: true,
          moveOnMouseWheel: false,
        },
      ],
    }

    chart.setOption(option)

    // Activate brush mode so click-drag draws a selection
    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })

    // Hover → map marker sync
    chart.on('showTip', (params: unknown) => {
      const typed = params as { dataIndex?: number }
      if (typed.dataIndex != null) onHover(typed.dataIndex)
    })
    chart.on('hideTip', () => onHover(null))

    // Brush end → zoom all charts
    chart.on('brushEnd', (params: unknown) => {
      const typed = params as { areas?: Array<{ coordRange?: [number, number] }> }
      if (typed.areas && typed.areas.length > 0) {
        const cr = typed.areas[0].coordRange
        if (cr && cr[1] > cr[0]) {
          onBrushZoom(cr[0], cr[1])
        }
      }
    })

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    return () => {
      observer.disconnect()
      onUnregister(chartId)
      chart.dispose()
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, field, label, color, area, altitudeSmartY, markLines, showGradient])

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={containerRef} className="h-[200px] w-full" />
    </div>
  )
}
