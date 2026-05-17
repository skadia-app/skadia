import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/lib/toast'
import type { Capabilities } from '@/api/routes'
import WorkshopReprocessPage from './WorkshopReprocessPage'

const mocks = vi.hoisted(() => ({
  capabilities: {
    features: {
      geo_poi_enrichment: { configured: false, available: false, reason: 'core_profile' },
      surface_enrichment: { configured: false, available: false, reason: 'core_profile' },
      osm_lift_snap: { configured: false, available: false, reason: 'core_profile' },
    },
  } as Capabilities,
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: mocks.capabilities }),
}))

vi.mock('@/api/activities', () => ({
  fetchActivities: vi.fn(),
  reprocessAllActivities: vi.fn(),
  isReprocessAccepted: vi.fn(),
}))

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <MemoryRouter>
      <ToastProvider>
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </ToastProvider>
    </MemoryRouter>
  )
}

describe('WorkshopReprocessPage capability gates', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('does not show full-only reprocess scopes in core capability mode', () => {
    render(<WorkshopReprocessPage />, { wrapper })

    expect(screen.queryByText('Gipfel & Pässe')).not.toBeInTheDocument()
    expect(screen.queryByText('Stopps & Einkehr')).not.toBeInTheDocument()
    expect(screen.queryByText('Untergrundtypen')).not.toBeInTheDocument()
    expect(screen.getByText('Anstiege')).toBeInTheDocument()
    expect(screen.getByText('Lifte / Seilbahnen')).toBeInTheDocument()
  })
})
