import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchSummitActivities, type SummitVisit } from '@/api/summits'
import { sportLabel } from '@/lib/sportTaxonomy'
import { fmtDist } from '@/lib/format'

interface SummitActivityListProps {
  summit: SummitVisit
}

export default function SummitActivityList({ summit }: SummitActivityListProps) {
  const navigate = useNavigate()

  const { data, isLoading, error } = useQuery({
    queryKey: ['summit-activities', summit.osm_id, summit.name],
    queryFn: () => fetchSummitActivities(summit.osm_id, summit.name),
    staleTime: 5 * 60 * 1000,
  })

  const activities = data ?? []

  return (
    <div className="flex flex-col gap-2">
      {isLoading && (
        <div className="text-sm text-neutral-500 dark:text-neutral-400">Lädt...</div>
      )}
      {!isLoading && error && (
        <div className="text-sm text-red-500 dark:text-red-400">
          Aktivitäten konnten nicht geladen werden.
        </div>
      )}
      {!isLoading && !error && activities.length === 0 && (
        <div className="text-sm text-neutral-500 dark:text-neutral-400">Keine Aktivitäten gefunden.</div>
      )}
      <div className="flex flex-col gap-2 max-h-72 overflow-y-auto">
        {activities.map(act => (
          <button
            key={act.activity_id}
            onClick={() => navigate(`/activities/${act.activity_id}`)}
            className="text-left p-3 border border-neutral-200 dark:border-neutral-700 rounded-lg
                       hover:border-neutral-300 dark:hover:border-neutral-600
                       hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors"
          >
            <div className="font-medium text-sm truncate text-neutral-900 dark:text-neutral-100">
              {act.title || 'Aktivität'}
            </div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400 flex flex-wrap gap-2 mt-1">
              <span>{new Date(act.started_at).toLocaleDateString('de-DE')}</span>
              <span>·</span>
              <span>{sportLabel(act.sport_type)}</span>
              {act.distance_m != null && (
                <>
                  <span>·</span>
                  <span>{fmtDist(act.distance_m)}</span>
                </>
              )}
              {act.elevation_gain_m != null && (
                <>
                  <span>·</span>
                  <span>↑{Math.round(act.elevation_gain_m)} m</span>
                </>
              )}
            </div>
          </button>
        ))}
      </div>
      {!isLoading && activities.length > 0 && (
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          {activities.length} Aktivität{activities.length !== 1 ? 'en' : ''}
        </div>
      )}
    </div>
  )
}
