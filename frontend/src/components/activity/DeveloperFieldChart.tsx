import React, { useRef, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  ToolboxComponent,
  BrushComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchDeveloperFields } from '@/api/activities'
import type { StreamPoint, DeveloperFieldInfo } from '@/api/types'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, ToolboxComponent, BrushComponent, CanvasRenderer])

// ── Constants ──────────────────────────────────────────────────────────────────

const MEANINGFUL_UNITS = new Set([
  'Watts', 'kN/m', 'Milliseconds', 'Centimeters', 'bw/s', 'C', 'deg C', 'm/s', 'N', 'rpm', '',
])

// Skip GPS metadata fields that are numeric but meaningless as charts.
// "Effort Pace" (Coros) is handled in the Pace chart alongside Garmin GAP / Stryd NGP.
const SKIP_KEYS = new Set(['Sat', 'hdop', 'idx', 'ehdop', 'evdop', 'vdop', 'Effort Pace'])

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
  hoverIdx?: number | null
  onHover?: (idx: number | null) => void
  onRegister?: (id: string, chart: echarts.ECharts) => void
  onUnregister?: (id: string) => void
  onBrushZoom?: (startPct: number, endPct: number) => void
  brushZoomLockRef?: React.MutableRefObject<boolean>
}

export function DeveloperFieldChart({
  streams, activityId, hoverIdx, onHover, onRegister, onUnregister, onBrushZoom, brushZoomLockRef,
}: DeveloperFieldChartProps) {
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
    return devFieldInfos.filter(f => {
      const displayName = f.field_name ?? f.field_key
      return MEANINGFUL_UNITS.has(f.units) && !SKIP_KEYS.has(f.field_key) && !SKIP_KEYS.has(displayName)
    })
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

  // ResizeObserver: mount once, tied to container lifetime
  useEffect(() => {
    if (!containerRef.current) return
    const ro = new ResizeObserver(() => instanceRef.current?.resize())
    ro.observe(containerRef.current)
    return () => ro.disconnect()
  }, [])

  // Dispose only on unmount — never dispose on data changes
  useEffect(() => {
    return () => {
      onUnregister?.('developer-fields')
      instanceRef.current?.dispose()
      instanceRef.current = null
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!containerRef.current) return
    if (fields.length === 0) return

    // Reuse existing instance; only init once per mount
    if (!instanceRef.current) {
      instanceRef.current = echarts.init(containerRef.current)
      onRegister?.('developer-fields', instanceRef.current)
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
      const displayName = field.field_name ?? field.field_key
      const vals = fieldValues.get(field.field_key) ?? []
      const color = FIELD_COLORS[displayName] ?? DEFAULT_PALETTE[colorIdx % DEFAULT_PALETTE.length]
      if (!FIELD_COLORS[displayName]) colorIdx++
      series.push({
        type: 'line',
        name: displayName,
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
      grid: { left: 52, right: rightPad, top: 8, bottom: 56 },
      toolbox: {
        feature: {
          brush: { type: ['lineX'] },
          saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2, name: 'Entwickler-Felder' },
        },
      },
      brush: {
        toolbox: ['lineX'],
        xAxisIndex: 0,
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
            const field = fields.find(f => (f.field_name ?? f.field_key) === tp.seriesName)
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

    chart.off('showTip')
    chart.off('hideTip')
    chart.off('brushEnd')

    chart.on('showTip', (params: unknown) => {
      const p = params as { dataIndex?: number }
      if (p.dataIndex != null) onHover?.(p.dataIndex)
    })
    chart.on('hideTip', () => onHover?.(null))

    chart.on('brushEnd', (params: unknown) => {
      if (brushZoomLockRef?.current) return
      type BrushArea = { range?: [number, number]; coordRange?: [number, number] }
      const p = params as { areas?: BrushArea[]; batch?: Array<{ areas?: BrushArea[] }> }
      const areas = p.areas ?? p.batch?.[0]?.areas
      const area = areas?.[0]
      if (!area) return
      const total = streams.length
      if (total === 0) return

      let startIdx: number, endIdx: number
      if (area.coordRange) {
        ;[startIdx, endIdx] = area.coordRange
      } else if (area.range) {
        // category xAxis: convertFromPixel returns the integer category index
        const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
        const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
        if (s == null || e == null) return
        startIdx = s as number
        endIdx = e as number
      } else {
        return
      }

      if (endIdx <= startIdx) return
      if (brushZoomLockRef) brushZoomLockRef.current = true
      onBrushZoom?.(
        Math.max(0, (startIdx / total) * 100),
        Math.min(100, (endIdx / total) * 100),
      )
      chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
      chart.dispatchAction({ type: 'takeGlobalCursor', key: 'brush', brushOption: { brushType: false } })
      requestAnimationFrame(() => { if (brushZoomLockRef) brushZoomLockRef.current = false })
    })

    return () => {
      // Do not dispose here — instance is reused across re-renders
    }
  }, [fields, fieldValues, timeLabels]) // eslint-disable-line react-hooks/exhaustive-deps

  // Sync tooltip position with other charts via hoverIdx
  useEffect(() => {
    const chart = instanceRef.current
    if (!chart) return
    if (hoverIdx == null) {
      chart.dispatchAction({ type: 'hideTip' })
    } else {
      chart.dispatchAction({ type: 'showTip', seriesIndex: 0, dataIndex: hoverIdx })
    }
  }, [hoverIdx])

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
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800/60">
        <span className="w-2 h-2 rounded-full shrink-0 bg-violet-500" />
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Entwickler-Felder</span>
      </div>
      <div ref={containerRef} className="h-[240px] w-full" />
    </div>
  )
}
