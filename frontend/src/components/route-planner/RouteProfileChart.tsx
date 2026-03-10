import { useRef, useEffect, useMemo } from 'react'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  AxisPointerComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { ElevationPoint, RouteSegment } from '@/api/routes'

echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  AxisPointerComponent,
  CanvasRenderer,
])

// ── Types ─────────────────────────────────────────────────────────────────────

interface ProfilePoint {
  distanceKm: number
  elevation: number
  grade: number          // gradient in percent
  isOffroad: boolean
}

interface Props {
  /** Elevation profile from POST /elevation/profile */
  profile: ElevationPoint[]
  /** Total route distance in meters — used to convert fraction→km */
  totalDistanceM: number
  /** Route segments — used to colour offroad sections */
  segments?: RouteSegment[]
  /** Callback: null when mouse leaves, otherwise lat/lng along route at fraction */
  onHoverFraction?: (fraction: number | null) => void
  /** Height of the chart container */
  height?: number
}

// ── Helpers ───────────────────────────────────────────────────────────────────

/**
 * Build a flat ProfilePoint array from the raw ElevationPoint list.
 * Grade is computed as a centred finite difference (m per km → %).
 */
function buildProfile(
  raw: ElevationPoint[],
  totalDistanceM: number,
  segments: RouteSegment[],
): ProfilePoint[] {
  if (raw.length < 2) return []

  // Pre-compute offroad fraction ranges from segments
  let coveredFraction = 0
  const offroadRanges: Array<{ start: number; end: number }> = []
  for (const seg of segments) {
    const totalM = segments.reduce((s, sg) => s + sg.distance_m, 0) || totalDistanceM
    const len = totalM > 0 ? seg.distance_m / totalM : 0
    if (seg.mode === 'offroad') {
      offroadRanges.push({ start: coveredFraction, end: coveredFraction + len })
    }
    coveredFraction += len
  }

  const points: ProfilePoint[] = raw.map((pt, i) => {
    const distanceKm = pt.fraction * totalDistanceM / 1000

    // Grade via centred difference
    let grade = 0
    if (i > 0 && i < raw.length - 1) {
      const prev = raw[i - 1]
      const next = raw[i + 1]
      const dElev = next.elevation - prev.elevation
      const dDist = (next.fraction - prev.fraction) * totalDistanceM
      grade = dDist > 0 ? (dElev / dDist) * 100 : 0
    } else if (i === 0 && raw.length > 1) {
      const next = raw[1]
      const dElev = next.elevation - raw[0].elevation
      const dDist = (next.fraction - raw[0].fraction) * totalDistanceM
      grade = dDist > 0 ? (dElev / dDist) * 100 : 0
    } else if (i === raw.length - 1 && raw.length > 1) {
      const prev = raw[raw.length - 2]
      const dElev = raw[i].elevation - prev.elevation
      const dDist = (raw[i].fraction - prev.fraction) * totalDistanceM
      grade = dDist > 0 ? (dElev / dDist) * 100 : 0
    }

    const isOffroad = offroadRanges.some(r => pt.fraction >= r.start && pt.fraction <= r.end)

    return { distanceKm, elevation: pt.elevation, grade, isOffroad }
  })

  return points
}

/**
 * Map a grade percentage to an RGBA colour for the visual series item.
 * > +8 %  → deep red
 * +4–8 %  → orange
 * 0–4 %   → green
 * negative → blue shades
 */
function gradeColor(grade: number): string {
  const clamped = Math.max(-15, Math.min(15, grade))
  if (clamped >= 8)  return 'rgba(220,38,38,0.90)'   // red-600
  if (clamped >= 4)  return 'rgba(249,115,22,0.85)'  // orange-500
  if (clamped >= 0)  return 'rgba(34,197,94,0.80)'   // green-500
  if (clamped >= -4) return 'rgba(96,165,250,0.80)'  // blue-400
  if (clamped >= -8) return 'rgba(59,130,246,0.85)'  // blue-500
  return 'rgba(37,99,235,0.90)'                       // blue-700
}

