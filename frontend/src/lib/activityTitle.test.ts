import { describe, expect, it } from 'vitest'
import { displayActivityTitle, editableActivityTitleSeed } from './activityTitle'
import type { Activity } from '@/api/types'

const base = {
  id: 'a1',
  user_id: 'u1',
  title: null,
  description: null,
  sport_type: 'hiking',
  sub_type: null,
  is_electric: false,
  is_virtual: false,
  is_commute: false,
  exclude_from_heatmap: false,
  exclude_from_stats: false,
  started_at: '2026-05-12T10:00:00Z',
  elapsed_time_s: 3600,
  distance_m: 10000,
  elevation_gain_m: 500,
  elevation_loss_m: 500,
  is_indoor: false,
  tags: [],
  visibility: 'private',
  enrichment_status: 'complete',
  needs_classification: false,
  has_gps: true,
  created_at: '2026-05-12T10:00:00Z',
  updated_at: '2026-05-12T10:00:00Z',
} satisfies Partial<Activity>

describe('activityTitle', () => {
  it('uses manual title before auto title', () => {
    const activity = { ...base, title: 'Mein Titel', title_auto: 'Hochplatte ab Marquartstein', title_is_manual: true } as Activity
    expect(displayActivityTitle(activity, 'Fallback')).toBe('Mein Titel')
  })

  it('uses auto title before non-manual source title', () => {
    const activity = { ...base, title: 'Morning Ride', title_auto: 'Hochplatte ab Marquartstein', title_is_manual: false } as Activity
    expect(displayActivityTitle(activity, 'Fallback')).toBe('Hochplatte ab Marquartstein')
  })

  it('falls back to source title when no auto title exists', () => {
    const activity = { ...base, title: 'Morning Ride', title_auto: null, title_is_manual: false } as Activity
    expect(displayActivityTitle(activity, 'Fallback')).toBe('Morning Ride')
  })

  it('uses fallback when no title exists', () => {
    const activity = { ...base, title: null, title_auto: null, title_is_manual: false } as Activity
    expect(displayActivityTitle(activity, 'Fallback')).toBe('Fallback')
  })

  it('ignores whitespace-only titles and trims selected titles', () => {
    const activity = { ...base, title: '   ', title_auto: '  Auto title  ', title_is_manual: true } as Activity
    expect(displayActivityTitle(activity, 'Fallback')).toBe('Auto title')
  })

  it('seeds edit form with manual title only when manual exists', () => {
    const autoOnly = { ...base, title: 'Source', title_auto: 'Auto', title_is_manual: false } as Activity
    const manual = { ...base, title: 'Manual', title_auto: 'Auto', title_is_manual: true } as Activity
    expect(editableActivityTitleSeed(autoOnly)).toBe('Auto')
    expect(editableActivityTitleSeed(manual)).toBe('Manual')
  })
})
