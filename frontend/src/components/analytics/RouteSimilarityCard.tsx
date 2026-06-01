import { useEffect, useState } from 'react'
import { apiClient } from '@/api/client'
import { useNavigate } from 'react-router-dom'
import { MapPin } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { sportLabel } from '@/lib/sportTaxonomy'
import { supportedLanguageFrom } from '@/i18n/localizedString'

interface SimilarRoute {
  id: string
  title: string
  sport_type: string
  started_at: string
  overlap_pct: number
}

export default function RouteSimilarityCard({ activityId }: { activityId: string }) {
  const [routes, setRoutes] = useState<SimilarRoute[]>([])
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)

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
        {t('analyticsHealthInsights.routeSimilarity.title')}
      </h3>
      <div className="space-y-1.5">
        {routes.map(r => (
          <button
            key={r.id}
            onClick={() => navigate(`/activities/${r.id}`)}
            className="w-full flex items-center justify-between text-left px-3 py-2 rounded-lg bg-neutral-50 dark:bg-neutral-800 hover:bg-neutral-100 dark:hover:bg-neutral-700 cursor-pointer transition-colors text-sm"
          >
            <div>
              <p className="font-medium truncate max-w-[200px]">{r.title || sportLabel(r.sport_type, undefined, { language })}</p>
              <p className="text-xs text-neutral-500">
                {new Date(r.started_at).toLocaleDateString(i18n.language)} · {sportLabel(r.sport_type, undefined, { language })}
              </p>
            </div>
            <span className="text-xs text-neutral-400 ml-2 flex-shrink-0">
              {Math.round(r.overlap_pct)}%
            </span>
          </button>
        ))}
      </div>
    </div>
  )
}
