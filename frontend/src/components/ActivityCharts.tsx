import { useRef, useEffect, useMemo, useCallback, useState } from 'react'
import * as echarts from 'echarts/core'
import { useQuery } from '@tanstack/react-query'
import { useECharts } from '@/hooks/useECharts'
import { LineChart, ScatterChart, GaugeChart } from 'echarts/charts'
import { getSegmentGroup } from '@/lib/sportTaxonomy'
import { fieldLabel, fieldUnit, defaultSpeedMode, type FieldKey } from '@/lib/sportFieldLabels'
import { getDevFieldLabel, getDevFieldUnit } from '@/lib/developerFieldUnits'
import {
  GridComponent,
  TitleComponent,
  TooltipComponent,
  LegendComponent,
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
import type { StreamPoint, ActivityEvent, DeveloperFieldInfo } from '@/api/types'
import { fetchDeveloperFields } from '@/api/activities'
import { CHART_TAB } from '@/components/activity/ChartTabs'
import { DiveDecoPanel } from '@/components/activity/DiveDecoPanel'
import { GlucoseChartPanel } from '@/components/activity/GlucoseChartPanel'
import { findClosestIdx } from '@/lib/utils'
import { re3GradeAdjustedSpeed } from '@/lib/re3'

echarts.use([
  LineChart,
  ScatterChart,
  GaugeChart,
  GridComponent,
  TitleComponent,
  TooltipComponent,
  LegendComponent,
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

type XMode = 'distance' | 'time'

type ChartKey = 'altitude' | 'heartRate' | 'speed' | 'power' | 'cadence' | 'temperature' | 'coreTemperature' | 'runningDynamics' | 'stepLength' | 'runningSmoothness' | 'powerPhase' | 'torqueEff' | 'pedalSm' | 'motorPower' | 'batteryLevel' | 'travelRange' | 'depth' | 'cycleLength' | 'grit' | 'flow' | 'gear' | 'potentialStamina' | 'staminaCurrent' | 'pressure' | 'decoTimeSeries'

interface ChartMeta {
  id: string
  label: string
  color: string
  tabKey: ChartKey | null
}

function getTabCharts(tab: string, sportType: string): ChartKey[] | null {
  if (tab === CHART_TAB.alles) return null // show all
  const isSwim = getSegmentGroup(sportType) === 'swim'
  switch (tab) {
    case CHART_TAB.uebersicht: {
      if (sportType === 'diving') return ['depth', 'heartRate', 'temperature']
      if (isSwim) return ['heartRate', 'cycleLength']
      return ['altitude', 'heartRate', 'speed']
    }
    case CHART_TAB.leistung: {
      const sg = getSegmentGroup(sportType)
      const charts: ChartKey[] = ['power', 'cadence']
      if (sg === 'swim' || sg === 'paddle') charts.push('cycleLength')
      if (sg === 'bike') charts.push('gear')
      return charts
    }
    case CHART_TAB.dynamik: return ['runningDynamics', 'stepLength', 'runningSmoothness']
    case CHART_TAB.pedal: return ['powerPhase', 'torqueEff', 'pedalSm']
    case CHART_TAB.electric: return ['motorPower', 'batteryLevel', 'travelRange']
    case CHART_TAB.tauchgang: return ['depth']
    case CHART_TAB.sensoren: return ['temperature', 'coreTemperature']
    case CHART_TAB.mtb: return ['grit', 'flow']
    case CHART_TAB.ausdauer: return ['potentialStamina', 'staminaCurrent']
    case CHART_TAB.barometer: return ['pressure']
    case CHART_TAB.deko: return ['depth', 'decoTimeSeries']
    case CHART_TAB.glucose: return []
    case CHART_TAB.developerFields: return []
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
  coreTemperature: number[]
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
  grit: number[]
  flow: number[]
  potentialStamina: number[]
  staminaCurrent: number[]
  pressure: number[]
  paceMinKm: number[]
  garminGapMinKm: number[]
  re3Gap: number[]
  strydNgpMinKm: number[]
  corosEffortPaceMinKm: number[]
  strydPower: number[]
  ndlMin: number[]
  cnsPct: number[]
  n2Pct: number[]
  decoStopM: number[]
  po2: number[]
  runningSmoothness: number[]
  maxDist: number
  maxElapsed: number
}

// ── Data processing ────────────────────────────────────────────────────────────

/** Convert m/s to min/km. Returns NaN for speeds ≤ 0.3 m/s (stationary noise). */
function msToMinPerKm(ms: number | null | undefined): number {
  return ms != null && ms > 0.3 ? 1000 / ms / 60 : NaN
}

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
  const coreTemperature: number[] = []
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
  const grit: number[] = []
  const flow: number[] = []
  const potentialStamina: number[] = []
  const staminaCurrent: number[] = []
  const pressure: number[] = []
  const paceMinKm: number[] = []
  const garminGapMinKm: number[] = []
  const re3Gap: number[] = []
  const strydNgpMinKm: number[] = []
  const corosEffortPaceMinKm: number[] = []
  const strydPower: number[] = []
  const ndlMin: number[] = []
  const cnsPct: number[] = []
  const n2Pct: number[] = []
  const decoStopM: number[] = []
  const po2: number[] = []
  const runningSmoothness: number[] = []

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
    coreTemperature.push(s.core_temperature_c ?? NaN)
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
    grit.push(s.grit ?? NaN)
    flow.push(s.flow ?? NaN)
    potentialStamina.push(s.potential_stamina ?? NaN)
    staminaCurrent.push(s.stamina ?? NaN)
    pressure.push(s.absolute_pressure_pa != null ? s.absolute_pressure_pa / 100 : NaN) // Pa → hPa

    // GAP comparison: Pace, Garmin GAP, RE3 GAP — all in min/km
    const speedVal = s.speed_ms ?? 0
    paceMinKm.push(speedVal > 0 ? 1000 / speedVal / 60 : NaN)
    garminGapMinKm.push(s.gap_speed_ms != null && s.gap_speed_ms > 0 ? 1000 / s.gap_speed_ms / 60 : NaN)
    if (speedVal > 0 && s.grade_pct != null) {
      const gradeVal = s.grade_pct / 100
      const gasMs = re3GradeAdjustedSpeed(speedVal, gradeVal)
      re3Gap.push(gasMs > 0 ? 1000 / gasMs / 60 : NaN)
    } else {
      re3Gap.push(NaN)
    }

    // Stryd developer fields
    strydNgpMinKm.push(msToMinPerKm(s.developer_fields?.['ngp'] as number | undefined))

    // Coros developer fields — "Effort Pace" in m/s like GAP
    corosEffortPaceMinKm.push(msToMinPerKm(s.developer_fields?.['Effort Pace'] as number | undefined))
    const strydPowerVal = s.developer_fields?.['Power'] as number | undefined
    strydPower.push(strydPowerVal != null && strydPowerVal > 0 ? strydPowerVal : NaN)

    // Deco data (diving)
    ndlMin.push(s.ndl_time_s != null ? Math.min(Math.round(s.ndl_time_s / 60), 60) : NaN)
    cnsPct.push(s.cns_load_pct ?? NaN)
    n2Pct.push(s.n2_load_pct ?? NaN)
    decoStopM.push((s.next_stop_depth_m ?? 0) > 0 ? s.next_stop_depth_m! : NaN)
    po2.push(s.po2 != null && s.po2 > 0 ? s.po2 : NaN)
    runningSmoothness.push(s.running_smoothness_pct ?? NaN)
  }

  const validDist = distanceKm.filter(v => !isNaN(v) && isFinite(v))
  const rawMax = validDist.length > 0 ? Math.max(...validDist) : 1
  const maxDist = Math.ceil(rawMax * 10) / 10
  const maxElapsed = elapsedMin.length > 0 ? Math.ceil(elapsedMin[elapsedMin.length - 1]) : 0

  return { distanceKm, elapsedMin, altitude, grade, heartRate, power, cadence, speed, speedMs, temperature, coreTemperature, groundContactTime, verticalOscillation, stepLength, torqueEffL, torqueEffR, pedalSmL, pedalSmR, motorPower, batteryLevel, travelRange, depth, cycleLength, grit, flow, potentialStamina, staminaCurrent, pressure, paceMinKm, garminGapMinKm, re3Gap, strydNgpMinKm, corosEffortPaceMinKm, strydPower, ndlMin, cnsPct, n2Pct, decoStopM, po2, runningSmoothness, maxDist, maxElapsed }
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

function formatElapsed(minutes: number, maxMinutes: number): string {
  if (maxMinutes < 60) return `${Math.round(minutes)}\u202fmin`
  const h = Math.floor(minutes / 60)
  const m = Math.round(minutes % 60)
  if (h === 0) return `${m}\u202fmin`
  return `${h}h${m.toString().padStart(2, '0')}`
}

function buildTooltipPosLine(
  xVal: number, xMode: XMode, data: ChartData,
): string {
  if (xMode === 'distance') {
    return `${xVal.toFixed(1)}\u202fkm`
  }
  const idx = findClosestIdx(data.elapsedMin, xVal)
  const kmStr = (data.distanceKm[idx] ?? 0).toFixed(1)
  return `${formatElapsed(xVal, data.maxElapsed)}\u2009\u00b7\u2009${kmStr}\u202fkm`
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
  /** Callback fired when the user toggles the speed/pace mode inside the chart panel. */
  onSpeedModeChange?: (mode: 'speed' | 'pace') => void
  /** Sport type used for tab-based chart filtering. */
  sportType?: string
  /** Active tab from ChartTabs. If set, only shows charts relevant to that tab. */
  activeTab?: string
  /** Gear-change events from the activity_events table. Only shown for bike activities. */
  gearEvents?: ActivityEvent[]
  /** Activity ID — required for the Deko tab (DiveDecoPanel). */
  activityId?: string
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

// ── Gear data computation ────────────────────────────────────────────────────

interface GearPoint {
  /** X-axis value (km or elapsed minutes) */
  xVal: number
  /** Front chainring tooth count */
  frontGear: number
  /** Rear sprocket tooth count */
  rearGear: number
  /** Front gear position number (1 = small, 2 = large) */
  frontGearNum: number
  /** Rear gear position number (1 = smallest/hardest, n = largest/easiest) */
  rearGearNum: number
}

function computeGearData(
  streams: StreamPoint[],
  data: ChartData,
  xMode: XMode,
  gearEvents: ActivityEvent[],
): GearPoint[] {
  if (streams.length === 0 || gearEvents.length === 0) return []

  // Pre-compute timestamps once — O(N) instead of O(N×M)
  const streamTimes = streams.map(s => new Date(s.time).getTime())
  const t0 = streamTimes[0]
  const tEnd = streamTimes[streamTimes.length - 1]

  const points: GearPoint[] = []

  for (const ev of gearEvents) {
    if (ev.front_gear == null || ev.rear_gear == null) continue
    const t = new Date(ev.timestamp).getTime()
    if (t < t0 || t > tEnd) continue

    // Binary search for closest stream index
    let lo = 0, hi = streamTimes.length - 1
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (streamTimes[mid] < t) lo = mid + 1
      else hi = mid
    }
    // Check neighbor for closer match
    const bestIdx = lo > 0 && Math.abs(streamTimes[lo - 1] - t) < Math.abs(streamTimes[lo] - t) ? lo - 1 : lo

    const xVal = xMode === 'distance' ? data.distanceKm[bestIdx] : data.elapsedMin[bestIdx]
    points.push({
      xVal,
      frontGear: ev.front_gear,
      rearGear: ev.rear_gear,
      frontGearNum: ev.front_gear_num ?? 0,
      rearGearNum: ev.rear_gear_num ?? 0,
    })
  }

  // Sort by X position
  points.sort((a, b) => a.xVal - b.xVal)
  return points
}

// ── Main component ─────────────────────────────────────────────────────────────

export default function ActivityCharts({ streams, onHover, onZoomChange, sunriseAt, sunsetAt, splitMarkers, defaultXMode, highlightRange, speedMode, onSpeedModeChange, sportType, activeTab, gearEvents, activityId }: Props) {
  const data = useMemo(() => buildChartData(streams), [streams])
  const segGroup = sportType ? getSegmentGroup(sportType) : 'generic'

  // Track hover index internally for power phase gauge live update
  const [hoverIdx, setHoverIdx] = useState<number | null>(null)
  const hoverIdxRef = useRef<number | null>(null)
  const wrappedOnHover = useCallback((idx: number | null) => {
    hoverIdxRef.current = idx
    // RAF throttle — gauge update at most once per frame
    requestAnimationFrame(() => {
      setHoverIdx(hoverIdxRef.current)
    })
    onHover(idx)
  }, [onHover])

  // Power phase averages for gauge diagram (pedal analytics)
  // Uses circular mean (atan2) because start angles cluster around 350°/0° boundary —
  // arithmetic mean of [350°, 5°] = 177° (wrong), circular mean = 357° (correct).
  const powerPhaseAvg = useMemo<PowerPhaseAvg | null>(() => {
    const pts = streams.filter(s => s.left_power_phase_start_deg != null && s.right_power_phase_start_deg != null)
    if (pts.length === 0) return null
    const circAvg = (arr: (number | undefined | null)[]) => {
      const valid = arr.filter((v): v is number => v != null)
      if (valid.length === 0) return 0
      const sinSum = valid.reduce((s, a) => s + Math.sin(a * Math.PI / 180), 0)
      const cosSum = valid.reduce((s, a) => s + Math.cos(a * Math.PI / 180), 0)
      let mean = Math.atan2(sinSum / valid.length, cosSum / valid.length) * 180 / Math.PI
      if (mean < 0) mean += 360
      return mean
    }
    // Balance: arithmetic mean is fine (0-100 range, no wrap)
    const balances = pts.map(s => s.left_right_balance).filter((v): v is number => v != null)
    const avgBal = balances.length > 0 ? balances.reduce((a, b) => a + b, 0) / balances.length : null
    return {
      leftStart: circAvg(pts.map(s => s.left_power_phase_start_deg)),
      leftEnd: circAvg(pts.map(s => s.left_power_phase_end_deg)),
      leftPeakStart: circAvg(pts.map(s => s.left_power_phase_peak_start_deg)),
      leftPeakEnd: circAvg(pts.map(s => s.left_power_phase_peak_end_deg)),
      rightStart: circAvg(pts.map(s => s.right_power_phase_start_deg)),
      rightEnd: circAvg(pts.map(s => s.right_power_phase_end_deg)),
      rightPeakStart: circAvg(pts.map(s => s.right_power_phase_peak_start_deg)),
      rightPeakEnd: circAvg(pts.map(s => s.right_power_phase_peak_end_deg)),
      leftBalance: avgBal,
    }
  }, [streams])

  const [isZoomed, setIsZoomed] = useState(false)
  const [xMode, setXMode] = useState<XMode>(defaultXMode ?? 'distance')

  // Persistent chart order & visibility (per segGroup)
  const storageKey = `skadia-charts-${segGroup}`
  const [chartOrder, setChartOrder] = useState<string[]>(() => {
    try { return JSON.parse(localStorage.getItem(`${storageKey}-order`) ?? 'null') ?? [] } catch { return [] }
  })
  const [hiddenIds, setHiddenIds] = useState<Set<string>>(() => {
    try { return new Set(JSON.parse(localStorage.getItem(`${storageKey}-hidden`) ?? '[]')) } catch { return new Set() }
  })

  // Drag state
  const dragIdRef = useRef<string | null>(null)
  const [dropTargetId, setDropTargetId] = useState<string | null>(null)

  const sunMarkLines = useMemo(
    () => computeSunMarkLines(streams, data, xMode, sunriseAt, sunsetAt),
    [streams, data, xMode, sunriseAt, sunsetAt],
  )

  // Stable reference for gear points — prevents useECharts re-init on every render
  const gearPoints = useMemo(
    () => gearEvents?.length ? computeGearData(streams, data, xMode, gearEvents) : [],
    [streams, data, xMode, gearEvents],
  )

  // Stable reference for deco extra series — prevents useECharts re-init on every hover
  // (inline object literal would be a new reference each render → setOption on every hover → lag + brush reset)
  const decoExtraSeries = useMemo(
    () => hasValidData(data.decoStopM) ? [{
      values: data.decoStopM,
      name: 'Dekotiefe (m)',
      color: '#ef4444',
      lineStyle: { color: '#ef4444', width: 0 },
      areaStyle: { color: 'rgba(239,68,68,0.25)' },
      symbol: 'none',
      z: 1,
      connectNulls: true,
      silent: true,
    }] : undefined,
    [data.decoStopM],
  )

  const toggleChart = useCallback((id: string) => {
    setHiddenIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      localStorage.setItem(`${storageKey}-hidden`, JSON.stringify([...next]))
      return next
    })
  }, [storageKey])

  const handleChartDrop = useCallback((targetId: string) => {
    const from = dragIdRef.current
    setDropTargetId(null)
    dragIdRef.current = null
    if (!from || from === targetId) return
    setChartOrder(prev => {
      const next = [...prev]
      const fi = next.indexOf(from)
      const ti = next.indexOf(targetId)
      if (fi === -1 || ti === -1) return prev
      next.splice(fi, 1)
      next.splice(ti, 0, from)
      localStorage.setItem(`${storageKey}-order`, JSON.stringify(next))
      return next
    })
  }, [storageKey])

  const handleChartDragEnd = useCallback(() => {
    dragIdRef.current = null
    setDropTargetId(null)
  }, [])

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

  const pendingChartOrderRef = useRef<string[] | null>(null)
  useEffect(() => {
    if (pendingChartOrderRef.current !== null) {
      setChartOrder(pendingChartOrderRef.current)
      pendingChartOrderRef.current = null
    }
  })

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

  const common = {
    data, onHover: wrappedOnHover,
    onBrushZoom: stableBrushZoom,
    onSyncZoom: syncZoom,
    onRegister: registerChart,
    onUnregister: unregisterChart,
    brushZoomLockRef,
    xMode,
    splitMarkers,
  }

  const tabCharts = activeTab ? getTabCharts(activeTab, sportType ?? '') : null
  const show = (key: ChartKey | null) => key === null || tabCharts === null || tabCharts.includes(key)

  // ── Build panel map ────────────────────────────────────────────────────────────
  // panelMap: id → ReactNode (without display:none wrapper — visibility controlled below)
  // panelMetas: ordered list of available chart metadata for the legend bar

  const panelMap = new Map<string, React.ReactNode>()
  const panelMetas: ChartMeta[] = []

  function addPanel(meta: ChartMeta, node: React.ReactNode) {
    panelMap.set(meta.id, node)
    panelMetas.push(meta)
  }

  const speedLabelText = (speedMode ?? defaultSpeedMode(sportType ?? '')) === 'pace' ? 'Pace' : 'Geschwindigkeit'
  const speedColor = segGroup === 'foot' ? '#3b82f6' : '#f97316'

  if (hasValidData(data.altitude) && sportType !== 'diving')
    addPanel({ id: 'altitude', label: 'Höhe', color: '#22c55e', tabKey: 'altitude' },
      <ChartPanel chartId="altitude" {...common}
        field="altitude" label="Höhe" unit="m" color="#22c55e" area
        markLines="minmax" altitudeSmartY showGradeColor
        sunMarkLines={sunMarkLines} />)
  if (segGroup === 'foot' && hasValidData(data.speed))
    addPanel({ id: 'speed', label: speedLabelText, color: speedColor, tabKey: 'speed' },
      <SpeedPaceChartPanel chartId="speed" {...common}
        speedMode={speedMode ?? defaultSpeedMode(sportType ?? '')}
        onSpeedModeChange={onSpeedModeChange ?? (() => {})}
        sportType={sportType ?? ''} />)

  if (hasValidData(data.heartRate))
    addPanel({ id: 'hr', label: 'Herzfrequenz', color: '#ef4444', tabKey: 'heartRate' },
      <ChartPanel chartId="hr" {...common}
        field="heartRate" label="Herzfrequenz" unit="bpm" color="#ef4444"
        markLines="max" />)

  if (hasValidData(data.power))
    addPanel({ id: 'power', label: 'Leistung', color: '#a855f7', tabKey: 'power' },
      <ChartPanel chartId="power" {...common}
        field="power" label="Leistung" unit="W" color="#a855f7"
        extraSeries={hasValidData(data.strydPower) ? [{ values: data.strydPower, name: 'Stryd', color: '#7c3aed' }] : undefined} />)

  if (hasValidData(data.cadence))
    addPanel({ id: 'cadence', label: sportType ? fieldLabel('cadence', sportType) : 'Kadenz', color: '#f97316', tabKey: 'cadence' },
      <ChartPanel chartId="cadence" {...common}
        field="cadence" label={sportType ? fieldLabel('cadence', sportType) : 'Kadenz'} unit={sportType ? fieldUnit('cadence', sportType) : 'rpm'} color="#f97316" />)

  if (segGroup !== 'foot' && hasValidData(data.speed))
    addPanel({ id: 'speed', label: speedLabelText, color: speedColor, tabKey: 'speed' },
      <SpeedPaceChartPanel chartId="speed" {...common}
        speedMode={speedMode ?? defaultSpeedMode(sportType ?? '')}
        onSpeedModeChange={onSpeedModeChange ?? (() => {})}
        sportType={sportType ?? ''} />)

  if (hasValidData(data.temperature))
    addPanel({ id: 'temperature', label: 'Temperatur', color: '#06b6d4', tabKey: 'temperature' },
      <ChartPanel chartId="temperature" {...common}
        field="temperature" label="Temperatur" unit="°C" color="#06b6d4"
        markLines="minmax" />)

  if (hasValidData(data.coreTemperature))
    addPanel({ id: 'core-temp', label: 'Kerntemperatur', color: '#f43f5e', tabKey: 'coreTemperature' },
      <ChartPanel chartId="core-temp" {...common}
        field="coreTemperature" label="Kerntemperatur" unit="°C" color="#f43f5e" />)

  if (hasValidData(data.groundContactTime) || hasValidData(data.verticalOscillation))
    addPanel({ id: 'running-dynamics', label: 'Laufdynamik', color: '#10b981', tabKey: 'runningDynamics' },
      <RunningDynamicsChartPanel chartId="running-dynamics" {...common} />)

  if (hasValidData(data.stepLength))
    addPanel({ id: 'step-length', label: 'Schrittlänge', color: '#0891b2', tabKey: 'stepLength' },
      <ChartPanel chartId="step-length" {...common}
        field="stepLength" label="Schrittlänge" unit="m" color="#0891b2"
        overrideValues={data.stepLength} />)

  if (hasValidData(data.runningSmoothness))
    addPanel({ id: 'running-smoothness', label: 'Running Smoothness', color: '#8b5cf6', tabKey: 'runningSmoothness' },
      <RunningSmoothnessChartPanel chartId="running-smoothness" {...common} />)

  if (powerPhaseAvg)
    addPanel({ id: 'power-phase', label: 'Pedalphase', color: '#ec4899', tabKey: 'powerPhase' },
      <PowerPhaseGaugePanel phaseData={powerPhaseAvg} streams={streams} hoverIdx={hoverIdx} />)

  if (hasValidData(data.torqueEffL) || hasValidData(data.torqueEffR))
    addPanel({ id: 'torque-eff', label: 'Drehmoment-Eff.', color: '#ec4899', tabKey: 'torqueEff' },
      <PedalDualChartPanel chartId="torque-eff" {...common}
        leftValues={data.torqueEffL} rightValues={data.torqueEffR}
        leftLabel="Drehmoment-Eff. Links" rightLabel="Drehmoment-Eff. Rechts"
        leftColor="#EC4899" rightColor="#A855F7"
        leftAxisName="Links\n(%)" rightAxisName="Rechts\n(%)"
        tooltipTitle="Drehmoment-Eff."
        leftTooltipSuffix="Links" rightTooltipSuffix="Rechts" />)

  if (hasValidData(data.pedalSmL) || hasValidData(data.pedalSmR))
    addPanel({ id: 'pedal-sm', label: 'Tritt-Gleichm.', color: '#a855f7', tabKey: 'pedalSm' },
      <PedalDualChartPanel chartId="pedal-sm" {...common}
        leftValues={data.pedalSmL} rightValues={data.pedalSmR}
        leftLabel="Tritt-Gleichm. Links" rightLabel="Tritt-Gleichm. Rechts"
        leftColor="#A855F7" rightColor="#8B5CF6"
        leftAxisName="Links\n(%)" rightAxisName="Rechts\n(%)"
        tooltipTitle="Tritt-Gleichm."
        leftTooltipSuffix="Links" rightTooltipSuffix="Rechts" />)

  if (hasValidData(data.motorPower))
    addPanel({ id: 'motor-power', label: 'Motorleistung', color: '#9333ea', tabKey: 'motorPower' },
      <ChartPanel chartId="motor-power" {...common}
        field="power" label="Motorleistung" unit="W" color="#9333EA"
        overrideValues={data.motorPower} />)

  if (hasValidData(data.batteryLevel))
    addPanel({ id: 'battery', label: 'Akku-Level', color: '#22c55e', tabKey: 'batteryLevel' },
      <ChartPanel chartId="battery" {...common}
        field="power" label="Akku-Level" unit="%" color="#22C55E"
        overrideValues={data.batteryLevel} yAxisFixed={[0, 100]} />)

  if (hasValidData(data.travelRange))
    addPanel({ id: 'travel-range', label: 'Reichweite', color: '#6b7280', tabKey: 'travelRange' },
      <ChartPanel chartId="travel-range" {...common}
        field="power" label="Reichweite" unit="km" color="#6B7280"
        overrideValues={data.travelRange} />)

  if (hasValidData(data.depth))
    addPanel({ id: 'depth', label: 'Tiefe', color: '#3b82f6', tabKey: 'depth' },
      <ChartPanel chartId="depth" {...common}
        field="altitude" label="Tiefe" unit="m" color="#3B82F6" area
        overrideValues={data.depth} invertY
        extraSeries={decoExtraSeries} />)

  if ((segGroup === 'swim' || segGroup === 'paddle') && hasValidData(data.cycleLength)) {
    const cycleLengthLabel = segGroup === 'swim' ? 'Distanz/Zug' : 'Distanz/Schlag'
    addPanel({ id: 'cycle-length', label: cycleLengthLabel, color: '#0ea5e9', tabKey: 'cycleLength' },
      <ScatterChartPanel chartId="cycle-length" {...common}
        values={data.cycleLength} label={cycleLengthLabel} unit="m" color="#0EA5E9" />)
  }

  if (gearPoints.length > 0)
    addPanel({ id: 'gear', label: 'Gangschaltung', color: '#94a3b8', tabKey: 'gear' },
      <GearChartPanel chartId="gear" {...common} gearPoints={gearPoints} />)

  if (segGroup === 'bike' && hasValidData(data.grit))
    addPanel({ id: 'grit', label: 'MTB Grit', color: '#d97706', tabKey: 'grit' },
      <ChartPanel chartId="grit" {...common}
        field="grit" label="MTB Grit" unit="" color="#D97706" />)

  if (segGroup === 'bike' && hasValidData(data.flow))
    addPanel({ id: 'flow', label: 'MTB Flow', color: '#10b981', tabKey: 'flow' },
      <ChartPanel chartId="flow" {...common}
        field="flow" label="MTB Flow" unit="" color="#10B981" />)

  if (hasValidData(data.potentialStamina) || hasValidData(data.staminaCurrent))
    addPanel({ id: 'potentialStamina', label: 'Garmin Stamina', color: '#8b5cf6', tabKey: 'potentialStamina' },
      <PedalDualChartPanel chartId="potentialStamina" {...common}
        leftValues={data.potentialStamina}
        rightValues={data.staminaCurrent}
        leftLabel="Stamina (Potential)" rightLabel="Stamina (Aktuell)"
        leftColor="#8B5CF6" rightColor="#EC4899"
        leftAxisName="%" rightAxisName="%"
        leftTooltipSuffix="Potential" rightTooltipSuffix="Aktuell"
        tooltipTitle="Garmin Stamina" />)

  if (hasValidData(data.pressure) && !hasValidData(data.depth))
    addPanel({ id: 'pressure', label: 'Barometer', color: '#78716c', tabKey: 'pressure' },
      <ChartPanel chartId="pressure" {...common}
        field="pressure" label="Barometer" unit="hPa" color="#78716c" />)

  if (hasValidData(data.ndlMin) || hasValidData(data.cnsPct) || hasValidData(data.n2Pct))
    addPanel({ id: 'deco-ts', label: 'Deko-Daten', color: '#3b82f6', tabKey: 'decoTimeSeries' },
      <DecoChartPanel chartId="deco-ts" {...common} />)

  if (hasValidData(data.po2))
    addPanel({ id: 'po2', label: 'pO₂', color: '#f59e0b', tabKey: 'decoTimeSeries' },
      <ChartPanel chartId="po2" {...common}
        label="pO₂" unit="bar" color="#f59e0b"
        overrideValues={data.po2} />)

  // Developer fields — added via addPanel so chip, drag-reorder, and visibility toggle
  // work identically to all other charts. Must happen BEFORE availableIds is computed.
  const hasDekoData = streams.some(s => s.ndl_time_s != null || s.cns_load_pct != null)
  const hasDevFields = streams.some(s => s.developer_fields && Object.keys(s.developer_fields).length > 0)
  const showDekoPanel = (activeTab === CHART_TAB.deko || activeTab === CHART_TAB.alles) && activityId != null && hasDekoData
  const showDeveloperFieldsPanel = (activeTab === CHART_TAB.developerFields || activeTab === CHART_TAB.alles) && activityId != null && hasDevFields
  if (showDeveloperFieldsPanel && activityId != null) {
    addPanel({ id: 'developer-fields', label: 'Entwickler-Felder', color: DEV_FIELD_COLOR, tabKey: null },
      <DeveloperFieldChart
        activityId={activityId}
        streams={streams}
        data={data}
        xMode={xMode}
        onHover={wrappedOnHover}
        onBrushZoom={stableBrushZoom}
        onSyncZoom={syncZoom}
        onRegister={registerChart}
        onUnregister={unregisterChart}
        brushZoomLockRef={brushZoomLockRef}
      />)
  }

  // Sync chartOrder with available ids:
  // - Keep persisted order, remove unavailable ids, append new ones at the end
  const availableIds = panelMetas.map(m => m.id)
  const effectiveOrder = [
    ...chartOrder.filter(id => availableIds.includes(id)),
    ...availableIds.filter(id => !chartOrder.includes(id)),
  ]
  // Save if it differs from current (first render or new charts)
  if (effectiveOrder.join(',') !== chartOrder.join(',')) {
    pendingChartOrderRef.current = effectiveOrder
  }

  const metaById = new Map(panelMetas.map(m => [m.id, m]))
  const hasGlucoseData = streams.some(s => s.glucose_mg_dl != null)
  if (panelMap.size === 0 && !showDekoPanel && !showDeveloperFieldsPanel && !hasGlucoseData) return null

  return (
    <div className="space-y-3">
      {/* ── Sticky header: title row + chip bar ── */}
      <div className="sticky z-30 bg-white dark:bg-neutral-950 pb-1 -mx-4 px-4 sm:-mx-6 sm:px-6"
           style={{ top: 'calc(3.5rem + var(--chart-tabs-height, 0px))' }}>
      {/* ── Title row ── */}
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

      {/* ── Chart legend chip bar ── */}
      {panelMetas.length > 0 && (
        <div className="flex flex-wrap gap-1.5 py-1">
          {effectiveOrder.map(id => {
            const meta = metaById.get(id)
            if (!meta) return null
            const isHidden = hiddenIds.has(id)
            const isDragOver = dropTargetId === id
            return (
              <button
                key={id}
                draggable
                onDragStart={e => { e.stopPropagation(); dragIdRef.current = id }}
                onDragOver={e => { e.preventDefault(); setDropTargetId(id) }}
                onDrop={() => handleChartDrop(id)}
                onDragLeave={() => setDropTargetId(null)}
                onDragEnd={handleChartDragEnd}
                onClick={() => toggleChart(id)}
                title={isHidden ? `${meta.label} einblenden` : `${meta.label} ausblenden`}
                className={[
                  'flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium',
                  'border transition-all cursor-grab active:cursor-grabbing select-none',
                  isHidden
                    ? 'opacity-35 border-neutral-200 dark:border-neutral-700 text-neutral-400 dark:text-neutral-600'
                    : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:border-neutral-400 dark:hover:border-neutral-500',
                  isDragOver ? 'ring-2 ring-orange-400 ring-offset-1' : '',
                ].join(' ')}
              >
                <span
                  className="w-2 h-2 rounded-full shrink-0 transition-opacity"
                  style={{ backgroundColor: meta.color, opacity: isHidden ? 0.4 : 1 }}
                />
                <span className={isHidden ? 'line-through' : ''}>{meta.label}</span>
              </button>
            )
          })}
        </div>
      )}
      </div>{/* end sticky header */}

      {/* ── Charts in user-controlled order ── */}
      {effectiveOrder.map(id => {
        const meta = metaById.get(id)
        const node = panelMap.get(id)
        if (!meta || !node) return null
        const visible = show(meta.tabKey) && !hiddenIds.has(id)
        const isDragOver = dropTargetId === id
        return (
          <div
            key={`${id}-${xMode}`}
            style={{ display: visible ? undefined : 'none' }}
            className={`relative group/chart ${isDragOver ? 'outline outline-2 outline-orange-400 outline-offset-2 rounded-xl' : ''}`}
            onDragOver={e => { e.preventDefault(); setDropTargetId(id) }}
            onDrop={() => handleChartDrop(id)}
            onDragLeave={e => {
              // Only clear if leaving the chart wrapper entirely (not entering a child)
              if (!e.currentTarget.contains(e.relatedTarget as Node)) setDropTargetId(null)
            }}
          >
            {/* Drag handle — visible on hover, left side */}
            <div
              draggable
              onDragStart={e => { e.stopPropagation(); dragIdRef.current = id }}
              onDragEnd={handleChartDragEnd}
              className="absolute left-0 top-0 bottom-0 w-5 z-10 cursor-grab active:cursor-grabbing
                         flex items-center justify-center rounded-l-xl
                         opacity-0 group-hover/chart:opacity-100 transition-opacity
                         hover:bg-neutral-100/80 dark:hover:bg-neutral-800/80"
              title="Verschieben"
            >
              <svg width="10" height="16" viewBox="0 0 10 16" fill="none" className="text-neutral-400 dark:text-neutral-500">
                <circle cx="2.5" cy="2" r="1.5" fill="currentColor"/>
                <circle cx="7.5" cy="2" r="1.5" fill="currentColor"/>
                <circle cx="2.5" cy="6" r="1.5" fill="currentColor"/>
                <circle cx="7.5" cy="6" r="1.5" fill="currentColor"/>
                <circle cx="2.5" cy="10" r="1.5" fill="currentColor"/>
                <circle cx="7.5" cy="10" r="1.5" fill="currentColor"/>
                <circle cx="2.5" cy="14" r="1.5" fill="currentColor"/>
                <circle cx="7.5" cy="14" r="1.5" fill="currentColor"/>
              </svg>
            </div>
            {node}
          </div>
        )
      })}

      {(activeTab === CHART_TAB.deko || activeTab === CHART_TAB.alles) && activityId && streams.some(s => s.ndl_time_s != null || s.cns_load_pct != null) && (
        <DiveDecoPanel activityId={activityId} streams={streams} />
      )}

      {(activeTab === CHART_TAB.glucose || activeTab === CHART_TAB.alles) && (
        <GlucoseChartPanel streams={streams} />
      )}

      <p className="text-xs text-neutral-400 dark:text-neutral-600">
        Ziehen zum Auswählen · Mausrad zum Zoomen · Nach Auswahl: Ziehen zum Verschieben
      </p>
    </div>
  )
}

// ── Shared chart helpers ───────────────────────────────────────────────────────

function buildSplitMarkLines(splitMarkers: SplitMarker[] | undefined, xMode: XMode): Record<string, unknown>[] {
  if (!splitMarkers?.length) return []
  return splitMarkers.map(marker => ({
    xAxis: xMode === 'distance' ? marker.distanceKm : marker.elapsedMin,
    lineStyle: { color: '#94a3b8', type: 'dashed', width: 1, opacity: 0.8 },
    label: { show: true, position: 'insideStartTop', formatter: marker.label, fontSize: 10, color: '#94a3b8' },
  }))
}

interface ChartInteractionOpts {
  chart: echarts.ECharts
  chartId: string
  onHover: (index: number | null) => void
  onBrushZoom: (startValue: number, endValue: number) => void
  onSyncZoom: (start: number, end: number, sourceId: string) => void
  onRegister: (id: string, chart: echarts.ECharts) => void
  onUnregister: (id: string) => void
  brushZoomLockRef: React.MutableRefObject<boolean>
  xMax: number
  xData: number[]
}

function setupChartInteractions(opts: ChartInteractionOpts): () => void {
  const { chart, chartId, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef, xMax, xData } = opts

  onRegister(chartId, chart)

  chart.dispatchAction({
    type: 'takeGlobalCursor',
    key: 'brush',
    brushOption: { brushType: 'lineX', brushMode: 'single' },
  })

  // ZRender mousemove for reliable real-hover detection.
  // For type:'value' xAxis, showTip does NOT include dataIndex in real-hover params,
  // so we use ZRender directly and convert pixel → axis value → closest stream index.
  const zr = chart.getZr()
  let lastHoverIdx: number | null = null

  const zrMouseMove = (e: unknown) => {
    const event = e as { offsetX?: number; offsetY?: number }
    if (event.offsetX == null || event.offsetY == null) return
    const inGrid = chart.containPixel({ gridIndex: 0 }, [event.offsetX, event.offsetY])
    if (!inGrid) {
      if (lastHoverIdx !== null) { lastHoverIdx = null; onHover(null) }
      return
    }
    const xVal = chart.convertFromPixel({ xAxisIndex: 0 }, event.offsetX) as number | null
    if (xVal == null) { if (lastHoverIdx !== null) { lastHoverIdx = null; onHover(null) } return }
    const idx = findClosestIdx(xData, xVal)
    if (idx !== lastHoverIdx) { lastHoverIdx = idx; onHover(idx) }
  }

  const zrMouseOut = () => {
    if (lastHoverIdx !== null) { lastHoverIdx = null; onHover(null) }
  }

  zr.on('mousemove', zrMouseMove)
  zr.on('mouseout', zrMouseOut)

  // showTip/hideTip sync: echarts.connect propagates tooltip events across all group members
  chart.on('showTip', (params: unknown) => {
    const typed = params as { dataIndex?: number }
    if (typed.dataIndex != null) {
      const idx = typed.dataIndex
      if (idx !== lastHoverIdx) { lastHoverIdx = idx; onHover(idx) }
    }
  })
  chart.on('hideTip', () => {
    if (lastHoverIdx !== null) { lastHoverIdx = null; onHover(null) }
  })

  type BrushArea = { range?: [number, number]; coordRange?: [number, number] }

  // Brush gesture ended → zoom all charts + map.
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

  return () => {
    onUnregister(chartId)
    zr.off('mousemove', zrMouseMove)
    zr.off('mouseout', zrMouseOut)
  }
}

// ── Developer Field Chart ─────────────────────────────────────────────────────

const DEV_FIELD_MEANINGFUL_UNITS = new Set([
  'Watts', 'kN/m', 'Milliseconds', 'Centimeters', 'bw/s', 'C', 'deg C', '°C', '°', 'm/s', 'N', 'rpm', '',
])
const DEV_FIELD_SKIP_KEYS = new Set(['Sat', 'hdop', 'idx', 'ehdop', 'evdop', 'vdop', 'Effort Pace'])
const DEV_FIELD_COLORS: Record<string, string> = {
  'Form Power': '#f59e0b', 'Air Power': '#0ea5e9', 'Leg Spring Stiffness': '#10b981',
  'Power': '#a855f7', 'Ground Time': '#f97316', 'Vertical Oscillation': '#06b6d4',
  'Cadence': '#3b82f6', 'ngp': '#8b5cf6',
}
const DEV_FIELD_PALETTE = ['#5470c6', '#91cc75', '#fac858', '#ee6666', '#73c0de', '#3ba272', '#fc8452', '#9a60b4', '#ea7ccc']

function groupDevFieldsByUnit(fields: DeveloperFieldInfo[]): Map<string, DeveloperFieldInfo[]> {
  const map = new Map<string, DeveloperFieldInfo[]>()
  for (const f of fields) {
    const existing = map.get(f.units)
    if (existing) existing.push(f)
    else map.set(f.units, [f])
  }
  return map
}

const DEV_FIELD_COLOR = '#8b5cf6'

interface DeveloperFieldChartProps {
  activityId: string
  streams: StreamPoint[]
  data: ChartData
  xMode: XMode
  onHover: (index: number | null) => void
  onBrushZoom: (startValue: number, endValue: number) => void
  onSyncZoom: (start: number, end: number, sourceId: string) => void
  onRegister: (id: string, chart: echarts.ECharts) => void
  onUnregister: (id: string) => void
  brushZoomLockRef: React.MutableRefObject<boolean>
}

function DeveloperFieldChart({
  activityId, streams, data, xMode,
  onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef,
}: DeveloperFieldChartProps) {
  const { data: devFieldInfos, isLoading } = useQuery<DeveloperFieldInfo[]>({
    queryKey: ['developer-fields', activityId],
    queryFn: () => fetchDeveloperFields(activityId),
    staleTime: 5 * 60 * 1000,
  })

  const fields = useMemo(() => {
    if (!devFieldInfos) return []
    return devFieldInfos.filter(f => {
      const displayName = getDevFieldLabel(f.field_key, f.field_name)
      return DEV_FIELD_MEANINGFUL_UNITS.has(f.units) && !DEV_FIELD_SKIP_KEYS.has(f.field_key) && !DEV_FIELD_SKIP_KEYS.has(displayName)
    })
  }, [devFieldInfos])

  const fieldValues = useMemo(() => {
    const map = new Map<string, (number | null)[]>()
    for (const field of fields) {
      map.set(field.field_key, streams.map(s => {
        const v = s.developer_fields?.[field.field_key]
        if (v == null || typeof v !== 'number' || isNaN(v) || !isFinite(v)) return null
        return v
      }))
    }
    return map
  }, [streams, fields])

  const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
  const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

  const containerRef = useECharts(
    () => {
      const unitGroups = groupDevFieldsByUnit(fields)
      const yAxes: echarts.EChartsCoreOption['yAxis'] = []
      const fieldYAxisIndex = new Map<string, number>()
      let axisIdx = 0
      for (const [unit, groupFields] of unitGroups) {
        for (const f of groupFields) fieldYAxisIndex.set(f.field_key, axisIdx)
        const resolvedUnit = groupFields.length > 0
          ? getDevFieldUnit(groupFields[0].field_key, unit)
          : unit
        yAxes.push({
          type: 'value', name: resolvedUnit || '',
          nameTextStyle: { fontSize: 10, color: '#666' },
          axisLabel: { fontSize: 11, color: '#666', margin: 8 },
          splitLine: axisIdx === 0 ? { lineStyle: { color: '#eee', type: 'dashed' } } : { show: false },
          axisLine: { show: false }, axisTick: { show: false },
          position: axisIdx % 2 === 0 ? 'left' : 'right',
          offset: axisIdx > 1 ? Math.floor(axisIdx / 2) * 50 : 0,
        })
        axisIdx++
      }

      const series: echarts.EChartsCoreOption['series'] = []
      let colorIdx = 0
      for (const field of fields) {
        const displayName = getDevFieldLabel(field.field_key, field.field_name)
        const vals = fieldValues.get(field.field_key) ?? []
        const color = DEV_FIELD_COLORS[displayName] ?? DEV_FIELD_PALETTE[colorIdx % DEV_FIELD_PALETTE.length]
        if (!DEV_FIELD_COLORS[displayName]) colorIdx++
        series.push({
          type: 'line', name: displayName,
          data: xData.map((x, i) => [x, vals[i] ?? null]),
          showSymbol: false,
          lineStyle: { width: 1.5, color }, itemStyle: { color },
          connectNulls: false,
          yAxisIndex: fieldYAxisIndex.get(field.field_key) ?? 0,
        })
      }

      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, xMax)

      return {
        animation: false,
        grid: { left: 52, right: unitGroups.size > 1 ? 76 : 52, top: 8, bottom: 56 },
        toolbox: { feature: {
          brush: { type: ['lineX'] },
          saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: 'Entwickler-Felder' },
        }},
        brush: {
          toolbox: ['lineX'],
          xAxisIndex: 0,
          brushStyle: { borderWidth: 1, color: 'rgba(249,115,22,0.12)', borderColor: '#f97316' },
        },
        tooltip: {
          trigger: 'axis', transitionDuration: 0, confine: true,
          backgroundColor: '#1f2937', borderColor: '#374151', borderWidth: 1, padding: [8, 10],
          textStyle: { fontSize: 12, color: '#f9fafb' },
          axisPointer: { type: 'line', lineStyle: { color: '#f97316', width: 1, type: 'dashed' }, animation: false },
          formatter: (params: unknown) => {
            const arr = Array.isArray(params) ? params : [params]
            if (!arr.length) return ''
            const dataIndex = (arr[0] as { dataIndex?: number }).dataIndex
            if (dataIndex == null) return ''
            const posStr = buildTooltipPosLine(xData[dataIndex] ?? 0, xMode, data)
            let html = `<div style="line-height:1.75;min-width:150px">`
            html += `<div style="color:#888;font-size:11px;margin-bottom:2px">${posStr}</div>`
            for (const p of arr) {
              const tp = p as { seriesName?: string; color?: string; value?: [number, number | null] }
              const v = tp.value?.[1]
              if (v == null || isNaN(v)) continue
              const field = fields.find(f => getDevFieldLabel(f.field_key, f.field_name) === tp.seriesName)
              const unit = field ? getDevFieldUnit(field.field_key, field.units) : ''
              const color = DEV_FIELD_COLORS[tp.seriesName ?? ''] ?? tp.color ?? '#888'
              html += `<div style="font-size:13px;color:${color}"><strong>${v.toFixed(1)}\u202f${unit}</strong> ${tp.seriesName}</div>`
            }
            return html + '</div>'
          },
        },
        legend: { type: 'scroll', bottom: 4, textStyle: { fontSize: 11, color: '#666' }, pageTextStyle: { color: '#666' } },
        xAxis: {
          type: 'value', min: 0, max: xMax,
          axisLabel: { fontSize: 11, color: '#666', formatter: xFormatter },
          splitLine: { show: false },
          axisLine: { lineStyle: { color: '#ddd' } },
          axisTick: { lineStyle: { color: '#ddd' } },
        },
        yAxis: yAxes,
        series,
        dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none', zoomOnMouseWheel: true, moveOnMouseMove: true, moveOnMouseWheel: false }],
      }
    },
    [fields, fieldValues, xData, xMax, xMode],
    (chart) => {
      return setupChartInteractions({ chart, chartId: 'developer-fields', onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef, xMax, xData })
    }
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        <span className="w-2 h-2 rounded-full shrink-0" style={{ backgroundColor: DEV_FIELD_COLOR }} />
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Entwickler-Felder</span>
      </div>
      {isLoading
        ? <div className="h-[240px] flex items-center justify-center text-sm text-neutral-400 dark:text-neutral-600">Lade…</div>
        : fields.length === 0
        ? <div className="h-[240px] flex items-center justify-center text-sm text-neutral-400 dark:text-neutral-600">Keine Entwickler-Felder</div>
        : <div ref={containerRef} className="h-[240px] w-full" />}
    </div>
  )
}

