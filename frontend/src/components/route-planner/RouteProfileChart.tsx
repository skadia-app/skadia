import { useMemo, useRef } from 'react'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  AxisPointerComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { RouteElevationPoint, RouteSegment } from '@/api/routes'
import type { RouteStats } from '@/hooks/useRoutePlanner'
import { useECharts } from '@/hooks/useECharts'
import { aggregateScaleLevels } from '@/lib/scaleAggregation'
import { ScaleBadge } from './ScaleBadge'
import { SummitBadge } from './SummitBadge'
import {
  suggestRoutesStrings,
  statsForestLabel,
  statsProtectedLabel,
} from '@/lib/strings/suggest-routes'

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
  elevation: number       // always a real number (null points are filtered out)
  grade: number          // gradient in percent
  isOffroad: boolean
}

interface Props {
  /** Elevation profile — RouteElevationPoint[] with cumulative dist_m/ele_m.
   *  null → "Höhenprofil nicht verfügbar" placeholder is rendered. */
  profile: RouteElevationPoint[] | null
  /** Total route distance in meters — used for hover fraction calculation and
   *  x-axis max when profile is shorter than the full route (e.g. partial fill). */
  totalDistanceM: number
  /** Route segments — used to colour offroad sections */
  segments?: RouteSegment[]
  /** Callback: null when mouse leaves, otherwise lat/lng along route at fraction */
  onHoverFraction?: (fraction: number | null) => void
  /** Height of the chart container */
  height?: number
  /** V6-Retrofit: zentraler Stats-Shape aus useRoutePlanner.routeStats.
   *  null = keine Stats verfügbar (BRouter-Engine, kein Segment, oder noch kein
   *  Segment fertig berechnet). */
  routeStats: RouteStats | null
  /** Scale-pin state, hoisted in useRoutePlanner. null = no pin. */
  scalePin: 'sac' | 'mtb' | 'vf' | null
  /** Toggle handler for the scale-pin (mutually exclusive radio). */
  onTogglePin: (kind: 'sac' | 'mtb' | 'vf') => void
  /** Hover handler — transient Map-Preview, clearet bei mouseleave. */
  onScaleHover?: (kind: 'sac' | 'mtb' | 'vf' | null) => void
  /** Hover-Callback — transient, aktiviert Karten-Marker während Hover */
  onSummitHover?: (active: boolean) => void
  /** Pin-Zustand für Summit-Marker (Klick-persistent) */
  summitPinned?: boolean
  /** Toggle-Callback für den Summit-Pin */
  onToggleSummitPin?: () => void
  /** True während Routing-Recompute. Chart freezt am letzten gerenderten Stand
   *  und committet erst nach Settle — verhindert ECharts-Re-Render-Storm
   *  bei jedem Zwischenergebnis während Drag-/Multi-Click-Operationen. */
  isCalculating?: boolean
}

// ── Helpers ───────────────────────────────────────────────────────────────────

/**
 * Build a flat ProfilePoint array from a RouteElevationPoint list.
 * Uses cumulative dist_m directly (no fraction back-calculation).
 * Grade is computed as a centred finite difference (m/m → %).
 */
