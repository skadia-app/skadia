import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { fmtDateTime } from '@/lib/format'
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
import { formatDuration, formatDistance } from '@/lib/utils'
import { sportLabel } from '@/lib/sportTaxonomy'
import { formatSpeedKmh } from '@/lib/sportFieldLabels'

import { SportIcon } from '@/components/SportIcon'

echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  CanvasRenderer,
])

// ── Colors for up to 3 activities ────────────────────────────────────────────

const COMPARE_COLORS = ['#3b82f6', '#ef4444', '#22c55e']

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

function formatHr(v: number | null | undefined): string {
  if (v == null || isNaN(v)) return '\u2014'
  return `${Math.round(v)} bpm`
}

function formatPower(v: number | null | undefined): string {
  if (v == null || isNaN(v)) return '\u2014'
  return `${Math.round(v)} W`
}

function formatElevation(v: number | null | undefined): string {
  if (v == null || isNaN(v) || v <= 0) return '\u2014'
  return `${Math.round(v)} m`
}

function formatCalories(v: number | null | undefined): string {
  if (v == null || isNaN(v)) return '\u2014'
  return `${Math.round(v)} kcal`
}

// ── Metric row definitions ───────────────────────────────────────────────────

interface MetricRow {
  label: string
  getValue: (a: Activity) => string
  getRawValue: (a: Activity) => number | null | undefined
  higherIsBetter?: boolean // defaults to false (lower is better for time/speed)
}

const METRIC_ROWS: MetricRow[] = [
  {
    label: 'Distanz',
    getValue: a => formatDistance(a.distance_m),
    getRawValue: a => a.distance_m,
    higherIsBetter: true,
  },
  {
    label: 'Dauer',
    getValue: a => formatDuration(a.elapsed_time_s ?? 0),
    getRawValue: a => a.elapsed_time_s,
    higherIsBetter: false,
  },
  {
    label: 'Höhenmeter',
    getValue: a => formatElevation(a.elevation_gain_m),
    getRawValue: a => a.elevation_gain_m,
    higherIsBetter: true,
  },
  {
    label: 'Geschwindigkeit',
    getValue: a => formatSpeedKmh(a.avg_speed_ms ?? 0),
    getRawValue: a => a.avg_speed_ms,
    higherIsBetter: true,
  },
  {
    label: 'Herzfrequenz',
    getValue: a => formatHr(a.avg_heart_rate),
    getRawValue: a => a.avg_heart_rate,
  },
  {
    label: 'Max. HF',
    getValue: a => formatHr(a.max_heart_rate),
    getRawValue: a => a.max_heart_rate,
  },
  {
    label: 'Leistung',
    getValue: a => formatPower(a.avg_power_w),
    getRawValue: a => a.avg_power_w,
    higherIsBetter: true,
  },
  {
    label: 'NP',
    getValue: a => formatPower(a.normalized_power_w),
    getRawValue: a => a.normalized_power_w,
    higherIsBetter: true,
  },
  {
    label: 'TSS',
    getValue: a => a.tss != null ? `${Math.round(a.tss)}` : '\u2014',
    getRawValue: a => a.tss,
  },
  {
    label: 'Kalorien',
    getValue: a => formatCalories(a.total_calories),
    getRawValue: a => a.total_calories,
    higherIsBetter: true,
  },
]

// ── Chart Tab definitions ────────────────────────────────────────────────────

const CHART_TABS: { key: ChartTab; label: string; unit: string; streamKey: keyof ProcessedStream }[] = [
  { key: 'speed', label: 'Geschwindigkeit', unit: 'km/h', streamKey: 'speed' },
  { key: 'heartRate', label: 'Herzfrequenz', unit: 'bpm', streamKey: 'heartRate' },
  { key: 'altitude', label: 'Höhe', unit: 'm', streamKey: 'altitude' },
  { key: 'power', label: 'Leistung', unit: 'W', streamKey: 'power' },
]

// ── Main Component ───────────────────────────────────────────────────────────