// ── Deco Time Series Chart Panel (Diving) ────────────────────────────────────

interface DecoChartPanelProps {
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

function DecoChartPanel({
  chartId, data, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister,
  brushZoomLockRef, xMode, splitMarkers,
}: DecoChartPanelProps) {
  const hasNdl = hasValidData(data.ndlMin)
  const hasCns = hasValidData(data.cnsPct)
  const hasN2 = hasValidData(data.n2Pct)
  const hasDecoStop = hasValidData(data.decoStopM)

  const containerRef = useECharts(
    () => {
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
      const gridTop = ((splitMarkers && splitMarkers.length > 0) ? 34 : 8) + 18

      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      const mlData = buildSplitMarkLines(splitMarkers, xMode)

      // Map to [x, y] pairs — required for type:'value' xAxis
      const xy = (values: number[]) => values.map((v, i) => [xData[i], v])

      const series: Record<string, unknown>[] = []
      if (hasNdl) series.push({
        name: 'NDL', type: 'line', data: xy(data.ndlMin), symbol: 'none',
        lineStyle: { color: '#22c55e', width: 2 }, itemStyle: { color: '#22c55e' },
        yAxisIndex: 0, markLine: mlData.length > 0 ? { data: mlData, silent: true } : undefined,
      })
      if (hasCns) series.push({
        name: 'CNS%', type: 'line', data: xy(data.cnsPct), symbol: 'none',
        lineStyle: { color: '#ef4444', width: 2 }, itemStyle: { color: '#ef4444' },
        yAxisIndex: 1,
      })
      if (hasN2) series.push({
        name: 'N₂%', type: 'line', data: xy(data.n2Pct), symbol: 'none',
        lineStyle: { color: '#3b82f6', width: 2 }, itemStyle: { color: '#3b82f6' },
        yAxisIndex: 2,
      })
      if (hasDecoStop) series.push({
        name: 'Deko-Stop', type: 'line', data: xy(data.decoStopM), symbol: 'none',
        step: 'end',
        lineStyle: { color: '#f97316', width: 2, type: 'dashed' }, itemStyle: { color: '#f97316' },
        yAxisIndex: 3,
      })

      return {
        animation: false,
        grid: { left: 52, right: 110, top: 8, bottom: 36 },

        brush: {
          toolbox: [],
          brushType: 'lineX',
          brushStyle: { borderWidth: 1, color: 'rgba(249,115,22,0.12)', borderColor: '#f97316' },
          throttleType: 'debounce', throttleDelay: 100,
        },

        toolbox: {
          feature: {
            brush: { show: false },
            saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: 'Deko-Zeitreihen' },
          },
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
            const xVal = xMode === 'distance' ? data.distanceKm[idx] : data.elapsedMin[idx]
            const posLine = buildTooltipPosLine(xVal, xMode, data)
            const units: Record<string, string> = { NDL: ' min', 'CNS%': ' %', 'N₂%': ' %', 'Deko-Stop': ' m' }
            let html = `<div style="line-height:1.75;min-width:150px">`
            html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
            for (const item of arr as Array<{ seriesName: string; value: number; marker: string }>) {
              const v = Array.isArray(item.value) ? item.value[1] : item.value
              if (v == null || isNaN(v as number)) continue
              const u = units[item.seriesName] ?? ''
              html += `<div style="font-size:12px">${item.marker} ${item.seriesName} <strong style="float:right;margin-left:12px">${Math.round(v as number)}${u}</strong></div>`
            }
            return html + '</div>'
          },
        },

        xAxis: {
          type: 'value',
          min: 0, max: xMax,
          axisLabel: { fontSize: 10, color: '#aaa', formatter: xFormatter },
          axisTick: { show: false },
          splitLine: { show: false },
        },

        yAxis: [
          { type: 'value', name: 'min', nameTextStyle: { fontSize: 10, color: '#9ca3af' },
            axisLabel: { fontSize: 10, color: '#9ca3af' },
            splitLine: { lineStyle: { color: '#f5f5f5', type: 'dashed' } },
            min: 0, max: 60 },
          { type: 'value', name: '%', nameTextStyle: { fontSize: 10, color: '#9ca3af' },
            axisLabel: { fontSize: 10, color: '#9ca3af' },
            splitLine: { show: false }, min: 0, max: 100, position: 'right' },
          { type: 'value', axisLabel: { show: false }, nameTextStyle: { show: false },
            splitLine: { show: false }, min: 0, position: 'right' },
          { type: 'value', name: 'm', nameTextStyle: { fontSize: 10, color: '#9ca3af' },
            axisLabel: { fontSize: 10, color: '#f97316' },
            splitLine: { show: false }, position: 'right', offset: 40 },
        ],

        dataZoom: [
          { type: 'inside', xAxisIndex: 0, filterMode: 'none',
            zoomOnMouseWheel: true, moveOnMouseMove: true, moveOnMouseWheel: false },
        ],
        series,
      }
    },
    [data, xMode, splitMarkers, hasNdl, hasCns, hasN2, hasDecoStop],
    (chart) => {
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      return setupChartInteractions({ chart, chartId, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef, xMax, xData })
    },
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-4 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60 flex-wrap">
        {hasNdl && (
          <div className="flex items-center gap-2">
            <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: '#22c55e' }} />
            <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
              NDL <span className="font-normal text-neutral-400 dark:text-neutral-600">(min)</span>
            </span>
          </div>
        )}
        {hasCns && (
          <div className="flex items-center gap-2">
            <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: '#ef4444' }} />
            <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
              CNS <span className="font-normal text-neutral-400 dark:text-neutral-600">(%)</span>
            </span>
          </div>
        )}
        {hasN2 && (
          <div className="flex items-center gap-2">
            <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: '#3b82f6' }} />
            <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
              N₂-Loading <span className="font-normal text-neutral-400 dark:text-neutral-600">(%)</span>
            </span>
          </div>
        )}
        {hasDecoStop && (
          <div className="flex items-center gap-2">
            <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: '#f97316' }} />
            <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
              Deko-Stop <span className="font-normal text-neutral-400 dark:text-neutral-600">(m)</span>
            </span>
          </div>
        )}
      </div>
      <div ref={containerRef} className="h-[180px] w-full" />
    </div>
  )
}

