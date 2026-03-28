import { useRef, useEffect, useMemo, useCallback, useState } from 'react'
import * as echarts from 'echarts/core'
import { useECharts } from '@/hooks/useECharts'
import { LineChart, ScatterChart } from 'echarts/charts'
import { getSegmentGroup } from '@/lib/sportTaxonomy'
import { fieldLabel, fieldUnit, type FieldKey } from '@/lib/sportFieldLabels'
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  AxisPointerComponent,
  BrushComponent,
  MarkLineComponent,
  MarkPointComponent,
  MarkAreaComponent,
  ToolboxComponent,
  VisualMapComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { StreamPoint } from '@/api/types'

echarts.use([
  LineChart,
  ScatterChart,
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  AxisPointerComponent,
  BrushComponent,
  MarkLineComponent,
  MarkPointComponent,
  MarkAreaComponent,
  ToolboxComponent,
  VisualMapComponent,
  CanvasRenderer,
])

// ── SVG symbols for sunrise/sunset mark points ──────────────────────────────

const SUNRISE_SVG = "image://data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32' width='32' height='32'%3E%3Cline x1='16' y1='4' x2='16' y2='9' stroke='%23f59e0b' stroke-width='2' stroke-linecap='round'/%3E%3Cline x1='6' y1='12' x2='9.5' y2='14' stroke='%23f59e0b' stroke-width='2' stroke-linecap='round'/%3E%3Cline x1='26' y1='12' x2='22.5' y2='14' stroke='%23f59e0b' stroke-width='2' stroke-linecap='round'/%3E%3Cpath d='M8 22 A8 8 0 0 1 24 22' fill='%23f59e0b'/%3E%3Cline x1='4' y1='22' x2='28' y2='22' stroke='%23f59e0b' stroke-width='2' stroke-linecap='round'/%3E%3C/svg%3E"

const SUNSET_SVG = "image://data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32' width='32' height='32'%3E%3Cline x1='16' y1='28' x2='16' y2='23' stroke='%23e67e22' stroke-width='2' stroke-linecap='round'/%3E%3Cline x1='6' y1='20' x2='9.5' y2='18' stroke='%23e67e22' stroke-width='2' stroke-linecap='round'/%3E%3Cline x1='26' y1='20' x2='22.5' y2='18' stroke='%23e67e22' stroke-width='2' stroke-linecap='round'/%3E%3Cpath d='M8 10 A8 8 0 0 0 24 10' fill='%23e67e22'/%3E%3Cline x1='4' y1='10' x2='28' y2='10' stroke='%23e67e22' stroke-width='2' stroke-linecap='round'/%3E%3C/svg%3E"

// ── Types ─────────────────────────────────────────────────────────────────────

// FieldKey imported from sportFieldLabels.ts
type XMode = 'distance' | 'time'

type ChartKey = 'altitude' | 'heartRate' | 'speed' | 'power' | 'cadence' | 'temperature' | 'runningDynamics' | 'stepLength' | 'torqueEff' | 'pedalSm' | 'motorPower' | 'batteryLevel' | 'travelRange' | 'depth' | 'cycleLength'

function getTabCharts(tab: string, sportType: string): ChartKey[] | null {
  if (tab === 'alles') return null // show all
  const isSwim = getSegmentGroup(sportType) === 'swim'
  switch (tab) {
    case 'uebersicht': return isSwim ? ['heartRate', 'cycleLength'] : ['altitude', 'heartRate', 'speed']
    case 'leistung': {
      const sg = getSegmentGroup(sportType)
      const charts: ChartKey[] = ['power', 'cadence']
      if (sg === 'swim' || sg === 'paddle') charts.push('cycleLength')
      return charts
    }
    case 'dynamik': return ['runningDynamics', 'stepLength']
    case 'pedal': return ['torqueEff', 'pedalSm']
    case 'ebike': return ['motorPower', 'batteryLevel', 'travelRange']
    case 'tauchgang': return ['depth']
    case 'sensoren': return ['temperature']
    default: return null
  }
}

interface ChartData {
  distanceKm: number[]
  elapsedMin: number[]
  altitude: number[]
  grade: number[]
  heartRate: number[]
  power: number[]
  cadence: number[]
  speed: number[]
  speedMs: number[]
  temperature: number[]
  groundContactTime: number[]
  verticalOscillation: number[]
  stepLength: number[]
  torqueEffL: number[]
  torqueEffR: number[]
  pedalSmL: number[]
  pedalSmR: number[]
  motorPower: number[]
  batteryLevel: number[]
  travelRange: number[]
  depth: number[]
  cycleLength: number[]
  maxDist: number
  maxElapsed: number
}

// ── Data processing ────────────────────────────────────────────────────────────

