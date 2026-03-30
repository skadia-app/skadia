import { useState, useRef, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  ToolboxComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchYearProgression } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'

echarts.use([
  LineChart,
  BarChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  ToolboxComponent,
  CanvasRenderer,
])

type MetricKey = 'distance_m' | 'elevation_gain_m' | 'moving_time_s' | 'tss' | 'total_calories'

const METRIC_OPTIONS: { value: MetricKey; label: string; unit: string; factor: number; decimals: number }[] = [
  { value: 'distance_m', label: 'Distanz', unit: 'km', factor: 0.001, decimals: 1 },
  { value: 'elevation_gain_m', label: 'Höhenmeter', unit: 'm', factor: 1, decimals: 0 },
  { value: 'moving_time_s', label: 'Bewegungszeit', unit: 'h', factor: 1 / 3600, decimals: 1 },
  { value: 'tss', label: 'TSS', unit: 'TSS', factor: 1, decimals: 0 },
  { value: 'total_calories', label: 'Kalorien', unit: 'kcal', factor: 1, decimals: 0 },
]

const YEAR_COLORS = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#ec4899',
  '#06b6d4',
  '#84cc16',
]

const MONTH_LABELS = ['Jan', 'Feb', 'Mär', 'Apr', 'Mai', 'Jun', 'Jul', 'Aug', 'Sep', 'Okt', 'Nov', 'Dez']

// Group ID for synchronized cross-chart tooltips
const CONNECT_GROUP = 'year-progression'

