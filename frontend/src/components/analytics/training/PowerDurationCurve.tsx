import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { getPowerCurve } from '@/api/activities'
import type { PDCParams } from '@/api/activities'
import type { PowerCurvePoint } from '@/api/types'
import { useECharts } from '@/hooks/useECharts'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const currentYear = new Date().getFullYear()

const DEFAULT_CURVES: Array<{ label: string; color: string; params: PDCParams }> = [
  { label: 'Dieses Jahr', color: '#F59E0B', params: { period: 'ytd' } },
  {
    label: 'Letztes Jahr',
    color: '#6366F1',
    params: {
      from: `${currentYear - 1}-01-01`,
      to: `${currentYear - 1}-12-31`,
    },
  },
  { label: 'Letzte 90 Tage', color: '#10B981', params: { period: '90d' } },
]

const DURATION_LABELS: Record<number, string> = {
  1: '1s',
  5: '5s',
  10: '10s',
  30: '30s',
  60: '1m',
  120: '2m',
  300: '5m',
  600: '10m',
  1200: '20m',
  1800: '30m',
  3600: '1h',
}

function formatDuration(s: number): string {
  return DURATION_LABELS[s] ?? `${s}s`
}

export default function PowerDurationCurve() {
  const query0 = useQuery({
    queryKey: ['power-curve', DEFAULT_CURVES[0].label, DEFAULT_CURVES[0].params],
    queryFn: () => getPowerCurve(DEFAULT_CURVES[0].params),
    staleTime: 5 * 60 * 1000,
  })
  const query1 = useQuery({
    queryKey: ['power-curve', DEFAULT_CURVES[1].label, DEFAULT_CURVES[1].params],
    queryFn: () => getPowerCurve(DEFAULT_CURVES[1].params),
    staleTime: 5 * 60 * 1000,
  })
  const query2 = useQuery({
    queryKey: ['power-curve', DEFAULT_CURVES[2].label, DEFAULT_CURVES[2].params],
    queryFn: () => getPowerCurve(DEFAULT_CURVES[2].params),
    staleTime: 5 * 60 * 1000,
  })

  const queries = [query0, query1, query2]
  const isLoading = queries.some(q => q.isLoading)
  const hasData = queries.some(q => (q.data?.points?.length ?? 0) > 0)

  const containerRef = useECharts(
    () => {
      if (!hasData) return { series: [] }

      const series = DEFAULT_CURVES.map((curve, i) => {
        const points = queries[i].data?.points
        if (!points?.length) return null
        return {
          name: curve.label,
          type: 'line' as const,
          smooth: true,
          symbol: 'none',
          data: points.map((p: PowerCurvePoint) => [p.duration_s, p.power_w]),
          lineStyle: { color: curve.color, width: 2 },
          itemStyle: { color: curve.color },
        }
      }).filter(Boolean)

      return {
        backgroundColor: 'transparent',
        grid: { left: 52, right: 20, top: 36, bottom: 40 },
        legend: {
          top: 4,
          textStyle: { fontSize: 10, color: '#9ca3af' },
          itemWidth: 16,
          itemHeight: 3,
        },
        tooltip: {
          trigger: 'axis',
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (params: unknown) => {
            const items = params as Array<{ seriesName: string; value: [number, number]; color: string }>
            if (!Array.isArray(items) || items.length === 0) return ''
            const dur = items[0].value[0]
            let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${formatDuration(dur)}</div>`
            for (const p of items) {
              html += `<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">
                <span style="display:inline-block;width:10px;height:2px;background:${p.color}"></span>
                <span style="color:#d1d5db">${p.seriesName}:</span>
                <strong style="color:#f9fafb">${Math.round(p.value[1])} W</strong>
              </div>`
            }
            return html
          },
        },
        xAxis: {
          type: 'log',
          min: 1,
          max: 3600,
          name: 'Dauer',
          nameTextStyle: { fontSize: 10, color: '#6b7280' },
          nameLocation: 'end',
          axisLabel: {
            fontSize: 10,
            color: '#6b7280',
            formatter: (value: number) => DURATION_LABELS[value] ?? '',
          },
          axisLine: { lineStyle: { color: '#374151' } },
          splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
        },
        yAxis: {
          type: 'value',
          name: 'Watt',
          nameTextStyle: { fontSize: 10, color: '#6b7280' },
          axisLabel: { fontSize: 10, color: '#6b7280', formatter: (v: number) => `${v}W` },
          splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
        },
        series,
      }
    },
    [query0.data, query1.data, query2.data, hasData], // eslint-disable-line react-hooks/exhaustive-deps
  )

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
        Power Duration Curve
      </h3>

      {isLoading ? (
        <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
          Lade Power-Kurven…
        </div>
      ) : !hasData ? (
        <div className="flex flex-col items-center justify-center h-[280px] gap-2 text-neutral-400">
          <span className="text-sm">Keine Leistungsdaten vorhanden</span>
          <span className="text-xs">Aktivitäten mit Leistungsmesser werden benötigt</span>
        </div>
      ) : (
        <div ref={containerRef} className="h-[280px] w-full" />
      )}
    </div>
  )
}
