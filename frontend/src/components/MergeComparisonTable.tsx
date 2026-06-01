import { useTranslation } from 'react-i18next'
import type { Activity } from '@/api/types'
import { fieldUnit } from '@/lib/sportFieldLabels'
import { formatHeartRate, formatPower } from '@/lib/units'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

type MergeValueKind =
  | 'heartRate'
  | 'power'
  | 'cadence'
  | 'temperature'
  | 'distance'
  | 'elevation'
  | 'speed'
  | 'text'

interface MergeField {
  key: string
  labelKey: string
  targetField: keyof Activity
  valueKind: MergeValueKind
}

const MERGE_FIELDS: MergeField[] = [
  { key: 'hr', labelKey: 'heartRate', targetField: 'avg_heart_rate', valueKind: 'heartRate' },
  { key: 'power', labelKey: 'power', targetField: 'avg_power_w', valueKind: 'power' },
  { key: 'cadence', labelKey: 'cadence', targetField: 'avg_cadence', valueKind: 'cadence' },
  { key: 'temperature', labelKey: 'temperature', targetField: 'avg_temperature_c', valueKind: 'temperature' },
  { key: 'gps', labelKey: 'gpsCoordinates', targetField: 'distance_m', valueKind: 'distance' },
  { key: 'altitude', labelKey: 'altitude', targetField: 'elevation_gain_m', valueKind: 'elevation' },
  { key: 'speed', labelKey: 'speed', targetField: 'avg_speed_ms', valueKind: 'speed' },
  { key: 'title', labelKey: 'title', targetField: 'title', valueKind: 'text' },
  { key: 'tags', labelKey: 'tags', targetField: 'tags', valueKind: 'text' },
  { key: 'gear', labelKey: 'gear', targetField: 'gear_id', valueKind: 'text' },
]

interface Props {
  target: Activity
  source: Activity
  selectedFields: string[]
  onToggleField: (field: string) => void
}

export function MergeComparisonTable({ target, source, selectedFields, onToggleField }: Props) {
  const { t } = useTranslation()
  const unitFormatters = useUnitFormatters()

  const formatVal = (activity: Activity, val: unknown, valueKind: MergeValueKind) => {
    if (val === null || val === undefined || val === '') return t('workshopOperations.mergeComparison.emptyValue')
    if (Array.isArray(val)) return val.length > 0 ? val.join(', ') : t('workshopOperations.mergeComparison.emptyValue')
    if (typeof val !== 'number' || Number.isNaN(val)) return String(val)

    switch (valueKind) {
      case 'heartRate':
        return formatHeartRate(val, unitFormatters.options)
      case 'power':
        return formatPower(val, unitFormatters.options)
      case 'cadence': {
        const cadence = new Intl.NumberFormat(unitFormatters.options.locale, { maximumFractionDigits: 0 })
          .format(Math.round(val))
        return `${cadence} ${fieldUnit('cadence', activity.sport_type, unitFormatters.options)}`
      }
      case 'temperature':
        return unitFormatters.formatTemperature(val)
      case 'distance':
        return unitFormatters.formatDistanceCompact(val)
      case 'elevation':
        return unitFormatters.formatElevation(val)
      case 'speed':
        return unitFormatters.formatSpeed(val)
      case 'text':
        return String(val)
    }
  }

  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b border-neutral-200 dark:border-neutral-700">
          <th className="text-left py-2 font-medium text-neutral-600 dark:text-neutral-400">{t('workshopOperations.mergeComparison.field')}</th>
          <th className="text-left py-2 font-medium text-neutral-600 dark:text-neutral-400">{t('workshopOperations.mergeComparison.target')}</th>
          <th className="text-left py-2 font-medium text-neutral-600 dark:text-neutral-400">{t('workshopOperations.mergeComparison.source')}</th>
          <th className="w-8"></th>
        </tr>
      </thead>
      <tbody>
        {MERGE_FIELDS.map(field => {
          const targetVal = target[field.targetField]
          const sourceVal = source[field.targetField]
          const hasSourceData = sourceVal !== null && sourceVal !== undefined && sourceVal !== '' && !(Array.isArray(sourceVal) && sourceVal.length === 0)
          const hasTargetData = targetVal !== null && targetVal !== undefined && targetVal !== '' && !(Array.isArray(targetVal) && targetVal.length === 0)
          const hasBenefit = !hasTargetData && hasSourceData

          return (
            <tr key={field.key} className="border-b border-neutral-100 dark:border-neutral-800">
              <td className="py-1.5 text-neutral-700 dark:text-neutral-300">{t(`workshopOperations.mergeComparison.fields.${field.labelKey}`)}</td>
              <td className="py-1.5 text-neutral-500 dark:text-neutral-400">{formatVal(target, targetVal, field.valueKind)}</td>
              <td className={`py-1.5 ${hasBenefit ? 'text-emerald-600 dark:text-emerald-400 font-medium' : 'text-neutral-500 dark:text-neutral-400'}`}>
                {formatVal(source, sourceVal, field.valueKind)}
                {hasBenefit && <span className="ml-1">←</span>}
              </td>
              <td className="py-1.5 text-center">
                <input
                  type="checkbox"
                  checked={selectedFields.includes(field.key)}
                  onChange={() => onToggleField(field.key)}
                  disabled={!hasSourceData}
                  className="cursor-pointer disabled:cursor-not-allowed"
                />
              </td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}
