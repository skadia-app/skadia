import { useState, useRef, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { PieChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchDistributions } from '@/api/analytics'
import type { Distribution } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'

echarts.use([PieChart, BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

type ChartType = 'pie' | 'bar'
type MetricKey = 'count' | 'distance' | 'duration'
type DistType = 'sport' | 'country' | 'gear' | 'distance_bucket'

const DISTRIBUTION_CARDS: { type: DistType; label: string }[] = [
  { type: 'sport', label: 'Sportart' },
  { type: 'country', label: 'Land' },
  { type: 'gear', label: 'Ausrustung' },
  { type: 'distance_bucket', label: 'Distanz-Verteilung' },
]

const METRIC_OPTIONS: { value: MetricKey; label: string; apiValue: string }[] = [
  { value: 'count', label: 'Anzahl', apiValue: 'count' },
  { value: 'distance', label: 'Distanz', apiValue: 'distance_m' },
  { value: 'duration', label: 'Zeit', apiValue: 'duration_s' },
]

const CHART_COLORS = [
  '#F97316', '#3B82F6', '#10B981', '#F59E0B', '#8B5CF6',
  '#EC4899', '#06B6D4', '#84CC16', '#EF4444', '#6366F1',
]

function fmtMetricValue(value: number, metric: MetricKey): string {
  if (metric === 'distance') {
    return `${(value / 1000).toFixed(0)} km`
  }
  if (metric === 'duration') {
    const h = Math.floor(value / 3600)
    return `${h} h`
  }
  return new Intl.NumberFormat('de-DE').format(Math.round(value))
}

interface DistChartProps {
  type: DistType
  label: string
  metric: MetricKey
  params: string
  onLabelClick: (label: string, type: DistType) => void
}

function DistChartCard({ type, label, metric, params, onLabelClick }: DistChartProps) {
  const [chartType, setChartType] = useState<ChartType>('pie')
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  const metricOption = METRIC_OPTIONS.find(o => o.value === metric)!

  const { data, isLoading, isError } = useQuery<Distribution[]>({
    queryKey: ['distributions', type, metric, params],
    queryFn: () => fetchDistributions(`type=${type}&metric=${metricOption.apiValue}&${params}`),
    retry: 1,
  })

  useEffect(() => {
    if (!containerRef.current || !data || data.length === 0) return

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    const topData = data.slice(0, 10)

    let option: EChartsCoreOption

    if (chartType === 'pie') {
      option = {
        tooltip: {
          trigger: 'item',
          formatter: (params: unknown) => {
            const p = params as { name: string; value: number; percent: number }
            return `<div style="font-size:12px">
              <div style="font-weight:600">${p.name}</div>
              <div>${fmtMetricValue(p.value, metric)} (${p.percent?.toFixed(1)}%)</div>
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
              name: d.label,
              value: d.value,
              itemStyle: { color: CHART_COLORS[i % CHART_COLORS.length] },
            })),
            label: { show: false },
            emphasis: {
              itemStyle: { shadowBlur: 8, shadowOffsetX: 0, shadowColor: 'rgba(0,0,0,0.3)' },
            },
          },
        ],
      }
    } else {
      option = {
        grid: { left: 80, right: 16, top: 8, bottom: 24 },
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'shadow' },
          formatter: (params: unknown) => {
            const items = params as Array<{ name: string; value: number }>
            if (!Array.isArray(items) || items.length === 0) return ''
            const item = items[0]
            return `<div style="font-size:12px">
              <div style="font-weight:600">${item.name}</div>
              <div>${fmtMetricValue(item.value, metric)}</div>
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
          data: [...topData].reverse().map(d => d.label),
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
              itemStyle: { color: CHART_COLORS[i % CHART_COLORS.length] },
            })),
            barCategoryGap: '30%',
          },
        ],
      }
    }

    chart.setOption(option)

    chart.on('click', (params: unknown) => {
      const p = params as { name?: string }
      if (p.name) {
        onLabelClick(p.name, type)
      }
    })

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current!)

    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [data, chartType, metric, type, onLabelClick])

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-semibold text-neutral-700 dark:text-neutral-300">{label}</span>
        <button
          onClick={() => setChartType(ct => ct === 'pie' ? 'bar' : 'pie')}
          className="text-xs px-2 py-0.5 rounded border border-neutral-200 dark:border-neutral-700
                     text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800
                     transition-colors"
          title={chartType === 'pie' ? 'Als Balkendiagramm' : 'Als Kreisdiagramm'}
        >
          {chartType === 'pie' ? 'Balken' : 'Kreis'}
        </button>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center h-40 text-sm text-neutral-400">Lade…</div>
      ) : isError ? (
        <div className="flex items-center justify-center h-40 text-sm text-neutral-400">Fehler</div>
      ) : !data || data.length === 0 ? (
        <div className="flex items-center justify-center h-40 text-sm text-neutral-400">Keine Daten</div>
      ) : (
        <div ref={containerRef} className="h-40 w-full" />
      )}
    </div>
  )
}

export default function DistributionCharts() {
  const { toQueryString, setFilters } = useAnalyticsFilters()
  const [metric, setMetric] = useState<MetricKey>('count')
  const params = toQueryString()

  const handleLabelClick = (label: string, type: DistType) => {
    if (type === 'sport') {
      setFilters({ sportTypes: [label] })
    } else if (type === 'country') {
      setFilters({ countries: [label] })
    } else if (type === 'gear') {
      // gear filter by name - use gearId if we had the ID, skip for now
    }
  }

  return (
    <div className="space-y-3">
      {/* Global metric toggle */}
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">Verteilungen</h3>
        <div className="flex rounded-md overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
          {METRIC_OPTIONS.map(opt => (
            <button
              key={opt.value}
              onClick={() => setMetric(opt.value)}
              className={metric === opt.value
                ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
              }
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {DISTRIBUTION_CARDS.map(card => (
          <DistChartCard
            key={card.type}
            type={card.type}
            label={card.label}
            metric={metric}
            params={params}
            onLabelClick={handleLabelClick}
          />
        ))}
      </div>

      <p className="text-xs text-neutral-400 text-center">
        Klick auf ein Segment filtert die Analytics-Ansicht
      </p>
    </div>
  )
}
