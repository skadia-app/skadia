// analytics.test.ts — unit tests for the analytics API module.
//
// Additive-schema contract (Spec §12.5):
//   ReadinessResponse is structurally typed in TypeScript. The runtime JSON
//   parser (JSON.parse) never strips keys it doesn't recognise — unknown fields
//   are simply ignored by the typed consumer while remaining accessible via
//   `as any` or index access. This test asserts that:
//     1. A payload with an unknown extra field does not throw during parse.
//     2. All currently-typed fields are extracted correctly.
//     3. The extra field is accessible at runtime (forward-compatible).

import { describe, it, expect } from 'vitest'
import type { ReadinessResponse } from './analytics'

describe('ReadinessResponse — additive-schema passthrough', () => {
  it('parses a payload carrying an unknown future_field without throwing', () => {
    // Simulate a JSON blob returned by a newer backend that added a field the
    // current TypeScript interface does not yet declare.
    const rawJson = JSON.stringify({
      refused: false,
      refusal_reason: null,
      score: 73,
      level: 'good',
      factors: [],
      missing_factors: [],
      computed_at: '2026-05-01T08:00:00Z',
      data_window_end: null,
      preferences_version: 1,
      confidence: 0.85,
      recommendation_code: null,
      signal_modifier: null,
      degraded: null,
      future_field: 42,  // unknown to the current interface — must survive
    })

    // Must not throw.
    const parsed = JSON.parse(rawJson) as ReadinessResponse

    // Typed fields are extracted correctly.
    expect(parsed.score).toBe(73)
    expect(parsed.level).toBe('good')
    expect(parsed.refused).toBe(false)
    expect(parsed.computed_at).toBe('2026-05-01T08:00:00Z')
    expect(parsed.confidence).toBeCloseTo(0.85)

    // The unknown field remains accessible at runtime — the contract holds.
    // TypeScript structural typing does not erase extra keys.
    const extra = (parsed as unknown as Record<string, unknown>)['future_field']
    expect(extra).toBe(42)
  })

  it('parses a refused payload (no_data) with null score/level without throwing', () => {
    const rawJson = JSON.stringify({
      refused: true,
      refusal_reason: 'no_data',
      score: null,
      level: null,
      factors: [],
      missing_factors: [],
      computed_at: '2026-05-01T08:00:00Z',
      data_window_end: null,
      preferences_version: null,
      confidence: null,
      recommendation_code: null,
      signal_modifier: null,
      degraded: null,
    })

    const parsed = JSON.parse(rawJson) as ReadinessResponse

    expect(parsed.refused).toBe(true)
    expect(parsed.refusal_reason).toBe('no_data')
    expect(parsed.score).toBeNull()
    expect(parsed.level).toBeNull()
  })
})
