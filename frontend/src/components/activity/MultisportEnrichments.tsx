import { useState } from 'react'
import { useQueries } from '@tanstack/react-query'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { fetchGeoTags, fetchStops, fetchSurfaces } from '@/api/enrichment'
import type { Activity, GeoTag, ActivityStop, ActivitySurface } from '@/api/types'

interface Props {
  childActivities: Activity[]
}

// ── Geo-Tags rendering helpers ────────────────────────────────────────────────

interface CategoryConfig {
  label: string
  icon: React.ReactNode
  order: number
}

const categoryConfigs: Record<string, CategoryConfig> = {
  summit: {
    label: 'Gipfel',
    order: 1,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 20l6.5-11L13 15l4-6 4 11H3z" />
      </svg>
    ),
  },
  pass: {
    label: 'Pässe',
    order: 2,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M4 20c2-6 5-10 8-10s6 4 8 10" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 10V4m-2 2l2-2 2 2" />
      </svg>
    ),
  },
  gastronomy: {
    label: 'Einkehr',
    order: 3,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 2l4 4-4 4" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M5 6h16" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M21 2l-7 7" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M14 9l-7 7 7 7 7-7" />
      </svg>
    ),
  },
  settlement: {
    label: 'Ortschaften',
    order: 4,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 21h18M5 21V7l7-4 7 4v14M9 21v-4h6v4" />
      </svg>
    ),
  },
  water: {
    label: 'Gewässer',
    order: 5,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M2 12c2-2 4-2 6 0s4 2 6 0 4-2 6 0M2 17c2-2 4-2 6 0s4 2 6 0 4-2 6 0" />
      </svg>
    ),
  },
  forest: {
    label: 'Wälder',
    order: 6,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 3l-7 10h4l-3 8h12l-3-8h4L12 3z" />
      </svg>
    ),
  },
  border: {
    label: 'Grenzen',
    order: 7,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1zM4 22v-7" />
      </svg>
    ),
  },
  stop: {
    label: 'Stopps',
    order: 8,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <circle cx="12" cy="12" r="9" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 7v5l3 3" />
      </svg>
    ),
  },
}

const POI_TYPE_LABELS: Record<string, string> = {
  restaurant: 'Restaurant',
  cafe: 'Café',
  alpine_hut: 'Berghütte / Alm',
  wilderness_hut: 'Schutzhütte',
  fast_food: 'Imbiss',
  ice_cream: 'Eisdiele',
  bar: 'Bar',
  pub: 'Pub',
  biergarten: 'Biergarten',
  kiosk: 'Kiosk',
  bakery: 'Bäckerei',
  butcher: 'Metzgerei',
  food_court: 'Food Court',
  guest_house: 'Pension',
  hostel: 'Hostel',
  hotel: 'Hotel',
}

function getGroupKey(category: string): string {
  if (category === 'summit' || category === 'peak') return 'summit'
  if (category === 'still_water' || category === 'flowing_water' || category === 'water') return 'water'
  return category
}

function getConfig(groupKey: string): CategoryConfig {
  return categoryConfigs[groupKey] ?? {
    label: groupKey,
    order: 99,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <circle cx="12" cy="12" r="3" />
      </svg>
    ),
  }
}

// ── Stops rendering helpers ───────────────────────────────────────────────────

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
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z" />
        <circle cx="12" cy="9" r="2.5" />
      </svg>
    )
  }
  if (['restaurant', 'fast_food', 'food_court'].includes(poiType)) {
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
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M8 22h8M12 11v11M5 2h14l-2 7a5 5 0 0 1-10 0L5 2z" />
      </svg>
    )
  }
  if (poiType === 'ice_cream') {
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 22l4-8H8l4 8z" />
        <circle cx="12" cy="10" r="4" />
      </svg>
    )
  }
  if (['alpine_hut', 'wilderness_hut'].includes(poiType)) {
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 20l6.5-11L13 15l4-6 4 11H3z" />
      </svg>
    )
  }
  if (['guest_house', 'hostel', 'hotel'].includes(poiType)) {
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 7v11M3 11h18M21 7v11M7 11V7a2 2 0 0 1 2-2h6a2 2 0 0 1 2 2v4" />
      </svg>
    )
  }
  if (['kiosk', 'bakery', 'butcher'].includes(poiType)) {
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M6 2L3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4z" />
        <line x1="3" y1="6" x2="21" y2="6" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M16 10a4 4 0 0 1-8 0" />
      </svg>
    )
  }
  return (
    <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z" />
      <circle cx="12" cy="9" r="2.5" />
    </svg>
  )
}

