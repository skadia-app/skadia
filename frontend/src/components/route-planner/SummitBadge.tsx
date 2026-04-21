import { useState } from 'react'
import type { PassedSummit } from '@/api/routes'
import { statsSummitsLabel } from '@/lib/strings/suggest-routes'

interface Props {
  summits: PassedSummit[]
  onHover: (active: boolean) => void
}

function summitIcon(category: PassedSummit['category']): string {
  return category === 'summit' ? '⛰' : '〰'
}

// osm_id=0 ist ein bekannter DB-Importbug — mehrere Gipfel können id=0 haben.
function summitKey(s: PassedSummit): string {
  return s.osm_id > 0 ? String(s.osm_id) : `${s.lat.toFixed(5)},${s.lng.toFixed(5)}`
}

export function SummitBadge({ summits, onHover }: Props) {
  const [open, setOpen] = useState(false)

  const openNow = () => {
    setOpen(true)
    onHover(true)
  }

  const closeWithGuard = (relatedTarget: EventTarget | null) => {
    const rt = relatedTarget as HTMLElement | null
    if (rt?.closest?.('[data-summit-badge]')) return
    setOpen(false)
    onHover(false)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape' && open) {
      setOpen(false)
      onHover(false)
    }
  }

  return (
    <div className="relative inline-block" data-summit-badge>
      <button
        type="button"
        className="inline-flex items-center gap-1 text-sm rounded px-1.5 py-0.5 hover:bg-neutral-100 dark:hover:bg-neutral-700 cursor-pointer transition-colors"
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-label={`${statsSummitsLabel(summits.length)}, Details öffnen`}
        onMouseEnter={openNow}
        onMouseLeave={(e) => closeWithGuard(e.relatedTarget)}
        onFocus={openNow}
        onBlur={(e) => closeWithGuard(e.relatedTarget)}
        onKeyDown={handleKeyDown}
      >
        {statsSummitsLabel(summits.length)}
      </button>
      {open && summits.length > 0 && (
        <div
          className="absolute z-20 bottom-full left-0 mb-1 min-w-[210px] max-h-56 overflow-y-auto rounded-lg bg-white/95 dark:bg-neutral-800/95 shadow-lg border border-neutral-200 dark:border-neutral-700 p-2 text-xs"
          role="listbox"
          aria-label="Gipfel und Pässe auf der Route"
          data-summit-badge
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
                  <span className="break-words">{s.name || '(kein Name)'}</span>
                </span>
                <span className="text-right text-neutral-500 dark:text-neutral-400 tabular-nums shrink-0 ml-auto pl-2">
                  {s.ele_m != null ? `${s.ele_m.toLocaleString('de')} m` : '—'}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