export default function YearProgressionChart() {
  const { toQueryString } = useAnalyticsFilters()
  const [metric, setMetric] = useState<MetricKey>('distance_m')
  const [selectedYears, setSelectedYears] = useState<Set<number>>(new Set())
  const [chartType, setChartType] = useState<'line' | 'bar'>('line')

  const monthlyRef = useRef<HTMLDivElement>(null)
  const cumulRef = useRef<HTMLDivElement>(null)
  const monthlyChart = useRef<echarts.ECharts | null>(null)
  const cumulChart = useRef<echarts.ECharts | null>(null)

  const queryParams = useMemo(() => {
    const base = toQueryString()
    const params = new URLSearchParams(base)
    params.set('metric', metric)
    return params.toString()
  }, [toQueryString, metric])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'year-progression', queryParams],
    queryFn: () => fetchYearProgression(queryParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  // Initialize selected years when data arrives — default: last 3 years
  useEffect(() => {
    if (!data?.years?.length) return
    const yearNumbers = data.years.map(y => y.year)
    const last3 = yearNumbers.slice(-3)
    setSelectedYears(new Set(last3))
  }, [data?.years?.map(y => y.year).join(',')])

  // Initialize charts when container divs become available (data-driven)
  useEffect(() => {
    if (!monthlyRef.current || !cumulRef.current) return
    if (monthlyChart.current) monthlyChart.current.dispose()
    if (cumulChart.current) cumulChart.current.dispose()

    const mc = echarts.init(monthlyRef.current, undefined, { renderer: 'canvas' })
    const cc = echarts.init(cumulRef.current, undefined, { renderer: 'canvas' })
    monthlyChart.current = mc
    cumulChart.current = cc

    echarts.connect(CONNECT_GROUP)
    mc.group = CONNECT_GROUP
    cc.group = CONNECT_GROUP

    const obs = new ResizeObserver(() => { mc.resize(); cc.resize() })
    if (monthlyRef.current) obs.observe(monthlyRef.current)
    if (cumulRef.current) obs.observe(cumulRef.current)

    return () => {
      obs.disconnect()
      mc.dispose()
      cc.dispose()
      monthlyChart.current = null
      cumulChart.current = null
    }
  }, [data])

  const metricMeta = METRIC_OPTIONS.find(m => m.value === metric)!

  // Update charts when data or selection changes
  useEffect(() => {
    if (!monthlyChart.current || !cumulChart.current || !data?.years?.length) return

    const selectedYearData = data.years.filter(y => selectedYears.has(y.year))

    const tooltipFormatter = (params: unknown) => {
      const items = params as Array<{ seriesName: string; value: number | null; axisValueLabel: string; color: string }>
      if (!Array.isArray(items) || items.length === 0) return ''
      let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>`
      for (const p of items) {
        if (p.value == null) continue
        const v = (p.value * metricMeta.factor).toFixed(metricMeta.decimals)
        html += `<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">
          <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>
          <span style="color:#d1d5db">${p.seriesName}:</span>
          <strong style="color:#f9fafb">${v} ${metricMeta.unit}</strong>
        </div>`
      }
      return html
    }

    const sharedConfig = {
      backgroundColor: 'transparent',
      grid: { left: 52, right: 20, top: 36, bottom: 40 },
      toolbox: {
        right: 8,
        top: 2,
        feature: {
          dataZoom: {
            yAxisIndex: 'none',
            title: { zoom: 'Bereich zoomen (ziehen)', back: 'Zoom zurücksetzen' },
            brushStyle: {
              borderWidth: 1,
              color: 'rgba(59,130,246,0.1)',
              borderColor: 'rgba(59,130,246,0.6)',
            },
          },
          restore: { title: 'Zurücksetzen' },
          brush: { show: false },
          saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2 },
        },
        iconStyle: { borderColor: '#6b7280' },
        emphasis: { iconStyle: { borderColor: '#d1d5db' } },
      },
      dataZoom: [
        { type: 'inside', xAxisIndex: 0, filterMode: 'none' },
      ],
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(17,24,39,0.92)',
        borderColor: '#374151',
        textStyle: { color: '#f3f4f6', fontSize: 12 },
        formatter: tooltipFormatter,
      },
      xAxis: {
        type: 'category',
        data: MONTH_LABELS,
        axisLabel: { fontSize: 10, color: '#6b7280' },
        axisLine: { lineStyle: { color: '#374151' } },
        splitLine: { show: false },
      },
      yAxis: {
        type: 'value',
        axisLabel: {
          fontSize: 10,
          color: '#6b7280',
          formatter: (v: number) => {
            const converted = v * metricMeta.factor
            return converted >= 1000 ? `${(converted / 1000).toFixed(0)}k` : converted.toFixed(0)
          },
        },
        splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
      },
      legend: {
        data: selectedYearData.map(y => String(y.year)),
        top: 4,
        textStyle: { fontSize: 11, color: '#9ca3af' },
      },
    }

    // Monthly chart
    const monthlySeries = selectedYearData.map((yearData, idx) => {
      // monthly is a 12-element array; replace 0 with null for missing months
      const monthValues = yearData.monthly.map(v => (v > 0 ? v : null))
      const color = YEAR_COLORS[idx % YEAR_COLORS.length]
      return chartType === 'line'
        ? {
            name: String(yearData.year),
            type: 'line' as const,
            data: monthValues,
            showSymbol: false,
            lineStyle: { width: 2, color },
            itemStyle: { color },
            connectNulls: false,
          }
        : {
            name: String(yearData.year),
            type: 'bar' as const,
            data: monthValues,
            itemStyle: { color },
          }
    })

    const monthlyOption: EChartsCoreOption = {
      ...sharedConfig,
      series: monthlySeries,
    }

    // Cumulative chart
    const cumulSeries = selectedYearData.map((yearData, idx) => {
      const cumValues = yearData.cumulative.map(v => (v > 0 ? v : null))
      const color = YEAR_COLORS[idx % YEAR_COLORS.length]
      return {
        name: String(yearData.year),
        type: 'line' as const,
        data: cumValues,
        showSymbol: false,
        lineStyle: { width: 2, color },
        itemStyle: { color },
        connectNulls: false,
      }
    })

    const cumulOption: EChartsCoreOption = {
      ...sharedConfig,
      series: cumulSeries,
    }

    monthlyChart.current.setOption(monthlyOption, { notMerge: true })
    cumulChart.current.setOption(cumulOption, { notMerge: true })
  }, [data, selectedYears, chartType, metric])

  const toggleYear = (year: number) => {
    setSelectedYears(prev => {
      const next = new Set(prev)
      if (next.has(year)) {
        if (next.size > 1) next.delete(year)
      } else {
        next.add(year)
      }
      return next
    })
  }

  const yearNumbers = data?.years?.map(y => y.year) ?? []
  const bestYear = data?.best_year
  const bestYearData = data?.years?.find(y => y.year === bestYear)
  const bestConverted = bestYearData
    ? (bestYearData.total * metricMeta.factor).toFixed(metricMeta.decimals)
    : null

  const hasData = (data?.years?.length ?? 0) > 0

  return (
    <div className="space-y-3">
      {/* Controls */}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-2">
          <h3 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
            Jahresvergleich
          </h3>
          {bestYear && bestConverted && (
            <span className="text-xs text-neutral-500 dark:text-neutral-400">
              — Bestes Jahr: <strong className="text-neutral-700 dark:text-neutral-200">{bestYear}</strong> mit {bestConverted} {metricMeta.unit}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <select
            value={metric}
            onChange={e => setMetric(e.target.value as MetricKey)}
            className="border border-neutral-200 dark:border-neutral-700 rounded px-2 py-1 text-xs
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          >
            {METRIC_OPTIONS.map(o => (
              <option key={o.value} value={o.value}>{o.label} ({o.unit})</option>
            ))}
          </select>
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
            <button
              onClick={() => setChartType('line')}
              className={chartType === 'line'
                ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
            >
              Linie
            </button>
            <button
              onClick={() => setChartType('bar')}
              className={chartType === 'bar'
                ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
            >
              Balken
            </button>
          </div>
        </div>
      </div>

      {/* Year toggles */}
      {hasData && (
        <div className="flex items-center gap-1.5 flex-wrap">
          {yearNumbers.map((year, idx) => {
            const color = YEAR_COLORS[idx % YEAR_COLORS.length]
            const active = selectedYears.has(year)
            return (
              <button
                key={year}
                onClick={() => toggleYear(year)}
                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border cursor-pointer transition-all
                  ${active
                    ? 'border-transparent text-white'
                    : 'border-neutral-200 dark:border-neutral-700 text-neutral-500 dark:text-neutral-400 bg-transparent'
                  }`}
                style={active ? { backgroundColor: color } : {}}
              >
                {!active && (
                  <span className="w-2 h-2 rounded-full inline-block" style={{ backgroundColor: color }} />
                )}
                {year}
              </button>
            )
          })}
        </div>
      )}

      {/* Charts */}
      {isLoading ? (
        <div className="flex items-center justify-center h-[240px] text-sm text-neutral-400 rounded-xl border border-neutral-200 dark:border-neutral-800">
          Lade Jahresvergleich…
        </div>
      ) : isError ? (
        <div className="flex items-center justify-center h-[240px] text-sm text-neutral-400 rounded-xl border border-neutral-200 dark:border-neutral-800">
          Fehler beim Laden
        </div>
      ) : !hasData ? (
        <div className="flex items-center justify-center h-[240px] text-sm text-neutral-400 rounded-xl border border-neutral-200 dark:border-neutral-800">
          Keine Daten vorhanden
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
          <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden">
            <div className="px-3 pt-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Monatlich</div>
            <div ref={monthlyRef} className="h-[240px] w-full" />
          </div>
          <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden">
            <div className="px-3 pt-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Kumulativ</div>
            <div ref={cumulRef} className="h-[240px] w-full" />
          </div>
        </div>
      )}
    </div>
  )
}
