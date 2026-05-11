import { describe, expect, it } from 'vitest'
import { shouldRenderLiftSection } from './ActivityDetailPage'
import type { Activity } from '@/api/types'

function makeActivity(overrides: Partial<Activity>): Activity {
  return {
    id: 'act-1',
    user_id: 'user-1',
    title: 'Test Activity',
    sport_type: 'cycling',
    started_at: '2024-01-01T08:00:00Z',
    elapsed_time_s: 7200,
    has_gps: true,
    is_virtual: false,
    tags: [],
    lift_count: 0,
    ...overrides,
  } as Activity
}

describe('shouldRenderLiftSection', () => {
  it('renders for confirmed lift totals', () => {
    expect(shouldRenderLiftSection(makeActivity({ lift_count: 1 }), 0)).toBe(true)
  })

  it('renders for heuristic-only pending candidates before lift_count changes', () => {
    expect(shouldRenderLiftSection(makeActivity({ lift_count: 0 }), 2)).toBe(true)
  })

  it('stays hidden when neither confirmed lifts nor unreviewed heuristic candidates exist', () => {
    expect(shouldRenderLiftSection(makeActivity({ lift_count: 0 }), 0)).toBe(false)
  })
})
