import type { ReactNode } from 'react'
import type { Activity } from '@/api/types'
import { formatDistance, formatDuration, formatElevationGain, formatElevationLoss } from '@/lib/utils'
import { fieldLabel, fieldUnit, formatSpeedValue, cyclesLabel, defaultSpeedMode } from '@/lib/sportFieldLabels'
import { getSegmentGroup } from '@/lib/sportTaxonomy'

interface StatsGridProps {
  activity: Activity
}

interface StatItem {
  label: string
  value: string | null
  badge?: ReactNode
  title?: string
}

const TSS_METHOD_INFO: Record<string, { icon: string; label: string }> = {
  power_np:  { icon: '⚡', label: 'Power-basiert (Normalized Power)' },
  power_avg: { icon: '⚡', label: 'Power-basiert (Durchschnitt)' },
  hr_stream: { icon: '♥',  label: 'HR-basiert (sekunden-weise)' },
  hr_avg:    { icon: '♥',  label: 'HR-basiert (Durchschnitt)' },
  pace_ngp:  { icon: '🏃', label: 'Pace-basiert (NGP)' },
  pace_swim: { icon: '🏊', label: 'Pace-basiert (CSS)' },
  multi:     { icon: '🔀', label: 'Multi-Sport' },
}

const STRENGTH_SPORT_TYPES = ['fitness_equipment', 'training', 'strength_training']

