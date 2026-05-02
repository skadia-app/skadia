import { describe, expect, it } from 'vitest'
import {
  filterSegmentSportSelection,
  removeSegmentSportSelectionKeys,
} from './segmentSportSelection'

describe('segment sport selection helpers', () => {
  it('keeps an explicit empty selection when the last selected sport is removed', () => {
    expect(removeSegmentSportSelectionKeys(['cycling'], ['cycling'])).toEqual([])
  })

  it('keeps null as all sports when no restricted selection exists', () => {
    expect(removeSegmentSportSelectionKeys(null, ['cycling'])).toBeNull()
  })

  it('keeps an explicit empty selection when segment-type filtering removes every selected sport', () => {
    expect(filterSegmentSportSelection(['cycling'], ['swimming'])).toEqual([])
  })

  it('removes old water and winter selections when switching to standard eligibility', () => {
    expect(filterSegmentSportSelection(['swimming', 'snowshoeing'], ['running', 'cycling'])).toEqual([])
  })

  it('preserves null so save helpers can expand all sports deliberately', () => {
    expect(filterSegmentSportSelection(null, ['swimming'])).toBeNull()
  })
})
