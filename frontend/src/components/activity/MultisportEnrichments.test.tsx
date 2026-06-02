import '@testing-library/jest-dom/vitest'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { useQueries } from '@tanstack/react-query'
import { i18n } from '@/i18n/i18n'
import type { Activity } from '@/api/types'
import MultisportEnrichments from './MultisportEnrichments'

const userPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' as 'metric' | 'imperial' }))

vi.mock('@tanstack/react-query', async importOriginal => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useQueries: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

describe('MultisportEnrichments', () => {
  afterEach(async () => {
    cleanup()
    await i18n.changeLanguage('de')
    userPrefs.locale = 'en'
    userPrefs.units = 'imperial'
    vi.mocked(useQueries).mockReset()
  })

  it('localizes visible stop, highway, and surface labels while preserving source keys', async () => {
    await i18n.changeLanguage('en')

    vi.mocked(useQueries).mockImplementation((input: {
      queries: Array<{ queryKey: readonly unknown[] }>
    }) => {
      const { queries } = input
      const queryKey = queries[0]?.queryKey[0]
      if (queryKey === 'activity-geo-tags') return [{ data: [] }] as never
      if (queryKey === 'activity-stops') {
        return [{
          data: [{
            id: 'stop-1',
            started_at: '2026-05-31T08:00:00Z',
            duration_s: 180,
          }],
        }] as never
      }
      if (queryKey === 'activity-track-summary') {
        return [{
          data: {
            surfaces: [{ type: 'Beton', length_m: 100 }],
            highways: [{ type: 'service', length_m: 100 }],
          },
        }] as never
      }
      return [] as never
    })

    render(
      <MultisportEnrichments
        childActivities={[{ id: 'activity-1' } as Activity]}
        timezone="UTC"
      />,
    )

    expect(screen.getByText('Stop')).toBeInTheDocument()
    expect(screen.getByText('Service road')).toBeInTheDocument()
    expect(screen.getByText('Concrete')).toBeInTheDocument()
    expect(screen.queryByText('Aufenthalt')).not.toBeInTheDocument()
    expect(screen.queryByText('Zufahrtsweg')).not.toBeInTheDocument()
    expect(screen.queryByText('Beton')).not.toBeInTheDocument()
  })

  it('uses localized unknown fallbacks for backend geo and stop tokens', async () => {
    await i18n.changeLanguage('en')

    vi.mocked(useQueries).mockImplementation((input: {
      queries: Array<{ queryKey: readonly unknown[] }>
    }) => {
      const { queries } = input
      const queryKey = queries[0]?.queryKey[0]
      if (queryKey === 'activity-geo-tags') {
        return [{
          data: [{
            id: 'geo-1',
            category: 'backend_new_category',
            name: 'Example point',
            dist_m: 42,
          }, {
            id: 'geo-2',
            category: 'gastronomy',
            name: 'Example stop',
            meta: { poi_type: 'backend_new_poi' },
          }],
        }] as never
      }
      if (queryKey === 'activity-stops') return [{ data: [] }] as never
      if (queryKey === 'activity-track-summary') return [{ data: { surfaces: [], highways: [] } }] as never
      return [] as never
    })

    render(
      <MultisportEnrichments
        childActivities={[{ id: 'activity-1' } as Activity]}
        timezone="UTC"
      />,
    )

    expect(screen.getAllByText('Unknown')).toHaveLength(2)
    expect(screen.queryByText('backend_new_category')).not.toBeInTheDocument()
    expect(screen.queryByText('backend_new_poi')).not.toBeInTheDocument()
  })

  it('uses localized unknown fallbacks for backend surface and highway tokens', async () => {
    await i18n.changeLanguage('en')

    vi.mocked(useQueries).mockImplementation((input: {
      queries: Array<{ queryKey: readonly unknown[] }>
    }) => {
      const { queries } = input
      const queryKey = queries[0]?.queryKey[0]
      if (queryKey === 'activity-geo-tags') return [{ data: [] }] as never
      if (queryKey === 'activity-stops') return [{ data: [] }] as never
      if (queryKey === 'activity-track-summary') {
        return [{
          data: {
            surfaces: [{ type: 'backend_new_surface', length_m: 100 }],
            highways: [{ type: 'backend_new_highway', length_m: 100 }],
          },
        }] as never
      }
      return [] as never
    })

    render(
      <MultisportEnrichments
        childActivities={[{ id: 'activity-1' } as Activity]}
        timezone="UTC"
      />,
    )

    expect(screen.getAllByText('Unknown')).toHaveLength(2)
    expect(screen.queryByText('backend_new_surface')).not.toBeInTheDocument()
    expect(screen.queryByText('backend_new_highway')).not.toBeInTheDocument()
  })

  it('formats geo tag and surface distances with selected user units', async () => {
    await i18n.changeLanguage('en')
    userPrefs.locale = 'en'
    userPrefs.units = 'imperial'

    vi.mocked(useQueries).mockImplementation((input: {
      queries: Array<{ queryKey: readonly unknown[] }>
    }) => {
      const { queries } = input
      const queryKey = queries[0]?.queryKey[0]
      if (queryKey === 'activity-geo-tags') {
        return [{
          data: [{
            id: 'geo-1',
            category: 'summit',
            name: 'Example summit',
            ele_m: 100,
            dist_m: 1609.344,
          }],
        }] as never
      }
      if (queryKey === 'activity-stops') return [{ data: [] }] as never
      if (queryKey === 'activity-track-summary') {
        return [{
          data: {
            surfaces: [{ type: 'Beton', length_m: 3218.688 }],
            highways: [],
          },
        }] as never
      }
      return [] as never
    })

    render(
      <MultisportEnrichments
        childActivities={[{ id: 'activity-1' } as Activity]}
        timezone="UTC"
      />,
    )

    expect(screen.getByText('(328 ft)')).toBeInTheDocument()
    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
    expect(screen.getByText('(2.0 mi)')).toBeInTheDocument()
    expect(screen.queryByText('(100 m)')).not.toBeInTheDocument()
    expect(screen.queryByText('1.6 km')).not.toBeInTheDocument()
  })
})