function buildChartData(streams: StreamPoint[]): ChartData {
  const distanceKm: number[] = []
  const elapsedMin: number[] = []
  const altitude: number[] = []
  const grade: number[] = []
  const heartRate: number[] = []
  const power: number[] = []
  const cadence: number[] = []
  const speed: number[] = []
  const speedMs: number[] = []
  const temperature: number[] = []
  const groundContactTime: number[] = []
  const verticalOscillation: number[] = []
  const stepLength: number[] = []
  const torqueEffL: number[] = []
  const torqueEffR: number[] = []
  const pedalSmL: number[] = []
  const pedalSmR: number[] = []
  const motorPower: number[] = []
  const batteryLevel: number[] = []
  const travelRange: number[] = []
  const depth: number[] = []
  const cycleLength: number[] = []

  const t0 = streams.length > 0 ? new Date(streams[0].time).getTime() : 0

  for (const s of streams) {
    distanceKm.push(
      s.distance_m != null
        ? s.distance_m / 1000
        : distanceKm.length > 0 ? distanceKm[distanceKm.length - 1] : 0,
    )
    elapsedMin.push((new Date(s.time).getTime() - t0) / 60000)
    altitude.push(s.altitude_m ?? NaN)
    grade.push(s.grade_pct ?? NaN)
    heartRate.push(s.heart_rate ?? NaN)
    power.push(s.power_w ?? NaN)
    cadence.push(s.cadence != null && s.cadence > 0 ? s.cadence : NaN)
    speed.push(s.speed_ms != null ? s.speed_ms * 3.6 : NaN)
    speedMs.push(s.speed_ms ?? NaN)
    temperature.push(s.temperature_c ?? NaN)
    groundContactTime.push(s.ground_contact_time_ms ?? NaN)
    verticalOscillation.push(s.vertical_oscillation_mm != null ? s.vertical_oscillation_mm / 10 : NaN)
    stepLength.push(s.step_length_mm != null ? s.step_length_mm / 1000 : NaN)
    torqueEffL.push(s.left_torque_effectiveness_pct ?? NaN)
    torqueEffR.push(s.right_torque_effectiveness_pct ?? NaN)
    pedalSmL.push(s.left_pedal_smoothness_pct ?? NaN)
    pedalSmR.push(s.right_pedal_smoothness_pct ?? NaN)
    motorPower.push(s.motor_power_w ?? NaN)
    batteryLevel.push(s.ebike_battery_level_pct ?? NaN)
    travelRange.push(s.ebike_travel_range_km ?? NaN)
    depth.push(s.depth_m ?? NaN)
    // Clamp: values > 50m are FIT parser artifacts (real stroke distance is 0.5–15m)
    cycleLength.push(s.cycle_length_m != null && s.cycle_length_m > 0 && s.cycle_length_m <= 50 ? s.cycle_length_m : NaN)
  }

  const validDist = distanceKm.filter(v => !isNaN(v) && isFinite(v))
  const rawMax = validDist.length > 0 ? Math.max(...validDist) : 1
  const maxDist = Math.ceil(rawMax * 10) / 10
  const maxElapsed = elapsedMin.length > 0 ? Math.ceil(elapsedMin[elapsedMin.length - 1]) : 0

  return { distanceKm, elapsedMin, altitude, grade, heartRate, power, cadence, speed, speedMs, temperature, groundContactTime, verticalOscillation, stepLength, torqueEffL, torqueEffR, pedalSmL, pedalSmR, motorPower, batteryLevel, travelRange, depth, cycleLength, maxDist, maxElapsed }
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

export interface SplitMarker {
  /** Elapsed minutes from start of merged stream */
  elapsedMin: number
  /** Distance in km from start of merged stream */
  distanceKm: number
  /** Label shown above the line, e.g. "🏊→🚴" */
  label: string
}

export interface ZoomRange {
  startKm: number
  endKm: number
  /** Value-based percentage (0–100) of the X-axis range */
  startPct: number
  /** Value-based percentage (0–100) of the X-axis range */
  endPct: number
  /** Index into the streams array where the zoomed range starts */
  startIdx: number
  /** Index into the streams array where the zoomed range ends */
  endIdx: number
}

interface SunMarkLine {
  xAxis: number
  label: string
  color: string
  type: 'sunrise' | 'sunset'
}

interface Props {
  streams: StreamPoint[]
  onHover: (index: number | null) => void
  onZoomChange?: (range: ZoomRange | null) => void
  sunriseAt?: string | null
  sunsetAt?: string | null
  splitMarkers?: SplitMarker[]
  /** Initial X-axis mode. Defaults to 'distance'. Pass 'time' for indoor/no-GPS activities. */
  defaultXMode?: XMode
  /** Distanzbereich (in km) der im Höhenprofil als markArea hervorgehoben wird. */
  highlightRange?: { startKm: number; endKm: number } | null
  /** Speed/Pace toggle mode: 'speed' = km/h, 'pace' = min/km (inverted Y axis). */
  speedMode?: 'speed' | 'pace'
  /** Sport type used for tab-based chart filtering. */
  sportType?: string
  /** Active tab from ChartTabs. If set, only shows charts relevant to that tab. */
  activeTab?: string
}

function computeSunMarkLines(
  streams: StreamPoint[],
  data: ChartData,
  xMode: XMode,
  sunriseAt?: string | null,
  sunsetAt?: string | null,
): SunMarkLine[] {
  if (streams.length === 0) return []
  const marks: SunMarkLine[] = []

  const t0 = new Date(streams[0].time).getTime()
  const tEnd = new Date(streams[streams.length - 1].time).getTime()

  const addMark = (iso: string, label: string, color: string, markType: 'sunrise' | 'sunset') => {
    const t = new Date(iso).getTime()
    if (t < t0 || t > tEnd) return
    // Find closest stream index by time
    let bestIdx = 0
    let bestDiff = Math.abs(new Date(streams[0].time).getTime() - t)
    for (let i = 1; i < streams.length; i++) {
      const d = Math.abs(new Date(streams[i].time).getTime() - t)
      if (d < bestDiff) { bestDiff = d; bestIdx = i }
    }
    const xVal = xMode === 'distance' ? data.distanceKm[bestIdx] : data.elapsedMin[bestIdx]
    marks.push({ xAxis: xVal, label, color, type: markType })
  }

  if (sunriseAt) addMark(sunriseAt, '\u2600', '#f59e0b', 'sunrise')  // amber
  if (sunsetAt) addMark(sunsetAt, '\u263E', '#e67e22', 'sunset')     // warm orange

  return marks
}

// ── Main component ─────────────────────────────────────────────────────────────

export default function ActivityCharts({ streams, onHover, onZoomChange, sunriseAt, sunsetAt, splitMarkers, defaultXMode, highlightRange, speedMode, sportType, activeTab }: Props) {
  const data = useMemo(() => buildChartData(streams), [streams])
  const [isZoomed, setIsZoomed] = useState(false)
  const [xMode, setXMode] = useState<XMode>(defaultXMode ?? 'distance')

  const sunMarkLines = useMemo(
    () => computeSunMarkLines(streams, data, xMode, sunriseAt, sunsetAt),
    [streams, data, xMode, sunriseAt, sunsetAt],
  )

  const chartInstancesRef = useRef<Map<string, echarts.ECharts>>(new Map())
  const isSyncingRef = useRef(false)
  // Shared lock across all ChartPanel instances to prevent cascading brushEnd
  // events from echarts.connect propagation (see brushEnd handler comment).
  const brushZoomLockRef = useRef(false)

  const registerChart = useCallback((id: string, chart: echarts.ECharts) => {
    chartInstancesRef.current.set(id, chart)
    chart.group = 'activity-charts'
    echarts.connect('activity-charts')
  }, [])

  const unregisterChart = useCallback((id: string) => {
    chartInstancesRef.current.delete(id)
  }, [])

  // ── Climb highlight: markArea on altitude chart ─────────────────────────────
  // With showGradeColor there are two series: series[0] = colored area, series[1] = green line.
  // markArea goes on the last series (which carries markLine/markPoint decorations).
  useEffect(() => {
    const altChart = chartInstancesRef.current.get('altitude')
    if (!altChart) return
    const opt = altChart.getOption() as { series?: unknown[] }
    const seriesCount = opt.series?.length ?? 1
    const markAreaSeries = Array.from({ length: seriesCount }, (_, i) =>
      i === seriesCount - 1
        ? {
            markArea: highlightRange
              ? {
                  silent: true,
                  itemStyle: { color: 'rgba(249, 115, 22, 0.3)' },
                  data: [[{ xAxis: highlightRange.startKm }, { xAxis: highlightRange.endKm }]],
                }
              : { data: [] },
          }
        : {},
    )
    altChart.setOption({ series: markAreaSeries })
  }, [highlightRange])

  // ── Helper: convert percentage-based zoom range to km + stream indices for map
  const pctToZoomRange = useCallback((startPct: number, endPct: number): ZoomRange => {
    const xArr = xMode === 'distance' ? data.distanceKm : data.elapsedMin
    const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
    const startVal = (startPct / 100) * xMax
    const endVal = (endPct / 100) * xMax

    const startIdx = findClosestIdx(xArr, startVal)
    const endIdx = findClosestIdx(xArr, endVal)

    let startKm: number, endKm: number
    if (xMode === 'distance') {
      startKm = startVal
      endKm = endVal
    } else {
      startKm = data.distanceKm[startIdx] ?? 0
      endKm = data.distanceKm[endIdx] ?? data.maxDist
    }
    return { startKm, endKm, startPct, endPct, startIdx, endIdx }
  }, [xMode, data])

  // Dedup guard — prevents cascading renders from echarts.connect propagation
  const lastSyncRef = useRef<{ s: number; e: number } | null>(null)

  // ── Sync zoom across charts (mouse wheel / pan) ──────────────────────────
  const syncZoom = useCallback((start: number, end: number, sourceId: string) => {
    if (isSyncingRef.current) return
    // Dedup: skip if same values (echarts.connect fires events on all connected charts)
    const rs = Math.round(start * 100)
    const re = Math.round(end * 100)
    if (lastSyncRef.current && lastSyncRef.current.s === rs && lastSyncRef.current.e === re) return
    lastSyncRef.current = { s: rs, e: re }

    isSyncingRef.current = true
    chartInstancesRef.current.forEach((chart, id) => {
      if (id !== sourceId) {
        chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start, end })
      }
    })

    const zoomed = start > 0.5 || end < 99.5
    setIsZoomed(zoomed)
    onZoomChange?.(zoomed ? pctToZoomRange(start, end) : null)

    isSyncingRef.current = false
  }, [onZoomChange, pctToZoomRange])

  // ── Brush zoom: receives DATA VALUES, converts to percentage start/end ────
  // Percentage-based dispatch is reliable with filterMode:'none' on value axes.
  // After zooming, brush mode is deactivated so drag-to-pan works.
  const handleBrushZoomFn = useCallback((startVal: number, endVal: number) => {
    const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
    if (xMax <= 0) return

    const startPct = Math.max(0, (startVal / xMax) * 100)
    const endPct = Math.min(100, (endVal / xMax) * 100)
    if (endPct - startPct < 0.5) return // ignore tiny selections

    // Set dedup guard BEFORE dispatching so echarts.connect echoes are skipped
    lastSyncRef.current = { s: Math.round(startPct * 100), e: Math.round(endPct * 100) }

    isSyncingRef.current = true
    chartInstancesRef.current.forEach(chart => {
      chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: startPct, end: endPct })
      chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: false },
      })
    })
    isSyncingRef.current = false

    setIsZoomed(true)

    // Compute stream indices + km range for the map highlight
    const xArr = xMode === 'distance' ? data.distanceKm : data.elapsedMin
    const startIdx = findClosestIdx(xArr, startVal)
    const endIdx = findClosestIdx(xArr, endVal)

    let startKm: number, endKm: number
    if (xMode === 'distance') {
      startKm = startVal
      endKm = endVal
    } else {
      startKm = data.distanceKm[startIdx] ?? 0
      endKm = data.distanceKm[endIdx] ?? data.maxDist
    }
    onZoomChange?.({ startKm, endKm, startPct, endPct, startIdx, endIdx })
  }, [xMode, data, onZoomChange])

  // Stable ref so ChartPanel closures always call the latest version
  const brushHandlerRef = useRef(handleBrushZoomFn)
  useEffect(() => { brushHandlerRef.current = handleBrushZoomFn }, [handleBrushZoomFn])
  const stableBrushZoom = useCallback((s: number, e: number) => brushHandlerRef.current(s, e), [])

  // ── Reset zoom: restore full view, reactivate brush ─────────────────────
  const handleResetZoom = useCallback(() => {
    lastSyncRef.current = null
    isSyncingRef.current = true
    chartInstancesRef.current.forEach(chart => {
      chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
      // Reactivate brush so the user can draw a new selection
      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: 'lineX', brushMode: 'single' },
      })
    })
    isSyncingRef.current = false
    setIsZoomed(false)
    onZoomChange?.(null)
  }, [onZoomChange])

  const switchXMode = useCallback((mode: XMode) => {
    if (isZoomed) {
      lastSyncRef.current = null
      isSyncingRef.current = true
      chartInstancesRef.current.forEach(chart => {
        chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
        chart.dispatchAction({
          type: 'takeGlobalCursor',
          key: 'brush',
          brushOption: { brushType: 'lineX', brushMode: 'single' },
        })
      })
      isSyncingRef.current = false
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
    onSyncZoom: syncZoom,
    onRegister: registerChart,
    onUnregister: unregisterChart,
    brushZoomLockRef,
    xMode,
    splitMarkers,
  }

  // Tab-based filtering: tabCharts === null means "show all"
  const tabCharts = activeTab ? getTabCharts(activeTab, sportType ?? '') : null
  const show = (key: ChartKey) => tabCharts === null || tabCharts.includes(key)

  // Build all panels that have data, always. Wrap each in a visibility div so
  // ECharts instances are never destroyed on tab switch — only hidden via CSS.
  const panels: React.ReactNode[] = []

  if (hasValidData(data.altitude)) panels.push(
    <div key={k('altitude')} style={{ display: show('altitude') ? undefined : 'none' }}>
      <ChartPanel chartId="altitude" {...common}
        field="altitude" label="Höhe" unit="m" color="#22c55e" area
        markLines="minmax" altitudeSmartY showGradeColor
        sunMarkLines={sunMarkLines} />
    </div>,
  )
  if (hasValidData(data.heartRate)) panels.push(
    <div key={k('hr')} style={{ display: show('heartRate') ? undefined : 'none' }}>
      <ChartPanel chartId="hr" {...common}
        field="heartRate" label="Herzfrequenz" unit="bpm" color="#ef4444"
        markLines="max" />
    </div>,
  )
  if (hasValidData(data.power)) panels.push(
    <div key={k('power')} style={{ display: show('power') ? undefined : 'none' }}>
      <ChartPanel chartId="power" {...common}
        field="power" label="Leistung" unit="W" color="#a855f7" />
    </div>,
  )
  if (hasValidData(data.cadence)) panels.push(
    <div key={k('cadence')} style={{ display: show('cadence') ? undefined : 'none' }}>
      <ChartPanel chartId="cadence" {...common}
        field="cadence" label={sportType ? fieldLabel('cadence', sportType) : 'Kadenz'} unit={sportType ? fieldUnit('cadence', sportType) : 'rpm'} color="#f97316" />
    </div>,
  )
  if (hasValidData(data.speed)) {
    // Speed/Pace: always render one panel with stable key to preserve ECharts instance
    const sg = sportType ? getSegmentGroup(sportType) : 'generic'
    const isRowingSport = sportType === 'rowing'
    const isSwimSport = sg === 'swim'

    if (speedMode === 'pace') {
      const divisor = isRowingSport ? 500 : isSwimSport ? 100 : 1000
      const paceUnit = isRowingSport ? 'min/500m' : isSwimSport ? 'min/100m' : 'min/km'
      // Clamp pace to max 30 min/km (= walking ~2 km/h) to avoid Y-axis explosion at stops
      const maxPace = 30
      const paceValues = data.speedMs.map(ms => {
        if (!ms || ms <= 0 || !isFinite(ms)) return NaN
        const pace = divisor / ms / 60
        return pace > maxPace ? NaN : pace
      })
      const paceFmt = (minPer: number): string => {
        if (isNaN(minPer) || !isFinite(minPer)) return '—'
        let min = Math.floor(minPer)
        let sec = Math.round((minPer - min) * 60)
        if (sec >= 60) { min += 1; sec = 0 }
        return `${min}:${sec.toString().padStart(2, '0')}\u202f${paceUnit}`
      }
      panels.push(
        <div key={k('speed')} style={{ display: show('speed') ? undefined : 'none' }}>
          <ChartPanel chartId="speed" {...common}
            field="speed" label="Pace" unit={paceUnit} color="#3b82f6"
            overrideValues={paceValues} invertY tooltipValueFn={paceFmt} />
        </div>,
      )
    } else {
      panels.push(
        <div key={k('speed')} style={{ display: show('speed') ? undefined : 'none' }}>
          <ChartPanel chartId="speed" {...common}
            field="speed" label={sportType ? fieldLabel('speed', sportType) : 'Geschwindigkeit'} unit="km/h" color="#3b82f6" />
        </div>,
      )
    }
  }
  if (hasValidData(data.temperature)) panels.push(
    <div key={k('temperature')} style={{ display: show('temperature') ? undefined : 'none' }}>
      <ChartPanel chartId="temperature" {...common}
        field="temperature" label="Temperatur" unit="°C" color="#06b6d4"
        markLines="minmax" />
    </div>,
  )
  // Running Dynamics: dual-axis GCT + VO chart
  if (hasValidData(data.groundContactTime) || hasValidData(data.verticalOscillation)) panels.push(
    <div key={k('running-dynamics')} style={{ display: show('runningDynamics') ? undefined : 'none' }}>
      <RunningDynamicsChartPanel chartId="running-dynamics" {...common} />
    </div>,
  )
  // Step Length: single series
  if (hasValidData(data.stepLength)) panels.push(
    <div key={k('step-length')} style={{ display: show('stepLength') ? undefined : 'none' }}>
      <ChartPanel chartId="step-length" {...common}
        field="stepLength" label="Schrittlänge" unit="m" color="#0891b2"
        overrideValues={data.stepLength} />
    </div>,
  )
  // Pedal Analytics: Torque Effectiveness L/R
  if (hasValidData(data.torqueEffL) || hasValidData(data.torqueEffR)) panels.push(
    <div key={k('torque-eff')} style={{ display: show('torqueEff') ? undefined : 'none' }}>
      <PedalDualChartPanel chartId="torque-eff" {...common}
        leftValues={data.torqueEffL} rightValues={data.torqueEffR}
        leftLabel="Drehmoment-Eff. Links" rightLabel="Drehmoment-Eff. Rechts"
        leftColor="#EC4899" rightColor="#A855F7"
        leftAxisName="Links\n(%)" rightAxisName="Rechts\n(%)"
        tooltipTitle="Drehmoment-Eff." />
    </div>,
  )
  // Pedal Analytics: Pedal Smoothness L/R
  if (hasValidData(data.pedalSmL) || hasValidData(data.pedalSmR)) panels.push(
    <div key={k('pedal-sm')} style={{ display: show('pedalSm') ? undefined : 'none' }}>
      <PedalDualChartPanel chartId="pedal-sm" {...common}
        leftValues={data.pedalSmL} rightValues={data.pedalSmR}
        leftLabel="Tritt-Gleichm. Links" rightLabel="Tritt-Gleichm. Rechts"
        leftColor="#A855F7" rightColor="#8B5CF6"
        leftAxisName="Links\n(%)" rightAxisName="Rechts\n(%)"
        tooltipTitle="Tritt-Gleichm." />
    </div>,
  )
  // E-Bike: Motor Power
  if (hasValidData(data.motorPower)) panels.push(
    <div key={k('motor-power')} style={{ display: show('motorPower') ? undefined : 'none' }}>
      <ChartPanel chartId="motor-power" {...common}
        field="power" label="Motorleistung" unit="W" color="#9333EA"
        overrideValues={data.motorPower} />
    </div>,
  )
  // E-Bike: Battery Level (fixed 0–100 % Y axis)
  if (hasValidData(data.batteryLevel)) panels.push(
    <div key={k('battery')} style={{ display: show('batteryLevel') ? undefined : 'none' }}>
      <ChartPanel chartId="battery" {...common}
        field="power" label="Akku-Level" unit="%" color="#22C55E"
        overrideValues={data.batteryLevel} yAxisFixed={[0, 100]} />
    </div>,
  )
  // E-Bike: Travel Range
  if (hasValidData(data.travelRange)) panels.push(
    <div key={k('travel-range')} style={{ display: show('travelRange') ? undefined : 'none' }}>
      <ChartPanel chartId="travel-range" {...common}
        field="power" label="Reichweite" unit="km" color="#6B7280"
        overrideValues={data.travelRange} />
    </div>,
  )
  // Dive: Depth profile — Y axis inverted so surface = top, depth = bottom
  if (hasValidData(data.depth)) panels.push(
    <div key={k('depth')} style={{ display: show('depth') ? undefined : 'none' }}>
      <ChartPanel chartId="depth" {...common}
        field="altitude" label="Tiefe" unit="m" color="#3B82F6" area
        overrideValues={data.depth} invertY />
    </div>,
  )
  // Cycle Length (stroke/stroke distance) — only for swim + paddle sports
  // For running: step_length_mm chart already exists. For cycling: cycle_length = pedal revolution (not useful as chart).
  {
    const sg = sportType ? getSegmentGroup(sportType) : 'generic'
    if ((sg === 'swim' || sg === 'paddle') && hasValidData(data.cycleLength)) {
      const cycleLengthLabel = sg === 'swim' ? 'Distanz/Zug' : 'Distanz/Schlag'
      panels.push(
        <div key={k('cycle-length')} style={{ display: show('cycleLength') ? undefined : 'none' }}>
          <ScatterChartPanel chartId="cycle-length" {...common}
            values={data.cycleLength} label={cycleLengthLabel} unit="m" color="#0EA5E9" />
        </div>,
      )
    }
  }

  if (panels.length === 0) return null

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Datenströme</h2>
        <div className="flex items-center gap-2">
          {isZoomed && (
            <button onClick={handleResetZoom}
              className="cursor-pointer text-sm px-3 py-1 rounded-md bg-accent-100 text-orange-700 hover:bg-orange-200
                         dark:bg-orange-900/30 dark:text-orange-300 dark:hover:bg-orange-900/50 transition-colors">
              ✕ Zoom zurücksetzen
            </button>
          )}
          <div className="flex rounded-lg border border-neutral-200 dark:border-neutral-700 overflow-hidden text-sm">
            <button onClick={() => switchXMode('distance')}
              className={`cursor-pointer px-3 py-1 transition-colors ${xMode === 'distance'
                ? 'bg-neutral-800 text-white dark:bg-neutral-200 dark:text-neutral-900'
                : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'}`}>
              km
            </button>
            <button onClick={() => switchXMode('time')}
              className={`cursor-pointer px-3 py-1 transition-colors border-l border-neutral-200 dark:border-neutral-700 ${xMode === 'time'
                ? 'bg-neutral-800 text-white dark:bg-neutral-200 dark:text-neutral-900'
                : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'}`}>
              Zeit
            </button>
          </div>
        </div>
      </div>
      {panels}
      <p className="text-xs text-neutral-400 dark:text-neutral-600">
        Ziehen zum Auswählen · Mausrad zum Zoomen · Nach Auswahl: Ziehen zum Verschieben
      </p>
    </div>
  )
}

