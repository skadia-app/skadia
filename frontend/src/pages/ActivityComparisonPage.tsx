import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQueries } from '@tanstack/react-query'
import { fmtDateTime } from '@/lib/format'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import type { SupportedLanguage } from '@/i18n/languages'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { fetchActivity, fetchActivityStreams } from '@/api/activities'
import type { Activity, StreamPoint } from '@/api/types'
import { formatDuration } from '@/lib/utils'
import type { UnitFormatters } from '@/lib/useUnitFormatters'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import {
  distanceInputUnit,
  distanceToDisplayValue,
  elevationInputUnit,
  elevationToDisplayValue,
  paceToDisplayValue,
  speedToDisplayValue,
} from '@/lib/units'
import { sportLabel } from '@/lib/sportTaxonomy'
import { displayActivityTitle } from '@/lib/activityTitle'
import { fieldUnit, formatSpeedValue, defaultSpeedMode } from '@/lib/sportFieldLabels'

import { SportIcon } from '@/components/SportIcon'

echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  CanvasRenderer,
])

// ── Colors for up to 6 activities ────────────────────────────────────────────

const COMPARE_COLORS = ['#3b82f6', '#ef4444', '#22c55e', '#f59e0b', '#8b5cf6', '#06b6d4']

// ── Types ────────────────────────────────────────────────────────────────────

type XMode = 'distance' | 'time'
type ChartTab = 'speed' | 'heartRate' | 'altitude' | 'power'

interface ProcessedStream {
  distanceKm: number[]
  elapsedMin: number[]
  speed: number[]
  heartRate: number[]
  altitude: number[]
  power: number[]
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function processStreams(streams: StreamPoint[]): ProcessedStream {
  const distanceKm: number[] = []
  const elapsedMin: number[] = []
  const speed: number[] = []
  const heartRate: number[] = []
  const altitude: number[] = []
  const power: number[] = []

  const t0 = streams.length > 0 ? new Date(streams[0].time).getTime() : 0

  for (const s of streams) {
    distanceKm.push(
      s.distance_m != null
        ? s.distance_m / 1000
        : distanceKm.length > 0 ? distanceKm[distanceKm.length - 1] : 0,
    )
    elapsedMin.push((new Date(s.time).getTime() - t0) / 60000)
    speed.push(s.speed_ms != null ? s.speed_ms * 3.6 : NaN)
    heartRate.push(s.heart_rate ?? NaN)
    altitude.push(s.altitude_m ?? NaN)
    power.push(s.power_w ?? NaN)
  }

  return { distanceKm, elapsedMin, speed, heartRate, altitude, power }
}

function hasValidData(arr: number[]): boolean {
  return arr.some(v => !isNaN(v))
}

function downsamplePairs(xs: number[], ys: number[], maxPoints: number): [number, number][] {
  const n = Math.min(xs.length, ys.length)
  if (n <= maxPoints) {
    const out: [number, number][] = []
    for (let i = 0; i < n; i++) out.push([xs[i], ys[i]])
    return out
  }
  const step = n / maxPoints
  const out: [number, number][] = []
  for (let i = 0; i < maxPoints; i++) {
    const idx = Math.floor(i * step)
    out.push([xs[idx], ys[idx]])
  }
  return out
}

function formatDecimalMinutes(value: number): string {
  const totalSeconds = Math.round(value * 60)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = String(totalSeconds % 60).padStart(2, '0')
  return `${minutes}:${seconds}`
}

function formatChartSpeedValue(
  value: number,
  speedMode: 'speed' | 'pace',
  unit: string,
  unitFormatters: Pick<UnitFormatters, 'options'>,
): string {
  if (!isFinite(value) || value <= 0) return '\u2014'
  if (speedMode === 'pace') {
    return `${formatDecimalMinutes(value)} ${unit}`
  }
  const formatted = new Intl.NumberFormat(unitFormatters.options.locale, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  }).format(value)
  return `${formatted} ${unit}`
}

