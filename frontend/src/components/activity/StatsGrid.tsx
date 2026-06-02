import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { Activity } from '@/api/types'
import { formatDuration } from '@/lib/utils'
import { fieldLabel, fieldUnit, formatSpeedValue, cyclesLabel, defaultSpeedMode } from '@/lib/sportFieldLabels'
import { getSegmentGroup } from '@/lib/sportTaxonomy'
import { getNetActivityTotals } from '@/lib/netActivityTotals'
import { localeForLanguage } from '@/i18n/languages'
import { useUser } from '@/providers/UserProvider'
import { formatDistance, formatElevation, formatShortLength, formatTemperature, type UnitFormatOptions } from '@/lib/units'

interface StatsGridProps {
  activity: Activity
  variant?: 'default' | 'overview'
}

interface StatItem {
  label: string
  value: string | null
  /** Smaller secondary value shown below the primary (e.g. speed below pace) */
  secondary?: string
  badge?: ReactNode
  title?: string
}

const TSS_METHOD_INFO: Record<string, { icon: string; labelKey: string }> = {
  power_np:  { icon: '⚡', labelKey: 'activityStats.tssMethods.powerNp' },
  power_avg: { icon: '⚡', labelKey: 'activityStats.tssMethods.powerAvg' },
  hr_stream: { icon: '♥',  labelKey: 'activityStats.tssMethods.hrStream' },
  hr_avg:    { icon: '♥',  labelKey: 'activityStats.tssMethods.hrAvg' },
  pace_ngp:  { icon: '🏃', labelKey: 'activityStats.tssMethods.paceNgp' },
  pace_swim: { icon: '🏊', labelKey: 'activityStats.tssMethods.paceSwim' },
  multi:     { icon: '🔀', labelKey: 'activityStats.tssMethods.multi' },
}

const STRENGTH_SPORT_TYPES = ['fitness_equipment', 'training', 'strength_training']

function formatSignedElevation(meters: number | null | undefined, sign: '+' | '−', options: UnitFormatOptions): string | null {
  if (meters == null || isNaN(meters) || meters <= 0) return null
  return `${sign}${formatElevation(meters, options)}`
}

