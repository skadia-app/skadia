import { memo, useMemo, useRef, useState } from 'react'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  AxisPointerComponent,
  DataZoomComponent,
  BrushComponent,
  ToolboxComponent,
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
import { HIGHWAY_LABELS, SURFACE_GROUPS } from '@/lib/surfaceConstants'

echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  AxisPointerComponent,
  DataZoomComponent,
  BrushComponent,
  ToolboxComponent,
  CanvasRenderer,
])

// ── Types ─────────────────────────────────────────────────────────────────────

interface ProfilePoint {
  distanceKm: number
  elevation: number | null  // null = DEM coverage gap (Spec §6 Class A); ECharts renders as gap
  grade: number             // gradient in percent; 0 for null/gap points
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
  onBrushZoom?: (startDistM: number, endDistM: number) => void
  onBrushReset?: () => void
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
 *
 * Null ele_m values (DEM coverage gaps, Spec §6 Class A) are preserved in the
 * output so ECharts can render them as transparent gaps rather than interpolating
 * fake heights. Grade is set to 0 for gap points.
 *
 * @internal Exported for unit-testing only.
 */
export function buildProfile(
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

  // Spec §6 Class A: null ele_m = DEM coverage gap. Keep null points so ECharts
  // renders an honest visual gap rather than linearly interpolating fake heights.
  // Require ≥2 non-null points to have a renderable chart at all.
  const nonNullCount = raw.reduce((n, pt) => (pt.ele_m !== null ? n + 1 : n), 0)
  if (nonNullCount < 2) return []

  // Build a lookup of non-null points for grade computation — grade is only
  // meaningful between points with actual elevation data.
  const nonNullPts = raw.filter((pt): pt is RouteElevationPoint & { ele_m: number } =>
    pt.ele_m !== null,
  )

  // O(1) lookup: dist_m → index into nonNullPts. Built once here so the
  // raw.map below does not call findIndex for every point (was O(N²)).
  const nonNullIdxByDist = new Map<number, number>(
    nonNullPts.map((pt, i) => [pt.dist_m, i]),
  )

  const points: ProfilePoint[] = raw.map(pt => {
    const distanceKm = pt.dist_m / 1000

    // Grade via centred finite difference using nearest non-null neighbours.
    // For gap (null) points: grade = 0 (no colour, no numeric meaning).
    let grade = 0
    if (pt.ele_m !== null) {
      const idx = nonNullIdxByDist.get(pt.dist_m) ?? -1
      if (idx > 0 && idx < nonNullPts.length - 1) {
        const prev = nonNullPts[idx - 1]
        const next = nonNullPts[idx + 1]
        const dDist = next.dist_m - prev.dist_m
        grade = dDist > 0 ? ((next.ele_m - prev.ele_m) / dDist) * 100 : 0
      } else if (idx === 0 && nonNullPts.length > 1) {
        const next = nonNullPts[1]
        const dDist = next.dist_m - pt.dist_m
        grade = dDist > 0 ? ((next.ele_m - pt.ele_m) / dDist) * 100 : 0
      } else if (idx === nonNullPts.length - 1 && nonNullPts.length > 1) {
        const prev = nonNullPts[nonNullPts.length - 2]
        const dDist = pt.dist_m - prev.dist_m
        grade = dDist > 0 ? ((pt.ele_m - prev.ele_m) / dDist) * 100 : 0
      }
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

interface DistRangeWaySurface {
  startDistM: number
  endDistM: number
  highway: string
  surface: string
}

/** Flatten all segments' sub_segments into global-dist-based ranges so the
 *  tooltip can binary-search highway+surface at the hovered position. */
function buildSubSegmentRanges(segments: RouteSegment[]): DistRangeWaySurface[] {
  const out: DistRangeWaySurface[] = []
  let offset = 0
  for (const seg of segments) {
    const subs = seg.sub_segments
    if (subs && subs.length > 0) {
      for (const sub of subs) {
        const from = sub.from_dist_m ?? 0
        const to = sub.to_dist_m ?? seg.distance_m
        out.push({
          startDistM: offset + from,
          endDistM: offset + to,
          highway: sub.highway,
          surface: sub.surface,
        })
      }
    } else if (seg.mode === 'offroad') {
      out.push({
        startDistM: offset,
        endDistM: offset + seg.distance_m,
        highway: 'offroad',
        surface: 'offroad',
      })
    }
    offset += seg.distance_m
  }
  return out
}

function lookupWaySurface(
  ranges: DistRangeWaySurface[],
  distM: number,
): { highway: string; surface: string } | null {
  for (const r of ranges) {
    if (distM >= r.startDistM && distM <= r.endDistM) {
      return { highway: r.highway, surface: r.surface }
    }
  }
  return null
}

// ── Component ─────────────────────────────────────────────────────────────────

function RouteProfileChart({
  profile,
  totalDistanceM,
  segments = [],
  onHoverFraction,
  onBrushZoom,
  onBrushReset,
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

  const subSegmentRanges = useMemo(
    () => buildSubSegmentRanges(segments),
    [segments],
  )

  // Snapshot of the last "good" frame (≥2 points, not during a recompute).
  // Held during:
  //   - isCalculating === true (recompute in flight, transient profile)
  //   - transient empty windows like insertViaWaypoint, where segments are
  //     briefly removed before _calculateSegment fires (would otherwise
  //     collapse the whole panel to "Höhenprofil nicht verfügbar")
  // Both points AND totalDistanceM are frozen together so the x-axis scale
  // stays consistent with the rendered data.
  const snap = useRef({ points: livePoints, totalDistanceM })
  const chartInstanceRef = useRef<echarts.ECharts | null>(null)
  const [isBrushZoomed, setIsBrushZoomed] = useState(false)
  const haveLive = livePoints.length >= 2
  if (!isCalculating && haveLive) {
    snap.current = { points: livePoints, totalDistanceM }
  }
  const useSnapshot = (!haveLive || isCalculating) && snap.current.points.length >= 2
  const points = useSnapshot ? snap.current.points : livePoints
  const effectiveTotalM = useSnapshot ? snap.current.totalDistanceM : totalDistanceM

  // Y-axis range with padding — skip null (gap) points
  const yRange = useMemo(() => {
    if (points.length === 0) return { min: 0, max: 100 }
    const elevations = points.map(p => p.elevation).filter((e): e is number => e !== null)
    if (elevations.length === 0) return { min: 0, max: 100 }
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

            const ws = lookupWaySurface(subSegmentRanges, pt.distanceKm * 1000)
            const wayLabel = ws ? (HIGHWAY_LABELS[ws.highway] ?? ws.highway) : null
            const surfaceLabel = ws ? (SURFACE_GROUPS[ws.surface] ?? ws.surface) : null

            return `
              <div style="font-size:11px;line-height:1.7">
                <div style="margin-bottom:2px"><span style="color:#9ca3af">Distanz:</span> <strong style="color:#f9fafb">${pt.distanceKm.toFixed(2)} km</strong></div>
                <div><span style="color:#9ca3af">Höhe:</span> <strong style="color:#f9fafb">${pt.elevation !== null ? `${Math.round(pt.elevation)} m` : '—'}</strong></div>
                <div><span style="color:#9ca3af">Steigung:</span> <strong style="color:${gradeColor(pt.grade)}">${formatGrade(pt.grade)}</strong></div>
                ${wayLabel ? `<div><span style="color:#9ca3af">Wegart:</span> <strong style="color:#f9fafb">${wayLabel}</strong></div>` : ''}
                ${surfaceLabel ? `<div><span style="color:#9ca3af">Oberfläche:</span> <strong style="color:#f9fafb">${surfaceLabel}</strong></div>` : ''}
                ${!ws && pt.isOffroad ? '<div><span style="color:#a78bfa">Offroad</span></div>' : ''}
              </div>
            `
          },
        },
        xAxis: {
          type: 'value',
          min: 0,
          max: effectiveTotalM / 1000,
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
        toolbox: onBrushZoom ? {
          show: false,
          feature: { brush: { type: ['lineX'] } },
        } : undefined,
        brush: onBrushZoom ? {
          toolbox: [],
          brushType: 'lineX',
          brushMode: 'single',
          xAxisIndex: 0,
          brushStyle: { borderWidth: 1, color: 'rgba(249,115,22,0.12)', borderColor: '#f97316' },
        } : undefined,
        dataZoom: onBrushZoom ? [
          // inside-dataZoom receives both the brush-end dispatchAction and the
          // user's mouse-wheel zoom. moveOnMouseMove stays false so the brush
          // cursor (left-drag) is not stolen by a pan-grab. moveOnMouseWheel
          // also stays false — the wheel should zoom, not pan.
          { type: 'inside', xAxisIndex: 0, filterMode: 'none', zoomOnMouseWheel: true, moveOnMouseMove: false, moveOnMouseWheel: false },
        ] : undefined,
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
    [points, yRange, effectiveTotalM, onHoverFraction, onBrushZoom],
    (chart) => {
      chartInstanceRef.current = chart
      const cleanups: Array<() => void> = []

      if (onHoverFraction) {
        // Use zrender's continuous mousemove (fires across the whole chart
        // area, not just on series points) and map pixel-x to data-x via
        // convertFromPixel. chart.on('mousemove') only fires when the cursor
        // is directly over a series data point, which makes hover tracking
        // unreliable on thin elevation lines.
        const xMaxKm = effectiveTotalM / 1000
        const handleMove = (e: { offsetX: number; offsetY: number }) => {
          const km = chart.convertFromPixel({ xAxisIndex: 0 }, e.offsetX) as number | undefined
          if (typeof km !== 'number' || !Number.isFinite(km) || xMaxKm <= 0) {
            onHoverFraction(null)
            return
          }
          const fraction = Math.max(0, Math.min(1, km / xMaxKm))
          onHoverFraction(fraction)
        }
        const handleOut = () => onHoverFraction(null)

        const zr = chart.getZr()
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        zr.on('mousemove', handleMove as any)
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        zr.on('globalout', handleOut as any)

        cleanups.push(() => {
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          zr.off('mousemove', handleMove as any)
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          zr.off('globalout', handleOut as any)
        })
      }

      if (onBrushZoom) {
        type BrushArea = { range?: [number, number]; coordRange?: [number, number] }
        const handleBrushEnd = (params: unknown) => {
          const p = params as {
            areas?: BrushArea[]
            batch?: Array<{ areas?: BrushArea[] }>
          }
          const areas = p.areas ?? p.batch?.[0]?.areas
          if (!areas || areas.length === 0) return

          const area = areas[0]
          let startKm: number
          let endKm: number
          if (area.coordRange) {
            [startKm, endKm] = area.coordRange
          } else if (area.range) {
            const start = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
            const end = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
            if (typeof start !== 'number' || typeof end !== 'number') return
            startKm = start
            endKm = end
          } else {
            return
          }

          const xMaxKm = effectiveTotalM / 1000
          const loKm = Math.max(0, Math.min(startKm, endKm))
          const hiKm = Math.min(xMaxKm, Math.max(startKm, endKm))
          if (xMaxKm <= 0 || hiKm - loKm < 0.005) return

          chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: (loKm / xMaxKm) * 100, end: (hiKm / xMaxKm) * 100 })
          chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
          // Re-arm brush so the user can immediately draw the next selection.
          chart.dispatchAction({
            type: 'takeGlobalCursor',
            key: 'brush',
            brushOption: { brushType: 'lineX', brushMode: 'single' },
          })
          setIsBrushZoomed(true)
          onBrushZoom(loKm * 1000, hiKm * 1000)
        }

        // Brush is the default cursor — user can immediately drag a range.
        // Without this, the inside-dataZoom takes over and the cursor becomes
        // a grab-hand. Keeping brush always-on means the chart cursor is the
        // crosshair selection cursor at all times.
        chart.dispatchAction({
          type: 'takeGlobalCursor',
          key: 'brush',
          brushOption: { brushType: 'lineX', brushMode: 'single' },
        })
        chart.on('brushEnd', handleBrushEnd)
        cleanups.push(() => chart.off('brushEnd', handleBrushEnd))

        // Mirror wheel-zoom into the same map-sync + "Zoom zurück"-state
        // as the brush flow. The dataZoom event fires for every range change,
        // including programmatic dispatchAction calls; we ignore the latter
        // by debouncing on the start/end percentage values themselves.
        const handleDataZoom = (params: unknown) => {
          const xMaxKm = effectiveTotalM / 1000
          if (xMaxKm <= 0) return
          // batch[0] is the standard shape for inside dataZoom events
          const p = params as { batch?: Array<{ start?: number; end?: number }>; start?: number; end?: number }
          const range = p.batch?.[0] ?? p
          const startPct = typeof range.start === 'number' ? range.start : 0
          const endPct = typeof range.end === 'number' ? range.end : 100
          if (startPct <= 0 && endPct >= 100) {
            setIsBrushZoomed(false)
            return
          }
          setIsBrushZoomed(true)
          onBrushZoom((startPct / 100) * xMaxKm * 1000, (endPct / 100) * xMaxKm * 1000)
        }
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        chart.on('dataZoom', handleDataZoom as any)
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        cleanups.push(() => chart.off('dataZoom', handleDataZoom as any))
      }

      return () => {
        chartInstanceRef.current = null
        cleanups.forEach(cleanup => cleanup())
      }
    },
  )

  const handleBrushReset = () => {
    const chart = chartInstanceRef.current
    if (chart) {
      chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
      // Re-arm brush after reset so the next drag immediately selects again.
      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: 'lineX', brushMode: 'single' },
      })
    }
    setIsBrushZoomed(false)
    onBrushReset?.()
  }

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

  // ── Elevation statistics — skip null (gap) points ────────────────────────
  const stats = useMemo(() => {
    if (points.length < 2) return null
    let gain = 0
    let loss = 0
    let prevEle: number | null = null
    for (const pt of points) {
      if (pt.elevation === null) continue
      if (prevEle !== null) {
        const delta = pt.elevation - prevEle
        if (delta > 0) gain += delta
        else loss += Math.abs(delta)
      }
      prevEle = pt.elevation
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

  // Placeholder only when truly empty — profile === null alone is too eager
  // because insertViaWaypoint briefly evicts the affected segment, making
  // effectiveProfile null for one render. The snapshot above keeps `points`
  // populated through that window.
  if (points.length === 0) {
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
                {/* Threshold mirrors StatsRow.tsx (Math.round(pct) >= 1 on 0..100 scale).
                    forest_pct/protected_pct here are 0..1, hence the *100 before rounding. */}
                {routeStats.forest_pct != null && Math.round(routeStats.forest_pct * 100) >= 1 && (
                  <span title="Waldanteil auf der Route">{statsForestLabel(Math.round(routeStats.forest_pct * 100))}</span>
                )}
                {routeStats.protected_pct != null && Math.round(routeStats.protected_pct * 100) >= 1 && (
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
          {isBrushZoomed && (
            <button
              type="button"
              onClick={handleBrushReset}
              className="text-[10px] text-neutral-500 hover:text-neutral-800 dark:text-neutral-400 dark:hover:text-neutral-100 underline underline-offset-2 cursor-pointer"
            >
              Zoom zurück
            </button>
          )}
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

export default memo(RouteProfileChart)
