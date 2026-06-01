import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useClickOutside } from '@/hooks/useClickOutside'
import { startOfWeek, endOfWeek, addWeeks, subWeeks } from 'date-fns'
import { localeForLanguage } from '@/i18n/languages'
import { supportedLanguageFrom } from '@/i18n/localizedString'

interface DateRangePopoverProps {
  /** The anchor date (typically Monday of the week to display) */
  anchorDate: Date
  /** Callback when a quick-action button is clicked with the new anchor date */
  onSelect: (newAnchorDate: Date) => void
  /** Only show popover when tab === 'weeks' */
  tab: 'weeks' | 'year'
}

export default function DateRangePopover({
  anchorDate,
  onSelect,
  tab,
}: DateRangePopoverProps) {
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const locale = localeForLanguage(language)
  const [isOpen, setIsOpen] = useState(false)
  const popoverRef = useRef<HTMLDivElement>(null)

  // Close popover when clicking outside
  useClickOutside(popoverRef, () => setIsOpen(false), isOpen)

  // Only show in weeks tab
  if (tab !== 'weeks') return null

  const weekStart = startOfWeek(anchorDate, { weekStartsOn: 1 })
  const weekEnd = endOfWeek(anchorDate, { weekStartsOn: 1 })

  const dateFormatter = new Intl.DateTimeFormat(locale, { dateStyle: 'medium' })
  const dateLabel = `${dateFormatter.format(weekStart)} – ${dateFormatter.format(weekEnd)}`

  function handleToday() {
    const today = startOfWeek(new Date(), { weekStartsOn: 1 })
    onSelect(today)
    setIsOpen(false)
  }

  function handleNextWeek() {
    const nextWeek = addWeeks(anchorDate, 1)
    onSelect(nextWeek)
    setIsOpen(false)
  }

  function handleLastWeek() {
    const lastWeek = subWeeks(anchorDate, 1)
    onSelect(lastWeek)
    setIsOpen(false)
  }

  return (
    <div className="relative" ref={popoverRef}>
      {/* Trigger Button */}
      <button
        onClick={() => setIsOpen(prev => !prev)}
        className="px-3 py-1.5 text-xs font-semibold rounded-md border border-neutral-200 dark:border-neutral-700
                   text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors
                   min-w-fit whitespace-nowrap"
      >
        {dateLabel}
      </button>

      {/* Popover */}
      {isOpen && (
        <div className="absolute top-full left-0 mt-1 z-50
                        rounded-lg border border-neutral-200 dark:border-neutral-700
                        bg-white dark:bg-neutral-800 shadow-lg
                        p-1 min-w-max">
          <button
            onClick={handleToday}
            className="cursor-pointer w-full px-3 py-1.5 text-xs text-left rounded transition-colors
                       text-neutral-700 dark:text-neutral-300
                       hover:bg-neutral-50 dark:hover:bg-neutral-800"
          >
            {t('dateRangePopover.today')}
          </button>
          <button
            onClick={handleNextWeek}
            className="cursor-pointer w-full px-3 py-1.5 text-xs text-left rounded transition-colors
                       text-neutral-700 dark:text-neutral-300
                       hover:bg-neutral-50 dark:hover:bg-neutral-800"
          >
            {t('dateRangePopover.nextWeek')}
          </button>
          <button
            onClick={handleLastWeek}
            className="cursor-pointer w-full px-3 py-1.5 text-xs text-left rounded transition-colors
                       text-neutral-700 dark:text-neutral-300
                       hover:bg-neutral-50 dark:hover:bg-neutral-800"
          >
            {t('dateRangePopover.lastWeek')}
          </button>
        </div>
      )}
    </div>
  )
}
