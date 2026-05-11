// i18n-strings — centralized for future extraction
const STRINGS = {
  numberPlaceholder: '0',
} as const

export interface UnitNumberValue {
  value: number | null
  unit: string
}

export interface UnitNumberInputProps {
  id?: string
  value: UnitNumberValue | null
  allowedUnits: string[]
  onChange: (value: UnitNumberValue) => void
  disabled?: boolean
}

export default function UnitNumberInput({
  id,
  value,
  allowedUnits,
  onChange,
  disabled = false,
}: UnitNumberInputProps) {
  const inputId = id ?? 'unit-number-input'
  const currentUnit = value?.unit ?? allowedUnits[0] ?? ''
  const currentNumber = value?.value ?? null
  const isSingleUnit = allowedUnits.length === 1

  function handleNumberChange(e: React.ChangeEvent<HTMLInputElement>) {
    const raw = e.target.value
    if (raw === '') {
      onChange({ value: null, unit: currentUnit })
      return
    }
    const parsed = parseFloat(raw)
    // Treat NaN (non-numeric input) as null — browser number inputs coerce these
    // to '' in real use, but we guard here for robustness.
    onChange({ value: isNaN(parsed) ? null : parsed, unit: currentUnit })
  }

  function handleUnitChange(e: React.ChangeEvent<HTMLSelectElement>) {
    onChange({ value: currentNumber, unit: e.target.value })
  }

  return (
    <div className="flex gap-2">
      <input
        id={inputId}
        type="number"
        value={currentNumber === null ? '' : String(currentNumber)}
        disabled={disabled}
        placeholder={STRINGS.numberPlaceholder}
        onChange={handleNumberChange}
        className="min-w-0 flex-1 rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT disabled:opacity-50"
      />
      {isSingleUnit ? (
        <span
          aria-label={`Einheit: ${currentUnit}`}
          className="flex items-center px-3 py-2 text-sm text-neutral-500 dark:text-neutral-400 border border-neutral-300 dark:border-neutral-600 rounded bg-neutral-50 dark:bg-neutral-800 select-none"
        >
          {currentUnit}
        </span>
      ) : (
        <select
          aria-label="Einheit"
          value={currentUnit}
          disabled={disabled}
          onChange={handleUnitChange}
          className="rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-2 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT disabled:opacity-50"
        >
          {allowedUnits.map((u) => (
            <option key={u} value={u}>{u}</option>
          ))}
        </select>
      )}
    </div>
  )
}
