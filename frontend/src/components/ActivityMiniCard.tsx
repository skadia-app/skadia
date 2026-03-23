import { Link } from 'react-router-dom'
import { sportLabel, sportColor } from '@/lib/sportTaxonomy'
import StaticTrackMap from '@/components/StaticTrackMap'
import type { PhotoFeedActivitySummary } from '@/api/photos'
import { fmtDuration, fmtDist } from '@/lib/format'

function formatDate(iso?: string): string {
  if (!iso) return ''
  return new Date(iso).toLocaleDateString('de-DE', {
    day: 'numeric', month: 'short', year: 'numeric',
  })
}

interface Props {
  activity: PhotoFeedActivitySummary
  photoCount: number
}

export default function ActivityMiniCard({ activity, photoCount }: Props) {
  const color = sportColor(activity.sport_type)
  const label = sportLabel(activity.sport_type)

  return (
    <div className="flex flex-col gap-3">
      {/* Track-Karte oder Placeholder */}
      <div className="rounded-lg overflow-hidden bg-neutral-100 dark:bg-neutral-800 aspect-[5/3] relative">
        {activity.track_geojson ? (
          <StaticTrackMap
            geojsonStr={activity.track_geojson}
            color={color}
            className="w-full h-full"
          />
        ) : activity.lat && activity.lng ? (
          <div className="w-full h-full flex items-center justify-center">
            <svg className="w-8 h-8 text-neutral-400" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7zm0 9.5c-1.38 0-2.5-1.12-2.5-2.5s1.12-2.5 2.5-2.5 2.5 1.12 2.5 2.5-1.12 2.5-2.5 2.5z"/>
            </svg>
          </div>
        ) : (
          <div className="w-full h-full flex items-center justify-center">
            <svg className="w-8 h-8 text-neutral-300 dark:text-neutral-600" viewBox="0 0 24 24" fill="currentColor">
              <path d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/>
            </svg>
          </div>
        )}
      </div>

      {/* Aktivitätsinfos */}
      <div className="flex flex-col gap-1.5 min-w-0">
        <div
          className="text-xs font-medium px-1.5 py-0.5 rounded-full self-start"
          style={{ backgroundColor: color + '22', color }}
        >
          {label}
        </div>

        <Link
          to={`/activities/${activity.id}`}
          className="text-sm font-semibold text-neutral-900 dark:text-neutral-100 hover:text-accent-DEFAULT line-clamp-2 leading-tight transition-colors"
        >
          {activity.title || 'Aktivität'}
        </Link>

        <div className="text-xs text-neutral-500 dark:text-neutral-400 space-y-0.5">
          {activity.started_at && <div>{formatDate(activity.started_at)}</div>}
          <div className="flex flex-wrap gap-x-2">
            {activity.distance_m && <span>{fmtDist(activity.distance_m)}</span>}
            {activity.moving_time_s && <span>{fmtDuration(activity.moving_time_s, 'compact')}</span>}
            {activity.elevation_gain_m && activity.elevation_gain_m > 5 && (
              <span>↑{Math.round(activity.elevation_gain_m)} m</span>
            )}
          </div>
        </div>

        <div className="text-xs text-neutral-400 dark:text-neutral-500">
          {photoCount} {photoCount === 1 ? 'Foto' : 'Fotos'}
        </div>
      </div>
    </div>
  )
}
