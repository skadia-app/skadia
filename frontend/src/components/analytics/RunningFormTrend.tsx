import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart, ScatterChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  MarkLineComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchRunningFormBaseline, fetchRunningFormTrend } from '@/api/analytics'
import { useECharts } from '@/hooks/useECharts'
import { toLocalISO } from '@/lib/dateUtils'

echarts.use([
  LineChart,
  ScatterChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  MarkLineComponent,
  DataZoomComponent,
  CanvasRenderer,
])

type TimeRange = '3m' | '6m' | '1y' | 'all'

const RANGE_LABELS: Record<TimeRange, string> = {
  '3m': '3M',
  '6m': '6M',
  '1y': '1J',
  'all': 'Alles',
}

const RANGE_TOOLTIPS: Record<TimeRange, string> = {
  '3m': 'Letzte 3 Monate',
  '6m': 'Letzte 6 Monate',
  '1y': 'Letztes Jahr (365 Tage)',
  'all': 'Gesamter Zeitraum',
}

function getDateRange(range: TimeRange): { from: string | undefined; to: string | undefined } {
  if (range === 'all') return { from: undefined, to: undefined }
  const to = new Date()
  const from = new Date()
  switch (range) {
    case '3m': from.setMonth(from.getMonth() - 3); break
    case '6m': from.setMonth(from.getMonth() - 6); break
    case '1y': from.setFullYear(from.getFullYear() - 1); break
  }
  return { from: toLocalISO(from), to: toLocalISO(to) }
}

function fmt(v: number | null | undefined, decimals = 1): string {
  if (v == null) return '—'
  return v.toFixed(decimals)
}

