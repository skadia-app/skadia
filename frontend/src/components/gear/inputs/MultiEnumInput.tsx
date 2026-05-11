import { useState, useMemo } from 'react'
import type { OptionValue } from '@/lib/generatedGearSchema'
import { GEAR_INPUT_STRINGS } from '@/lib/gearComponentStrings'

// i18n-strings — centralized for future extraction
const STRINGS = {
  searchPlaceholder: GEAR_INPUT_STRINGS.searchPlaceholder,
  noOptions: 'Keine weiteren Optionen',
  removeChip: (label: string) => `${label} entfernen`,
} as const

export interface MultiEnumInputProps {
  id?: string
  value: string[]
  options: OptionValue[]
  onChange: (value: string[]) => void
  allowFreeText?: boolean
  disabled?: boolean
}

export default function MultiEnumInput({
  id,
  value,
  options,
  onChange,
  allowFreeText = false,
  disabled = false,
}: MultiEnumInputProps) {
  const inputId = id ?? 'multi-enum-input'
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)

  const selectedSet = useMemo(() => new Set(value), [value])

  const available = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return options.filter(
      (o) =>
        !selectedSet.has(o.key) &&
        (needle === '' ||
          o.labelDe.toLowerCase().includes(needle) ||
          o.key.toLowerCase().includes(needle)),
    )
  }, [options, selectedSet, query])

  const selectedOptions = useMemo(
    () => value.map(key => options.find(o => o.key === key) ?? { key, labelDe: key }),
    [options, value],
  )

  const customValue = query.trim()
  const exactOption = options.some(
    o => o.key.toLowerCase() === customValue.toLowerCase() ||
      o.labelDe.toLowerCase() === customValue.toLowerCase(),
  )
  const showCustomOption = allowFreeText &&
    customValue !== '' &&
    !selectedSet.has(customValue) &&
    !exactOption

  function addOption(key: string) {
    if (selectedSet.has(key)) return // dedupe guard
    onChange([...value, key])
    setQuery('')
  }

  function removeOption(key: string) {
    onChange(value.filter((k) => k !== key))
  }

  return (
    <div className="space-y-2">
      {/* Selected chips */}
      {selectedOptions.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {selectedOptions.map((opt) => (
            <span
              key={opt.key}
              className="inline-flex items-center gap-1 rounded-full bg-accent-100 dark:bg-accent-900/40 text-accent-800 dark:text-accent-200 px-2 py-0.5 text-xs font-medium"
            >
              {opt.labelDe}
              <button
                type="button"
                aria-label={STRINGS.removeChip(opt.labelDe)}
                disabled={disabled}
                onClick={() => removeOption(opt.key)}
                className="ml-0.5 rounded-full hover:bg-accent-200 dark:hover:bg-accent-800 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT disabled:opacity-50 cursor-pointer"
              >
                ×
              </button>
            </span>
          ))}
        </div>
      )}

      {/* Search input + dropdown */}
      <div
        className="relative"
        onBlur={(e) => {
          const next = e.relatedTarget as Node | null
          if (!next || !e.currentTarget.contains(next)) {
            setOpen(false)
          }
        }}
      >
        <input
          id={inputId}
          type="text"
          role="combobox"
          aria-expanded={open}
          aria-controls={`${inputId}-listbox`}
          disabled={disabled}
          value={query}
          placeholder={STRINGS.searchPlaceholder}
          onFocus={() => setOpen(true)}
          onChange={(e) => {
            setQuery(e.target.value)
            setOpen(true)
          }}
          className="w-full rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT disabled:opacity-50"
        />
        {open && (
          <div
            id={`${inputId}-listbox`}
            role="listbox"
            className="absolute z-30 mt-1 max-h-48 w-full overflow-y-auto rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg"
          >
            {available.length > 0 || showCustomOption ? (
              <>
                {available.map((opt) => (
                <button
                  key={opt.key}
                  type="button"
                  role="option"
                  aria-selected={false}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => addOption(opt.key)}
                  className="block w-full px-3 py-2 text-left text-sm text-neutral-800 dark:text-neutral-100 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
                >
                  {opt.labelDe}
                </button>
                ))}
                {showCustomOption && (
                  <button
                    type="button"
                    role="option"
                    aria-selected={false}
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => addOption(customValue)}
                    className="block w-full px-3 py-2 text-left text-sm text-accent-700 dark:text-accent-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
                  >
                    "{customValue}" verwenden
                  </button>
                )}
              </>
            ) : (
              <div className="px-3 py-2 text-sm text-neutral-400">{STRINGS.noOptions}</div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
