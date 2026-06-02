import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { fetchDiveInfo } from '@/api/activities'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import type { StreamPoint } from '@/api/types'

interface DiveDecoPanelProps {
  activityId: string
  streams: StreamPoint[]
}

const LB_PER_CUBIC_FOOT_PER_KG_PER_LITER = 62.4279606

function waterTypeLabel(raw: string, t: (key: string) => string): string {
  switch (raw.toLowerCase()) {
    case 'fresh': return t('activityDetail.dive.waterTypes.fresh')
    case 'salt': return t('activityDetail.dive.waterTypes.salt')
    case 'en13319': return 'EN 13319'
    default: return raw
  }
}

export function DiveDecoPanel({ activityId, streams }: DiveDecoPanelProps) {
  const { t } = useTranslation()
  const { options, formatElevation } = useUnitFormatters()
  const { data: diveInfo, isLoading } = useQuery({
    queryKey: ['dive-info', activityId],
    queryFn: () => fetchDiveInfo(activityId),
    staleTime: 5 * 60 * 1000,
  })

  // ── NDL Gauge value — show MINIMUM NDL during the dive (most critical moment) ──

  let minNdlSec: number | null = null
  for (const s of streams) {
    if (s.ndl_time_s != null) {
      if (minNdlSec === null || s.ndl_time_s < minNdlSec) minNdlSec = s.ndl_time_s
    }
  }
  const ndlMin = minNdlSec != null ? Math.round(minNdlSec / 60) : null

  // Deco chart data is now rendered by DecoChartPanel in ActivityCharts.tsx
  const hasAnyDecoStreams = streams.some(s => s.ndl_time_s != null || s.cns_load_pct != null)
  const formatDepth = (meters: number | null | undefined) => {
    if (meters == null) return '–'
    if (options.units === 'imperial') return formatElevation(meters)
    return `${new Intl.NumberFormat(options.locale, {
      minimumFractionDigits: 1,
      maximumFractionDigits: 1,
    }).format(meters)} m`
  }
  const formatWaterDensity = (kgPerLiter: number | null | undefined) => {
    if (kgPerLiter == null) return '–'
    if (options.units === 'imperial') {
      return `${new Intl.NumberFormat(options.locale, {
        minimumFractionDigits: 1,
        maximumFractionDigits: 1,
      }).format(kgPerLiter * LB_PER_CUBIC_FOOT_PER_KG_PER_LITER)} lb/ft³`
    }
    return `${new Intl.NumberFormat(options.locale, {
      minimumFractionDigits: 3,
      maximumFractionDigits: 3,
    }).format(kgPerLiter)} kg/l`
  }

  // ── Render ─────────────────────────────────────────────────────────────────

  if (isLoading) {
    return (
      <div className="text-sm text-neutral-500 dark:text-neutral-400 py-4 text-center">
        {t('activityDetail.dive.loadingInfo')}
      </div>
    )
  }

  return (
    <div className="space-y-4">

      {/* A) Equipment Info */}
      {diveInfo?.settings && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4">
          <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">
            {t('activityDetail.dive.equipment')}
          </h3>
          <dl className="grid grid-cols-2 sm:grid-cols-3 gap-x-6 gap-y-2 text-sm">
            <div>
              <dt className="text-xs text-neutral-500 dark:text-neutral-400">{t('activityDetail.dive.decoModel')}</dt>
              <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                {diveInfo.settings.deco_model || '—'}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-neutral-500 dark:text-neutral-400">{t('activityDetail.dive.gfLabel')}</dt>
              <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                {diveInfo.settings.gf_low?.toString() ?? '–'}/{diveInfo.settings.gf_high?.toString() ?? '–'}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-neutral-500 dark:text-neutral-400">{t('activityDetail.dive.waterType')}</dt>
              <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                {diveInfo.settings.water_type ? waterTypeLabel(diveInfo.settings.water_type, t) : '–'}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-neutral-500 dark:text-neutral-400">{t('activityDetail.dive.waterDensity')}</dt>
              <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                {formatWaterDensity(diveInfo.settings.water_density)}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-neutral-500 dark:text-neutral-400">{t('activityDetail.dive.po2Limits')}</dt>
              <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                {diveInfo.settings.po2_warn?.toLocaleString(options.locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) ?? '–'} / {diveInfo.settings.po2_critical?.toLocaleString(options.locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) ?? '–'} / {diveInfo.settings.po2_deco?.toLocaleString(options.locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) ?? '–'} bar
              </dd>
            </div>
            {((diveInfo.settings.ccr_low_setpoint ?? 0) > 0 || (diveInfo.settings.ccr_high_setpoint ?? 0) > 0) && (
              <div>
                <dt className="text-xs text-neutral-500 dark:text-neutral-400">{t('activityDetail.dive.ccrSetpoints')}</dt>
                <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                  {t('activityDetail.dive.ccrLow')}: {diveInfo.settings.ccr_low_setpoint?.toFixed(2) ?? '–'} bar
                  {' @ '}{formatDepth(diveInfo.settings.ccr_low_setpoint_depth_m)}
                  {' · '}
                  {t('activityDetail.dive.ccrHigh')}: {diveInfo.settings.ccr_high_setpoint?.toFixed(2) ?? '–'} bar
                  {' @ '}{formatDepth(diveInfo.settings.ccr_high_setpoint_depth_m)}
                </dd>
              </div>
            )}
          </dl>
        </div>
      )}

      {/* C) NDL Gauge */}
      {ndlMin !== null && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 flex items-center gap-6">
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">{t('activityDetail.dive.ndlGauge')}</div>
            {ndlMin <= 0 ? (
              <div className="text-2xl font-bold text-red-600 dark:text-red-400">{t('activityDetail.dive.ndlDeco')}</div>
            ) : (
              <div className={`text-3xl font-bold tabular-nums ${
                ndlMin > 10
                  ? 'text-green-600 dark:text-green-400'
                  : ndlMin >= 5
                    ? 'text-amber-600 dark:text-amber-400'
                    : 'text-red-600 dark:text-red-400'
              }`}>
                {ndlMin}
                <span className="text-base font-normal text-neutral-500 dark:text-neutral-400 ml-1">{t('activityDetail.dive.minutesShort')}</span>
              </div>
            )}
          </div>
          {/* Visual color bar */}
          <div className="flex-1 h-3 rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-hidden">
            {ndlMin > 0 && (
              <div
                className={`h-full rounded-full transition-all ${
                  ndlMin > 10 ? 'bg-green-500' : ndlMin >= 5 ? 'bg-amber-500' : 'bg-red-500'
                }`}
                style={{ width: `${Math.min(100, (ndlMin / 30) * 100)}%` }}
              />
            )}
            {ndlMin <= 0 && <div className="h-full bg-red-500 w-full" />}
          </div>
        </div>
      )}

      {!hasAnyDecoStreams && !diveInfo?.settings && (
        <div className="text-sm text-neutral-500 dark:text-neutral-400 py-4 text-center">
          {t('activityDetail.dive.noDecoData')}
        </div>
      )}
    </div>
  )
}
