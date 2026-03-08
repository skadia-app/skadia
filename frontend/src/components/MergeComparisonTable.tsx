import type { Activity } from '@/api/types'

interface MergeField {
  key: string
  label: string
  targetField: keyof Activity
  unit: string
}

const MERGE_FIELDS: MergeField[] = [
  { key: 'hr', label: 'Herzfrequenz', targetField: 'avg_heart_rate', unit: 'bpm' },
  { key: 'power', label: 'Leistung', targetField: 'avg_power_w', unit: 'W' },
  { key: 'cadence', label: 'Kadenz', targetField: 'avg_cadence', unit: 'rpm' },
  { key: 'temperature', label: 'Temperatur', targetField: 'avg_temperature_c', unit: '°C' },
  { key: 'gps', label: 'GPS-Koordinaten', targetField: 'distance_m', unit: 'm' },
  { key: 'altitude', label: 'Höhe', targetField: 'elevation_gain_m', unit: 'm' },
  { key: 'speed', label: 'Geschwindigkeit', targetField: 'avg_speed_ms', unit: 'm/s' },
  { key: 'title', label: 'Titel', targetField: 'title', unit: '' },
  { key: 'tags', label: 'Tags', targetField: 'tags', unit: '' },
  { key: 'gear', label: 'Ausrüstung', targetField: 'gear_id', unit: '' },
]

interface Props {
  target: Activity
  source: Activity
  selectedFields: string[]
  onToggleField: (field: string) => void
}

export function MergeComparisonTable({ target, source, selectedFields, onToggleField }: Props) {
  const formatVal = (val: unknown, unit: string) => {
    if (val === null || val === undefined || val === '') return '–'
    if (Array.isArray(val)) return val.length > 0 ? val.join(', ') : '–'
    return `${val}${unit ? ' ' + unit : ''}`
  }

  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b border-neutral-200 dark:border-neutral-700">
          <th className="text-left py-2 font-medium text-neutral-600 dark:text-neutral-400">Feld</th>
          <th className="text-left py-2 font-medium text-neutral-600 dark:text-neutral-400">Ziel</th>
          <th className="text-left py-2 font-medium text-neutral-600 dark:text-neutral-400">Quelle</th>
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
              <td className="py-1.5 text-neutral-700 dark:text-neutral-300">{field.label}</td>
              <td className="py-1.5 text-neutral-500 dark:text-neutral-400">{formatVal(targetVal, field.unit)}</td>
              <td className={`py-1.5 ${hasBenefit ? 'text-emerald-600 dark:text-emerald-400 font-medium' : 'text-neutral-500 dark:text-neutral-400'}`}>
                {formatVal(sourceVal, field.unit)}
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
