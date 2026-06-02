import { Link } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import type { Activity } from '@/api/types'
import type { BestEffortRanked, NearMiss } from '@/api/bestEfforts'
import { activityCountryCodes, countryCodeToFlag } from '@/lib/countryFlags'
import { displayActivityTitle } from '@/lib/activityTitle'
import { fmtActivityDate, fmtTime } from '@/lib/format'
import { sportDetailLabel } from '@/lib/sportTaxonomy'
import { categoryLabel, formatBestEffortValue } from '@/lib/bestEffortLabels'
import TagsBadges from '@/components/activity/TagsBadges'
import ActivityGearSection from '@/components/activity/ActivityGearSection'
import WeatherBadge from '@/components/WeatherBadge'
import { SunInfo } from '@/components/activity/SunInfo'
import { isSupportedLanguage, localeForLanguage, type SupportedLanguage } from '@/i18n/languages'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import type { UnitFormatOptions } from '@/lib/units'

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

function currentLanguage(language: string): SupportedLanguage {
  const normalized = language.split('-')[0]
  return isSupportedLanguage(normalized) ? normalized : 'de'
}

function bestEffortLabel(category: string, t: TFunction, language: SupportedLanguage): string {
  switch (category) {
    case 'longest_duration': return t('activityContext.bestEfforts.longestDuration')
    case 'longest_distance': return t('activityContext.bestEfforts.longestDistance')
    case 'most_elevation': return t('activityContext.bestEfforts.mostElevation')
    default: return categoryLabel(category, language)
  }
}

function bestEffortTopLabel(percentile: number | null | undefined, locale: string, t: TFunction): string | null {
  if (percentile == null) return null
  const normalized = Math.min(1, Math.max(0, percentile))
  const topPercent = normalized >= 1 ? 1 : Math.ceil((1 - normalized) * 100)
  return t('activityContext.bestEfforts.topPercent', { percent: topPercent.toLocaleString(locale) })
}

function bestEffortValueLabel(effort: BestEffortRanked, unitOptions: UnitFormatOptions): string | null {
  const value = formatBestEffortValue(effort.category, effort.elapsed_time_s, effort.value_numeric, unitOptions)
  return value === '—' ? null : value
}

function activityLocationText(activity: Activity): string | null {
  const { start_city, end_city, start_state, start_country } = activity
  const region = [start_state, start_country].filter(Boolean).join(', ')
  if (!start_city) return end_city ? (region ? `${end_city} · ${region}` : end_city) : (region || null)

  const place = end_city && end_city !== start_city ? `${start_city} → ${end_city}` : start_city
  return region ? `${place} · ${region}` : place
}

function MetaItem({ label, children, className = '', valueClassName = 'truncate' }: { label: string; children: ReactNode; className?: string; valueClassName?: string }) {
  return (
    <div className={`min-w-0 ${className}`}>
      <div className="text-[11px] font-semibold uppercase tracking-wide text-neutral-400 dark:text-neutral-500">{label}</div>
      <div className={`${valueClassName} text-sm font-medium text-neutral-800 dark:text-neutral-100`}>{children}</div>
    </div>
  )
}

