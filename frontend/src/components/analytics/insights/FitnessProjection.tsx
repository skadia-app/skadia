import { useRef, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchInsightsProjection } from '@/api/analytics'
import type { Projection } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'

echarts.use([BarChart, LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

function onTrackBadge(onTrack: boolean): string {
  return onTrack
    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    : 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
}

function metricLabel(metric: string): string {
  const labels: Record<string, string> = {
    ctl: 'CTL (Fitness)',
    distance: 'Distanz',
    elevation: 'Höhenmeter',
    tss: 'TSS',
    duration: 'Dauer',
  }
  return labels[metric.toLowerCase()] ?? metric
}

function ProjectionCard({ projection }: { projection: Projection }) {
  const progress = projection.target > 0 ? Math.min(100, (projection.current / projection.target) * 100) : 0

  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 space-y-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium text-neutral-700 dark:text-neutral-200">
          {metricLabel(projection.metric)}
        </span>
        <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${onTrackBadge(projection.on_track)}`}>
          {projection.on_track ? 'Im Plan' : 'Hinter Plan'}
        </span>
      </div>

      {/* Progress bar */}
      <div className="space-y-1">
        <div className="flex justify-between text-xs text-neutral-500 dark:text-neutral-400">
          <span>Aktuell: <strong className="text-neutral-700 dark:text-neutral-200">{projection.current.toFixed(1)}</strong></span>
          <span>Ziel: <strong className="text-neutral-700 dark:text-neutral-200">{projection.target.toFixed(1)}</strong></span>
        </div>
        <div className="w-full bg-neutral-200 dark:bg-neutral-700 rounded-full h-2">
          <div
            className={`h-2 rounded-full transition-all ${projection.on_track ? 'bg-emerald-500' : 'bg-rose-500'}`}
            style={{ width: `${progress}%` }}
          />
        </div>
      </div>

      <div className="flex items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400">
        {projection.predicted_date && (
          <span>
            Voraussichtlich: <strong className="text-neutral-700 dark:text-neutral-200">{projection.predicted_date}</strong>
          </span>
        )}
        {projection.weekly_needed > 0 && (
          <span>
            Wöchentlich nötig: <strong className="text-neutral-700 dark:text-neutral-200">{projection.weekly_needed.toFixed(1)}</strong>
          </span>
        )}
      </div>

      {projection.explanation && (
        <p className="text-xs text-neutral-400 dark:text-neutral-500 italic">{projection.explanation}</p>
      )}
    </div>
  )
}

export default function FitnessProjection() {
  const { toQueryString } = useAnalyticsFilters()
  const queryParams = toQueryString()
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'insights', 'projection', queryParams],
    queryFn: async () => {
      try {
        return await fetchInsightsProjection(queryParams)
      } catch {
        return null
      }
    },
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  // Build a simple bar chart showing current vs target for each projection
  useEffect(() => {
    if (!containerRef.current) return
    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart
    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)
    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [])

  useEffect(() => {
    if (!chartRef.current || !data || data.length === 0) return

    const labels = data.map(p => metricLabel(p.metric))
    const current = data.map(p => p.current)
    const target = data.map(p => p.target)

    const option: EChartsCoreOption = {
      backgroundColor: 'transparent',
      grid: { left: 80, right: 20, top: 36, bottom: 30 },
      legend: {
        data: ['Aktuell', 'Ziel'],
        top: 4,
        textStyle: { fontSize: 10, color: '#9ca3af' },
        itemHeight: 8,
        itemWidth: 12,
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        backgroundColor: 'rgba(17,24,39,0.92)',
        borderColor: '#374151',
        textStyle: { color: '#f3f4f6', fontSize: 12 },
      },
      xAxis: {
        type: 'value',
        axisLabel: { fontSize: 10, color: '#6b7280' },
        splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
      },
      yAxis: {
        type: 'category',
        data: labels,
        axisLabel: { fontSize: 10, color: '#9ca3af' },
        splitLine: { show: false },
      },
      series: [
        {
          name: 'Aktuell',
          type: 'bar',
          data: current,
          itemStyle: { color: '#3b82f6', borderRadius: [0, 3, 3, 0] },
          barMaxWidth: 20,
        },
        {
          name: 'Ziel',
          type: 'bar',
          data: target,
          itemStyle: { color: '#1f2937', borderRadius: [0, 3, 3, 0] },
          barMaxWidth: 20,
          barGap: '10%',
        },
      ],
    }
    chartRef.current.setOption(option, { notMerge: true })
  }, [data])

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 animate-pulse">
        <div className="h-4 w-40 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
        <div className="h-[180px] bg-neutral-200 dark:bg-neutral-700 rounded" />
      </div>
    )
  }

  if (isError || !data || data.length === 0) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200 mb-3">Ziel-Projektionen</h3>
        <div className="flex flex-col items-center justify-center h-[120px] gap-2 text-neutral-400 text-sm">
          <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
              d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
          </svg>
          <span>Keine Projektionsdaten verfügbar</span>
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200">Ziel-Projektionen</h3>

      {/* Overview chart */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        <div ref={containerRef} className="w-full h-[180px]" />
      </div>

      {/* Detail cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {data.map((projection, i) => (
          <ProjectionCard key={i} projection={projection} />
        ))}
      </div>
    </div>
  )
}