function buildProfile(
  raw: RouteElevationPoint[],
  segments: RouteSegment[],
): ProfilePoint[] {
  if (raw.length < 2) return []

  // Pre-compute offroad dist_m ranges from segments
  let coveredM = 0
  const offroadRanges: Array<{ start: number; end: number }> = []
  for (const seg of segments) {
    const len = seg.distance_m
    if (seg.mode === 'offroad') {
      offroadRanges.push({ start: coveredM, end: coveredM + len })
    }
    coveredM += len
  }

  const points: ProfilePoint[] = raw.map((pt, i) => {
    const distanceKm = pt.dist_m / 1000

    // Grade via centred finite difference (ele per dist → %)
    let grade = 0
    if (i > 0 && i < raw.length - 1) {
      const prev = raw[i - 1]
      const next = raw[i + 1]
      const dElev = next.ele_m - prev.ele_m
      const dDist = next.dist_m - prev.dist_m
      grade = dDist > 0 ? (dElev / dDist) * 100 : 0
    } else if (i === 0 && raw.length > 1) {
      const next = raw[1]
      const dElev = next.ele_m - raw[0].ele_m
      const dDist = next.dist_m - raw[0].dist_m
      grade = dDist > 0 ? (dElev / dDist) * 100 : 0
    } else if (i === raw.length - 1 && raw.length > 1) {
      const prev = raw[raw.length - 2]
      const dElev = raw[i].ele_m - prev.ele_m
      const dDist = raw[i].dist_m - prev.dist_m
      grade = dDist > 0 ? (dElev / dDist) * 100 : 0
    }

    const isOffroad = offroadRanges.some(r => pt.dist_m >= r.start && pt.dist_m <= r.end)

    return { distanceKm, elevation: pt.ele_m, grade, isOffroad }
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
  routeStats,
  scalePin,
  onTogglePin,
  onScaleHover,
  onSummitHover,
  summitPinned = false,
  onToggleSummitPin,
  isCalculating = false,
}: Props) {
  // Build processed data — empty array when profile is null (renders placeholder)
  const livePoints = useMemo(
    () => profile !== null ? buildProfile(profile, segments) : [],
    [profile, segments],
  )

  // Freeze chart input while a recompute is in flight: hold the last settled
  // points until isCalculating goes false again. Avoids ECharts setOption
  // storms during rapid drag-release sequences.
  const lastSettledPoints = useRef(livePoints)
  if (!isCalculating) lastSettledPoints.current = livePoints
  const points = isCalculating ? lastSettledPoints.current : livePoints

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

  const containerRef = useECharts(
    () => {
      if (points.length === 0) return {}

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

      return {
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
          appendToBody: true,
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
              color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: 'rgba(34,197,94,0.35)' },
                { offset: 0.6, color: 'rgba(34,197,94,0.12)' },
                { offset: 1, color: 'rgba(34,197,94,0.02)' },
              ]),
            },
            encode: { x: 0, y: 1 },
          },
        ],
      } as EChartsCoreOption
    },
    [points, yRange, totalDistanceM, onHoverFraction],
    (chart) => {
      if (!onHoverFraction) return

      const handleMove = (params: { dataIndex?: number }) => {
        if (params.dataIndex == null || !points[params.dataIndex]) {
          onHoverFraction(null)
          return
        }
        const pt = points[params.dataIndex]
        const fraction = totalDistanceM > 0 ? pt.distanceKm / (totalDistanceM / 1000) : 0
        onHoverFraction(fraction)
      }

      const handleOut = () => onHoverFraction(null)

      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      chart.on('mousemove', handleMove as any)
      chart.on('mouseout', handleOut)

      return () => {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        chart.off('mousemove', handleMove as any)
        chart.off('mouseout', handleOut)
      }
    },
  )

  // ── Scale aggregation (LF-#6: ONE parsedCoords memo shared by SAC + MTB) ──
  // V6-Retrofit: parsedCoords kommt aus routeStats.coordinates (merged + deduped
  // über alle Segmente). Bei null → leeres Array → keine Scale-Badges.
  const parsedCoords = useMemo<number[][]>(
    () => routeStats?.coordinates ?? [],
    [routeStats?.coordinates],
  )

  const sacAggregate = useMemo(
    () => aggregateScaleLevels(routeStats?.sac_ranges ?? undefined, parsedCoords, 'sac'),
    [routeStats?.sac_ranges, parsedCoords],
  )

  const mtbAggregate = useMemo(
    () => aggregateScaleLevels(routeStats?.mtb_ranges ?? undefined, parsedCoords, 'mtb'),
    [routeStats?.mtb_ranges, parsedCoords],
  )

  // LF-#6: vfAggregate uses the SAME parsedCoords memo as sac/mtb.
  const vfAggregate = useMemo(
    () => aggregateScaleLevels(routeStats?.vf_ranges ?? undefined, parsedCoords, 'vf'),
    [routeStats?.vf_ranges, parsedCoords],
  )

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
    return { gain: Math.round(gain), loss: Math.round(loss) }
  }, [points])

  // ── Legend items ──────────────────────────────────────────────────────────
  const legendItems = [
    { color: 'rgba(220,38,38,0.90)', label: '> 8 %' },
    { color: 'rgba(249,115,22,0.85)', label: '4–8 %' },
    { color: 'rgba(34,197,94,0.80)', label: '0–4 %' },
    { color: 'rgba(96,165,250,0.80)', label: '< 0 %' },
    { color: 'rgba(139,92,246,0.85)', label: 'Offroad' },
  ]

  if (profile === null || points.length === 0) {
    return (
      <div
        className="flex items-center justify-center rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 text-sm text-neutral-400"
        style={{ height }}
      >
        Höhenprofil nicht verfügbar
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
          <div className="flex items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400 flex-wrap">
            <span>
              <span className="text-green-500">↑</span>{' '}
              <strong className="text-neutral-700 dark:text-neutral-200">{stats.gain} m</strong>
            </span>
            <span>
              <span className="text-blue-400">↓</span>{' '}
              <strong className="text-neutral-700 dark:text-neutral-200">{stats.loss} m</strong>
            </span>
            {routeStats && (
              <>
                {routeStats.forest_pct != null && routeStats.forest_pct > 0 && (
                  <span title="Waldanteil auf der Route">{statsForestLabel(Math.round(routeStats.forest_pct * 100))}</span>
                )}
                {routeStats.protected_pct != null && routeStats.protected_pct > 0.05 && (
                  <span title="Anteil durch Schutzgebiet auf der Route">{statsProtectedLabel(Math.round(routeStats.protected_pct * 100))}</span>
                )}
                {routeStats.summits_passed && routeStats.summits_passed.length > 0 && (
                  <SummitBadge
                    summits={routeStats.summits_passed}
                    onHover={onSummitHover ?? (() => {})}
                    isPinned={summitPinned}
                    onTogglePin={onToggleSummitPin ?? (() => {})}
                  />
                )}
                {sacAggregate.breakdown.length > 0 && sacAggregate.maxLevel && (
                  <ScaleBadge
                    kind="sac"
                    breakdown={sacAggregate.breakdown}
                    maxLevel={sacAggregate.maxLevel}
                    isPinned={scalePin === 'sac'}
                    onTogglePin={() => onTogglePin('sac')}
                    onHover={(active) => onScaleHover?.(active ? 'sac' : null)}
                  />
                )}
                {mtbAggregate.breakdown.length > 0 && mtbAggregate.maxLevel && (
                  <ScaleBadge
                    kind="mtb"
                    breakdown={mtbAggregate.breakdown}
                    maxLevel={mtbAggregate.maxLevel}
                    isPinned={scalePin === 'mtb'}
                    onTogglePin={() => onTogglePin('mtb')}
                    onHover={(active) => onScaleHover?.(active ? 'mtb' : null)}
                  />
                )}
                {vfAggregate.breakdown.length > 0 && vfAggregate.maxLevel && (
                  <ScaleBadge
                    kind="vf"
                    breakdown={vfAggregate.breakdown}
                    maxLevel={vfAggregate.maxLevel}
                    isPinned={scalePin === 'vf'}
                    onTogglePin={() => onTogglePin('vf')}
                    onHover={(active) => onScaleHover?.(active ? 'vf' : null)}
                  />
                )}
                {/*
                 * I9: vf_unknown_ranges rendert IMMER wenn nicht-leer, unabhängig von scalePin.
                 * Begründung: K?-Ranges sind ein Safety-Warning — der User darf sie nicht
                 * nur sehen, wenn er zufällig den VF-Pin aktiviert hat. MapPage-Layer
                 * LAYER_VF_UNKNOWN ist analog dauerhaft aktiv (Task 22a.6).
                 */}
                {routeStats.vf_unknown_ranges && routeStats.vf_unknown_ranges.length > 0 && (
                  <span
                    role="status"
                    aria-label={suggestRoutesStrings.stats.vfUnknownAriaLabel}
                    className="inline-flex items-center gap-1 text-xs px-1.5 py-0.5 rounded bg-red-100 text-red-900 dark:bg-red-950 dark:text-red-200"
                    title={suggestRoutesStrings.stats.vfUnknownTitle}
                  >
                    {suggestRoutesStrings.stats.vfUnknownLabel}
                  </span>
                )}
              </>
            )}
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
