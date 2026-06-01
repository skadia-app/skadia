import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { PieChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import * as echarts from 'echarts/core'
import { fetchDistributions } from '@/api/analytics'
import type { Distribution } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { useECharts } from '@/hooks/useECharts'
import { sportLabel } from '@/lib/sportTaxonomy'
import { cn } from '@/lib/utils'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { formatDistance, type UnitFormatOptions } from '@/lib/units'

echarts.use([PieChart, BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

type ChartType = 'pie' | 'bar'
type MetricKey = 'count' | 'distance' | 'duration'
type DistType = 'sport' | 'country' | 'gear' | 'distance_bucket'

const DISTRIBUTION_CARDS: { type: DistType; labelKey: string }[] = [
  { type: 'sport', labelKey: 'analyticsTrainingStats.distributions.cards.sport' },
  { type: 'country', labelKey: 'analyticsTrainingStats.distributions.cards.country' },
  { type: 'gear', labelKey: 'analyticsTrainingStats.distributions.cards.gear' },
  { type: 'distance_bucket', labelKey: 'analyticsTrainingStats.distributions.cards.distanceBucket' },
]

const METRIC_OPTIONS: { value: MetricKey; labelKey: string; apiValue: string }[] = [
  { value: 'count', labelKey: 'analyticsTrainingStats.metrics.count', apiValue: 'count' },
  { value: 'distance', labelKey: 'analyticsTrainingStats.metrics.distance', apiValue: 'distance_m' },
  { value: 'duration', labelKey: 'analyticsTrainingStats.metrics.time', apiValue: 'moving_time_s' },
]

const CHART_COLORS = [
  '#F97316', '#3B82F6', '#10B981', '#F59E0B', '#8B5CF6',
  '#EC4899', '#06B6D4', '#84CC16', '#EF4444', '#6366F1',
]

// Backend returns raw English keys for label fields (sport_type column,
// 'Unknown', 'No Gear', 'No distance' placeholders). Map them to the German
// display strings the rest of the UI uses.
const PLACEHOLDER_LABEL_KEYS: Record<string, string> = {
  Unknown: 'analyticsTrainingStats.placeholders.unknown',
  unknown: 'analyticsTrainingStats.placeholders.unknown',
  'No Gear': 'analyticsTrainingStats.placeholders.noGear',
  'No distance': 'analyticsTrainingStats.placeholders.noDistance',
}

function displayLabel(raw: string, type: DistType, t: ReturnType<typeof useTranslation>['t']): string {
  if (type === 'sport') return sportLabel(raw)
  const key = PLACEHOLDER_LABEL_KEYS[raw]
  return key ? t(key) : raw
}

function fmtMetricValue(value: number, metric: MetricKey, options: UnitFormatOptions, t: ReturnType<typeof useTranslation>['t']): string {
  if (metric === 'distance') {
    return formatDistance(value, options)
  }
  if (metric === 'duration') {
    const h = Math.floor(value / 3600)
    if (h >= 1) {
      return t('analyticsTrainingStats.distributions.durationHours', {
        value: new Intl.NumberFormat(options.locale, { maximumFractionDigits: 0 }).format(h),
      })
    }
    const minutes = Math.round(value / 60)
    return t('analyticsTrainingStats.distributions.durationMinutes', {
      value: new Intl.NumberFormat(options.locale, { maximumFractionDigits: 0 }).format(minutes),
    })
  }
  return new Intl.NumberFormat(options.locale).format(Math.round(value))
}

interface DistChartProps {
  type: DistType
  label: string
  metric: MetricKey
  params: string
  onLabelClick: (label: string, type: DistType) => void
}

function DistChartCard({ type, label, metric, params, onLabelClick }: DistChartProps) {
  const { t } = useTranslation()
  const { options: unitOptions } = useUnitFormatters()
  const [chartType, setChartType] = useState<ChartType>('pie')
  const percentFormatter = new Intl.NumberFormat(unitOptions.locale, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  })

  const metricOption = METRIC_OPTIONS.find(o => o.value === metric)!

  const { data, isLoading, isError } = useQuery<Distribution[]>({
    queryKey: ['distributions', type, metric, params],
    queryFn: () => fetchDistributions(`type=${type}&metric=${metricOption.apiValue}&${params}`),
    retry: 1,
  })

  const containerRef = useECharts(
    () => {
      if (!data || data.length === 0) return {}

      const topData = data.slice(0, 10)

      if (chartType === 'pie') {
        return {
          tooltip: {
            confine: true,
            trigger: 'item',
            formatter: (params: unknown) => {
              const p = params as { name: string; value: number; percent: number }
              return `<div style="font-size:12px">
                <div style="font-weight:600">${p.name}</div>
                <div>${fmtMetricValue(p.value, metric, unitOptions, t)} (${percentFormatter.format(p.percent ?? 0)}%)</div>
              </div>`
            },
          },
          legend: {
            orient: 'vertical',
            right: 4,
            top: 'center',
            textStyle: { fontSize: 10, color: '#9ca3af' },
            itemWidth: 10,
            itemHeight: 10,
            formatter: (name: string) => name.length > 12 ? name.slice(0, 12) + '…' : name,
          },
          series: [
            {
              type: 'pie',
              radius: ['40%', '70%'],
              center: ['38%', '50%'],
              data: topData.map((d, i) => ({
                name: displayLabel(d.label, type, t),
                value: d.value,
                // sport_type carries the canonical backend token; the click handler
                // reads this field so the filter receives the raw key, never the
                // localized display label.
                sport_type: d.label,
                itemStyle: { color: CHART_COLORS[i % CHART_COLORS.length] },
              })),
              label: { show: false },
              emphasis: {
                itemStyle: { shadowBlur: 8, shadowOffsetX: 0, shadowColor: 'rgba(0,0,0,0.3)' },
              },
            },
          ],
        }
      }

      return {
        grid: { left: 80, right: 16, top: 8, bottom: 24 },
        tooltip: {
          confine: true,
          trigger: 'axis',
          axisPointer: { type: 'cross' },
          formatter: (params: unknown) => {
            const items = params as Array<{ name: string; value: number }>
            if (!Array.isArray(items) || items.length === 0) return ''
            const item = items[0]
            return `<div style="font-size:12px">
              <div style="font-weight:600">${item.name}</div>
              <div>${fmtMetricValue(item.value, metric, unitOptions, t)}</div>
            </div>`
          },
        },
        xAxis: {
          type: 'value',
          axisLabel: { fontSize: 9, color: '#9ca3af' },
          splitLine: { lineStyle: { color: '#e5e7eb', type: 'dashed' } },
        },
        yAxis: {
          type: 'category',
          data: [...topData].reverse().map(d => displayLabel(d.label, type, t)),
          axisLabel: {
            fontSize: 10,
            color: '#9ca3af',
            formatter: (name: string) => name.length > 12 ? name.slice(0, 12) + '…' : name,
          },
        },
        series: [
          {
            type: 'bar',
            data: [...topData].reverse().map((d, i) => ({
              value: d.value,
              // Canonical raw key — read by the click handler so the filter
              // receives the backend token, not the localized display label.
              sport_type: d.label,
              itemStyle: { color: CHART_COLORS[i % CHART_COLORS.length] },
            })),
            barCategoryGap: '30%',
          },
        ],
      }
    },
    [data, chartType, metric, unitOptions, percentFormatter, t],
    (chart) => {
      const handler = (params: unknown) => {
        const p = params as { name?: string; data?: { sport_type?: string } }
        if (p.name) onLabelClick(p.data?.sport_type ?? p.name, type)
      }
      chart.on('click', handler)
      return () => chart.off('click', handler)
    },
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-semibold text-neutral-700 dark:text-neutral-300">{label}</span>
        <button
          onClick={() => setChartType(ct => ct === 'pie' ? 'bar' : 'pie')}
          className="text-xs px-2 py-0.5 rounded border border-neutral-200 dark:border-neutral-700
                     text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800
                     cursor-pointer transition-colors"
          title={chartType === 'pie' ? t('analyticsTrainingStats.chartView.asBarChart') : t('analyticsTrainingStats.chartView.asPieChart')}
        >
          {chartType === 'pie' ? t('analyticsTrainingStats.chartView.bar') : t('analyticsTrainingStats.chartView.pie')}
        </button>
      </div>

      <div className="relative h-40">
        {isLoading && (
          <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">{t('common.loading')}</div>
        )}
        {!isLoading && isError && (
          <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">{t('common.error')}</div>
        )}
        {!isLoading && !isError && (!data || data.length === 0) && (
          <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">{t('analyticsTrainingStats.states.noDataShort')}</div>
        )}
        <div ref={containerRef} className={cn("w-full h-full", { invisible: isLoading || isError || !data || data.length === 0 })} />
      </div>
    </div>
  )
}

export default function DistributionCharts() {
  const { t } = useTranslation()
  const { toQueryString, setFilters } = useAnalyticsFilters()
  const [metric, setMetric] = useState<MetricKey>('count')
  const params = toQueryString()

  const handleLabelClick = (label: string, type: DistType) => {
    if (type === 'sport') {
      if (label.toLowerCase() === 'unknown') return
      setFilters({ sportTypes: [label] })
    } else if (type === 'country') {
      setFilters({ countries: [label] })
    }
  }

  return (
    <div className="space-y-3">
      {/* Global metric toggle */}
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">{t('analyticsTrainingStats.distributions.title')}</h3>
        <div className="flex rounded-md overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
          {METRIC_OPTIONS.map(opt => (
            <button
              key={opt.value}
              onClick={() => setMetric(opt.value)}
              className={metric === opt.value
                ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'
              }
            >
              {t(opt.labelKey)}
            </button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {DISTRIBUTION_CARDS.map(card => (
          <DistChartCard
            key={card.type}
            type={card.type}
            label={t(card.labelKey)}
            metric={metric}
            params={params}
            onLabelClick={handleLabelClick}
          />
        ))}
      </div>

      <p className="text-xs text-neutral-400 text-center">
        {t('analyticsTrainingStats.distributions.clickHint')}
      </p>
    </div>
  )
}
