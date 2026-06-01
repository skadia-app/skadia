import '@testing-library/jest-dom/vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { i18n } from '@/i18n/i18n'
import ProfileEditor from '../ProfileEditor'

vi.mock('@/api/routes', () => ({
  getProfileDefaults: vi.fn().mockResolvedValue({
    profile: 'hiking',
    path_cost: 1,
    track_cost: 1,
    track_grade1_cost: 1,
    track_grade2_cost: 1,
    track_grade3_cost: 1,
    track_grade4_cost: 1,
    track_grade5_cost: 1,
    footway_cost: 1,
    cycleway_cost: 1,
    bridleway_cost: 1,
    pedestrian_cost: 1,
    residential_cost: 1,
    service_cost: 1,
    unclassified_cost: 1,
    tertiary_cost: 1,
    secondary_cost: 1,
    primary_cost: 1,
  }),
}))

vi.mock('@/lib/routingProfiles', async () => {
  const actual = await vi.importActual<typeof import('@/lib/routingProfiles')>('@/lib/routingProfiles')
  return {
    ...actual,
    getProfileDefaults: vi.fn().mockReturnValue({ profile: 'hiking' }),
    cacheServerDefaults: vi.fn(),
  }
})

describe('ProfileEditor highway labels', () => {
  it('renders highway cost labels from the locale catalog', async () => {
    await i18n.changeLanguage('de')

    render(<ProfileEditor config={{ profile: 'hiking' }} onChange={vi.fn()} />)

    fireEvent.click(await screen.findByRole('button', { name: /Wegekosten/i }))

    expect(screen.getByText('Pfad')).toBeInTheDocument()
    expect(screen.getByText('Wirtschaftsweg (Basis)')).toBeInTheDocument()
    expect(screen.queryByText('Path')).not.toBeInTheDocument()
    expect(screen.queryByText('Track (base)')).not.toBeInTheDocument()
  })
})
