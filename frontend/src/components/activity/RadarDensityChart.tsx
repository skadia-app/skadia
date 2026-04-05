import { useMemo, useState } from 'react'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  VisualMapComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useECharts } from '@/hooks/useECharts'
import type { StreamPoint } from '@/api/types'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, VisualMapComponent, CanvasRenderer])

type XMode = 'distance' | 'time'

interface Props {
  streams: StreamPoint[]
}

export function RadarDensityChart({ streams }: Props) {
  const hasRadarData = useMemo(
    () => streams.some(s => s.radar_vehicle_count != null),
    [streams],
  )

  const [xMode, setXMode] = useState<XMode>('distance')

  const hasDistance = useMemo(() => streams.some(s => s.distance_m != null), [streams])

  const { xData, vehicleData, rangeData, hasRange } = useMemo(() => {
    const t0 = streams.length > 0 ? new Date(streams[0].time).getTime() : 0

    const xValues: number[] = []
    const vehicles: (number | null)[] = []
    const ranges: (number | null)[] = []
    let anyRange = false

    for (const s of streams) {
      if (s.radar_vehicle_count == null) continue

      if (xMode === 'distance') {
        xValues.push((s.distance_m ?? 0) / 1000)
      } else {
        xValues.push((new Date(s.time).getTime() - t0) / 60000)
      }

      vehicles.push(s.radar_vehicle_count)

      if (s.radar_nearest_range_m != null) {
        ranges.push(s.radar_nearest_range_m)
        anyRange = true
      } else {
        ranges.push(null)
      }
    }

    return { xData: xValues, vehicleData: vehicles, rangeData: ranges, hasRange: anyRange }
  }, [streams, xMode])

  const containerRef = useECharts(
    (theme) => {
      if (xData.length === 0) return { series: [] }

      // Build range series data as [x, y] pairs to support visualMap on the line
      const rangeSeriesData = xData.map((x, i) => [x, rangeData[i]] as [number, number | null])

      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const series: any[] = [
        {
          name: 'Vehicles',
          type: 'line',
          yAxisIndex: 0,
          data: xData.map((x, i) => [x, vehicleData[i]]),
          showSymbol: false,
          lineStyle: { width: 2, color: '#F97316' },
          itemStyle: { color: '#F97316' },
          areaStyle: { color: 'rgba(249,115,22,0.12)' },
          smooth: 0.2,
        },
      ]

      if (hasRange) {
        series.push({
          name: 'Nearest vehicle',
          type: 'line',
          yAxisIndex: 1,
          data: rangeSeriesData,
          showSymbol: false,
          lineStyle: { width: 2 },
          // visualMap will override the color
          smooth: 0.2,
          // encode dimension 1 = y value, used by visualMap
          encode: { x: 0, y: 1 },
        })
      }

      const xFormatter = xMode === 'distance'
        ? (v: number) => `${v.toFixed(2)} km`
        : (v: number) => {
            const h = Math.floor(v / 60)
            const m = Math.floor(v % 60)
            return h > 0 ? `${h}h ${m}m` : `${m} min`
          }

      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const visualMapPieces: any[] = hasRange
        ? [
            { lte: 10, color: '#ef4444', label: '<10 m' },
            { gt: 10, lte: 30, color: '#eab308', label: '10–30 m' },
            { gt: 30, color: '#22c55e', label: '>30 m' },
          ]
        : []

      return {
        backgroundColor: 'transparent',
        grid: {
          left: 48,
          right: hasRange ? 52 : 16,
          top: hasRange ? 44 : 28,
          bottom: 36,
        },
        legend: hasRange
          ? {
              data: ['Vehicles', 'Nearest vehicle'],
              top: 8,
              textStyle: { fontSize: 10, color: theme.textColor },
              itemHeight: 8,
              itemWidth: 14,
            }
          : undefined,
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'cross' },
          backgroundColor: theme.isDark ? 'rgba(17,24,39,0.92)' : 'rgba(255,255,255,0.95)',
          borderColor: theme.isDark ? '#374151' : '#e5e7eb',
          textStyle: { color: theme.isDark ? '#f3f4f6' : '#111827', fontSize: 12 },
          formatter: (params: unknown) => {
            const items = params as Array<{
              seriesName: string
              value: [number, number | null]
              color: string
            }>
            if (!Array.isArray(items) || items.length === 0) return ''
            const xLabel = xFormatter(items[0].value[0])
            let html = `<div style="font-size:11px;color:${theme.textColor};margin-bottom:4px">${xLabel}</div>`
            for (const p of items) {
              if (p.value[1] == null) continue
              const unit = p.seriesName === 'Nearest vehicle' ? ' m' : ''
              html += `<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">
                <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>
                <span>${p.seriesName}:</span>
                <strong>${p.value[1]}${unit}</strong>
              </div>`
            }
            return html
          },
        },
        xAxis: {
          type: 'value',
          min: 0,
          axisLabel: { fontSize: 10, color: theme.textColor, formatter: xFormatter },
          splitLine: { lineStyle: { color: theme.gridLineColor, type: 'dashed' } },
          axisLine: { lineStyle: { color: theme.gridLineColor } },
        },
        yAxis: [
          {
            type: 'value',
            name: 'Vehicles',
            nameTextStyle: { fontSize: 9, color: theme.textColor },
            axisLabel: {
              fontSize: 10,
              color: theme.textColor,
              formatter: (v: number) => String(Math.round(v)),
            },
            splitLine: { lineStyle: { color: theme.gridLineColor, type: 'dashed' } },
            minInterval: 1,
          },
          ...(hasRange
            ? [
                {
                  type: 'value',
                  name: 'Distance (m)',
                  nameTextStyle: { fontSize: 9, color: theme.textColor },
                  axisLabel: {
                    fontSize: 10,
                    color: theme.textColor,
                    formatter: (v: number) => `${v} m`,
                  },
                  splitLine: { show: false },
                  inverse: true,  // closer = higher on screen (smaller = more dangerous)
                },
              ]
            : []),
        ],
        visualMap: hasRange
          ? [
              {
                show: false,
                type: 'piecewise',
                seriesIndex: 1,
                dimension: 1,
                pieces: visualMapPieces,
                outOfRange: { color: theme.textColor },
              },
            ]
          : undefined,
        series,
      }
    },
    [xData, vehicleData, rangeData, hasRange, xMode],
  )

  if (!hasRadarData) return null

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
      <div className="flex items-center justify-between px-4 pt-3 pb-1">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          Radar — Vehicle Density
        </h3>
        {/* X-axis toggle — only show if distance data is available */}
        {hasDistance && (
          <div className="flex text-xs border border-neutral-200 dark:border-neutral-700 rounded overflow-hidden">
            <button
              className={`px-3 py-1 transition-colors cursor-pointer ${
                xMode === 'distance'
                  ? 'bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900'
                  : 'text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300'
              }`}
              onClick={() => setXMode('distance')}
            >
              Distance
            </button>
            <button
              className={`px-3 py-1 transition-colors border-l border-neutral-200 dark:border-neutral-700 cursor-pointer ${
                xMode === 'time'
                  ? 'bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900'
                  : 'text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300'
              }`}
              onClick={() => setXMode('time')}
            >
              Time
            </button>
          </div>
        )}
      </div>

      {/* Color legend for range zones */}
      {hasRange && (
        <div className="flex items-center gap-4 px-4 pb-1 text-xs text-neutral-500 dark:text-neutral-400">
          <span className="flex items-center gap-1">
            <span className="inline-block w-3 h-1.5 rounded-sm bg-red-500" />
            &lt;10 m
          </span>
          <span className="flex items-center gap-1">
            <span className="inline-block w-3 h-1.5 rounded-sm bg-yellow-500" />
            10–30 m
          </span>
          <span className="flex items-center gap-1">
            <span className="inline-block w-3 h-1.5 rounded-sm bg-green-500" />
            &gt;30 m
          </span>
        </div>
      )}

      <div ref={containerRef} className="h-[220px] w-full" />
    </div>
  )
}
