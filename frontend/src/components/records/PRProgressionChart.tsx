import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { ScatterChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useECharts } from '@/hooks/useECharts'
import { fetchPRHistory, type PRHistoryEntry } from '@/api/bestEfforts'
import { fmtDuration } from '@/lib/format'
import { categoryLabel } from '@/lib/bestEffortLabels'

echarts.use([ScatterChart, GridComponent, TooltipComponent, CanvasRenderer])

// ── Rank colors ────────────────────────────────────────────────────────────

/** Color per rank: gold / silver / bronze / gray. */
function rankColor(rank: number): string {
  if (rank === 1) return '#f59e0b'   // gold
  if (rank === 2) return '#94a3b8'   // silver
  if (rank === 3) return '#d97706'   // bronze
  return '#6b7280'                   // rest
}

// ── Value formatting ───────────────────────────────────────────────────────

/** Format a y-axis value for display. Power categories → Watts, otherwise → mm:ss. */
function formatYValue(category: string, value: number): string {
  if (category.startsWith('power_')) return `${Math.round(value)} W`
  return fmtDuration(value)
}

// ── Props ──────────────────────────────────────────────────────────────────

interface PRProgressionChartProps {
  /** Best-effort category key, e.g. "dist_5km", "power_5min" */
  category: string
  /** Sport group, e.g. "running", "cycling" */
  sportGroup: string
  /** Only fetch and render when this is true (lazy loading). */
  isExpanded: boolean
  height?: number
}

// ── Component ──────────────────────────────────────────────────────────────

export function PRProgressionChart({
  category,
  sportGroup,
  isExpanded,
  height = 220,
}: PRProgressionChartProps) {
  const navigate = useNavigate()

  const { data: history = [], isLoading, isError } = useQuery({
    queryKey: ['pr-history', category, sportGroup],
    queryFn: () => fetchPRHistory(category, sportGroup),
    enabled: isExpanded,
    staleTime: 5 * 60 * 1000,
  })

  // Build scatter data sorted by date for the chart
  const scatterData = useMemo<Array<[number, number, PRHistoryEntry]>>(() => {
    return history
      .map((entry): [number, number, PRHistoryEntry] | null => {
        const yVal = category.startsWith('power_') ? entry.value_numeric : entry.elapsed_time_s
        if (yVal == null) return null
        const ts = new Date(entry.achieved_at).getTime()
        return [ts, yVal, entry]
      })
      .filter((d): d is [number, number, PRHistoryEntry] => d !== null)
      .sort((a, b) => a[0] - b[0])
  }, [history, category])

  const containerRef = useECharts(
    (theme) => ({
      backgroundColor: 'transparent',
      tooltip: {
        trigger: 'item' as const,
        backgroundColor: 'rgba(17,24,39,0.95)',
        borderColor: '#374151',
        textStyle: { color: '#f3f4f6', fontSize: 11 },
        formatter: (params: { data: [number, number, PRHistoryEntry] }) => {
          const [ts, yVal, entry] = params.data
          const date = new Date(ts).toLocaleDateString('de-DE', {
            day: '2-digit', month: 'short', year: 'numeric',
          })
          const val = formatYValue(category, yVal)
          const name = entry.activity_name ? `<br/><span style="opacity:0.7">${entry.activity_name}</span>` : ''
          const rankBadge = entry.rank <= 3
            ? `<span style="color:${rankColor(entry.rank)};font-weight:600"> #${entry.rank}</span>`
            : ` #${entry.rank}`
          return `${date}${rankBadge}<br/><b>${val}</b>${name}`
        },
      },
      grid: { left: 58, right: 16, top: 12, bottom: 36 },
      xAxis: {
        type: 'time' as const,
        axisLabel: {
          color: theme.textColor,
          fontSize: 10,
          formatter: (val: number) =>
            new Date(val).toLocaleDateString('de-DE', { month: 'short', year: '2-digit' }),
        },
        axisLine: { lineStyle: { color: '#374151' } },
        splitLine: { show: false },
      },
      yAxis: {
        type: 'value' as const,
        inverse: !category.startsWith('power_'), // lower time = better → invert for time-based
        axisLabel: {
          color: theme.textColor,
          fontSize: 10,
          formatter: (val: number) => formatYValue(category, val),
        },
        splitLine: {
          lineStyle: { color: theme.gridLineColor, type: 'dashed' as const },
        },
      },
      series: [
        {
          type: 'scatter' as const,
          data: scatterData,
          symbolSize: (data: [number, number, PRHistoryEntry]) => (data[2].rank <= 3 ? 10 : 7),
          itemStyle: {
            color: (params: { data: [number, number, PRHistoryEntry] }) =>
              rankColor(params.data[2].rank),
            opacity: 0.9,
          },
        },
      ],
    }),
    [scatterData, category],
    (chart) => {
      const handler = (params: { data: [number, number, PRHistoryEntry] }) => {
        const activityId = params.data?.[2]?.activity_id
        if (activityId) navigate(`/activities/${activityId}`)
      }
      chart.on('click', handler)
      return () => chart.off('click', handler)
    },
  )

  // ── Render states ──────────────────────────────────────────────────────

  if (!isExpanded) return null

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8 text-xs text-neutral-400">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-neutral-300 border-t-orange-500 mr-2" />
        Lade Progression…
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex items-center justify-center py-6 text-xs text-rose-500 dark:text-rose-400">
        Fehler beim Laden der Verlaufsdaten
      </div>
    )
  }

  if (scatterData.length === 0) {
    return (
      <div className="flex items-center justify-center py-6 text-xs text-neutral-400 dark:text-neutral-600">
        Keine Verlaufsdaten für {categoryLabel(category)}
      </div>
    )
  }

  return (
    <div className="px-4 pb-4 pt-2">
      {/* Legend */}
      <div className="flex items-center gap-4 mb-2">
        {[
          { rank: 1, label: '#1' },
          { rank: 2, label: '#2' },
          { rank: 3, label: '#3' },
          { rank: 4, label: 'Rest' },
        ].map(({ rank, label }) => (
          <span key={rank} className="flex items-center gap-1 text-[10px] text-neutral-500 dark:text-neutral-500">
            <span
              className="w-2 h-2 rounded-full inline-block"
              style={{ backgroundColor: rankColor(rank) }}
            />
            {label}
          </span>
        ))}
        <span className="ml-auto text-[10px] text-neutral-400 dark:text-neutral-600">
          {scatterData.length} Einträge · Klick → Aktivität
        </span>
      </div>
      <div ref={containerRef} style={{ width: '100%', height }} className="cursor-pointer" />
    </div>
  )
}
