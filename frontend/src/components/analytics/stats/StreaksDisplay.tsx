import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchStreaks } from '@/api/analytics'
import type { StreaksResponse } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { useECharts } from '@/hooks/useECharts'

echarts.use([BarChart, GridComponent, TooltipComponent, CanvasRenderer])

function fmtDate(dateStr: string | undefined): string {
  if (!dateStr) return '—'
  try {
    return new Date(dateStr).toLocaleDateString('de-DE', { year: 'numeric', month: 'short', day: 'numeric' })
  } catch {
    return dateStr
  }
}

interface StreakCardProps {
  label: string
  current: number
  record: number
  unit: string
  recordFrom?: string
  recordTo?: string
}

function StreakCard({ label, current, record, unit, recordFrom, recordTo }: StreakCardProps) {
  const isFire = current >= 7
  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4 space-y-3">
      <div className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">{label}</div>

      {/* Current streak */}
      <div className="flex items-baseline gap-2">
        {isFire && <span className="text-xl leading-none">🔥</span>}
        <div>
          <div className="text-3xl font-bold tabular-nums text-neutral-900 dark:text-neutral-100 leading-none">
            {current}
          </div>
          <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            aktuell {current === 1 ? unit.split('/')[0] : unit.split('/')[1] || unit.split('/')[0]}
          </div>
        </div>
      </div>

      {/* Record */}
      {record > 0 && (
        <div className="border-t border-neutral-100 dark:border-neutral-800 pt-2">
          <div className="flex items-center gap-1 text-xs text-amber-700 dark:text-amber-400">
            <span>🏆</span>
            <span className="font-semibold">Rekord: {record} {record === 1 ? unit.split('/')[0] : unit.split('/')[1] || unit.split('/')[0]}</span>
          </div>
          {(recordFrom || recordTo) && (
            <div className="text-xs text-neutral-400 mt-0.5">
              {fmtDate(recordFrom)} – {fmtDate(recordTo)}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

interface MiniBarChartProps {
  data: Array<{ count: number; from: string; to: string }>
}

function StreakHistoryChart({ data }: MiniBarChartProps) {
  const sorted = [...data].sort((a, b) => b.count - a.count).slice(0, 10)

  const containerRef = useECharts(
    () => ({
      grid: { left: 8, right: 8, top: 8, bottom: 24 },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const items = params as Array<{ dataIndex: number; value: number }>
          if (!Array.isArray(items) || items.length === 0) return ''
          const item = items[0]
          const d = sorted[item.dataIndex]
          return `<div style="font-size:11px">
            <div style="font-weight:600">${item.value} Tage</div>
            <div style="color:#9ca3af">${fmtDate(d.from)} – ${fmtDate(d.to)}</div>
          </div>`
        },
      },
      xAxis: {
        type: 'category',
        data: sorted.map((_, i) => `#${i + 1}`),
        axisLabel: { fontSize: 9, color: '#9ca3af' },
        axisTick: { show: false },
        axisLine: { show: false },
      },
      yAxis: {
        type: 'value',
        show: false,
      },
      series: [
        {
          type: 'bar',
          data: sorted.map((d, i) => ({
            value: d.count,
            itemStyle: {
              color: i === 0 ? '#F59E0B' : '#F97316',
              opacity: 1 - i * 0.08,
            },
          })),
          barCategoryGap: '20%',
          label: {
            show: true,
            position: 'top',
            fontSize: 9,
            color: '#6b7280',
          },
        },
      ],
    }),
    [data],
  )

  return <div ref={containerRef} className="h-24 w-full" />
}

export default function StreaksDisplay() {
  const { toQueryString } = useAnalyticsFilters()
  const params = toQueryString()

  const { data, isLoading, isError } = useQuery<StreaksResponse>({
    queryKey: ['streaks', params],
    queryFn: () => fetchStreaks(params),
    retry: 1,
  })

  if (isLoading) {
    return (
      <div className="space-y-3 animate-pulse">
        <div className="grid grid-cols-2 gap-3">
          {[0, 1].map(i => (
            <div key={i} className="h-32 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-100 dark:bg-neutral-800" />
          ))}
        </div>
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-400">
        Fehler beim Laden der Serien
      </div>
    )
  }

  const hasAnyStreak = (data.current_streak > 0) || (data.max_streak > 0) ||
    (data.current_week_streak > 0) || (data.max_week_streak > 0)

  if (!hasAnyStreak) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-8 text-center">
        <div className="text-3xl mb-3">🎯</div>
        <div className="text-sm font-medium text-neutral-600 dark:text-neutral-400">Noch keine Serien</div>
        <div className="text-xs text-neutral-400 mt-1">Trainiere an mehreren aufeinanderfolgenden Tagen, um Serien zu starten.</div>
      </div>
    )
  }

  const streakHistory = data.streak_history ?? []

  return (
    <div className="space-y-4">
      <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">Serien</h3>

      <div className="grid grid-cols-2 gap-3">
        <StreakCard
          label="Tages-Serie"
          current={data.current_streak}
          record={data.max_streak}
          unit="Tag/Tage"
        />
        <StreakCard
          label="Wochen-Serie"
          current={data.current_week_streak}
          record={data.max_week_streak}
          unit="Woche/Wochen"
        />
      </div>

      {/* Streak history chart */}
      {streakHistory.length > 0 && (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3">
          <div className="text-xs font-semibold text-neutral-600 dark:text-neutral-400 mb-2">Top Serien</div>
          <StreakHistoryChart data={streakHistory} />
        </div>
      )}
    </div>
  )
}
