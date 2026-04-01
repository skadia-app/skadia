import type { FusionActivityInfo } from '@/pages/FusionPage'

// ── Field key constants ────────────────────────────────────────────────────────

export const FUSION_FIELDS = {
  GPS: 'gps',
  HR: 'hr',
  POWER: 'power',
  ALTITUDE: 'altitude',
  CADENCE: 'cadence',
  TEMPERATURE: 'temperature',
  SPEED: 'speed',
  GCT: 'gct',
  VERTICAL_OSCILLATION: 'vertical_oscillation',
  STANCE_TIME: 'stance_time',
  STEP_LENGTH: 'step_length',
  LEFT_RIGHT_BALANCE: 'left_right_balance',
  TORQUE_EFFECTIVENESS_L: 'torque_effectiveness_l',
  TORQUE_EFFECTIVENESS_R: 'torque_effectiveness_r',
  PEDAL_SMOOTHNESS_L: 'pedal_smoothness_l',
  PEDAL_SMOOTHNESS_R: 'pedal_smoothness_r',
} as const

// ── Field definitions ─────────────────────────────────────────────────────────

type FieldCategory = 'core' | 'running' | 'pedal'

interface FieldDef {
  key: string
  label: string
  category: FieldCategory
  hasKey: keyof FusionActivityInfo | null
  allowFuse: boolean
  getDisplay?: (a: FusionActivityInfo) => string
}

const FIELDS: FieldDef[] = [
  {
    key: FUSION_FIELDS.GPS,
    label: 'GPS-Track',
    category: 'core',
    hasKey: 'has_gps',
    allowFuse: true,
    getDisplay: a => `${Math.round((a.distance_m / 1000) * 10) / 10} km`,
  },
  {
    key: FUSION_FIELDS.HR,
    label: 'Herzfrequenz',
    category: 'core',
    hasKey: 'has_hr',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.POWER,
    label: 'Leistung',
    category: 'core',
    hasKey: 'has_power',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.ALTITUDE,
    label: 'Höhenprofil',
    category: 'core',
    hasKey: 'has_altitude',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.CADENCE,
    label: 'Kadenz',
    category: 'core',
    hasKey: 'has_cadence',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.TEMPERATURE,
    label: 'Temperatur',
    category: 'core',
    hasKey: 'has_temperature',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.SPEED,
    label: 'Geschwindigkeit',
    category: 'core',
    hasKey: 'has_gps',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.GCT,
    label: 'Ground Contact Time',
    category: 'running',
    hasKey: 'has_running_dynamics',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.VERTICAL_OSCILLATION,
    label: 'Vertical Oscillation',
    category: 'running',
    hasKey: 'has_running_dynamics',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.STANCE_TIME,
    label: 'Stance Time',
    category: 'running',
    hasKey: 'has_running_dynamics',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.STEP_LENGTH,
    label: 'Step Length',
    category: 'running',
    hasKey: 'has_running_dynamics',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.LEFT_RIGHT_BALANCE,
    label: 'L/R Balance',
    category: 'running',
    hasKey: 'has_running_dynamics',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.TORQUE_EFFECTIVENESS_L,
    label: 'Torque Eff. (L)',
    category: 'pedal',
    hasKey: 'has_power',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.TORQUE_EFFECTIVENESS_R,
    label: 'Torque Eff. (R)',
    category: 'pedal',
    hasKey: 'has_power',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.PEDAL_SMOOTHNESS_L,
    label: 'Pedal Smooth. (L)',
    category: 'pedal',
    hasKey: 'has_power',
    allowFuse: false,
  },
  {
    key: FUSION_FIELDS.PEDAL_SMOOTHNESS_R,
    label: 'Pedal Smooth. (R)',
    category: 'pedal',
    hasKey: 'has_power',
    allowFuse: false,
  },
]

// ── Category divider rows ─────────────────────────────────────────────────────

const CATEGORY_LABELS: Record<FieldCategory, string | null> = {
  core: null, // no divider for core
  running: 'Running Dynamics',
  pedal: 'Pedal Analytics',
}

// ── Color pill helper ─────────────────────────────────────────────────────────

function colorPillStyle(hex: string): { background: string; color: string } {
  // Generate a light tint from the hex color
  // We approximate by returning inline style using the original hex with opacity
  return { background: `${hex}22`, color: hex }
}

// ── Props ─────────────────────────────────────────────────────────────────────