// ── Pedal Analytics Dual-Axis Chart Panel ─────────────────────────────────────

interface PedalDualChartPanelProps {
  chartId: string
  data: ChartData
  leftValues: number[]
  rightValues: number[]
  leftLabel: string
  rightLabel: string
  leftColor: string
  rightColor: string
  leftAxisName: string
  rightAxisName: string
  tooltipTitle: string
  onHover: (index: number | null) => void
  onBrushZoom: (startValue: number, endValue: number) => void
  onSyncZoom: (start: number, end: number, sourceId: string) => void
  onRegister: (id: string, chart: echarts.ECharts) => void
  onUnregister: (id: string) => void
  brushZoomLockRef: React.MutableRefObject<boolean>
  xMode: XMode
  splitMarkers?: SplitMarker[]
}

function PedalDualChartPanel({
  chartId, data, leftValues, rightValues, leftLabel, rightLabel,
  leftColor, rightColor, leftAxisName, rightAxisName, tooltipTitle,
  onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister,
  brushZoomLockRef, xMode, splitMarkers,
}: PedalDualChartPanelProps) {
  const containerRef = useECharts(
    () => {
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

      const gridTop = (splitMarkers && splitMarkers.length > 0) ? 34 : 8
      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      const mlData: Record<string, unknown>[] = []
      if (splitMarkers && splitMarkers.length > 0) {
        for (const marker of splitMarkers) {
          mlData.push({
            xAxis: xMode === 'distance' ? marker.distanceKm : marker.elapsedMin,
            lineStyle: { color: '#94a3b8', type: 'dashed', width: 1, opacity: 0.8 },
            label: {
              show: true,
              position: 'insideStartTop',
              formatter: marker.label,
              fontSize: 10,
              color: '#94a3b8',
            },
          })
        }
      }

      return {
        animation: false,
        grid: { left: 52, right: 76, top: gridTop, bottom: 36 },

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
            const idx = p.dataIndex
            const kmStr = (data.distanceKm[idx] ?? 0).toFixed(1)
            const elapsed = data.elapsedMin[idx] ?? 0
            const timeStr = formatElapsed(elapsed, data.maxElapsed)
            const posLine = xMode === 'distance'
              ? `${kmStr}\u202fkm`
              : `${timeStr}\u2009·\u2009${kmStr}\u202fkm`
            let html = `<div style="line-height:1.75;min-width:160px">`
            html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
            html += `<div style="font-size:11px;color:#888;margin-bottom:2px">${tooltipTitle}</div>`
            const lv = leftValues[idx]
            if (!isNaN(lv)) html += `<div style="font-size:13px;color:${leftColor}"><strong>${lv.toFixed(1)}\u202f%</strong> Links</div>`
            const rv = rightValues[idx]
            if (!isNaN(rv)) html += `<div style="font-size:13px;color:${rightColor}"><strong>${rv.toFixed(1)}\u202f%</strong> Rechts</div>`
            return html + '</div>'
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
          min: 0,
          max: 100,
          name: '%',
          nameTextStyle: { fontSize: 10, color: '#666' },
          axisLabel: { fontSize: 11, color: '#666', margin: 8, formatter: (v: number) => `${v}` },
          splitLine: { lineStyle: { color: '#eee', type: 'dashed' } },
          axisLine: { show: false },
          axisTick: { show: false },
        },

        series: [
          {
            type: 'line',
            data: xData.map((x, i) => [x, leftValues[i]]),
            showSymbol: false,
            lineStyle: { width: 1.5, color: leftColor },
            itemStyle: { color: leftColor },
            connectNulls: false,
            ...(mlData.length > 0 ? {
              markLine: {
                silent: true,
                symbol: ['none', 'none'],
                data: mlData,
              },
            } : {}),
          },
          {
            type: 'line',
            data: xData.map((x, i) => [x, rightValues[i]]),
            showSymbol: false,
            lineStyle: { width: 1.5, color: rightColor },
            itemStyle: { color: rightColor },
            connectNulls: false,
          },
        ],

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
      } as echarts.EChartsCoreOption
    },
    [data, xMode, splitMarkers, leftValues, rightValues, leftColor, rightColor, tooltipTitle], // eslint-disable-line react-hooks/exhaustive-deps
    (chart) => {
      onRegister(chartId, chart)

      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: 'lineX', brushMode: 'single' },
      })

      chart.on('showTip', (params: unknown) => {
        const typed = params as { dataIndex?: number }
        if (typed.dataIndex != null) onHover(typed.dataIndex)
      })
      chart.on('hideTip', () => onHover(null))

      type BrushArea = { range?: [number, number]; coordRange?: [number, number] }

      chart.on('brushEnd', (params: unknown) => {
        if (brushZoomLockRef.current) return
        brushZoomLockRef.current = true

        const p = params as {
          areas?: BrushArea[]
          batch?: Array<{ areas?: BrushArea[] }>
        }
        const areas = p.areas ?? p.batch?.[0]?.areas
        if (!areas || areas.length === 0) {
          brushZoomLockRef.current = false
          return
        }

        const area = areas[0]
        let startData: number
        let endData: number

        if (area.coordRange) {
          [startData, endData] = area.coordRange
        } else if (area.range) {
          const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
          const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
          if (s == null || e == null) { brushZoomLockRef.current = false; return }
          startData = s as number
          endData = e as number
        } else {
          brushZoomLockRef.current = false
          return
        }

        if (endData <= startData) { brushZoomLockRef.current = false; return }
        onBrushZoom(Math.max(0, startData), Math.min(xMax, endData))

        requestAnimationFrame(() => { brushZoomLockRef.current = false })
      })

      chart.on('dataZoom', (params: unknown) => {
        const p = params as {
          batch?: Array<{ start: number; end: number }>
          start?: number
          end?: number
        }
        const z = p.batch?.[0] ?? (p.start != null && p.end != null ? p as { start: number; end: number } : null)
        if (z != null) onSyncZoom(z.start, z.end, chartId)
      })

      return () => onUnregister(chartId)
    },
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-4 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        <div className="flex items-center gap-2">
          <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: leftColor }} />
          <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
            {leftLabel} <span className="font-normal text-neutral-400 dark:text-neutral-600">(%)</span>
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: rightColor }} />
          <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
            {rightLabel} <span className="font-normal text-neutral-400 dark:text-neutral-600">(%)</span>
          </span>
        </div>
      </div>
      <div ref={containerRef} className="h-[180px] w-full" />
    </div>
  )
}

