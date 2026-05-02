import { useState } from 'react'
import { SportFilter } from '@/components/sport/SportFilter'
import type { SportFilterValue } from '@/components/sport/SportFilter'
import { expandSportSelection } from '@/components/analytics/filters/SportOnlyFilterBar'

// F008: Modal only uses sportTypes from SportFilterValue.
// predicates + modifiers are accepted by SportFilter but not propagated.
//
// F002: expandSportSelection handles the bare-category → sport:subtype workaround.

// Sentinel-aware value shape:
// - null  = "all selected" (default, no filter applied)
// - []    = "explicitly none selected" (after 'Alle abwählen')
// - [...] = "specific subset"
export type SportPickerValue = string[] | null

interface SportHierarchyPickerProps {
  value: SportPickerValue
  onChange: (sports: SportPickerValue) => void
  className?: string
  // Optional date range that scopes "hideUnused" — only sports active in
  // [from, to] are shown. Without it, falls back to all-time stats.
  dateRange?: { from?: string; to?: string }
}

export default function SportHierarchyPicker({ value, onChange, className, dateRange }: SportHierarchyPickerProps) {
  const [filterValue, setFilterValue] = useState<SportFilterValue>({
    sportTypes: value,
  })

  function handleChange(next: SportFilterValue) {
    setFilterValue(next)
    if (next.sportTypes === null) {
      onChange(null)
      return
    }
    if (next.sportTypes.length === 0) {
      onChange([])
      return
    }
    onChange(expandSportSelection(next.sportTypes))
  }

  return (
    <SportFilter
      preset="ACTIVITY_ALL"
      value={filterValue}
      onChange={handleChange}
      showModifiers={false}
      hideUnused
      dateRange={dateRange}
      className={className}
    />
  )
}
