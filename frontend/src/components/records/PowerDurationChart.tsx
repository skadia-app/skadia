import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useECharts } from '@/hooks/useECharts'
import type { BestEffortRanked } from '@/api/bestEfforts'
import { POWER_DURATIONS, categoryLabel } from '@/lib/bestEffortLabels'
import type { SupportedLanguage } from '@/i18n/languages'
import { localeForLanguage } from '@/i18n/languages'
import { formatPower } from '@/lib/units'

echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

function hexToRgba(hex: string, alpha: number): string {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return `rgba(${r},${g},${b},${alpha})`
}

interface PowerSeries {
  name: string
  color: string
  sportGroups: string[]
}

interface PowerDurationChartProps {
  records: BestEffortRanked[]
  series: PowerSeries[]
  height?: number
  language?: SupportedLanguage
}

export function PowerDurationChart({ records, series, height = 220, language }: PowerDurationChartProps) {
  const { t } = useTranslation()
  const locale = localeForLanguage(language ?? 'de')
  const rank1 = useMemo(() => records.filter((r) => r.rank === 1), [records])
  const durationLabels = useMemo(() => POWER_DURATIONS.map((category) =>
    categoryLabel(category, language)
      .replace(' Sek', 's')
      .replace(' Min', 'm')
      .replace(' Std', 'h'),
  ), [language])

  const chartSeries = useMemo(() => {
    return series.map((s) => {
      const data = POWER_DURATIONS.map((cat) => {
        const matching = rank1
          .filter((r) => r.category === cat && s.sportGroups.includes(r.sport_group))
          .sort((a, b) => (b.value_numeric ?? 0) - (a.value_numeric ?? 0))
        return matching[0]?.value_numeric ?? null
      })
      return { ...s, data }
    }).filter((s) => s.data.some((v) => v !== null))
  }, [rank1, series])

  const containerRef = useECharts(
    (theme) => ({
      backgroundColor: 'transparent',
      tooltip: {
        trigger: 'axis' as const,
        backgroundColor: 'rgba(17,24,39,0.95)',
        borderColor: '#374151',
        textStyle: { color: '#f3f4f6', fontSize: 11 },
        valueFormatter: (v: number | string) => typeof v === 'number' ? formatPower(v, { locale }) : v,
      },
      grid: { left: 50, right: 20, top: 15, bottom: 35 },
      xAxis: {
        type: 'category' as const,
        data: durationLabels,
        axisLabel: { color: theme.textColor, fontSize: 10 },
        axisLine: { lineStyle: { color: '#374151' } },
        splitLine: { show: false },
      },
      yAxis: {
        type: 'value' as const,
        name: 'W',
        nameTextStyle: { color: theme.textColor, fontSize: 10 },
        axisLabel: { color: theme.textColor, fontSize: 10 },
        splitLine: { lineStyle: { color: theme.gridLineColor, type: 'dashed' as const } },
      },
      series: chartSeries.map((s) => ({
        name: s.name,
        type: 'line' as const,
        smooth: true,
        data: s.data,
        lineStyle: { color: s.color, width: 2 },
        itemStyle: { color: s.color },
        areaStyle: { color: hexToRgba(s.color, 0.06) },
        symbol: 'circle' as const,
        symbolSize: 5,
        connectNulls: true,
      })),
    }),
    [chartSeries, durationLabels, locale],
  )

  if (chartSeries.length === 0) return null

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">{t('records.powerCurve.title')}</span>
          <span className="text-[10px] text-neutral-500 dark:text-neutral-600 bg-neutral-100 dark:bg-neutral-800 px-1.5 py-0.5 rounded">{t('records.powerCurve.includeZero')}</span>
        </div>
        <div className="flex items-center gap-3">
          {chartSeries.map((s) => (
            <span key={s.name} className="flex items-center gap-1.5 text-[10px] text-neutral-500 dark:text-neutral-400">
              <span className="w-2 h-2 rounded-full" style={{ backgroundColor: s.color }} />
              {s.name}
            </span>
          ))}
        </div>
      </div>
      <div ref={containerRef} style={{ width: '100%', height }} />
    </div>
  )
}
