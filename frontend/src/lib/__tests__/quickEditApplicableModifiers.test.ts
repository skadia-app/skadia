import { describe, it, expect } from 'vitest'
import { QuickEditApplicableModifierKeys, isQuickEditApplicableModifier } from '../generatedQuickEditApplicableModifiers'

describe('quickEdit applicable modifiers', () => {
  it('only iterates applicable modifiers from generated list', () => {
    expect([...QuickEditApplicableModifierKeys]).toEqual(['is_electric', 'is_indoor', 'is_virtual'])
  })

  it('isQuickEditApplicableModifier returns true for known keys', () => {
    expect(isQuickEditApplicableModifier('is_electric')).toBe(true)
    expect(isQuickEditApplicableModifier('is_indoor')).toBe(true)
    expect(isQuickEditApplicableModifier('is_virtual')).toBe(true)
  })

  it('isQuickEditApplicableModifier returns false for unknown keys', () => {
    expect(isQuickEditApplicableModifier('is_commute')).toBe(false)
    expect(isQuickEditApplicableModifier('')).toBe(false)
  })
})
