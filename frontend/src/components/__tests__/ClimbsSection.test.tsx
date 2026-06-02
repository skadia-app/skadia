import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import type { Climb } from '@/api/types'

let mockClimbs: Climb[] = []
const mockUserPrefs = vi.hoisted((): { locale: 'de' | 'en'; units: 'metric' | 'imperial' } => ({
  locale: 'de',
  units: 'metric',
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: mockClimbs, isLoading: false, isError: false }),
}))

vi.mock('@/api/enrichment', () => ({
  fetchActivityClimbs: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

import '@/i18n/i18n'
import ClimbsSection from '../ClimbsSection'

describe('ClimbsSection', () => {
  beforeEach(() => {
    mockClimbs = [{
      id: 'climb-1',
      climb_number: 1,
      start_distance_m: 1000,
      end_distance_m: 1600,
      length_m: 600,
      elevation_gain_m: 42,
      avg_gradient_pct: 7,
      max_gradient_pct: 11,
      category: '4',
      sport_class: null,
    }]
    mockUserPrefs.locale = 'de'
    mockUserPrefs.units = 'metric'
  })

  it('explains that max gradient uses a 50m window', () => {
    render(<ClimbsSection activityId="act-1" />)

    expect(screen.getByText('Max. Steigung')).toBeInTheDocument()
    expect(screen.getByTitle(/50-m-Abschnitt/)).toBeInTheDocument()
  })

  it('formats climb length and elevation with user units', () => {
    mockUserPrefs.units = 'imperial'

    render(<ClimbsSection activityId="act-1" />)

    expect(screen.getByText('1.969 ft')).toBeInTheDocument()
    expect(screen.getByText('138 ft')).toBeInTheDocument()
    expect(screen.queryByText('600 m')).not.toBeInTheDocument()
    expect(screen.queryByText('42 m')).not.toBeInTheDocument()
  })
})
