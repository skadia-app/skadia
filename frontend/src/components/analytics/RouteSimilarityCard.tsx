import { useEffect, useState } from 'react'
import { apiClient } from '@/api/client'
import { useNavigate } from 'react-router-dom'
import { MapPin } from 'lucide-react'

interface SimilarRoute {
  id: string
  title: string
  sport_type: string
  started_at: string
  hausdorff_dist: number
}

export default function RouteSimilarityCard({ activityId }: { activityId: string }) {
  const [routes, setRoutes] = useState<SimilarRoute[]>([])
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  useEffect(() => {
    setLoading(true)
    apiClient<SimilarRoute[]>(`/analytics/similar-routes?activity_id=${activityId}&max_results=5`)
      .then(setRoutes)
      .catch(() => setRoutes([]))
      .finally(() => setLoading(false))
  }, [activityId])

  if (loading) return null
  if (routes.length === 0) return null

  return (
    <div className="mt-4">
      <h3 className="text-sm font-semibold mb-2 flex items-center gap-1.5">
        <MapPin className="w-3.5 h-3.5" />
        Ähnliche Routen
      </h3>
      <div className="space-y-1.5">
        {routes.map(r => (
          <button
            key={r.id}
            onClick={() => navigate(`/activities/${r.id}`)}
            className="w-full flex items-center justify-between text-left px-3 py-2 rounded-lg bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors text-sm"
          >
            <div>
              <p className="font-medium truncate max-w-[200px]">{r.title}</p>
              <p className="text-xs text-neutral-500">
                {new Date(r.started_at).toLocaleDateString('de-DE')} · {r.sport_type}
              </p>
            </div>
            <span className="text-xs text-neutral-400 ml-2 flex-shrink-0">
              Δ{r.hausdorff_dist.toFixed(3)}°
            </span>
          </button>
        ))}
      </div>
    </div>
  )
}
