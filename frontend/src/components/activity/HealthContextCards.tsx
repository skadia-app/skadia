import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { fetchHealthContext } from '@/api/activities'
import type { HealthContext } from '@/api/types'
import { isSupportedLanguage, localeForLanguage } from '@/i18n/languages'

interface HealthContextCardsProps {
  activityId: string
}

function activeLocale(language: string): string {
  const languageCode = language.split('-')[0]
  return localeForLanguage(isSupportedLanguage(languageCode) ? languageCode : 'de')
}

function formatInteger(value: number, locale: string): string {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(Math.round(value))
}

function formatDurationMin(
  totalMin: number,
  locale: string,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  const number = new Intl.NumberFormat(locale, { maximumFractionDigits: 0 })
  const h = Math.floor(totalMin / 60)
  const m = totalMin % 60
  if (h > 0) {
    return t('healthContextCards.duration.hoursMinutes', {
      hours: number.format(h),
      minutes: number.format(m),
    })
  }
  return t('healthContextCards.duration.minutes', { minutes: number.format(m) })
}

function formatHrvTime(ts: string, locale: string): string {
  // ts is a full timestamp string — extract HH:MM
  try {
    const d = new Date(ts)
    if (isNaN(d.getTime())) return ts
    return d.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' })
  } catch {
    return ts
  }
}

function scoreColor(score: number): string {
  if (score >= 80) return 'text-green-600 dark:text-green-400'
  if (score >= 60) return 'text-amber-500 dark:text-amber-400'
  return 'text-red-500 dark:text-red-400'
}

function scoreBg(score: number): string {
  if (score >= 80) return 'bg-green-500'
  if (score >= 60) return 'bg-amber-400'
  return 'bg-red-500'
}

interface SleepCardProps {
  hc: HealthContext
}

function SleepCard({ hc }: SleepCardProps) {
  const { t, i18n } = useTranslation()
  const locale = activeLocale(i18n.language)
  const totalMin = hc.sleep_duration_min ?? 0
  const deepMin = hc.deep_sleep_min ?? 0
  const remMin = hc.rem_sleep_min ?? 0
  const lightMin = totalMin > 0 ? Math.max(0, totalMin - deepMin - remMin) : 0

  const deepPct = totalMin > 0 ? (deepMin / totalMin) * 100 : 0
  const remPct = totalMin > 0 ? (remMin / totalMin) * 100 : 0
  const lightPct = totalMin > 0 ? (lightMin / totalMin) * 100 : 0
  const deepSleepLabel = t('healthCharts.sleepStages.deep')
  const lightSleepLabel = t('healthCharts.sleepStages.light')
  const remSleepLabel = t('healthCharts.sleepStages.rem')
  const deepDuration = formatDurationMin(deepMin, locale, t)
  const remDuration = formatDurationMin(remMin, locale, t)
  const lightDuration = formatDurationMin(lightMin, locale, t)

  return (
    <div className="flex-1 min-w-0 rounded-lg border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 space-y-1.5">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          {t('healthDashboard.charts.sleep')}
        </span>
        {hc.sleep_score != null && (
          <span className={`text-xs font-semibold tabular-nums ${scoreColor(hc.sleep_score)}`}>
            {hc.sleep_score}/100
          </span>
        )}
      </div>

      {hc.sleep_duration_min != null && (
        <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          {formatDurationMin(hc.sleep_duration_min, locale, t)}
        </div>
      )}

      {/* Sleep stage breakdown bar */}
      {totalMin > 0 && (deepMin > 0 || remMin > 0) && (
        <div className="space-y-1">
          <div className="flex h-1.5 rounded-full overflow-hidden gap-px">
            {deepPct > 0 && (
              <div
                className="bg-indigo-600 dark:bg-indigo-500"
                style={{ width: `${deepPct}%` }}
                title={t('healthContextCards.stageTooltip', { stage: deepSleepLabel, duration: deepDuration })}
              />
            )}
            {remPct > 0 && (
              <div
                className="bg-violet-500 dark:bg-violet-400"
                style={{ width: `${remPct}%` }}
                title={t('healthContextCards.stageTooltip', { stage: remSleepLabel, duration: remDuration })}
              />
            )}
            {lightPct > 0 && (
              <div
                className="bg-sky-300 dark:bg-sky-600"
                style={{ width: `${lightPct}%` }}
                title={t('healthContextCards.stageTooltip', { stage: lightSleepLabel, duration: lightDuration })}
              />
            )}
          </div>
          <div className="flex gap-3 text-[10px] text-neutral-500 dark:text-neutral-400">
            {deepMin > 0 && (
              <span className="flex items-center gap-1">
                <span className="inline-block w-2 h-2 rounded-sm bg-indigo-600 dark:bg-indigo-500" />
                {deepSleepLabel} {deepDuration}
              </span>
            )}
            {remMin > 0 && <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-violet-500 dark:bg-violet-400" />{remSleepLabel} {remDuration}</span>}
          </div>
        </div>
      )}

      {/* Score bar */}
      {hc.sleep_score != null && (
        <div className="h-1 rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-hidden">
          <div
            className={`h-full rounded-full transition-all ${scoreBg(hc.sleep_score)}`}
            style={{ width: `${hc.sleep_score}%` }}
          />
        </div>
      )}
    </div>
  )
}

interface HrvCardProps {
  hc: HealthContext
}

function HrvCard({ hc }: HrvCardProps) {
  const { i18n } = useTranslation()
  const locale = activeLocale(i18n.language)
  return (
    <div className="flex-1 min-w-0 rounded-lg border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 space-y-1">
      <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">HRV</div>
      <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
        {formatInteger(hc.hrv_rmssd ?? 0, locale)} <span className="text-xs font-normal text-neutral-500 dark:text-neutral-400">ms</span>
      </div>
      {hc.hrv_measured_at && (
        <div className="text-[11px] text-neutral-400 dark:text-neutral-500">
          {formatHrvTime(hc.hrv_measured_at, locale)}
        </div>
      )}
    </div>
  )
}

interface RestingHrCardProps {
  hc: HealthContext
}

function RestingHrCard({ hc }: RestingHrCardProps) {
  const { t, i18n } = useTranslation()
  const locale = activeLocale(i18n.language)
  return (
    <div className="flex-1 min-w-0 rounded-lg border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 space-y-1">
      <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
        {t('analyticsHealthInsights.factorNames.restingHr')}
      </div>
      <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
        {formatInteger(hc.resting_hr ?? 0, locale)} <span className="text-xs font-normal text-neutral-500 dark:text-neutral-400">bpm</span>
      </div>
    </div>
  )
}

export function HealthContextCards({ activityId }: HealthContextCardsProps) {
  const { data: hc } = useQuery({
    queryKey: ['healthContext', activityId],
    queryFn: () => fetchHealthContext(activityId),
    staleTime: 5 * 60 * 1000,
  })

  if (!hc) return null

  const hasSleep = hc.sleep_duration_min != null || hc.sleep_score != null
  const hasHrv = hc.hrv_rmssd != null
  const hasRhr = hc.resting_hr != null

  if (!hasSleep && !hasHrv && !hasRhr) return null

  return (
    <div className="flex flex-wrap gap-3">
      {hasSleep && <SleepCard hc={hc} />}
      {hasHrv && <HrvCard hc={hc} />}
      {hasRhr && <RestingHrCard hc={hc} />}
    </div>
  )
}