export default function ActivityComparisonPage() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const idsParam = searchParams.get('ids') ?? ''
  const ids = useMemo(() => idsParam.split(',').filter(Boolean).slice(0, 3), [idsParam])

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

  const isLoading = activityQueries.some(q => q.isLoading) || streamQueries.some(q => q.isLoading)
  const hasError = activityQueries.some(q => q.isError) || streamQueries.some(q => q.isError)

  if (ids.length < 2) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <p className="text-neutral-500 dark:text-neutral-400">
          Mindestens 2 Aktivitäten zum Vergleichen auswahlen.
        </p>
        <button
          onClick={() => navigate('/activities')}
          className="px-4 py-2 text-sm rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white font-medium cursor-pointer"
        >
          Zur Aktivitätenliste
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
        <p className="text-red-500">Fehler beim Laden der Aktivitäten.</p>
        <button
          onClick={() => navigate('/activities')}
          className="px-4 py-2 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer"
        >
          Zuruck
        </button>
      </div>
    )
  }

  // Find best values for highlighting
  function findBestIdx(row: MetricRow): number | null {
    const values = activities.map(a => row.getRawValue(a))
    const validIdxs = values.map((v, i) => ({ v, i })).filter(({ v }) => v != null && !isNaN(v as number))
    if (validIdxs.length < 2) return null
    const higherIsBetter = row.higherIsBetter ?? false
    return validIdxs.reduce((best, curr) =>
      higherIsBetter
        ? ((curr.v as number) > (best.v as number) ? curr : best)
        : ((curr.v as number) < (best.v as number) ? curr : best),
    ).i
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
          Aktivitäten vergleichen
        </h1>
        <span className="text-sm text-neutral-400">{activities.length} Aktivitäten</span>
      </div>

      {/* Section A: Metrics Table */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50">
              <th className="text-left px-4 py-3 font-medium text-neutral-500 dark:text-neutral-400 w-36">
                Metrik
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
                        {a.title ?? a.title_auto ?? sportLabel(a.sport_type, a.sub_type)}
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
            {METRIC_ROWS.map(row => {
              const bestIdx = findBestIdx(row)
              return (
                <tr key={row.label}>
                  <td className="px-4 py-2.5 text-neutral-500 dark:text-neutral-400 font-medium">
                    {row.label}
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
            {CHART_TABS.map(tab => (
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
              Nach Distanz
            </button>
            <button
              onClick={() => setXMode('time')}
              className={`px-3 py-1.5 cursor-pointer transition-colors ${
                xMode === 'time'
                  ? 'bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                  : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
              }`}
            >
              Nach Zeit
            </button>
          </div>
        </div>
        <ComparisonChart
          activities={activities}
          processedStreams={processedStreams}
          tab={chartTab}
          xMode={xMode}
        />
      </div>

      {/* Section C: Map Overlay */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <ComparisonMap
          activities={activities}
          streams={streams}
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
}: {
  activities: Activity[]
  processedStreams: ProcessedStream[]
  tab: ChartTab
  xMode: XMode
}) {
  const chartRef = useRef<HTMLDivElement>(null)
  const chartInstance = useRef<echarts.ECharts | null>(null)

  const tabDef = CHART_TABS.find(t => t.key === tab)!

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

    const series = activities.map((a, i) => {
      const ps = processedStreams[i]
      if (!ps) return null

      const xData = xMode === 'distance' ? ps.distanceKm : ps.elapsedMin
      const yData = ps[tabDef.streamKey] as number[]

      if (!hasValidData(yData)) return null

      const data = xData.map((x, j) => [x, yData[j]])

      return {
        name: a.title ?? a.title_auto ?? sportLabel(a.sport_type, a.sub_type),
        type: 'line' as const,
        data,
        showSymbol: false,
        lineStyle: { width: 1.5, color: COMPARE_COLORS[i] },
        itemStyle: { color: COMPARE_COLORS[i] },
        emphasis: { lineStyle: { width: 2 } },
        sampling: 'lttb' as const,
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
      },
      legend: {
        bottom: 0,
        textStyle: { color: textColor, fontSize: 11 },
      },
      xAxis: {
        type: 'value',
        name: xMode === 'distance' ? 'km' : 'min',
        nameTextStyle: { color: textColor },
        axisLabel: { color: textColor },
        splitLine: { lineStyle: { color: gridColor } },
        axisLine: { lineStyle: { color: gridColor } },
      },
      yAxis: {
        type: 'value',
        name: tabDef.unit,
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
  }, [activities, processedStreams, tab, xMode, tabDef])

  return <div ref={chartRef} className="w-full h-80" />
}

// ── MapLibre Comparison Map ──────────────────────────────────────────────────

function ComparisonMap({
  activities,
  streams,
}: {
  activities: Activity[]
  streams: StreamPoint[][]
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
        Keine GPS-Daten vorhanden.
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
                {a.title ?? a.title_auto ?? sportLabel(a.sport_type, a.sub_type)}
              </span>
            </div>
          )
        })}
      </div>
    </div>
  )
}