function chartSpeedUnit(sportType: string, unitFormatters: Pick<UnitFormatters, 'options'>): string {
  return fieldUnit('speed', sportType, unitFormatters.options)
}

function chartSpeedDisplayValue(
  kmh: number,
  speedMode: 'speed' | 'pace',
  speedUnit: string,
  unitFormatters: Pick<UnitFormatters, 'options'>,
): number {
  const speedMs = kmh / 3.6
  if (speedMode !== 'pace') return speedToDisplayValue(speedMs, unitFormatters.options.units)
  if (speedUnit === 'min/100m') return speedMs > 0 ? (100 / speedMs) / 60 : NaN
  if (speedUnit === 'min/500m') return speedMs > 0 ? (500 / speedMs) / 60 : NaN
  return speedMs > 0 ? paceToDisplayValue(1000 / speedMs, unitFormatters.options.units) : NaN
}

function formatHr(v: number | null | undefined): string {
  if (v == null || isNaN(v)) return '\u2014'
  return `${Math.round(v)} bpm`
}

function formatPower(v: number | null | undefined): string {
  if (v == null || isNaN(v)) return '\u2014'
  return `${Math.round(v)} W`
}

function formatCalories(v: number | null | undefined): string {
  if (v == null || isNaN(v)) return '\u2014'
  return `${Math.round(v)} kcal`
}

// ── Metric row definitions ───────────────────────────────────────────────────

interface MetricRow {
  labelKey: string
  getValue: (a: Activity) => string
  getRawValue: (a: Activity) => number | null | undefined
  higherIsBetter?: boolean // defaults to false (lower is better for time/speed)
}

function metricRows(unitFormatters: Pick<UnitFormatters, 'formatDistance' | 'formatElevation' | 'options'>): MetricRow[] {
  return [
    {
      labelKey: 'distance',
      getValue: a => unitFormatters.formatDistance(a.distance_m),
      getRawValue: a => a.distance_m,
      higherIsBetter: true,
    },
    {
      labelKey: 'duration',
      getValue: a => formatDuration(a.elapsed_time_s ?? 0),
      getRawValue: a => a.elapsed_time_s,
      higherIsBetter: false,
    },
    {
      labelKey: 'elevationGain',
      getValue: a => unitFormatters.formatElevation(a.elevation_gain_m),
      getRawValue: a => a.elevation_gain_m,
      higherIsBetter: true,
    },
    {
      labelKey: 'speed',
      getValue: a => formatSpeedValue(a.avg_speed_ms ?? 0, a.sport_type, undefined, unitFormatters.options),
      getRawValue: a => a.avg_speed_ms,
      higherIsBetter: true,
    },
    {
      labelKey: 'heartRate',
      getValue: a => formatHr(a.avg_heart_rate),
      getRawValue: a => a.avg_heart_rate,
    },
    {
      labelKey: 'maxHeartRate',
      getValue: a => formatHr(a.max_heart_rate),
      getRawValue: a => a.max_heart_rate,
    },
    {
      labelKey: 'power',
      getValue: a => formatPower(a.avg_power_w),
      getRawValue: a => a.avg_power_w,
      higherIsBetter: true,
    },
    {
      labelKey: 'np',
      getValue: a => formatPower(a.normalized_power_w),
      getRawValue: a => a.normalized_power_w,
      higherIsBetter: true,
    },
    {
      labelKey: 'tss',
      getValue: a => a.tss != null ? `${Math.round(a.tss)}` : '\u2014',
      getRawValue: a => a.tss,
    },
    {
      labelKey: 'calories',
      getValue: a => formatCalories(a.total_calories),
      getRawValue: a => a.total_calories,
      higherIsBetter: true,
    },
  ]
}

// ── Chart Tab type ───────────────────────────────────────────────────────────

type ChartTabDef = { key: ChartTab; label: string; unit: string; streamKey: keyof ProcessedStream }

// ── Main Component ───────────────────────────────────────────────────────────

