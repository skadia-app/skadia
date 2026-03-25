import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { BarChart, PieChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  ToolboxComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchZoneDistributionSeries } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { useECharts } from '@/hooks/useECharts'
import { fmtDuration } from '@/lib/format'

echarts.use([BarChart, PieChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, ToolboxComponent, CanvasRenderer])

type ZoneType = 'hr' | 'power' | 'cadence'
type Granularity = 'week' | 'month'
type ChartView = 'bar' | 'pie'

// Zone colors: 1=green, 2=light-green, 3=yellow, 4=orange, 5=red
const ZONE_COLORS = [
  '#22c55e', // Zone 1
  '#86efac', // Zone 2
  '#eab308', // Zone 3
  '#f97316', // Zone 4
  '#ef4444', // Zone 5
]

const ZONE_LABELS = ['Zone 1', 'Zone 2', 'Zone 3', 'Zone 4', 'Zone 5']


export default function ZoneDistributionChart() {
  const { toQueryString } = useAnalyticsFilters()
  const [zoneType, setZoneType] = useState<ZoneType>('hr')
  const [granularity, setGranularity] = useState<Granularity>('month')
  const [chartView, setChartView] = useState<ChartView>('bar')

  const queryParams = useMemo(() => {
    const base = toQueryString()
    const params = new URLSearchParams(base)
    params.set('zone_type', zoneType)
    params.set('granularity', granularity)
    return params.toString()
  }, [toQueryString, zoneType, granularity])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'zone-distribution', queryParams],
    queryFn: () => fetchZoneDistributionSeries(queryParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const containerRef = useECharts(
    () => {
      if (!data || data.length === 0) return { series: [] }

      const periodsSet = new Set<string>()
      const zoneIndicesSet = new Set<number>()
      for (const point of data) {
        periodsSet.add(point.period)
        zoneIndicesSet.add(point.zone_index)
      }
      const periods = Array.from(periodsSet).sort()
      const zoneIndices = Array.from(zoneIndicesSet).sort((a, b) => a - b)

      const zoneTotals = new Map<number, number>()
      for (const point of data) {
        zoneTotals.set(point.zone_index, (zoneTotals.get(point.zone_index) ?? 0) + point.total_seconds)
      }

      if (chartView === 'pie') {
        const pieData = zoneIndices.map(zoneIdx => ({
          name: ZONE_LABELS[zoneIdx - 1] ?? `Zone ${zoneIdx}`,
          value: zoneTotals.get(zoneIdx) ?? 0,
          itemStyle: { color: ZONE_COLORS[zoneIdx - 1] ?? '#9ca3af' },
        })).filter(d => d.value > 0)

        return {
          backgroundColor: 'transparent',
          tooltip: {
            trigger: 'item',
            backgroundColor: 'rgba(17,24,39,0.92)',
            borderColor: '#374151',
            textStyle: { color: '#f3f4f6', fontSize: 12 },
            formatter: (params: unknown) => {
              const p = params as { name: string; value: number; percent: number }
              return `<div style="font-size:12px">
                <div style="font-weight:600;margin-bottom:2px">${p.name}</div>
                <div>${fmtDuration(p.value, 'abbr')} (${p.percent?.toFixed(1)}%)</div>
              </div>`
            },
          },
          legend: {
            orient: 'vertical',
            right: 8,
            top: 'center',
            textStyle: { fontSize: 10, color: '#9ca3af' },
            itemWidth: 10,
            itemHeight: 10,
          },
          series: [
            {
              type: 'pie',
              radius: ['40%', '70%'],
              center: ['40%', '50%'],
              data: pieData,
              label: { show: false },
              emphasis: {
                itemStyle: { shadowBlur: 8, shadowOffsetX: 0, shadowColor: 'rgba(0,0,0,0.3)' },
              },
            },
          ],
        }
      }

      const series = zoneIndices.map(zoneIdx => {
        const values = periods.map(period => {
          const match = data.find(d => d.period === period && d.zone_index === zoneIdx)
          return match ? match.total_seconds : 0
        })
        const color = ZONE_COLORS[zoneIdx - 1] ?? '#9ca3af'
        const label = ZONE_LABELS[zoneIdx - 1] ?? `Zone ${zoneIdx}`
        return {
          name: label,
          type: 'bar' as const,
          stack: 'zones',
          data: values,
          itemStyle: { color },
          emphasis: { focus: 'series' as const },
        }
      })

      return {
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
          },
          iconStyle: { borderColor: '#6b7280' },
          emphasis: { iconStyle: { borderColor: '#d1d5db' } },
        },
        dataZoom: [
          { type: 'inside', xAxisIndex: 0, filterMode: 'none' },
        ],
        legend: {
          data: zoneIndices.map(i => ZONE_LABELS[i - 1] ?? `Zone ${i}`),
          top: 4,
          textStyle: { fontSize: 10, color: '#9ca3af' },
          itemWidth: 12,
          itemHeight: 8,
        },
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'shadow' },
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (params: unknown) => {
            const items = params as Array<{ seriesName: string; value: number; axisValueLabel: string; color: string }>
            if (!Array.isArray(items) || items.length === 0) return ''
            let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>`
            let total = 0
            for (const p of items) {
              if (p.value <= 0) continue
              html += `<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">
                <span style="display:inline-block;width:8px;height:8px;background:${p.color}"></span>
                <span style="color:#d1d5db">${p.seriesName}:</span>
                <strong style="color:#f9fafb">${fmtDuration(p.value, 'abbr')}</strong>
              </div>`
              total += p.value
            }
            html += `<div style="margin-top:4px;padding-top:4px;border-top:1px solid #374151;color:#9ca3af;font-size:11px">
              Gesamt: <strong style="color:#f9fafb">${fmtDuration(total, 'abbr')}</strong>
            </div>`
            return html
          },
        },
        xAxis: {
          type: 'category',
          data: periods,
          axisLabel: {
            fontSize: 10,
            color: '#6b7280',
            rotate: periods.length > 12 ? 30 : 0,
          },
          axisLine: { lineStyle: { color: '#374151' } },
        },
        yAxis: {
          type: 'value',
          axisLabel: {
            fontSize: 10,
            color: '#6b7280',
            formatter: (v: number) => {
              const h = v / 3600
              return h >= 1 ? `${h.toFixed(0)}h` : `${(v / 60).toFixed(0)}m`
            },
          },
          splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
        },
        series,
      }
    },
    [data, chartView],
  )

  const hasData = data && data.length > 0

  const zoneTypeLabel: Record<ZoneType, string> = { hr: 'Herzfrequenz', power: 'Leistung', cadence: 'Kadenz' }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
          Zonenverteilung
        </h3>
        <div className="flex items-center gap-2 flex-wrap">
          {/* Zone type toggle */}
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
            {(['hr', 'power', 'cadence'] as ZoneType[]).map(zt => (
              <button
                key={zt}
                onClick={() => setZoneType(zt)}
                className={zoneType === zt
                  ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                  : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
              >
                {zoneTypeLabel[zt]}
              </button>
            ))}
          </div>
          {/* Granularity toggle — only relevant in bar mode */}
          {chartView === 'bar' && (
            <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
              <button
                onClick={() => setGranularity('week')}
                className={granularity === 'week'
                  ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                  : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
              >
                Woche
              </button>
              <button
                onClick={() => setGranularity('month')}
                className={granularity === 'month'
                  ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                  : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
              >
                Monat
              </button>
            </div>
          )}
          {/* Chart view toggle: bar ↔ pie */}
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
            <button
              onClick={() => setChartView('bar')}
              title="Zeitverlauf (Balken)"
              className={chartView === 'bar'
                ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
            >
              Balken
            </button>
            <button
              onClick={() => setChartView('pie')}
              title="Gesamtverteilung (Kreis)"
              className={chartView === 'pie'
                ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
            >
              Kreis
            </button>
          </div>
        </div>
      </div>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
            Lade Zonenverteilung…
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
            Fehler beim Laden
          </div>
        ) : !hasData ? (
          <div className="flex flex-col items-center justify-center h-[280px] gap-2 text-neutral-400">
            <span className="text-sm">Keine Zonen-Daten vorhanden</span>
            <span className="text-xs">Für {zoneTypeLabel[zoneType]}-Zonen werden Aktivitäten mit entsprechenden Messwerten benötigt</span>
          </div>
        ) : (
          <div ref={containerRef} className="h-[280px] w-full" />
        )}
      </div>
    </div>
  )
}
