import { useState, useMemo, useRef, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import {
  format,
  startOfWeek,
  endOfWeek,
  addWeeks,
  subWeeks,
  isToday,
  getISOWeek,
  endOfMonth,
  eachDayOfInterval,
  getDay,
} from 'date-fns'
import { de } from 'date-fns/locale'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { HeatmapChart } from 'echarts/charts'
import { CalendarComponent, TooltipComponent, VisualMapComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchActivities } from '@/api/activities'
import { fetchPMC } from '@/api/stats'
import { fetchHealthDailySummary, fetchHealthSleep, fetchHealthHRV } from '@/api/health'
import type { HealthDailySummary, HealthSleep, HealthHRVSnapshot } from '@/api/health'
import type { Activity } from '@/api/types'
import type { PMCDataPoint } from '@/api/stats'
import { sportColor, sportLabel } from '@/lib/sportTaxonomy'
import { formatDuration, formatDistance } from '@/lib/utils'

echarts.use([HeatmapChart, CalendarComponent, TooltipComponent, VisualMapComponent, CanvasRenderer])

type WeekCount = 4 | 8 | 12
type ViewTab = 'weeks' | 'year'
type YearViewMode = 'calendar' | 'heatmap'

const DAY_LABELS = ['Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa', 'So']
// Short day headers for month grid (Mon-first, 7 columns)
const MONTH_DAY_LABELS = ['Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa', 'So']
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
  const [yearViewMode, setYearViewMode] = useState<YearViewMode>('calendar')
  const [selectedYear, setSelectedYear] = useState<number>(() => new Date().getFullYear())

  // Task 3: Year picker dropdown
  const [yearPickerOpen, setYearPickerOpen] = useState(false)
  const yearPickerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!yearPickerOpen) return
    function handleClick(e: MouseEvent) {
      if (!yearPickerRef.current?.contains(e.target as Node)) {
        setYearPickerOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [yearPickerOpen])

  // Task 5: Health data toggle
  const [showHealthData, setShowHealthData] = useState<boolean>(() => {
    try { const s = localStorage.getItem('skadia-cal-health'); return s === null ? true : s === 'true' } catch { return true }
  })

  function toggleHealthData() {
    setShowHealthData(prev => {
      const next = !prev
      try { localStorage.setItem('skadia-cal-health', String(next)) } catch {}
      return next
    })
  }

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

  // ── Fetch Year Activities (for Calendar + Heatmap) ────────────────────────
  const yearStart = `${selectedYear}-01-01`
  const yearEnd = `${selectedYear}-12-31`

  const { data: yearActivitiesData } = useQuery({
    queryKey: ['activities-year', yearStart, yearEnd],
    queryFn: () => fetchActivities(undefined, 1000, 'started_at', 'desc', {
      dateFrom: yearStart,
      dateTo: yearEnd,
    }),
    enabled: tab === 'year',
  })

  const yearActivities = yearActivitiesData?.activities ?? []

  const yearActivitiesByDate = useMemo(() => {
    const map = new Map<string, Activity[]>()
    for (const a of yearActivities) {
      const key = a.started_at.slice(0, 10)
      const list = map.get(key)
      if (list) list.push(a)
      else map.set(key, [a])
    }
    return map
  }, [yearActivities])

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

  // ── Task 5: Fetch Health Data ──────────────────────────────────────────────
  const { data: healthDailies } = useQuery({
    queryKey: ['health-daily', fromStr, toStr],
    queryFn: () => fetchHealthDailySummary(fromStr, toStr),
    enabled: showHealthData && tab === 'weeks',
  })

  const { data: healthSleepData } = useQuery({
    queryKey: ['health-sleep', fromStr, toStr],
    queryFn: () => fetchHealthSleep(fromStr, toStr),
    enabled: showHealthData && tab === 'weeks',
  })

  const { data: healthHRVData } = useQuery({
    queryKey: ['health-hrv', fromStr, toStr],
    queryFn: () => fetchHealthHRV(fromStr, toStr),
    enabled: showHealthData && tab === 'weeks',
  })

  const healthDailyByDate = useMemo(() => {
    const map = new Map<string, HealthDailySummary>()
    for (const d of healthDailies ?? []) map.set(d.calendar_date, d)
    return map
  }, [healthDailies])

  const healthSleepByDate = useMemo(() => {
    const map = new Map<string, HealthSleep>()
    for (const s of healthSleepData ?? []) map.set(s.calendar_date, s)
    return map
  }, [healthSleepData])

  const healthHRVByDate = useMemo(() => {
    const map = new Map<string, HealthHRVSnapshot>()
    for (const h of healthHRVData ?? []) {
      const dk = h.measured_at_utc.slice(0, 10)
      const existing = map.get(dk)
      if (!existing || h.time_of_day === 'MORNING') map.set(dk, h)
    }
    return map
  }, [healthHRVData])

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
          totalTime += a.moving_time_s ?? a.elapsed_time_s ?? 0
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
    setAnchorDate((prev) => subWeeks(prev, weekCount))
  }

  function goForward() {
    setAnchorDate((prev) => addWeeks(prev, weekCount))
  }

  function goToday() {
    setAnchorDate(startOfWeek(new Date(), { weekStartsOn: 1 }))
  }

  function goPrevYear() {
    setSelectedYear((y) => y - 1)
  }

  function goNextYear() {
    setSelectedYear((y) => y + 1)
  }

  const currentYear = new Date().getFullYear()

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-4 flex-wrap gap-3">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Kalender
        </h1>

        <div className="flex items-center gap-3 flex-wrap">
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
              {/* Health data toggle — links next to Wochen/Jahr */}
              <label className="flex items-center gap-1.5 cursor-pointer select-none text-xs text-neutral-600 dark:text-neutral-300">
                <div
                  onClick={toggleHealthData}
                  className={[
                    'relative w-8 h-4 rounded-full transition-colors cursor-pointer',
                    showHealthData
                      ? 'bg-[color:var(--color-accent-DEFAULT)]'
                      : 'bg-neutral-300 dark:bg-neutral-600',
                  ].join(' ')}
                >
                  <div className={[
                    'absolute top-0.5 w-3 h-3 rounded-full bg-white shadow transition-transform',
                    showHealthData ? 'translate-x-4' : 'translate-x-0.5',
                  ].join(' ')} />
                </div>
                Gesundheitsdaten
              </label>

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
                  ‹
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
                  ›
                </button>
              </div>

            </>
          )}

          {tab === 'year' && (
            <>
              {/* Year view mode toggle */}
              <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
                <button
                  onClick={() => setYearViewMode('calendar')}
                  className={
                    yearViewMode === 'calendar'
                      ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                      : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
                  }
                >
                  Kalender
                </button>
                <button
                  onClick={() => setYearViewMode('heatmap')}
                  className={
                    yearViewMode === 'heatmap'
                      ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                      : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
                  }
                >
                  TSS-Heatmap
                </button>
              </div>

              {/* Task 3: Year navigation with dropdown picker */}
              <div className="flex items-center gap-1" ref={yearPickerRef}>
                <button
                  onClick={goPrevYear}
                  className="px-2.5 py-1.5 text-xs rounded-md border border-neutral-200 dark:border-neutral-700
                             text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
                >
                  ‹
                </button>

                {/* Clickable year label */}
                <div className="relative">
                  <button
                    onClick={() => setYearPickerOpen(prev => !prev)}
                    className="px-2.5 py-1.5 text-xs font-semibold rounded-md border border-neutral-200 dark:border-neutral-700
                               text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors
                               min-w-[3.5rem] text-center"
                  >
                    {selectedYear}
                  </button>

                  {yearPickerOpen && (
                    <div className="absolute top-full left-1/2 -translate-x-1/2 mt-1 z-50
                                    rounded-lg border border-neutral-200 dark:border-neutral-700
                                    bg-white dark:bg-neutral-800 shadow-lg
                                    max-h-48 overflow-y-auto py-1 min-w-[5rem]">
                      {Array.from(
                        { length: currentYear - 2010 + 2 },
                        (_, i) => currentYear + 1 - i,
                      ).map((year) => (
                        <button
                          key={year}
                          onClick={() => {
                            setSelectedYear(year)
                            setYearPickerOpen(false)
                          }}
                          className={[
                            'w-full px-3 py-1.5 text-xs text-left transition-colors',
                            year === selectedYear
                              ? 'bg-[color:var(--color-accent-DEFAULT)] text-white font-semibold'
                              : 'text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800',
                          ].join(' ')}
                        >
                          {year}
                        </button>
                      ))}
                    </div>
                  )}
                </div>

                <button
                  onClick={goNextYear}
                  disabled={selectedYear >= currentYear + 1}
                  className="px-2.5 py-1.5 text-xs rounded-md border border-neutral-200 dark:border-neutral-700
                             text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors
                             disabled:opacity-40 disabled:cursor-not-allowed"
                >
                  ›
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
          showHealthData={showHealthData}
          healthDailyByDate={healthDailyByDate}
          healthSleepByDate={healthSleepByDate}
          healthHRVByDate={healthHRVByDate}
        />
      ) : yearViewMode === 'calendar' ? (
        <YearCalendarView
          year={selectedYear}
          activitiesByDate={yearActivitiesByDate}
          navigate={navigate}
        />
      ) : (
        <YearHeatmapView activitiesByDate={yearActivitiesByDate} year={selectedYear} />
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
  showHealthData: boolean
  healthDailyByDate: Map<string, HealthDailySummary>
  healthSleepByDate: Map<string, HealthSleep>
  healthHRVByDate: Map<string, HealthHRVSnapshot>
}

function WeekGridView({ weeks, weekSummaries, activitiesByDate, navigate, showHealthData, healthDailyByDate, healthSleepByDate, healthHRVByDate }: WeekGridProps) {
  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <table className="w-full text-sm table-fixed">
        <thead>
          <tr className="border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50">
            {/* Task 4: Wochenspalte Header */}
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
                {/* Task 4: Week summary cell — date bold, KW small below */}
                <td className="px-2 py-2 border-r border-neutral-100 dark:border-neutral-800/50 bg-neutral-50/50 dark:bg-neutral-900/30">
                  {/* Datum-Range fett oben */}
                  <div className="text-xs font-bold text-neutral-700 dark:text-neutral-300">
                    {format(summary.monday, 'dd.MM.')}–{format(summary.sunday, 'dd.MM.')}
                  </div>
                  {/* KW klein darunter */}
                  <div className="text-[10px] text-neutral-400 dark:text-neutral-500">
                    KW {summary.weekNum}
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
                          <span className="text-accent-DEFAULT">ATL</span>
                          <span className="font-medium tabular-nums text-accent-DEFAULT">
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
                {days.map((day, di) => {
                  // Task 4: detect first day of month or first day with new month in row
                  const isFirstInMonth =
                    day.getDate() === 1 ||
                    (di > 0 && days[di - 1].getMonth() !== day.getMonth())
                  return (
                    <DayCell
                      key={dateKey(day)}
                      day={day}
                      activities={activitiesByDate.get(dateKey(day)) ?? []}
                      navigate={navigate}
                      isFirstInMonth={isFirstInMonth}
                      showHealthData={showHealthData}
                      healthDaily={healthDailyByDate.get(dateKey(day))}
                      healthSleep={healthSleepByDate.get(dateKey(day))}
                      healthHRV={healthHRVByDate.get(dateKey(day))}
                    />
                  )
                })}
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
  isFirstInMonth?: boolean
  showHealthData: boolean
  healthDaily?: HealthDailySummary
  healthSleep?: HealthSleep
  healthHRV?: HealthHRVSnapshot
}

function DayCell({ day, activities, navigate, isFirstInMonth, showHealthData, healthDaily, healthSleep, healthHRV }: DayCellProps) {
  const today = isToday(day)
  const shown = activities.slice(0, MAX_CARDS_PER_CELL)
  const extra = activities.length - MAX_CARDS_PER_CELL

  // Task 5: Sleep duration in hours
  const sleepH = healthSleep
    ? ((healthSleep.deep_seconds ?? 0) + (healthSleep.light_seconds ?? 0) + (healthSleep.rem_seconds ?? 0)) / 3600
    : null

  return (
    <td
      className={`px-1 py-1.5 min-h-[80px] align-top border-r last:border-r-0 border-neutral-100 dark:border-neutral-800/50 ${
        today
          ? 'bg-[color:var(--color-accent-100)]/20 dark:bg-[color:var(--color-accent-900)]/20'
          : ''
      }`}
    >
      {/* Task 4: Date header — date bold, month indicator on transition */}
      <div className={`mb-1 px-0.5 flex items-baseline gap-0.5 ${today ? 'text-[color:var(--color-accent-DEFAULT)]' : ''}`}>
        <span className={`text-xs font-bold ${today ? '' : 'text-neutral-700 dark:text-neutral-300'}`}>
          {format(day, 'd')}
        </span>
        {isFirstInMonth && (
          <span className="text-[9px] font-medium text-[color:var(--color-accent-DEFAULT)] uppercase tracking-wide">
            {format(day, 'MMM', { locale: de })}
          </span>
        )}
      </div>

      {showHealthData ? (
        /* Task 5: Two-column layout: health left | activities right */
        <div className="flex gap-1">
          {/* Health column */}
          <div className="flex flex-col gap-0.5 text-[9px] text-neutral-500 dark:text-neutral-400 min-w-[44px] shrink-0">
            {healthDaily?.resting_heart_rate != null && (
              <div className="flex items-center gap-0.5">
                <svg className="w-2.5 h-2.5 text-rose-500 shrink-0" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"/>
                </svg>
                <span className="tabular-nums">{healthDaily.resting_heart_rate}</span>
              </div>
            )}
            {healthDaily?.total_steps != null && healthDaily.total_steps > 0 && (
              <div className="flex items-center gap-0.5">
                {/* Sneaker/shoe icon — clearly "steps", not sport activity */}
                <svg className="w-2.5 h-2.5 text-emerald-500 shrink-0" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M21 16c0-1.1-.9-2-2-2h-1l-1.2-4.2C16.4 8.7 15.3 8 14 8H4C2.9 8 2 8.9 2 10v7h1c0 1.1.9 2 2 2s2-.9 2-2h8c0 1.1.9 2 2 2s2-.9 2-2h2v-1zm-16 2c-.55 0-1-.45-1-1s.45-1 1-1 1 .45 1 1-.45 1-1 1zm12 0c-.55 0-1-.45-1-1s.45-1 1-1 1 .45 1 1-.45 1-1 1zm1-4H9.5L8 10h6l.8 2.7.6 1.3H18z"/>
                </svg>
                <span className="tabular-nums">
                  {healthDaily.total_steps >= 1000
                    ? `${(healthDaily.total_steps / 1000).toFixed(1)}k`
                    : healthDaily.total_steps}
                </span>
              </div>
            )}
            {sleepH != null && sleepH > 0 && (
              <div className="flex items-center gap-0.5">
                {/* Bed icon — clearly "sleep" */}
                <svg className="w-2.5 h-2.5 text-indigo-500 shrink-0" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M7 13c1.66 0 3-1.34 3-3S8.66 7 7 7s-3 1.34-3 3 1.34 3 3 3zm12-6h-8v7H3V5H1v15h2v-3h18v3h2v-9c0-2.21-1.79-4-4-4z"/>
                </svg>
                <span className="tabular-nums">{sleepH.toFixed(1)}h</span>
              </div>
            )}
            {healthHRV?.rmssd_ms != null && (
              <div className="flex items-center gap-0.5">
                <svg className="w-2.5 h-2.5 text-violet-500 shrink-0" fill="none" stroke="currentColor" strokeWidth="2.5" viewBox="0 0 24 24">
                  <polyline points="2,12 5,12 7,6 9,18 11,10 13,14 15,12 22,12"/>
                </svg>
                <span className="tabular-nums">{Math.round(healthHRV.rmssd_ms)}</span>
              </div>
            )}
          </div>

          {/* Activities column */}
          <div className="flex-1 min-w-0 space-y-0.5">
            {shown.map((a) => (
              <ActivityCard key={a.id} activity={a} onClick={() => navigate(`/activities/${a.id}`)} />
            ))}
            {extra > 0 && (
              <div className="text-[10px] text-neutral-400 dark:text-neutral-500 px-1">
                +{extra} mehr
              </div>
            )}
          </div>
        </div>
      ) : (
        /* Standard layout without health data */
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
      )}
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

  const tooltipParts = [title, label]
  if (activity.elapsed_time_s > 0) tooltipParts.push(formatDuration(activity.elapsed_time_s))
  if (activity.distance_m != null && activity.distance_m > 0) tooltipParts.push(formatDistance(activity.distance_m))

  return (
    <button
      onClick={(e) => {
        e.stopPropagation()
        onClick()
      }}
      title={tooltipParts.join(' · ')}
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

// ── Year Calendar View ────────────────────────────────────────────────────────

interface YearCalendarViewProps {
  year: number
  activitiesByDate: Map<string, Activity[]>
  navigate: (path: string) => void
}

// Returns 0=Mon…6=Sun index for a given Date (JS getDay is 0=Sun)
function mondayFirstDayIndex(d: Date): number {
  return (getDay(d) + 6) % 7
}

function YearCalendarView({ year, activitiesByDate, navigate }: YearCalendarViewProps) {
  // Task 2: Freeze state for click-to-freeze overlay
  const [frozenDate, setFrozenDate] = useState<string | null>(null)

  function handleFreezeToggle(key: string) {
    setFrozenDate(prev => prev === key ? null : key)
  }

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (frozenDate && !(e.target as Element).closest('[data-calendar-day]')) {
        setFrozenDate(null)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [frozenDate])

  const months = useMemo(() => {
    return Array.from({ length: 12 }, (_, i) => {
      const firstDay = new Date(year, i, 1)
      const lastDay = endOfMonth(firstDay)
      const days = eachDayOfInterval({ start: firstDay, end: lastDay })
      const leadingBlanks = mondayFirstDayIndex(firstDay)
      return { month: i, firstDay, days, leadingBlanks }
    })
  }, [year])

  const MONTH_NAMES = [
    'Januar', 'Februar', 'März', 'April', 'Mai', 'Juni',
    'Juli', 'August', 'September', 'Oktober', 'November', 'Dezember',
  ]

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {months.map(({ month, days, leadingBlanks }) => (
        <MonthCard
          key={month}
          monthName={MONTH_NAMES[month]}
          days={days}
          leadingBlanks={leadingBlanks}
          activitiesByDate={activitiesByDate}
          navigate={navigate}
          frozenDate={frozenDate}
          onFreezeToggle={handleFreezeToggle}
        />
      ))}
    </div>
  )
}

// ── Month Card ────────────────────────────────────────────────────────────────

interface MonthCardProps {
  monthName: string
  days: Date[]
  leadingBlanks: number
  activitiesByDate: Map<string, Activity[]>
  navigate: (path: string) => void
  frozenDate: string | null
  onFreezeToggle: (key: string) => void
}

function MonthCard({ monthName, days, leadingBlanks, activitiesByDate, navigate, frozenDate, onFreezeToggle }: MonthCardProps) {
  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900">
      {/* Month header */}
      <div className="px-3 py-2 border-b border-neutral-100 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/80 rounded-t-xl">
        <span className="text-xs font-semibold text-neutral-700 dark:text-neutral-300">
          {monthName}
        </span>
      </div>

      {/* Day-of-week header */}
      <div className="grid grid-cols-7 px-2 pt-2 pb-1">
        {MONTH_DAY_LABELS.map((d) => (
          <div
            key={d}
            className="text-center text-[9px] font-medium text-neutral-400 dark:text-neutral-500 pb-1"
          >
            {d}
          </div>
        ))}

        {/* Leading blank cells */}
        {Array.from({ length: leadingBlanks }, (_, i) => (
          <div key={`blank-${i}`} />
        ))}

        {/* Day cells */}
        {days.map((day) => {
          const key = dateKey(day)
          const dayActivities = activitiesByDate.get(key) ?? []
          return (
            <MonthDayCell
              key={key}
              day={day}
              activities={dayActivities}
              navigate={navigate}
              frozenDate={frozenDate}
              onFreezeToggle={onFreezeToggle}
            />
          )
        })}
      </div>
    </div>
  )
}

// ── Month Day Cell ────────────────────────────────────────────────────────────

interface MonthDayCellProps {
  day: Date
  activities: Activity[]
  navigate: (path: string) => void
  frozenDate: string | null
  onFreezeToggle: (key: string) => void
}

function MonthDayCell({ day, activities, navigate, frozenDate, onFreezeToggle }: MonthDayCellProps) {
  const today = isToday(day)
  const hasActivities = activities.length > 0
  const key = dateKey(day)
  const isFrozen = frozenDate === key

  // For single activity: use that sport's color for faint bg tint
  const singleColor = hasActivities && activities.length === 1
    ? sportColor(activities[0].sport_type)
    : null

  // Dots — max 3 visible, then "+N"
  const MAX_DOTS = 3
  const shownDots = activities.slice(0, MAX_DOTS)
  const extraDots = activities.length - MAX_DOTS

  return (
    <div
      className="relative flex flex-col items-center pb-1.5 group"
      data-calendar-day={key}
    >
      {/* Day number */}
      <div
        onClick={hasActivities ? () => onFreezeToggle(key) : undefined}
        className={[
          'w-6 h-6 flex items-center justify-center rounded-full text-[11px] font-medium transition-all',
          hasActivities ? 'cursor-pointer' : 'cursor-default',
          isFrozen ? 'ring-2 ring-offset-1 ring-[color:var(--color-accent-DEFAULT)] scale-110' : '',
          today
            ? 'text-white font-bold'
            : hasActivities
              ? 'text-neutral-700 dark:text-neutral-200'
              : 'text-neutral-400 dark:text-neutral-600',
        ].join(' ')}
        style={
          today
            ? { backgroundColor: 'var(--color-accent-DEFAULT)' }
            : singleColor
              ? { backgroundColor: `${singleColor}1a` }
              : undefined
        }
      >
        {day.getDate()}
      </div>

      {/* Activity dots */}
      {hasActivities && (
        <div className="flex items-center justify-center gap-0.5 mt-0.5 h-2">
          {shownDots.map((a) => (
            <div
              key={a.id}
              className="w-1.5 h-1.5 rounded-full flex-shrink-0"
              style={{ backgroundColor: sportColor(a.sport_type) }}
            />
          ))}
          {extraDots > 0 && (
            <span className="text-[8px] leading-none text-neutral-400 dark:text-neutral-500 ml-0.5">
              +{extraDots}
            </span>
          )}
        </div>
      )}

      {/* Overlay — hover (pointer-events-none) OR frozen (clickable) */}
      {hasActivities && (
        <div
          className={[
            'absolute bottom-full left-1/2 -translate-x-1/2 mb-1 z-50',
            isFrozen ? 'block' : 'hidden group-hover:block',
            isFrozen ? '' : 'pointer-events-none',
            'w-52 rounded-lg shadow-lg border border-neutral-200 dark:border-neutral-700',
            'bg-white dark:bg-neutral-800 text-xs',
          ].join(' ')}
        >
          {/* Arrow */}
          <div className="absolute bottom-[-5px] left-1/2 -translate-x-1/2 w-2.5 h-2.5
                          border-b border-r border-neutral-200 dark:border-neutral-700
                          bg-white dark:bg-neutral-800 rotate-45" />

          <div className="p-2 space-y-1.5">
            <div className="text-[10px] font-semibold text-neutral-500 dark:text-neutral-400 mb-1 flex justify-between items-center">
              <span>{format(day, 'EEEE, d. MMMM', { locale: de })}</span>
              {isFrozen && (
                <button
                  onClick={() => onFreezeToggle(key)}
                  className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 ml-2 leading-none text-sm"
                >
                  ×
                </button>
              )}
            </div>
            {activities.map((a) => {
              const color = sportColor(a.sport_type)
              const label = sportLabel(a.sport_type, a.sub_type)
              const title = a.title ?? label
              return (
                <a
                  key={a.id}
                  href={`/activities/${a.id}`}
                  onClick={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    navigate(`/activities/${a.id}`)
                  }}
                  className="flex items-start gap-1.5 rounded px-1 py-0.5
                             hover:bg-neutral-50 dark:hover:bg-neutral-700/60 transition-colors
                             cursor-pointer"
                  style={{ borderLeft: `3px solid ${color}` }}
                >
                  <div className="min-w-0 flex-1">
                    <div
                      className="font-medium text-neutral-800 dark:text-neutral-100 truncate"
                      style={{ color }}
                    >
                      {title}
                    </div>
                    <div className="text-neutral-500 dark:text-neutral-400 flex gap-2 tabular-nums text-[9px]">
                      <span>{label}</span>
                      {(a.moving_time_s ?? a.elapsed_time_s) > 0 && (
                        <span>{formatDuration(a.moving_time_s ?? a.elapsed_time_s)}</span>
                      )}
                      {a.distance_m != null && a.distance_m > 0 && (
                        <span>{formatDistance(a.distance_m)}</span>
                      )}
                    </div>
                  </div>
                </a>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

// ── Year Heatmap View (ECharts Calendar) ──────────────────────────────────────

interface YearHeatmapProps {
  activitiesByDate: Map<string, Activity[]>
  year: number
}

function YearHeatmapView({ activitiesByDate, year }: YearHeatmapProps) {
  const containerRef = useRef<HTMLDivElement>(null)

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
        <h2 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          TSS-Heatmap {year}
        </h2>
        {/* Task 6: TSS info text */}
        <p className="text-[10px] text-neutral-400 dark:text-neutral-500 mb-3">
          Tägliche Trainingsbelastung (TSS). Nur Aktivitäten mit berechnetem TSS (z.B. Radfahren mit Leistungsmesser oder HR-Zonen).
        </p>
        <div ref={containerRef} className="h-[220px] w-full" />
      </div>
    </div>
  )
}
