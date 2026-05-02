import { useState } from 'react'
import { SportFilter } from '@/components/sport/SportFilter'
import type { SportFilterValue } from '@/components/sport/SportFilter'
import RangePresetBar from './RangePresetBar'
import type { Preset, DateRange, PresetValue } from './RangePresetBar'
import { sportsInCategory } from '@/lib/sportTaxonomy'
import { CATEGORIES } from '@/lib/sportTaxonomy'

export interface SportOnlyFilterValue {
  selectedSports: string[] | null
  from: string
  to: string
}

interface Props {
  value: SportOnlyFilterValue
  onChange: (value: SportOnlyFilterValue) => void
  rangePresets?: Preset[]
}

const DEFAULT_PRESETS: Preset[] = [
  { value: '1y', label: '1 Jahr' },
  { value: 'all', label: 'Alles' },
  { value: 'custom', label: 'Benutzerdefiniert' },
]

/**
 * Expands a mixed list of sport tokens (bare-category, sport:subtype, sport) into
 * only `sport:subtype` tokens.
 *
 * // WORKAROUND bis sport-taxonomy-rework: bare-sport tokens kollabieren subtype filter im Backend (filter.go:165-218). Wir expandieren clients-side. Entfernen sobald Parallel-Session den Parser fixt.
 */
export function expandSportSelection(selectedSports: string[]): string[] {
  const categoryKeys = new Set(CATEGORIES.map(c => c.key))
  const result: string[] = []

  for (const token of selectedSports) {
    if (token.includes(':')) {
      // Already a sport:subtype token — pass through.
      result.push(token)
      continue
    }

    if (categoryKeys.has(token)) {
      // Bare category key — expand to all subtype tokens within the category.
      const defs = sportsInCategory(token)
      for (const def of defs) {
        if (def.subTypes.length > 0) {
          for (const sub of def.subTypes) {
            result.push(`${def.sportType}:${sub.subType}`)
          }
        } else {
          // Sports without subtypes can't be represented as sport:subtype tokens;
          // emit the bare sport token as a fallback for this edge case.
          result.push(def.sportType)
        }
      }
      continue
    }

    // Unrecognised bare token (neither a known category key nor a sport:subtype pair).
    // Passing it to the backend would produce a non-matching filter; drop it and warn.
    console.warn('[expandSportSelection] dropping unrecognised sport token:', token)
  }

  return result
}

function sportFilterValueFromSelected(selected: string[] | null): SportFilterValue {
  return { sportTypes: selected }
}

export default function SportOnlyFilterBar({ value, onChange, rangePresets }: Props) {
  const presets = rangePresets ?? DEFAULT_PRESETS
  const [rangePreset, setRangePreset] = useState<PresetValue>('all')

  function handleSportChange(sfv: SportFilterValue) {
    onChange({ ...value, selectedSports: sfv.sportTypes === null ? null : sfv.sportTypes })
  }

  function handleRangeChange(range: DateRange, preset: PresetValue) {
    setRangePreset(preset)
    onChange({ ...value, from: range.from, to: range.to })
  }

  const sportFilterValue = sportFilterValueFromSelected(value.selectedSports)

  return (
    <div className="space-y-3">
      <RangePresetBar
        presets={presets}
        value={rangePreset}
        onChange={handleRangeChange}
      />
      <SportFilter
        preset="ACTIVITY_ALL"
        value={sportFilterValue}
        onChange={handleSportChange}
        showModifiers={false}
      />
    </div>
  )
}