export default function RunningFormTrend() {
  const [range, setRange] = useState<TimeRange>('3m')

  const { from, to } = useMemo(() => getDateRange(range), [range])

  const { data: baseline, isLoading: baselineLoading } = useQuery({
    queryKey: ['running-form', 'baseline'],
    queryFn: fetchRunningFormBaseline,
    staleTime: 10 * 60 * 1000,
    retry: 1,
  })

  const { data: trend, isLoading: trendLoading, isError } = useQuery({
    queryKey: ['running-form', 'trend', from, to],
    queryFn: () => fetchRunningFormTrend(from, to),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const points = trend?.points ?? []
  const hasVerticalRatio = points.some(p => p.vertical_ratio != null)

  const containerRef = useECharts(
    (theme) => {
      const dates = points.map(p => p.date)
      const dutyFactorData = points.map(p => p.duty_factor ?? null)
      const verticalRatioData = points.map(p => p.vertical_ratio ?? null)

      const dfBaseline = baseline?.duty_factor_median ?? null
      const vrBaseline = baseline?.vertical_ratio_median ?? null

      const yAxes: echarts.EChartsCoreOption['yAxis'] = [
        {
          type: 'value',
          name: 'Duty Factor',
          nameTextStyle: { color: theme.textColor, fontSize: 11 },
          axisLine: { lineStyle: { color: theme.textColor } },
          axisLabel: { color: theme.textColor, fontSize: 11, formatter: (v: number) => v.toFixed(2) },
          splitLine: { lineStyle: { color: theme.gridLineColor } },
          min: 'dataMin',
          max: 'dataMax',
        },
      ]

      if (hasVerticalRatio) {
        yAxes.push({
          type: 'value',
          name: 'Vert. Ratio',
          nameTextStyle: { color: theme.textColor, fontSize: 11 },
          axisLine: { lineStyle: { color: theme.textColor } },
          axisLabel: { color: theme.textColor, fontSize: 11, formatter: (v: number) => v.toFixed(1) + '%' },
          splitLine: { show: false },
          min: 'dataMin',
          max: 'dataMax',
        } as echarts.EChartsCoreOption['yAxis'] extends any[] ? echarts.EChartsCoreOption['yAxis'][number] : never)
      }

      const series: echarts.EChartsCoreOption['series'] = [
        {
          name: 'Duty Factor',
          type: 'line',
          data: dutyFactorData,
          yAxisIndex: 0,
          smooth: true,
          showSymbol: points.length <= 30,
          lineStyle: { color: '#3b82f6', width: 2 },
          itemStyle: { color: '#3b82f6' },
          connectNulls: false,
          markLine: dfBaseline != null ? {
            silent: true,
            symbol: 'none',
            lineStyle: { color: '#9ca3af', type: 'dashed', width: 1 },
            label: {
              formatter: () => `Median: ${dfBaseline.toFixed(2)}`,
              color: theme.textColor,
              fontSize: 10,
              position: 'insideEndTop',
            },
            data: [{ yAxis: dfBaseline }],
          } : undefined,
        },
      ]

      if (hasVerticalRatio) {
        series.push({
          name: 'Vert. Ratio',
          type: 'line',
          data: verticalRatioData,
          yAxisIndex: 1,
          smooth: true,
          showSymbol: points.length <= 30,
          lineStyle: { color: '#10b981', width: 2 },
          itemStyle: { color: '#10b981' },
          connectNulls: false,
          markLine: vrBaseline != null ? {
            silent: true,
            symbol: 'none',
            lineStyle: { color: '#9ca3af', type: 'dashed', width: 1 },
            label: {
              formatter: () => `Median: ${vrBaseline.toFixed(1)}%`,
              color: theme.textColor,
              fontSize: 10,
              position: 'insideEndBottom',
            },
            data: [{ yAxis: vrBaseline }],
          } : undefined,
        })
      }

      return {
        animation: false,
        grid: { left: 60, right: hasVerticalRatio ? 70 : 20, top: 20, bottom: 60 },
        xAxis: {
          type: 'category',
          data: dates,
          axisLabel: { color: theme.textColor, fontSize: 11, rotate: 30 },
          axisLine: { lineStyle: { color: theme.gridLineColor } },
          axisTick: { show: false },
        },
        yAxis: yAxes,
        series,
        tooltip: {
          trigger: 'axis',
          backgroundColor: theme.isDark ? '#1c1c1c' : '#fff',
          borderColor: theme.isDark ? '#404040' : '#e5e7eb',
          textStyle: { color: theme.isDark ? '#e5e7eb' : '#111827', fontSize: 12 },
          formatter: (params: any) => {
            if (!Array.isArray(params) || params.length === 0) return ''
            const date = params[0].axisValue
            const idx = params[0].dataIndex
            const p = points[idx]
            if (!p) return date
            let html = `<div style="font-weight:600;margin-bottom:4px">${date}</div>`
            if (p.duty_factor != null) {
              html += `<div><span style="color:#3b82f6">●</span> Duty Factor: <b>${fmt(p.duty_factor, 3)}</b></div>`
            }
            if (p.vertical_ratio != null) {
              html += `<div><span style="color:#10b981">●</span> Vert. Ratio: <b>${fmt(p.vertical_ratio, 1)}%</b></div>`
            }
            if (p.gct_asymmetry != null) {
              html += `<div><span style="color:#f59e0b">●</span> GCT-Asymm.: <b>${fmt(p.gct_asymmetry, 1)}%</b></div>`
            }
            if (p.tier > 0) {
              html += `<div style="color:${theme.textColor};font-size:11px;margin-top:2px">Tier ${p.tier}</div>`
            }
            return html
          },
        },
        legend: {
          show: hasVerticalRatio,
          top: 'top',
          textStyle: { color: theme.textColor, fontSize: 11 },
          itemStyle: { borderWidth: 0 },
        },
        dataZoom: points.length > 60 ? [
          { type: 'inside', xAxisIndex: 0, filterMode: 'none' },
          { type: 'slider', xAxisIndex: 0, height: 20, bottom: 8, fillerColor: 'rgba(59,130,246,0.12)', handleStyle: { color: '#3b82f6' }, textStyle: { color: theme.textColor } },
        ] : [],
      }
    },
    [points, baseline, hasVerticalRatio],
  )

  const isLoading = baselineLoading || trendLoading
  const isEmpty = !isLoading && points.length === 0

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
        <div>
          <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
            Lauftechnik — Verlauf
          </h3>
          {baseline && baseline.activity_count > 0 && (
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              90-Tage-Baseline: {baseline.activity_count} {baseline.activity_count === 1 ? 'Aktivität' : 'Aktivitäten'}
              {baseline.duty_factor_median != null && (
                <> · Duty Factor Ø {fmt(baseline.duty_factor_median, 3)}</>
              )}
              {baseline.vertical_ratio_median != null && (
                <> · Vert. Ratio Ø {fmt(baseline.vertical_ratio_median, 1)}%</>
              )}
              {baseline.gct_asymmetry_median != null && (
                <> · GCT-Asymm. Ø {fmt(baseline.gct_asymmetry_median, 1)}%</>
              )}
            </p>
          )}
        </div>

        {/* Time range buttons */}
        <div className="flex items-center gap-1">
          {(Object.keys(RANGE_LABELS) as TimeRange[]).map((r) => (
            <button
              key={r}
              title={RANGE_TOOLTIPS[r]}
              onClick={() => setRange(r)}
              className={[
                'px-2 py-0.5 rounded text-xs font-medium transition-colors',
                range === r
                  ? 'bg-blue-500 text-white'
                  : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800',
              ].join(' ')}
            >
              {RANGE_LABELS[r]}
            </button>
          ))}
        </div>
      </div>

      {/* Chart area */}
      {isLoading && (
        <div className="h-[280px] flex items-center justify-center text-neutral-400 text-sm">
          Laden…
        </div>
      )}

      {isError && (
        <div className="h-[280px] flex items-center justify-center text-red-500 text-sm">
          Fehler beim Laden der Lauftechnik-Daten.
        </div>
      )}

      {isEmpty && (
        <div className="h-[280px] flex items-center justify-center text-neutral-400 dark:text-neutral-500 text-sm">
          Keine Lauftechnik-Daten im gewählten Zeitraum.
          <br />
          <span className="text-xs mt-1 block text-center">
            Lauftechnik-Daten werden von Garmin-Geräten mit Running Dynamics erfasst.
          </span>
        </div>
      )}

      {!isLoading && !isError && !isEmpty && (
        <div ref={containerRef} className="h-[280px] w-full" />
      )}
    </div>
  )
}
