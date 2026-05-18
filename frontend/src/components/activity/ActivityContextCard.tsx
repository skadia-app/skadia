import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import type { Activity } from '@/api/types'
import type { BestEffortRanked, NearMiss } from '@/api/bestEfforts'
import { updateActivity } from '@/api/activities'
import { activityCountryCodes, countryCodeToFlag } from '@/lib/countryFlags'
import { displayActivityTitle } from '@/lib/activityTitle'
import { fmtActivityDate, fmtTime } from '@/lib/format'
import { sportDetailLabel } from '@/lib/sportTaxonomy'
import TagsBadges from '@/components/activity/TagsBadges'
import WeatherBadge from '@/components/WeatherBadge'
import Toggle from '@/components/Toggle'

interface ActivityContextCardProps {
  activity: Activity
  bestEfforts: BestEffortRanked[]
  nearMisses: NearMiss[]
  weatherDisplayFields?: string[]
}

function sectionTitle(label: string) {
  return <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">{label}</div>
}

function platformBadge(label: string, letter: string, href: string, color: string) {
  return (
    <a key={label} href={href} target="_blank" rel="noopener noreferrer" className="flex items-center justify-between gap-2 rounded-md px-0.5 py-1 text-sm hover:bg-neutral-50 dark:hover:bg-neutral-800">
      <span className="flex items-center gap-2">
        <span className={`inline-flex h-4 w-4 items-center justify-center rounded text-[10px] font-bold text-white ${color}`}>{letter}</span>
        <span className="font-medium text-neutral-800 dark:text-neutral-100">{label}</span>
      </span>
      <span className="text-neutral-400">↗</span>
    </a>
  )
}

function bestEffortLabel(category: string): string {
  switch (category) {
    case 'longest_duration': return 'Längste Dauer'
    case 'longest_distance': return 'Längste Distanz'
    case 'most_elevation': return 'Meiste Höhenmeter'
    default: return category.replaceAll('_', ' ')
  }
}

