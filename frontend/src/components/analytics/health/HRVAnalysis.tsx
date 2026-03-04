import { useState, useRef, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart, ScatterChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  MarkPointComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchHRVAnalysis } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'

echarts.use([
  LineChart,
  ScatterChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  MarkPointComponent,
  DataZoomComponent,
  CanvasRenderer,
])

type TimePreset = '7d' | '30d' | '90d' | '1y' | 'all'

const presetLabels: Record<TimePreset, string> = {
  '7d': '7T',
  '30d': '30T',
  '90d': '90T',
  '1y': '1J',
  'all': 'Alle',
}

function getDateRange(preset: TimePreset): { from: string; to: string } {
  const to = new Date()
  const from = new Date()
  switch (preset) {
    case '7d': from.setDate(from.getDate() - 7); break
    case '30d': from.setDate(from.getDate() - 30); break
    case '90d': from.setDate(from.getDate() - 90); break
    case '1y': from.setFullYear(from.getFullYear() - 1); break
    case 'all': return { from: '0001-01-01', to: to.toISOString().split('T')[0] }
  }
  return {
    from: from.toISOString().split('T')[0],
    to: to.toISOString().split('T')[0],
  }
}

export default function HRVAnalysisChart() {
  const { toQueryString } = useAnalyticsFilters()
  const [preset, setPreset] = useState<TimePreset>('90d')
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  const { from, to } = useMemo(() => getDateRange(preset), [preset])

  const queryParams = useMemo(() => {
    const base = toQueryString()
    const params = new URLSearchParams(base)
    params.set('from', from)
    params.set('to', to)
    return params.toString()
  }, [toQueryString, from, to])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'hrv', queryParams],
    queryFn: () => fetchHRVAnalysis(queryParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

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
    if (!chartRef.current || !data) return

    const points = data.points ?? []
    if (points.length === 0) {
      chartRef.current.setOption({ series: [] }, { notMerge: true })
      return
    }

    const dates = points.map(p => p.date)
    const rmssdValues = points.map(p => p.rmssd)
    const mean = data.baseline_mean
    const std = data.baseline_std

    // Baseline band: two constant series for upper/lower
    const upperBand = dates.map(() => mean + std)
    const lowerBand = dates.map(() => mean - std)

    // Anomaly markPoints: find indices where date matches anomaly dates
    const anomalyDates = new Set(data.anomalies ?? [])
    const markPointData = points
      .filter(p => anomalyDates.has(p.date) && p.rmssd != null)
      .map(p => ({
        name: 'Anomalie',
        coord: [p.date, p.rmssd],
        symbol: 'pin',
        symbolSize: 24,
        label: { show: false },
        itemStyle: { color: '#f97316' },
      }))

    const option: EChartsCoreOption = {
      backgroundColor: 'transparent',
      grid: { left: 52, right: 20, top: 44, bottom: 40 },
      legend: {
        data: ['RMSSD', 'Baseline ±1 SD'],
        top: 8,
        textStyle: { fontSize: 10, color: '#9ca3af' },
        itemHeight: 8,
        itemWidth: 14,
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
        backgroundColor: 'rgba(17,24,39,0.92)',
        borderColor: '#374151',
        textStyle: { color: '#f3f4f6', fontSize: 12 },
        formatter: (params: unknown) => {
          const items = params as Array<{
            seriesName: string
            value: number | null
            axisValueLabel: string
            color: string
            seriesType: string
          }>
          if (!Array.isArray(items) || items.length === 0) return ''
          let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>`
          for (const p of items) {
            if (p.seriesName === 'Baseline ±1 SD' || p.value == null) continue
            html += `<div style="display:flex;align-items:center;gap:6px">
              <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>
              <span style="color:#d1d5db">${p.seriesName}:</span>
              <strong style="color:#f9fafb">${p.value.toFixed(1)} ms</strong>
            </div>`
          }
          return html
        },
      },
      xAxis: {
        type: 'category',
        data: dates,
        axisLabel: { fontSize: 10, color: '#6b7280' },
        splitLine: { show: false },
        axisLine: { lineStyle: { color: '#374151' } },
      },
      yAxis: {
        type: 'value',
        axisLabel: {
          fontSize: 10,
          color: '#6b7280',
          formatter: (v: number) => `${v}ms`,
        },
        splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
      },
      dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none' }],
      series: [
        // Upper baseline band (invisible, just acts as top boundary)
        {
          name: 'Baseline ±1 SD',
          type: 'line',
          data: upperBand,
          showSymbol: false,
          lineStyle: { opacity: 0 },
          areaStyle: {
            color: 'rgba(156,163,175,0.0)',
            origin: 'auto',
          },
          stack: 'baseline-band',
          z: 0,
        },
        // Lower band: fills between lower and upper
        {
          name: '_baseline-lower',
          type: 'line',
          data: lowerBand,
          showSymbol: false,
          lineStyle: { opacity: 0 },
          areaStyle: {
            color: 'rgba(156,163,175,0.2)',
            origin: 'auto',
          },
          stack: 'baseline-band',
          z: 0,
          tooltip: { show: false },
          legendHoverLink: false,
        },
        // RMSSD scatter points
        {
          name: 'RMSSD',
          type: 'scatter',
          data: rmssdValues,
          symbolSize: 5,
          itemStyle: { color: '#6366f1' },
          z: 3,
          markPoint: {
            data: markPointData,
            z: 5,
          },
        },
        // RMSSD trend line (same data, no symbol)
        {
          name: '_rmssd-line',
          type: 'line',
          data: rmssdValues,
          showSymbol: false,
          lineStyle: { width: 1.5, color: '#6366f1', opacity: 0.5 },
          itemStyle: { color: '#6366f1' },
          z: 2,
          tooltip: { show: false },
          legendHoverLink: false,
        },
        // Baseline mean line
        {
          name: '_baseline-mean',
          type: 'line',
          data: dates.map(() => mean),
          showSymbol: false,
          lineStyle: { width: 1, color: '#9ca3af', type: 'dashed' },
          z: 1,
          tooltip: { show: false },
          legendHoverLink: false,
        },
      ],
    }

    chartRef.current.setOption(option, { notMerge: true })
  }, [data])

  const anomalyCount = data?.anomalies?.length ?? 0

  return (
    <div className="space-y-3">
      {/* Header */}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          HRV-Analyse
        </h2>
        <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
          {(Object.keys(presetLabels) as TimePreset[]).map(p => (
            <button
              key={p}
              onClick={() => setPreset(p)}
              className={
                p === preset
                  ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                  : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
              }
            >
              {presetLabels[p]}
            </button>
          ))}
        </div>
      </div>

      {/* Info panel */}
      {data && (
        <div className="flex items-center gap-3 flex-wrap">
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300">
            7-Tage-Avg: {data.seven_day_avg.toFixed(1)} ms
          </span>
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
            Baseline: {data.baseline_mean.toFixed(1)} ms
          </span>
          {anomalyCount > 0 && (
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300">
              {anomalyCount} {anomalyCount === 1 ? 'Anomalie' : 'Anomalien'}
            </span>
          )}
          {data.morning_avg != null && (
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300">
              Morgen-Avg: {data.morning_avg.toFixed(1)} ms
            </span>
          )}
          {data.night_avg != null && (
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300">
              Nacht-Avg: {data.night_avg.toFixed(1)} ms
            </span>
          )}
        </div>
      )}

      {/* Chart */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        {isLoading ? (
          <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
            Lade HRV-Daten…
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
            Fehler beim Laden der HRV-Daten
          </div>
        ) : !data || data.points.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-[280px] gap-2 text-neutral-400">
            <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z" />
            </svg>
            <span className="text-sm">Keine HRV-Daten für diesen Zeitraum</span>
          </div>
        ) : (
          <div ref={containerRef} className="h-[280px] w-full" />
        )}
      </div>
    </div>
  )
}
