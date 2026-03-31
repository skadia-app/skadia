// Syncs ECharts mousemove events to the global comparison store:
// - setHoverTimestamp (elapsed seconds)
// - setHoverPoints (GPS positions for each source)

import { useRef, useCallback } from 'react'
import type * as echarts from 'echarts/core'
import { setHoverTimestamp, setHoverPoints, type HoverGpsPoint } from '@/components/comparison/useComparisonStore'
import type { GpsPoint } from './useStreamData'

type EChartsInstance = ReturnType<typeof echarts.init>

export interface SourceGpsData {
  color: string
  gpsPoints: GpsPoint[]
}

// Binary search — nearest GPS point for elapsed time
function findNearestGPS(gpsPoints: GpsPoint[], elapsed: number): GpsPoint | null {
  if (gpsPoints.length === 0) return null
  let lo = 0, hi = gpsPoints.length - 1
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (gpsPoints[mid].elapsed < elapsed) lo = mid + 1
    else hi = mid
  }
  if (lo > 0 && Math.abs(gpsPoints[lo - 1].elapsed - elapsed) < Math.abs(gpsPoints[lo].elapsed - elapsed)) lo--
  return gpsPoints[lo]
}

/**
 * Returns a ref-callback pair that attaches/detaches hover handlers to an ECharts container.
 *
 * Usage:
 *   const { attachHover, detachHover } = useHoverSync(sourcesGpsData)
 *   // in chart init effect:
 *   attachHover(containerEl, chart)
 *   // in cleanup:
 *   detachHover(containerEl)
 */
export function useHoverSync(sourcesGpsData: SourceGpsData[]) {
  const sourcesRef = useRef(sourcesGpsData)
  sourcesRef.current = sourcesGpsData

  const handlersRef = useRef<{
    move: (e: MouseEvent) => void
    leave: () => void
  } | null>(null)

  const attachHover = useCallback((container: HTMLElement, chart: EChartsInstance) => {
    const handleMouseMove = (e: MouseEvent) => {
      const rect = container.getBoundingClientRect()
      const elapsed = chart.convertFromPixel({ xAxisIndex: 0 }, e.clientX - rect.left)
      if (elapsed == null) return
      const t = elapsed as number
      setHoverTimestamp(t)

      const points: HoverGpsPoint[] = []
      for (const src of sourcesRef.current) {
        const gps = findNearestGPS(src.gpsPoints, t)
        if (gps) points.push({ lat: gps.lat, lng: gps.lng, color: src.color })
      }
      setHoverPoints(points.length > 0 ? points : null)
    }

    const handleMouseLeave = () => {
      setHoverTimestamp(null)
      setHoverPoints(null)
    }

    handlersRef.current = { move: handleMouseMove, leave: handleMouseLeave }
    container.addEventListener('mousemove', handleMouseMove)
    container.addEventListener('mouseleave', handleMouseLeave)
  }, [])

  const detachHover = useCallback((container: HTMLElement) => {
    if (!handlersRef.current) return
    container.removeEventListener('mousemove', handlersRef.current.move)
    container.removeEventListener('mouseleave', handlersRef.current.leave)
    handlersRef.current = null
    setHoverTimestamp(null)
    setHoverPoints(null)
  }, [])

  return { attachHover, detachHover }
}
