import { useRef, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  TitleComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchDeveloperFields } from '@/api/activities'
import type { StreamPoint, DeveloperFieldInfo } from '@/api/types'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, TitleComponent, CanvasRenderer])

// ── Constants ──────────────────────────────────────────────────────────────────

const MEANINGFUL_UNITS = new Set([
  'Watts', 'kN/m', 'Milliseconds', 'Centimeters', 'bw/s', 'C', 'deg C', 'm/s', 'N', 'rpm', '',
])

// Skip GPS metadata fields that are numeric but meaningless as charts
const SKIP_KEYS = new Set(['Sat', 'hdop', 'idx', 'ehdop', 'evdop', 'vdop'])

const FIELD_COLORS: Record<string, string> = {
  'Form Power': '#f59e0b',
  'Air Power': '#0ea5e9',
  'Leg Spring Stiffness': '#10b981',
  'Power': '#a855f7',
  'Ground Time': '#f97316',
  'Vertical Oscillation': '#06b6d4',
  'Cadence': '#3b82f6',
  'ngp': '#8b5cf6',
}

// ECharts default palette for unknown fields
const DEFAULT_PALETTE = [
  '#5470c6', '#91cc75', '#fac858', '#ee6666', '#73c0de',
  '#3ba272', '#fc8452', '#9a60b4', '#ea7ccc',
]

// ── Helper: group fields by unit ───────────────────────────────────────────────

function groupByUnit(fields: DeveloperFieldInfo[]): Map<string, DeveloperFieldInfo[]> {
  const map = new Map<string, DeveloperFieldInfo[]>()
  for (const f of fields) {
    const existing = map.get(f.units)
    if (existing) {
      existing.push(f)
    } else {
      map.set(f.units, [f])
    }
  }
  return map
}

// ── Component ──────────────────────────────────────────────────────────────────

interface DeveloperFieldChartProps {
  streams: StreamPoint[]
  activityId: string
}

