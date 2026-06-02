// frontend/src/components/activity/MapColorLegend.tsx
import { useTranslation } from 'react-i18next'
import type { ScaleDefinition } from '@/lib/mapColorUtils'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

interface Props {
  scale: ScaleDefinition | null
}

export default function MapColorLegend({ scale }: Props) {
  const { t } = useTranslation()
  const unitFormatters = useUnitFormatters()
  // Hide but keep slot to prevent layout shifts
  const hidden = !scale || scale.type === 'constant'

  return (
    <div
      className={`absolute top-3 right-14 z-10 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md p-2 text-xs text-neutral-700 dark:text-neutral-300 max-w-[220px] ${
        hidden ? 'invisible' : ''
      }`}
      aria-hidden={hidden}
    >
      {scale && scale.type === 'linear' && <LinearLegend scale={scale} metricLabel={metricName(scale.metric, t)} formatValue={v => formatScaleValue(scale, v, unitFormatters)} />}
      {scale && scale.type === 'zones' && <ZonesLegend scale={scale} metricLabel={metricName(scale.metric, t)} formatValue={v => formatScaleValue(scale, v, unitFormatters)} />}
    </div>
  )
}

function LinearLegend({
  scale,
  metricLabel,
  formatValue,
}: {
  scale: Extract<ScaleDefinition, { type: 'linear' }>
  metricLabel: string
  formatValue: (value: number) => string
}) {
  const gradient = scale.stops
    .map(s => s.color)
    .join(', ')
  const min = scale.stops[0].value
  const max = scale.stops[scale.stops.length - 1].value

  return (
    <div className="min-w-[140px]">
      <div className="font-medium mb-1">{metricLabel}</div>
      <div
        className="h-2 rounded-full mb-1"
        style={{ background: `linear-gradient(to right, ${gradient})` }}
      />
      <div className="flex justify-between items-baseline gap-3 text-[10px] tabular-nums">
        <span>{formatValue(min)}</span>
        <span>{formatValue(max)}</span>
      </div>
    </div>
  )
}

function ZonesLegend({
  scale,
  metricLabel,
  formatValue,
}: {
  scale: Extract<ScaleDefinition, { type: 'zones' }>
  metricLabel: string
  formatValue: (value: number) => string
}) {
  return (
    <div>
      <div className="font-medium mb-1">{metricLabel}</div>
      <ul className="space-y-0.5 max-h-40 overflow-y-auto">
        {scale.zoneColors.map((color, i) => (
          <li key={i} className="flex items-center gap-1.5 text-[10px]">
            <span className="inline-block w-3 h-3 rounded-sm" style={{ background: color }} />
            <span className="tabular-nums">
              Z{i + 1}:{' '}
              {i === scale.zoneColors.length - 1 || scale.zoneHighs[i] >= 9999
                ? <>&gt; {formatValue(i > 0 ? scale.zoneHighs[i - 1] : 0)}</>
                : <>≤ {formatValue(scale.zoneHighs[i])}</>}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

function metricName(m: ScaleDefinition['metric'], t: (key: string) => string): string {
  const mapping: Record<typeof m, string> = {
    sport: t('activityDetail.mapColorLegend.metrics.sport'),
    hr: t('activityDetail.mapColorLegend.metrics.hr'),
    speed: t('activityDetail.mapColorLegend.metrics.speed'),
    pace: t('activityDetail.mapColorLegend.metrics.pace'),
    altitude: t('activityDetail.mapColorLegend.metrics.altitude'),
    cadence: t('activityDetail.mapColorLegend.metrics.cadence'),
    power: t('activityDetail.mapColorLegend.metrics.power'),
    temperature: t('activityDetail.mapColorLegend.metrics.temperature'),
  }
  return mapping[m]
}

function formatScaleValue(
  scale: Extract<ScaleDefinition, { type: 'linear' | 'zones' }>,
  value: number,
  unitFormatters: ReturnType<typeof useUnitFormatters>,
): string {
  switch (scale.metric) {
    case 'speed':
      return unitFormatters.formatSpeed(value / 3.6)
    case 'pace':
      return unitFormatters.formatPace(value * 60)
    case 'altitude':
      return unitFormatters.formatElevation(value)
    case 'temperature':
      return unitFormatters.formatTemperature(value)
    default:
      return `${Math.round(value)} ${scale.unit}`
  }
}