export function StatsGrid({ activity, variant = 'default' }: StatsGridProps) {
  const { t } = useTranslation()
  const { locale, units } = useUser()
  const unitOptions = { locale: localeForLanguage(locale), units }
  const sport = activity.sport_type
  const net = getNetActivityTotals(activity)
  const gain = formatSignedElevation(net.elevationGainM, '+', unitOptions)
  const loss = formatSignedElevation(net.elevationLossM, '−', unitOptions)
  const isDEM = activity.elevation_source === 'dem'

  const stats: (StatItem | null)[] = [
    { label: t('activityStats.distance'), value: formatDistance(net.distanceM, unitOptions) },
    { label: t('activityStats.duration'), value: formatDuration(activity.elapsed_time_s ?? 0) },
    { label: t('activityStats.movingTime'), value: activity.moving_time_s ? formatDuration(activity.moving_time_s) : null },
    {
      label: t('activityStats.ascent'),
      value: gain,
      badge: isDEM ? (
        <span
          className="text-xs text-emerald-600 dark:text-emerald-400 ml-1 font-normal"
          title={t('activityStats.demCorrectedElevation')}
        >
          DEM
        </span>
      ) : undefined,
    },
    { label: t('activityStats.descent'), value: loss },

    // Pace-native sports show pace primary plus speed secondary.
    activity.avg_speed_ms != null ? (
      defaultSpeedMode(sport) === 'pace'
        ? { label: t('activityStats.avgPace'), value: formatSpeedValue(activity.avg_speed_ms, sport, 'pace', unitOptions), secondary: formatSpeedValue(activity.avg_speed_ms, sport, 'speed', unitOptions) }
        : { label: t('activityStats.avgSpeed'), value: formatSpeedValue(activity.avg_speed_ms, sport, 'speed', unitOptions) }
    ) : null,

    // Best pace / max speed uses the same primary/secondary pattern.
    activity.max_speed_ms != null ? (
      defaultSpeedMode(sport) === 'pace'
        ? { label: t('activityStats.bestPace'), value: formatSpeedValue(activity.max_speed_ms, sport, 'pace', unitOptions), secondary: formatSpeedValue(activity.max_speed_ms, sport, 'speed', unitOptions) }
        : { label: t('activityStats.maxSpeed'), value: formatSpeedValue(activity.max_speed_ms, sport, 'speed', unitOptions) }
    ) : null,

    activity.avg_heart_rate != null ? {
      label: t('activityStats.avgHeartRate'),
      value: `${Math.round(activity.avg_heart_rate)} bpm`,
    } : null,
    activity.max_heart_rate != null ? {
      label: t('activityStats.maxHeartRate'),
      value: `${activity.max_heart_rate} bpm`,
    } : null,
    activity.min_heart_rate != null ? {
      label: t('activityStats.minHeartRate'),
      value: `${activity.min_heart_rate} bpm`,
    } : null,

    // Sport-specific field labels are still provided by the domain label layer.
    activity.avg_power_w != null ? {
      label: `Ø ${fieldLabel('power', sport)}`,
      value: `${Math.round(activity.avg_power_w)} W`,
    } : null,
    activity.max_power_w != null ? {
      label: `Max. ${fieldLabel('power', sport)}`,
      value: `${Math.round(activity.max_power_w)} W`,
    } : null,
    activity.normalized_power_w != null ? {
      label: 'NP',
      value: `${Math.round(activity.normalized_power_w)} W`,
      title: t('activityStats.normalizedPower'),
    } : null,

    activity.avg_cadence != null ? {
      label: `Ø ${fieldLabel('cadence', sport)}`,
      value: `${Math.round(activity.avg_cadence)} ${fieldUnit('cadence', sport)}`,
      title: fieldLabel('cadence', sport),
    } : null,

    activity.avg_temperature_c != null ? {
      label: t('activityStats.avgTemperature'),
      value: formatTemperature(activity.avg_temperature_c, unitOptions),
    } : null,
    (activity.min_temperature_c != null && activity.max_temperature_c != null && activity.min_temperature_c !== activity.max_temperature_c) ? {
      label: t('activityStats.temperatureMinMax'),
      value: `${formatTemperature(activity.min_temperature_c, unitOptions)} – ${formatTemperature(activity.max_temperature_c, unitOptions)}`,
    } : null,

    // TSS (prefer computed_tss if available, fall back to tss)
    (() => {
      const tssValue = activity.computed_tss ?? activity.tss
      if (tssValue == null || tssValue <= 0) return null
      const method = activity.tss_method ? TSS_METHOD_INFO[activity.tss_method] : undefined
      const isStrength = STRENGTH_SPORT_TYPES.includes(sport)
      const isHrBased = method?.icon === '♥'
      const methodSuffix = method ? ` ${method.icon}` : ''
      const baseTitle = method ? t(method.labelKey) : 'Training Stress Score'
      const warningNote = isStrength && isHrBased ? ` ${t('activityStats.strengthHrTssWarning')}` : ''
      return {
        label: `TSS${methodSuffix}`,
        value: `${Math.round(tssValue)}`,
        title: baseTitle + warningNote,
      }
    })(),

    // TRIMP
    activity.trimp != null && activity.trimp > 0 ? {
      label: 'TRIMP',
      value: `${Math.round(activity.trimp)}`,
      title: t('activityStats.trimpTitle'),
    } : null,

    activity.total_calories != null ? {
      label: t('activityStats.calories'),
      value: `${Math.round(activity.total_calories)} kcal`,
    } : null,

    activity.total_work_j != null ? {
      label: t('activityStats.work'),
      value: `${(activity.total_work_j / 1000).toFixed(1)} kJ`,
    } : null,

    // Total cycles use sport-specific labels from the domain label layer.
    activity.total_cycles != null && cyclesLabel(sport) != null ? {
      label: cyclesLabel(sport)!,
      value: `${activity.total_cycles.toLocaleString(unitOptions.locale)}`,
    } : null,

    // IF (Intensity Factor)
    activity.intensity_factor != null ? {
      label: 'IF',
      value: activity.intensity_factor.toFixed(2),
      title: t('activityStats.intensityFactor'),
    } : null,

    // Balance L/R
    activity.left_right_balance != null ? {
      label: 'Balance L/R',
      value: `${activity.left_right_balance.toFixed(1)} / ${(100 - activity.left_right_balance).toFixed(1)} %`,
    } : null,

    activity.training_load_peak != null ? {
      label: t('activityStats.trainingLoad'),
      value: `${Math.round(activity.training_load_peak)}`,
    } : null,

    activity.anaerobic_training_effect != null ? {
      label: t('activityStats.anaerobicTrainingEffect'),
      value: activity.anaerobic_training_effect.toFixed(1),
      title: t('activityStats.anaerobicTrainingEffectTitle'),
    } : null,

    // Ø SpO2
    activity.avg_spo2_pct != null ? {
      label: 'Ø SpO2',
      value: `${activity.avg_spo2_pct.toFixed(1)} %`,
      title: t('activityStats.avgSpo2Title'),
    } : null,

    activity.is_electric && activity.avg_lev_motor_power_w != null ? {
      label: t('activityStats.avgMotorPower'),
      value: `${Math.round(activity.avg_lev_motor_power_w)} W`,
    } : null,
    activity.is_electric && activity.max_lev_motor_power_w != null ? {
      label: t('activityStats.maxMotorPower'),
      value: `${Math.round(activity.max_lev_motor_power_w)} W`,
    } : null,
    activity.is_electric && activity.lev_battery_consumption_pct != null ? {
      label: t('activityStats.batteryConsumption'),
      value: `${activity.lev_battery_consumption_pct.toFixed(1)} %`,
    } : null,

    activity.pool_length_m != null ? {
      label: t('activityStats.poolLength'), value: formatElevation(activity.pool_length_m, unitOptions),
    } : null,
    activity.num_lengths != null ? {
      label: t('activityStats.lengths'), value: `${activity.num_lengths}`,
    } : null,
    activity.num_active_lengths != null && activity.num_lengths != null && activity.num_active_lengths !== activity.num_lengths ? {
      label: t('activityStats.activeLengths'), value: `${activity.num_active_lengths}`,
    } : null,
    activity.avg_stroke_count != null ? {
      label: t('activityStats.avgStrokesPerLength'), value: `${activity.avg_stroke_count.toFixed(1)}`,
    } : null,
    activity.avg_stroke_distance_m != null ? {
      label: getSegmentGroup(sport) === 'swim' ? t('activityStats.avgDistancePerStroke') : t('activityStats.avgDistancePerStrike'),
      value: formatShortLength(activity.avg_stroke_distance_m, unitOptions),
    } : null,
  ]

  const visible = (stats.filter(Boolean) as StatItem[]).filter(s => s.value && s.value !== '—')

  if (visible.length === 0) return null

  return (
    <div className={variant === 'overview'
      ? 'grid grid-cols-2 xl:grid-cols-4 gap-3'
      : 'grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4'}
    >
      {visible.map(({ label, value, secondary, badge, title }) => (
        <div key={label} className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
          <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1" title={title}>{label}</div>
          <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
            {value}{badge}
          </div>
          {secondary && (
            <div className="text-xs text-neutral-400 dark:text-neutral-500 tabular-nums mt-0.5">{secondary}</div>
          )}
        </div>
      ))}
    </div>
  )
}