// ── Running Dynamics Dual-Axis Chart Panel ─────────────────────────────────────

interface RunningDynamicsChartPanelProps {
  chartId: string
  data: ChartData
  onHover: (index: number | null) => void
  onBrushZoom: (startValue: number, endValue: number) => void
  onSyncZoom: (start: number, end: number, sourceId: string) => void
  onRegister: (id: string, chart: echarts.ECharts) => void
  onUnregister: (id: string) => void
  brushZoomLockRef: React.MutableRefObject<boolean>
  xMode: XMode
  splitMarkers?: SplitMarker[]
}

function RunningDynamicsChartPanel({
  chartId, data, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister,
  brushZoomLockRef, xMode, splitMarkers,
}: RunningDynamicsChartPanelProps) {
  const containerRef = useECharts(
    () => {
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
      const gctValues = data.groundContactTime
      const voValues = data.verticalOscillation

      const gridTop = (splitMarkers && splitMarkers.length > 0) ? 34 : 8
      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      const mlData: Record<string, unknown>[] = []
      if (splitMarkers && splitMarkers.length > 0) {
        for (const marker of splitMarkers) {
          mlData.push({
            xAxis: xMode === 'distance' ? marker.distanceKm : marker.elapsedMin,
            lineStyle: { color: '#94a3b8', type: 'dashed', width: 1, opacity: 0.8 },
            label: {
              show: true,
              position: 'insideStartTop',
              formatter: marker.label,
              fontSize: 10,
              color: '#94a3b8',
            },
          })
        }
      }

      return {
        animation: false,
        grid: { left: 52, right: 76, top: gridTop, bottom: 36 },

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
            const idx = p.dataIndex
            const kmStr = (data.distanceKm[idx] ?? 0).toFixed(1)
            const elapsed = data.elapsedMin[idx] ?? 0
            const timeStr = formatElapsed(elapsed, data.maxElapsed)
            const posLine = xMode === 'distance'
              ? `${kmStr}\u202fkm`
              : `${timeStr}\u2009·\u2009${kmStr}\u202fkm`
            let html = `<div style="line-height:1.75;min-width:150px">`
            html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
            const gct = gctValues[idx]
            if (!isNaN(gct)) html += `<div style="font-size:13px;color:#10B981"><strong>${Math.round(gct)}\u202fms</strong> GCT</div>`
            const vo = voValues[idx]
            if (!isNaN(vo)) html += `<div style="font-size:13px;color:#06B6D4"><strong>${vo.toFixed(1)}\u202fcm</strong> Vert. Oszi.</div>`
            return html + '</div>'
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

        yAxis: [
          {
            type: 'value',
            name: 'GCT\n(ms)',
            nameTextStyle: { fontSize: 10, color: '#10B981' },
            axisLabel: { fontSize: 11, color: '#10B981', margin: 8 },
            splitLine: { lineStyle: { color: '#eee', type: 'dashed' } },
            axisLine: { show: false },
            axisTick: { show: false },
          },
          {
            type: 'value',
            name: 'VO\n(cm)',
            nameTextStyle: { fontSize: 10, color: '#06B6D4' },
            axisLabel: { fontSize: 11, color: '#06B6D4', margin: 8 },
            splitLine: { show: false },
            axisLine: { show: false },
            axisTick: { show: false },
          },
        ],

        series: [
          {
            type: 'line',
            yAxisIndex: 0,
            data: xData.map((x, i) => [x, gctValues[i]]),
            showSymbol: false,
            lineStyle: { width: 1.5, color: '#10B981' },
            itemStyle: { color: '#10B981' },
            connectNulls: false,
            ...(mlData.length > 0 ? {
              markLine: {
                silent: true,
                symbol: ['none', 'none'],
                data: mlData,
              },
            } : {}),
          },
          {
            type: 'line',
            yAxisIndex: 1,
            data: xData.map((x, i) => [x, voValues[i]]),
            showSymbol: false,
            lineStyle: { width: 1.5, color: '#06B6D4' },
            itemStyle: { color: '#06B6D4' },
            connectNulls: false,
          },
        ],

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
      } as echarts.EChartsCoreOption
    },
    [data, xMode, splitMarkers], // eslint-disable-line react-hooks/exhaustive-deps
    (chart) => {
      onRegister(chartId, chart)

      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: 'lineX', brushMode: 'single' },
      })

      chart.on('showTip', (params: unknown) => {
        const typed = params as { dataIndex?: number }
        if (typed.dataIndex != null) onHover(typed.dataIndex)
      })
      chart.on('hideTip', () => onHover(null))

      type BrushArea = { range?: [number, number]; coordRange?: [number, number] }

      chart.on('brushEnd', (params: unknown) => {
        if (brushZoomLockRef.current) return
        brushZoomLockRef.current = true

        const p = params as {
          areas?: BrushArea[]
          batch?: Array<{ areas?: BrushArea[] }>
        }
        const areas = p.areas ?? p.batch?.[0]?.areas
        if (!areas || areas.length === 0) {
          brushZoomLockRef.current = false
          return
        }

        const area = areas[0]
        let startData: number
        let endData: number

        if (area.coordRange) {
          [startData, endData] = area.coordRange
        } else if (area.range) {
          const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
          const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
          if (s == null || e == null) { brushZoomLockRef.current = false; return }
          startData = s as number
          endData = e as number
        } else {
          brushZoomLockRef.current = false
          return
        }

        if (endData <= startData) { brushZoomLockRef.current = false; return }
        onBrushZoom(Math.max(0, startData), Math.min(xMax, endData))

        requestAnimationFrame(() => { brushZoomLockRef.current = false })
      })

      chart.on('dataZoom', (params: unknown) => {
        const p = params as {
          batch?: Array<{ start: number; end: number }>
          start?: number
          end?: number
        }
        const z = p.batch?.[0] ?? (p.start != null && p.end != null ? p as { start: number; end: number } : null)
        if (z != null) onSyncZoom(z.start, z.end, chartId)
      })

      return () => onUnregister(chartId)
    },
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-4 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        <div className="flex items-center gap-2">
          <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: '#10B981' }} />
          <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
            Bodenkontaktzeit <span className="font-normal text-neutral-400 dark:text-neutral-600">(ms)</span>
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: '#06B6D4' }} />
          <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
            Vertikale Bewegung <span className="font-normal text-neutral-400 dark:text-neutral-600">(cm)</span>
          </span>
        </div>
      </div>
      <div ref={containerRef} className="h-[180px] w-full" />
    </div>
  )
}

