/**
 * ZoneTable — editable table of training zones.
 *
 * Columns: Zone Name | Lower Bound | Upper Bound
 * - Lower bounds auto-compute from previous zone's upper bound
 * - Upper bounds are directly editable (last zone shows "max")
 * - Zone names are editable text inputs
 * - Add / Remove zone buttons
 * - Validates strictly ascending order on every change
 */

import { useId } from 'react'

export interface ZoneRow {
  name: string
  high: number
}

interface ZoneTableProps {
  rows: ZoneRow[]
  onChange: (rows: ZoneRow[]) => void
  /** Unit label shown in column header, e.g. "bpm", "W", "km/h" */
  unit: string
  /** Whether to convert mps → km/h for display (speed zones stored as m/s) */
  convertMpsToKmh?: boolean
  disabled?: boolean
}

function toDisplay(value: number, convertMpsToKmh: boolean): number {
  return convertMpsToKmh ? Math.round(value * 3.6 * 10) / 10 : value
}

function fromDisplay(value: number, convertMpsToKmh: boolean): number {
  return convertMpsToKmh ? Math.round((value / 3.6) * 1000) / 1000 : value
}

export default function ZoneTable({ rows, onChange, unit, convertMpsToKmh = false, disabled = false }: ZoneTableProps) {
  const idPrefix = useId()

  function updateName(index: number, name: string) {
    const next = rows.map((r, i) => (i === index ? { ...r, name } : r))
    onChange(next)
  }

  function updateHigh(index: number, displayValue: string) {
    const parsed = parseFloat(displayValue)
    if (isNaN(parsed) || parsed <= 0) return
    const storeValue = fromDisplay(parsed, convertMpsToKmh)

    const next = rows.map((r, i) => (i === index ? { ...r, high: storeValue } : r))

    // Enforce ascending order: nudge subsequent zones if needed
    for (let i = index + 1; i < next.length; i++) {
      if (next[i].high <= next[i - 1].high) {
        next[i] = { ...next[i], high: fromDisplay(toDisplay(next[i - 1].high, convertMpsToKmh) + (convertMpsToKmh ? 0.1 : 1), convertMpsToKmh) }
      }
    }
    onChange(next)
  }

  function addZone() {
    const lastHigh = rows.length > 0 ? rows[rows.length - 1].high : 0
    const increment = convertMpsToKmh ? fromDisplay(1, true) : 10
    onChange([...rows, { name: `Zone ${rows.length + 1}`, high: lastHigh + increment }])
  }

  function removeZone(index: number) {
    if (rows.length <= 2) return // minimum 2 zones
    onChange(rows.filter((_, i) => i !== index))
  }

  function getLow(index: number): string {
    if (index === 0) return '0'
    const prev = toDisplay(rows[index - 1].high, convertMpsToKmh)
    return convertMpsToKmh ? String(prev + 0.1) : String(Math.floor(prev) + 1)
  }

  return (
    <div className="space-y-2">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-neutral-500 dark:text-neutral-400 border-b border-neutral-200 dark:border-neutral-700">
            <th className="pb-2 font-medium w-32">Zone name</th>
            <th className="pb-2 font-medium w-28 text-right">Low ({unit})</th>
            <th className="pb-2 font-medium w-28 text-right">High ({unit})</th>
            <th className="pb-2 w-8" />
          </tr>
        </thead>
        <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
          {rows.map((row, i) => {
            const isLast = i === rows.length - 1
            const displayHigh = toDisplay(row.high, convertMpsToKmh)

            return (
              <tr key={`${idPrefix}-${i}`} className="group">
                <td className="py-1.5 pr-2">
                  <input
                    type="text"
                    value={row.name}
                    onChange={e => updateName(i, e.target.value)}
                    disabled={disabled}
                    className="w-full rounded border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-2 py-1 text-sm text-neutral-900 dark:text-neutral-100 disabled:opacity-50"
                    aria-label={`Zone ${i + 1} name`}
                  />
                </td>
                <td className="py-1.5 pr-2 text-right text-neutral-500 dark:text-neutral-400 tabular-nums">
                  {getLow(i)}
                </td>
                <td className="py-1.5 pr-2 text-right">
                  {isLast ? (
                    <span className="text-neutral-400 dark:text-neutral-500 tabular-nums">max</span>
                  ) : (
                    <input
                      type="number"
                      value={displayHigh}
                      step={convertMpsToKmh ? 0.1 : 1}
                      min={0}
                      onChange={e => updateHigh(i, e.target.value)}
                      disabled={disabled}
                      className="w-24 rounded border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-2 py-1 text-sm text-right text-neutral-900 dark:text-neutral-100 tabular-nums disabled:opacity-50"
                      aria-label={`Zone ${i + 1} upper bound`}
                    />
                  )}
                </td>
                <td className="py-1.5 text-right">
                  <button
                    type="button"
                    onClick={() => removeZone(i)}
                    disabled={disabled || rows.length <= 2}
                    className="opacity-0 group-hover:opacity-100 text-neutral-400 hover:text-red-500 disabled:opacity-0 transition-opacity text-xs leading-none px-1"
                    aria-label={`Remove zone ${i + 1}`}
                    title="Remove zone"
                  >
                    ✕
                  </button>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>

      <button
        type="button"
        onClick={addZone}
        disabled={disabled || rows.length >= 10}
        className="text-xs text-accent-DEFAULT hover:text-accent-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
      >
        + Add zone
      </button>
    </div>
  )
}
