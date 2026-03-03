import type { ClimbProEvent } from '@/api/types'

interface Props {
  events: ClimbProEvent[]
}

function eventLabel(event: string): string {
  const labels: Record<string, string> = {
    approach: 'Anfahrt',
    start: 'Start',
    complete: 'Gipfel',
  }
  return labels[event] ?? event
}

function eventColor(event: string): string {
  switch (event) {
    case 'approach': return 'bg-neutral-200 dark:bg-neutral-700 text-neutral-700 dark:text-neutral-300'
    case 'start': return 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-400'
    case 'complete': return 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
    default: return 'bg-neutral-200 dark:bg-neutral-700 text-neutral-600 dark:text-neutral-400'
  }
}

function categoryLabel(cat?: number): string | null {
  if (cat == null) return null
  // Garmin ClimbPro categories: 0=uncategorized, 1=Cat4, 2=Cat3, 3=Cat2, 4=Cat1, 5=HC
  const labels: Record<number, string> = {
    1: 'Kat. 4',
    2: 'Kat. 3',
    3: 'Kat. 2',
    4: 'Kat. 1',
    5: 'HC',
  }
  return labels[cat] ?? null
}

function fmtDist(m?: number): string {
  if (m == null) return ''
  if (m >= 1000) return `${(m / 1000).toFixed(1)} km`
  return `${Math.round(m)} m`
}

export default function ClimbProChart({ events }: Props) {
  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">ClimbPro</h2>
      <div className="flex flex-wrap gap-2">
        {events.map(ev => {
          const cat = categoryLabel(ev.climb_category)
          return (
            <div
              key={ev.id}
              className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-2.5 min-w-[120px]"
            >
              <div className="flex items-center gap-2 mb-1">
                {ev.climb_number != null && (
                  <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">#{ev.climb_number}</span>
                )}
                <span className={`text-xs px-1.5 py-0.5 rounded ${eventColor(ev.climb_pro_event)}`}>
                  {eventLabel(ev.climb_pro_event)}
                </span>
                {cat && (
                  <span className="text-xs px-1.5 py-0.5 rounded bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400">
                    {cat}
                  </span>
                )}
              </div>
              {ev.current_dist_m != null && (
                <div className="text-xs text-neutral-500 dark:text-neutral-400">
                  {fmtDist(ev.current_dist_m)}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
