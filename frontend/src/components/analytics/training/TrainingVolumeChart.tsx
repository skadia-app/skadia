import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  ToolboxComponent,
  BrushComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchTrainingVolumeSeries } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { useECharts } from '@/hooks/useECharts'
import { sportChartColor } from '@/lib/sportColors'
import { sportLabel } from '@/lib/sportTaxonomy'
import { cn } from '@/lib/utils'
import { bindLineXBrushZoom, lineXBrush, lineXBrushToolboxFeature } from './chartBrushZoom'

// BrushComponent backs the line-brush toolbox action used for chart zoom.
echarts.use([
  BarChart,
  LineChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  ToolboxComponent,
  BrushComponent,
  CanvasRenderer,
])

type Granularity = 'week' | 'month'
type VolumeMetric = 'distance' | 'duration' | 'tss'
type ChartType = 'bar' | 'line'

function getMetricField(metric: VolumeMetric): 'distance_m' | 'moving_time_s' | 'tss' {
  switch (metric) {
    case 'distance': return 'distance_m'
    case 'duration': return 'moving_time_s'
    case 'tss': return 'tss'
  }
}

export default function TrainingVolumeChart() {
  const { toQueryString } = useAnalyticsFilters()
  const [granularity, setGranularity] = useState<Granularity>('month')
  const [volumeMetric, setVolumeMetric] = useState<VolumeMetric>('distance')
  const [chartType, setChartType] = useState<ChartType>('bar')
  const queryParams = useMemo(() => {
    const base = toQueryString()
    const params = new URLSearchParams(base)
    params.set('granularity', granularity)
    params.set('metric', getMetricField(volumeMetric))
    return params.toString()
  }, [toQueryString, granularity, volumeMetric])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'training-volume', queryParams],
    queryFn: () => fetchTrainingVolumeSeries(queryParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const containerRef = useECharts(
    () => {
      if (!data || data.length === 0) return { series: [] }

      const periodsSet = new Set<string>()
      const sportsSet = new Set<string>()
      for (const point of data) {
        periodsSet.add(point.period)
        // Only register sports that actually have non-zero data — otherwise the
        // legend pollutes with sports the backend returns as zero-rows for
        // periods where no activity of that sport exists.
        if (point.value > 0) sportsSet.add(point.sport_type)
      }
      const periods = Array.from(periodsSet).sort()
      const sports = Array.from(sportsSet).sort()

      const series = sports.map(sport => {
        const label = sportLabel(sport)
        const values = periods.map(period => {
          const match = data.find(d => d.period === period && d.sport_type === sport)
          return match ? (match.value > 0 ? match.value : null) : null
        })
        const color = sportChartColor(sport)
        if (chartType === 'line') {
          return {
            name: label, type: 'line' as const, data: values, showSymbol: false,
            lineStyle: { width: 2, color }, itemStyle: { color },
            emphasis: { focus: 'series' as const }, connectNulls: false,
          }
        }
        return {
          name: label, type: 'bar' as const, stack: 'volume', data: values,
          itemStyle: { color }, emphasis: { focus: 'series' as const },
        }
      })

      const metricLabel = volumeMetric === 'distance' ? 'km' : volumeMetric === 'duration' ? 'h' : 'TSS'
      const metricFactor = volumeMetric === 'distance' ? 0.001 : volumeMetric === 'duration' ? 1 / 3600 : 1
      const metricDecimals = volumeMetric === 'tss' ? 0 : 1

      return {
        backgroundColor: 'transparent',
        grid: { left: 52, right: 20, top: 36, bottom: 50 },
        brush: lineXBrush,
        toolbox: {
          right: 8, top: 2,
          feature: {
            brush: lineXBrushToolboxFeature,
            restore: { title: 'Zurücksetzen' },
            saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2 },
          },
          iconStyle: { borderColor: '#6b7280' },
          emphasis: { iconStyle: { borderColor: '#d1d5db' } },
        },
        legend: { data: sports.map(s => sportLabel(s)), top: 4, textStyle: { fontSize: 10, color: '#9ca3af' }, itemWidth: 12, itemHeight: 8 },
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'cross' },
          confine: true,
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (params: unknown) => {
            const items = params as Array<{ seriesName: string; value: number | null; axisValueLabel: string; color: string }>
            if (!Array.isArray(items) || items.length === 0) return ''
            let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>`
            let total = 0
            for (const p of items) {
              if (p.value == null || p.value <= 0) continue
              const v = (p.value * metricFactor).toFixed(metricDecimals)
              html += `<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">
                <span style="display:inline-block;width:8px;height:8px;background:${p.color}"></span>
                <span style="color:#d1d5db">${p.seriesName}:</span>
                <strong style="color:#f9fafb">${v} ${metricLabel}</strong>
              </div>`
              total += p.value
            }
            const totalFormatted = (total * metricFactor).toFixed(metricDecimals)
            html += `<div style="margin-top:4px;padding-top:4px;border-top:1px solid #374151;color:#9ca3af;font-size:11px">
              Gesamt: <strong style="color:#f9fafb">${totalFormatted} ${metricLabel}</strong>
            </div>`
            return html
          },
        },
        xAxis: {
          type: 'category', data: periods,
          axisLabel: { fontSize: 10, color: '#6b7280', rotate: periods.length > 12 ? 30 : 0 },
          axisLine: { lineStyle: { color: '#374151' } },
        },
        yAxis: {
          type: 'value',
          axisLabel: {
            fontSize: 10, color: '#6b7280',
            formatter: (v: number) => {
              const converted = v * metricFactor
              return converted >= 1000 ? `${(converted / 1000).toFixed(0)}k` : converted.toFixed(0)
            },
          },
          splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
        },
        dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none' }],
        series,
      }
    },
    [data, volumeMetric, chartType],
    (chart) => {
      const periodCount = new Set((data ?? []).map(point => point.period)).size
      return bindLineXBrushZoom(chart, { dataZoomIndexes: [0], maxValue: Math.max(0, periodCount - 1) })
    },
  )

  const hasData = data && data.length > 0
  const hasTSSValues = volumeMetric !== 'tss' || (data ?? []).some(d => (d.value ?? 0) > 0)

  const showLoading = isLoading
  const showError = !isLoading && isError
  const showEmpty = !isLoading && !isError && !hasData
  const showTSSEmpty = !isLoading && !isError && hasData && volumeMetric === 'tss' && !hasTSSValues
  const showChart = !isLoading && !isError && hasData && hasTSSValues

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
          Trainingsvolumen
        </h3>
        <div className="flex items-center gap-2 flex-wrap">
          {/* Granularity toggle */}
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
          {/* Metric toggle */}
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
            <button
              onClick={() => setVolumeMetric('distance')}
              className={volumeMetric === 'distance'
                ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
            >
              Distanz
            </button>
            <button
              onClick={() => setVolumeMetric('duration')}
              className={volumeMetric === 'duration'
                ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
            >
              Dauer
            </button>
            <button
              onClick={() => setVolumeMetric('tss')}
              className={volumeMetric === 'tss'
                ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
            >
              TSS
            </button>
          </div>
          {/* Chart type toggle */}
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
            <button
              onClick={() => setChartType('bar')}
              title="Balkendiagramm"
              className={chartType === 'bar'
                ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
            >
              Balken
            </button>
            <button
              onClick={() => setChartType('line')}
              title="Liniendiagramm"
              className={chartType === 'line'
                ? 'px-2.5 py-1 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                : 'px-2.5 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'}
            >
              Linie
            </button>
          </div>
        </div>
      </div>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden">
        <div className="relative h-[280px]">
          {showLoading && (
            <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">
              Lade Trainingsvolumen…
            </div>
          )}
          {showError && (
            <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">
              Fehler beim Laden
            </div>
          )}
          {showEmpty && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-neutral-400">
              <span className="text-sm">Keine Daten vorhanden</span>
            </div>
          )}
          {showTSSEmpty && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 px-6 text-center text-neutral-400">
              <span className="text-sm font-medium">Keine TSS-Daten</span>
              <span className="text-xs leading-relaxed">
                Trainingsstress (TSS) wird aus Powermeter- oder HF-Daten berechnet — für die Aktivitäten in diesem Zeitraum sind keine TSS-Werte verfügbar.
              </span>
            </div>
          )}
          <div
            ref={containerRef}
            className={cn('w-full h-full', { invisible: !showChart })}
          />
        </div>
      </div>

      {/* Quick info */}
      {hasData && data && (
        <p className="text-xs text-neutral-400">
          {data.length} Datenpunkte — je Sport-Typ gestapelt
        </p>
      )}
    </div>
  )
}
