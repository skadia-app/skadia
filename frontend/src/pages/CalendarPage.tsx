import { useState, useMemo, useRef, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { format, startOfWeek, endOfWeek, addWeeks, subWeeks, isToday, isSameDay, getISOWeek } from 'date-fns'
import { de } from 'date-fns/locale'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { HeatmapChart } from 'echarts/charts'
import { CalendarComponent, TooltipComponent, VisualMapComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchActivities } from '@/api/activities'
import { fetchPMC } from '@/api/stats'
import type { Activity } from '@/api/types'
import type { PMCDataPoint } from '@/api/stats'
import { sportColor, sportLabel } from '@/lib/sportTaxonomy'
import { SportIcon } from '@/components/SportIcon'
import { formatDuration, formatDistance } from '@/lib/utils'

echarts.use([HeatmapChart, CalendarComponent, TooltipComponent, VisualMapComponent, CanvasRenderer])

type WeekCount = 4 | 8 | 12
type ViewTab = 'weeks' | 'year'

const DAY_LABELS = ['Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa', 'So']
const MAX_CARDS_PER_CELL = 3

// ── Helpers ───────────────────────────────────────────────────────────────────

function dateKey(d: Date): string {
  return format(d, 'yyyy-MM-dd')
}

function formatWeeklyTime(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return `${h}:${m.toString().padStart(2, '0')}`
}

function formatWeeklyDistance(meters: number): string {
  if (meters < 1000) return `${Math.round(meters)} m`
  return `${(meters / 1000).toFixed(1)} km`
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function CalendarPage() {
  const navigate = useNavigate()
  const [weekCount, setWeekCount] = useState<WeekCount>(8)
  const [tab, setTab] = useState<ViewTab>('weeks')
  // anchorDate = the Monday of the "most recent" week shown.
  // We start with "this week's Monday" so the current week is visible.
  const [anchorDate, setAnchorDate] = useState<Date>(() =>
    startOfWeek(new Date(), { weekStartsOn: 1 }),
  )

  // Computed date range for the grid
  const { gridStart, gridEnd } = useMemo(() => {
    const end = endOfWeek(anchorDate, { weekStartsOn: 1 })
    const start = startOfWeek(subWeeks(anchorDate, weekCount - 1), { weekStartsOn: 1 })
    return { gridStart: start, gridEnd: end }
  }, [anchorDate, weekCount])

  const fromStr = dateKey(gridStart)
  const toStr = dateKey(gridEnd)

  // ── Fetch Activities ──────────────────────────────────────────────────────
  const { data: activitiesData } = useQuery({
    queryKey: ['calendar-activities', fromStr, toStr],
    queryFn: () => fetchActivities(undefined, 500, 'started_at', 'desc', {
      dateFrom: fromStr,
      dateTo: toStr,
    }),
  })

  const activities = activitiesData?.activities ?? []

  // Group activities by date
  const activitiesByDate = useMemo(() => {
    const map = new Map<string, Activity[]>()
    for (const a of activities) {
      const key = a.started_at.slice(0, 10)
      const list = map.get(key)
      if (list) list.push(a)
      else map.set(key, [a])
    }
    return map
  }, [activities])

  // ── Fetch PMC data ────────────────────────────────────────────────────────
  const { data: pmcData } = useQuery({
    queryKey: ['calendar-pmc', fromStr, toStr],
    queryFn: () => fetchPMC(fromStr, toStr),
  })

  const pmcByDate = useMemo(() => {
    const map = new Map<string, PMCDataPoint>()
    if (pmcData) {
      for (const p of pmcData) {
        map.set(p.date, p)
      }
    }
    return map
  }, [pmcData])

  // ── Build weeks array ─────────────────────────────────────────────────────
  const weeks = useMemo(() => {
    const result: Date[][] = []
    let current = new Date(gridStart)
    for (let w = 0; w < weekCount; w++) {
      const days: Date[] = []
      for (let d = 0; d < 7; d++) {
        days.push(new Date(current))
        current.setDate(current.getDate() + 1)
      }
      result.push(days)
    }
    return result
  }, [gridStart, weekCount])

  // ── Weekly summaries ──────────────────────────────────────────────────────
  const weekSummaries = useMemo(() => {
    return weeks.map((days) => {
      const monday = days[0]
      const sunday = days[6]
      const weekNum = getISOWeek(monday)

      let totalTSS = 0
      let totalDistance = 0
      let totalTime = 0
      let actCount = 0

      for (const day of days) {
        const key = dateKey(day)
        const dayActivities = activitiesByDate.get(key) ?? []
        for (const a of dayActivities) {
          totalTSS += a.tss ?? 0
          totalDistance += a.distance_m ?? 0
          totalTime += a.elapsed_time_s ?? 0
          actCount++
        }
      }

      // PMC values at end of week (Sunday)
      const sundayKey = dateKey(sunday)
      const pmc = pmcByDate.get(sundayKey)

      return {
        weekNum,
        monday,
        sunday,
        totalTSS: Math.round(totalTSS),
        totalDistance,
        totalTime,
        actCount,
        ctl: pmc?.ctl,
        atl: pmc?.atl,
        tsb: pmc?.tsb,
      }
    })
  }, [weeks, activitiesByDate, pmcByDate])

  // ── Navigation ────────────────────────────────────────────────────────────
  function goBack() {
    setAnchorDate((prev) => subWeeks(prev, weekCount === 4 ? 4 : 4))
  }

  function goForward() {
    setAnchorDate((prev) => addWeeks(prev, weekCount === 4 ? 4 : 4))
  }

  function goToday() {
    setAnchorDate(startOfWeek(new Date(), { weekStartsOn: 1 }))
  }

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-4 flex-wrap gap-3">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Kalender
        </h1>

        <div className="flex items-center gap-3">
          {/* Tab Toggle */}
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
            <button
              onClick={() => setTab('weeks')}
              className={
                tab === 'weeks'
                  ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                  : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
              }
            >
              Wochen
            </button>
            <button
              onClick={() => setTab('year')}
              className={
                tab === 'year'
                  ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                  : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
              }
            >
              Jahr
            </button>
          </div>

          {tab === 'weeks' && (
            <>
              {/* Week count toggle */}
              <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
                {([4, 8, 12] as WeekCount[]).map((n) => (
                  <button
                    key={n}
                    onClick={() => setWeekCount(n)}
                    className={
                      n === weekCount
                        ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                        : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
                    }
                  >
                    {n} Wo
                  </button>
                ))}
              </div>

              {/* Navigation */}
              <div className="flex items-center gap-1">
                <button
                  onClick={goBack}
                  className="px-2.5 py-1.5 text-xs rounded-md border border-neutral-200 dark:border-neutral-700
                             text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
                >
                  Zurueck
                </button>
                <button
                  onClick={goToday}
                  className="px-2.5 py-1.5 text-xs rounded-md border border-neutral-200 dark:border-neutral-700
                             text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
                >
                  Heute
                </button>
                <button
                  onClick={goForward}
                  className="px-2.5 py-1.5 text-xs rounded-md border border-neutral-200 dark:border-neutral-700
                             text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
                >
                  Vor
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      {tab === 'weeks' ? (
        <WeekGridView
          weeks={weeks}
          weekSummaries={weekSummaries}
          activitiesByDate={activitiesByDate}
          navigate={navigate}
        />
      ) : (
        <YearHeatmapView activitiesByDate={activitiesByDate} />
      )}
    </div>
  )
}

// ── Week Grid View ────────────────────────────────────────────────────────────

interface WeekGridProps {
  weeks: Date[][]
  weekSummaries: Array<{
    weekNum: number
    monday: Date
    sunday: Date
    totalTSS: number
    totalDistance: number
    totalTime: number
    actCount: number
    ctl?: number
    atl?: number
    tsb?: number
  }>
  activitiesByDate: Map<string, Activity[]>
  navigate: (path: string) => void
}

function WeekGridView({ weeks, weekSummaries, activitiesByDate, navigate }: WeekGridProps) {
  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <table className="w-full text-sm table-fixed">
        <thead>
          <tr className="border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50">
            <th className="text-left px-2 py-2 text-neutral-500 dark:text-neutral-400 font-medium w-[140px] text-xs">
              Woche
            </th>
            {DAY_LABELS.map((d) => (
              <th
                key={d}
                className="text-center px-1 py-2 text-neutral-500 dark:text-neutral-400 font-medium text-xs"
              >
                {d}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
          {weeks.map((days, wi) => {
            const summary = weekSummaries[wi]
            return (
              <tr key={wi} className="align-top">
                {/* Week summary cell */}
                <td className="px-2 py-2 border-r border-neutral-100 dark:border-neutral-800/50 bg-neutral-50/50 dark:bg-neutral-900/30">
                  <div className="text-xs font-medium text-neutral-700 dark:text-neutral-300">
                    KW {summary.weekNum}
                  </div>
                  <div className="text-[10px] text-neutral-400 dark:text-neutral-500">
                    {format(summary.monday, 'dd.MM.')} – {format(summary.sunday, 'dd.MM.')}
                  </div>
                  {summary.actCount > 0 && (
                    <div className="mt-1.5 space-y-0.5 text-[10px] text-neutral-500 dark:text-neutral-400">
                      <div className="flex justify-between">
                        <span>TSS</span>
                        <span className="font-medium tabular-nums">{summary.totalTSS}</span>
                      </div>
                      <div className="flex justify-between">
                        <span>Distanz</span>
                        <span className="font-medium tabular-nums">
                          {formatWeeklyDistance(summary.totalDistance)}
                        </span>
                      </div>
                      <div className="flex justify-between">
                        <span>Zeit</span>
                        <span className="font-medium tabular-nums">
                          {formatWeeklyTime(summary.totalTime)}
                        </span>
                      </div>
                    </div>
                  )}
                  {(summary.ctl != null || summary.atl != null || summary.tsb != null) && (
                    <div className="mt-1 pt-1 border-t border-neutral-200/50 dark:border-neutral-700/50 space-y-0.5 text-[10px]">
                      {summary.ctl != null && (
                        <div className="flex justify-between">
                          <span className="text-blue-500">CTL</span>
                          <span className="font-medium tabular-nums text-blue-500">
                            {summary.ctl.toFixed(0)}
                          </span>
                        </div>
                      )}
                      {summary.atl != null && (
                        <div className="flex justify-between">
                          <span className="text-orange-500">ATL</span>
                          <span className="font-medium tabular-nums text-orange-500">
                            {summary.atl.toFixed(0)}
                          </span>
                        </div>
                      )}
                      {summary.tsb != null && (
                        <div className="flex justify-between">
                          <span className={summary.tsb >= 0 ? 'text-emerald-500' : 'text-rose-500'}>
                            TSB
                          </span>
                          <span
                            className={`font-medium tabular-nums ${
                              summary.tsb >= 0 ? 'text-emerald-500' : 'text-rose-500'
                            }`}
                          >
                            {summary.tsb >= 0 ? '+' : ''}
                            {summary.tsb.toFixed(0)}
                          </span>
                        </div>
                      )}
                    </div>
                  )}
                </td>

                {/* Day cells */}
                {days.map((day) => (
                  <DayCell
                    key={dateKey(day)}
                    day={day}
                    activities={activitiesByDate.get(dateKey(day)) ?? []}
                    navigate={navigate}
                  />
                ))}
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

// ── Day Cell ──────────────────────────────────────────────────────────────────

interface DayCellProps {
  day: Date
  activities: Activity[]
  navigate: (path: string) => void
}

function DayCell({ day, activities, navigate }: DayCellProps) {
  const today = isToday(day)
  const shown = activities.slice(0, MAX_CARDS_PER_CELL)
  const extra = activities.length - MAX_CARDS_PER_CELL

  return (
    <td
      className={`px-1 py-1.5 min-h-[80px] align-top border-r last:border-r-0 border-neutral-100 dark:border-neutral-800/50 ${
        today
          ? 'bg-[color:var(--color-accent-100)]/20 dark:bg-[color:var(--color-accent-900)]/20'
          : ''
      }`}
    >
      <div
        className={`text-[10px] mb-1 px-0.5 ${
          today
            ? 'font-bold text-[color:var(--color-accent-DEFAULT)]'
            : 'text-neutral-400 dark:text-neutral-500'
        }`}
      >
        {format(day, 'd.M.')}
      </div>

      <div className="space-y-0.5">
        {shown.map((a) => (
          <ActivityCard key={a.id} activity={a} onClick={() => navigate(`/activities/${a.id}`)} />
        ))}
        {extra > 0 && (
          <div className="text-[10px] text-neutral-400 dark:text-neutral-500 px-1">
            +{extra} mehr
          </div>
        )}
      </div>
    </td>
  )
}

// ── Activity Card ─────────────────────────────────────────────────────────────

interface ActivityCardProps {
  activity: Activity
  onClick: () => void
}

function ActivityCard({ activity, onClick }: ActivityCardProps) {
  const color = sportColor(activity.sport_type)
  const label = sportLabel(activity.sport_type, activity.sub_type)
  const title = activity.title ?? label

  return (
    <button
      onClick={(e) => {
        e.stopPropagation()
        onClick()
      }}
      className="w-full text-left rounded px-1 py-0.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors
                 flex items-start gap-1 group cursor-pointer"
      style={{ borderLeft: `3px solid ${color}` }}
    >
      <div className="min-w-0 flex-1">
        <div className="text-[10px] font-medium text-neutral-700 dark:text-neutral-300 truncate group-hover:text-neutral-900 dark:group-hover:text-neutral-100 transition-colors">
          {title}
        </div>
        <div className="text-[9px] text-neutral-400 dark:text-neutral-500 flex gap-1.5 tabular-nums">
          {activity.elapsed_time_s > 0 && (
            <span>{formatDuration(activity.elapsed_time_s)}</span>
          )}
          {activity.distance_m != null && activity.distance_m > 0 && (
            <span>{formatDistance(activity.distance_m)}</span>
          )}
        </div>
      </div>
    </button>
  )
}

// ── Year Heatmap View (ECharts Calendar) ──────────────────────────────────────

interface YearHeatmapProps {
  activitiesByDate: Map<string, Activity[]>
}

function YearHeatmapView({ activitiesByDate }: YearHeatmapProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const year = new Date().getFullYear()

  // Compute daily TSS sums for the year
  const heatmapData = useMemo(() => {
    const data: [string, number][] = []
    const start = new Date(year, 0, 1)
    const end = new Date(year, 11, 31)
    const current = new Date(start)
    while (current <= end) {
      const key = dateKey(current)
      const dayActivities = activitiesByDate.get(key) ?? []
      const tss = dayActivities.reduce((sum, a) => sum + (a.tss ?? 0), 0)
      data.push([key, Math.round(tss)])
      current.setDate(current.getDate() + 1)
    }
    return data
  }, [activitiesByDate, year])

  const maxTSS = useMemo(() => {
    let max = 0
    for (const [, val] of heatmapData) {
      if (val > max) max = val
    }
    return Math.max(max, 100)
  }, [heatmapData])

  useEffect(() => {
    if (!containerRef.current) return

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })

    const isDark = document.documentElement.classList.contains('dark')
    const textColor = isDark ? '#999' : '#666'
    const borderColor = isDark ? '#262626' : '#fff'

    const option: EChartsCoreOption = {
      tooltip: {
        position: 'top',
        formatter: (params: unknown) => {
          const p = params as { value: [string, number] }
          if (!p.value) return ''
          return `${p.value[0]}<br/>TSS: <strong>${p.value[1]}</strong>`
        },
      },
      visualMap: {
        min: 0,
        max: maxTSS,
        calculable: true,
        orient: 'horizontal',
        left: 'center',
        top: 0,
        inRange: {
          color: isDark
            ? ['#1a1a2e', '#f97316', '#ef4444']
            : ['#fef3c7', '#f97316', '#dc2626'],
        },
        textStyle: { color: textColor, fontSize: 11 },
      },
      calendar: {
        top: 60,
        left: 40,
        right: 20,
        cellSize: ['auto', 16],
        range: String(year),
        itemStyle: {
          borderWidth: 2,
          borderColor: borderColor,
        },
        dayLabel: {
          nameMap: ['So', 'Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa'],
          color: textColor,
          fontSize: 10,
        },
        monthLabel: {
          nameMap: ['Jan', 'Feb', 'Mär', 'Apr', 'Mai', 'Jun', 'Jul', 'Aug', 'Sep', 'Okt', 'Nov', 'Dez'],
          color: textColor,
          fontSize: 11,
        },
        yearLabel: { show: false },
        splitLine: {
          lineStyle: { color: isDark ? '#404040' : '#d4d4d4' },
        },
      },
      series: [
        {
          type: 'heatmap',
          coordinateSystem: 'calendar',
          data: heatmapData,
        },
      ],
    }

    chart.setOption(option)

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    return () => {
      observer.disconnect()
      chart.dispose()
    }
  }, [heatmapData, maxTSS, year])

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="p-4">
        <h2 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          TSS-Heatmap {year}
        </h2>
        <div ref={containerRef} className="h-[220px] w-full" />
      </div>
    </div>
  )
}
