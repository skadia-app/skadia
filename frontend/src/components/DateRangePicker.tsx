import { useEffect, useRef, useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useClickOutside } from '@/hooks/useClickOutside'
import { DayPicker, useDayPicker } from 'react-day-picker'
import { toLocalISO } from '@/lib/dateUtils'
import { dateFnsLocaleForLanguage } from '@/lib/format'
import 'react-day-picker/style.css'
import type { DateRange } from 'react-day-picker'
import { CalendarDays, ChevronLeft, ChevronRight } from 'lucide-react'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import { localeForLanguage } from '@/i18n/languages'

// ─── Custom Month/Year Caption ─────────────────────────────────────────────────

export function MonthYearCaption({ calendarMonth }: { calendarMonth: { date: Date } }) {
  const { goToMonth } = useDayPicker()
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const locale = localeForLanguage(language)
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

  function shiftMonth(delta: number) {
    goToMonth(new Date(year, month + delta))
  }

  return (
    <div className="relative" ref={wrapRef}>
      <div className="flex items-center justify-between gap-1.5 mb-1">
        <button
          type="button"
          aria-label={t('dateRangePicker.previousMonth')}
          onClick={() => shiftMonth(-1)}
          className="p-1 rounded text-neutral-500 hover:text-neutral-800 dark:text-neutral-400 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
        >
          <ChevronLeft className="w-4 h-4" />
        </button>
        <div className="flex items-center gap-1.5">
          <button
            onClick={() => setMode(m => m === 'month' ? null : 'month')}
            className="text-sm font-semibold text-neutral-800 dark:text-neutral-200 hover:text-[var(--color-accent-DEFAULT)] cursor-pointer transition-colors"
          >
            {new Date(year, month, 1).toLocaleDateString(locale, { month: 'long' })}
          </button>
          <button
            onClick={() => setMode(m => m === 'year' ? null : 'year')}
            className="text-sm font-semibold text-neutral-800 dark:text-neutral-200 hover:text-[var(--color-accent-DEFAULT)] cursor-pointer transition-colors"
          >
            {year}
          </button>
        </div>
        <button
          type="button"
          aria-label={t('dateRangePicker.nextMonth')}
          onClick={() => shiftMonth(1)}
          className="p-1 rounded text-neutral-500 hover:text-neutral-800 dark:text-neutral-400 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
        >
          <ChevronRight className="w-4 h-4" />
        </button>
      </div>

      {/* Month grid */}
      {mode === 'month' && (
        <div className="absolute top-7 left-0 z-30 grid grid-cols-3 gap-1 p-2 bg-white dark:bg-neutral-900 rounded-xl shadow-xl border border-neutral-200 dark:border-neutral-700 w-44">
          {Array.from({ length: 12 }, (_, i) => new Date(year, i, 1).toLocaleDateString(locale, { month: 'short' })).map((m, i) => (
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

const SHORTCUTS: { key: string; getRange: () => [Date, Date] }[] = [
  {
    key: 'last30Days',
    getRange: () => {
      const to = today(); const from = new Date(to); from.setDate(from.getDate() - 30); return [from, to]
    },
  },
  {
    key: 'last365Days',
    getRange: () => {
      const to = today(); const from = new Date(to); from.setFullYear(from.getFullYear() - 1); return [from, to]
    },
  },
  {
    key: 'thisMonth',
    getRange: () => {
      const n = today()
      return [new Date(n.getFullYear(), n.getMonth(), 1), new Date(n.getFullYear(), n.getMonth() + 1, 0)]
    },
  },
  {
    key: 'lastMonth',
    getRange: () => {
      const n = today()
      return [new Date(n.getFullYear(), n.getMonth() - 1, 1), new Date(n.getFullYear(), n.getMonth(), 0)]
    },
  },
  {
    key: 'thisYear',
    getRange: () => { const y = today().getFullYear(); return [new Date(y, 0, 1), new Date(y, 11, 31)] },
  },
  {
    key: 'lastYear',
    getRange: () => { const y = today().getFullYear() - 1; return [new Date(y, 0, 1), new Date(y, 11, 31)] },
  },
]

function formatDisplay(from: string, to: string, t: ReturnType<typeof useTranslation>['t'], locale: string): string {
  if (!from && !to) return t('dateRangePicker.placeholder')
  const fmt = (s: string) =>
    new Date(s).toLocaleDateString(locale, { day: '2-digit', month: '2-digit', year: '2-digit' })
  if (from && to) return `${fmt(from)} – ${fmt(to)}`
  if (from) return t('dateRangePicker.from', { date: fmt(from) })
  return t('dateRangePicker.to', { date: fmt(to) })
}

// ─── Public component ──────────────────────────────────────────────────────────

interface DateRangePickerProps {
  dateFrom: string
  dateTo: string
  onChange: (from: string, to: string) => void
  /** Open the popover on first render. Useful when the picker is revealed
   *  by a parent's toggle and the trigger button would force a redundant click. */
  defaultOpen?: boolean
  /** Hide the picker's own trigger button — only the popover renders.
   *  Caller is responsible for mounting/unmounting the component to control visibility. */
  triggerless?: boolean
  /** Called when the popover closes (outside click, ESC, range completion).
   *  Lets a parent dismiss the wrapper. */
  onClose?: () => void
}

export default function DateRangePicker({ dateFrom, dateTo, onChange, defaultOpen, triggerless, onClose }: DateRangePickerProps) {
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const locale = localeForLanguage(language)
  const dateFnsLocale = dateFnsLocaleForLanguage(language)
  const [open, setOpen] = useState(defaultOpen ?? false)
  const ref = useRef<HTMLDivElement>(null)

  useClickOutside(ref, () => { setOpen(false); onClose?.() }, open)

  const hasFilter = !!(dateFrom || dateTo)

  return (
    <div className="relative" ref={ref}>
      {!triggerless && (
        <button
          onClick={() => setOpen(v => !v)}
          className={`inline-flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg border cursor-pointer transition-colors whitespace-nowrap ${
            hasFilter
              ? 'border-accent-400 bg-orange-50 dark:bg-orange-950/30 text-accent-600 dark:text-orange-400'
              : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
          }`}
        >
          <CalendarDays className="w-3.5 h-3.5 flex-shrink-0" />
          {formatDisplay(dateFrom, dateTo, t, locale)}
          {hasFilter && (
            <span
              onClick={e => { e.stopPropagation(); onChange('', '') }}
              className="ml-0.5 text-neutral-400 hover:text-red-500 transition-colors"
            >
              &#x2715;
            </span>
          )}
        </button>
      )}

      {open && (
        <div className="absolute right-0 top-full mt-1 z-20 bg-white dark:bg-neutral-900 rounded-xl shadow-lg border border-neutral-200 dark:border-neutral-800 flex">
          {/* Shortcuts */}
          <div className="flex flex-col justify-center gap-0.5 p-2 border-r border-neutral-100 dark:border-neutral-800 min-w-36">
            {SHORTCUTS.map(s => (
              <button
                key={s.key}
                onClick={() => { const [f, t] = s.getRange(); onChange(toLocalISO(f), toLocalISO(t)); setOpen(false) }}
                className="px-3 py-1.5 text-xs text-left rounded-lg text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer transition-colors whitespace-nowrap"
              >
                {t(`dateRangePicker.shortcuts.${s.key}`)}
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
            locale={dateFnsLocale}
            components={{
              MonthCaption: MonthYearCaption as any,
              // Hide default Nav arrows — our custom MonthYearCaption renders prev/next inline.
              Nav: () => <></>,
            }}
            className="p-3 text-sm"
          />
        </div>
      )}
    </div>
  )
}