export default function ActivityComparisonPage() {
  const { t, i18n } = useTranslation()
  const unitFormatters = useUnitFormatters()
  const language = supportedLanguageFrom(i18n.language)
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const idsParam = searchParams.get('ids') ?? ''
  const ids = useMemo(() => idsParam.split(',').filter(Boolean).slice(0, 6), [idsParam])

  const [chartTab, setChartTab] = useState<ChartTab>('speed')
  const [xMode, setXMode] = useState<XMode>('distance')

  // Fetch activities and streams in parallel
  const activityQueries = useQueries({
    queries: ids.map(id => ({
      queryKey: ['activity', id],
      queryFn: () => fetchActivity(id),
    })),
  })

  const streamQueries = useQueries({
    queries: ids.map(id => ({
      queryKey: ['activity-streams', id],
      queryFn: () => fetchActivityStreams(id),
    })),
  })

  const activities = activityQueries.map(q => q.data).filter((a): a is Activity => !!a)
  const streams = streamQueries.map(q => q.data).filter((s): s is StreamPoint[] => !!s)
  const processedStreams = useMemo(() => streams.map(s => processStreams(s)), [streams])
  const rows = useMemo(() => metricRows(unitFormatters), [unitFormatters])

  const primarySport = activities[0]?.sport_type ?? 'generic'
  const speedMode = defaultSpeedMode(primarySport)

  const chartTabs = useMemo<ChartTabDef[]>(() => [
    {
      key: 'speed',
      label: speedMode === 'pace' ? t('activityComparison.chartTabs.pace') : t('activityComparison.chartTabs.speed'),
      unit: chartSpeedUnit(primarySport, unitFormatters),
      streamKey: 'speed',
    },
    { key: 'heartRate', label: t('activityComparison.chartTabs.heartRate'), unit: 'bpm', streamKey: 'heartRate' },
    { key: 'altitude', label: t('activityComparison.chartTabs.altitude'), unit: elevationInputUnit(unitFormatters.options.units), streamKey: 'altitude' },
    { key: 'power', label: t('activityComparison.chartTabs.power'), unit: 'W', streamKey: 'power' },
  ], [primarySport, speedMode, t, unitFormatters])

  const isLoading = activityQueries.some(q => q.isLoading) || streamQueries.some(q => q.isLoading)
  const hasError = activityQueries.some(q => q.isError) || streamQueries.some(q => q.isError)
  const bestIdxMap = useMemo(() =>
    rows.map(row => {
      const values = activities.map(a => row.getRawValue(a))
      const validIdxs = values.map((v, i) => ({ v, i })).filter(({ v }) => v != null && !isNaN(v as number))
      if (validIdxs.length < 2) return null
      const higherIsBetter = row.higherIsBetter ?? false
      return validIdxs.reduce((best, curr) =>
        higherIsBetter
          ? ((curr.v as number) > (best.v as number) ? curr : best)
          : ((curr.v as number) < (best.v as number) ? curr : best),
      ).i
    }),
  [activities, rows])

  if (ids.length < 2) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <p className="text-neutral-500 dark:text-neutral-400">
          {t('activityComparison.selectAtLeastTwo')}
        </p>
        <button
          onClick={() => navigate('/activities')}
          className="px-4 py-2 text-sm rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white font-medium cursor-pointer"
        >
          {t('activityComparison.backToActivities')}
        </button>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-neutral-300 border-t-blue-600 dark:border-neutral-600 dark:border-t-blue-400" />
      </div>
    )
  }

  if (hasError || activities.length < 2) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <p className="text-red-500">{t('activityComparison.loadError')}</p>
        <button
          onClick={() => navigate('/activities')}
          className="px-4 py-2 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer"
        >
          {t('common.back')}
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <button
          onClick={() => navigate('/activities')}
          className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors cursor-pointer"
        >
          <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" />
          </svg>
        </button>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          {t('activityComparison.title')}
        </h1>
        <span className="text-sm text-neutral-400">{t('activityComparison.activityCount', { count: activities.length })}</span>
      </div>

      {/* Section A: Metrics Table */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-x-auto">
        <table className={`w-full ${activities.length >= 5 ? 'text-xs' : 'text-sm'}`}>
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50">
              <th className="text-left px-4 py-3 font-medium text-neutral-500 dark:text-neutral-400 w-36">
                {t('activityComparison.metric')}
              </th>
              {activities.map((a, i) => (
                <th key={a.id} className="text-left px-4 py-3">
                  <div className="flex items-center gap-2">
                    <div
                      className="w-3 h-3 rounded-full flex-shrink-0"
                      style={{ backgroundColor: COMPARE_COLORS[i] }}
                    />
                    <SportIcon sport={a.sport_type} subType={a.sub_type} isElectric={a.is_electric} className="w-4 h-4 flex-shrink-0" />
                    <div className="min-w-0">
                      <div className="font-medium text-neutral-800 dark:text-neutral-200 truncate">
                        {displayActivityTitle(a, sportLabel(a.sport_type, a.sub_type, { language }))}
                      </div>
                      <div className="text-xs text-neutral-400 dark:text-neutral-500">
                        {fmtDateTime(a.started_at, a.timezone)}
                      </div>
                    </div>
                  </div>
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
            {rows.map((row, rowIndex) => {
              const bestIdx = bestIdxMap[rowIndex]
              return (
                <tr key={row.labelKey}>
                  <td className="px-4 py-2.5 text-neutral-500 dark:text-neutral-400 font-medium">
                    {t(`activityComparison.metrics.${row.labelKey}`)}
                  </td>
                  {activities.map((a, i) => (
                    <td
                      key={a.id}
                      className={`px-4 py-2.5 tabular-nums ${
                        bestIdx === i
                          ? 'text-neutral-900 dark:text-neutral-100 font-semibold'
                          : 'text-neutral-600 dark:text-neutral-400'
                      }`}
                    >
                      {row.getValue(a)}
                    </td>
                  ))}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      {/* Section B: Time Series Charts */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex gap-1">
            {chartTabs.map(tab => (
              <button
                key={tab.key}
                onClick={() => setChartTab(tab.key)}
                className={`px-3 py-1.5 text-xs rounded-lg cursor-pointer transition-colors ${
                  chartTab === tab.key
                    ? 'bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                    : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                {tab.label}
              </button>
            ))}
          </div>
          <div className="flex gap-1 rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
            <button
              onClick={() => setXMode('distance')}
              className={`px-3 py-1.5 cursor-pointer transition-colors ${
                xMode === 'distance'
                  ? 'bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                  : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
              }`}
            >
              {t('activityComparison.xMode.distance')}
            </button>
            <button
              onClick={() => setXMode('time')}
              className={`px-3 py-1.5 cursor-pointer transition-colors ${
                xMode === 'time'
                  ? 'bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                  : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
              }`}
            >
              {t('activityComparison.xMode.time')}
            </button>
          </div>
        </div>
        <ComparisonChart
          activities={activities}
          processedStreams={processedStreams}
          tab={chartTab}
          xMode={xMode}
          chartTabs={chartTabs}
          primarySport={primarySport}
          speedMode={speedMode}
          unitFormatters={unitFormatters}
          language={language}
        />
      </div>

      {/* Section C: Map Overlay */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <ComparisonMap
          activities={activities}
          streams={streams}
          noGpsLabel={t('activityComparison.noGpsData')}
          language={language}
        />
      </div>
    </div>
  )
}

// ── ECharts Comparison Chart ─────────────────────────────────────────────────

function ComparisonChart({
  activities,
  processedStreams,
  tab,
  xMode,
  chartTabs,
  primarySport,
  speedMode,
  unitFormatters,
  language,
}: {
  activities: Activity[]
  processedStreams: ProcessedStream[]
  tab: ChartTab
  xMode: XMode
  chartTabs: ChartTabDef[]
  primarySport: string
  speedMode: 'speed' | 'pace'
  unitFormatters: Pick<UnitFormatters, 'options'>
  language: SupportedLanguage
}) {
  const chartRef = useRef<HTMLDivElement>(null)
  const chartInstance = useRef<echarts.ECharts | null>(null)

  const tabDef = chartTabs.find(t => t.key === tab)!

  useEffect(() => {
    if (!chartRef.current) return
    chartInstance.current = echarts.init(chartRef.current, undefined, { renderer: 'canvas' })
    const handleResize = () => chartInstance.current?.resize()
    window.addEventListener('resize', handleResize)
    return () => {
      window.removeEventListener('resize', handleResize)
      chartInstance.current?.dispose()
      chartInstance.current = null
    }
  }, [])

  useEffect(() => {
    if (!chartInstance.current || processedStreams.length === 0) return

    const isDark = document.documentElement.classList.contains('dark')
    const textColor = isDark ? '#a3a3a3' : '#737373'
    const gridColor = isDark ? '#404040' : '#e5e5e5'
    const speedUnit = chartSpeedUnit(primarySport, unitFormatters)

    const series = activities.map((a, i) => {
      const ps = processedStreams[i]
      if (!ps) return null

      const xDataRaw = xMode === 'distance' ? ps.distanceKm : ps.elapsedMin
      const yDataRaw = ps[tabDef.streamKey] as number[]

      if (!hasValidData(yDataRaw)) return null

      const xData = xMode === 'distance'
        ? xDataRaw.map(km => distanceToDisplayValue(km * 1000, unitFormatters.options.units))
        : xDataRaw
      const yData = tab === 'speed'
        ? yDataRaw.map(kmh => chartSpeedDisplayValue(kmh, speedMode, speedUnit, unitFormatters))
        : tab === 'altitude'
          ? yDataRaw.map(meters => elevationToDisplayValue(meters, unitFormatters.options.units))
          : yDataRaw

      const data = downsamplePairs(xData, yData, 1000)

      return {
        name: displayActivityTitle(a, sportLabel(a.sport_type, a.sub_type, { language })),
        type: 'line' as const,
        data,
        showSymbol: false,
        lineStyle: { width: 1.5, color: COMPARE_COLORS[i] },
        itemStyle: { color: COMPARE_COLORS[i] },
        emphasis: { lineStyle: { width: 2 } },
      }
    }).filter(Boolean)

    chartInstance.current.setOption({
      animation: false,
      grid: {
        left: 50,
        right: 20,
        top: 30,
        bottom: 60,
      },
      tooltip: {
        trigger: 'axis',
        backgroundColor: isDark ? '#262626' : '#fff',
        borderColor: isDark ? '#525252' : '#e5e5e5',
        textStyle: { color: isDark ? '#e5e5e5' : '#262626', fontSize: 12 },
        formatter: tab === 'speed'
          ? (params: unknown) => {
              const items = params as { seriesName: string; value: [number, number]; color: string }[]
              if (!items || items.length === 0) return ''
              const xLabel = xMode === 'distance'
                ? `${items[0].value[0].toFixed(2)} ${distanceInputUnit(unitFormatters.options.units)}`
                : `${items[0].value[0].toFixed(1)} min`
              const lines = items.map((item) => {
                const formatted = formatChartSpeedValue(item.value[1], speedMode, speedUnit, unitFormatters)
                return `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${item.color};margin-right:4px;"></span>${item.seriesName}: <b>${formatted}</b>`
              })
              return `${xLabel}<br/>${lines.join('<br/>')}`
            }
          : undefined,
      },
      legend: {
        bottom: 0,
        textStyle: { color: textColor, fontSize: 11 },
      },
      xAxis: {
        type: 'value',
        name: xMode === 'distance' ? distanceInputUnit(unitFormatters.options.units) : 'min',
        nameTextStyle: { color: textColor },
        axisLabel: { color: textColor },
        splitLine: { lineStyle: { color: gridColor } },
        axisLine: { lineStyle: { color: gridColor } },
      },
      yAxis: {
        type: 'value',
        name: tab === 'speed'
          ? speedUnit
          : tab === 'altitude'
            ? elevationInputUnit(unitFormatters.options.units)
            : tabDef.unit,
        nameTextStyle: { color: textColor },
        axisLabel: { color: textColor },
        splitLine: { lineStyle: { color: gridColor } },
      },
      dataZoom: [
        {
          type: 'inside',
          xAxisIndex: 0,
        },
      ],
      series,
    }, true)
  }, [activities, processedStreams, tab, xMode, tabDef, primarySport, speedMode, chartTabs, unitFormatters, language])

  return <div ref={chartRef} className="w-full h-80" />
}

// ── MapLibre Comparison Map ──────────────────────────────────────────────────

function ComparisonMap({
  activities,
  streams,
  noGpsLabel,
  language,
}: {
  activities: Activity[]
  streams: StreamPoint[][]
  noGpsLabel: string
  language: SupportedLanguage
}) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)

  // Filter to only activities that have GPS
  const gpsActivities = useMemo(() => {
    return activities
      .map((a, i) => ({ activity: a, stream: streams[i] }))
      .filter(({ stream }) => stream && stream.some(p => p.lat != null && p.lng != null))
  }, [activities, streams])

  useEffect(() => {
    if (!mapContainerRef.current || gpsActivities.length === 0) return

    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: 'https://tiles.openfreemap.org/styles/liberty',
      center: [0, 0],
      zoom: 2,
      attributionControl: false,
    })
    mapRef.current = map

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      const bounds = new maplibregl.LngLatBounds()

      gpsActivities.forEach(({ activity, stream }, i) => {
        const originalIdx = activities.indexOf(activity)
        const color = COMPARE_COLORS[originalIdx] ?? COMPARE_COLORS[0]

        const coords: [number, number][] = []
        for (const p of stream) {
          if (p.lat != null && p.lng != null) {
            coords.push([p.lng, p.lat])
            bounds.extend([p.lng, p.lat])
          }
        }

        if (coords.length < 2) return

        const sourceId = `track-${i}`
        const layerId = `track-line-${i}`

        map.addSource(sourceId, {
          type: 'geojson',
          data: {
            type: 'Feature',
            properties: {},
            geometry: {
              type: 'LineString',
              coordinates: coords,
            },
          },
        })

        map.addLayer({
          id: layerId,
          type: 'line',
          source: sourceId,
          paint: {
            'line-color': color,
            'line-width': 3,
            'line-opacity': 0.85,
          },
          layout: {
            'line-cap': 'round',
            'line-join': 'round',
          },
        })
      })

      if (!bounds.isEmpty()) {
        map.fitBounds(bounds, { padding: 50, maxZoom: 15 })
      }
    })

    return () => {
      map.remove()
      mapRef.current = null
    }
  }, [gpsActivities, activities])

  if (gpsActivities.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 text-neutral-400 dark:text-neutral-500">
        {noGpsLabel}
      </div>
    )
  }

  return (
    <div className="relative">
      <div ref={mapContainerRef} className="w-full h-96" />
      {/* Legend */}
      <div className="absolute bottom-3 left-3 bg-white/90 dark:bg-neutral-900/90 backdrop-blur-sm rounded-lg px-3 py-2 shadow-lg border border-neutral-200 dark:border-neutral-700 space-y-1">
        {activities.map((a, i) => {
          const hasGps = gpsActivities.some(g => g.activity === a)
          if (!hasGps) return null
          return (
            <div key={a.id} className="flex items-center gap-2 text-xs">
              <div
                className="w-4 h-0.5 rounded-full flex-shrink-0"
                style={{ backgroundColor: COMPARE_COLORS[i] }}
              />
              <span className="text-neutral-700 dark:text-neutral-300 truncate max-w-[200px]">
                {displayActivityTitle(a, sportLabel(a.sport_type, a.sub_type, { language }))}
              </span>
            </div>
          )
        })}
      </div>
    </div>
  )
}
