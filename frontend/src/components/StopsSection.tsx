import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchStops } from '@/api/enrichment'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import type { ActivityStop } from '@/api/types'

interface Props {
  activityId: string
}

function formatDurationShort(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  const min = Math.floor(seconds / 60)
  const sec = seconds % 60
  if (min < 60) return sec > 0 ? `${min}m ${sec}s` : `${min} min`
  const h = Math.floor(min / 60)
  const rm = min % 60
  return rm > 0 ? `${h}h ${rm}m` : `${h}h`
}

function PoiIcon({ poiType }: { poiType?: string }) {
  const cls = 'w-4 h-4 shrink-0 text-neutral-500 dark:text-neutral-400'
  if (!poiType) {
    // MapPin fallback
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z" />
        <circle cx="12" cy="9" r="2.5" />
      </svg>
    )
  }
  if (['restaurant', 'fast_food', 'food_court'].includes(poiType)) {
    // UtensilsCrossed
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 2l4 4-4 4" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M5 6h16" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M21 2l-7 7" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M14 9l-7 7 7 7 7-7" />
      </svg>
    )
  }
  if (['cafe', 'biergarten'].includes(poiType)) {
    // Coffee cup
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M17 8h1a4 4 0 0 1 0 8h-1" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 8h14v9a4 4 0 0 1-4 4H7a4 4 0 0 1-4-4V8z" />
        <line x1="6" y1="2" x2="6" y2="4" />
        <line x1="10" y1="2" x2="10" y2="4" />
        <line x1="14" y1="2" x2="14" y2="4" />
      </svg>
    )
  }
  if (['bar', 'pub'].includes(poiType)) {
    // Wine glass
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M8 22h8M12 11v11M5 2h14l-2 7a5 5 0 0 1-10 0L5 2z" />
      </svg>
    )
  }
  if (poiType === 'ice_cream') {
    // Ice cream / dessert
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 22l4-8H8l4 8z" />
        <circle cx="12" cy="10" r="4" />
      </svg>
    )
  }
  if (['alpine_hut', 'wilderness_hut'].includes(poiType)) {
    // Mountain
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 20l6.5-11L13 15l4-6 4 11H3z" />
      </svg>
    )
  }
  if (['guest_house', 'hostel', 'hotel'].includes(poiType)) {
    // Bed
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 7v11M3 11h18M21 7v11M7 11V7a2 2 0 0 1 2-2h6a2 2 0 0 1 2 2v4" />
      </svg>
    )
  }
  if (['kiosk', 'bakery', 'butcher'].includes(poiType)) {
    // Shopping bag
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M6 2L3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4z" />
        <line x1="3" y1="6" x2="21" y2="6" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M16 10a4 4 0 0 1-8 0" />
      </svg>
    )
  }
  // Default MapPin
  return (
    <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z" />
      <circle cx="12" cy="9" r="2.5" />
    </svg>
  )
}

function getStopTitle(stop: ActivityStop): string {
  if (stop.poi_name) return stop.poi_name
  if (stop.name) return stop.name
  return 'Aufenthalt'
}

export default function StopsSection({ activityId }: Props) {
  const [collapsed, setCollapsed] = useState(false)

  const { data: stops = [], isLoading, isError } = useQuery({
    queryKey: ['activity-stops', activityId],
    queryFn: () => fetchStops(activityId),
  })

  if (isLoading) return null
  if (isError) return null
  if (stops.length === 0) return null

  return (
    <div>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center gap-2 mb-3 group"
      >
        <svg
          className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '' : 'rotate-90'}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors">
          Stopps
        </h2>
        <span className="text-sm text-neutral-400">({stops.length})</span>
      </button>

      {!collapsed && (
        <div className="space-y-1.5">
          {stops.map(stop => {
            const title = getStopTitle(stop)
            const isFallback = !stop.poi_name && !stop.name
            return (
              <div
                key={stop.id}
                className="flex items-center justify-between text-sm px-3 py-2 rounded-lg bg-neutral-50 dark:bg-neutral-800/40"
              >
                <div className="flex items-center gap-3">
                  <span className="text-xs tabular-nums text-neutral-500 dark:text-neutral-400">
                    {format(new Date(stop.started_at), 'HH:mm', { locale: de })}
                  </span>
                  <PoiIcon poiType={stop.poi_type} />
                  <span className={isFallback ? 'text-neutral-500 dark:text-neutral-400' : 'font-medium text-neutral-800 dark:text-neutral-200'}>
                    {title}
                  </span>
                </div>
                <span className="tabular-nums text-neutral-600 dark:text-neutral-400">
                  {formatDurationShort(stop.duration_s)}
                </span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
