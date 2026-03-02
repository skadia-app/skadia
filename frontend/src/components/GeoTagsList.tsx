import { useQuery } from '@tanstack/react-query'
import { fetchGeoTags } from '@/api/enrichment'
import type { GeoTag } from '@/api/types'

interface GeoTagsListProps {
  activityId: string
}

const categoryLabels: Record<string, string> = {
  summit: 'Gipfel',
  place: 'Orte',
  water: 'Gewaesser',
  pass: 'Paesse',
  poi: 'Sehenswuerdigkeiten',
  stop: 'Stopps',
}

function categoryLabel(category: string): string {
  return categoryLabels[category] ?? category
}

export default function GeoTagsList({ activityId }: GeoTagsListProps) {
  const { data: tags, isLoading } = useQuery({
    queryKey: ['geo-tags', activityId],
    queryFn: () => fetchGeoTags(activityId),
  })

  if (isLoading || !tags || tags.length === 0) return null

  const grouped = tags.reduce<Record<string, GeoTag[]>>((acc, tag) => {
    const key = tag.category
    if (!acc[key]) acc[key] = []
    acc[key].push(tag)
    return acc
  }, {})

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Geo-Tags</h2>
      {Object.entries(grouped).map(([category, items]) => (
        <div key={category}>
          <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-2">
            {categoryLabel(category)}
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
            {items.map(tag => (
              <div
                key={tag.id}
                className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 space-y-1"
              >
                <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                  {tag.name}
                </div>
                <div className="flex flex-wrap gap-3 text-xs text-neutral-500 dark:text-neutral-400">
                  {tag.ele != null && <span>{Math.round(tag.ele)} m</span>}
                  {tag.dist_m != null && (
                    <span>
                      {tag.dist_m >= 1000
                        ? `${(tag.dist_m / 1000).toFixed(1)} km`
                        : `${Math.round(tag.dist_m)} m`}
                    </span>
                  )}
                  {tag.duration_min != null && <span>{Math.round(tag.duration_min)} min</span>}
                </div>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
