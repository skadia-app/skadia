import { cn } from '@/lib/utils'

export interface ColoredSelectOption {
  value: string
  label: string
  /** Optional CSS color (hex or css-name) rendered as a dot in the trigger when this option is selected. */
  color?: string
  /**
   * Optional shorter text shown in the *trigger* when this option is selected.
   * The full `label` is still shown in the dropdown list. Useful when the
   * full label is too long for narrow layouts (e.g. "T3 — anspruchsvolles
   * Bergwandern" → `shortLabel: 'T3'`).
   */
  shortLabel?: string
}

interface ColoredSelectProps {
  value: string | null
  onChange: (value: string | null) => void
  options: ReadonlyArray<ColoredSelectOption>
  placeholder?: string
  disabled?: boolean
  className?: string
  /**
   * If true and `value` is not in `options`, it is rendered as an extra
   * "(veraltet) X" option and the trigger turns amber. Useful when a stored
   * legacy value must remain selectable until the user picks a valid one.
   */
  allowLegacy?: boolean
}

/**
 * Native `<select>` styled to match the app theme, with an optional color dot
 * rendered inside the trigger (left of the value) when the selected option
 * carries a `color`. Replaces our previous Radix-based `DropdownMenu` selects —
 * the native element avoids React-19 + Radix `Presence` re-entrant render loops,
 * gives us OS-native typeahead, and keeps look-and-feel consistent with other
 * `<select>` inputs in the app.
 */
export function ColoredSelect({
  value,
  onChange,
  options,
  placeholder = '—',
  disabled,
  className,
  allowLegacy,
}: ColoredSelectProps) {
  const current = options.find(o => o.value === value) ?? null
  const isLegacy = !!allowLegacy && value != null && value !== '' && !current
  const swatch = current?.color
  const triggerShort = current?.shortLabel && current.shortLabel !== current.label ? current.shortLabel : null

  return (
    <div className={cn('relative', className)}>
      {swatch && (
        <span
          aria-hidden
          className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 h-2 w-2 rounded-full border border-neutral-300 dark:border-neutral-600 z-10"
          style={{ backgroundColor: swatch }}
        />
      )}
      {triggerShort && (
        // Overlay masks the native select's selected-option text when it would
        // overflow. The dropdown list itself is unaffected (browser renders it
        // from <option>.label). pointer-events-none lets clicks reach the select.
        // Mirrors the <select>'s border + padding box so the text starts at the
        // exact same x-offset as a non-overlaid select would.
        <span
          aria-hidden
          className={cn(
            'pointer-events-none absolute inset-0 flex items-center rounded-md border border-transparent bg-white dark:bg-neutral-800 pr-2 text-sm text-neutral-900 dark:text-neutral-100',
            swatch ? 'pl-5' : 'pl-2',
          )}
        >
          {triggerShort}
        </span>
      )}
      <select
        value={value ?? ''}
        disabled={disabled}
        onChange={e => onChange(e.target.value === '' ? null : e.target.value)}
        className={cn(
          'w-full rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-800 py-1.5 pr-2 text-sm text-neutral-900 dark:text-neutral-100 transition-colors duration-150 hover:border-neutral-400 dark:hover:border-neutral-600 focus:outline-none focus-visible:ring-1 focus-visible:ring-accent-DEFAULT disabled:opacity-50',
          swatch ? 'pl-5' : 'pl-2',
          isLegacy && 'border-amber-400 dark:border-amber-600 text-amber-700 dark:text-amber-400',
        )}
      >
        <option value="">{placeholder}</option>
        {options.map(opt => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
        {isLegacy && value != null && (
          <option value={value}>{`(veraltet) ${value}`}</option>
        )}
      </select>
    </div>
  )
}