// ── Power Phase Gauge (Pedal Analytics) ──────────────────────────────────────

interface PowerPhaseAvg {
  leftStart: number; leftEnd: number
  leftPeakStart: number; leftPeakEnd: number
  rightStart: number; rightEnd: number
  rightPeakStart: number; rightPeakEnd: number
  leftBalance: number | null
}

function buildGaugeOption(
  startDeg: number, endDeg: number,
  peakStartDeg: number, peakEndDeg: number,
  fullColor: string, peakColor: string,
  ringColor: string, label: string, balance: string | null,
): Record<string, unknown> {
  // ECharts gauge: startAngle=90 means 12 o'clock (TDC), clockwise
  // Gauge angles are counter-clockwise in ECharts, so we invert
  const toGaugeAngle = (deg: number) => 90 - deg

  return {
    type: 'gauge',
    center: ['50%', '50%'],
    radius: '90%',
    startAngle: 90,
    endAngle: -270,
    min: 0,
    max: 360,
    splitNumber: 4,
    pointer: { show: false },
    progress: { show: false },
    axisTick: { show: false },
    splitLine: {
      show: true,
      length: 8,
      distance: 2,
      lineStyle: { width: 2, color: '#737373' },
    },
    axisLabel: {
      show: true,
      distance: 35,
      fontSize: 10,
      color: '#a3a3a3',
      formatter: (value: number) => {
        if (value === 0) return 'TDC'
        if (value === 180) return 'BDC'
        return ''
      },
    },
    title: {
      show: true,
      offsetCenter: [0, balance ? '-15%' : '0%'],
      fontSize: 13,
      fontWeight: 'bold',
      color: '#737373',
    },
    detail: balance ? {
      show: true,
      offsetCenter: [0, '10%'],
      fontSize: 12,
      color: '#a3a3a3',
      formatter: balance,
    } : { show: false },
    data: [{ name: label, value: 0 }],
    axisLine: {
      lineStyle: {
        width: 22,
        color: buildGaugeColors(startDeg, endDeg, fullColor, ringColor),
      },
    },
    // Overlay for peak phase via markLine is not supported,
    // so we use a second gauge series for the peak
  }
}

