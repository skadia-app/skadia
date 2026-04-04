import { REPROCESS_SCOPES, GEO_SCOPE_KEYS, GEO_SCOPE_LABELS, type AnyReprocessKey } from '@/lib/reprocessScopes'

interface Props {
  selected: Set<AnyReprocessKey>
  onToggle: (key: AnyReprocessKey) => void
  layout?: 'vertical' | 'inline'
  showDescriptions?: boolean
}

export function ScopeCheckboxList({ selected, onToggle, layout = 'vertical', showDescriptions = false }: Props) {
  const isInline = layout === 'inline'

  const containerClass = isInline
    ? 'flex flex-wrap gap-x-5 gap-y-1.5'
    : 'space-y-2'

  const labelClass = isInline
    ? 'flex items-center gap-1.5 text-sm text-neutral-700 dark:text-neutral-300 cursor-pointer'
    : showDescriptions
      ? 'flex items-start gap-2.5 text-sm text-neutral-700 dark:text-neutral-300 cursor-pointer select-none'
      : 'flex items-center gap-3 cursor-pointer group'

  const inputClass = isInline
    ? 'rounded'
    : showDescriptions
      ? 'rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT mt-0.5'
      : 'accent-accent-DEFAULT'

  const spanClass = isInline
    ? undefined
    : showDescriptions
      ? undefined
      : 'text-sm text-neutral-600 dark:text-neutral-400 group-hover:text-neutral-900 dark:group-hover:text-neutral-100'

  const separatorClass = isInline
    ? 'text-xs text-neutral-400 dark:text-neutral-500 w-full pt-1'
    : 'text-xs text-neutral-400 dark:text-neutral-500 pt-1'

  return (
    <div className={containerClass}>
      {REPROCESS_SCOPES.map(([key, label, description]) => (
        <label key={key} className={labelClass}>
          <input
            type="checkbox"
            checked={selected.has(key)}
            onChange={() => onToggle(key)}
            className={inputClass}
          />
          {showDescriptions ? (
            <span>
              {label}
              {description && (
                <span className="block text-xs text-neutral-400 dark:text-neutral-500 font-normal">
                  {description}
                </span>
              )}
            </span>
          ) : spanClass ? (
            <span className={spanClass}>{label}</span>
          ) : (
            label
          )}
        </label>
      ))}
      <p className={separatorClass}>Geo-Anreicherung (ML, asynchron)</p>
      {GEO_SCOPE_KEYS.map(key => (
        <label key={key} className={labelClass}>
          <input
            type="checkbox"
            checked={selected.has(key)}
            onChange={() => onToggle(key)}
            className={isInline ? inputClass : showDescriptions ? 'rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT' : inputClass}
          />
          {spanClass ? (
            <span className={spanClass}>{GEO_SCOPE_LABELS[key]}</span>
          ) : (
            GEO_SCOPE_LABELS[key]
          )}
        </label>
      ))}
    </div>
  )
}