export default function ActivityContextCard({ activity, bestEfforts, nearMisses, weatherDisplayFields }: ActivityContextCardProps) {
  const queryClient = useQueryClient()
  const title = displayActivityTitle(activity, 'Kein Titel')
  const flags = activityCountryCodes(activity).map(code => countryCodeToFlag(code)).filter(Boolean)
  const timeRange = activity.ended_at ? `${fmtTime(activity.started_at, activity.timezone)} - ${fmtTime(activity.ended_at, activity.timezone)}` : fmtTime(activity.started_at, activity.timezone)
  const hasPlatforms = activity.garmin_activity_id || activity.strava_activity_id || activity.komoot_tour_id
  const commuteContext = activity.has_gps && activity.start_lat != null && activity.start_lon != null && activity.end_lat != null && activity.end_lon != null ? {
    hasGps: true,
    distanceM: activity.distance_m ?? 0,
    startLat: activity.start_lat,
    startLon: activity.start_lon,
    endLat: activity.end_lat,
    endLon: activity.end_lon,
  } : undefined

  async function setVisibility(field: 'exclude_from_heatmap' | 'exclude_from_stats', active: boolean) {
    const updated = await updateActivity(activity.id, { [field]: !active })
    queryClient.setQueryData(['activity', activity.id], updated)
    queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
  }

  return (
    <aside className="overflow-hidden rounded-lg border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
      <div className="space-y-2 bg-[#23211e] px-4 py-4 text-white">
        {activity.recording_group_id && (
          <span className="inline-flex rounded-full bg-orange-50 px-2 py-0.5 text-xs font-semibold text-orange-700">
            Autogruppierung
          </span>
        )}
        <div className="flex items-start justify-between gap-3">
          <h1 className="min-w-0 text-2xl font-semibold leading-tight">{title}</h1>
          {flags.length > 0 && <div className="shrink-0 text-lg leading-none">{flags.join(' ')}</div>}
        </div>
        <div className="text-sm text-neutral-300">
          {sportDetailLabel(activity.sport_type, activity.sub_type)} · {fmtActivityDate(activity.started_at, activity.timezone)} · {fmtTime(activity.started_at, activity.timezone)} Uhr
        </div>
      </div>

      <div className="space-y-4 px-4 py-4">
        <div className="space-y-2 text-sm">
          <div className="grid grid-cols-[72px_1fr] gap-2">
            <span className="text-neutral-500 dark:text-neutral-400">Gerät</span>
            <span className="font-semibold text-neutral-900 dark:text-neutral-100">{activity.recording_device_name || activity.source_device || 'Unbekannt'}</span>
          </div>
          <div className="grid grid-cols-[72px_1fr] gap-2">
            <span className="text-neutral-500 dark:text-neutral-400">Aufzeichnung</span>
            <span className="font-semibold text-neutral-900 dark:text-neutral-100">{timeRange}</span>
          </div>
          <div className="flex items-center gap-3 text-neutral-600 dark:text-neutral-300">
            {activity.sunrise_at && <span>☀ {fmtTime(activity.sunrise_at, activity.timezone)}</span>}
            {activity.sunset_at && <span>☾ {fmtTime(activity.sunset_at, activity.timezone)}</span>}
          </div>
          {(activity.start_city || activity.start_state || activity.start_country) && (
            <div className="flex items-start gap-2 text-neutral-600 dark:text-neutral-300">
              <span className="mt-0.5 text-red-500">•</span>
              <span>{[activity.start_city, activity.start_state, activity.start_country].filter(Boolean).join(' · ')}</span>
            </div>
          )}
        </div>

        <div className="border-t border-neutral-200 pt-3 dark:border-neutral-800">
          {sectionTitle('Tags')}
          <TagsBadges
            tags={activity.tags}
            activityId={activity.id}
            isVirtual={activity.is_virtual}
            commuteContext={commuteContext}
          />
        </div>

        <div className="border-t border-neutral-200 pt-3 dark:border-neutral-800">
          {sectionTitle('Sichtbarkeit')}
          <div className="space-y-2 text-sm">
            <div className="flex items-center justify-between gap-3 rounded-lg border border-neutral-200 px-3 py-2 dark:border-neutral-800">
              <span className="text-neutral-700 dark:text-neutral-200">Heatmap aktiv</span>
              <Toggle
                checked={!activity.exclude_from_heatmap}
                onChange={checked => void setVisibility('exclude_from_heatmap', checked)}
                size="sm"
                aria-label="Heatmap aktiv"
              />
            </div>
            <div className="flex items-center justify-between gap-3 rounded-lg border border-neutral-200 px-3 py-2 dark:border-neutral-800">
              <span className="text-neutral-700 dark:text-neutral-200">Statistik aktiv</span>
              <Toggle
                checked={!activity.exclude_from_stats}
                onChange={checked => void setVisibility('exclude_from_stats', checked)}
                size="sm"
                aria-label="Statistik aktiv"
              />
            </div>
          </div>
        </div>

        {hasPlatforms && (
          <div className="border-t border-neutral-200 pt-3 dark:border-neutral-800">
            {sectionTitle('Plattformen')}
            <div className="space-y-1">
              {activity.garmin_activity_id && platformBadge('Garmin Connect', 'G', `https://connect.garmin.com/app/activity/${activity.garmin_activity_id}`, 'bg-sky-600')}
              {activity.strava_activity_id && platformBadge('Strava', 'S', `https://www.strava.com/activities/${activity.strava_activity_id}`, 'bg-orange-600')}
              {activity.komoot_tour_id && platformBadge('Komoot', 'K', `https://www.komoot.com/tour/${activity.komoot_tour_id}`, 'bg-green-600')}
            </div>
          </div>
        )}

        {activity.weather_readings && activity.weather_readings.length > 0 && (
          <div className="border-t border-neutral-200 pt-3 dark:border-neutral-800">
            {sectionTitle('Wetter')}
            <WeatherBadge readings={activity.weather_readings} displayFields={weatherDisplayFields} timezone={activity.timezone} variant="context" />
          </div>
        )}

        {(bestEfforts.length > 0 || nearMisses.length > 0) && (
          <div className="border-t border-neutral-200 pt-3 dark:border-neutral-800">
            {sectionTitle('Bestleistungen')}
            <div className="rounded-lg border border-orange-200 bg-orange-50 px-3 py-2 text-sm dark:border-orange-900 dark:bg-orange-950/20">
              {bestEfforts[0] ? (
                <div className="flex items-center gap-3">
                  <span className="rounded-full bg-neutral-900 px-2 py-0.5 text-xs font-bold text-white">#{bestEfforts[0].rank}</span>
                  <div className="min-w-0">
                    <div className="font-semibold text-neutral-900 dark:text-neutral-100">{bestEffortLabel(bestEfforts[0].category)}</div>
                    <div className="text-xs text-neutral-500 dark:text-neutral-400">
                      {bestEfforts[0].percentile != null ? `Top ${bestEfforts[0].percentile}%` : 'Bestleistung'}
                    </div>
                  </div>
                </div>
              ) : (
                <span className="text-neutral-600 dark:text-neutral-300">Knapp verpasst</span>
              )}
            </div>
          </div>
        )}

        {activity.recording_device_id && (
          <Link to={`/hardware/detail/${activity.recording_device_id}`} className="block text-xs text-neutral-500 hover:text-orange-600">
            Gerätedetails öffnen →
          </Link>
        )}
      </div>
    </aside>
  )
}