// ── Scatter Chart Panel (for sporadic data like stroke distance) ──────────────

interface ScatterChartPanelProps {
  chartId: string
  data: ChartData
  values: number[]
  label: string
  unit: string
  color: string
  onHover: (index: number | null) => void
  onBrushZoom: (startValue: number, endValue: number) => void
  onSyncZoom: (start: number, end: number, sourceId: string) => void
  onRegister: (id: string, chart: echarts.ECharts) => void
  onUnregister: (id: string) => void
  brushZoomLockRef: React.MutableRefObject<boolean>
  xMode: XMode
  splitMarkers?: SplitMarker[]
}

function ScatterChartPanel({
  chartId, data, values, label, unit, color,
  onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister,
  brushZoomLockRef, xMode, splitMarkers,
}: ScatterChartPanelProps) {
  const containerRef = useECharts(
    () => {
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

      const gridTop = (splitMarkers && splitMarkers.length > 0) ? 34 : 8
      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      // Average of non-NaN values for markLine
      const validValues = values.filter(v => !isNaN(v) && isFinite(v))
      const avg = validValues.length > 0
        ? validValues.reduce((a, b) => a + b, 0) / validValues.length
        : null

      // Split marker lines
      const mlData: Record<string, unknown>[] = []
      if (avg != null) {
        mlData.push({
          yAxis: avg,
          lineStyle: { color, type: 'dashed', width: 1.5, opacity: 0.7 },
          label: {
            show: true,
            position: 'insideEndTop',
            formatter: `Ø ${avg.toFixed(2)} ${unit}`,
            fontSize: 11,
            color,
            backgroundColor: 'rgba(255,255,255,0.88)',
            padding: [2, 5],
            borderRadius: 3,
          },
        })
      }
      if (splitMarkers && splitMarkers.length > 0) {
        for (const marker of splitMarkers) {
          mlData.push({
            xAxis: xMode === 'distance' ? marker.distanceKm : marker.elapsedMin,
            lineStyle: { color: '#94a3b8', type: 'dashed', width: 1, opacity: 0.8 },
            label: {
              show: true,
              position: 'insideStartTop',
              formatter: marker.label,
              fontSize: 10,
              color: '#94a3b8',
            },
          })
        }
      }

      return {
        animation: false,
        grid: { left: 52, right: 52, top: gridTop, bottom: 36 },

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
          trigger: 'item',
          transitionDuration: 0,
          confine: true,
          backgroundColor: 'rgba(255,255,255,0.96)',
          borderColor: '#e5e5e5',
          borderWidth: 1,
          padding: [8, 10],
          textStyle: { fontSize: 12, color: '#333' },
          formatter: (params: unknown) => {
            const p = params as { dataIndex?: number; data?: [number, number] }
            if (p?.dataIndex == null || !p.data) return ''
            const idx = p.dataIndex
            const val = p.data[1]
            const kmStr = (data.distanceKm[idx] ?? 0).toFixed(1)
            const elapsed = data.elapsedMin[idx] ?? 0
            const timeStr = formatElapsed(elapsed, data.maxElapsed)
            const posLine = xMode === 'distance'
              ? `${kmStr}\u202fkm`
              : `${timeStr}\u2009·\u2009${kmStr}\u202fkm`
            return `<div style="line-height:1.75;min-width:120px"><div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div><div style="font-size:13px;color:${color}"><strong>${!isNaN(val) ? val.toFixed(2) : '—'}\u202f${unit}</strong></div></div>`
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
          min: 0,
          name: unit,
          nameTextStyle: { fontSize: 10, color: '#666' },
          axisLabel: { fontSize: 11, color: '#666', margin: 8 },
          splitLine: { lineStyle: { color: '#eee', type: 'dashed' } },
          axisLine: { show: false },
          axisTick: { show: false },
        },

        series: [{
          type: 'scatter',
          data: xData.map((x, i) => [x, values[i]]),
          symbolSize: 3,
          itemStyle: { color, opacity: 0.7 },
          ...(mlData.length > 0 ? {
            markLine: {
              silent: true,
              symbol: ['none', 'none'],
              data: mlData,
            },
          } : {}),
        }],

        dataZoom: [{
          type: 'inside',
          xAxisIndex: 0,
          filterMode: 'none',
          zoomOnMouseWheel: true,
          moveOnMouseMove: true,
          moveOnMouseWheel: false,
        }],
      } as echarts.EChartsCoreOption
    },
    [data, values, xMode, splitMarkers],
    (chart) => {
      onRegister(chartId, chart)

      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: 'lineX', brushMode: 'single' },
      })

      chart.on('showTip', (params: unknown) => {
        const typed = params as { dataIndex?: number }
        if (typed.dataIndex != null) onHover(typed.dataIndex)
      })
      chart.on('hideTip', () => onHover(null))

      type BrushArea = { range?: [number, number]; coordRange?: [number, number] }

      chart.on('brushEnd', (params: unknown) => {
        if (brushZoomLockRef.current) return
        brushZoomLockRef.current = true

        const p = params as {
          areas?: BrushArea[]
          batch?: Array<{ areas?: BrushArea[] }>
        }
        const areas = p.areas ?? p.batch?.[0]?.areas
        if (!areas || areas.length === 0) {
          brushZoomLockRef.current = false
          return
        }

        const area = areas[0]
        let startData: number
        let endData: number

        if (area.coordRange) {
          [startData, endData] = area.coordRange
        } else if (area.range) {
          const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
          const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
          if (s == null || e == null) { brushZoomLockRef.current = false; return }
          startData = s as number
          endData = e as number
        } else {
          brushZoomLockRef.current = false
          return
        }

        if (endData <= startData) { brushZoomLockRef.current = false; return }
        onBrushZoom(Math.max(0, startData), Math.min(xMax, endData))

        requestAnimationFrame(() => { brushZoomLockRef.current = false })
      })

      chart.on('dataZoom', (params: unknown) => {
        const p = params as {
          batch?: Array<{ start: number; end: number }>
          start?: number
          end?: number
        }
        const z = p.batch?.[0] ?? (p.start != null && p.end != null ? p as { start: number; end: number } : null)
        if (z != null) onSyncZoom(z.start, z.end, chartId)
      })

      return () => onUnregister(chartId)
    },
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: color }} />
        <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
          {label} <span className="font-normal text-neutral-400 dark:text-neutral-600">({unit})</span>
        </span>
      </div>
      <div ref={containerRef} className="h-[180px] w-full" />
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
  onBrushZoom: (startValue: number, endValue: number) => void
  onSyncZoom: (start: number, end: number, sourceId: string) => void
  onRegister: (id: string, chart: echarts.ECharts) => void
  onUnregister: (id: string) => void
  brushZoomLockRef: React.MutableRefObject<boolean>
  markLines?: 'none' | 'max' | 'minmax'
  altitudeSmartY?: boolean
  /** Elevation chart: use grade_pct data for grade-based area colorization */
  showGradeColor?: boolean
  xMode: XMode
  sunMarkLines?: SunMarkLine[]
  splitMarkers?: SplitMarker[]
  /** Override the data values array (used for pace mode: pre-converted values). */
  overrideValues?: number[]
  /** Invert the Y axis (used for pace mode so slower pace = lower on screen). */
  invertY?: boolean
  /** Custom value formatter for tooltip (used for pace mode). */
  tooltipValueFn?: (v: number) => string
  /** Fix Y axis to a specific [min, max] range (e.g. [0, 100] for battery %). */
  yAxisFixed?: [number, number]
}

