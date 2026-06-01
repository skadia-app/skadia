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
import { useTranslation } from 'react-i18next'
import { speedFromDisplayValue, speedToDisplayValue, type UnitSystem } from '@/lib/units'

export interface ZoneRow {
  name: string
  high: number
}

interface ZoneTableProps {
  rows: ZoneRow[]
  onChange: (rows: ZoneRow[]) => void
  /** Unit label shown in column header, e.g. "bpm", "W", "km/h", "mph" */
  unit: string
  unitSystem?: UnitSystem
  /** Whether to convert m/s speed-zone values into the display speed unit */
  convertMpsToKmh?: boolean
  disabled?: boolean
}

function roundDisplaySpeed(value: number): number {
  return Math.round(value * 10) / 10
}

function resolveSpeedUnitSystem(unitSystem: UnitSystem | undefined, unit: string): UnitSystem {
  return unitSystem ?? (unit === 'mph' ? 'imperial' : 'metric')
}

function toDisplay(value: number, convertMpsToSpeed: boolean, unitSystem: UnitSystem): number {
  if (!convertMpsToSpeed) return value
  return roundDisplaySpeed(speedToDisplayValue(value, unitSystem))
}

function fromDisplay(value: number, convertMpsToSpeed: boolean, unitSystem: UnitSystem): number {
  if (!convertMpsToSpeed) return value
  const metersPerSecond = speedFromDisplayValue(value, unitSystem)
  return Math.round(metersPerSecond * 1000) / 1000
}

export default function ZoneTable({ rows, onChange, unit, unitSystem, convertMpsToKmh = false, disabled = false }: ZoneTableProps) {
  const idPrefix = useId()
  const { t } = useTranslation()
  const speedUnits = resolveSpeedUnitSystem(unitSystem, unit)

  function updateName(index: number, name: string) {
    const next = rows.map((r, i) => (i === index ? { ...r, name } : r))
    onChange(next)
  }

  function updateHigh(index: number, displayValue: string) {
    const parsed = parseFloat(displayValue)
    if (isNaN(parsed) || parsed <= 0) return
    const storeValue = fromDisplay(parsed, convertMpsToKmh, speedUnits)

    const next = rows.map((r, i) => (i === index ? { ...r, high: storeValue } : r))

    // Enforce ascending order: nudge subsequent zones if needed
    for (let i = index + 1; i < next.length; i++) {
      if (next[i].high <= next[i - 1].high) {
        next[i] = { ...next[i], high: fromDisplay(toDisplay(next[i - 1].high, convertMpsToKmh, speedUnits) + (convertMpsToKmh ? 0.1 : 1), convertMpsToKmh, speedUnits) }
      }
    }
    onChange(next)
  }

  function addZone() {
    const lastHigh = rows.length > 0 ? rows[rows.length - 1].high : 0
    const increment = convertMpsToKmh ? fromDisplay(1, true, speedUnits) : 10
    onChange([...rows, { name: t('activityTables.zoneTable.defaultName', { number: rows.length + 1 }), high: lastHigh + increment }])
  }

  function removeZone(index: number) {
    if (rows.length <= 2) return // minimum 2 zones
    onChange(rows.filter((_, i) => i !== index))
  }

  function getLow(index: number): string {
    if (index === 0) return '0'
    const prev = toDisplay(rows[index - 1].high, convertMpsToKmh, speedUnits)
    return convertMpsToKmh ? roundDisplaySpeed(prev + 0.1).toFixed(1) : String(Math.floor(prev) + 1)
  }

  return (
    <div className="space-y-2">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-neutral-500 dark:text-neutral-400 border-b border-neutral-200 dark:border-neutral-700">
            <th className="pb-2 font-medium w-32">{t('activityTables.zoneTable.name')}</th>
            <th className="pb-2 font-medium w-28 text-right">{t('activityTables.zoneTable.low')} ({unit})</th>
            <th className="pb-2 font-medium w-28 text-right">{t('activityTables.zoneTable.high')} ({unit})</th>
            <th className="pb-2 w-8" />
          </tr>
        </thead>
        <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
          {rows.map((row, i) => {
            const isLast = i === rows.length - 1
            const displayHigh = toDisplay(row.high, convertMpsToKmh, speedUnits)

            return (
              <tr key={`${idPrefix}-${i}`} className="group">
                <td className="py-1.5 pr-2">
                  <input
                    type="text"
                    value={row.name}
                    onChange={e => updateName(i, e.target.value)}
                    disabled={disabled}
                    className="w-full rounded border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-2 py-1 text-sm text-neutral-900 dark:text-neutral-100 disabled:opacity-50"
                    aria-label={t('activityTables.zoneTable.nameAria', { number: i + 1 })}
                  />
                </td>
                <td className="py-1.5 pr-2 text-right text-neutral-500 dark:text-neutral-400 tabular-nums">
                  {getLow(i)}
                </td>
                <td className="py-1.5 pr-2 text-right">
                  {isLast ? (
                    <span className="text-neutral-400 dark:text-neutral-500 tabular-nums">{t('activityTables.zoneTable.max')}</span>
                  ) : (
                    <input
                      type="number"
                      value={displayHigh}
                      step={convertMpsToKmh ? 0.1 : 1}
                      min={0}
                      onChange={e => updateHigh(i, e.target.value)}
                      disabled={disabled}
                      className="w-24 rounded border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-2 py-1 text-sm text-right text-neutral-900 dark:text-neutral-100 tabular-nums disabled:opacity-50"
                      aria-label={t('activityTables.zoneTable.upperBoundAria', { number: i + 1 })}
                    />
                  )}
                </td>
                <td className="py-1.5 text-right">
                  <button
                    type="button"
                    onClick={() => removeZone(i)}
                    disabled={disabled || rows.length <= 2}
                    className="opacity-0 group-hover:opacity-100 text-neutral-400 hover:text-red-500 disabled:opacity-0 transition-opacity text-xs leading-none px-1"
                    aria-label={t('activityTables.zoneTable.removeAria', { number: i + 1 })}
                    title={t('activityTables.zoneTable.removeTitle')}
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
        {t('activityTables.zoneTable.add')}
      </button>
    </div>
  )
}
