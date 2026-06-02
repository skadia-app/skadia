import { useState, useMemo } from 'react'
import type { OptionValue } from '@/lib/generatedGearSchema'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import { gearSchemaLabel } from '@/lib/gearSchemaHelpers'
import { useTranslation } from 'react-i18next'

type DisplayOptionValue = OptionValue & { displayLabel?: string }

export interface MultiEnumInputProps {
  id?: string
  value: string[]
  options: DisplayOptionValue[]
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
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const strings = {
    searchPlaceholder: t('gearZones.multiEnumInput.searchPlaceholder'),
    noOptions: t('gearZones.multiEnumInput.noOptions'),
    removeChip: (label: string) => t('gearZones.multiEnumInput.removeChip', { label }),
    customValueLabel: (value: string) => t('gearZones.multiEnumInput.customValueLabel', { value }),
  }
  const inputId = id ?? 'multi-enum-input'
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)

  const selectedSet = useMemo(() => new Set(value), [value])

  const available = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return options.filter(
      (o) => {
        const label = optionLabel(o, language)
        return !selectedSet.has(o.key) &&
          (needle === '' ||
            label.toLowerCase().includes(needle) ||
            o.key.toLowerCase().includes(needle))
      },
    )
  }, [options, selectedSet, query, language])

  const selectedOptions = useMemo(
    () => value.map((key) => {
      const option = options.find(o => o.key === key)
      return { key, label: option ? schemaOptionLabel(option, language) : key }
    }),
    [options, value, language],
  )

  const customValue = query.trim()
  const exactOption = options.some(
    o => o.key.toLowerCase() === customValue.toLowerCase() ||
      optionLabel(o, language).toLowerCase() === customValue.toLowerCase() ||
      schemaOptionLabel(o, language).toLowerCase() === customValue.toLowerCase(),
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
              {opt.label}
              <button
                type="button"
                aria-label={strings.removeChip(opt.label)}
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
          placeholder={strings.searchPlaceholder}
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
                  {optionLabel(opt, language)}
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
                    {strings.customValueLabel(customValue)}
                  </button>
                )}
              </>
            ) : (
              <div className="px-3 py-2 text-sm text-neutral-400">{strings.noOptions}</div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function optionLabel(option: DisplayOptionValue, language: ReturnType<typeof supportedLanguageFrom>): string {
  return option.displayLabel ?? schemaOptionLabel(option, language)
}

function schemaOptionLabel(option: DisplayOptionValue, language: ReturnType<typeof supportedLanguageFrom>): string {
  return gearSchemaLabel(option, language)
}