// ── Surfaces helpers ──────────────────────────────────────────────────────────

const SURFACE_COLORS: Record<string, string> = {
  asphalt: '#374151',
  paved: '#6b7280',
  unpaved: '#92400e',
  gravel: '#b45309',
  dirt: '#78350f',
  grass: '#166534',
  cobblestone: '#4b5563',
  sand: '#d97706',
}

const SURFACE_LABELS: Record<string, string> = {
  asphalt: 'Asphalt',
  paved: 'Befestigt',
  unpaved: 'Unbefestigt',
  gravel: 'Schotter',
  dirt: 'Feldweg',
  grass: 'Gras',
  cobblestone: 'Pflaster',
  sand: 'Sand',
}

// ── Section wrapper ───────────────────────────────────────────────────────────

function CollapsibleSection({
  title,
  count,
  children,
}: {
  title: string
  count?: number
  children: React.ReactNode
}) {
  const [collapsed, setCollapsed] = useState(false)
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
          {title}
        </h2>
        {count != null && (
          <span className="text-sm text-neutral-400">({count})</span>
        )}
      </button>
      {!collapsed && children}
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export default function MultisportEnrichments({ childActivities }: Props) {
  const ids = childActivities.map(c => c.id)

  const geoTagQueries = useQueries({
    queries: ids.map(id => ({
      queryKey: ['activity-geo-tags', id],
      queryFn: () => fetchGeoTags(id),
      staleTime: 5 * 60 * 1000,
    })),
  })

  const stopQueries = useQueries({
    queries: ids.map(id => ({
      queryKey: ['activity-stops', id],
      queryFn: () => fetchStops(id),
      staleTime: 5 * 60 * 1000,
    })),
  })

  const surfaceQueries = useQueries({
    queries: ids.map(id => ({
      queryKey: ['activity-surfaces', id],
      queryFn: () => fetchSurfaces(id),
      staleTime: 5 * 60 * 1000,
    })),
  })

  // Merge geo-tags: dedup by osm_id or category+name
  const mergedGeoTags: GeoTag[] = []
  const seenGeoKeys = new Set<string>()
  for (const q of geoTagQueries) {
    for (const tag of (q.data ?? [])) {
      const key = tag.osm_id != null ? `osm:${tag.osm_id}` : `${tag.category}:${tag.name}`
      if (!seenGeoKeys.has(key)) {
        seenGeoKeys.add(key)
        mergedGeoTags.push(tag)
      }
    }
  }

  // Merge stops: all stops filtered by duration
  const allStops: ActivityStop[] = stopQueries
    .flatMap(q => q.data ?? [])
    .filter(s => s.duration_s >= 120)
    .sort((a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime())

  // Merge surfaces: aggregate by surface_type
  const surfaceMap = new Map<string, { length_m: number; path_type: string }>()
  for (const q of surfaceQueries) {
    for (const s of (q.data ?? [])) {
      const existing = surfaceMap.get(s.surface_type)
      if (existing) {
        existing.length_m += s.length_m
      } else {
        surfaceMap.set(s.surface_type, { length_m: s.length_m, path_type: s.path_type })
      }
    }
  }
  const totalSurfaceLength = [...surfaceMap.values()].reduce((sum, s) => sum + s.length_m, 0)
  const mergedSurfaces: ActivitySurface[] = [...surfaceMap.entries()]
    .map(([surface_type, { length_m, path_type }]) => ({
      id: surface_type,
      surface_type,
      path_type,
      length_m,
      percentage: totalSurfaceLength > 0 ? (length_m / totalSurfaceLength) * 100 : 0,
    }))
    .sort((a, b) => b.length_m - a.length_m)

  if (mergedGeoTags.length === 0 && allStops.length === 0 && mergedSurfaces.length === 0) {
    return null
  }

  return (
    <div className="space-y-6">
      {/* Geo-Tags */}
      {mergedGeoTags.length > 0 && (
        <CollapsibleSection title="Geo-Tags" count={mergedGeoTags.length}>
          <GeoTagsDisplay tags={mergedGeoTags} />
        </CollapsibleSection>
      )}

      {/* Stopps */}
      {allStops.length > 0 && (
        <CollapsibleSection title="Stopps" count={allStops.length}>
          <StopsDisplay stops={allStops} />
        </CollapsibleSection>
      )}

      {/* Untergrund */}
      {mergedSurfaces.length > 0 && (
        <CollapsibleSection title="Untergrund">
          <SurfacesDisplay surfaces={mergedSurfaces} />
        </CollapsibleSection>
      )}
    </div>
  )
}

// ── Sub-renderers ─────────────────────────────────────────────────────────────

function GeoTagsDisplay({ tags }: { tags: GeoTag[] }) {
  // Group tags by normalized category key
  const grouped = new Map<string, GeoTag[]>()
  for (const tag of tags) {
    const key = getGroupKey(tag.category)
    if (!grouped.has(key)) grouped.set(key, [])
    grouped.get(key)!.push(tag)
  }

  const sortedGroups = [...grouped.entries()].sort(
    (a, b) => getConfig(a[0]).order - getConfig(b[0]).order
  )

  return (
    <div className="space-y-4">
      {sortedGroups.map(([groupKey, items]) => {
        const config = getConfig(groupKey)
        return (
          <div key={groupKey}>
            <h3 className="flex items-center gap-1.5 text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-2">
              <span className="text-neutral-400 dark:text-neutral-500">{config.icon}</span>
              {config.label}
            </h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
              {items.map(tag => {
                const poiType = groupKey === 'gastronomy'
                  ? (tag.meta?.poi_type as string | undefined)
                  : undefined
                return (
                  <div
                    key={tag.id}
                    className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 space-y-1"
                  >
                    <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                      {tag.name}
                      {(tag.ele_m ?? tag.ele) != null && (groupKey === 'summit' || groupKey === 'pass') && (
                        <span className="ml-1.5 text-neutral-500 dark:text-neutral-400 font-normal">
                          ({Math.round(tag.ele_m ?? tag.ele!)} m)
                        </span>
                      )}
                    </div>
                    <div className="flex flex-wrap gap-3 text-xs text-neutral-500 dark:text-neutral-400">
                      {poiType && (
                        <span>{POI_TYPE_LABELS[poiType] ?? poiType}</span>
                      )}
                      {(tag.ele_m ?? tag.ele) != null && groupKey !== 'summit' && groupKey !== 'pass' && (
                        <span>{Math.round(tag.ele_m ?? tag.ele!)} m</span>
                      )}
                      {tag.dist_m != null && (
                        <span>
                          {tag.dist_m >= 1000
                            ? `${(tag.dist_m / 1000).toFixed(1)} km`
                            : `${Math.round(tag.dist_m)} m`}
                        </span>
                      )}
                      {tag.duration_min != null && (
                        <span>{Math.round(tag.duration_min)} min</span>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        )
      })}
    </div>
  )
}

function StopsDisplay({ stops }: { stops: ActivityStop[] }) {
  return (
    <div className="space-y-1.5">
      {stops.map(stop => {
        const title = stop.poi_name ?? stop.name ?? 'Aufenthalt'
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
  )
}

function SurfacesDisplay({ surfaces }: { surfaces: ActivitySurface[] }) {
  return (
    <div className="space-y-3">
      {/* Stacked bar */}
      <div className="flex h-4 rounded-full overflow-hidden gap-0.5">
        {surfaces.map(s => (
          <div
            key={s.id}
            style={{
              width: `${s.percentage}%`,
              backgroundColor: SURFACE_COLORS[s.surface_type] ?? '#9ca3af',
            }}
            title={`${SURFACE_LABELS[s.surface_type] ?? s.surface_type}: ${s.percentage.toFixed(0)}%`}
          />
        ))}
      </div>

      {/* Legend */}
      <div className="flex flex-wrap gap-3 text-xs text-neutral-600 dark:text-neutral-400">
        {surfaces.map(s => (
          <span key={s.id} className="flex items-center gap-1.5">
            <span
              className="w-2.5 h-2.5 rounded-full inline-block"
              style={{ backgroundColor: SURFACE_COLORS[s.surface_type] ?? '#9ca3af' }}
            />
            {SURFACE_LABELS[s.surface_type] ?? s.surface_type}
            <span className="tabular-nums">{s.percentage.toFixed(0)}%</span>
            {s.length_m >= 1000
              ? <span className="text-neutral-400">({(s.length_m / 1000).toFixed(1)} km)</span>
              : <span className="text-neutral-400">({Math.round(s.length_m)} m)</span>
            }
          </span>
        ))}
      </div>
    </div>
  )
}
