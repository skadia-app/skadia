import { describe, expect, it } from 'vitest'
import { deepEqual } from './deepEqual'

describe('deepEqual', () => {
  it('returns true for identical primitives', () => {
    expect(deepEqual(1, 1)).toBe(true)
    expect(deepEqual('a', 'a')).toBe(true)
    expect(deepEqual(null, null)).toBe(true)
    expect(deepEqual(undefined, undefined)).toBe(true)
    expect(deepEqual(true, true)).toBe(true)
  })

  it('returns false for differing primitives', () => {
    expect(deepEqual(1, 2)).toBe(false)
    expect(deepEqual('a', 'b')).toBe(false)
    expect(deepEqual(null, undefined)).toBe(false)
    expect(deepEqual(0, false)).toBe(false)
  })

  it('returns true for objects with the same keys/values regardless of order', () => {
    expect(deepEqual({ a: 1, b: 2 }, { b: 2, a: 1 })).toBe(true)
  })

  it('returns true for nested objects with reordered keys', () => {
    expect(deepEqual(
      { outer: { x: 1, y: 2 }, z: 3 },
      { z: 3, outer: { y: 2, x: 1 } },
    )).toBe(true)
  })

  it('treats undefined values as missing keys', () => {
    expect(deepEqual({ a: 1, b: undefined }, { a: 1 })).toBe(true)
    expect(deepEqual({ a: 1 }, { a: 1, b: undefined })).toBe(true)
  })

  it('detects key-deletion via undefined eviction', () => {
    // Initial has b=2. Form sets b=undefined to "delete" it. Form is dirty.
    expect(deepEqual({ a: 1, b: 2 }, { a: 1, b: undefined })).toBe(false)
  })

  it('returns false when an array element differs', () => {
    expect(deepEqual([1, 2, 3], [1, 2, 4])).toBe(false)
  })

  it('returns false when array length differs', () => {
    expect(deepEqual([1, 2], [1, 2, 3])).toBe(false)
  })

  it('returns true for equal arrays of objects', () => {
    expect(deepEqual(
      [{ a: 1 }, { b: 2 }],
      [{ a: 1 }, { b: 2 }],
    )).toBe(true)
  })

  it('returns false when comparing array to object with same keys', () => {
    expect(deepEqual([1, 2], { 0: 1, 1: 2 })).toBe(false)
  })

  it('returns false when comparing null and {}', () => {
    expect(deepEqual(null, {})).toBe(false)
  })

  it('handles deeply nested unsaved form attributes', () => {
    const initial = {
      'fork.travel_mm': 160,
      'fork.brand': undefined,
      attributes: { wheel_size: '29' },
    }
    const current = {
      attributes: { wheel_size: '29' },
      'fork.travel_mm': 160,
    }
    expect(deepEqual(initial, current)).toBe(true)
  })
})
