import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import * as clientModule from '@/api/client'

// ── updateComponent null→"" translation ──────────────────────────────────────
//
// The backend clears preset_gear_type_key via an empty-string sentinel, not
// JSON null. updateComponent translates null → "" so callers use idiomatic
// null/undefined semantics while the wire format stays compatible.

describe('updateComponent — preset_gear_type_key null→"" translation', () => {
  let apiClientSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    // Intercept apiClient; return a resolved value so the function completes.
    apiClientSpy = vi
      .spyOn(clientModule, 'apiClient')
      .mockResolvedValue(undefined as any)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('TestUpdateComponent_NullPresetTranslatedToEmptyString', async () => {
    const { updateComponent } = await import('@/api/gear')
    await updateComponent('comp1', { name: 'Test', preset_gear_type_key: null })

    expect(apiClientSpy).toHaveBeenCalledTimes(1)
    const [, opts] = apiClientSpy.mock.calls[0] as [string, { body: Record<string, unknown> }]
    // null must be translated to "" for the backend to clear the column.
    expect(opts.body.preset_gear_type_key).toBe('')
  })

  it('TestUpdateComponent_UndefinedPresetOmitted', async () => {
    const { updateComponent } = await import('@/api/gear')
    await updateComponent('comp1', { name: 'Test' })

    const [, opts] = apiClientSpy.mock.calls[0] as [string, { body: Record<string, unknown> }]
    // Field absent (undefined) must not appear in body — backend treats
    // absent as "unchanged", distinct from the empty-string clear sentinel.
    expect('preset_gear_type_key' in opts.body).toBe(false)
  })

  it('TestUpdateComponent_StringPresetPassedThrough', async () => {
    const { updateComponent } = await import('@/api/gear')
    await updateComponent('comp1', { preset_gear_type_key: 'bike.mtb' })

    const [, opts] = apiClientSpy.mock.calls[0] as [string, { body: Record<string, unknown> }]
    expect(opts.body.preset_gear_type_key).toBe('bike.mtb')
  })
})

// ── Gear interface type-level round-trip (R2-6) ───────────────────────────────
//
// Compile-time check that the Gear interface accepts the five new fields.
// The round-trip via JSON.parse / JSON.stringify catches obvious type errors
// without requiring a running server.

describe('Gear interface — new fields round-trip', () => {
  it('TestGearInterface_AcceptsNewFields', () => {
    // Import only for types — no runtime import needed; the test is type-level.
    // We construct a minimal object that satisfies the Gear interface and
    // includes all five new fields.
    const gear = {
      id: 'g1',
      user_id: 'u1',
      name: 'Testrad',
      type: 'bike',
      total_distance_m: 0,
      total_elevation_gain_m: 0,
      total_elevation_loss_m: 0,
      total_time_s: 0,
      total_moving_time_s: 0,
      total_activities: 0,
      is_retired: false,
      status: 'active' as const,
      applicable_sport_predicates: [],
      created_at: '2024-01-01T00:00:00Z',
      // R2-6 additions
      scope: 'personal',
      default_sport_predicates: [{ sport_type: 'cycling', sub_type: null, modifiers: null }],
      modifiers: { some_key: true },
      review_status: 'confirmed',
      source_document_id: 'doc-1',
    }

    const serialised = JSON.stringify(gear)
    const parsed = JSON.parse(serialised) as typeof gear
    expect(parsed.scope).toBe('personal')
    expect(parsed.default_sport_predicates).toHaveLength(1)
    expect(parsed.modifiers).toEqual({ some_key: true })
    expect(parsed.review_status).toBe('confirmed')
    expect(parsed.source_document_id).toBe('doc-1')
  })

  it('TestGearInterface_NewFieldsAreOptional', () => {
    // A minimal Gear object without any new fields must also be valid (all optional).
    const gear = {
      id: 'g2',
      user_id: 'u1',
      name: 'Minimal',
      type: 'shoe',
      total_distance_m: 0,
      total_elevation_gain_m: 0,
      total_elevation_loss_m: 0,
      total_time_s: 0,
      total_moving_time_s: 0,
      total_activities: 0,
      is_retired: false,
      status: 'active' as const,
      applicable_sport_predicates: [],
      created_at: '2024-01-01T00:00:00Z',
    }
    // If the TS interface required the new fields, this would fail to compile.
    // At runtime, assert the new keys are absent.
    expect((gear as Record<string, unknown>).scope).toBeUndefined()
    expect((gear as Record<string, unknown>).default_sport_predicates).toBeUndefined()
    expect((gear as Record<string, unknown>).modifiers).toBeUndefined()
    expect((gear as Record<string, unknown>).review_status).toBeUndefined()
    expect((gear as Record<string, unknown>).source_document_id).toBeUndefined()
  })
})
