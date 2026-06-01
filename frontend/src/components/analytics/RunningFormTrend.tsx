import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import type { YAXisOption, LineSeriesOption } from 'echarts/types/dist/shared'
import { LineChart, ScatterChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  MarkLineComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useTranslation } from 'react-i18next'
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

const TIME_RANGES: TimeRange[] = ['3m', '6m', '1y', 'all']

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

function fmt(v: number | null | undefined, locale: string, decimals = 1): string {
  if (v == null) return '—'
  return new Intl.NumberFormat(locale, {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  }).format(v)
}

export default function RunningFormTrend() {
  const { t, i18n } = useTranslation()
  const [range, setRange] = useState<TimeRange>('3m')
  const locale = i18n.language || 'de'
  const dutyFactorLabel = t('analyticsHealthInsights.runningForm.dutyFactor')
  const verticalRatioLabel = t('analyticsHealthInsights.runningForm.verticalRatioShort')
  const gctAsymmetryLabel = t('analyticsHealthInsights.runningForm.gctAsymmetryShort')
  const medianLabel = t('analyticsHealthInsights.common.median')

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

  const isLoading = baselineLoading || trendLoading
  const isEmpty = !isLoading && points.length === 0
  const hasData = !isLoading && !isError && !isEmpty

  const containerRef = useECharts(
    (theme) => {
      const dates = points.map(p => p.date)
      const dutyFactorData = points.map(p => p.duty_factor ?? null)
      const verticalRatioData = points.map(p => p.vertical_ratio ?? null)

      const dfBaseline = baseline?.duty_factor_median ?? null
      const vrBaseline = baseline?.vertical_ratio_median ?? null

      const yAxes: YAXisOption[] = [
        {
          type: 'value',
          name: dutyFactorLabel,
          nameTextStyle: { color: theme.textColor, fontSize: 11 },
          axisLine: { lineStyle: { color: theme.textColor } },
          axisLabel: { color: theme.textColor, fontSize: 11, formatter: (v: number) => fmt(v, locale, 2) },
          splitLine: { lineStyle: { color: theme.gridLineColor } },
          min: 'dataMin',
          max: 'dataMax',
        },
      ]

      if (hasVerticalRatio) {
        yAxes.push({
          type: 'value',
          name: verticalRatioLabel,
          nameTextStyle: { color: theme.textColor, fontSize: 11 },
          axisLine: { lineStyle: { color: theme.textColor } },
          axisLabel: { color: theme.textColor, fontSize: 11, formatter: (v: number) => `${fmt(v * 100, locale, 1)}%` },
          splitLine: { show: false },
          min: 'dataMin',
          max: 'dataMax',
        })
      }

      const series: LineSeriesOption[] = [
        {
          name: dutyFactorLabel,
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
              formatter: () => `${medianLabel}: ${fmt(dfBaseline, locale, 2)}`,
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
          name: verticalRatioLabel,
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
              formatter: () => `${medianLabel}: ${fmt(vrBaseline * 100, locale, 1)}%`,
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
          confine: true,
          trigger: 'axis',
          axisPointer: { type: 'cross' },
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
              html += `<div><span style="color:#3b82f6">●</span> ${dutyFactorLabel}: <b>${fmt(p.duty_factor, locale, 3)}</b></div>`
            }
            if (p.vertical_ratio != null) {
              html += `<div><span style="color:#10b981">●</span> ${verticalRatioLabel}: <b>${fmt(p.vertical_ratio * 100, locale, 1)}%</b></div>`
            }
            if (p.gct_asymmetry != null) {
              html += `<div><span style="color:#f59e0b">●</span> ${gctAsymmetryLabel}: <b>${fmt(p.gct_asymmetry, locale, 1)}%</b></div>`
            }
            if (p.tier > 0) {
              html += `<div style="color:${theme.textColor};font-size:11px;margin-top:2px">${t('analyticsHealthInsights.runningForm.tier', { tier: p.tier })}</div>`
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
    [points, baseline, hasVerticalRatio, dutyFactorLabel, verticalRatioLabel, gctAsymmetryLabel, medianLabel, locale, t],
    { enabled: hasData },
  )

  // Sport-gate: once the baseline has loaded, hide the entire component for
  // users who have never recorded running form data (no running-dynamics device).
  // During loading we keep it mounted to avoid a layout shift on first render.
  if (!baselineLoading && baseline != null && baseline.activity_count === 0) {
    return null
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
        <div>
          <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
            {t('analyticsHealthInsights.runningForm.title')}
          </h3>
          {baseline && baseline.activity_count > 0 && (
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              {t('analyticsHealthInsights.runningForm.baselineCount', { count: baseline.activity_count })}
              {baseline.duty_factor_median != null && (
                <> · {t('analyticsHealthInsights.runningForm.averageMetric', { label: dutyFactorLabel, value: fmt(baseline.duty_factor_median, locale, 3) })}</>
              )}
              {baseline.vertical_ratio_median != null && (
                <> · {t('analyticsHealthInsights.runningForm.averageMetric', { label: verticalRatioLabel, value: `${fmt(baseline.vertical_ratio_median * 100, locale, 1)}%` })}</>
              )}
              {baseline.gct_asymmetry_median != null && (
                <> · {t('analyticsHealthInsights.runningForm.averageMetric', { label: gctAsymmetryLabel, value: `${fmt(baseline.gct_asymmetry_median, locale, 1)}%` })}</>
              )}
            </p>
          )}
        </div>

        {/* Time range buttons */}
        <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
          {TIME_RANGES.map((r) => (
            <button
              key={r}
              title={t(`analyticsHealthInsights.runningForm.rangeTooltips.${r}`)}
              onClick={() => setRange(r)}
              className={range === r
                ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
            >
              {t(`analyticsHealthInsights.runningForm.rangeLabels.${r}`)}
            </button>
          ))}
        </div>
      </div>

      {/* Chart area — container always mounted; useECharts enabled only when data ready */}
      <div className="relative h-[280px]">
        {isLoading && (
          <div className="absolute inset-0 flex items-center justify-center text-neutral-400 text-sm">
            {t('analyticsHealthInsights.common.loading')}
          </div>
        )}
        {!isLoading && isError && (
          <div className="absolute inset-0 flex items-center justify-center text-red-500 text-sm">
            {t('analyticsHealthInsights.runningForm.loadFailed')}
          </div>
        )}
        {!isLoading && !isError && isEmpty && (
          <div className="absolute inset-0 flex flex-col items-center justify-center text-neutral-400 dark:text-neutral-500 text-sm gap-1">
            <span>{t('analyticsHealthInsights.runningForm.noData')}</span>
            <span className="text-xs text-center">
              {t('analyticsHealthInsights.runningForm.noDataHint')}
            </span>
          </div>
        )}
        {/* Container stays in DOM; chart only initialised when hasData to avoid removeChild crash on range toggle */}
        <div
          ref={containerRef}
          className="w-full h-full"
          style={{ visibility: hasData ? 'visible' : 'hidden' }}
        />
      </div>
    </div>
  )
}
