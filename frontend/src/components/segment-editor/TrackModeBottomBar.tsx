// frontend/src/components/segment-editor/TrackModeBottomBar.tsx
import { useEffect, useMemo, useRef } from 'react'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, BrushComponent, ToolboxComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useRangeBrush } from '@/hooks/useRangeBrush'

echarts.use([LineChart, GridComponent, BrushComponent, ToolboxComponent, CanvasRenderer])

export interface TrackPoint {
  lat: number
  lng: number
  time?: string
  elevation?: number | null
}

export interface TrackModeBottomBarProps {
  trackPoints: TrackPoint[]
  range: [number, number]
  onRangeChange: (r: [number, number]) => void
  onCommitRange: (r: [number, number]) => void   // fires on mouse-up / keyboard-commit, triggers map fitBounds
  onReverse: () => void
  onReset: () => void
  onPreviewOrSave: () => void
  isPreview: boolean
}

export function TrackModeBottomBar(props: TrackModeBottomBarProps) {
  const { trackPoints, range, onRangeChange, onCommitRange, onReverse, onReset, onPreviewOrSave, isPreview } = props
  const chartRef = useRef<HTMLDivElement | null>(null)
  const echartsInstanceRef = useRef<echarts.ECharts | null>(null)

  const { bindEcharts, setStart, setEnd } = useRangeBrush({
    totalPoints: trackPoints.length,
    initial: range,
    onBrushChange: onRangeChange,
  })

  // Compute elevation series and cumulative distance-per-index for km labels.
  const { elevationSeries, hasElevation, cumulativeKm } = useMemo(() => {
    const elev = trackPoints.map(p => (p.elevation ?? null))
    const has = elev.some(e => e != null)
    const km: number[] = [0]
    for (let i = 1; i < trackPoints.length; i++) {
      const a = trackPoints[i - 1]; const b = trackPoints[i]
      const d = haversineKm(a.lat, a.lng, b.lat, b.lng)
      km.push(km[i - 1] + d)
    }
    return { elevationSeries: elev, hasElevation: has, cumulativeKm: km }
  }, [trackPoints])

  // Init ECharts.
  useEffect(() => {
    if (!chartRef.current) return
    const ec = echarts.init(chartRef.current, undefined, { renderer: 'canvas' })
    echartsInstanceRef.current = ec
    ec.setOption({
      grid: { top: 4, right: 8, bottom: 4, left: 8, containLabel: false },
      xAxis: {
        type: 'value',
        min: 0, max: trackPoints.length - 1,
        show: false,
      },
      yAxis: { type: 'value', show: false, min: hasElevation ? 'dataMin' : 0, max: hasElevation ? 'dataMax' : 1 },
      brush: {
        xAxisIndex: 0,
        throttleType: 'debounce', throttleDelay: 30,
        transformable: true,
        brushType: 'lineX',
        brushMode: 'single',
      },
      toolbox: { show: false },
      series: [{
        type: 'line',
        data: elevationSeries.map((y, i) => [i, y ?? 0]),
        showSymbol: false,
        lineStyle: { color: '#3b82f6', width: 1.5 },
        areaStyle: { color: 'rgba(59,130,246,0.15)' },
      }],
    })
    bindEcharts({
      on: ec.on.bind(ec),
      off: ec.off.bind(ec),
      dispatchAction: ec.dispatchAction.bind(ec),
    })
    return () => { ec.dispose(); echartsInstanceRef.current = null }
  }, [trackPoints, hasElevation, elevationSeries, bindEcharts])

  // Reflect external range changes back to the brush visual.
  useEffect(() => {
    const ec = echartsInstanceRef.current
    if (!ec) return
    ec.dispatchAction({
      type: 'brush',
      areas: [{ brushType: 'lineX', coordRange: [range[0], range[1]], xAxisIndex: 0 }],
    })
  }, [range])

  const startKm = cumulativeKm[range[0]]?.toFixed(1) ?? '0.0'
  const endKm = cumulativeKm[range[1]]?.toFixed(1) ?? '0.0'

  // Keyboard handler: arrow-keys on focused handle buttons.
  const startRef = useRef<HTMLButtonElement>(null)
  const endRef = useRef<HTMLButtonElement>(null)
  const handleKeyDown = (which: 'start' | 'end') => (e: React.KeyboardEvent) => {
    const cur = which === 'start' ? range[0] : range[1]
    const setter = which === 'start' ? setStart : setEnd
    const step = e.shiftKey ? 10 : (e.key === 'PageUp' || e.key === 'PageDown' ? 100 : 1)
    if (e.key === 'ArrowLeft' || e.key === 'PageUp') { e.preventDefault(); setter(cur - step); onCommitRange(which === 'start' ? [cur - step, range[1]] : [range[0], cur - step]) }
    if (e.key === 'ArrowRight' || e.key === 'PageDown') { e.preventDefault(); setter(cur + step); onCommitRange(which === 'start' ? [cur + step, range[1]] : [range[0], cur + step]) }
    if (e.key === 'Home') { e.preventDefault(); setter(0) }
    if (e.key === 'End') { e.preventDefault(); setter(trackPoints.length - 1) }
  }

  return (
    <div className="border-t border-neutral-800 bg-neutral-900 p-3">
      {/* Row 1 — Elevation brush */}
      <div className="mb-2">
        <div ref={chartRef} className="h-[50px] w-full" />
        {!hasElevation && (
          <div className="-mt-6 text-center text-xs text-neutral-500">Keine Höhendaten</div>
        )}
      </div>

      {/* Row 2 — Range slider. Use a dual-handle input via two native inputs. */}
      <div className="mb-2 flex items-center gap-2 text-xs text-neutral-400">
        <span className="w-16">km {startKm}</span>
        <div className="relative flex-1 h-6">
          <input
            type="range"
            min={0} max={trackPoints.length - 1}
            value={range[0]}
            onChange={e => setStart(Number(e.target.value))}
            onMouseUp={() => onCommitRange(range)}
            onKeyUp={() => onCommitRange(range)}
            className="absolute inset-0 w-full"
            aria-label="Start-Punkt"
            aria-valuemin={0} aria-valuemax={trackPoints.length - 1} aria-valuenow={range[0]}
          />
          <input
            type="range"
            min={0} max={trackPoints.length - 1}
            value={range[1]}
            onChange={e => setEnd(Number(e.target.value))}
            onMouseUp={() => onCommitRange(range)}
            onKeyUp={() => onCommitRange(range)}
            className="absolute inset-0 w-full"
            aria-label="Ende-Punkt"
            aria-valuemin={0} aria-valuemax={trackPoints.length - 1} aria-valuenow={range[1]}
          />
        </div>
        <span className="w-16 text-right">km {endKm}</span>
      </div>

      {/* Row 3 — Pfeil-Bar + Aktionen */}
      <div className="flex items-center gap-2 text-sm">
        <PillGroup label="Start" value={range[0]} onPrev={() => setStart(range[0] - 1)} onNext={() => setStart(range[0] + 1)} handleRef={startRef} onKeyDown={handleKeyDown('start')} />
        <PillGroup label="Ende"  value={range[1]} onPrev={() => setEnd(range[1] - 1)}  onNext={() => setEnd(range[1] + 1)}   handleRef={endRef}   onKeyDown={handleKeyDown('end')} />
        <span className="text-xs text-neutral-500 ml-2">← → fein · ⇧+← ⇧+→ ×10</span>
        <div className="flex-1" />
        <button type="button" onClick={onReverse} className="rounded bg-neutral-800 px-2 py-1 text-xs">⇄ Umkehren</button>
        <button type="button" onClick={onReset} className="rounded bg-neutral-800 px-2 py-1 text-xs">Reset</button>
        <button type="button" onClick={onPreviewOrSave} className="rounded bg-blue-600 px-3 py-1 text-xs font-medium text-white hover:bg-blue-500">
          {isPreview ? 'Speichern' : 'Vorschau'}
        </button>
      </div>
    </div>
  )
}

