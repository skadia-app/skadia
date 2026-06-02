import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { FusionActivityInfo } from '@/pages/FusionPage'
import FusionFieldTable from './FusionFieldTable'

const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'en',
  units: 'imperial',
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}))

function activity(overrides: Partial<FusionActivityInfo> = {}): FusionActivityInfo {
  return {
    id: 'source-a',
    label: 'A',
    color: '#3b82f6',
    deviceLabel: null,
    has_gps: true,
    has_hr: false,
    has_power: false,
    has_altitude: false,
    has_cadence: false,
    has_temperature: false,
    has_running_dynamics: false,
    distance_m: 1609.344,
    ...overrides,
  }
}

describe('FusionFieldTable units', () => {
  beforeEach(() => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
  })

  it('formats GPS source distance with the active unit preference', () => {
    render(
      <FusionFieldTable
        activities={[activity()]}
        fieldMap={{ gps: 'source-a' }}
        onFieldChange={vi.fn()}
        titleSource="source-a"
        onTitleChange={vi.fn()}
        tagsSelection="source-a"
        onTagsChange={vi.fn()}
      />,
    )

    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
    expect(screen.queryByText('1.6 km')).not.toBeInTheDocument()
  })
})
