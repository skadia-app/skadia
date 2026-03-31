// Converts an ECharts brush-end event into a time range and updates
// the comparison store. Also manages the zoom reset state.

import { useRef, useCallback } from 'react'
import type * as echarts from 'echarts/core'
import { setTimeRange } from '@/components/comparison/useComparisonStore'

type EChartsInstance = ReturnType<typeof echarts.init>

interface BrushArea {
  range?: [number, number]
  coordRange?: [number, number]
}

export interface BrushZoomControls {
  bindBrushZoom: (chart: EChartsInstance, xMax: number) => void
  resetZoom: (chart: EChartsInstance) => void
}

/**
 * Returns helpers that wire brush events to datazoom + store time range.
 * Usage:
 *   const { bindBrushZoom, resetZoom } = useBrushZoom(setIsZoomed)
 *   // after chart option set: bindBrushZoom(chart, xMax)
 */
export function useBrushZoom(
  onZoomChange: (zoomed: boolean) => void,
): BrushZoomControls {
  const lockRef = useRef(false)

  const bindBrushZoom = useCallback((chart: EChartsInstance, xMax: number) => {
    // Activate brush cursor
    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })

    chart.off('brushEnd')
    chart.off('dataZoom')

    chart.on('brushEnd', (params: unknown) => {
      if (lockRef.current) return
      lockRef.current = true

      const p = params as { areas?: BrushArea[]; batch?: Array<{ areas?: BrushArea[] }> }
      const areas = p.areas ?? p.batch?.[0]?.areas
      if (!areas || areas.length === 0) { lockRef.current = false; return }

      const area = areas[0]
      let startData: number, endData: number

      if (area.coordRange) {
        ;[startData, endData] = area.coordRange
      } else if (area.range) {
        const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
        const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
        if (s == null || e == null) { lockRef.current = false; return }
        startData = s as number
        endData = e as number
      } else { lockRef.current = false; return }

      if (endData <= startData) { lockRef.current = false; return }
      const startPct = Math.max(0, (Math.max(0, startData) / xMax) * 100)
      const endPct = Math.min(100, (Math.min(xMax, endData) / xMax) * 100)
      if (endPct - startPct < 0.5) { lockRef.current = false; return }

      chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: startPct, end: endPct })
      chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: false },
      })

      // Update global time range (seconds)
      setTimeRange([startData, endData])
      onZoomChange(true)
      requestAnimationFrame(() => { lockRef.current = false })
    })

    chart.on('dataZoom', (params: unknown) => {
      const p = params as { batch?: Array<{ start: number; end: number }>; start?: number; end?: number }
      const z = p.batch?.[0] ?? (p.start != null && p.end != null ? p as { start: number; end: number } : null)
      if (z == null) return
      const isZoomed = z.start > 0.5 || z.end < 99.5
      onZoomChange(isZoomed)
      if (!isZoomed) setTimeRange(null)
    })
  }, [onZoomChange])

  const resetZoom = useCallback((chart: EChartsInstance) => {
    chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })
    setTimeRange(null)
    onZoomChange(false)
  }, [onZoomChange])

  return { bindBrushZoom, resetZoom }
}