function buildGaugeColors(startDeg: number, endDeg: number, color: string, ring: string): [number, string][] {
  // Handle wrap-around (e.g. 346° → 201°)
  const s = startDeg / 360
  const e = endDeg > startDeg ? endDeg / 360 : 1  // if wraps, fill to end
  const colors: [number, string][] = []

  if (endDeg > startDeg) {
    // No wrap: ring → active → ring
    if (s > 0) colors.push([s, ring])
    colors.push([e, color])
    if (e < 1) colors.push([1, ring])
  } else {
    // Wrap-around: active → ring → active
    colors.push([endDeg / 360, color])
    if (endDeg / 360 < s) colors.push([s, ring])
    colors.push([1, color])
  }
  return colors
}

function PowerPhaseGaugePanel({ phaseData, streams, hoverIdx }: {
  phaseData: PowerPhaseAvg
  streams: StreamPoint[]
  hoverIdx: number | null
}) {
  // Live data: use hovered stream point, or fall back to averages
  const liveData = useMemo<PowerPhaseAvg>(() => {
    if (hoverIdx == null || hoverIdx < 0 || hoverIdx >= streams.length) return phaseData
    const s = streams[hoverIdx]
    if (s.left_power_phase_start_deg == null) return phaseData
    return {
      leftStart: s.left_power_phase_start_deg ?? phaseData.leftStart,
      leftEnd: s.left_power_phase_end_deg ?? phaseData.leftEnd,
      leftPeakStart: s.left_power_phase_peak_start_deg ?? phaseData.leftPeakStart,
      leftPeakEnd: s.left_power_phase_peak_end_deg ?? phaseData.leftPeakEnd,
      rightStart: s.right_power_phase_start_deg ?? phaseData.rightStart,
      rightEnd: s.right_power_phase_end_deg ?? phaseData.rightEnd,
      rightPeakStart: s.right_power_phase_peak_start_deg ?? phaseData.rightPeakStart,
      rightPeakEnd: s.right_power_phase_peak_end_deg ?? phaseData.rightPeakEnd,
      leftBalance: s.left_right_balance ?? phaseData.leftBalance,
    }
  }, [hoverIdx, streams, phaseData])

  const containerRef = useECharts(() => {
    const d = liveData
    const isDark = document.documentElement.classList.contains('dark')
    const ringColor = isDark ? '#404040' : '#e5e5e5'
    const balL = d.leftBalance != null ? `${d.leftBalance.toFixed(0)}%` : null
    const balR = d.leftBalance != null ? `${(100 - d.leftBalance).toFixed(0)}%` : null
    const isLive = hoverIdx != null

    const leftFull = buildGaugeOption(
      d.leftStart, d.leftEnd,
      d.leftPeakStart, d.leftPeakEnd,
      '#34d399', '#10b981', ringColor, 'LINKS', balL,
    )
    const leftPeak = {
      ...buildGaugeOption(
        d.leftPeakStart, d.leftPeakEnd,
        d.leftPeakStart, d.leftPeakEnd,
        '#10b981', '#10b981', 'transparent', '', null,
      ),
      title: { show: false },
      axisLine: {
        lineStyle: {
          width: 16,
          color: buildGaugeColors(d.leftPeakStart, d.leftPeakEnd, '#10b981', 'transparent'),
        },
      },
    }

    const rightFull = buildGaugeOption(
      d.rightStart, d.rightEnd,
      d.rightPeakStart, d.rightPeakEnd,
      '#60a5fa', '#3b82f6', ringColor, 'RECHTS', balR,
    )
    const rightPeak = {
      ...buildGaugeOption(
        d.rightPeakStart, d.rightPeakEnd,
        d.rightPeakStart, d.rightPeakEnd,
        '#3b82f6', '#3b82f6', 'transparent', '', null,
      ),
      title: { show: false },
      axisLine: {
        lineStyle: {
          width: 16,
          color: buildGaugeColors(d.rightPeakStart, d.rightPeakEnd, '#3b82f6', 'transparent'),
        },
      },
    }

    // Two gauges side by side — center in upper 75% to leave room for labels below
    const cy = '44%'
    leftFull.center = ['25%', cy]
    leftPeak.center = ['25%', cy]
    rightFull.center = ['75%', cy]
    rightPeak.center = ['75%', cy]
    const r = '70%'
    leftFull.radius = r
    leftPeak.radius = r
    rightFull.radius = r
    rightPeak.radius = r

    return {
      animation: false,
      title: {
        text: isLive ? 'Kraftphasen (live)' : 'Kraftphasen (Ø)',
        left: 4, top: 4,
        textStyle: { fontSize: 11, fontWeight: 500, color: isLive ? '#10b981' : '#888' },
      },
      toolbox: {
        feature: {
          saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: 'Kraftphasen' },
        },
      },
      series: [leftFull, leftPeak, rightFull, rightPeak],
    }
  }, [liveData, hoverIdx])

  const d = liveData

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={containerRef} className="h-[240px] w-full" />
      {/* Degree labels below */}
      <div className="flex justify-around text-center -mt-1 mb-1">
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          <div>PP {d.leftStart.toFixed(0)}°–{d.leftEnd.toFixed(0)}°</div>
          <div className="text-[10px] text-neutral-400 dark:text-neutral-500">
            Peak {d.leftPeakStart.toFixed(0)}°–{d.leftPeakEnd.toFixed(0)}°
          </div>
        </div>
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          <div>PP {d.rightStart.toFixed(0)}°–{d.rightEnd.toFixed(0)}°</div>
          <div className="text-[10px] text-neutral-400 dark:text-neutral-500">
            Peak {d.rightPeakStart.toFixed(0)}°–{d.rightPeakEnd.toFixed(0)}°
          </div>
        </div>
      </div>
      {/* Legend */}
      <div className="flex justify-center gap-6 text-xs text-neutral-500">
        <div className="flex items-center gap-1.5">
          <span className="inline-block w-3 h-3 rounded-sm" style={{ background: '#10b981' }} />
          L Power Phase
        </div>
        <div className="flex items-center gap-1.5">
          <span className="inline-block w-3 h-3 rounded-sm" style={{ background: '#3b82f6' }} />
          R Power Phase
        </div>
      </div>
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
  /** Suffix shown after each value in the tooltip */
  leftTooltipSuffix: string
  rightTooltipSuffix: string
  /** Unit string shown in the header after label (default: "%") */
  headerUnit?: string
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
  leftTooltipSuffix, rightTooltipSuffix,
  headerUnit = '%',
  onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister,
  brushZoomLockRef, xMode, splitMarkers,
}: PedalDualChartPanelProps) {
  const containerRef = useECharts(
    () => {
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

      const gridTop = ((splitMarkers && splitMarkers.length > 0) ? 34 : 8) + 18
      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      const mlData = buildSplitMarkLines(splitMarkers, xMode)

      return {
        animation: false,
        grid: { left: 52, right: 76, top: gridTop, bottom: 36 },

        title: {
          text: `${tooltipTitle} (${headerUnit})`,
          left: 4,
          top: 4,
          textStyle: { fontSize: 11, fontWeight: 500, color: '#888' },
        },

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
            saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: tooltipTitle },
          },
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
            const xVal = xMode === 'distance' ? data.distanceKm[idx] : data.elapsedMin[idx]
            const posLine = buildTooltipPosLine(xVal, xMode, data)
            let html = `<div style="line-height:1.75;min-width:160px">`
            html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
            html += `<div style="font-size:11px;color:#888;margin-bottom:2px">${tooltipTitle}</div>`
            const lv = leftValues[idx]
            if (!isNaN(lv)) html += `<div style="font-size:13px;color:${leftColor}"><strong>${lv.toFixed(1)}\u202f${headerUnit}</strong> ${leftTooltipSuffix}</div>`
            const rv = rightValues[idx]
            if (!isNaN(rv)) html += `<div style="font-size:13px;color:${rightColor}"><strong>${rv.toFixed(1)}\u202f${headerUnit}</strong> ${rightTooltipSuffix}</div>`
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
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      return setupChartInteractions({ chart, chartId, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef, xMax, xData })
    },
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-4 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        <div className="flex items-center gap-2">
          <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: leftColor }} />
          <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
            {leftLabel}{headerUnit ? <span className="font-normal text-neutral-400 dark:text-neutral-600"> ({headerUnit})</span> : null}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: rightColor }} />
          <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
            {rightLabel}{headerUnit ? <span className="font-normal text-neutral-400 dark:text-neutral-600"> ({headerUnit})</span> : null}
          </span>
        </div>
      </div>
      <div ref={containerRef} className="h-[180px] w-full" />
    </div>
  )
}

