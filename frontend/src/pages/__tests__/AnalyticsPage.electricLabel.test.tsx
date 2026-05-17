import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AnalyticsPage from '../AnalyticsPage'
import { GlobalSportSettingsProvider } from '@/lib/globalSportSettings'
import type { Capabilities } from '@/api/routes'

const mocks = vi.hoisted(() => ({
  capabilities: {
    features: {
      geo_poi_enrichment: { configured: true, available: true },
    },
  } as Capabilities,
}))

vi.mock('@/api/stats', () => ({
  fetchSportTypeStats: vi.fn(async () => []),
}))

vi.mock('@/api/tags', () => ({
  fetchTags: vi.fn(async () => []),
}))

vi.mock('@/api/gear', () => ({
  fetchGear: vi.fn(async () => []),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: mocks.capabilities }),
}))

function renderAnalyticsPage(initialPath = '/analytics/training') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <GlobalSportSettingsProvider>
        <MemoryRouter initialEntries={[initialPath]}>
          <AnalyticsPage />
        </MemoryRouter>
      </GlobalSportSettingsProvider>
    </QueryClientProvider>,
  )
}

describe('AnalyticsPage electric modifier label', () => {
  beforeEach(() => {
    mocks.capabilities = {
      features: {
        geo_poi_enrichment: { configured: true, available: true },
      },
    } as Capabilities
  })

  it('uses E-Antrieb for the generic electric filter', () => {
    renderAnalyticsPage()

    fireEvent.click(screen.getByRole('button', { name: /Filter/ }))

    expect(screen.getByText('E-Antrieb')).toBeInTheDocument()
    expect(screen.queryByText('E-Bike')).not.toBeInTheDocument()
  })

  it('hides the summit collection tab when geodata enrichment is unavailable', () => {
    mocks.capabilities = {
      features: {
        geo_poi_enrichment: { configured: false, available: false, reason: 'core_profile' },
      },
    } as Capabilities

    renderAnalyticsPage()

    expect(screen.queryByRole('link', { name: /Gipfelsammlung/i })).not.toBeInTheDocument()
  })
})
