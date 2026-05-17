import { useState, useRef, type ReactNode } from 'react'
import type { Capabilities } from '@/api/routes'
import { REPROCESS_SCOPES, GEO_SCOPE_KEYS, GEO_SCOPE_LABELS, GEO_SCOPE_DESCRIPTIONS, scopeAvailable, type AnyReprocessKey } from '@/lib/reprocessScopes'
import { scopeRequiresML } from '@/lib/enrichmentModuleCatalog'

interface CheckboxItemProps {
  label: string
  description?: string
  checked: boolean
  onChange: () => void
  isInline: boolean
  badge?: ReactNode
}

function CheckboxItem({ label, description, checked, onChange, isInline, badge }: CheckboxItemProps) {
  const [visible, setVisible] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const handleEnter = () => {
    if (!description) return
    timer.current = setTimeout(() => setVisible(true), 1000)
  }

  const handleLeave = () => {
    if (timer.current) { clearTimeout(timer.current); timer.current = null }
    setVisible(false)
  }

  return (
    <label
      className={isInline
        ? 'relative flex items-center gap-1.5 cursor-pointer'
        : 'relative flex items-center gap-3 cursor-pointer group select-none'
      }
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
    >
      <input
        type="checkbox"
        checked={checked}
        onChange={onChange}
        className={isInline ? 'rounded' : 'accent-accent-DEFAULT'}
      />
      <span className={isInline
        ? 'text-sm text-neutral-700 dark:text-neutral-300'
        : 'text-sm text-neutral-600 dark:text-neutral-400 group-hover:text-neutral-900 dark:group-hover:text-neutral-100'
      }>
        {label}{badge}
      </span>
      {description && visible && (
        <div className="absolute left-0 top-full mt-1 z-50 px-2.5 py-1.5 rounded-md
                        bg-neutral-800 dark:bg-neutral-700 text-white text-xs
                        shadow-lg pointer-events-none w-max max-w-64 leading-snug">
          {description}
        </div>
      )}
    </label>
  )
}

interface Props {
  selected: Set<AnyReprocessKey>
  onToggle: (key: AnyReprocessKey) => void
  layout?: 'vertical' | 'inline'
  capabilities?: Capabilities | null
}

export function ScopeCheckboxList({ selected, onToggle, layout = 'vertical', capabilities }: Props) {
  const isInline = layout === 'inline'
  const visibleReprocessScopes = REPROCESS_SCOPES.filter(([key]) => scopeAvailable(capabilities, key))
  const visibleGeoScopes = GEO_SCOPE_KEYS.filter(key => scopeAvailable(capabilities, key))

  return (
    <div className={isInline ? 'flex flex-wrap gap-x-5 gap-y-1.5' : 'space-y-2'}>
      {visibleReprocessScopes.map(([key, label, description]) => (
        <CheckboxItem
          key={key}
          label={label}
          description={description}
          checked={selected.has(key)}
          onChange={() => onToggle(key)}
          isInline={isInline}
        />
      ))}
      <p className={isInline
        ? 'w-full pt-1 text-xs text-neutral-400 dark:text-neutral-500'
        : 'pt-1 text-xs text-neutral-400 dark:text-neutral-500'
      }>
        Geo-Anreicherung (asynchron)
      </p>
      {visibleGeoScopes.map(key => (
        <CheckboxItem
          key={key}
          label={GEO_SCOPE_LABELS[key]}
          description={GEO_SCOPE_DESCRIPTIONS[key]}
          checked={selected.has(key)}
          onChange={() => onToggle(key)}
          isInline={isInline}
          badge={scopeRequiresML(key) ? (
            <span className="ml-1 text-xs font-medium text-purple-600 dark:text-purple-400">ML</span>
          ) : undefined}
        />
      ))}
    </div>
  )
}