// ── Speed/Pace Unified Chart Panel ────────────────────────────────────────────

/** 30-second simple moving average — smooths noisy per-second pace data. */
function sma(arr: number[], window = 30): number[] {
  const result = new Array(arr.length).fill(NaN)
  let sum = 0, count = 0
  for (let i = 0; i < arr.length; i++) {
    if (!isNaN(arr[i])) { sum += arr[i]; count++ }
    if (i >= window) {
      if (!isNaN(arr[i - window])) { sum -= arr[i - window]; count-- }
    }
    if (count > 0) result[i] = sum / count
  }
  return result
}

/** Format a min/km value as "m:ss min/km" for tooltip display. */
function formatMinKm(minKm: number): string {
  if (isNaN(minKm) || !isFinite(minKm) || minKm <= 0) return '—'
  const min = Math.floor(minKm)
  let sec = Math.round((minKm - min) * 60)
  if (sec >= 60) { return `${min + 1}:00\u202fmin/km` }
  return `${min}:${sec.toString().padStart(2, '0')}\u202fmin/km`
}

const COLOR_PACE = '#3B82F6'
const COLOR_GARMIN_GAP = '#F59E0B'
const COLOR_RE3_GAP = '#10B981'
const COLOR_COROS_EFFORT = '#06B6D4'

