import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { ActivitySet } from '@/api/types'
import { StrengthSetsTable } from './StrengthSetsTable'

function activitySet(overrides: Partial<ActivitySet> = {}): ActivitySet {
  return {
    message_index: 1,
    set_type: 0,
    repetitions: 10,
    weight_kg: 100,
    exercise_categories: [0],
    ...overrides,
  }
}

describe('StrengthSetsTable', () => {
  it('formats FIT kilogram display-unit weights without changing the stored kg value', () => {
    render(<StrengthSetsTable sets={[activitySet({ weight_display_unit: 1 })]} />)

    expect(screen.getByText('100.0 kg')).toBeInTheDocument()
  })

  it('converts stored kg weights when FIT display-unit requests pounds', () => {
    render(<StrengthSetsTable sets={[activitySet({ weight_display_unit: 2 })]} />)

    expect(screen.getByText('220.5 lbs')).toBeInTheDocument()
  })

  it('does not guess unknown imported display-units', () => {
    render(<StrengthSetsTable sets={[activitySet({ weight_display_unit: 0 })]} />)

    expect(screen.getByText('100.0 kg')).toBeInTheDocument()
  })
})
