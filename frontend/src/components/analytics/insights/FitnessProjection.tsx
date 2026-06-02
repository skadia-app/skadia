import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import * as echarts from 'echarts/core'
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
import { useECharts } from '@/hooks/useECharts'
import FilterActiveBadge from '@/components/analytics/filters/FilterActiveBadge'

echarts.use([BarChart, LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

function onTrackBadge(onTrack: boolean): string {
  return onTrack
    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    : 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
}

const PROJECTION_METRIC_LABEL_KEYS: Record<string, string> = {
  ctl: 'analyticsHealthInsights.factorNames.ctl',
  distance: 'analyticsHealthInsights.factorNames.distance',
  elevation: 'analyticsHealthInsights.factorNames.elevation',
  tss: 'analyticsHealthInsights.factorNames.tss',
  duration: 'analyticsHealthInsights.factorNames.duration',
}

export function formatProjectionMetricLabel(metric: string, t: (key: string) => string): string {
  const labelKey = PROJECTION_METRIC_LABEL_KEYS[metric.toLowerCase()]
  return t(labelKey ?? 'common.unknown')
}

function ProjectionCard({ projection }: { projection: Projection }) {
  const { t } = useTranslation()
  const progress = projection.target > 0 ? Math.min(100, (projection.current / projection.target) * 100) : 0

  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 space-y-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium text-neutral-700 dark:text-neutral-200">
          {formatProjectionMetricLabel(projection.metric, t)}
        </span>
        <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${onTrackBadge(projection.on_track)}`}>
          {projection.on_track
            ? t('analyticsHealthInsights.projection.onTrack')
            : t('analyticsHealthInsights.projection.behindPlan')}
        </span>
      </div>

      {/* Progress bar */}
      <div className="space-y-1">
        <div className="flex justify-between text-xs text-neutral-500 dark:text-neutral-400">
          <span>{t('analyticsHealthInsights.projection.current')}: <strong className="text-neutral-700 dark:text-neutral-200">{projection.current.toFixed(1)}</strong></span>
          <span>{t('analyticsHealthInsights.projection.target')}: <strong className="text-neutral-700 dark:text-neutral-200">{projection.target.toFixed(1)}</strong></span>
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
            {t('analyticsHealthInsights.projection.expected')}: <strong className="text-neutral-700 dark:text-neutral-200">{projection.predicted_date}</strong>
          </span>
        )}
        {projection.weekly_needed > 0 && (
          <span>
            {t('analyticsHealthInsights.projection.weeklyNeeded')}: <strong className="text-neutral-700 dark:text-neutral-200">{projection.weekly_needed.toFixed(1)}</strong>
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
  const { t } = useTranslation()
  const { toQueryString } = useAnalyticsFilters()
  const queryParams = toQueryString()
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

  const containerRef = useECharts(
    () => {
      if (!data || !Array.isArray(data) || data.length === 0) return { series: [] }

      const labels = data.map(p => formatProjectionMetricLabel(p.metric, t))
      const current = data.map(p => p.current)
      const target = data.map(p => p.target)

      return {
        backgroundColor: 'transparent',
        grid: { left: 80, right: 20, top: 36, bottom: 30 },
        legend: {
          data: [t('analyticsHealthInsights.projection.current'), t('analyticsHealthInsights.projection.target')],
          top: 4,
          textStyle: { fontSize: 10, color: '#9ca3af' },
          itemHeight: 8,
          itemWidth: 12,
        },
        tooltip: {
          confine: true,
          trigger: 'axis',
          axisPointer: { type: 'cross' },
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
            name: t('analyticsHealthInsights.projection.current'),
            type: 'bar',
            data: current,
            itemStyle: { color: '#3b82f6', borderRadius: [0, 3, 3, 0] },
            barMaxWidth: 20,
          },
          {
            name: t('analyticsHealthInsights.projection.target'),
            type: 'bar',
            data: target,
            itemStyle: { color: '#1f2937', borderRadius: [0, 3, 3, 0] },
            barMaxWidth: 20,
            barGap: '10%',
          },
        ],
      }
    },
    [data, t],
  )

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 animate-pulse">
        <div className="h-4 w-40 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
        <div className="h-[180px] bg-neutral-200 dark:bg-neutral-700 rounded" />
      </div>
    )
  }

  if (isError || !data || !Array.isArray(data) || data.length === 0) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <div className="flex items-center gap-2 mb-3">
          <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200">{t('analyticsHealthInsights.projection.title')}</h3>
          <FilterActiveBadge />
        </div>
        <div className="flex flex-col items-center justify-center h-[120px] gap-2 text-neutral-400 text-sm">
          <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
              d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
          </svg>
          <span>{t('analyticsHealthInsights.projection.noData')}</span>
        </div>
      </div>
    )
  }

  const projections = Array.isArray(data) ? data : []

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
      <div className="flex items-center gap-2">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200">{t('analyticsHealthInsights.projection.title')}</h3>
        <FilterActiveBadge />
      </div>

      {/* Overview chart */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        <div ref={containerRef} className="w-full h-[180px]" />
      </div>

      {/* Detail cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {projections.map((projection, i) => (
          <ProjectionCard key={i} projection={projection} />
        ))}
      </div>
    </div>
  )
}