function formatGrade(grade: number): string {
  const sign = grade >= 0 ? '+' : ''
  return `${sign}${grade.toFixed(1)} %`
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function RouteProfileChart({
  profile,
  totalDistanceM,
  segments = [],
  onHoverFraction,
  height = 180,
}: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  // Build processed data
  const points = useMemo(
    () => buildProfile(profile, totalDistanceM, segments),
    [profile, totalDistanceM, segments],
  )

  // Y-axis range with padding
  const yRange = useMemo(() => {
    if (points.length === 0) return { min: 0, max: 100 }
    const elevations = points.map(p => p.elevation)
    const lo = Math.min(...elevations)
    const hi = Math.max(...elevations)
    const range = hi - lo
    const pad = Math.max(10, range * 0.15)
    return {
      min: Math.floor(lo - pad),
      max: Math.ceil(hi + pad),
    }
  }, [points])

  // Gradient definition id (unique per instance in case of multiple charts)
  const gradientId = 'route-profile-gradient'

  // ── Init / destroy chart ──────────────────────────────────────────────────
  useEffect(() => {
    if (!containerRef.current) return
    if (chartRef.current) chartRef.current.dispose()

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    const obs = new ResizeObserver(() => chart.resize())
    obs.observe(containerRef.current)

    return () => {
      obs.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, []) // mount/unmount only

  // ── Update chart option when data changes ─────────────────────────────────
  useEffect(() => {
    const chart = chartRef.current
    if (!chart || points.length === 0) return

    const xData = points.map(p => p.distanceKm)
    const yData = points.map(p => p.elevation)

    // Piece-wise visual mapping: each segment coloured by grade
    const visualPieces = points.map((p, i) => ({
      gt: i === 0 ? undefined : xData[i - 1],
      lte: xData[i],
      color: p.isOffroad
        ? 'rgba(139,92,246,0.85)' // violet for offroad
        : gradeColor(p.grade),
    }))

    const option: EChartsCoreOption = {
      backgroundColor: 'transparent',
      animation: false,
      grid: {
        left: 48,
        right: 16,
        top: 12,
        bottom: 28,
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: {
          type: 'cross',
          crossStyle: { color: '#6b7280', width: 1 },
          lineStyle: { color: '#6b7280', width: 1, type: 'dashed' },
        },
        backgroundColor: 'rgba(17,24,39,0.92)',
        borderColor: '#374151',
        borderWidth: 1,
        textStyle: { color: '#f3f4f6', fontSize: 11 },
        formatter: (params: unknown) => {
          const items = params as Array<{
            dataIndex: number
            axisValue: number
          }>
          if (!Array.isArray(items) || items.length === 0) return ''
          const idx = items[0].dataIndex
          const pt = points[idx]
          if (!pt) return ''

          const modeLabel = pt.isOffroad
            ? '<span style="color:#a78bfa">Offroad</span>'
            : '<span style="color:#6b7280">Straße/Weg</span>'

          return `
            <div style="font-size:11px;line-height:1.7">
              <div style="color:#9ca3af;margin-bottom:2px">${pt.distanceKm.toFixed(2)} km</div>
              <div><span style="color:#9ca3af">Höhe:</span> <strong style="color:#f9fafb">${Math.round(pt.elevation)} m</strong></div>
              <div><span style="color:#9ca3af">Steigung:</span> <strong style="color:${gradeColor(pt.grade)}">${formatGrade(pt.grade)}</strong></div>
              <div>${modeLabel}</div>
            </div>
          `
        },
      },
      xAxis: {
        type: 'value',
        min: 0,
        max: totalDistanceM / 1000,
        axisLabel: {
          fontSize: 10,
          color: '#6b7280',
          formatter: (v: number) => `${v.toFixed(1)} km`,
        },
        axisLine: { lineStyle: { color: '#374151' } },
        splitLine: { show: false },
        axisTick: { show: false },
      },
      yAxis: {
        type: 'value',
        min: yRange.min,
        max: yRange.max,
        axisLabel: {
          fontSize: 10,
          color: '#6b7280',
          formatter: (v: number) => `${v} m`,
        },
        splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
      },
      visualMap: {
        show: false,
        type: 'piecewise',
        dimension: 0,
        pieces: visualPieces,
        seriesIndex: 0,
        outOfRange: { color: 'rgba(34,197,94,0.80)' },
      },
      series: [
        {
          type: 'line',
          data: xData.map((x, i) => [x, yData[i]]),
          symbol: 'none',
          smooth: 0.2,
          lineStyle: { width: 2 },
          areaStyle: {
            // Gradient fill: green at bottom → near-transparent at top
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(34,197,94,0.35)' },
              { offset: 0.6, color: 'rgba(34,197,94,0.12)' },
              { offset: 1, color: 'rgba(34,197,94,0.02)' },
            ]),
          },
          encode: { x: 0, y: 1 },
        },
      ],
    }

    chart.setOption(option, { notMerge: true })
  }, [points, yRange, totalDistanceM])

  // ── Hover → callback ──────────────────────────────────────────────────────
  useEffect(() => {
    const chart = chartRef.current
    if (!chart || !onHoverFraction) return

    const handleMove = (params: { dataIndex?: number }) => {
      if (params.dataIndex == null || !points[params.dataIndex]) {
        onHoverFraction(null)
        return
      }
      const pt = points[params.dataIndex]
      // fraction = distanceKm / totalKm
      const fraction = totalDistanceM > 0 ? pt.distanceKm / (totalDistanceM / 1000) : 0
      onHoverFraction(fraction)
    }

    const handleOut = () => onHoverFraction(null)

    chart.on('mousemove', handleMove as Parameters<typeof chart.on>[1])
    chart.on('mouseout', handleOut)

    return () => {
      chart.off('mousemove', handleMove as Parameters<typeof chart.off>[1])
      chart.off('mouseout', handleOut)
    }
  }, [points, totalDistanceM, onHoverFraction])

  // ── Elevation statistics ───────────────────────────────────────────────────
  const stats = useMemo(() => {
    if (points.length < 2) return null
    let gain = 0
    let loss = 0
    for (let i = 1; i < points.length; i++) {
      const delta = points[i].elevation - points[i - 1].elevation
      if (delta > 0) gain += delta
      else loss += Math.abs(delta)
    }
    const maxGrade = Math.max(...points.map(p => p.grade))
    return { gain: Math.round(gain), loss: Math.round(loss), maxGrade }
  }, [points])

  // ── Legend items ──────────────────────────────────────────────────────────
  const legendItems = [
    { color: 'rgba(220,38,38,0.90)', label: '> 8 %' },
    { color: 'rgba(249,115,22,0.85)', label: '4–8 %' },
    { color: 'rgba(34,197,94,0.80)', label: '0–4 %' },
    { color: 'rgba(96,165,250,0.80)', label: '< 0 %' },
    { color: 'rgba(139,92,246,0.85)', label: 'Offroad' },
  ]

  if (points.length === 0) {
    return (
      <div
        className="flex items-center justify-center rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 text-sm text-neutral-400"
        style={{ height }}
      >
        Kein Höhenprofil verfügbar
      </div>
    )
  }

  return (
    <div className="space-y-1">
      {/* Chart */}
      <div
        className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden"
      >
        <div ref={containerRef} style={{ height, width: '100%' }} />
      </div>

      {/* Bottom bar: stats + legend */}
      <div className="flex items-center justify-between flex-wrap gap-x-4 gap-y-1 px-1">
        {/* Stats */}
        {stats && (
          <div className="flex items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400">
            <span>
              <span className="text-green-500">↑</span>{' '}
              <strong className="text-neutral-700 dark:text-neutral-200">{stats.gain} m</strong>
            </span>
            <span>
              <span className="text-blue-400">↓</span>{' '}
              <strong className="text-neutral-700 dark:text-neutral-200">{stats.loss} m</strong>
            </span>
            <span>
              Max:{' '}
              <strong className="text-neutral-700 dark:text-neutral-200">
                {formatGrade(stats.maxGrade)}
              </strong>
            </span>
          </div>
        )}

        {/* Gradient legend */}
        <div className="flex items-center gap-2 flex-wrap">
          {legendItems.map(item => (
            <div key={item.label} className="flex items-center gap-1">
              <span
                className="inline-block w-3 h-2 rounded-sm"
                style={{ background: item.color }}
              />
              <span className="text-[10px] text-neutral-400">{item.label}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
