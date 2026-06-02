import { useState, useMemo, useEffect, useRef } from 'react'
import type { OptionValue } from '@/lib/generatedGearSchema'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import { gearSchemaLabel } from '@/lib/gearSchemaHelpers'
import { useTranslation } from 'react-i18next'

type DisplayOptionValue = OptionValue & { displayLabel?: string }

export interface SearchableEnumInputProps {
  id?: string
  value: string
  options: DisplayOptionValue[]
  onChange: (value: string) => void
  /** When true, freetext values not in the options list are accepted */
  allowFreeText?: boolean
  placeholder?: string
  disabled?: boolean
}

export default function SearchableEnumInput({
  id,
  value,
  options,
  onChange,
  allowFreeText = false,
  placeholder,
  disabled = false,
}: SearchableEnumInputProps) {
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const strings = {
    searchPlaceholder: t('gearZones.searchableEnumInput.searchPlaceholder'),
    noResults: t('gearZones.searchableEnumInput.noResults'),
    customValueLabel: (value: string) => t('gearZones.searchableEnumInput.customValueLabel', { value }),
  }
  const inputId = id ?? 'searchable-enum-input'
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(-1)
  const listRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Sync query with externally-controlled value label
  const selectedOption = options.find(o => o.key === value)
  const displayValue = selectedOption ? schemaOptionLabel(selectedOption, language) : value
  const resolvedPlaceholder = placeholder ?? strings.searchPlaceholder

  useEffect(() => {
    if (!open) setQuery(displayValue)
  }, [open, displayValue])

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle || query === displayValue) return options
    return options.filter(
      (o) => {
        const label = optionLabel(o, language)
        return label.toLowerCase().includes(needle) ||
          o.key.toLowerCase().includes(needle)
      },
    )
  }, [options, query, displayValue, language])

  // Whether the current query is a custom value (not matching any option key or label exactly)
  const isCustomValue =
    allowFreeText &&
    query.trim() !== '' &&
    query !== displayValue &&
    !options.some(o => optionMatchesQuery(o, query.trim(), language))

  const totalItems = filtered.length + (isCustomValue ? 1 : 0)

  function choose(optKey: string) {
    onChange(optKey)
    setOpen(false)
  }

  function chooseCustom() {
    if (!allowFreeText) return
    const custom = query.trim()
    if (!custom) return
    onChange(custom)
    setOpen(false)
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (!open) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        setOpen(true)
        e.preventDefault()
      }
      return
    }

    if (e.key === 'Escape') {
      setOpen(false)
      e.currentTarget.blur()
      return
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex(prev => Math.min(prev + 1, totalItems - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex(prev => Math.max(prev - 1, 0))
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      if (activeIndex >= 0) {
        e.preventDefault()
        if (activeIndex < filtered.length) {
          choose(filtered[activeIndex].key)
        } else {
          chooseCustom()
        }
      } else if (e.key === 'Enter') {
        // When nothing highlighted but freetext allowed
        if (isCustomValue) {
          e.preventDefault()
          chooseCustom()
        }
      }
    }
  }

  return (
    <div
      ref={containerRef}
      className="relative"
      onBlur={(e) => {
        const next = e.relatedTarget as Node | null
        if (!next || !containerRef.current?.contains(next)) {
          setOpen(false)
          setActiveIndex(-1)
        }
      }}
    >
      <input
        id={inputId}
        role="combobox"
        aria-expanded={open}
        aria-controls={`${inputId}-listbox`}
        aria-autocomplete="list"
        aria-activedescendant={
          open && activeIndex >= 0 ? `${inputId}-option-${activeIndex}` : undefined
        }
        type="text"
        value={open ? query : displayValue}
        disabled={disabled}
        placeholder={resolvedPlaceholder}
        onFocus={() => {
          setQuery('')
          setOpen(true)
          setActiveIndex(-1)
        }}
        onChange={(e) => {
          setQuery(e.target.value)
          setActiveIndex(-1)
          setOpen(true)
        }}
        onKeyDown={handleKeyDown}
        className="w-full rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT disabled:opacity-50"
      />
      {open && (
        <div
          ref={listRef}
          id={`${inputId}-listbox`}
          role="listbox"
          className="absolute z-30 mt-1 max-h-64 w-full overflow-y-auto rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg"
        >
          {filtered.length > 0 ? (
            filtered.map((opt, idx) => (
              <button
                key={opt.key}
                id={`${inputId}-option-${idx}`}
                type="button"
                role="option"
                aria-selected={opt.key === value}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => choose(opt.key)}
                className={`block w-full px-3 py-2 text-left text-sm text-neutral-800 dark:text-neutral-100 cursor-pointer ${
                  activeIndex === idx
                    ? 'bg-accent-50 dark:bg-accent-900/30'
                    : 'hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                {optionLabel(opt, language)}
              </button>
            ))
          ) : !isCustomValue ? (
            <div className="px-3 py-2 text-sm text-neutral-400">{strings.noResults}</div>
          ) : null}
          {isCustomValue && (
            <button
              id={`${inputId}-option-${filtered.length}`}
              type="button"
              role="option"
              aria-selected={false}
              onMouseDown={(e) => e.preventDefault()}
              onClick={chooseCustom}
              className={`block w-full px-3 py-2 text-left text-sm text-neutral-500 dark:text-neutral-400 italic cursor-pointer ${
                activeIndex === filtered.length
                  ? 'bg-accent-50 dark:bg-accent-900/30'
                  : 'hover:bg-neutral-100 dark:hover:bg-neutral-800'
              }`}
            >
              {strings.customValueLabel(query.trim())}
            </button>
          )}
        </div>
      )}
    </div>
  )
}

function optionLabel(option: DisplayOptionValue, language: ReturnType<typeof supportedLanguageFrom>): string {
  return option.displayLabel ?? schemaOptionLabel(option, language)
}

function schemaOptionLabel(option: DisplayOptionValue, language: ReturnType<typeof supportedLanguageFrom>): string {
  return gearSchemaLabel(option, language)
}

function optionMatchesQuery(option: DisplayOptionValue, query: string, language: ReturnType<typeof supportedLanguageFrom>): boolean {
  return option.key === query ||
    optionLabel(option, language) === query ||
    schemaOptionLabel(option, language) === query
}
