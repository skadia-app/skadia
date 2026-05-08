import { describe, expect, it } from 'vitest'
import { applyMapModifierParams } from '../mapFilterUrl'

describe('applyMapModifierParams', () => {
  it('serializes map electric and commute filters as modifier columns, not system tags', () => {
    const params = new URLSearchParams()

    applyMapModifierParams(params, { isElectric: true, isCommute: false })

    expect(params.get('is_electric')).toBe('true')
    expect(params.get('is_commute')).toBe('false')
    expect(params.has('include_tags')).toBe(false)
    expect(params.has('exclude_tags')).toBe(false)
  })

  it('omits neutral modifiers', () => {
    const params = new URLSearchParams()

    applyMapModifierParams(params, { isElectric: null, isCommute: null })

    expect(params.toString()).toBe('')
  })
})