interface SpeedPaceChartPanelProps {
  chartId: string
  data: ChartData
  speedMode: 'speed' | 'pace'
  onSpeedModeChange: (mode: 'speed' | 'pace') => void
  sportType: string
  onHover: (index: number | null) => void
  onBrushZoom: (startValue: number, endValue: number) => void
  onSyncZoom: (start: number, end: number, sourceId: string) => void
  onRegister: (id: string, chart: echarts.ECharts) => void
  onUnregister: (id: string) => void
  brushZoomLockRef: React.MutableRefObject<boolean>
  xMode: XMode
  splitMarkers?: SplitMarker[]
}

function SpeedPaceChartPanel({
  chartId, data, speedMode, onSpeedModeChange, sportType,
  onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister,
  brushZoomLockRef, xMode, splitMarkers,
}: SpeedPaceChartPanelProps) {
  const segGroup = getSegmentGroup(sportType)
  const isRowingSport = sportType === 'rowing'
  const isSwimSport = segGroup === 'swim'
  const hasGarminGap = speedMode === 'pace' && hasValidData(data.garminGapMinKm)
  const hasRe3Gap = speedMode === 'pace' && hasValidData(data.re3Gap)
  const hasStrydNgp = speedMode === 'pace' && hasValidData(data.strydNgpMinKm)
  const hasCorosEffort = speedMode === 'pace' && hasValidData(data.corosEffortPaceMinKm)

  const paceSmoothed = useMemo(() => {
    if (speedMode !== 'pace') return null
    const divisor = isRowingSport ? 500 : isSwimSport ? 100 : 1000
    const maxPace = 30
    const raw = data.speedMs.map(ms => {
      if (!ms || ms <= 0 || !isFinite(ms)) return NaN
      const p = divisor / ms / 60
      return p > maxPace ? NaN : p
    })
    return sma(raw)
  }, [data, speedMode, isRowingSport, isSwimSport])

  const garminSmoothed = useMemo(() =>
    hasGarminGap ? sma(data.garminGapMinKm) : null
  , [data, hasGarminGap])

  const re3Smoothed = useMemo(() =>
    hasRe3Gap ? sma(data.re3Gap) : null
  , [data, hasRe3Gap])

  const strydNgpSmoothed = useMemo(() =>
    hasStrydNgp ? sma(data.strydNgpMinKm) : null
  , [data, hasStrydNgp])

  const corosEffortSmoothed = useMemo(() =>
    hasCorosEffort ? sma(data.corosEffortPaceMinKm) : null
  , [data, hasCorosEffort])

  const paceUnit = isRowingSport ? 'min/500m' : isSwimSport ? 'min/100m' : 'min/km'
  // fieldLabel('speed', ...) returns sport-specific label (e.g. "Pace" for foot) — but in
  // speed mode we always want "Geschwindigkeit" regardless of sport type
  const speedLabel = 'Geschwindigkeit'

  const containerRef = useECharts(
    () => {
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

      const gridTop = ((splitMarkers && splitMarkers.length > 0) ? 34 : 8) + 18
      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      const mlData = buildSplitMarkLines(splitMarkers, xMode)

      if (speedMode === 'speed') {
        return {
          animation: false,
          grid: { left: 52, right: 76, top: gridTop, bottom: 36 },

          title: {
            text: `${speedLabel} (km/h)`,
            left: 4,
            top: 4,
            textStyle: { fontSize: 11, fontWeight: 500, color: '#888' },
          },

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
              saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: speedLabel },
            },
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
              const v = data.speed[idx]
              const xVal = xMode === 'distance' ? data.distanceKm[idx] : data.elapsedMin[idx]
              const posLine = buildTooltipPosLine(xVal, xMode, data)
              let html = `<div style="line-height:1.75;min-width:130px">`
              html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
              if (!isNaN(v)) html += `<div style="font-size:13px;color:#3b82f6"><strong>${v.toFixed(1)}\u202fkm/h</strong></div>`
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
            axisLabel: { fontSize: 11, color: '#666', margin: 8 },
            splitLine: { lineStyle: { color: '#eee', type: 'dashed' } },
            axisLine: { show: false },
            axisTick: { show: false },
          },

          series: [{
            type: 'line',
            data: xData.map((x, i) => [x, data.speed[i]]),
            showSymbol: false,
            lineStyle: { width: 1.5, color: '#3b82f6' },
            itemStyle: { color: '#3b82f6' },
            connectNulls: false,
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
      }

      // Pace mode
      const ps = paceSmoothed!
      const gs = garminSmoothed
      const rs = re3Smoothed
      const ns = strydNgpSmoothed
      const cs = corosEffortSmoothed

      const allValues = [
        ...ps,
        ...(gs ?? []),
        ...(rs ?? []),
        ...(ns ?? []),
        ...(cs ?? []),
      ].filter(v => !isNaN(v) && isFinite(v) && v > 0 && v < 30)

      const yMin = allValues.length > 0 ? Math.max(0, Math.floor(Math.min(...allValues) * 10) / 10 - 0.5) : 0
      const yMax = allValues.length > 0 ? Math.ceil(Math.max(...allValues) * 10) / 10 + 0.5 : 15

      const series: echarts.EChartsCoreOption['series'] = [
        {
          type: 'line',
          name: 'Pace',
          data: xData.map((x, i) => [x, ps[i]]),
          showSymbol: false,
          lineStyle: { width: 1.5, color: COLOR_PACE },
          itemStyle: { color: COLOR_PACE },
          connectNulls: false,
          ...(mlData.length > 0 ? {
            markLine: {
              silent: true,
              symbol: ['none', 'none'],
              data: mlData,
            },
          } : {}),
        },
        ...(gs ? [{
          type: 'line' as const,
          name: 'Garmin GAP',
          data: xData.map((x, i) => [x, gs[i]]),
          showSymbol: false,
          lineStyle: { width: 1.5, color: COLOR_GARMIN_GAP },
          itemStyle: { color: COLOR_GARMIN_GAP },
          connectNulls: false,
        }] : []),
        ...(rs ? [{
          type: 'line' as const,
          name: 'RE3 GAP',
          data: xData.map((x, i) => [x, rs[i]]),
          showSymbol: false,
          lineStyle: { width: 1.5, color: COLOR_RE3_GAP },
          itemStyle: { color: COLOR_RE3_GAP },
          connectNulls: false,
        }] : []),
        ...(ns ? [{
          type: 'line' as const,
          name: 'Stryd NGP',
          data: xData.map((x, i) => [x, ns[i]]),
          showSymbol: false,
          lineStyle: { width: 1.5, color: '#a855f7' },
          itemStyle: { color: '#a855f7' },
          symbol: 'none',
          z: 1,
          connectNulls: false,
        }] : []),
        ...(cs ? [{
          type: 'line' as const,
          name: 'Coros Effort Pace',
          data: xData.map((x, i) => [x, cs[i]]),
          showSymbol: false,
          lineStyle: { width: 1.5, color: COLOR_COROS_EFFORT },
          itemStyle: { color: COLOR_COROS_EFFORT },
          connectNulls: false,
        }] : []),
      ]

      return {
        animation: false,
        grid: { left: 52, right: 76, top: gridTop, bottom: 36 },

        title: {
          text: `Pace (${paceUnit})`,
          left: 4,
          top: 4,
          textStyle: { fontSize: 11, fontWeight: 500, color: '#888' },
        },

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
            saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: `Pace (${paceUnit})` },
          },
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
            const xVal = xMode === 'distance' ? data.distanceKm[idx] : data.elapsedMin[idx]
            const posLine = buildTooltipPosLine(xVal, xMode, data)
            let html = `<div style="line-height:1.75;min-width:180px">`
            html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
            const pv = ps[idx]
            if (!isNaN(pv)) {
              // Primary pace: sport-specific unit
              let min = Math.floor(pv)
              let sec = Math.round((pv - min) * 60)
              if (sec >= 60) { min += 1; sec = 0 }
              html += `<div style="font-size:13px;color:${COLOR_PACE}"><strong>${min}:${sec.toString().padStart(2, '0')}\u202f${paceUnit}</strong> Pace</div>`
            }
            if (gs) {
              const gv = gs[idx]
              if (!isNaN(gv)) html += `<div style="font-size:13px;color:${COLOR_GARMIN_GAP}"><strong>${formatMinKm(gv)}</strong> Garmin GAP</div>`
            }
            if (rs) {
              const rv = rs[idx]
              if (!isNaN(rv)) html += `<div style="font-size:13px;color:${COLOR_RE3_GAP}"><strong>${formatMinKm(rv)}</strong> RE3 GAP</div>`
            }
            if (ns) {
              const nv = ns[idx]
              if (!isNaN(nv)) html += `<div style="font-size:13px;color:#a855f7"><strong>${formatMinKm(nv)}</strong> Stryd NGP</div>`
            }
            if (cs) {
              const cv = cs[idx]
              if (!isNaN(cv)) html += `<div style="font-size:13px;color:${COLOR_COROS_EFFORT}"><strong>${formatMinKm(cv)}</strong> Coros Effort Pace</div>`
            }
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
          inverse: true,
          min: yMin,
          max: yMax,
          axisLabel: {
            fontSize: 11,
            color: '#666',
            margin: 8,
            formatter: (v: number) => {
              const min = Math.floor(v)
              const sec = Math.round((v - min) * 60)
              return `${min}:${sec.toString().padStart(2, '0')}`
            },
          },
          splitLine: { lineStyle: { color: '#eee', type: 'dashed' } },
          axisLine: { show: false },
          axisTick: { show: false },
        },

        series,

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
    [data, speedMode, sportType, xMode, splitMarkers, paceSmoothed, garminSmoothed, re3Smoothed, strydNgpSmoothed, corosEffortSmoothed], // eslint-disable-line react-hooks/exhaustive-deps
    (chart) => {
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      return setupChartInteractions({ chart, chartId, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef, xMax, xData })
    },
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-3 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        {speedMode === 'pace' ? (
          <>
            <div className="flex items-center gap-1.5">
              <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: COLOR_PACE }} />
              <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">Pace</span>
            </div>
            {hasGarminGap && (
              <div className="flex items-center gap-1.5">
                <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: COLOR_GARMIN_GAP }} />
                <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">Garmin GAP</span>
              </div>
            )}
            {hasRe3Gap && (
              <div className="flex items-center gap-1.5">
                <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: COLOR_RE3_GAP }} />
                <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">RE3 GAP</span>
              </div>
            )}
            {hasStrydNgp && (
              <div className="flex items-center gap-1.5">
                <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: '#a855f7' }} />
                <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">Stryd NGP</span>
              </div>
            )}
            {hasCorosEffort && (
              <div className="flex items-center gap-1.5">
                <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: COLOR_COROS_EFFORT }} />
                <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">Coros Effort Pace</span>
              </div>
            )}
          </>
        ) : (
          <div className="flex items-center gap-1.5">
            <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: '#3b82f6' }} />
            <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
              {speedLabel} <span className="font-normal text-neutral-400 dark:text-neutral-600">(km/h)</span>
            </span>
          </div>
        )}
        <div role="radiogroup" aria-label="Geschwindigkeitsformat" className="ml-auto inline-flex rounded-lg border border-neutral-200 dark:border-neutral-700 text-xs overflow-hidden">
          <button
            role="radio"
            aria-checked={speedMode === 'pace'}
            onClick={() => onSpeedModeChange('pace')}
            className={`px-2.5 py-0.5 cursor-pointer transition-colors ${
              speedMode === 'pace'
                ? 'bg-accent-DEFAULT text-white'
                : 'hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-400'
            }`}
          >
            Pace
          </button>
          <button
            role="radio"
            aria-checked={speedMode === 'speed'}
            onClick={() => onSpeedModeChange('speed')}
            className={`px-2.5 py-0.5 cursor-pointer transition-colors ${
              speedMode === 'speed'
                ? 'bg-accent-DEFAULT text-white'
                : 'hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-400'
            }`}
          >
            Geschwindigkeit
          </button>
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

      const gridTop = ((splitMarkers && splitMarkers.length > 0) ? 34 : 8) + 18
      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      const mlData = buildSplitMarkLines(splitMarkers, xMode)

      return {
        animation: false,
        grid: { left: 52, right: 76, top: gridTop, bottom: 36 },

        title: {
          text: 'Laufdynamik (GCT / VO)',
          left: 4,
          top: 4,
          textStyle: { fontSize: 11, fontWeight: 500, color: '#888' },
        },

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
            saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: 'Laufdynamik' },
          },
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
            const xVal = xMode === 'distance' ? data.distanceKm[idx] : data.elapsedMin[idx]
            const posLine = buildTooltipPosLine(xVal, xMode, data)
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
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      return setupChartInteractions({ chart, chartId, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef, xMax, xData })
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

