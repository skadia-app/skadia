import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchHealthBloodPressure } from '@/api/analytics'
import { toLocalISO } from '@/lib/dateUtils'
import { useECharts } from '@/hooks/useECharts'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, CanvasRenderer])

type TimePreset = '30d' | '90d' | '1y' | 'all'

const presetLabels: Record<TimePreset, string> = {
  '30d': '30T',
  '90d': '90T',
  '1y': '1J',
  'all': 'Alle',
}

function getDateRange(preset: TimePreset): { from: string; to: string } {
  const to = new Date()
  const from = new Date()
  switch (preset) {
    case '30d': from.setDate(from.getDate() - 30); break
    case '90d': from.setDate(from.getDate() - 90); break
    case '1y': from.setFullYear(from.getFullYear() - 1); break
    case 'all': return { from: '0001-01-01', to: toLocalISO(to) }
  }
  return {
    from: toLocalISO(from),
    to: toLocalISO(to),
  }
}

export default function BloodPressureChart() {
  const [preset, setPreset] = useState<TimePreset>('90d')

  const { from, to } = useMemo(() => getDateRange(preset), [preset])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['health', 'blood-pressure', from, to],
    queryFn: () => fetchHealthBloodPressure(from, to),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const containerRef = useECharts(
    () => {
      if (!data || data.length === 0) return { series: [] }

      const byDate = new Map<string, { sysSum: number; diaSum: number; pulseSum: number; count: number; pulseCount: number }>()
      for (const entry of data) {
        if (entry.systolic == null && entry.diastolic == null) continue
        const date = entry.measured_at_utc.split('T')[0]
        const existing = byDate.get(date)
        if (existing) {
          if (entry.systolic != null) { existing.sysSum += entry.systolic; existing.count++ }
          if (entry.diastolic != null) existing.diaSum += entry.diastolic
          if (entry.pulse != null) { existing.pulseSum += entry.pulse; existing.pulseCount++ }
        } else {
          byDate.set(date, {
            sysSum: entry.systolic ?? 0,
            diaSum: entry.diastolic ?? 0,
            pulseSum: entry.pulse ?? 0,
            count: entry.systolic != null ? 1 : 0,
            pulseCount: entry.pulse != null ? 1 : 0,
          })
        }
      }

      const dates = Array.from(byDate.keys()).sort()
      const systolicValues = dates.map(d => {
        const e = byDate.get(d)!
        return e.count > 0 ? Math.round(e.sysSum / e.count) : null
      })
      const diastolicValues = dates.map(d => {
        const e = byDate.get(d)!
        return e.count > 0 ? Math.round(e.diaSum / e.count) : null
      })
      const pulseValues = dates.map(d => {
        const e = byDate.get(d)!
        return e.pulseCount > 0 ? Math.round(e.pulseSum / e.pulseCount) : null
      })

      const hasPulse = pulseValues.some(v => v != null)

      return {
        backgroundColor: 'transparent',
        grid: { left: 52, right: 20, top: 44, bottom: 40 },
        legend: {
          data: hasPulse ? ['Systolisch', 'Diastolisch', 'Puls'] : ['Systolisch', 'Diastolisch'],
          top: 8,
          textStyle: { fontSize: 10, color: '#9ca3af' },
          itemHeight: 8,
          itemWidth: 14,
        },
        tooltip: {
          trigger: 'axis',
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (params: unknown) => {
            const items = params as Array<{ seriesName: string; value: number | null; axisValueLabel: string; color: string }>
            if (!Array.isArray(items) || items.length === 0) return ''
            let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>`
            for (const p of items) {
              if (p.value == null) continue
              const unit = p.seriesName === 'Puls' ? ' bpm' : ' mmHg'
              html += `<div style="display:flex;align-items:center;gap:6px">
                <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>
                <span style="color:#d1d5db">${p.seriesName}:</span>
                <strong style="color:#f9fafb">${p.value}${unit}</strong>
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
          axisLabel: { fontSize: 10, color: '#6b7280' },
          splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
        },
        dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none' }],
        series: [
          {
            name: 'Systolisch',
            type: 'line',
            data: systolicValues,
            showSymbol: true,
            symbolSize: 6,
            smooth: 0.3,
            lineStyle: { width: 2, color: '#ef4444' },
            itemStyle: { color: '#ef4444' },
          },
          {
            name: 'Diastolisch',
            type: 'line',
            data: diastolicValues,
            showSymbol: true,
            symbolSize: 6,
            smooth: 0.3,
            lineStyle: { width: 2, color: '#3b82f6' },
            itemStyle: { color: '#3b82f6' },
          },
          ...(hasPulse
            ? [
                {
                  name: 'Puls',
                  type: 'line' as const,
                  data: pulseValues,
                  showSymbol: false,
                  smooth: 0.3,
                  lineStyle: { width: 1.5, color: '#a855f7', type: 'dashed' as const },
                  itemStyle: { color: '#a855f7' },
                },
              ]
            : []),
        ],
      }
    },
    [data],
  )

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          Blutdruck
        </h2>
        <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
          {(Object.keys(presetLabels) as TimePreset[]).map(p => (
            <button
              key={p}
              onClick={() => setPreset(p)}
              className={
                p === preset
                  ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                  : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'
              }
            >
              {presetLabels[p]}
            </button>
          ))}
        </div>
      </div>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        {isLoading ? (
          <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
            Lade Blutdruckdaten...
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
            Fehler beim Laden der Blutdruckdaten
          </div>
        ) : !data || data.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-[280px] gap-3 text-neutral-400">
            <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z" />
            </svg>
            <span className="text-sm">Keine Blutdruckdaten vorhanden</span>
          </div>
        ) : (
          <div ref={containerRef} className="h-[280px] w-full" />
        )}
      </div>
    </div>
  )
}
