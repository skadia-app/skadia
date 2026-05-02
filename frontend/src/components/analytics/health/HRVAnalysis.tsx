import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart, ScatterChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  MarkPointComponent,
  MarkLineComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchHRVAnalysis } from '@/api/analytics'
import { useChangepoints, changepointsToMarkLines } from '@/components/analytics/ChangepointMarkers'
import { useECharts } from '@/hooks/useECharts'
import { cn } from '@/lib/utils'
import PureAggregateRangePicker from '@/components/analytics/filters/PureAggregateRangePicker'
import { type DateRange } from '@/components/analytics/filters/RangePresetBar'

echarts.use([
  LineChart,
  ScatterChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  MarkPointComponent,
  MarkLineComponent,
  DataZoomComponent,
  CanvasRenderer,
])

export default function HRVAnalysisChart() {
  const [range, setRange] = useState<DateRange | null>(null)
  const changepoints = useChangepoints('hrv_rmssd', 180)

  const queryParams = range
    ? new URLSearchParams({ from: range.from, to: range.to }).toString()
    : null

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'hrv', queryParams],
    queryFn: () => fetchHRVAnalysis(queryParams!),
    enabled: queryParams !== null,
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const containerRef = useECharts(
    () => {
      if (!data) return { series: [] }

      const points = data.points ?? []
      if (points.length === 0) return { series: [] }

      const dates = points.map(p => p.date)
      const rmssdValues = points.map(p => p.rmssd)
      const mean = data.baseline_mean
      const std = data.baseline_std

      const upperBand = dates.map(() => mean + std)
      const lowerBand = dates.map(() => mean - std)

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

      return {
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
          confine: true,
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
          {
            name: 'Baseline ±1 SD',
            type: 'line',
            data: upperBand,
            showSymbol: false,
            lineStyle: { opacity: 0 },
            areaStyle: { color: 'rgba(156,163,175,0.0)', origin: 'auto' },
            stack: 'baseline-band',
            z: 0,
          },
          {
            name: '_baseline-lower',
            type: 'line',
            data: lowerBand,
            showSymbol: false,
            lineStyle: { opacity: 0 },
            areaStyle: { color: 'rgba(156,163,175,0.2)', origin: 'auto' },
            stack: 'baseline-band',
            z: 0,
            tooltip: { show: false },
            legendHoverLink: false,
          },
          {
            name: 'RMSSD',
            type: 'scatter',
            data: rmssdValues,
            symbolSize: 5,
            itemStyle: { color: '#6366f1' },
            z: 3,
            markPoint: { data: markPointData, z: 5 },
            markLine: changepoints.length > 0 ? {
              silent: true,
              data: changepointsToMarkLines(changepoints),
            } : undefined,
          },
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
    },
    [data, changepoints],
  )

  const anomalyCount = data?.anomalies?.length ?? 0

  return (
    <div className="space-y-3">
      {/* Header */}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
            HRV-Analyse
          </h2>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            RMSSD-Trend mit Anomalie-Erkennung
          </p>
        </div>
        <PureAggregateRangePicker defaultPreset="90d" onChange={setRange} />
      </div>

      {/* Info panel — only when data has points */}
      {data && data.points.length > 0 && (
        <div className="flex items-center gap-3 flex-wrap">
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300">
            7-Tage-Avg: {data.seven_day_avg.toFixed(1)} ms
          </span>
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
            Baseline: {data.baseline_mean.toFixed(1)} ms
          </span>
          {anomalyCount > 0 && (
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-accent-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300">
              {anomalyCount} {anomalyCount === 1 ? 'Anomalie' : 'Anomalien'}
            </span>
          )}
        </div>
      )}

      {/* Chart — always-mount pattern prevents ECharts dispose crash on preset toggle */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        <div className="relative h-[280px]">
          {isLoading && (
            <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">
              Lade HRV-Daten…
            </div>
          )}
          {!isLoading && isError && (
            <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">
              Fehler beim Laden der HRV-Daten
            </div>
          )}
          {!isLoading && !isError && (!data || !data.points?.length) && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 text-neutral-400">
              <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                  d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z" />
              </svg>
              <span className="text-sm">Keine HRV-Daten für diesen Zeitraum</span>
            </div>
          )}
          <div
            ref={containerRef}
            className={cn('w-full h-full', {
              invisible: isLoading || isError || !data || !data.points?.length,
            })}
          />
        </div>
      </div>
    </div>
  )
}