// ── Running Smoothness Chart Panel ────────────────────────────────────────────

interface RunningSmoothnessChartPanelProps {
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

function RunningSmoothnessChartPanel({
  chartId, data, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister,
  brushZoomLockRef, xMode, splitMarkers,
}: RunningSmoothnessChartPanelProps) {
  const containerRef = useECharts(
    () => {
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
      const values = data.runningSmoothness

      const gridTop = ((splitMarkers && splitMarkers.length > 0) ? 34 : 8) + 18
      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      const mlData = buildSplitMarkLines(splitMarkers, xMode)

      return {
        animation: false,
        grid: { left: 52, right: 16, top: gridTop, bottom: 36 },

        title: {
          text: 'Running Smoothness',
          left: 4,
          top: 4,
          textStyle: { fontSize: 11, fontWeight: 500, color: '#888' },
        },

        brush: {
          toolbox: [],
          brushType: 'lineX',
          brushStyle: {
            borderWidth: 1,
            color: 'rgba(139,92,246,0.12)',
            borderColor: '#8b5cf6',
          },
          throttleType: 'debounce',
          throttleDelay: 100,
        },

        toolbox: {
          feature: {
            brush: { show: false },
            saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: 'Running Smoothness' },
          },
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
            lineStyle: { color: '#8b5cf6', width: 1, type: 'dashed' },
            animation: false,
          },
          formatter: (params: unknown) => {
            const arr = Array.isArray(params) ? params : [params]
            const p = arr[0] as { dataIndex?: number }
            if (p?.dataIndex == null) return ''
            const idx = p.dataIndex
            const xVal = xMode === 'distance' ? data.distanceKm[idx] : data.elapsedMin[idx]
            const posLine = buildTooltipPosLine(xVal, xMode, data)
            let html = `<div style="line-height:1.75;min-width:150px">`
            html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
            const v = values[idx]
            if (!isNaN(v)) html += `<div style="font-size:13px;color:#8B5CF6"><strong>${Math.round(v)}\u202f%</strong> Running Smoothness</div>`
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
          name: '%',
          nameTextStyle: { fontSize: 10, color: '#8B5CF6' },
          min: 0,
          max: 100,
          axisLabel: { fontSize: 11, color: '#8B5CF6', margin: 8 },
          splitLine: { lineStyle: { color: '#eee', type: 'dashed' } },
          axisLine: { show: false },
          axisTick: { show: false },
        },

        series: [
          {
            type: 'line',
            data: xData.map((x, i) => [x, values[i]]),
            showSymbol: false,
            lineStyle: { width: 1.5, color: '#8B5CF6' },
            itemStyle: { color: '#8B5CF6' },
            areaStyle: { color: 'rgba(139,92,246,0.08)' },
            connectNulls: false,
            ...(mlData.length > 0 ? {
              markLine: {
                silent: true,
                symbol: ['none', 'none'],
                data: mlData,
              },
            } : {}),
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
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      return setupChartInteractions({ chart, chartId, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef, xMax, xData })
    },
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-4 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        <div className="flex items-center gap-2">
          <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: '#8B5CF6' }} />
          <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
            Running Smoothness <span className="font-normal text-neutral-400 dark:text-neutral-600">(%)</span>
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

      const gridTop = ((splitMarkers && splitMarkers.length > 0) ? 34 : 8) + 18
      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      // Average of non-NaN values for markLine
      const validValues = values.filter(v => !isNaN(v) && isFinite(v))
      const avg = validValues.length > 0
        ? validValues.reduce((a, b) => a + b, 0) / validValues.length
        : null

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
      mlData.push(...buildSplitMarkLines(splitMarkers, xMode))

      return {
        animation: false,
        grid: { left: 52, right: 52, top: gridTop, bottom: 36 },

        title: {
          text: `${label} (${unit})`,
          left: 4,
          top: 4,
          textStyle: { fontSize: 11, fontWeight: 500, color: '#888' },
        },

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
            saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: label },
          },
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
            const xVal = xMode === 'distance' ? data.distanceKm[idx] : data.elapsedMin[idx]
            const posLine = buildTooltipPosLine(xVal, xMode, data)
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
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      return setupChartInteractions({ chart, chartId, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef, xMax, xData })
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
  /** Data field key. Optional when overrideValues is provided (overrideValues takes precedence). */
  field?: FieldKey
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
  /** Additional overlay series to render on the same chart. */
  extraSeries?: Array<{
    values: number[]
    name: string
    color: string
    lineStyle?: { color?: string; width?: number }
    areaStyle?: { color?: string }
    symbol?: string
    z?: number
    /** Connect NaN gaps so the filled area stays contiguous (useful for sparse deco-stop data). */
    connectNulls?: boolean
    /** Disable mouse events on this series (prevents blocking brush/zoom). */
    silent?: boolean
  }>
}

function ChartPanel({
  chartId, data, field, unit, label, color, area,
  onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister,
  brushZoomLockRef, markLines = 'none', altitudeSmartY, showGradeColor, xMode,
  sunMarkLines, splitMarkers, overrideValues, invertY, tooltipValueFn, yAxisFixed, extraSeries,
}: ChartPanelProps) {
  const containerRef = useECharts(
    () => {
      const values = overrideValues ?? (field ? data[field] : [])
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

      // Extra top padding for title + sunrise/sunset icons or split marker labels above the chart
      const gridTop = ((sunMarkLines && sunMarkLines.length > 0) || (splitMarkers && splitMarkers.length > 0) ? 34 : 8) + 18

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

      mlData.push(...buildSplitMarkLines(splitMarkers, xMode))

      // ── Tooltip builder ─────────────────────────────────────────────────────
      const decimals = ['altitude', 'temperature', 'heartRate', 'power', 'cadence'].includes(field ?? '') ? 0 : 1

      const buildTooltip = (idx: number): string => {
        const v = values[idx]
        if (isNaN(v)) return ''
        const xVal = xMode === 'distance' ? data.distanceKm[idx] : data.elapsedMin[idx]
        const posLine = buildTooltipPosLine(xVal, xMode, data)

        const valStr = tooltipValueFn ? tooltipValueFn(v) : `${v.toFixed(decimals)}\u2009${unit}`

        let html = `<div style="line-height:1.75;min-width:130px">`
        html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
        html += `<div style="font-size:13px;color:${color}"><strong>${valStr}</strong></div>`

        if (extraSeries) {
          for (const es of extraSeries) {
            const ev = es.values[idx]
            if (!isNaN(ev)) {
              html += `<div style="font-size:13px;color:${es.color}"><strong>${ev.toFixed(decimals)}\u2009${unit}</strong> ${es.name}</div>`
            }
          }
        }

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

        title: {
          text: `${label} (${unit})`,
          left: 4,
          top: 4,
          textStyle: { fontSize: 11, fontWeight: 500, color: '#888' },
        },

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
            saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: label },
          },
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
              ...(extraSeries ?? []).map(es => ({
                type: 'line' as const,
                name: es.name,
                data: xData.map((x, i) => [x, es.values[i]]),
                showSymbol: false,
                symbol: es.symbol ?? 'none',
                lineStyle: es.lineStyle
                  ? { ...es.lineStyle, type: 'solid' as const }
                  : { width: 1.5, color: es.color, type: 'solid' as const },
                itemStyle: { color: es.color },
                ...(es.areaStyle != null ? { areaStyle: es.areaStyle } : {}),
                connectNulls: es.connectNulls ?? false,
                z: es.z ?? 2,
                ...(es.silent != null ? { silent: es.silent } : {}),
              })),
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
    [data, field, xMode, splitMarkers, showGradeColor, overrideValues, invertY, yAxisFixed, extraSeries], // eslint-disable-line react-hooks/exhaustive-deps
    (chart) => {
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      return setupChartInteractions({ chart, chartId, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef, xMax, xData })
    },
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-3 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        <div className="flex items-center gap-2">
          <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: color }} />
          <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
            {label}{' '}
            <span className="font-normal text-neutral-400 dark:text-neutral-600">({unit})</span>
          </span>
        </div>
        {extraSeries?.map(es => (
          <div key={es.name} className="flex items-center gap-2">
            <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: es.color }} />
            <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">{es.name}</span>
          </div>
        ))}
      </div>
      <div ref={containerRef} className="h-[180px] w-full" />
    </div>
  )
}