export function DeveloperFieldChart({ streams, activityId }: DeveloperFieldChartProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const instanceRef = useRef<echarts.ECharts | null>(null)

  const { data: devFieldInfos, isLoading, isError } = useQuery<DeveloperFieldInfo[]>({
    queryKey: ['developer-fields', activityId],
    queryFn: () => fetchDeveloperFields(activityId),
    staleTime: 5 * 60 * 1000,
  })

  // Filter to fields with meaningful units, excluding GPS metadata
  const fields = useMemo(() => {
    if (!devFieldInfos) return []
    return devFieldInfos.filter(f =>
      MEANINGFUL_UNITS.has(f.units) && !SKIP_KEYS.has(f.field_key)
    )
  }, [devFieldInfos])

  // Extract time labels and per-field values from streams
  const { timeLabels, fieldValues } = useMemo(() => {
    const t0 = streams.length > 0 ? new Date(streams[0].time).getTime() : 0
    const timeLabels = streams.map(s => {
      const elapsed = (new Date(s.time).getTime() - t0) / 60000
      const h = Math.floor(elapsed / 60)
      const m = Math.floor(elapsed % 60)
      const sec = Math.round((elapsed * 60) % 60)
      if (h > 0) return `${h}h${m.toString().padStart(2, '0')}`
      return `${m}:${sec.toString().padStart(2, '0')}`
    })
    const fieldValues = new Map<string, number[]>()
    for (const field of fields) {
      const vals = streams.map(s => {
        const v = s.developer_fields?.[field.field_key]
        if (v == null || typeof v !== 'number') return NaN
        // Allow negative values (e.g. temperature in °C) and zero (e.g. ground contact time)
        return isNaN(v) || !isFinite(v) ? NaN : v
      })
      fieldValues.set(field.field_key, vals)
    }
    return { timeLabels, fieldValues }
  }, [streams, fields])

  // Dispose only on unmount — never dispose on data changes
  useEffect(() => {
    return () => {
      instanceRef.current?.dispose()
      instanceRef.current = null
    }
  }, [])

  useEffect(() => {
    if (!containerRef.current) return
    if (fields.length === 0) return

    // Reuse existing instance; only init once per mount
    if (!instanceRef.current) {
      instanceRef.current = echarts.init(containerRef.current)
    }
    const chart = instanceRef.current

    const unitGroups = groupByUnit(fields)
    const yAxes: echarts.EChartsCoreOption['yAxis'] = []
    const fieldYAxisIndex = new Map<string, number>()

    let axisIdx = 0
    for (const [unit, groupFields] of unitGroups) {
      for (const f of groupFields) {
        fieldYAxisIndex.set(f.field_key, axisIdx)
      }
      yAxes.push({
        type: 'value',
        name: unit || '',
        nameTextStyle: { fontSize: 10, color: '#666' },
        axisLabel: { fontSize: 11, color: '#666', margin: 8 },
        splitLine: axisIdx === 0
          ? { lineStyle: { color: '#eee', type: 'dashed' } }
          : { show: false },
        axisLine: { show: false },
        axisTick: { show: false },
        position: axisIdx % 2 === 0 ? 'left' : 'right',
        offset: axisIdx > 1 ? Math.floor(axisIdx / 2) * 50 : 0,
      })
      axisIdx++
    }

    const series: echarts.EChartsCoreOption['series'] = []
    let colorIdx = 0
    for (const field of fields) {
      const vals = fieldValues.get(field.field_key) ?? []
      const color = FIELD_COLORS[field.field_key] ?? DEFAULT_PALETTE[colorIdx % DEFAULT_PALETTE.length]
      if (!FIELD_COLORS[field.field_key]) colorIdx++
      series.push({
        type: 'line',
        name: field.field_key,
        data: vals,
        showSymbol: false,
        lineStyle: { width: 1.5, color },
        itemStyle: { color },
        connectNulls: false,
        yAxisIndex: fieldYAxisIndex.get(field.field_key) ?? 0,
      })
    }

    const rightPad = unitGroups.size > 1 ? 76 : 52

    chart.setOption({
      animation: false,
      grid: { left: 52, right: rightPad, top: 36, bottom: 56 },
      title: {
        text: 'Entwickler-Felder',
        left: 4,
        top: 4,
        textStyle: { fontSize: 11, fontWeight: 500, color: '#888' },
      },
      tooltip: {
        trigger: 'axis',
        transitionDuration: 0,
        confine: true,
        backgroundColor: '#1f2937',
        borderColor: '#374151',
        borderWidth: 1,
        padding: [8, 10],
        textStyle: { fontSize: 12, color: '#f9fafb' },
        axisPointer: {
          type: 'line',
          lineStyle: { color: '#f97316', width: 1, type: 'dashed' },
          animation: false,
        },
        formatter: (params: unknown) => {
          const arr = Array.isArray(params) ? params : [params]
          if (!arr.length) return ''
          const idx = (arr[0] as { dataIndex?: number }).dataIndex
          if (idx == null) return ''
          let html = `<div style="line-height:1.75;min-width:150px">`
          html += `<div style="color:#888;font-size:11px;margin-bottom:2px">${timeLabels[idx] ?? ''}</div>`
          for (const p of arr) {
            const tp = p as { seriesName?: string; color?: string; value?: number }
            if (tp.value == null || isNaN(tp.value as number)) continue
            const field = fields.find(f => f.field_key === tp.seriesName)
            const unit = field?.units ?? ''
            const color = FIELD_COLORS[tp.seriesName ?? ''] ?? tp.color ?? '#888'
            html += `<div style="font-size:13px;color:${color}"><strong>${(tp.value as number).toFixed(1)}\u202f${unit}</strong> ${tp.seriesName}</div>`
          }
          return html + '</div>'
        },
      },
      legend: {
        type: 'scroll',
        bottom: 4,
        textStyle: { fontSize: 11, color: '#666' },
        pageTextStyle: { color: '#666' },
      },
      xAxis: {
        type: 'category',
        data: timeLabels,
        axisLabel: {
          fontSize: 11,
          color: '#666',
          interval: Math.max(1, Math.floor(timeLabels.length / 8)),
        },
        splitLine: { show: false },
        axisLine: { lineStyle: { color: '#ddd' } },
        axisTick: { lineStyle: { color: '#ddd' } },
      },
      yAxis: yAxes,
      series,
      dataZoom: [{
        type: 'inside',
        xAxisIndex: 0,
        filterMode: 'none',
        zoomOnMouseWheel: true,
        moveOnMouseMove: true,
        moveOnMouseWheel: false,
      }],
    } as echarts.EChartsCoreOption)

    const ro = new ResizeObserver(() => chart.resize())
    ro.observe(containerRef.current)

    return () => {
      ro.disconnect()
      // Do not dispose here — instance is reused across re-renders
    }
  }, [fields, fieldValues, timeLabels]) // eslint-disable-line react-hooks/exhaustive-deps

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-500 dark:text-neutral-400">
        Lade Entwickler-Felder…
      </div>
    )
  }

  if (isError) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-500 dark:text-neutral-400">
        Fehler beim Laden der Entwickler-Felder.
      </div>
    )
  }

  if (fields.length === 0) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-500 dark:text-neutral-400">
        Keine darstellbaren Entwickler-Felder vorhanden.
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={containerRef} className="h-[240px] w-full" />
    </div>
  )
}