export function StatsGrid({ activity }: StatsGridProps) {
  const sport = activity.sport_type
  const gain = formatElevationGain(activity.elevation_gain_m)
  const loss = formatElevationLoss(activity.elevation_loss_m)
  const isDEM = activity.elevation_source === 'dem'

  const stats: (StatItem | null)[] = [
    { label: 'Distanz', value: formatDistance(activity.distance_m) },
    { label: 'Dauer', value: formatDuration(activity.elapsed_time_s ?? 0) },
    { label: 'Bewegungszeit', value: activity.moving_time_s ? formatDuration(activity.moving_time_s) : null },
    {
      label: 'Anstieg',
      value: gain,
      badge: isDEM ? (
        <span
          className="text-xs text-emerald-600 dark:text-emerald-400 ml-1 font-normal"
          title="Höhe via DEM (Open-Meteo) korrigiert"
        >
          DEM
        </span>
      ) : undefined,
    },
    { label: 'Abstieg', value: loss },

    // Ø Pace + Ø Geschwindigkeit (pace-native sports get both; speed-native get only speed)
    ...(activity.avg_speed_ms != null
      ? defaultSpeedMode(sport) === 'pace'
        ? [
            { label: 'Ø Pace', value: formatSpeedValue(activity.avg_speed_ms, sport, 'pace') },
            { label: `Ø ${fieldLabel('speed', sport)}`, value: formatSpeedValue(activity.avg_speed_ms, sport, 'speed') },
          ]
        : [
            { label: `Ø ${fieldLabel('speed', sport)}`, value: formatSpeedValue(activity.avg_speed_ms, sport, 'speed') },
          ]
      : []),

    // Beste Pace + Max. Geschwindigkeit (same dual/single logic)
    ...(activity.max_speed_ms != null
      ? defaultSpeedMode(sport) === 'pace'
        ? [
            { label: 'Beste Pace', value: formatSpeedValue(activity.max_speed_ms, sport, 'pace') },
            { label: 'Max. Geschwindigkeit', value: formatSpeedValue(activity.max_speed_ms, sport, 'speed') },
          ]
        : [
            { label: 'Max. Geschwindigkeit', value: formatSpeedValue(activity.max_speed_ms, sport, 'speed') },
          ]
      : []),

    // Herzfrequenz
    activity.avg_heart_rate != null ? {
      label: 'Ø Herzfrequenz',
      value: `${Math.round(activity.avg_heart_rate)} bpm`,
    } : null,
    activity.max_heart_rate != null ? {
      label: 'Max. Herzfrequenz',
      value: `${activity.max_heart_rate} bpm`,
    } : null,
    activity.min_heart_rate != null ? {
      label: 'Min. Herzfrequenz',
      value: `${activity.min_heart_rate} bpm`,
    } : null,

    // Leistung
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
      title: 'Normalisierte Leistung',
    } : null,

    // Kadenz
    activity.avg_cadence != null ? {
      label: `Ø ${fieldLabel('cadence', sport)}`,
      value: `${Math.round(activity.avg_cadence)} ${fieldUnit('cadence', sport)}`,
      title: fieldLabel('cadence', sport),
    } : null,

    // Temperatur
    activity.avg_temperature_c != null ? {
      label: 'Ø Temperatur',
      value: `${Math.round(activity.avg_temperature_c)} °C`,
    } : null,
    (activity.min_temperature_c != null && activity.max_temperature_c != null && activity.min_temperature_c !== activity.max_temperature_c) ? {
      label: 'Temperatur (Min–Max)',
      value: `${Math.round(activity.min_temperature_c)} – ${Math.round(activity.max_temperature_c)} °C`,
    } : null,

    // TSS (prefer computed_tss if available, fall back to tss)
    (() => {
      const tssValue = activity.computed_tss ?? activity.tss
      if (tssValue == null || tssValue <= 0) return null
      const method = activity.tss_method ? TSS_METHOD_INFO[activity.tss_method] : undefined
      const isStrength = STRENGTH_SPORT_TYPES.includes(sport)
      const isHrBased = method?.icon === '♥'
      const methodSuffix = method ? ` ${method.icon}` : ''
      const baseTitle = method?.label ?? 'Training Stress Score'
      const warningNote = isStrength && isHrBased ? ' — bei Krafttraining möglicherweise ungenau' : ''
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
      title: 'Edwards TRIMP — Zonen-gewichtete Belastungsminuten (Standard: 1,2,3,4,5)',
    } : null,

    // Kalorien
    activity.total_calories != null ? {
      label: 'Kalorien',
      value: `${Math.round(activity.total_calories)} kcal`,
    } : null,

    // Arbeit (nur bei Power)
    activity.total_work_j != null ? {
      label: 'Arbeit',
      value: `${(activity.total_work_j / 1000).toFixed(1)} kJ`,
    } : null,

    // Gesamt-Zyklen (sportspezifisch)
    activity.total_cycles != null && cyclesLabel(sport) != null ? {
      label: cyclesLabel(sport)!,
      value: `${activity.total_cycles.toLocaleString('de-DE')}`,
    } : null,

    // IF (Intensity Factor)
    activity.intensity_factor != null ? {
      label: 'IF',
      value: activity.intensity_factor.toFixed(2),
      title: 'Intensity Factor (Intensitätsfaktor)',
    } : null,

    // Balance L/R
    activity.left_right_balance != null ? {
      label: 'Balance L/R',
      value: `${activity.left_right_balance.toFixed(1)} / ${(100 - activity.left_right_balance).toFixed(1)} %`,
    } : null,

    // Belastungswert
    activity.training_load_peak != null ? {
      label: 'Belastungswert',
      value: `${Math.round(activity.training_load_peak)}`,
    } : null,

    // Anaerober TE
    activity.anaerobic_training_effect != null ? {
      label: 'Anaerob TE',
      value: activity.anaerobic_training_effect.toFixed(1),
      title: 'Anaerober Trainingseffekt',
    } : null,

    // Ø SpO2
    activity.avg_spo2_pct != null ? {
      label: 'Ø SpO2',
      value: `${activity.avg_spo2_pct.toFixed(1)} %`,
      title: 'Durchschnittliche Sauerstoffsättigung',
    } : null,

    // E-Bike
    activity.is_ebike && activity.avg_lev_motor_power_w != null ? {
      label: 'Ø Motorleistung',
      value: `${Math.round(activity.avg_lev_motor_power_w)} W`,
    } : null,
    activity.is_ebike && activity.max_lev_motor_power_w != null ? {
      label: 'Max. Motorleistung',
      value: `${Math.round(activity.max_lev_motor_power_w)} W`,
    } : null,
    activity.is_ebike && activity.lev_battery_consumption_pct != null ? {
      label: 'Akkuverbrauch',
      value: `${activity.lev_battery_consumption_pct.toFixed(1)} %`,
    } : null,

    // Pool-Info (nur wenn pool_length_m vorhanden)
    activity.pool_length_m != null ? {
      label: 'Bahnlänge', value: `${activity.pool_length_m} m`,
    } : null,
    activity.num_lengths != null ? {
      label: 'Bahnen', value: `${activity.num_lengths}`,
    } : null,
    activity.num_active_lengths != null && activity.num_lengths != null && activity.num_active_lengths !== activity.num_lengths ? {
      label: 'Aktive Bahnen', value: `${activity.num_active_lengths}`,
    } : null,
    activity.avg_stroke_count != null ? {
      label: 'Ø Züge/Bahn', value: `${activity.avg_stroke_count.toFixed(1)}`,
    } : null,
    activity.avg_stroke_distance_m != null ? {
      label: getSegmentGroup(sport) === 'swim' ? 'Ø Distanz/Zug' : 'Ø Distanz/Schlag',
      value: `${activity.avg_stroke_distance_m.toFixed(2)} m`,
    } : null,
  ]

  const visible = (stats.filter(Boolean) as StatItem[]).filter(s => s.value && s.value !== '—')

  if (visible.length === 0) return null

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
      {visible.map(({ label, value, badge, title }) => (
        <div key={label} className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
          <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1" title={title}>{label}</div>
          <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
            {value}{badge}
          </div>
        </div>
      ))}
    </div>
  )
}
