import type { FusionCandidate } from '@/api/fusion'

interface ActivityInfo {
  id: string
  label: string
  candidate: FusionCandidate | null
  isTarget: boolean
}

interface FusionFieldTableProps {
  activities: ActivityInfo[]
  fieldMap: Record<string, string>
  onFieldChange: (field: string, sourceId: string) => void
  tagsMerge: boolean
  onTagsMergeChange: (v: boolean) => void
  titleSource: string | null
  onTitleSourceChange: (id: string) => void
}

const FIELDS: Array<{
  key: string
  label: string
  hasKey: keyof FusionCandidate | null
  allowFuse: boolean
  getDisplay: (c: FusionCandidate) => string
}> = [
  {
    key: 'gps',
    label: 'GPS-Track',
    hasKey: 'has_gps',
    allowFuse: true,
    getDisplay: c => `${Math.round(c.distance_m / 100) / 10} km`,
  },
  {
    key: 'hr',
    label: 'Herzfrequenz',
    hasKey: 'has_hr',
    allowFuse: false,
    getDisplay: () => 'HR',
  },
  {
    key: 'power',
    label: 'Leistung',
    hasKey: 'has_power',
    allowFuse: false,
    getDisplay: () => 'Power',
  },
  {
    key: 'altitude',
    label: 'Höhenprofil',
    hasKey: 'has_altitude',
    allowFuse: false,
    getDisplay: () => 'Altitude',
  },
  {
    key: 'cadence',
    label: 'Kadenz',
    hasKey: 'has_cadence',
    allowFuse: false,
    getDisplay: () => 'Cadence',
  },
  {
    key: 'temperature',
    label: 'Temperatur',
    hasKey: 'has_temperature',
    allowFuse: false,
    getDisplay: () => 'Temp',
  },
  {
    key: 'speed',
    label: 'Geschwindigkeit',
    hasKey: 'has_gps' as keyof FusionCandidate, // Speed availability proxied via GPS presence
    allowFuse: false,
    getDisplay: () => 'Speed',
  },
]

export default function FusionFieldTable({
  activities,
  fieldMap,
  onFieldChange,
  tagsMerge,
  onTagsMergeChange,
  titleSource,
  onTitleSourceChange,
}: FusionFieldTableProps) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-neutral-200 dark:border-neutral-700">
            <th className="text-left py-2 pr-4 font-medium text-neutral-600 dark:text-neutral-400 w-32">
              Feld
            </th>
            {activities.map(a => (
              <th key={a.id} className="text-center py-2 px-2 font-medium">
                <span
                  className={`inline-block px-2 py-0.5 rounded-full text-xs ${
                    a.isTarget
                      ? 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                      : 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                  }`}
                >
                  {a.label}
                </span>
              </th>
            ))}
            <th className="text-center py-2 px-2 font-medium text-neutral-600 dark:text-neutral-400 text-xs">
              Fuse
            </th>
          </tr>
        </thead>
        <tbody>
          {FIELDS.map(field => {
            const selected = fieldMap[field.key]
            return (
              <tr
                key={field.key}
                className="border-b border-neutral-100 dark:border-neutral-800 hover:bg-neutral-50 dark:hover:bg-neutral-800/30 transition-colors"
              >
                <td className="py-2 pr-4 text-neutral-700 dark:text-neutral-300 font-medium text-xs">
                  {field.label}
                </td>
                {activities.map(a => {
                  const hasData =
                    field.hasKey != null
                      ? (a.candidate?.[field.hasKey] as boolean | undefined) ?? a.isTarget
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
                        {a.candidate && (
                          <span className="text-xs text-neutral-400">
                            {hasData ? field.getDisplay(a.candidate) : '—'}
                          </span>
                        )}
                        {a.isTarget && !a.candidate && (
                          <span className="text-xs text-neutral-400">Target</span>
                        )}
                      </label>
                    </td>
                  )
                })}
                {/* Fuse-Option (nur GPS) */}
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
          })}

          {/* Titel-Zeile */}
          <tr className="border-b border-neutral-100 dark:border-neutral-800 hover:bg-neutral-50 dark:hover:bg-neutral-800/30 transition-colors">
            <td className="py-2 pr-4 text-neutral-700 dark:text-neutral-300 font-medium text-xs">
              Titel
            </td>
            {activities.map(a => (
              <td key={a.id} className="text-center py-2 px-2">
                <label className="inline-flex items-center cursor-pointer">
                  <input
                    type="radio"
                    name="title-source"
                    value={a.id}
                    checked={titleSource === a.id}
                    onChange={() => onTitleSourceChange(a.id)}
                    className="accent-accent-DEFAULT"
                  />
                </label>
              </td>
            ))}
            <td />
          </tr>

          {/* Tags-Zeile */}
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