interface FusionFieldTableProps {
  activities: FusionActivityInfo[]
  fieldMap: Record<string, string>
  onFieldChange: (field: string, sourceId: string) => void
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function FusionFieldTable({
  activities,
  fieldMap,
  onFieldChange,
}: FusionFieldTableProps) {
  const manyColumns = activities.length > 4

  // Track which category dividers have been rendered
  const renderedDividers = new Set<FieldCategory>()

  const rows: React.ReactNode[] = []

  for (const field of FIELDS) {
    // Insert category divider on first field of each non-core category
    if (field.category !== 'core' && !renderedDividers.has(field.category)) {
      renderedDividers.add(field.category)
      const catLabel = CATEGORY_LABELS[field.category]
      rows.push(
        <tr key={`divider-${field.category}`} className="border-b border-neutral-100 dark:border-neutral-800 bg-blue-50/30 dark:bg-blue-900/10">
          <td className="py-2.5 font-medium text-blue-700 dark:text-blue-400 text-xs">
            {catLabel}
          </td>
          <td colSpan={activities.length + 1} className="py-2.5 text-xs text-blue-600 dark:text-blue-500 text-center">
            ↓ Erweiterte Felder
          </td>
        </tr>
      )
    }

    const selected = fieldMap[field.key]
    const isCategoryField = field.category !== 'core'

    rows.push(
      <tr
        key={field.key}
        className="border-b border-neutral-100 dark:border-neutral-800 hover:bg-neutral-50 dark:hover:bg-neutral-800/30 transition-colors"
      >
        <td className={`py-2 pr-4 font-medium text-xs ${isCategoryField ? 'pl-4 text-neutral-500 dark:text-neutral-400' : 'text-neutral-700 dark:text-neutral-300'}`}>
          {field.label}
        </td>
        {activities.map(a => {
          const hasData = field.hasKey != null
            ? (a[field.hasKey] as boolean | undefined) ?? false
            : true
          const isSelected = selected === a.id
          return (
            <td key={a.id} className="text-center py-2 px-2">
              <label
                className={`inline-flex flex-col items-center gap-1 ${
                  hasData ? 'cursor-pointer' : 'opacity-30 cursor-not-allowed'
                }`}
              >
                <input
                  type="radio"
                  name={`field-${field.key}`}
                  value={a.id}
                  checked={isSelected}
                  disabled={!hasData}
                  onChange={() => hasData && onFieldChange(field.key, a.id)}
                  className="accent-accent-DEFAULT"
                />
                {field.getDisplay && (
                  <span className="text-xs text-neutral-400">
                    {hasData ? field.getDisplay(a) : '—'}
                  </span>
                )}
              </label>
            </td>
          )
        })}
        {/* Fuse column */}
        <td className="text-center py-2 px-2">
          {field.allowFuse ? (
            <label className="inline-flex flex-col items-center gap-1 cursor-pointer">
              <input
                type="radio"
                name={`field-${field.key}`}
                value="fuse"
                checked={selected === 'fuse'}
                onChange={() => onFieldChange(field.key, 'fuse')}
                className="accent-accent-DEFAULT"
              />
              <span className="text-xs text-accent-DEFAULT font-medium">Fuse</span>
            </label>
          ) : (
            <span className="text-neutral-200 dark:text-neutral-700 text-xs">—</span>
          )}
        </td>
      </tr>
    )
  }

  return (
    <div className={manyColumns ? 'overflow-x-auto' : ''}>
      <table className={`w-full text-sm ${manyColumns ? 'min-w-[700px]' : ''}`}>
        <thead>
          <tr className="border-b border-neutral-200 dark:border-neutral-700">
            <th className="text-left py-2 pr-4 font-medium text-neutral-600 dark:text-neutral-400 w-36 text-xs">
              Feld
            </th>
            {activities.map(a => (
              <th key={a.id} className="text-center py-2 px-2 font-medium">
                <div className="flex flex-col items-center gap-1">
                  <span
                    className="inline-block px-2 py-0.5 rounded-full text-xs font-semibold"
                    style={colorPillStyle(a.color)}
                  >
                    {a.label}
                  </span>
                  {a.deviceLabel && (
                    <span className="text-[10px] font-normal text-neutral-400 dark:text-neutral-500 whitespace-nowrap">
                      {a.deviceLabel}
                    </span>
                  )}
                </div>
              </th>
            ))}
            <th className="text-center py-2 px-2 font-medium text-neutral-600 dark:text-neutral-400 text-xs w-16">
              Fuse
            </th>
          </tr>
        </thead>
        <tbody>
          {rows}
        </tbody>
      </table>
    </div>
  )
}
