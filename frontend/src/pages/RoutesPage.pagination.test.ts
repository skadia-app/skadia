import { describe, expect, it } from 'vitest'

import { nextRoutesOffset } from './RoutesPage'

describe('routes pagination helpers', () => {
  it('returns the next offset while more routes exist', () => {
    expect(nextRoutesOffset({ total: 129, offset: 0, limit: 50, routes: Array.from({ length: 50 }) })).toBe(50)
    expect(nextRoutesOffset({ total: 129, offset: 50, limit: 50, routes: Array.from({ length: 50 }) })).toBe(100)
  })

  it('stops after the final partial page', () => {
    expect(nextRoutesOffset({ total: 129, offset: 100, limit: 50, routes: Array.from({ length: 29 }) })).toBeUndefined()
  })
})
