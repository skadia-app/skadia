import type { FusionActivityInfo } from '@/pages/FusionPage'

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
  // Core
  {
    key: 'gps',
    label: 'GPS-Track',
    category: 'core',
    hasKey: 'has_gps',
    allowFuse: true,
    getDisplay: a => `${Math.round((a.distance_m / 1000) * 10) / 10} km`,
  },
  {
    key: 'hr',
    label: 'Herzfrequenz',
    category: 'core',
    hasKey: 'has_hr',
    allowFuse: false,
  },
  {
    key: 'power',
    label: 'Leistung',
    category: 'core',
    hasKey: 'has_power',
    allowFuse: false,
  },
  {
    key: 'altitude',
    label: 'Höhenprofil',
    category: 'core',
    hasKey: 'has_altitude',
    allowFuse: false,
  },
  {
    key: 'cadence',
    label: 'Kadenz',
    category: 'core',
    hasKey: 'has_cadence',
    allowFuse: false,
  },
  {
    key: 'temperature',
    label: 'Temperatur',
    category: 'core',
    hasKey: 'has_temperature',
    allowFuse: false,
  },
  {
    key: 'speed',
    label: 'Geschwindigkeit',
    category: 'core',
    hasKey: 'has_gps',
    allowFuse: false,
  },
  // Running Dynamics
  {
    key: 'gct',
    label: 'Ground Contact Time',
    category: 'running',
    hasKey: 'has_hr', // proxied — GCT available when running dynamics present (HR sensor)
    allowFuse: false,
  },
  {
    key: 'vertical_oscillation',
    label: 'Vertical Oscillation',
    category: 'running',
    hasKey: 'has_hr',
    allowFuse: false,
  },
  {
    key: 'stance_time',
    label: 'Stance Time',
    category: 'running',
    hasKey: 'has_hr',
    allowFuse: false,
  },
  {
    key: 'step_length',
    label: 'Step Length',
    category: 'running',
    hasKey: 'has_hr',
    allowFuse: false,
  },
  {
    key: 'left_right_balance',
    label: 'L/R Balance',
    category: 'running',
    hasKey: 'has_hr',
    allowFuse: false,
  },
  // Pedal Analytics
  {
    key: 'torque_effectiveness_l',
    label: 'Torque Eff. (L)',
    category: 'pedal',
    hasKey: 'has_power',
    allowFuse: false,
  },
  {
    key: 'torque_effectiveness_r',
    label: 'Torque Eff. (R)',
    category: 'pedal',
    hasKey: 'has_power',
    allowFuse: false,
  },
  {
    key: 'pedal_smoothness_l',
    label: 'Pedal Smooth. (L)',
    category: 'pedal',
    hasKey: 'has_power',
    allowFuse: false,
  },
  {
    key: 'pedal_smoothness_r',
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
  tagsMerge: boolean
  onTagsMergeChange: (v: boolean) => void
  titleSource: string | null
  onTitleSourceChange: (id: string) => void
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function FusionFieldTable({
  activities,
  fieldMap,
  onFieldChange,
  tagsMerge,
  onTagsMergeChange,
  titleSource,
  onTitleSourceChange,
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

          {/* Divider before Titel/Tags */}
          <tr className="border-b border-neutral-200 dark:border-neutral-700">
            <td colSpan={activities.length + 2} className="pt-3" />
          </tr>

          {/* Titel row */}
          <tr className="border-b border-neutral-100 dark:border-neutral-800 hover:bg-neutral-50 dark:hover:bg-neutral-800/30 transition-colors">
            <td className="py-2 pr-4 text-neutral-700 dark:text-neutral-300 font-medium text-xs">
              Titel
            </td>
            {activities.map(a => (
              <td key={a.id} className="text-center py-2 px-2">
                <label className="inline-flex flex-col items-center gap-1 cursor-pointer">
                  <input
                    type="radio"
                    name="title-source"
                    value={a.id}
                    checked={titleSource === a.id}
                    onChange={() => onTitleSourceChange(a.id)}
                    className="accent-accent-DEFAULT"
                  />
                  <span className="text-xs text-neutral-400">{a.label}</span>
                </label>
              </td>
            ))}
            <td />
          </tr>

          {/* Tags row */}
          <tr>
            <td className="py-2 pr-4 text-neutral-700 dark:text-neutral-300 font-medium text-xs">
              Tags
            </td>
            <td colSpan={activities.length + 1} className="py-2 px-2">
              <label className="inline-flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={tagsMerge}
                  onChange={e => onTagsMergeChange(e.target.checked)}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-xs text-neutral-600 dark:text-neutral-400">
                  Tags aus allen Quellen zusammenführen
                </span>
              </label>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  )
}