// ── Gear Chart Panel (Gangschaltung — Treppenfunktion) ────────────────────────

interface GearChartPanelProps {
  chartId: string
  data: ChartData
  gearPoints: GearPoint[]
  onHover: (index: number | null) => void
  onBrushZoom: (startValue: number, endValue: number) => void
  onSyncZoom: (start: number, end: number, sourceId: string) => void
  onRegister: (id: string, chart: echarts.ECharts) => void
  onUnregister: (id: string) => void
  brushZoomLockRef: React.MutableRefObject<boolean>
  xMode: XMode
  splitMarkers?: SplitMarker[]
}

function GearChartPanel({
  chartId, data, gearPoints,
  onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister,
  brushZoomLockRef, xMode, splitMarkers,
}: GearChartPanelProps) {
  const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

  // Build step-chart series: [xVal, ratio] pairs for ECharts step:'start' line
  const stepData = useMemo(() => {
    if (gearPoints.length === 0) return []
    const result: [number, number][] = []
    for (const pt of gearPoints) {
      if (pt.rearGear === 0) continue
      result.push([pt.xVal, pt.frontGear / pt.rearGear])
    }
    // Extend the last step to the end of the activity
    if (result.length > 0) {
      result.push([xMax, result[result.length - 1][1]])
    }
    return result
  }, [gearPoints, xMax])

  const containerRef = useECharts(
    () => {
      if (stepData.length === 0) return null as unknown as echarts.EChartsCoreOption

      const gridTop = ((splitMarkers && splitMarkers.length > 0) ? 34 : 8) + 18
      const xFormatter = xMode === 'distance'
        ? (v: number) => v.toFixed(1)
        : (v: number) => formatElapsed(v, data.maxElapsed)

      const mlData = buildSplitMarkLines(splitMarkers, xMode)

      // Y range from actual ratios
      const ratios = stepData.slice(0, -1).map(d => d[1]).filter(v => !isNaN(v) && isFinite(v))
      const yMin = Math.max(0, Math.floor(Math.min(...ratios) * 10) / 10 - 0.1)
      const yMax = Math.ceil(Math.max(...ratios) * 10) / 10 + 0.1

      return {
        animation: false,
        grid: { left: 52, right: 76, top: gridTop, bottom: 36 },

        title: {
          text: 'Schaltung (Übersetzung)',
          left: 4,
          top: 4,
          textStyle: { fontSize: 11, fontWeight: 500, color: '#888' },
        },

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
            saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: 'Schaltung' },
          },
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
            const p = arr[0] as { data?: [number, number] }
            if (!p?.data) return ''
            const xVal = p.data[0]
            const ratio = p.data[1]
            if (isNaN(ratio) || !isFinite(ratio)) return ''

            // Find the active gear at this X position (last event at or before xVal)
            let activeGear: GearPoint | null = null
            for (let i = gearPoints.length - 1; i >= 0; i--) {
              if (gearPoints[i].xVal <= xVal && gearPoints[i].rearGear > 0) {
                activeGear = gearPoints[i]
                break
              }
            }

            const posLine = buildTooltipPosLine(xVal, xMode, data)

            let html = `<div style="line-height:1.75;min-width:140px">`
            html += `<div style="color:#888;font-size:11px;margin-bottom:1px">${posLine}</div>`
            if (activeGear) {
              html += `<div style="font-size:14px;font-weight:600;color:#6366f1">${activeGear.frontGear}\u00d7${activeGear.rearGear}</div>`
              html += `<div style="font-size:11px;color:#888">Übersetzung: ${ratio.toFixed(2)}</div>`
            } else {
              html += `<div style="font-size:13px;color:#6366f1"><strong>Übersetzung ${ratio.toFixed(2)}</strong></div>`
            }
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
          min: yMin,
          max: yMax,
          name: 'Gang',
          nameTextStyle: { fontSize: 10, color: '#666' },
          axisLabel: {
            fontSize: 11,
            color: '#666',
            margin: 8,
            formatter: (v: number) => v.toFixed(1),
          },
          splitLine: { lineStyle: { color: '#eee', type: 'dashed' } },
          axisLine: { show: false },
          axisTick: { show: false },
        },

        series: [{
          type: 'line',
          data: stepData,
          step: 'start',
          showSymbol: false,
          lineStyle: { width: 2, color: '#6366f1' },
          itemStyle: { color: '#6366f1' },
          areaStyle: { color: 'rgba(99,102,241,0.12)' },
          connectNulls: false,
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
    [data, stepData, gearPoints, xMode, splitMarkers, xMax], // eslint-disable-line react-hooks/exhaustive-deps
    (chart) => {
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      return setupChartInteractions({ chart, chartId, onHover, onBrushZoom, onSyncZoom, onRegister, onUnregister, brushZoomLockRef, xMax, xData })
    },
  )

  if (stepData.length === 0) return null

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        <span className="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: '#6366f1' }} />
        <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
          Gangschaltung{' '}
          <span className="font-normal text-neutral-400 dark:text-neutral-600">(Übersetzungsverhältnis)</span>
        </span>
      </div>
      <div ref={containerRef} className="h-[180px] w-full" />
    </div>
  )
}
