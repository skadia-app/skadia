import { useRef, useEffect } from 'react'
import * as echarts from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  AxisPointerComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { StreamPoint } from '@/api/types'

echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, DataZoomComponent, AxisPointerComponent, CanvasRenderer])

interface Props {
  streams: StreamPoint[]
  onHover: (index: number | null) => void
}

interface ChartData {
  distanceKm: number[]
  altitude: number[]
  heartRate: number[]
  power: number[]
  cadence: number[]
  speed: number[]
}

function buildChartData(streams: StreamPoint[]): ChartData {
  const distanceKm: number[] = []
  const altitude: number[] = []
  const heartRate: number[] = []
  const power: number[] = []
  const cadence: number[] = []
  const speed: number[] = []

  for (const s of streams) {
    distanceKm.push(s.distance_m != null ? s.distance_m / 1000 : distanceKm.length > 0 ? distanceKm[distanceKm.length - 1] : 0)
    altitude.push(s.altitude_m ?? NaN)
    heartRate.push(s.heart_rate ?? NaN)
    power.push(s.power_w ?? NaN)
    cadence.push(s.cadence ?? NaN)
    speed.push(s.speed_ms != null ? s.speed_ms * 3.6 : NaN)
  }

  return { distanceKm, altitude, heartRate, power, cadence, speed }
}

function hasValidData(arr: number[]): boolean {
  return arr.some(v => !isNaN(v))
}

export default function ActivityCharts({ streams, onHover }: Props) {
  if (streams.length === 0) return null

  const data = buildChartData(streams)
  const charts: { id: string; label: string; component: React.ReactNode }[] = []

  if (hasValidData(data.altitude)) {
    charts.push({
      id: 'altitude',
      label: 'Höhenprofil',
      component: <ChartPanel key="altitude" chartId="altitude" data={data} field="altitude" label="Höhe (m)" color="#22c55e" area onHover={onHover} />,
    })
  }

  if (hasValidData(data.heartRate)) {
    charts.push({
      id: 'hr',
      label: 'Herzfrequenz',
      component: <ChartPanel key="hr" chartId="hr" data={data} field="heartRate" label="HR (bpm)" color="#ef4444" onHover={onHover} />,
    })
  }

  if (hasValidData(data.power)) {
    charts.push({
      id: 'power',
      label: 'Leistung',
      component: <ChartPanel key="power" chartId="power" data={data} field="power" label="Leistung (W)" color="#a855f7" onHover={onHover} />,
    })
  }

  if (hasValidData(data.cadence)) {
    charts.push({
      id: 'cadence',
      label: 'Kadenz',
      component: <ChartPanel key="cadence" chartId="cadence" data={data} field="cadence" label="Kadenz (rpm)" color="#f97316" onHover={onHover} />,
    })
  }

  if (hasValidData(data.speed)) {
    charts.push({
      id: 'speed',
      label: 'Geschwindigkeit',
      component: <ChartPanel key="speed" chartId="speed" data={data} field="speed" label="km/h" color="#3b82f6" onHover={onHover} />,
    })
  }

  if (charts.length === 0) return null

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Datenströme</h2>
      {charts.map(c => c.component)}
    </div>
  )
}

// ── Individual Chart Panel ────────────────────────────────────────────────────

type FieldKey = 'altitude' | 'heartRate' | 'power' | 'cadence' | 'speed'

function ChartPanel({ chartId, data, field, label, color, area, onHover }: {
  chartId: string
  data: ChartData
  field: FieldKey
  label: string
  color: string
  area?: boolean
  onHover: (index: number | null) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    const values = data[field]
    const xData = data.distanceKm

    const option: echarts.EChartsOption = {
      grid: {
        left: 50,
        right: 16,
        top: 24,
        bottom: 40,
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: {
          type: 'cross',
          label: { show: false },
          crossStyle: { color: '#999', type: 'dashed' },
        },
        formatter: (params: unknown) => {
          const p = Array.isArray(params) ? params[0] : params
          const typed = p as { dataIndex: number; value: [number, number] }
          if (!typed?.value) return ''
          return `${typed.value[0].toFixed(2)} km: <strong>${typed.value[1].toFixed(1)}</strong> ${label.split('(')[1]?.replace(')', '') ?? ''}`
        },
      },
      xAxis: {
        type: 'value',
        name: 'km',
        nameLocation: 'end',
        nameTextStyle: { fontSize: 11, color: '#999' },
        axisLabel: { fontSize: 11, color: '#999' },
        splitLine: { show: false },
      },
      yAxis: {
        type: 'value',
        name: label,
        nameLocation: 'end',
        nameTextStyle: { fontSize: 11, color: '#999' },
        axisLabel: { fontSize: 11, color: '#999' },
        splitLine: { lineStyle: { color: '#e5e5e5', type: 'dashed' } },
      },
      series: [
        {
          type: 'line',
          data: xData.map((x, i) => [x, values[i]]),
          showSymbol: false,
          lineStyle: { width: 1.5, color },
          itemStyle: { color },
          areaStyle: area ? { color: `${color}20` } : undefined,
          connectNulls: false,
          sampling: 'lttb',
        },
      ],
      dataZoom: [
        {
          type: 'inside',
          xAxisIndex: 0,
          filterMode: 'none',
        },
      ],
    }

    chart.setOption(option)

    // Hover callback for map sync
    chart.on('showTip', (params: unknown) => {
      const typed = params as { dataIndex?: number }
      if (typed.dataIndex != null) {
        onHover(typed.dataIndex)
      }
    })

    chart.on('hideTip', () => {
      onHover(null)
    })

    // Connect all charts in the group for axisPointer sync
    chart.group = 'activity-charts'
    echarts.connect('activity-charts')

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [data, field, label, color, area]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={containerRef} className="h-[200px] w-full" />
    </div>
  )
}
