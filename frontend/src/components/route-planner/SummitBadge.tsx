import type { PassedSummit } from '@/api/routes'
import { useHoverPopover } from '@/hooks/useHoverPopover'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { useTranslation } from 'react-i18next'

interface Props {
  summits: PassedSummit[]
  onHover: (active: boolean) => void
  isPinned: boolean
  onTogglePin: () => void
}

function summitIcon(category: PassedSummit['category']): string {
  return category === 'summit' ? '⛰' : '〰'
}

// osm_id=0 is a known DB import issue: multiple summits can have id=0.
function summitKey(s: PassedSummit): string {
  return s.osm_id > 0 ? String(s.osm_id) : `${s.lat.toFixed(5)},${s.lng.toFixed(5)}`
}

export function SummitBadge({ summits, onHover, isPinned, onTogglePin }: Props) {
  const { t } = useTranslation()
  const { formatElevation } = useUnitFormatters()
  const { open, triggerProps, popoverProps } = useHoverPopover({
    onOpen: () => onHover(true),
    onClose: () => onHover(false),
  })
  const summary = t('routePlanner.summits.summary', { count: summits.length })

  return (
    <div className="relative inline-block" {...popoverProps}>
      <button
        type="button"
        className={`inline-flex items-center gap-1 text-sm rounded px-1.5 py-0.5 hover:bg-neutral-100 dark:hover:bg-neutral-700 cursor-pointer transition-colors ${isPinned ? 'ring-2 ring-accent' : ''}`}
        aria-haspopup="listbox"
        aria-pressed={isPinned}
        aria-label={t('routePlanner.summits.detailsAria', { summary })}
        {...triggerProps}
        onClick={onTogglePin}
      >
        {summary}
      </button>
      {open && summits.length > 0 && (
        <div
          className="absolute z-20 bottom-full left-0 mb-1 min-w-[210px] max-h-56 overflow-y-auto rounded-lg bg-white/95 dark:bg-neutral-800/95 shadow-lg border border-neutral-200 dark:border-neutral-700 p-2 text-xs"
          role="listbox"
          aria-label={t('routePlanner.summits.listAria')}
          {...popoverProps}
        >
          <ul className="space-y-1">
            {summits.map((s) => (
              <li
                key={summitKey(s)}
                role="option"
                aria-selected={false}
                className="flex items-center justify-between gap-2 text-neutral-700 dark:text-neutral-300"
              >
                <span className="flex items-center gap-1 min-w-0 shrink">
                  <span aria-hidden="true" className="shrink-0">{summitIcon(s.category)}</span>
                  <span className="break-words">{s.name || t('routePlanner.summits.noName')}</span>
                </span>
                <span className="text-right text-neutral-500 dark:text-neutral-400 tabular-nums shrink-0 ml-auto pl-2">
                  {formatElevation(s.ele_m)}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
