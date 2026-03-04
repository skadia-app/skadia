import { useState, useRef, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  MarkLineComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchEddington } from '@/api/analytics'
import type { EddingtonResult } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'

echarts.use([BarChart, GridComponent, TooltipComponent, MarkLineComponent, CanvasRenderer])

type MetricKey = 'distance_km' | 'elevation_m'

const METRIC_OPTIONS: { value: MetricKey; label: string; apiValue: string; unit: string }[] = [
  { value: 'distance_km', label: 'Distanz', apiValue: 'distance_km', unit: 'km' },
  { value: 'elevation_m', label: 'Höhenmeter', apiValue: 'elevation_m', unit: 'm' },
]

export default function EddingtonChart() {
  const { toQueryString } = useAnalyticsFilters()
  const [metric, setMetric] = useState<MetricKey>('distance_km')
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  const params = toQueryString()
  const metricOption = METRIC_OPTIONS.find(o => o.value === metric)!

  const { data, isLoading, isError } = useQuery<EddingtonResult>({
    queryKey: ['eddington', metric, params],
    queryFn: () => fetchEddington(`metric=${metricOption.apiValue}&${params}`),
    retry: 1,
  })

  useEffect(() => {
    if (!containerRef.current || !data) return

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    // top_values: sorted descending - take top 50
    const values = (data.top_values ?? []).slice(0, 50)
    // x-axis: rank 1, 2, 3... (day rank)
    const labels = values.map((_, i) => `#${i + 1}`)
    const eddNum = data.number

    const barColors = values.map((_v, i) =>
      i < eddNum ? '#F97316' : '#d1d5db'
    )

    const option: EChartsCoreOption = {
      grid: {
        left: 50,
        right: 16,
        top: 16,
        bottom: 40,
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const items = params as Array<{ name: string; value: number; dataIndex: number }>
          if (!Array.isArray(items) || items.length === 0) return ''
          const item = items[0]
          const rank = item.dataIndex + 1
          return `<div style="font-size:12px">
            <div style="font-weight:600;margin-bottom:4px">Rang ${rank}</div>
            <div>${item.value.toFixed(1)} ${metricOption.unit}</div>
            ${rank <= eddNum ? '<div style="color:#F97316;font-size:11px">Zählt zur Eddington-Zahl</div>' : ''}
          </div>`
        },
      },
      xAxis: {
        type: 'category',
        data: labels,
        axisLabel: {
          fontSize: 10,
          color: '#9ca3af',
          interval: Math.floor(values.length / 10),
        },
      },
      yAxis: {
        type: 'value',
        name: metricOption.unit,
        nameTextStyle: { color: '#9ca3af', fontSize: 10 },
        axisLabel: { fontSize: 10, color: '#9ca3af' },
        splitLine: { lineStyle: { color: '#e5e7eb', type: 'dashed' } },
      },
      series: [
        {
          type: 'bar',
          data: values.map((v, i) => ({
            value: v,
            itemStyle: { color: barColors[i] },
          })),
          barCategoryGap: '20%',
          markLine: eddNum > 0 && eddNum <= values.length ? {
            silent: true,
            symbol: 'none',
            lineStyle: { color: '#ef4444', width: 2, type: 'dashed' },
            label: {
              formatter: `E${eddNum}`,
              fontSize: 11,
              color: '#ef4444',
              position: 'end',
            },
            data: [{ xAxis: eddNum - 0.5 }],
          } : undefined,
        },
      ],
    }

    chart.setOption(option)

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current!)

    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [data, metricOption.unit])

  const nextLevel = data?.next_levels?.[0]

  return (
    <div className="space-y-3">
      {/* Header */}
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">Eddington-Zahl</h3>
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

      {/* Info box */}
      {data && (
        <div className="rounded-lg border border-[var(--color-accent-DEFAULT)]/30 bg-[var(--color-accent-DEFAULT)]/5 dark:bg-[var(--color-accent-DEFAULT)]/10 p-3">
          <div className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
            Eddington-Zahl: <span className="text-[var(--color-accent-DEFAULT)] text-lg">{data.number}</span>
            <span className="text-neutral-500 dark:text-neutral-400 font-normal ml-1">{metricOption.unit}</span>
          </div>
          {nextLevel && (
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
              Fur E{nextLevel.level}: noch{' '}
              <span className="font-semibold text-neutral-700 dark:text-neutral-300">{nextLevel.remaining}</span>{' '}
              {nextLevel.remaining === 1 ? 'Tag' : 'Tage'} mit mindestens {nextLevel.level} {metricOption.unit}
            </div>
          )}
        </div>
      )}

      {/* Chart */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center h-48 text-sm text-neutral-400">
            Lade Eddington-Daten…
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-48 text-sm text-neutral-400">
            Fehler beim Laden
          </div>
        ) : !data || !data.top_values || data.top_values.length === 0 ? (
          <div className="flex items-center justify-center h-48 text-sm text-neutral-400">
            Keine Daten verfugbar
          </div>
        ) : (
          <div ref={containerRef} className="h-48 w-full" />
        )}
      </div>
    </div>
  )
}