function ChartPanel({
  chartId, data, field, unit, label, color, area,
  onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister,
  brushZoomLockRef, markLines = 'none', altitudeSmartY, showGradeColor, xMode,
  sunMarkLines, splitMarkers, overrideValues, invertY, tooltipValueFn, yAxisFixed,
}: ChartPanelProps) {
  const containerRef = useECharts(
    () => {
      const values = overrideValues ?? data[field]
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

      // ── Y-axis smart scaling ─────────────────────────────────────────────────
      const yAxisExtra: Record<string, unknown> = {}
      if (altitudeSmartY) {
        const range = calcAltitudeYRange(values)
        if (range) { yAxisExtra.min = range.min; yAxisExtra.max = range.max }
      }
      if (yAxisFixed) { yAxisExtra.min = yAxisFixed[0]; yAxisExtra.max = yAxisFixed[1] }
      if (invertY) { yAxisExtra.inverse = true }

      // Extra top padding for sunrise/sunset icons or split marker labels above the chart
      const gridTop = (sunMarkLines && sunMarkLines.length > 0) || (splitMarkers && splitMarkers.length > 0) ? 34 : 8

      // ── Mark lines ──────────────────────────────────────────────────────────
      const mlData: Record<string, unknown>[] = []
      if (markLines === 'max' || markLines === 'minmax') {
        mlData.push({ type: 'max', name: 'Max', label: { position: 'insideMiddleBottom' } })
      }
      if (markLines === 'minmax') {
        mlData.push({ type: 'min', name: 'Min', label: { position: 'insideMiddleTop' } })
      }

      if (sunMarkLines && sunMarkLines.length > 0) {
        for (const sm of sunMarkLines) {
          mlData.push({
            xAxis: sm.xAxis,
            name: sm.label,
            label: { show: false },
            lineStyle: { type: 'dashed', color: sm.color, width: 1.5 },
          })
        }
      }

      if (splitMarkers && splitMarkers.length > 0) {
        for (const marker of splitMarkers) {
          mlData.push({
            xAxis: xMode === 'distance' ? marker.distanceKm : marker.elapsedMin,
            lineStyle: { color: '#94a3b8', type: 'dashed', width: 1, opacity: 0.8 },
            label: {
              show: true,
              position: 'insideStartTop',
              formatter: marker.label,
              fontSize: 10,
              color: '#94a3b8',
            },
          })
        }
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

        const valStr = tooltipValueFn ? tooltipValueFn(v) : `${v.toFixed(decimals)}\u2009${unit}`

        let html = `<div style="line-height:1.75;min-width:130px">`
        html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
        html += `<div style="font-size:13px"><strong>${valStr}</strong></div>`

        if (showGradeColor) {
          const gradeVal = data.grade[idx]
          if (gradeVal != null && !isNaN(gradeVal)) {
            const sign = gradeVal >= 0 ? '+' : ''
            const gc = Math.abs(gradeVal) > 10 ? '#dc2626' : Math.abs(gradeVal) > 4 ? '#ea580c' : '#16a34a'
            html += `<div style="color:${gc};font-size:12px;margin-top:1px">Steigung\u2009${sign}${gradeVal.toFixed(1)}\u202f%</div>`
          }
        }
        return html + '</div>'
      }

      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      return {
        animation: false,
        grid: { left: 52, right: 76, top: gridTop, bottom: 36 },

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

        series: showGradeColor
          ? [
              // Series 0: colored area driven by grade (dimension 2) via visualMap
              {
                type: 'line',
                data: xData.map((x, i) => [x, values[i], data.grade[i]]),
                showSymbol: false,
                lineStyle: { width: 0, color: 'transparent' },
                itemStyle: { color },
                areaStyle: { color: `${color}20`, opacity: 1 },
                connectNulls: false,
              },
              // Series 1: green line on top (no area, carries markLine/markPoint)
              {
                type: 'line',
                data: xData.map((x, i) => [x, values[i]]),
                showSymbol: false,
                lineStyle: { width: 1.5, color },
                itemStyle: { color },
                areaStyle: undefined,
                connectNulls: false,
                ...(mlData.length > 0 ? {
                  markLine: {
                    silent: true,
                    symbol: ['none', 'none'],
                    label: {
                      show: true,
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
                ...(sunMarkLines && sunMarkLines.length > 0 ? {
                  markPoint: {
                    data: sunMarkLines.map(sm => ({
                      coord: [sm.xAxis, yAxisExtra.max ?? Math.ceil(Math.max(...values.filter(v => !isNaN(v))))],
                      symbol: sm.type === 'sunrise' ? SUNRISE_SVG : SUNSET_SVG,
                      symbolSize: 24,
                      symbolOffset: [0, -14],
                      label: { show: false },
                    })),
                    animation: false,
                  },
                } : {}),
              },
            ]
          : [
              {
                type: 'line',
                data: xData.map((x, i) => [x, values[i]]),
                showSymbol: false,
                lineStyle: { width: 1.5, color },
                itemStyle: { color },
                areaStyle: area ? { color: `${color}20` } : undefined,
                connectNulls: false,
                ...(mlData.length > 0 ? {
                  markLine: {
                    silent: true,
                    symbol: ['none', 'none'],
                    label: {
                      show: true,
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
                ...(sunMarkLines && sunMarkLines.length > 0 ? {
                  markPoint: {
                    data: sunMarkLines.map(sm => ({
                      coord: [sm.xAxis, yAxisExtra.max ?? Math.ceil(Math.max(...values.filter(v => !isNaN(v))))],
                      symbol: sm.type === 'sunrise' ? SUNRISE_SVG : SUNSET_SVG,
                      symbolSize: 24,
                      symbolOffset: [0, -14],
                      label: { show: false },
                    })),
                    animation: false,
                  },
                } : {}),
              },
            ],

        ...(showGradeColor ? {
          visualMap: [{
            show: false,
            dimension: 2,  // grade_pct
            seriesIndex: 0,  // the colored area series
            pieces: [
              { gt: -100, lte: -12, color: 'rgba(124,58,237,0.50)' },   // Very steep downhill → Purple
              { gt: -12,  lte: -5,  color: 'rgba(59,130,246,0.40)' },   // Steep downhill → Blue
              { gt: -5,   lte: -2,  color: 'rgba(96,165,250,0.30)' },   // Moderate downhill → Light blue
              { gt: -2,   lte: 2,   color: 'rgba(34,197,94,0.25)' },    // Flat → Green
              { gt: 2,    lte: 5,   color: 'rgba(251,191,36,0.35)' },   // Moderate uphill → Yellow
              { gt: 5,    lte: 12,  color: 'rgba(249,115,22,0.45)' },   // Steep uphill → Orange
              { gt: 12,   lte: 20,  color: 'rgba(220,38,38,0.55)' },    // Very steep → Red
              { gt: 20,             color: 'rgba(153,27,27,0.65)' },    // Extreme → Dark red
            ],
          }],
        } : {}),

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
      } as echarts.EChartsCoreOption
    },
    [data, field, xMode, splitMarkers, showGradeColor, overrideValues, invertY, yAxisFixed], // eslint-disable-line react-hooks/exhaustive-deps
    (chart) => {
      onRegister(chartId, chart)

      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

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

      // Brush areas type
      type BrushArea = { range?: [number, number]; coordRange?: [number, number] }

      // Brush gesture ended → zoom all charts + map
      // Uses shared brushZoomLockRef to prevent cascading: dataZoom dispatch
      // triggers brushEnd again on all echarts.connect'd charts with the SAME
      // pixel range but a DIFFERENT axis scale → exponential zoom-in.
      chart.on('brushEnd', (params: unknown) => {
        if (brushZoomLockRef.current) return
        brushZoomLockRef.current = true

        const p = params as {
          areas?: BrushArea[]
          batch?: Array<{ areas?: BrushArea[] }>
        }
        const areas = p.areas ?? p.batch?.[0]?.areas
        if (!areas || areas.length === 0) {
          brushZoomLockRef.current = false
          return
        }

        const area = areas[0]
        let startData: number
        let endData: number

        if (area.coordRange) {
          [startData, endData] = area.coordRange
        } else if (area.range) {
          const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
          const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
          if (s == null || e == null) { brushZoomLockRef.current = false; return }
          startData = s as number
          endData = e as number
        } else {
          brushZoomLockRef.current = false
          return
        }

        if (endData <= startData) { brushZoomLockRef.current = false; return }
        onBrushZoom(Math.max(0, startData), Math.min(xMax, endData))

        requestAnimationFrame(() => { brushZoomLockRef.current = false })
      })

      // Mouse-wheel / pan → sync all other charts + update map.
      chart.on('dataZoom', (params: unknown) => {
        const p = params as {
          batch?: Array<{ start: number; end: number }>
          start?: number
          end?: number
        }
        const z = p.batch?.[0] ?? (p.start != null && p.end != null ? p as { start: number; end: number } : null)
        if (z != null) onSyncZoom(z.start, z.end, chartId)
      })

      return () => onUnregister(chartId)
    },
  )

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
