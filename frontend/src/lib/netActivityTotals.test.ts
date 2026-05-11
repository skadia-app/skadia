import { describe, it, expect } from 'vitest'
import { getNetActivityTotals } from './netActivityTotals'
import type { Activity } from '@/api/types'

function makeActivity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'test-id',
    user_id: 'user-1',
    title: 'Test',
    sport_type: 'cycling',
    started_at: '2024-01-01T10:00:00Z',
    elapsed_time_s: 3600,
    has_gps: false,
    is_virtual: false,
    tags: [],
    ...overrides,
  } as Activity
}

describe('getNetActivityTotals', () => {
  it('returns unmodified values when activity has no lifts', () => {
    const activity = makeActivity({
      distance_m: 10000,
      elevation_gain_m: 500,
      elevation_loss_m: 400,
    })
    const result = getNetActivityTotals(activity)
    expect(result.distanceM).toBe(10000)
    expect(result.elevationGainM).toBe(500)
    expect(result.elevationLossM).toBe(400)
  })

  it('returns unmodified values when exclude_lift_subtraction is true', () => {
    const activity = makeActivity({
      distance_m: 10000,
      elevation_gain_m: 500,
      elevation_loss_m: 400,
      lift_distance_m: 2000,
      lift_ascent_m: 300,
      lift_descent_m: 200,
      exclude_lift_subtraction: true,
    })
    const result = getNetActivityTotals(activity)
    expect(result.distanceM).toBe(10000)
    expect(result.elevationGainM).toBe(500)
    expect(result.elevationLossM).toBe(400)
  })

  it('subtracts lift values when exclude_lift_subtraction is false', () => {
    const activity = makeActivity({
      distance_m: 10000,
      elevation_gain_m: 500,
      elevation_loss_m: 400,
      lift_distance_m: 2000,
      lift_ascent_m: 300,
      lift_descent_m: 200,
      exclude_lift_subtraction: false,
    })
    const result = getNetActivityTotals(activity)
    expect(result.distanceM).toBe(8000)
    expect(result.elevationGainM).toBe(200)
    expect(result.elevationLossM).toBe(200)
  })

  it('subtracts lift values when exclude_lift_subtraction is undefined', () => {
    const activity = makeActivity({
      distance_m: 5000,
      elevation_gain_m: 200,
      elevation_loss_m: 150,
      lift_distance_m: 1000,
      lift_ascent_m: 100,
      lift_descent_m: 80,
    })
    const result = getNetActivityTotals(activity)
    expect(result.distanceM).toBe(4000)
    expect(result.elevationGainM).toBe(100)
    expect(result.elevationLossM).toBe(70)
  })

  it('clamps to zero when lift values exceed activity values (defensive)', () => {
    const activity = makeActivity({
      distance_m: 500,
      elevation_gain_m: 100,
      elevation_loss_m: 80,
      lift_distance_m: 2000,
      lift_ascent_m: 300,
      lift_descent_m: 200,
      exclude_lift_subtraction: false,
    })
    const result = getNetActivityTotals(activity)
    expect(result.distanceM).toBe(0)
    expect(result.elevationGainM).toBe(0)
    expect(result.elevationLossM).toBe(0)
  })

  it('returns zeros when all fields are undefined', () => {
    const activity = makeActivity()
    const result = getNetActivityTotals(activity)
    expect(result.distanceM).toBe(0)
    expect(result.elevationGainM).toBe(0)
    expect(result.elevationLossM).toBe(0)
  })
})