export default function ActivityContextCard({ activity, bestEfforts, nearMisses, weatherDisplayFields }: ActivityContextCardProps) {
  const { t, i18n } = useTranslation()
  const unitFormatters = useUnitFormatters()
  const language = currentLanguage(i18n.language)
  const locale = localeForLanguage(language)
  const title = displayActivityTitle(activity, t('activityActions.noTitle'))
  const flags = activityCountryCodes(activity).map(code => countryCodeToFlag(code)).filter(Boolean)
  const timeRange = activity.ended_at
    ? `${fmtTime(activity.started_at, activity.timezone, locale)} - ${fmtTime(activity.ended_at, activity.timezone, locale)}`
    : fmtTime(activity.started_at, activity.timezone, locale)
  const locationText = activityLocationText(activity)
  const hasPlatforms = activity.garmin_activity_id || activity.strava_activity_id || activity.komoot_tour_id
  const commuteContext = activity.has_gps && activity.start_lat != null && activity.start_lon != null && activity.end_lat != null && activity.end_lon != null ? {
    hasGps: true,
    distanceM: activity.distance_m ?? 0,
    startLat: activity.start_lat,
    startLon: activity.start_lon,
    endLat: activity.end_lat,
    endLon: activity.end_lon,
  } : undefined

  return (
    <aside className="overflow-hidden rounded-lg border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
      <div className="space-y-2 bg-[#23211e] px-4 py-4 text-white">
        {activity.recording_group_id && (
          <span className="inline-flex rounded-full bg-orange-50 px-2 py-0.5 text-xs font-semibold text-orange-700">
            {t('activityContext.autoGrouped')}
          </span>
        )}
        <div className="flex items-start justify-between gap-3">
          <h1 className="min-w-0 text-2xl font-semibold leading-tight">{title}</h1>
          {flags.length > 0 && <div className="shrink-0 text-lg leading-none">{flags.join(' ')}</div>}
        </div>
        <div className="space-y-0.5">
          <div className="text-sm font-semibold text-neutral-100">
            {sportDetailLabel(activity.sport_type, activity.sub_type, language)}
          </div>
          <div className="text-sm text-neutral-300">
            {fmtActivityDate(activity.started_at, activity.timezone, locale)} · {t('activityActions.timeOfDay', { time: fmtTime(activity.started_at, activity.timezone, locale) })}
          </div>
        </div>
      </div>

      <div className="space-y-4 px-4 py-4">
        <div className="space-y-3 border-b border-neutral-200 pb-4 dark:border-neutral-800">
          {locationText && (
            <div className="flex items-start gap-2 text-sm text-neutral-700 dark:text-neutral-200">
              <span className="mt-0.5 text-base leading-none" aria-hidden>📍</span>
              <span className="min-w-0 leading-snug">{locationText}</span>
            </div>
          )}

          <div className="space-y-3">
            <MetaItem label={t('activityContext.device')} className="col-span-2" valueClassName="leading-snug">
              {activity.recording_device_id ? (
                <Link
                  to={`/hardware/detail/${activity.recording_device_id}`}
                  className="break-words hover:text-orange-600"
                >
                  {activity.recording_device_name || activity.source_device || t('common.unknown')}
                </Link>
              ) : (
                activity.recording_device_name || activity.source_device || t('common.unknown')
            )}
            </MetaItem>
            <MetaItem label={t('activityContext.recording')}>{timeRange}</MetaItem>
          </div>
        </div>

        {((activity.sunrise_at || activity.sunset_at) || (activity.weather_readings && activity.weather_readings.length > 0)) && (
          <div>
            {sectionTitle(t('activityContext.weather'))}
            <div className="space-y-2">
              {(activity.sunrise_at || activity.sunset_at) && (
                <div className="border-b border-neutral-100 pb-2.5 dark:border-neutral-800">
                  <SunInfo sunriseAt={activity.sunrise_at} sunsetAt={activity.sunset_at} timezone={activity.timezone} />
                </div>
              )}
              {activity.weather_readings && activity.weather_readings.length > 0 && (
                <WeatherBadge readings={activity.weather_readings} displayFields={weatherDisplayFields} timezone={activity.timezone} variant="context" />
              )}
            </div>
          </div>
        )}

        <div>
          {sectionTitle(t('activityContext.tags'))}
          <TagsBadges
            tags={activity.tags}
            activityId={activity.id}
            isVirtual={activity.is_virtual}
            commuteContext={commuteContext}
          />
        </div>

        <div className="border-t border-neutral-200 pt-3 dark:border-neutral-800">
          {sectionTitle(t('activityContext.gear'))}
          <ActivityGearSection activityId={activity.id} assignments={activity.gear_assignments} />
        </div>

        {hasPlatforms && (
          <div className="border-t border-neutral-200 pt-3 dark:border-neutral-800">
            {sectionTitle(t('activityContext.platforms'))}
            <div className="space-y-1">
              {activity.garmin_activity_id && platformBadge('Garmin Connect', 'G', `https://connect.garmin.com/app/activity/${activity.garmin_activity_id}`, 'bg-sky-600')}
              {activity.strava_activity_id && platformBadge('Strava', 'S', `https://www.strava.com/activities/${activity.strava_activity_id}`, 'bg-orange-600')}
              {activity.komoot_tour_id && platformBadge('Komoot', 'K', `https://www.komoot.com/tour/${activity.komoot_tour_id}`, 'bg-green-600')}
            </div>
          </div>
        )}

        {(bestEfforts.length > 0 || nearMisses.length > 0) && (
          <div className="border-t border-neutral-200 pt-3 dark:border-neutral-800">
            {sectionTitle(t('activityContext.bestEfforts.title'))}
            <div className="rounded-lg border border-orange-200 bg-orange-50 px-3 py-2 text-sm dark:border-orange-900 dark:bg-orange-950/20">
              {bestEfforts[0] ? (
                <div className="flex items-center gap-3">
                  <span className="rounded-full bg-neutral-900 px-2 py-0.5 text-xs font-bold text-white">#{bestEfforts[0].rank}</span>
                  <div className="min-w-0">
                    <div className="font-semibold text-neutral-900 dark:text-neutral-100">{bestEffortLabel(bestEfforts[0].category, t, language)}</div>
                    <div className="text-xs text-neutral-500 dark:text-neutral-400">
                      {[
                        bestEffortTopLabel(bestEfforts[0].percentile, locale, t),
                        bestEffortValueLabel(bestEfforts[0], unitFormatters.options),
                      ].filter(Boolean).join(' · ')}
                    </div>
                  </div>
                </div>
              ) : (
                <span className="text-neutral-600 dark:text-neutral-300">{t('activityContext.bestEfforts.nearMiss')}</span>
              )}
            </div>
          </div>
        )}

      </div>
    </aside>
  )
}
