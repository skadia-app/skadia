import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import ProfileEditor from '../ProfileEditor'

vi.mock('@/api/routes', () => ({
  getProfileDefaults: vi.fn().mockResolvedValue({ profile: 'hiking' }),
}))

vi.mock('@/lib/routingProfiles', async () => {
  const actual = await vi.importActual<typeof import('@/lib/routingProfiles')>('@/lib/routingProfiles')
  return {
    ...actual,
    getProfileDefaults: vi.fn().mockReturnValue({ profile: 'hiking' }),
    cacheServerDefaults: vi.fn(),
  }
})

describe('ProfileEditor modifier filters', () => {
  it('shows only E-Antrieb in the normal routing profile editor', () => {
    render(
      <ProfileEditor
        config={{
          profile: 'hiking',
          modifier_filters: {
            is_electric: true,
            is_indoor: false,
            is_virtual: false,
            is_commute: false,
          },
        }}
        onChange={vi.fn()}
      />,
    )

    expect(screen.getByText('E-Antrieb')).toBeInTheDocument()
    expect(screen.queryByText('Indoor')).not.toBeInTheDocument()
    expect(screen.queryByText('Virtuell')).not.toBeInTheDocument()
    expect(screen.queryByText('Pendeln')).not.toBeInTheDocument()
  })
})
