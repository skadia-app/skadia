import { useTranslation } from 'react-i18next'
import type { Activity, DiveInfo } from '@/api/types'
import { fmtDuration } from '@/lib/format'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { ActivityStatCard } from './ActivityStatCard'
import { isSupportedLanguage, localeForLanguage, type SupportedLanguage } from '@/i18n/languages'

interface Props {
  activity: Activity
  diveInfo?: DiveInfo
}

function currentLanguage(language: string): SupportedLanguage {
  const normalized = language.split('-')[0]
  return isSupportedLanguage(normalized) ? normalized : 'de'
}

function fmtTime(isoString: string | undefined, locale: string): string | null {
  if (!isoString) return null
  return new Date(isoString).toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' })
}

function fmtWaterType(waterType: string, t: (key: string) => string): string {
  if (waterType === 'fresh') return t('activityDetail.dive.waterTypes.fresh')
  if (waterType === 'salt') return t('activityDetail.dive.waterTypes.salt')
  if (waterType === 'en13319') return 'EN 13319'
  return waterType
}

export function DiveStats({ activity, diveInfo }: Props) {
  const { t, i18n } = useTranslation()
  const { options, formatElevation } = useUnitFormatters()
  const locale = localeForLanguage(currentLanguage(i18n.language))
  const entryTime = fmtTime(activity.started_at, locale)
  const exitTime = fmtTime(activity.ended_at, locale)
  const formatDepth = (meters: number) => {
    if (options.units === 'imperial') return formatElevation(meters)
    return `${new Intl.NumberFormat(options.locale, {
      minimumFractionDigits: 1,
      maximumFractionDigits: 1,
    }).format(meters)} m`
  }
  const formatPercent = (value: number) => `${new Intl.NumberFormat(options.locale, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  }).format(value)} %`

  const stats = [
    activity.max_depth_m != null ? { label: t('activityDetail.dive.maxDepth'), value: formatDepth(activity.max_depth_m) } : null,
    activity.avg_depth_m != null ? { label: t('activityDetail.dive.avgDepth'), value: formatDepth(activity.avg_depth_m) } : null,
    activity.dive_number != null ? { label: t('activityDetail.dive.diveNumber'), value: `${activity.dive_number}` } : null,
    entryTime != null ? { label: t('activityDetail.dive.entryTime'), value: entryTime } : null,
    exitTime != null ? { label: t('activityDetail.dive.exitTime'), value: exitTime } : null,
    diveInfo?.settings?.water_type
      ? { label: t('activityDetail.dive.waterBodyType'), value: fmtWaterType(diveInfo.settings.water_type, t) }
      : null,
    activity.start_cns_pct != null ? { label: t('activityDetail.dive.cnsStart'), value: formatPercent(activity.start_cns_pct) } : null,
    activity.end_cns_pct != null ? { label: t('activityDetail.dive.cnsEnd'), value: formatPercent(activity.end_cns_pct) } : null,
    activity.start_n2_pct != null ? { label: t('activityDetail.dive.n2Start'), value: formatPercent(activity.start_n2_pct) } : null,
    activity.end_n2_pct != null ? { label: t('activityDetail.dive.n2End'), value: formatPercent(activity.end_n2_pct) } : null,
    activity.o2_toxicity_otu != null ? { label: t('activityDetail.dive.o2ToxicityOtu'), value: `${Math.round(activity.o2_toxicity_otu)}` } : null,
    diveInfo?.po2_stats != null ? {
      label: t('activityDetail.dive.po2Min'),
      value: diveInfo.po2_stats.min_po2_depth_m != null
        ? `${diveInfo.po2_stats.min_po2.toFixed(2)} bar @ ${formatDepth(diveInfo.po2_stats.min_po2_depth_m)}`
        : `${diveInfo.po2_stats.min_po2.toFixed(2)} bar`,
    } : null,
    diveInfo?.po2_stats != null ? {
      label: t('activityDetail.dive.po2Max'),
      value: diveInfo.po2_stats.max_po2_depth_m != null
        ? `${diveInfo.po2_stats.max_po2.toFixed(2)} bar @ ${formatDepth(diveInfo.po2_stats.max_po2_depth_m)}`
        : `${diveInfo.po2_stats.max_po2.toFixed(2)} bar`,
    } : null,
    activity.surface_interval_s != null ? { label: t('activityDetail.dive.surfaceInterval'), value: fmtDuration(activity.surface_interval_s, 'short') } : null,
  ].filter(Boolean) as { label: string; value: string }[]

  if (stats.length === 0) return null

  return (
    <div>
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-2">{t('activityDetail.dive.title')}</h3>
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
        {stats.map(s => (
          <ActivityStatCard key={s.label} label={s.label} value={s.value} />
        ))}
      </div>
    </div>
  )
}
