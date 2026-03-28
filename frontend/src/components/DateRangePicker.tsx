import { useEffect, useRef, useState, useMemo } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { DayPicker, useDayPicker } from 'react-day-picker'
import { toLocalISO } from '@/lib/dateUtils'
import 'react-day-picker/style.css'
import type { DateRange } from 'react-day-picker'
import { de } from 'date-fns/locale'
import { CalendarDays } from 'lucide-react'

// ─── Custom Month/Year Caption ─────────────────────────────────────────────────

const MONTHS_SHORT = ['Jan', 'Feb', 'Mär', 'Apr', 'Mai', 'Jun', 'Jul', 'Aug', 'Sep', 'Okt', 'Nov', 'Dez']
const MONTHS_FULL  = ['Januar', 'Februar', 'März', 'April', 'Mai', 'Juni', 'Juli', 'August', 'September', 'Oktober', 'November', 'Dezember']

export function MonthYearCaption({ calendarMonth }: { calendarMonth: { date: Date } }) {
  const { goToMonth } = useDayPicker()
  const [mode, setMode] = useState<null | 'month' | 'year'>(null)
  const yearListRef = useRef<HTMLDivElement>(null)

  const d = calendarMonth.date
  const year = d.getFullYear()
  const month = d.getMonth()
  const currentYear = new Date().getFullYear()

  const years = useMemo(() => {
    const result: number[] = []
    for (let y = currentYear; y >= 1970; y--) result.push(y)
    return result
  }, [currentYear])

  useEffect(() => {
    if (mode === 'year' && yearListRef.current) {
      const el = yearListRef.current.querySelector<HTMLElement>('[data-active="true"]')
      el?.scrollIntoView({ block: 'center', behavior: 'instant' })
    }
  }, [mode])

  // Close picker on outside click
  const wrapRef = useRef<HTMLDivElement>(null)
  useClickOutside(wrapRef, () => setMode(null), !!mode)

  return (
    <div className="relative" ref={wrapRef}>
      <div className="flex items-center gap-1.5">
        <button
          onClick={() => setMode(m => m === 'month' ? null : 'month')}
          className="text-sm font-semibold text-neutral-800 dark:text-neutral-200 hover:text-[var(--color-accent-DEFAULT)] cursor-pointer transition-colors"
        >
          {MONTHS_FULL[month]}
        </button>
        <button
          onClick={() => setMode(m => m === 'year' ? null : 'year')}
          className="text-sm font-semibold text-neutral-800 dark:text-neutral-200 hover:text-[var(--color-accent-DEFAULT)] cursor-pointer transition-colors"
        >
          {year}
        </button>
      </div>

      {/* Month grid */}
      {mode === 'month' && (
        <div className="absolute top-7 left-0 z-30 grid grid-cols-3 gap-1 p-2 bg-white dark:bg-neutral-900 rounded-xl shadow-xl border border-neutral-200 dark:border-neutral-700 w-44">
          {MONTHS_SHORT.map((m, i) => (
            <button
              key={m}
              onClick={() => { goToMonth(new Date(year, i)); setMode(null) }}
              style={i === month ? { backgroundColor: 'var(--color-accent-DEFAULT)', color: '#fff' } : undefined}
              className={`px-1 py-1.5 rounded text-xs cursor-pointer transition-colors ${
                i === month ? '' : 'text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
              }`}
            >
              {m}
            </button>
          ))}
        </div>
      )}

      {/* Scrollable year drum */}
      {mode === 'year' && (
        <div
          ref={yearListRef}
          className="absolute top-7 left-0 z-30 h-48 w-20 overflow-y-auto bg-white dark:bg-neutral-900 rounded-xl shadow-xl border border-neutral-200 dark:border-neutral-700 py-1 scroll-smooth"
        >
          {years.map(y => (
            <button
              key={y}
              data-active={y === year ? 'true' : undefined}
              onClick={() => { goToMonth(new Date(y, month)); setMode(null) }}
              style={y === year ? { backgroundColor: 'var(--color-accent-DEFAULT)', color: '#fff' } : undefined}
              className={`w-full px-3 py-1.5 text-xs text-left font-medium cursor-pointer transition-colors ${
                y === year ? '' : 'text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
              }`}
            >
              {y}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── Shortcuts ─────────────────────────────────────────────────────────────────

function today() { return new Date() }

const SHORTCUTS: { label: string; getRange: () => [Date, Date] }[] = [
  {
    label: 'Letzte 30 Tage',
    getRange: () => {
      const to = today(); const from = new Date(to); from.setDate(from.getDate() - 30); return [from, to]
    },
  },
  {
    label: 'Letzte 365 Tage',
    getRange: () => {
      const to = today(); const from = new Date(to); from.setFullYear(from.getFullYear() - 1); return [from, to]
    },
  },
  {
    label: 'Diesen Monat',
    getRange: () => {
      const n = today()
      return [new Date(n.getFullYear(), n.getMonth(), 1), new Date(n.getFullYear(), n.getMonth() + 1, 0)]
    },
  },
  {
    label: 'Letzten Monat',
    getRange: () => {
      const n = today()
      return [new Date(n.getFullYear(), n.getMonth() - 1, 1), new Date(n.getFullYear(), n.getMonth(), 0)]
    },
  },
  {
    label: 'Dieses Jahr',
    getRange: () => { const y = today().getFullYear(); return [new Date(y, 0, 1), new Date(y, 11, 31)] },
  },
  {
    label: 'Letztes Jahr',
    getRange: () => { const y = today().getFullYear() - 1; return [new Date(y, 0, 1), new Date(y, 11, 31)] },
  },
]

function formatDisplay(from: string, to: string): string {
  if (!from && !to) return 'Zeitraum'
  const fmt = (s: string) =>
    new Date(s).toLocaleDateString('de-DE', { day: '2-digit', month: '2-digit', year: '2-digit' })
  if (from && to) return `${fmt(from)} – ${fmt(to)}`
  if (from) return `Ab ${fmt(from)}`
  return `Bis ${fmt(to)}`
}

// ─── Public component ──────────────────────────────────────────────────────────

interface DateRangePickerProps {
  dateFrom: string
  dateTo: string
  onChange: (from: string, to: string) => void
}

export default function DateRangePicker({ dateFrom, dateTo, onChange }: DateRangePickerProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useClickOutside(ref, () => setOpen(false), open)

  const hasFilter = !!(dateFrom || dateTo)

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(v => !v)}
        className={`inline-flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg border cursor-pointer transition-colors whitespace-nowrap ${
          hasFilter
            ? 'border-accent-400 bg-orange-50 dark:bg-orange-950/30 text-accent-600 dark:text-orange-400'
            : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
        }`}
      >
        <CalendarDays className="w-3.5 h-3.5 flex-shrink-0" />
        {formatDisplay(dateFrom, dateTo)}
        {hasFilter && (
          <span
            onClick={e => { e.stopPropagation(); onChange('', '') }}
            className="ml-0.5 text-neutral-400 hover:text-red-500 transition-colors"
          >
            &#x2715;
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-1 z-20 bg-white dark:bg-neutral-900 rounded-xl shadow-lg border border-neutral-200 dark:border-neutral-800 flex">
          {/* Shortcuts */}
          <div className="flex flex-col justify-center gap-0.5 p-2 border-r border-neutral-100 dark:border-neutral-800 min-w-36">
            {SHORTCUTS.map(s => (
              <button
                key={s.label}
                onClick={() => { const [f, t] = s.getRange(); onChange(toLocalISO(f), toLocalISO(t)); setOpen(false) }}
                className="px-3 py-1.5 text-xs text-left rounded-lg text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer transition-colors whitespace-nowrap"
              >
                {s.label}
              </button>
            ))}
          </div>

          {/* DayPicker with custom caption */}
          <DayPicker
            mode="range"
            numberOfMonths={2}
            selected={{
              from: dateFrom ? new Date(dateFrom) : undefined,
              to: dateTo ? new Date(dateTo) : undefined,
            }}
            onSelect={(range: DateRange | undefined) => {
              onChange(
                range?.from ? toLocalISO(range.from) : '',
                range?.to   ? toLocalISO(range.to)   : '',
              )
              if (range?.from && range?.to) setOpen(false)
            }}
            locale={de}
            components={{ MonthCaption: MonthYearCaption as any }}
            className="p-3 text-sm"
          />
        </div>
      )}
    </div>
  )
}
