import { useState } from 'react'
import { SportFilter } from '@/components/sport/SportFilter'
import type { SportFilterValue } from '@/components/sport/SportFilter'
import { expandSportSelection } from '@/components/analytics/filters/SportOnlyFilterBar'

// F008: Modal only uses sportTypes from SportFilterValue.
// predicates + modifiers are accepted by SportFilter but not propagated.
//
// F002: expandSportSelection handles the bare-category → sport:subtype workaround.

interface SportHierarchyPickerProps {
  value: string[]
  onChange: (sports: string[]) => void
  className?: string
}

export default function SportHierarchyPicker({ value, onChange, className }: SportHierarchyPickerProps) {
  const [filterValue, setFilterValue] = useState<SportFilterValue>({
    sportTypes: value.length > 0 ? value : null,
  })

  function handleChange(next: SportFilterValue) {
    setFilterValue(next)
    // Extract only sportTypes, expand category-level tokens before passing up.
    const raw = next.sportTypes ?? []
    const expanded = expandSportSelection(raw)
    onChange(expanded)
  }

  return (
    <SportFilter
      preset="ACTIVITY_ALL"
      value={filterValue}
      onChange={handleChange}
      showModifiers={false}
      className={className}
    />
  )
}
