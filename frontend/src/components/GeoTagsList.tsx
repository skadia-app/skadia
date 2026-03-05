import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchGeoTags } from '@/api/enrichment'
import type { GeoTag } from '@/api/types'

interface GeoTagsListProps {
  activityId: string
}

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
  settlement: {
    label: 'Ortschaften',
    order: 4,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 21h18M5 21V7l7-4 7 4v14M9 21v-4h6v4" />
      </svg>
    ),
  },
  still_water: {
    label: 'Gewässer',
    order: 5,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M2 12c2-2 4-2 6 0s4 2 6 0 4-2 6 0M2 17c2-2 4-2 6 0s4 2 6 0 4-2 6 0" />
      </svg>
    ),
  },
  flowing_water: {
    label: 'Gewässer',
    order: 5,
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M2 12c2-2 4-2 6 0s4 2 6 0 4-2 6 0M2 17c2-2 4-2 6 0s4 2 6 0 4-2 6 0" />
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

function getGroupKey(category: string): string {
  if (category === 'summit' || category === 'peak') return 'summit'
  if (category === 'still_water' || category === 'flowing_water' || category === 'water') return 'water'
  return category
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

function formatPoiType(poiType: string): string {
  return POI_TYPE_LABELS[poiType] ?? poiType
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

export default function GeoTagsList({ activityId }: GeoTagsListProps) {
  const [collapsed, setCollapsed] = useState(false)

  const { data: tags, isLoading } = useQuery({
    queryKey: ['geo-tags', activityId],
    queryFn: () => fetchGeoTags(activityId),
  })

  if (isLoading || !tags || tags.length === 0) return null

  // Group tags by normalized category key
  const grouped = new Map<string, GeoTag[]>()
  for (const tag of tags) {
    const key = getGroupKey(tag.category)
    if (!grouped.has(key)) grouped.set(key, [])
    grouped.get(key)!.push(tag)
  }

  // Sort groups by configured order
  const sortedGroups = [...grouped.entries()].sort(
    (a, b) => getConfig(a[0]).order - getConfig(b[0]).order
  )

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
          Geo-Tags
        </h2>
        <span className="text-sm text-neutral-400">({tags.length})</span>
      </button>

      {!collapsed && (
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
                          {tag.ele != null && (groupKey === 'summit' || groupKey === 'pass') && (
                            <span className="ml-1.5 text-neutral-500 dark:text-neutral-400 font-normal">
                              ({Math.round(tag.ele)} m)
                            </span>
                          )}
                        </div>
                        <div className="flex flex-wrap gap-3 text-xs text-neutral-500 dark:text-neutral-400">
                          {poiType && (
                            <span>{formatPoiType(poiType)}</span>
                          )}
                          {tag.ele != null && groupKey !== 'summit' && groupKey !== 'pass' && (
                            <span>{Math.round(tag.ele)} m</span>
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
      )}
    </div>
  )
}
