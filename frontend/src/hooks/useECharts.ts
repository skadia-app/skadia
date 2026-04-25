import { useRef, useEffect } from 'react'
import * as echarts from 'echarts/core'

export interface EChartsTheme {
  isDark: boolean
  textColor: string
  gridLineColor: string
}

export function getTheme(): EChartsTheme {
  const isDark = document.documentElement.classList.contains('dark')
  return {
    isDark,
    textColor: isDark ? '#9ca3af' : '#6b7280',
    gridLineColor: isDark ? '#262626' : '#f5f5f5',
  }
}

/**
 * Manages an ECharts instance: init once on mount, setOption on dep change,
 * dispose on unmount.
 *
 * @param getOption - Called with theme to produce chart options. Re-runs on dep change.
 * @param deps - Re-run setOption when these change.
 * @param onChart - Optional callback receiving the chart instance after init.
 *   Re-runs on dep change AFTER setOption — useful for re-binding event handlers
 *   that close over fresh state. Return optional cleanup fn (called before next
 *   onChart and on dispose).
 */
export function useECharts(
  getOption: (theme: EChartsTheme) => echarts.EChartsCoreOption,
  deps: React.DependencyList,
  onChart?: (chart: echarts.ECharts) => (() => void) | void,
) {
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)
  const onChartCleanupRef = useRef<(() => void) | null>(null)

  // Mount/unmount only — no re-init on dep change.
  useEffect(() => {
    if (!containerRef.current) return
    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    return () => {
      onChartCleanupRef.current?.()
      onChartCleanupRef.current = null
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [])

  // Update on dep change — only setOption + re-bind handlers, no teardown.
  useEffect(() => {
    const chart = chartRef.current
    if (!chart) return
    chart.setOption(getOption(getTheme()))
    onChartCleanupRef.current?.()
    const cleanup = onChart?.(chart)
    onChartCleanupRef.current = (cleanup ?? null) as (() => void) | null
  }, deps) // eslint-disable-line react-hooks/exhaustive-deps — NICHT ENTFERNEN: deps werden vom Caller übergeben, getOption ist absichtlich ausgeschlossen (sonst infinite Re-Renders bei inline Arrows)

  return containerRef
}
