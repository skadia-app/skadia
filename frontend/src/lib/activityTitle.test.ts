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

  it('keeps non-manual source title before auto title', () => {
    const activity = { ...base, title: 'Morning Ride', title_auto: 'Hochplatte ab Marquartstein', title_is_manual: false } as Activity
    expect(displayActivityTitle(activity, 'Fallback')).toBe('Morning Ride')
  })

  it('keeps a non-generic server-resolved title even when title_auto is present', () => {
    const activity = { ...base, title: 'Kampenwand mit Max', title_auto: 'Hochplatte ab Marquartstein', title_is_manual: false } as Activity
    expect(displayActivityTitle(activity, 'Fallback')).toBe('Kampenwand mit Max')
  })

  it('uses server-resolved display title before local fallback fields', () => {
    const activity = { ...base, title: 'Morning Ride', display_title: 'Hochplatte ab Marquartstein', title_auto: 'Hochplatte ab Marquartstein', title_is_manual: false } as Activity
    expect(displayActivityTitle(activity, 'Fallback')).toBe('Hochplatte ab Marquartstein')
  })

  it.each([
    'Mountainbike-Fahrt am Morgen',
    'Lauf am Nachmittag',
    'E-Mountainbike-Fahrt am Morgen',
    'Mountainbike-Fahrt am Abend',
    'Abendlauf',
    'Mountainbike-Fahrt zur Mittagszeit',
    'Mittagslauf',
    'Morning Mountain Bike Ride',
    'Afternoon Ride',
  ])('keeps provider source title %s when falling back locally', (title) => {
    const activity = { ...base, title, title_auto: 'Hochplatte ab Marquartstein', title_is_manual: false } as Activity
    expect(displayActivityTitle(activity, 'Fallback')).toBe(title)
  })

  it.each([
    'Recovery Run',
    'Alpine Trail Run',
    'Birthday Ride',
    'Coffee Ride',
    'Lauf mit Max',
    'Kampenwand-Lauf',
    'Mountainbike-Fahrt mit Max',
    'Radfahren zur Arbeit',
  ])('keeps meaningful source title %s when falling back locally', (title) => {
    const activity = { ...base, title, title_auto: 'Hochplatte ab Marquartstein', title_is_manual: false } as Activity
    expect(displayActivityTitle(activity, 'Fallback')).toBe(title)
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
    const autoOnly = { ...base, title: 'Source', display_title: 'Auto', title_auto: 'Auto', title_is_manual: false } as Activity
    const manual = { ...base, title: 'Manual', title_auto: 'Auto', title_is_manual: true } as Activity
    expect(editableActivityTitleSeed(autoOnly)).toBe('Auto')
    expect(editableActivityTitleSeed(manual)).toBe('Manual')
  })
})