function PillGroup({ label, value, onPrev, onNext, handleRef, onKeyDown }: {
  label: string; value: number; onPrev: () => void; onNext: () => void;
  handleRef: React.RefObject<HTMLButtonElement>; onKeyDown: (e: React.KeyboardEvent) => void
}) {
  return (
    <div className="flex items-center gap-1 rounded bg-neutral-800 px-2 py-1">
      <span className="text-xs text-neutral-400">{label}</span>
      <button type="button" onClick={onPrev} aria-label={`${label} minus 1`}>◄</button>
      <button ref={handleRef} type="button" onKeyDown={onKeyDown} className="min-w-[3ch] text-center font-mono">{value}</button>
      <button type="button" onClick={onNext} aria-label={`${label} plus 1`}>►</button>
    </div>
  )
}

function haversineKm(lat1: number, lng1: number, lat2: number, lng2: number): number {
  const R = 6371
  const dLat = (lat2 - lat1) * Math.PI / 180
  const dLng = (lng2 - lng1) * Math.PI / 180
  const a = Math.sin(dLat/2)**2 + Math.cos(lat1*Math.PI/180)*Math.cos(lat2*Math.PI/180)*Math.sin(dLng/2)**2
  return 2 * R * Math.asin(Math.min(1, Math.sqrt(a)))
}
