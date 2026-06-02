import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  MarkAreaComponent,
  MarkLineComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useECharts } from '@/hooks/useECharts'
import { fmtDuration } from '@/lib/format'
import type { StreamPoint } from '@/api/types'

echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  MarkAreaComponent,
  MarkLineComponent,
  CanvasRenderer,
])

// ── Types ─────────────────────────────────────────────────────────────────────

interface GlucoseChartPanelProps {
  streams: StreamPoint[]
}

// ── Component ─────────────────────────────────────────────────────────────────

export function GlucoseChartPanel({ streams }: GlucoseChartPanelProps) {
  const { t } = useTranslation()
  const { hasGlucose, xValues, glucoseValues } = useMemo(() => {
    let hasAny = false
    const xs: number[] = []
    const ys: (number | null)[] = []
    const t0 = streams.length > 0 ? new Date(streams[0].time).getTime() : 0
    for (const s of streams) {
      if (s.glucose_mg_dl != null) hasAny = true
      xs.push(Math.round((new Date(s.time).getTime() - t0) / 1000))
      ys.push(s.glucose_mg_dl != null ? s.glucose_mg_dl : null)
    }
    return { hasGlucose: hasAny, xValues: xs, glucoseValues: ys }
  }, [streams])

  const containerRef = useECharts(
    (theme) => {
      if (!hasGlucose) return { series: [] }

      const data = xValues.map((x, i) => [x, glucoseValues[i]])
      const zoneNormal = t('activityDetail.glucose.zones.normal')
      const zoneElevated = t('activityDetail.glucose.zones.elevated')
      const zoneHigh = t('activityDetail.glucose.zones.high')

      return {
        backgroundColor: 'transparent',
        grid: { left: 52, right: 20, top: 20, bottom: 48 },
        tooltip: {
          trigger: 'axis',
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (params: unknown) => {
            const items = params as Array<{ value: [number, number | null]; axisValueLabel: string }>
            if (!Array.isArray(items) || items.length === 0) return ''
            const elapsed = items[0].value[0]
            const val = items[0].value[1]
            if (val == null) return ''
            let zone = zoneNormal
            let zoneColor = '#22c55e'
            if (val >= 200) { zone = zoneHigh; zoneColor = '#ef4444' }
            else if (val >= 160) { zone = zoneElevated; zoneColor = '#f59e0b' }
            return `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${fmtDuration(elapsed)}</div>
              <div><strong style="color:#f9fafb">${val.toFixed(1)} mg/dL</strong> <span style="color:${zoneColor};font-size:10px">${zone}</span></div>`
          },
        },
        xAxis: {
          type: 'value',
          min: 0,
          max: xValues[xValues.length - 1] ?? 0,
          axisLabel: {
            fontSize: 10,
            color: theme.textColor,
            formatter: (v: number) => fmtDuration(v),
          },
          splitLine: { show: false },
          axisLine: { lineStyle: { color: '#374151' } },
        },
        yAxis: {
          type: 'value',
          axisLabel: {
            fontSize: 10,
            color: theme.textColor,
            formatter: (v: number) => `${v}`,
          },
          splitLine: { lineStyle: { color: theme.gridLineColor, type: 'dashed' } },
          name: 'mg/dL',
          nameTextStyle: { color: theme.textColor, fontSize: 10 },
        },
        dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none' }],
        series: [
          {
            type: 'line',
            data,
            showSymbol: false,
            smooth: 0.2,
            lineStyle: { width: 2, color: '#6366f1' },
            itemStyle: { color: '#6366f1' },
            connectNulls: false,
            areaStyle: {
              color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: 'rgba(99,102,241,0.25)' },
                { offset: 1, color: 'rgba(99,102,241,0.02)' },
              ]),
            },
            markArea: {
              silent: true,
              data: [
                // Green zone: < 160 (normal during exercise)
                [
                  { yAxis: 0, itemStyle: { color: 'rgba(34,197,94,0.08)' } },
                  { yAxis: 160 },
                ],
                // Yellow zone: 160 – 200 (elevated during exercise)
                [
                  { yAxis: 160, itemStyle: { color: 'rgba(245,158,11,0.09)' } },
                  { yAxis: 200 },
                ],
                // Red zone: >= 200 (high even during exercise)
                [
                  { yAxis: 200, itemStyle: { color: 'rgba(239,68,68,0.08)' } },
                  { yAxis: 999 },
                ],
              ],
            },
            markLine: {
              silent: true,
              symbol: ['none', 'none'],
              data: [
                {
                  yAxis: 100,
                  lineStyle: { color: '#9ca3af', type: 'dashed', width: 1, opacity: 0.4 },
                  label: { show: true, position: 'insideEndTop', formatter: t('activityDetail.glucose.fastingLabel', { value: 100 }), fontSize: 9, color: '#9ca3af' },
                },
                {
                  yAxis: 125,
                  lineStyle: { color: '#9ca3af', type: 'dashed', width: 1, opacity: 0.4 },
                  label: { show: true, position: 'insideEndTop', formatter: t('activityDetail.glucose.fastingLabel', { value: 125 }), fontSize: 9, color: '#9ca3af' },
                },
              ],
            },
          },
        ],
      }
    },
    [hasGlucose, t, xValues, glucoseValues],
  )

  if (!hasGlucose) return null

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
          {t('activityDetail.glucose.title')}
        </h3>
        <div className="flex items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400">
          <span className="flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-sm bg-green-500 opacity-70" />
            {t('activityDetail.glucose.zones.normal')}
          </span>
          <span className="flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-sm bg-amber-500 opacity-70" />
            {t('activityDetail.glucose.zones.elevated')}
          </span>
          <span className="flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-sm bg-red-500 opacity-70" />
            {t('activityDetail.glucose.zones.high')}
          </span>
        </div>
      </div>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        <div ref={containerRef} className="h-[200px] w-full" />
      </div>

      <p className="text-xs text-neutral-400 dark:text-neutral-500 leading-relaxed">
        {t('activityDetail.glucose.disclaimer')}
      </p>
    </div>
  )
}
